package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/processor/domain"
)

// Broker publica mensagens com confirmação (implementado por platform/rabbitmq.Publisher).
type Broker interface {
	Publish(ctx context.Context, routingKey string, body []byte, messageID string) error
}

// StatusPublisher implementa a porta usecase.StatusPublisher publicando eventos video.status.*.
type StatusPublisher struct {
	broker Broker
	now    func() time.Time
}

// NewStatusPublisher cria o publicador.
func NewStatusPublisher(b Broker) *StatusPublisher {
	return &StatusPublisher{broker: b, now: time.Now}
}

// Publish converte a mudança de status no contrato de evento e publica.
// O message_id é determinístico (<video>-<status>) para permitir deduplicação.
func (p *StatusPublisher) Publish(ctx context.Context, job domain.Job, c domain.StatusChange) error {
	body, err := json.Marshal(events.VideoStatusChanged{
		VideoID: job.VideoID, UserID: job.UserID, UserEmail: job.UserEmail, UserName: job.UserName,
		OriginalName: job.OriginalName, Status: string(c.Status), ZipKey: c.ZipKey,
		FrameCount: c.FrameCount, Error: c.Error, OccurredAt: p.now().UTC(),
	})
	if err != nil {
		return err
	}
	msgID := fmt.Sprintf("%s-%s", job.VideoID, strings.ToLower(string(c.Status)))
	return p.broker.Publish(ctx, events.StatusRoutingKey(string(c.Status)), body, msgID)
}
