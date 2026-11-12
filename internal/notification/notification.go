// Package notification notifica o usuário (e-mail) quando o processamento de um vídeo falha.
package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
)

// Message é um e-mail a ser enviado.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Sender é a porta de envio (SMTP, SES, SendGrid...).
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

var sentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "fiapx_notifications_total",
	Help: "Notificações por resultado.",
}, []string{"result"})

// Notifier decide quando e o que notificar.
type Notifier struct {
	sender Sender
	logger *slog.Logger
}

// NewNotifier cria o notificador.
func NewNotifier(sender Sender, logger *slog.Logger) *Notifier {
	return &Notifier{sender: sender, logger: logger}
}

// Handle envia o e-mail de falha. Eventos que não são FAILED são ignorados.
func (n *Notifier) Handle(ctx context.Context, evt events.VideoStatusChanged) error {
	if evt.Status != events.StatusFailed {
		return nil
	}
	if evt.UserEmail == "" {
		sentTotal.WithLabelValues("skipped").Inc()
		n.logger.Warn("evento de falha sem e-mail do usuário", "video_id", evt.VideoID)
		return nil
	}
	msg := BuildFailureMessage(evt)
	if err := n.sender.Send(ctx, msg); err != nil {
		sentTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("enviando e-mail: %w", err)
	}
	sentTotal.WithLabelValues("sent").Inc()
	n.logger.Info("usuário notificado", "video_id", evt.VideoID, "to", evt.UserEmail)
	return nil
}

// BuildFailureMessage monta o e-mail de falha.
func BuildFailureMessage(evt events.VideoStatusChanged) Message {
	name := evt.UserName
	if name == "" {
		name = "usuário"
	}
	reason := evt.Error
	if reason == "" {
		reason = "erro desconhecido"
	}
	body := fmt.Sprintf(`Olá, %s.

Infelizmente não conseguimos processar o seu vídeo "%s".

Motivo: %s
ID do vídeo: %s
Data: %s

Verifique se o arquivo é um vídeo válido e tente enviá-lo novamente.

Equipe FIAP X`, name, evt.OriginalName, reason, evt.VideoID, evt.OccurredAt.Format(time.RFC1123))
	return Message{
		To:      evt.UserEmail,
		Subject: fmt.Sprintf("FIAP X - Falha ao processar o vídeo %s", evt.OriginalName),
		Body:    body,
	}
}

// Handler adapta o Notifier ao consumidor RabbitMQ.
func Handler(n *Notifier) rabbitmq.Handler {
	return func(ctx context.Context, d rabbitmq.Delivery) error {
		var evt events.VideoStatusChanged
		if err := json.Unmarshal(d.Body, &evt); err != nil {
			return rabbitmq.Permanent(fmt.Errorf("evento inválido: %w", err))
		}
		return n.Handle(ctx, evt)
	}
}

// SMTPSender envia e-mails via SMTP (MailHog em desenvolvimento).
type SMTPSender struct {
	Addr     string
	From     string
	Username string
	Password string
	send     func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// NewSMTPSender cria o sender.
func NewSMTPSender(addr, from, username, password string) *SMTPSender {
	return &SMTPSender{Addr: addr, From: from, Username: username, Password: password, send: smtp.SendMail}
}

// Send envia a mensagem.
func (s *SMTPSender) Send(_ context.Context, msg Message) error {
	var auth smtp.Auth
	if s.Username != "" {
		host := s.Addr
		if i := strings.LastIndex(host, ":"); i > 0 {
			host = host[:i]
		}
		auth = smtp.PlainAuth("", s.Username, s.Password, host)
	}
	// O envelope SMTP (MAIL FROM / RCPT TO) aceita apenas o endereço puro, sem nome de exibição.
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("SMTP_FROM inválido: %w", err)
	}
	to, err := mail.ParseAddress(msg.To)
	if err != nil {
		return fmt.Errorf("destinatário inválido: %w", err)
	}
	return s.send(s.Addr, auth, from.Address, []string{to.Address}, s.render(msg))
}

func (s *SMTPSender) render(msg Message) []byte {
	clean := func(v string) string { return strings.NewReplacer("\r", "", "\n", "").Replace(v) }
	var b strings.Builder
	b.WriteString("From: " + clean(s.From) + "\r\n")
	b.WriteString("To: " + clean(msg.To) + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", clean(msg.Subject)) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(msg.Body, "\n", "\r\n"))
	return []byte(b.String())
}
