package mail

import (
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"c0de-webhook/internal/config"
)

// Sender is the interface for sending emails. Implementations can be
// swapped for testing.
type Sender interface {
	Send(to, subject, textBody, htmlBody string) error
}

// SMTPSender sends emails via SMTP.
type SMTPSender struct {
	cfg config.SMTPConfig
}

func NewSMTPSender(cfg config.SMTPConfig) *SMTPSender {
	return &SMTPSender{cfg: cfg}
}

func (s *SMTPSender) Send(to, subject, textBody, htmlBody string) error {
	recipients := parseRecipients(to)
	if len(recipients) == 0 {
		return fmt.Errorf("no valid recipients")
	}

	msg := buildMessage(s.cfg.From, recipients, subject, textBody, htmlBody)

	client, err := s.connect()
	if err != nil {
		return fmt.Errorf("connecting to SMTP: %w", err)
	}
	defer client.Close()

	if auth := s.buildAuth(); auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}

	// Use SMTP username as envelope sender if set (required by some servers
	// like Plesk that reject MAIL FROM != authenticated user). The From header
	// in the message body still uses the configured From address.
	envelopeFrom := s.cfg.From
	if s.cfg.Username != "" {
		envelopeFrom = s.cfg.Username
	}
	if err := client.Mail(envelopeFrom); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing data: %w", err)
	}

	return client.Quit()
}

func (s *SMTPSender) connect() (*smtp.Client, error) {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	tlsConfig := &tls.Config{
		ServerName:         s.cfg.Host,
		InsecureSkipVerify: s.cfg.TLSSkipVerify,
	}

	switch s.cfg.Port {
	case 465:
		// Implicit TLS (SMTPS)
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsConfig)
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, s.cfg.Host)
	default:
		// Plain or STARTTLS
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return nil, err
		}
		client, err := smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			return nil, err
		}
		if s.cfg.TLS {
			if err := client.StartTLS(tlsConfig); err != nil {
				client.Close()
				return nil, fmt.Errorf("STARTTLS: %w", err)
			}
		}
		return client, nil
	}
}

func (s *SMTPSender) buildAuth() smtp.Auth {
	switch strings.ToLower(s.cfg.AuthMethod) {
	case "plain":
		return smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	case "login":
		return &loginAuth{username: s.cfg.Username, password: s.cfg.Password}
	case "crammd5":
		return smtp.CRAMMD5Auth(s.cfg.Username, s.cfg.Password)
	default:
		return nil
	}
}

// loginAuth implements smtp.Auth for the LOGIN mechanism (required by M365).
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	challenge := strings.ToLower(strings.TrimSpace(string(fromServer)))
	switch {
	case strings.Contains(challenge, "username"):
		return []byte(a.username), nil
	case strings.Contains(challenge, "password"):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected LOGIN challenge: %s", fromServer)
	}
}

func parseRecipients(to string) []string {
	parts := strings.Split(to, ",")
	var recipients []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			recipients = append(recipients, p)
		}
	}
	return recipients
}

func buildMessage(from string, to []string, subject, textBody, htmlBody string) []byte {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("From: %s\r\n", from))
	b.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject)))
	b.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	b.WriteString("MIME-Version: 1.0\r\n")

	hasText := textBody != ""
	hasHTML := htmlBody != ""

	switch {
	case hasText && hasHTML:
		boundary := fmt.Sprintf("----=_Part_%d", time.Now().UnixNano())
		b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
		b.WriteString("\r\n")
		b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(textBody)
		b.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
		b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(htmlBody)
		b.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))
	case hasHTML:
		b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(htmlBody)
	default:
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(textBody)
	}

	return []byte(b.String())
}
