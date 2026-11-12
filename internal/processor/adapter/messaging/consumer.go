// Package messaging contém os adaptadores de mensageria do worker: o consumidor da fila
// de vídeos (entrada) e o publicador de status (saída), além das métricas do processamento.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
	"github.com/monnclaro/soat-hackaton/internal/processor/domain"
	"github.com/monnclaro/soat-hackaton/internal/processor/usecase"
)

var (
	jobsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiapx_processor_jobs_total",
		Help: "Jobs de processamento por resultado (completed, failed, retry).",
	}, []string{"result"})
	jobDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "fiapx_processor_job_duration_seconds",
		Help:    "Duração do processamento de um vídeo.",
		Buckets: []float64{1, 2, 5, 10, 30, 60, 120, 300, 600, 1800},
	})
	jobsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fiapx_processor_jobs_in_flight",
		Help: "Vídeos sendo processados neste momento.",
	})
	framesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fiapx_processor_frames_extracted_total",
		Help: "Total de frames extraídos.",
	})
)

// Executor é o caso de uso chamado para cada mensagem.
type Executor interface {
	Execute(ctx context.Context, job domain.Job, attempt int) (usecase.Result, error)
}

// VideoUploadedHandler consome video.uploaded, converte o contrato em domain.Job,
// executa o caso de uso e registra as métricas.
func VideoUploadedHandler(uc Executor) rabbitmq.Handler {
	return func(ctx context.Context, d rabbitmq.Delivery) error {
		var evt events.VideoUploaded
		if err := json.Unmarshal(d.Body, &evt); err != nil || evt.VideoID == "" || evt.StorageKey == "" {
			return rabbitmq.Permanent(fmt.Errorf("mensagem video.uploaded inválida: %v", err))
		}
		job := domain.Job{
			VideoID: evt.VideoID, UserID: evt.UserID, UserEmail: evt.UserEmail, UserName: evt.UserName,
			OriginalName: evt.OriginalName, StorageKey: evt.StorageKey,
		}

		jobsInFlight.Inc()
		res, err := uc.Execute(ctx, job, d.Attempt)
		jobsInFlight.Dec()

		jobsTotal.WithLabelValues(string(res.Outcome)).Inc()
		if res.Outcome == domain.OutcomeCompleted {
			jobDuration.Observe(res.Duration.Seconds())
			framesTotal.Add(float64(res.Frames))
		}
		return err
	}
}
