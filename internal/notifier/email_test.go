package notifier

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/internal/config"
)

func newTestConfig() *config.Config {
	return &config.Config{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		SMTPUser:    "user@example.com",
		SMTPPass:    "secret",
		SMTPTLS:     true,
		SMTPTimeout: 10,
		NotifyEmail: "test@example.com",
	}
}

func TestEmailNotifier_Name(t *testing.T) {
	n := NewEmailNotifier(newTestConfig())
	if got := n.Name(); got != "email" {
		t.Errorf("Name() = %q, want %q", got, "email")
	}
}

func TestEmailNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"fully configured", newTestConfig(), true},
		{"missing host", &config.Config{SMTPHost: "", NotifyEmail: "a@b.com"}, false},
		{"missing email", &config.Config{SMTPHost: "smtp.example.com", NotifyEmail: ""}, false},
		{"both missing", &config.Config{SMTPHost: "", NotifyEmail: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewEmailNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmailNotifier_RenderHTML(t *testing.T) {
	n := NewEmailNotifier(newTestConfig())
	html, err := n.renderHTML("Test Title", "Test Message")
	if err != nil {
		t.Fatalf("renderHTML() error = %v", err)
	}
	if !strings.Contains(html, "Test Title") {
		t.Errorf("renderHTML() missing title")
	}
	if !strings.Contains(html, "Test Message") {
		t.Errorf("renderHTML() missing message")
	}
	if !strings.Contains(html, "Sent at") {
		t.Errorf("renderHTML() missing timestamp")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Errorf("renderHTML() missing doctype")
	}
}

func TestEmailNotifier_Send_ReturnsErrorOnNoServer(t *testing.T) {
	cfg := newTestConfig()
	cfg.SMTPHost = "127.0.0.1"
	cfg.SMTPPort = 51999
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	err := n.Send(context.Background(), "Test", "Body")
	if err == nil {
		t.Fatal("Send() expected error, got nil")
	}
}

// generateTestCert creates a self-signed TLS cert for testing.
func generateTestCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create cert: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}

func TestEmailNotifier_TLSModeDirect(t *testing.T) {
	// Test sendDirectTLS by connecting to a plain TCP listener.
	// The TLS handshake will fail since the server doesn't speak TLS.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 1024)
		conn.Read(buf)
	}()

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = host
	cfg.SMTPPort = port
	cfg.SMTPTLS = true
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	err = n.sendDirectTLS(context.Background(), addr, "test msg", "test@example.com", time.Second)
	<-serverDone

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tls dial") {
		t.Errorf("error = %q, want substring %q", err.Error(), "tls dial")
	}
}

func TestEmailNotifier_TLSModeSTARTTLS(t *testing.T) {
	// Test sendSTARTTLS: server sends greeting but doesn't support STARTTLS
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Send SMTP greeting but no STARTTLS capability
		conn.Write([]byte("220 localhost ESMTP test\r\n"))
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 1024)
		conn.Read(buf)
	}()

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = host
	cfg.SMTPPort = port
	cfg.SMTPTLS = true
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	err = n.sendSTARTTLS(context.Background(), addr, "test msg", "test@example.com", time.Second)
	<-serverDone

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "starttls") {
		t.Errorf("error = %q, want substring %q", err.Error(), "starttls")
	}
}

func TestEmailNotifier_TLSModePlain(t *testing.T) {
	// Test sendPlain: connects to a server that doesn't handle SMTP properly
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		conn.Read(buf)
	}()

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = host
	cfg.SMTPPort = port
	cfg.SMTPTLS = false
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	err = n.sendPlain(context.Background(), addr, "test msg", "test@example.com", time.Second)
	<-serverDone

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEmailNotifier_SMTPTimeoutConfig(t *testing.T) {
	// Verify the SMTPTimeout config value is respected by the dial timeout.
	// Connect to a port where nobody is listening — should fail fast.
	cfg := newTestConfig()
	cfg.SMTPHost = "127.0.0.1"
	cfg.SMTPPort = 51999
	cfg.SMTPTimeout = 1

	n := NewEmailNotifier(cfg)
	start := time.Now()
	err := n.Send(context.Background(), "Test", "Body")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if elapsed > 3*time.Second {
		t.Errorf("Send() took %v, expected fast failure with 1s timeout", elapsed)
	}
}

func TestEmailNotifier_Send_SMTPDialog(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		conn.Write([]byte("220 localhost ESMTP test\r\n"))

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line := string(buf[:n])
		if !strings.HasPrefix(line, "EHLO") && !strings.HasPrefix(line, "HELO") {
			serverDone <- fmt.Errorf("expected EHLO/HELO, got: %s", line)
			return
		}
		conn.Write([]byte("250-localhost\r\n250 AUTH LOGIN PLAIN\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "AUTH") {
			serverDone <- fmt.Errorf("expected AUTH, got: %s", line)
			return
		}
		conn.Write([]byte("235 Authentication successful\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "MAIL FROM") {
			serverDone <- fmt.Errorf("expected MAIL FROM, got: %s", line)
			return
		}
		conn.Write([]byte("250 OK\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "RCPT TO") {
			serverDone <- fmt.Errorf("expected RCPT TO, got: %s", line)
			return
		}
		conn.Write([]byte("250 OK\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "DATA") {
			serverDone <- fmt.Errorf("expected DATA, got: %s", line)
			return
		}
		conn.Write([]byte("354 Start mail input\r\n"))

		var msgBuf strings.Builder
		for {
			b := make([]byte, 1)
			_, err := conn.Read(b)
			if err != nil {
				break
			}
			msgBuf.WriteByte(b[0])
			s := msgBuf.String()
			if strings.HasSuffix(s, "\r\n.\r\n") {
				break
			}
		}
		conn.Write([]byte("250 OK: queued\r\n"))

		n, err = conn.Read(buf)
		if err != nil {
			serverDone <- fmt.Errorf("read error: %w", err)
			return
		}
		line = string(buf[:n])
		if !strings.HasPrefix(line, "QUIT") {
			serverDone <- fmt.Errorf("expected QUIT, got: %s", line)
			return
		}
		conn.Write([]byte("221 Bye\r\n"))

		serverDone <- nil
		_ = msgBuf
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = host
	cfg.SMTPPort = port
	cfg.SMTPTLS = false
	cfg.SMTPTimeout = 2

	n := NewEmailNotifier(cfg)
	err = n.Send(context.Background(), "Test Subject", "Hello Body")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("SMTP server error: %v", err)
	}
}

func TestEmailNotifier_TLS_Direct(t *testing.T) {
	cert := generateTestCert(t)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	if err != nil {
		t.Fatalf("failed to start TLS listener: %v", err)
	}
	defer ln.Close()

	serverDone := make(chan error, 1)
	go func() {
		defer func() { serverDone <- nil }()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		conn.Write([]byte("220 localhost ESMTP test\r\n"))

		buf := make([]byte, 1024)
		_, err = conn.Read(buf)
		if err != nil {
			return
		}

		// Read remaining commands until quit
		for {
			b := make([]byte, 1024)
			_, err := conn.Read(b)
			if err != nil {
				break
			}
			conn.Write([]byte("250 OK\r\n"))
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := newTestConfig()
	cfg.SMTPHost = "localhost"
	cfg.SMTPPort = port
	cfg.SMTPTLS = true
	cfg.SMTPTimeout = 2

	n := NewEmailNotifier(cfg)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	err = n.sendDirectTLS(context.Background(), addr, "", "test@example.com", time.Duration(cfg.SMTPTimeout)*time.Second)
	<-serverDone

	// Expect TLS cert verification error (self-signed cert not trusted)
	if err == nil {
		t.Fatal("expected TLS verification error, got nil")
	}
	if !strings.Contains(err.Error(), "tls: failed to verify certificate") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// captureSentMessage runs a throwaway SMTP server, sends one message through
// the email notifier and returns the raw DATA payload (dot-unstuffed).
func captureSentMessage(t *testing.T, mutate func(*EmailNotifier), title, message string) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	payloads := make(chan string, 1)
	serverErr := make(chan error, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		if _, err := conn.Write([]byte("220 localhost ESMTP test\r\n")); err != nil {
			serverErr <- err
			return
		}

		r := bufio.NewReader(conn)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				serverErr <- err
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				conn.Write([]byte("250-localhost\r\n250 AUTH PLAIN\r\n"))
			case strings.HasPrefix(cmd, "HELO"):
				conn.Write([]byte("250 localhost\r\n"))
			case strings.HasPrefix(cmd, "AUTH"):
				conn.Write([]byte("235 Authentication successful\r\n"))
			case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RCPT TO"):
				conn.Write([]byte("250 OK\r\n"))
			case strings.HasPrefix(cmd, "DATA"):
				conn.Write([]byte("354 Start mail input\r\n"))
				var b strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						serverErr <- err
						return
					}
					if l == ".\r\n" {
						break
					}
					// Undo SMTP dot-stuffing.
					b.WriteString(strings.TrimPrefix(l, "."))
				}
				payloads <- b.String()
				conn.Write([]byte("250 OK: queued\r\n"))
			case strings.HasPrefix(cmd, "QUIT"):
				conn.Write([]byte("221 Bye\r\n"))
				return
			}
		}
	}()

	cfg := newTestConfig()
	cfg.SMTPHost = host
	cfg.SMTPPort = port
	cfg.SMTPTLS = false
	cfg.SMTPTimeout = 2
	cfg.SMTPUser = "" // avoid auth for a simpler dialog; delivery path is identical

	n := NewEmailNotifier(cfg)
	if mutate != nil {
		mutate(n)
	}
	if err := n.Send(context.Background(), title, message); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	select {
	case msg := <-payloads:
		return msg
	case err := <-serverErr:
		t.Fatalf("fake SMTP server error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the SMTP payload")
	}
	return ""
}

// icsPayload extracts and base64-decodes the text/calendar part.
func icsPayload(t *testing.T, msg string) string {
	t.Helper()

	const marker = "Content-Type: text/calendar"
	i := strings.Index(msg, marker)
	if i < 0 {
		t.Fatalf("message has no text/calendar part:\n%s", msg)
	}
	rest := msg[i:]
	j := strings.Index(rest, "\r\n\r\n")
	if j < 0 {
		t.Fatalf("malformed text/calendar part headers:\n%s", rest)
	}
	body := rest[j+4:]
	if k := strings.Index(body, "\r\n--"); k >= 0 {
		body = body[:k]
	}
	body = strings.ReplaceAll(body, "\r\n", "")
	body = strings.ReplaceAll(body, "\n", "")

	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("text/calendar body is not valid base64: %v", err)
	}
	return string(raw)
}

func TestEmailNotifier_MessageIncludesCalendarAttachment(t *testing.T) {
	msg := captureSentMessage(t, nil,
		"Reminder: Dana - birthday",
		"Upcoming birthday for Dana on June 15 (14 days away)")

	if !strings.Contains(msg, "Content-Type: multipart/mixed; boundary=") {
		t.Errorf("message is not multipart/mixed:\n%s", msg)
	}
	if !strings.Contains(msg, "Content-Type: text/calendar; charset=utf-8; method=PUBLISH") {
		t.Errorf("missing text/calendar part with method=PUBLISH")
	}
	if !strings.Contains(msg, `Content-Disposition: attachment; filename="reminder-dana-birthday.ics"`) {
		t.Errorf("missing attachment disposition/filename:\n%s", msg)
	}
	if !strings.Contains(msg, "Content-Transfer-Encoding: base64") {
		t.Errorf("calendar part is not base64 encoded")
	}

	// The HTML body must still be present.
	if !strings.Contains(msg, "Content-Type: text/html; charset=\"UTF-8\"") {
		t.Errorf("HTML body part missing")
	}
	if !strings.Contains(msg, "<!DOCTYPE html>") || !strings.Contains(msg, "Upcoming birthday for Dana") {
		t.Errorf("HTML body content missing")
	}

	ics := icsPayload(t, msg)
	if !strings.HasPrefix(ics, "BEGIN:VCALENDAR\r\n") {
		t.Errorf("ICS does not start with BEGIN:VCALENDAR:\n%q", ics[:min(len(ics), 40)])
	}
	for _, want := range []string{"BEGIN:VEVENT", "END:VEVENT", "DTSTART;VALUE=DATE:", "DTEND;VALUE=DATE:", "TRANSP:TRANSPARENT", "UID:"} {
		if !strings.Contains(ics, want) {
			t.Errorf("ICS missing %q:\n%s", want, ics)
		}
	}
}

func TestEmailNotifier_CalendarUsesCRLFAndFolding(t *testing.T) {
	msg := captureSentMessage(t, nil, "Reminder: Dana - anniversary",
		"Upcoming anniversary for Dana on January 2 (3 days away)")
	ics := icsPayload(t, msg)

	if !strings.Contains(ics, "\r\n") {
		t.Fatalf("ICS has no CRLF line endings: %q", ics)
	}
	if got, want := strings.Count(ics, "\n"), strings.Count(ics, "\r\n"); got != want {
		t.Errorf("ICS has %d LF but %d CRLF (bare line feeds present)", got, want)
	}
	for _, line := range strings.Split(ics, "\r\n") {
		if len(line) > 75 {
			t.Errorf("ICS line exceeds 75 octets (%d): %q", len(line), line)
		}
	}
}

func TestGenerateICS_EscapesTextValues(t *testing.T) {
	now := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	ics, err := generateICS("Party, with; specials \\ ok", "Line one\nLine two, and more; stuff", now)
	if err != nil {
		t.Fatalf("generateICS() error = %v", err)
	}

	if !strings.Contains(ics, `SUMMARY:Party\, with\; specials \\ ok`) {
		t.Errorf("SUMMARY not escaped per RFC 5545:\n%s", ics)
	}
	if !strings.Contains(ics, `DESCRIPTION:Line one\nLine two\, and more\; stuff`) {
		t.Errorf("DESCRIPTION not escaped per RFC 5545:\n%s", ics)
	}
	if !strings.Contains(ics, "METHOD:PUBLISH") {
		t.Errorf("missing METHOD:PUBLISH")
	}
	if !strings.HasSuffix(ics, "END:VCALENDAR\r\n") {
		t.Errorf("ICS does not end with END:VCALENDAR and CRLF")
	}
}

func TestGenerateICS_FoldsLongSummary(t *testing.T) {
	now := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	long := strings.Repeat("very long reminder summary ", 8) // > 75 octets
	ics, err := generateICS(long, "body", now)
	if err != nil {
		t.Fatalf("generateICS() error = %v", err)
	}
	if !strings.Contains(ics, "\r\n ") {
		t.Errorf("long summary was not folded onto a continuation line")
	}
	for _, line := range strings.Split(ics, "\r\n") {
		if len(line) > 75 {
			t.Errorf("folded line exceeds 75 octets (%d)", len(line))
		}
	}
	// Folding must be reversible to the escaped original.
	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	if !strings.Contains(unfolded, "SUMMARY:"+strings.TrimSpace(long)) {
		t.Errorf("unfolding did not restore the summary")
	}
}

func TestResolveEventDate(t *testing.T) {
	now := time.Date(2026, time.November, 11, 9, 30, 0, 0, time.UTC)
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}

	tests := []struct {
		name    string
		message string
		want    time.Time
	}{
		{"no date falls back to today", "Body without any date", day(2026, time.November, 11)},
		{"upcoming date in current year", "Upcoming birthday for Dana on June 15 (5 days away)", day(2026, time.June, 15)},
		{"year boundary resolves forward", "Upcoming birthday for Dana on January 2 (3 days away)", day(2027, time.January, 2)},
		{"catch-up date resolves backward", "Missed reminder: birthday for Dana was November 6 (5 days ago)", day(2026, time.November, 6)},
		{"unparseable month falls back to today", "Upcoming birthday for Dana on Shortly 9 (1 day away)", day(2026, time.November, 11)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveEventDate(tt.message, now); !got.Equal(tt.want) {
				t.Errorf("resolveEventDate() = %s, want %s", got.Format("2006-01-02"), tt.want.Format("2006-01-02"))
			}
		})
	}
}

func TestEmailNotifier_CalendarFailureStillDelivers(t *testing.T) {
	msg := captureSentMessage(t, func(n *EmailNotifier) {
		n.icsGenerator = func(string, string, time.Time) (string, error) {
			return "", errors.New("forced ICS failure")
		}
	}, "Reminder: Dana - birthday", "Upcoming birthday for Dana on June 15 (14 days away)")

	if strings.Contains(msg, "text/calendar") {
		t.Errorf("calendar attachment present despite generator failure:\n%s", msg)
	}
	if strings.Contains(msg, "multipart/mixed") {
		t.Errorf("message unexpectedly multipart after ICS failure:\n%s", msg)
	}
	if !strings.Contains(msg, "Content-Type: text/html; charset=\"UTF-8\"") {
		t.Errorf("HTML body part missing after ICS failure")
	}
	if !strings.Contains(msg, "Upcoming birthday for Dana") {
		t.Errorf("body content missing after ICS failure")
	}
}

func TestEmailNotifier_DigestSkipsCalendarAttachment(t *testing.T) {
	msg := captureSentMessage(t, nil, "Reminder digest",
		"Upcoming birthday for Dana on June 15 (5 days away)\n"+
			"Upcoming anniversary for Sam on July 1 (21 days away)")

	if strings.Contains(msg, "text/calendar") {
		t.Errorf("digest message must not carry a single-event calendar attachment")
	}
	if !strings.Contains(msg, "Content-Type: text/html; charset=\"UTF-8\"") {
		t.Errorf("digest HTML body missing")
	}
}

func TestIsDigest(t *testing.T) {
	if !isDigest("Reminder digest", "anything") {
		t.Error("title marker should be detected as digest")
	}
	if !isDigest("Reminder", "one on June 15 and another on July 1") {
		t.Error("multiple dates should be detected as digest")
	}
	if isDigest("Reminder: Dana - birthday", "Upcoming birthday for Dana on June 15 (5 days away)") {
		t.Error("single-event reminder misdetected as digest")
	}
}
