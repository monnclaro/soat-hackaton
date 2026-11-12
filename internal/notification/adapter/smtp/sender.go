// Package smtp implementa a porta Sender enviando e-mails via SMTP (Mailpit local, qualquer provedor em produção).
package smtp

import (
	"context"
	"fmt"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/monnclaro/soat-hackaton/internal/notification/domain"
)

// Sender envia e-mails via SMTP.
type Sender struct {
	addr     string
	from     string
	username string
	password string
	send     func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// New cria o sender. Sem username, envia sem autenticação.
func New(addr, from, username, password string) *Sender {
	return &Sender{addr: addr, from: from, username: username, password: password, send: smtp.SendMail}
}

// Send envia a mensagem.
func (s *Sender) Send(_ context.Context, msg domain.Message) error {
	var auth smtp.Auth
	if s.username != "" {
		host := s.addr
		if i := strings.LastIndex(host, ":"); i > 0 {
			host = host[:i]
		}
		auth = smtp.PlainAuth("", s.username, s.password, host)
	}
	// O envelope SMTP (MAIL FROM / RCPT TO) aceita apenas o endereço puro, sem nome de exibição.
	from, err := mail.ParseAddress(s.from)
	if err != nil {
		return fmt.Errorf("SMTP_FROM inválido: %w", err)
	}
	to, err := mail.ParseAddress(msg.To)
	if err != nil {
		return fmt.Errorf("destinatário inválido: %w", err)
	}
	return s.send(s.addr, auth, from.Address, []string{to.Address}, s.render(msg))
}

// render monta a mensagem MIME, removendo quebras de linha dos cabeçalhos (evita injeção).
func (s *Sender) render(msg domain.Message) []byte {
	clean := func(v string) string { return strings.NewReplacer("\r", "", "\n", "").Replace(v) }
	var b strings.Builder
	b.WriteString("From: " + clean(s.from) + "\r\n")
	b.WriteString("To: " + clean(msg.To) + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", clean(msg.Subject)) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(msg.Body, "\n", "\r\n"))
	return []byte(b.String())
}
