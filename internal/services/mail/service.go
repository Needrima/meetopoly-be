package mail

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

// Config is SMTP connection settings.
type Config struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

type smtpService struct {
	cfg Config
}

// New builds an SMTP mail Service.
func New(cfg Config) Service {
	return &smtpService{cfg: cfg}
}

func (s *smtpService) Send(ctx context.Context, msg Message) error {
	if s.cfg.User == "" || s.cfg.Pass == "" {
		return fmt.Errorf("smtp credentials not configured")
	}
	from := s.cfg.From
	if from == "" {
		from = s.cfg.User
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	auth := smtp.PlainAuth("", s.cfg.User, s.cfg.Pass, s.cfg.Host)

	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(msg.To)
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(msg.Subject)
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.Body)

	errCh := make(chan error, 1)
	go func() {
		errCh <- smtp.SendMail(addr, auth, extractAddress(from), []string{msg.To}, []byte(b.String()))
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("smtp send: %w", err)
		}
		return nil
	}
}

func extractAddress(from string) string {
	start := strings.LastIndex(from, "<")
	end := strings.LastIndex(from, ">")
	if start >= 0 && end > start {
		return strings.TrimSpace(from[start+1 : end])
	}
	return strings.TrimSpace(from)
}
