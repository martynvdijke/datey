package notifier

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/smtp"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/datey/datey/internal/config"
)

//go:embed email_template.html
var emailTemplateFS embed.FS

type emailTemplateData struct {
	Title     string
	Message   string
	Timestamp string
}

type EmailNotifier struct {
	cfg  *config.Config
	tmpl *template.Template

	// icsGenerator renders the .ics attachment body. It is a field (rather
	// than a bare package function) so tests can inject a failure and prove
	// that a broken attachment never blocks delivery.
	icsGenerator func(title, message string, now time.Time) (string, error)
}

func NewEmailNotifier(cfg *config.Config) *EmailNotifier {
	tmpl := template.Must(template.ParseFS(emailTemplateFS, "email_template.html"))
	return &EmailNotifier{cfg: cfg, tmpl: tmpl, icsGenerator: generateICS}
}

func (n *EmailNotifier) Name() string { return "email" }

func (n *EmailNotifier) IsConfigured() bool {
	return n.cfg.SMTPHost != "" && n.cfg.NotifyEmail != ""
}

func (n *EmailNotifier) Send(ctx context.Context, title, message string) error {
	return n.SendTo(ctx, title, message, "")
}

func (n *EmailNotifier) SendTo(ctx context.Context, title, message string, target string) error {
	addr := fmt.Sprintf("%s:%d", n.cfg.SMTPHost, n.cfg.SMTPPort)
	recipient := n.cfg.NotifyEmail
	if target != "" {
		recipient = target
	}

	// Render HTML body with plain-text fallback
	htmlBody, htmlErr := n.renderHTML(title, message)
	body := message
	contentType := "text/plain; charset=\"UTF-8\""
	if htmlErr == nil {
		body = htmlBody
		contentType = "text/html; charset=\"UTF-8\""
	} else {
		slog.Warn("email HTML rendering failed, falling back to plain text", "source", "notifier", "error", htmlErr)
	}

	// Optional "Add to calendar" attachment. Any failure here must never stop
	// the notification from going out.
	icsBody, icsFilename := n.buildICS(title, message)

	timeout := time.Duration(n.cfg.SMTPTimeout) * time.Second

	// Build the full message
	fullMsg := buildEmailMessage(n.cfg.SMTPUser, recipient, title, contentType, body, icsBody, icsFilename)

	// Choose connection mode
	switch {
	case n.cfg.SMTPPort == 465:
		return n.sendDirectTLS(ctx, addr, fullMsg, recipient, timeout)
	case n.cfg.SMTPTLS:
		return n.sendSTARTTLS(ctx, addr, fullMsg, recipient, timeout)
	default:
		return n.sendPlain(ctx, addr, fullMsg, recipient, timeout)
	}
}

// buildICS renders the iCalendar attachment, returning an empty body and
// filename when no attachment should be added. Failures are logged and
// swallowed: the email is still delivered without the attachment.
func (n *EmailNotifier) buildICS(title, message string) (body, filename string) {
	if isDigest(title, message) {
		// A single VEVENT would misrepresent a multi-event digest.
		slog.Debug("email: skipping calendar attachment for digest message", "source", "notifier")
		return "", ""
	}

	gen := n.icsGenerator
	if gen == nil {
		gen = generateICS
	}

	ics, err := gen(title, message, time.Now())
	if err != nil {
		slog.Warn("email: calendar attachment generation failed, sending without it", "source", "notifier", "error", err)
		return "", ""
	}
	if ics == "" {
		return "", ""
	}
	return ics, icsFilename(title)
}

// buildEmailMessage assembles the RFC 5322 message. With no icsBody it keeps
// the original single-part body; with one it emits a multipart/mixed message
// carrying the text/html body plus the text/calendar attachment.
func buildEmailMessage(from, to, subject, contentType, body, icsBody, icsFilename string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")

	if icsBody == "" {
		fmt.Fprintf(&b, "Content-Type: %s\r\n\r\n", contentType)
		b.WriteString(body)
		return b.String()
	}

	boundary := fmt.Sprintf("datey-%d", time.Now().UnixNano())
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary)

	b.WriteString("--" + boundary + "\r\n")
	fmt.Fprintf(&b, "Content-Type: %s\r\n\r\n", contentType)
	b.WriteString(body)
	if !strings.HasSuffix(body, "\r\n") {
		b.WriteString("\r\n")
	}

	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/calendar; charset=utf-8; method=PUBLISH\r\n")
	fmt.Fprintf(&b, "Content-Disposition: attachment; filename=\"%s\"\r\n", icsFilename)
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(base64Lines(icsBody))

	b.WriteString("\r\n--" + boundary + "--\r\n")
	return b.String()
}

func (n *EmailNotifier) sendDirectTLS(ctx context.Context, addr, msg string, recipient string, timeout time.Duration) error {
	tlsCfg := &tls.Config{ServerName: n.cfg.SMTPHost}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, n.cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	return n.sendWithClient(client, msg, recipient)
}

func (n *EmailNotifier) sendSTARTTLS(ctx context.Context, addr, msg string, recipient string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	client, err := smtp.NewClient(conn, n.cfg.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}

	if err := client.StartTLS(&tls.Config{ServerName: n.cfg.SMTPHost}); err != nil {
		_ = client.Close()
		return fmt.Errorf("starttls: %w", err)
	}

	err = n.sendWithClient(client, msg, recipient)
	_ = client.Close()
	return err
}

func (n *EmailNotifier) sendPlain(ctx context.Context, addr, msg string, recipient string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	client, err := smtp.NewClient(conn, n.cfg.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}

	err = n.sendWithClient(client, msg, recipient)
	_ = client.Close()
	return err
}

func (n *EmailNotifier) sendWithClient(client *smtp.Client, msg string, recipient string) error {
	// Authenticate if credentials provided
	if n.cfg.SMTPUser != "" {
		auth := smtp.PlainAuth("", n.cfg.SMTPUser, n.cfg.SMTPPass, n.cfg.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}

	// Mail from
	if err := client.Mail(n.cfg.SMTPUser); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}

	// Recipient
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("rcpt: %w", err)
	}

	// Data
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}

	if _, err := w.Write([]byte(msg)); err != nil {
		_ = w.Close()
		return fmt.Errorf("write: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}

	return client.Quit()
}

func (n *EmailNotifier) renderHTML(title, message string) (string, error) {
	var buf bytes.Buffer
	err := n.tmpl.Execute(&buf, emailTemplateData{
		Title:     title,
		Message:   message,
		Timestamp: time.Now().Format("January 2, 2006 at 3:04 PM"),
	})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

const (
	// icsLineOctets is the RFC 5545 content-line octet limit, excluding CRLF.
	icsLineOctets = 75
	// icsProdid identifies the generator in the VCALENDAR header.
	icsProdid = "-//Datey//Datey Reminder//EN"
)

// reminderDateRe matches the calendar date embedded in reminder messages by
// the scheduler, e.g. "Upcoming birthday for Dana on June 15 (14 days away)"
// or "Missed reminder: birthday for Dana was June 15 (2 days ago)". The year is
// not part of the message, so it is resolved separately (see resolveEventDate).
var reminderDateRe = regexp.MustCompile(`(?i)\b(?:on|was)\s+([A-Za-z]+)\s+(\d{1,2})\b`)

// isDigest reports whether the message describes more than one event, in which
// case a single VEVENT attachment would be wrong. No digest renderer exists in
// this codebase yet, so detection is heuristic: an explicit "digest" marker or
// more than one reminder date in the same message.
func isDigest(title, message string) bool {
	if strings.Contains(strings.ToLower(title), "digest") || strings.Contains(strings.ToLower(message), "digest") {
		return true
	}
	return len(reminderDateRe.FindAllString(message, -1)) > 1
}

// generateICS renders a minimal RFC 5545 single-event calendar. The calendar
// package (internal/ical) is not reused because its event type has no
// TRANSP property, omits DTEND for all-day events, and keeps its escaping and
// folding helpers unexported; reproducing the few lines needed here avoids
// refactoring a shared renderer for notification-only concerns.
func generateICS(title, message string, now time.Time) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("cannot build calendar event: empty summary")
	}

	date := resolveEventDate(message, now)
	uid := icsUID(title, message, date)

	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:" + icsProdid + "\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	b.WriteString("METHOD:PUBLISH\r\n")
	b.WriteString("BEGIN:VEVENT\r\n")
	writeICSProperty(&b, "UID", uid)
	writeICSProperty(&b, "DTSTAMP", now.UTC().Format("20060102T150405Z"))
	writeICSProperty(&b, "DTSTART;VALUE=DATE", date.Format("20060102"))
	// DTEND is exclusive for DATE values: the day after the event.
	writeICSProperty(&b, "DTEND;VALUE=DATE", date.AddDate(0, 0, 1).Format("20060102"))
	writeICSProperty(&b, "SUMMARY", escapeICSText(title))
	writeICSProperty(&b, "DESCRIPTION", escapeICSText(message))
	writeICSProperty(&b, "TRANSP", "TRANSPARENT")
	b.WriteString("END:VEVENT\r\n")
	b.WriteString("END:VCALENDAR\r\n")
	return b.String(), nil
}

// resolveEventDate extracts the event date from a reminder message. The
// scheduler formats dates as "January 2" without a year, so the year is chosen
// as the candidate nearest to now, which also handles year boundaries (a
// December date reminded about in January resolves to the previous year).
// When no date is present the event is dated today.
func resolveEventDate(message string, now time.Time) time.Time {
	m := reminderDateRe.FindStringSubmatch(message)
	if m == nil {
		return startOfDay(now)
	}

	monthDay, err := time.Parse("January 2", m[1]+" "+m[2])
	if err != nil {
		return startOfDay(now)
	}

	now = startOfDay(now)
	best := time.Date(now.Year(), monthDay.Month(), monthDay.Day(), 0, 0, 0, 0, time.UTC)
	bestDiff := absDuration(best.Sub(now))
	for _, year := range []int{now.Year() - 1, now.Year() + 1} {
		candidate := time.Date(year, monthDay.Month(), monthDay.Day(), 0, 0, 0, 0, time.UTC)
		if diff := absDuration(candidate.Sub(now)); diff < bestDiff {
			best, bestDiff = candidate, diff
		}
	}
	return best
}

func startOfDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// icsUID derives a stable identifier so a re-sent reminder updates the same
// calendar entry instead of creating a duplicate.
func icsUID(title, message string, date time.Time) string {
	sum := sha256.Sum256([]byte(title + "\x00" + message + "\x00" + date.Format("20060102")))
	return fmt.Sprintf("datey-%x@datey", sum[:12])
}

// icsFilename builds the attachment filename from the notification title.
func icsFilename(title string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
		if b.Len() >= 48 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "datey-reminder"
	}
	return slug + ".ics"
}

// writeICSProperty writes one content line, folding it at the RFC 5545
// 75-octet limit with a single leading space on continuation lines.
func writeICSProperty(b *strings.Builder, name, value string) {
	segs := foldICSLine(name + ":" + value)
	b.WriteString(segs[0])
	b.WriteString("\r\n")
	for _, s := range segs[1:] {
		b.WriteString(" ")
		b.WriteString(s)
		b.WriteString("\r\n")
	}
}

// foldICSLine splits a content line into segments of at most 75 octets while
// never splitting a UTF-8 rune. Continuation segments are capped at 74 octets
// because of the leading folding space.
func foldICSLine(line string) []string {
	if line == "" {
		return []string{""}
	}
	var segs []string
	max := icsLineOctets
	for len(line) > 0 {
		if len(line) <= max {
			segs = append(segs, line)
			break
		}
		cut := max
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		if cut == 0 {
			cut = max
		}
		segs = append(segs, line[:cut])
		line = line[cut:]
		max = icsLineOctets - 1
	}
	return segs
}

// escapeICSText escapes a TEXT value per RFC 5545 §3.3.11.
func escapeICSText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case ';':
			b.WriteString(`\;`)
		case ',':
			b.WriteString(`\,`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			// Bare CR is dropped; breaks are encoded as "\n".
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// base64Lines encodes s as base64 wrapped at 76 octets per line with CRLF
// terminators, as required for a MIME body.
func base64Lines(s string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76])
		b.WriteString("\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc)
	return b.String()
}
