// Package processor implementa o worker que extrai frames de vídeos (ffmpeg) e gera o ZIP.
package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
)

// Storage é a porta de armazenamento de objetos.
type Storage interface {
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (int64, error)
}

// FrameExtractor extrai frames de um vídeo para um diretório e retorna os arquivos gerados.
type FrameExtractor interface {
	Extract(ctx context.Context, videoPath, outDir string) ([]string, error)
}

// Publisher publica eventos no broker.
type Publisher interface {
	Publish(ctx context.Context, routingKey string, body []byte, messageID string) error
}

// ProcessingError é uma falha definitiva do vídeo (arquivo corrompido, sem frames...).
// Não adianta tentar de novo: o vídeo é marcado como FAILED e o usuário é notificado.
type ProcessingError struct{ Reason string }

func (e *ProcessingError) Error() string { return e.Reason }

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

// Config do processador.
type Config struct {
	WorkDir     string
	JobTimeout  time.Duration
	MaxAttempts int
}

// Processor orquestra download → extração → zip → upload → evento.
type Processor struct {
	storage   Storage
	extractor FrameExtractor
	pub       Publisher
	cfg       Config
	logger    *slog.Logger
	now       func() time.Time
}

// New cria o processador.
func New(st Storage, ex FrameExtractor, pub Publisher, cfg Config, logger *slog.Logger) *Processor {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.JobTimeout <= 0 {
		cfg.JobTimeout = 30 * time.Minute
	}
	return &Processor{storage: st, extractor: ex, pub: pub, cfg: cfg, logger: logger, now: time.Now}
}

// Handle processa um vídeo. Retorna erro apenas para falhas transitórias que devem ser
// reentregues (a mensagem permanece na fila). Após MaxAttempts tentativas o vídeo é marcado FAILED.
func (p *Processor) Handle(ctx context.Context, evt events.VideoUploaded, attempt int) error {
	log := p.logger.With("video_id", evt.VideoID, "attempt", attempt)
	jobsInFlight.Inc()
	defer jobsInFlight.Dec()
	start := p.now()

	if err := p.publishStatus(ctx, evt, events.StatusProcessing, func(*events.VideoStatusChanged) {}); err != nil {
		return err
	}
	log.Info("processamento iniciado")

	zipKey, frames, err := p.process(ctx, evt)
	if err != nil {
		var perr *ProcessingError
		permanent := errors.As(err, &perr)
		if !permanent && attempt < p.cfg.MaxAttempts {
			jobsTotal.WithLabelValues("retry").Inc()
			log.Warn("falha transitória, será reprocessado", "error", err)
			return err
		}
		reason := err.Error()
		if !permanent {
			reason = fmt.Sprintf("falha após %d tentativas: %s", attempt, reason)
		}
		if perr := p.publishStatus(ctx, evt, events.StatusFailed, func(e *events.VideoStatusChanged) { e.Error = reason }); perr != nil {
			return perr
		}
		jobsTotal.WithLabelValues("failed").Inc()
		log.Error("processamento falhou", "error", reason)
		return nil
	}

	if err := p.publishStatus(ctx, evt, events.StatusCompleted, func(e *events.VideoStatusChanged) {
		e.ZipKey = zipKey
		e.FrameCount = frames
	}); err != nil {
		return err
	}
	elapsed := p.now().Sub(start)
	jobsTotal.WithLabelValues("completed").Inc()
	jobDuration.Observe(elapsed.Seconds())
	framesTotal.Add(float64(frames))
	log.Info("processamento concluído", "frames", frames, "duration_ms", elapsed.Milliseconds())
	return nil
}

func (p *Processor) process(ctx context.Context, evt events.VideoUploaded) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.JobTimeout)
	defer cancel()

	if err := os.MkdirAll(p.cfg.WorkDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("criando diretório de trabalho: %w", err)
	}
	tmp, err := os.MkdirTemp(p.cfg.WorkDir, "job-*")
	if err != nil {
		return "", 0, fmt.Errorf("criando diretório temporário: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	videoPath := filepath.Join(tmp, "input"+strings.ToLower(filepath.Ext(evt.OriginalName)))
	if err := p.download(ctx, evt.StorageKey, videoPath); err != nil {
		return "", 0, err
	}

	framesDir := filepath.Join(tmp, "frames")
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return "", 0, err
	}
	frames, err := p.extractor.Extract(ctx, videoPath, framesDir)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", 0, &ProcessingError{Reason: "tempo limite de processamento excedido"}
		}
		return "", 0, err
	}
	if len(frames) == 0 {
		return "", 0, &ProcessingError{Reason: "nenhum frame foi extraído do vídeo"}
	}

	zipPath := filepath.Join(tmp, "frames.zip")
	if err := ZipFilesToPath(frames, zipPath); err != nil {
		return "", 0, fmt.Errorf("criando zip: %w", err)
	}
	zf, err := os.Open(zipPath)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = zf.Close() }()
	info, err := zf.Stat()
	if err != nil {
		return "", 0, err
	}
	zipKey := fmt.Sprintf("outputs/%s/%s.zip", evt.UserID, evt.VideoID)
	if _, err := p.storage.Put(ctx, zipKey, zf, info.Size(), "application/zip"); err != nil {
		return "", 0, fmt.Errorf("enviando zip: %w", err)
	}
	return zipKey, len(frames), nil
}

func (p *Processor) download(ctx context.Context, key, dst string) error {
	rc, _, err := p.storage.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("baixando vídeo: %w", err)
	}
	defer func() { _ = rc.Close() }()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, rc); err != nil {
		_ = f.Close()
		return fmt.Errorf("baixando vídeo: %w", err)
	}
	return f.Close()
}

func (p *Processor) publishStatus(ctx context.Context, evt events.VideoUploaded, status string, fill func(*events.VideoStatusChanged)) error {
	out := events.VideoStatusChanged{
		VideoID: evt.VideoID, UserID: evt.UserID, UserEmail: evt.UserEmail, UserName: evt.UserName,
		OriginalName: evt.OriginalName, Status: status, OccurredAt: p.now().UTC(),
	}
	fill(&out)
	body, err := json.Marshal(out)
	if err != nil {
		return err
	}
	msgID := fmt.Sprintf("%s-%s", evt.VideoID, strings.ToLower(status))
	if err := p.pub.Publish(ctx, events.StatusRoutingKey(status), body, msgID); err != nil {
		return fmt.Errorf("publicando status %s: %w", status, err)
	}
	return nil
}

// Handler adapta o Processor ao consumidor RabbitMQ.
func Handler(p *Processor) rabbitmq.Handler {
	return func(ctx context.Context, d rabbitmq.Delivery) error {
		var evt events.VideoUploaded
		if err := json.Unmarshal(d.Body, &evt); err != nil || evt.VideoID == "" || evt.StorageKey == "" {
			return rabbitmq.Permanent(fmt.Errorf("mensagem video.uploaded inválida: %v", err))
		}
		return p.Handle(ctx, evt, d.Attempt)
	}
}
