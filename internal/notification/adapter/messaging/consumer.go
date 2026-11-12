// Package messaging contém o consumidor de eventos de falha do notification-service.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/notification/domain"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
)

var notificationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "fiapx_notifications_total",
	Help: "Notificações por resultado.",
}, []string{"result"})

// Notifier é o caso de uso chamado para cada falha.
type Notifier interface {
	Execute(ctx context.Context, f domain.Failure) (bool, error)
}

// FailedVideoHandler consome video.status.failed, converte o contrato em domain.Failure
// e chama o caso de uso. Eventos que não são FAILED são ignorados.
func FailedVideoHandler(uc Notifier) rabbitmq.Handler {
	return func(ctx context.Context, d rabbitmq.Delivery) error {
		var evt events.VideoStatusChanged
		if err := json.Unmarshal(d.Body, &evt); err != nil {
			return rabbitmq.Permanent(fmt.Errorf("evento inválido: %w", err))
		}
		if evt.Status != events.StatusFailed {
			return nil
		}
		sent, err := uc.Execute(ctx, domain.Failure{
			VideoID: evt.VideoID, UserEmail: evt.UserEmail, UserName: evt.UserName,
			OriginalName: evt.OriginalName, Reason: evt.Error, OccurredAt: evt.OccurredAt,
		})
		switch {
		case err != nil:
			notificationsTotal.WithLabelValues("error").Inc()
			return err // SMTP indisponível: reentrega com backoff
		case sent:
			notificationsTotal.WithLabelValues("sent").Inc()
		default:
			notificationsTotal.WithLabelValues("skipped").Inc()
		}
		return nil
	}
}
