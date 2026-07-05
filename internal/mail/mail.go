// Package mail sends a rendered note as an HTML email over SMTP — the same
// Gmail SMTP + app-password pattern tools-browsernotes uses.
package mail

import (
	"net"
	"net/smtp"
	"strings"
)

// Config holds SMTP connection details.
type Config struct {
	Host     string
	Port     string
	Username string
	Password string
}

// Send delivers an HTML email from from to to via the SMTP server in cfg.
func Send(cfg Config, from, to, subject, htmlBody string) error {
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	return smtp.SendMail(addr, auth, from, []string{to}, buildMessage(from, to, subject, htmlBody))
}

func buildMessage(from, to, subject, htmlBody string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}
