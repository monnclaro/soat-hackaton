package messaging

import (
	"context"
	"log/slog"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/video/usecase"
)

// OutboxStore lê as mensagens pendentes e marca como publicadas as que o broker confirmou.
type OutboxStore interface {
	DispatchPending(ctx context.Context, limit int, publish func(context.Context, usecase.OutgoingMessage) error) (int, error)
}

// Publisher publica no broker aguardando confirmação.
type Publisher interface {
	Publish(ctx context.Context, routingKey string, body []byte, messageID string) error
}

// OutboxRelay publica periodicamente as mensagens do outbox (transactional outbox).
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
	return r.store.DispatchPending(ctx, r.batch, func(ctx context.Context, m usecase.OutgoingMessage) error {
		return r.pub.Publish(ctx, m.Topic, m.Payload, m.ID)
	})
}

// Run executa o relay até o ctx ser cancelado. Se o lote veio cheio, roda de novo
// imediatamente (drena picos rápido); caso contrário, aguarda o intervalo.
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
