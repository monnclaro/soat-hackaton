// Package messaging contém os adaptadores de mensageria do video-service:
// o consumidor de eventos de status e o relay que publica o outbox no RabbitMQ.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
	"github.com/monnclaro/soat-hackaton/internal/video/domain"
	"github.com/monnclaro/soat-hackaton/internal/video/usecase"
)

var statusUpdates = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "fiapx_video_status_updates_total",
	Help: "Mudanças de status aplicadas/ignoradas.",
}, []string{"status", "applied"})

// StatusHandler consome eventos video.status.* (publicados pelo processor-worker),
// converte o contrato de mensagem para o domínio e chama o caso de uso ApplyStatus.
func StatusHandler(videos *usecase.Videos) rabbitmq.Handler {
	return func(ctx context.Context, d rabbitmq.Delivery) error {
		var evt events.VideoStatusChanged
		if err := json.Unmarshal(d.Body, &evt); err != nil || evt.VideoID == "" {
			return rabbitmq.Permanent(fmt.Errorf("evento de status inválido: %v", err))
		}
		applied, err := videos.ApplyStatus(ctx, evt.VideoID, domain.StatusChange{
			Status:     domain.Status(evt.Status),
			ZipKey:     evt.ZipKey,
			FrameCount: evt.FrameCount,
			Error:      evt.Error,
		})
		if errors.Is(err, domain.ErrNotFound) {
			return rabbitmq.Permanent(err) // vídeo inexistente: reentregar não resolve
		}
		if err != nil {
			return err // falha transitória (banco): reentrega
		}
		statusUpdates.WithLabelValues(evt.Status, fmt.Sprint(applied)).Inc()
		return nil
	}
}
