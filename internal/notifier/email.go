package notifier

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/datey/datey/internal/config"
)

type EmailNotifier struct {
	cfg *config.Config
}

func NewEmailNotifier(cfg *config.Config) *EmailNotifier {
	return &EmailNotifier{cfg: cfg}
}

func (n *EmailNotifier) Name() string { return "email" }

func (n *EmailNotifier) IsConfigured() bool {
	return n.cfg.SMTPHost != "" && n.cfg.NotifyEmail != ""
}

func (n *EmailNotifier) Send(ctx context.Context, title, message string) error {
	auth := smtp.PlainAuth("", n.cfg.SMTPUser, n.cfg.SMTPPass, n.cfg.SMTPHost)

	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n%s",
		n.cfg.SMTPUser, n.cfg.NotifyEmail, title, message)

	addr := fmt.Sprintf("%s:%d", n.cfg.SMTPHost, n.cfg.SMTPPort)
	return smtp.SendMail(addr, auth, n.cfg.SMTPUser, []string{n.cfg.NotifyEmail}, []byte(body))
}
