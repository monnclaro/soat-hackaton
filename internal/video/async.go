package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
)

// OutboxStore lê mensagens pendentes e as marca como publicadas.
type OutboxStore interface {
	DispatchPending(ctx context.Context, limit int, publish func(context.Context, OutboxMessage) error) (int, error)
}

// Publisher publica no broker com confirmação.
type Publisher interface {
	Publish(ctx context.Context, routingKey string, body []byte, messageID string) error
}

// OutboxRelay publica periodicamente as mensagens do outbox no RabbitMQ.
type OutboxRelay struct {
	store    OutboxStore
	pub      Publisher
	interval time.Duration
	batch    int
	logger   *slog.Logger
}

// NewOutboxRelay cria o relay.
func NewOutboxRelay(store OutboxStore, pub Publisher, interval time.Duration, batch int, logger *slog.Logger) *OutboxRelay {
	return &OutboxRelay{store: store, pub: pub, interval: interval, batch: batch, logger: logger}
}

// RunOnce publica um lote de mensagens pendentes.
func (r *OutboxRelay) RunOnce(ctx context.Context) (int, error) {
	return r.store.DispatchPending(ctx, r.batch, func(ctx context.Context, m OutboxMessage) error {
		return r.pub.Publish(ctx, m.RoutingKey, m.Payload, m.ID)
	})
}

// Run executa o relay até o ctx ser cancelado. Se o lote veio cheio, roda de novo imediatamente
// (drena picos rapidamente); caso contrário aguarda o intervalo.
func (r *OutboxRelay) Run(ctx context.Context) {
	for {
		n, err := r.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			r.logger.Warn("falha no relay do outbox", "error", err, "published", n)
		}
		if n >= r.batch && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.interval):
		}
	}
}

// StatusHandler consome eventos video.status.* e atualiza o vídeo.
func StatusHandler(svc *Service) rabbitmq.Handler {
	return func(ctx context.Context, d rabbitmq.Delivery) error {
		var evt events.VideoStatusChanged
		if err := json.Unmarshal(d.Body, &evt); err != nil || evt.VideoID == "" {
			return rabbitmq.Permanent(fmt.Errorf("evento de status inválido: %v", err))
		}
		err := svc.ApplyStatus(ctx, evt)
		if errors.Is(err, ErrNotFound) {
			return rabbitmq.Permanent(err)
		}
		return err
	}
}
