package notifier

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/smtp"
	"time"

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
}

func NewEmailNotifier(cfg *config.Config) *EmailNotifier {
	tmpl := template.Must(template.ParseFS(emailTemplateFS, "email_template.html"))
	return &EmailNotifier{cfg: cfg, tmpl: tmpl}
}

func (n *EmailNotifier) Name() string { return "email" }

func (n *EmailNotifier) IsConfigured() bool {
	return n.cfg.SMTPHost != "" && n.cfg.NotifyEmail != ""
}

func (n *EmailNotifier) Send(ctx context.Context, title, message string) error {
	addr := fmt.Sprintf("%s:%d", n.cfg.SMTPHost, n.cfg.SMTPPort)

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

	timeout := time.Duration(n.cfg.SMTPTimeout) * time.Second

	// Build the full message
	fullMsg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: %s\r\n\r\n%s",
		n.cfg.SMTPUser, n.cfg.NotifyEmail, title, contentType, body)

	// Choose connection mode
	switch {
	case n.cfg.SMTPPort == 465:
		return n.sendDirectTLS(ctx, addr, fullMsg, timeout)
	case n.cfg.SMTPTLS:
		return n.sendSTARTTLS(ctx, addr, fullMsg, timeout)
	default:
		return n.sendPlain(ctx, addr, fullMsg, timeout)
	}
}

func (n *EmailNotifier) sendDirectTLS(ctx context.Context, addr, msg string, timeout time.Duration) error {
	tlsCfg := &tls.Config{ServerName: n.cfg.SMTPHost}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, n.cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	return n.sendWithClient(client, msg)
}

func (n *EmailNotifier) sendSTARTTLS(ctx context.Context, addr, msg string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	client, err := smtp.NewClient(conn, n.cfg.SMTPHost)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}

	if err := client.StartTLS(&tls.Config{ServerName: n.cfg.SMTPHost}); err != nil {
		client.Close()
		return fmt.Errorf("starttls: %w", err)
	}

	err = n.sendWithClient(client, msg)
	client.Close()
	return err
}

func (n *EmailNotifier) sendPlain(ctx context.Context, addr, msg string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	client, err := smtp.NewClient(conn, n.cfg.SMTPHost)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}

	err = n.sendWithClient(client, msg)
	client.Close()
	return err
}

func (n *EmailNotifier) sendWithClient(client *smtp.Client, msg string) error {
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
	if err := client.Rcpt(n.cfg.NotifyEmail); err != nil {
		return fmt.Errorf("rcpt: %w", err)
	}

	// Data
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}

	if _, err := w.Write([]byte(msg)); err != nil {
		w.Close()
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
