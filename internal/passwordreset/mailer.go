package passwordreset

import (
	"context"
	"log/slog"
	"mime"
	"net"
	"net/smtp"

	"github.com/kgazineu/finApp-back/internal/config"
)

// NewMailer sends through SMTP when cfg.Host is set. Without it the e-mail, code included,
// only goes to the log: fine for development; config.Load refuses that in production.
func NewMailer(cfg config.SMTP) Mailer {
	if cfg.Host == "" {
		slog.Warn("SMTP_HOST não definido: códigos de recuperação de senha serão escritos no log")
		return logMailer{}
	}

	m := smtpMailer{addr: net.JoinHostPort(cfg.Host, cfg.Port), from: cfg.From}
	if cfg.Username != "" {
		m.auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	return m
}

type smtpMailer struct {
	addr string
	from string
	auth smtp.Auth
}

// ponytail: net/smtp sem contexto nem pool; troque por um provedor/fila se o volume crescer
func (m smtpMailer) Send(_ context.Context, to, subject, body string) error {
	msg := "From: " + m.from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + body

	return smtp.SendMail(m.addr, m.auth, m.from, []string{to}, []byte(msg))
}

type logMailer struct{}

func (logMailer) Send(_ context.Context, to, subject, body string) error {
	slog.Info("e-mail (SMTP não configurado)", "to", to, "subject", subject, "body", body)
	return nil
}
