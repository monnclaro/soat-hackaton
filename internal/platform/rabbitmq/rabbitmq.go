// Package rabbitmq encapsula conexão, topologia, publicação confiável (publisher confirms)
// e consumo concorrente com ack manual.
package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Nomes da topologia de mensageria.
const (
	Exchange           = "fiapx.events"
	DeadLetterExchange = "fiapx.dlx"

	QueueVideoUploaded = "processor.video-uploaded"
	QueueStatusChanged = "video-service.status-changed"
	QueueNotifications = "notification.video-failed"
	QueueDeadLetter    = "fiapx.dead-letter"
)

// Dial conecta ao RabbitMQ com retentativas (backoff exponencial) até o ctx expirar.
func Dial(ctx context.Context, url string, logger *slog.Logger) (*amqp.Connection, error) {
	backoff := 500 * time.Millisecond
	for {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		logger.Warn("rabbitmq indisponível, tentando novamente", "error", err, "retry_in", backoff.String())
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("conectando ao rabbitmq: %w", err)
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 10*time.Second)
	}
}

// DeclareTopology declara exchanges e filas (idempotente). Todas as filas são quorum queues
// (replicadas e persistidas em disco) com dead-letter e limite de reentregas.
func DeclareTopology(ch *amqp.Channel, deliveryLimit int) error {
	if err := ch.ExchangeDeclare(Exchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declarando exchange: %w", err)
	}
	if err := ch.ExchangeDeclare(DeadLetterExchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declarando dlx: %w", err)
	}
	if _, err := ch.QueueDeclare(QueueDeadLetter, true, false, false, false, amqp.Table{"x-queue-type": "quorum"}); err != nil {
		return fmt.Errorf("declarando dlq: %w", err)
	}
	if err := ch.QueueBind(QueueDeadLetter, "#", DeadLetterExchange, false, nil); err != nil {
		return fmt.Errorf("bind dlq: %w", err)
	}

	bindings := []struct{ queue, key string }{
		{QueueVideoUploaded, "video.uploaded"},
		{QueueStatusChanged, "video.status.*"},
		{QueueNotifications, "video.status.failed"},
	}
	args := amqp.Table{
		"x-queue-type":           "quorum",
		"x-dead-letter-exchange": DeadLetterExchange,
		"x-delivery-limit":       int32(deliveryLimit),
	}
	for _, b := range bindings {
		if _, err := ch.QueueDeclare(b.queue, true, false, false, false, args); err != nil {
			return fmt.Errorf("declarando fila %s: %w", b.queue, err)
		}
		if err := ch.QueueBind(b.queue, b.key, Exchange, false, nil); err != nil {
			return fmt.Errorf("bind fila %s: %w", b.queue, err)
		}
	}
	return nil
}

// Setup abre um canal, declara a topologia e fecha o canal.
func Setup(conn *amqp.Connection, deliveryLimit int) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()
	return DeclareTopology(ch, deliveryLimit)
}

// Publisher publica mensagens persistentes aguardando a confirmação do broker.
type Publisher struct {
	mu sync.Mutex
	ch *amqp.Channel
}

// NewPublisher cria um publisher com o canal em modo confirm.
func NewPublisher(conn *amqp.Connection) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("ativando publisher confirms: %w", err)
	}
	return &Publisher{ch: ch}, nil
}

// Publish envia a mensagem e só retorna nil após o ack do broker (garantia de persistência).
func (p *Publisher) Publish(ctx context.Context, routingKey string, body []byte, messageID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	dc, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, Exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
	if err != nil {
		return fmt.Errorf("publicando %s: %w", routingKey, err)
	}
	ok, err := dc.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("aguardando confirmação: %w", err)
	}
	if !ok {
		return fmt.Errorf("broker recusou a mensagem %s", messageID)
	}
	return nil
}

// Close fecha o canal do publisher.
func (p *Publisher) Close() error { return p.ch.Close() }

// Delivery é a mensagem entregue ao handler.
type Delivery struct {
	MessageID  string
	RoutingKey string
	Body       []byte
	// Attempt começa em 1 e cresce a cada reentrega (header x-delivery-count das quorum queues).
	Attempt int
}

// Handler processa uma mensagem. Retornar nil confirma (ack); erro comum reenfileira;
// erro marcado com Permanent descarta para a DLQ.
type Handler func(ctx context.Context, d Delivery) error

type permanentError struct{ err error }

func (p permanentError) Error() string { return p.err.Error() }
func (p permanentError) Unwrap() error { return p.err }

// Permanent marca um erro como não recuperável (mensagem vai para a DLQ sem reentrega).
func Permanent(err error) error { return permanentError{err: err} }

// IsPermanent indica se o erro foi marcado com Permanent.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

type action int

const (
	actionAck action = iota
	actionRequeue
	actionReject
)

func decide(err error) action {
	switch {
	case err == nil:
		return actionAck
	case IsPermanent(err):
		return actionReject
	default:
		return actionRequeue
	}
}

// retryDelay cresce quadraticamente (1s, 4s, 9s...) com teto de 30s.
func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return min(time.Duration(attempt*attempt)*time.Second, 30*time.Second)
}

func attemptFrom(headers amqp.Table) int {
	switch v := headers["x-delivery-count"].(type) {
	case int64:
		return int(v) + 1
	case int32:
		return int(v) + 1
	case int:
		return v + 1
	default:
		return 1
	}
}

// Consume consome a fila com `concurrency` goroutines (prefetch igual à concorrência).
// Ao cancelar ctx, para de receber novas mensagens e aguarda as em andamento terminarem.
// Retorna erro se a conexão cair (o processo deve reiniciar — crash-only).
func Consume(ctx context.Context, conn *amqp.Connection, queue string, concurrency int, logger *slog.Logger, h Handler) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()
	if err := ch.Qos(concurrency, 0, false); err != nil {
		return fmt.Errorf("configurando prefetch: %w", err)
	}
	tag := fmt.Sprintf("%s-%d", queue, time.Now().UnixNano())
	msgs, err := ch.Consume(queue, tag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consumindo %s: %w", queue, err)
	}

	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range msgs {
				// Mensagens em andamento terminam mesmo durante o shutdown.
				d := Delivery{MessageID: m.MessageId, RoutingKey: m.RoutingKey, Body: m.Body, Attempt: attemptFrom(m.Headers)}
				herr := h(context.WithoutCancel(ctx), d)
				switch decide(herr) {
				case actionAck:
					_ = m.Ack(false)
				case actionReject:
					logger.Error("mensagem descartada para DLQ", "queue", queue, "message_id", m.MessageId, "error", herr)
					_ = m.Nack(false, false)
				case actionRequeue:
					delay := retryDelay(d.Attempt)
					logger.Warn("falha transitória, reenfileirando", "queue", queue, "message_id", m.MessageId, "attempt", d.Attempt, "retry_in", delay.String(), "error", herr)
					// Backoff antes do requeue para não esgotar o x-delivery-limit em milissegundos
					// enquanto a dependência (SMTP, S3, banco) se recupera.
					time.Sleep(delay)
					_ = m.Nack(false, true)
				}
			}
		}()
	}

	logger.Info("consumidor iniciado", "queue", queue, "concurrency", concurrency)
	closed := conn.NotifyClose(make(chan *amqp.Error, 1))
	select {
	case <-ctx.Done():
		_ = ch.Cancel(tag, false)
		wg.Wait()
		return nil
	case e := <-closed:
		wg.Wait()
		return fmt.Errorf("conexão com rabbitmq encerrada: %v", e)
	}
}
