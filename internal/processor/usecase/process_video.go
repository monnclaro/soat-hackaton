package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/processor/domain"
)

// Config do caso de uso.
type Config struct {
	WorkDir     string        // diretório para arquivos temporários
	JobTimeout  time.Duration // tempo máximo por vídeo
	MaxAttempts int           // tentativas em falhas transitórias
}

// Result descreve o que aconteceu (usado pelos adaptadores para métricas e logs).
type Result struct {
	Outcome  domain.Outcome
	Frames   int
	Duration time.Duration
}

// ProcessVideo é o caso de uso de processamento.
type ProcessVideo struct {
	storage   FileStorage
	extractor FrameExtractor
	archiver  Archiver
	status    StatusPublisher
	cfg       Config
	policy    domain.RetryPolicy
	logger    *slog.Logger
	now       func() time.Time
}

// NewProcessVideo cria o caso de uso com as dependências injetadas.
func NewProcessVideo(st FileStorage, ex FrameExtractor, ar Archiver, sp StatusPublisher, cfg Config, logger *slog.Logger) *ProcessVideo {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.JobTimeout <= 0 {
		cfg.JobTimeout = 30 * time.Minute
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = os.TempDir()
	}
	return &ProcessVideo{
		storage: st, extractor: ex, archiver: ar, status: sp, cfg: cfg,
		policy: domain.RetryPolicy{MaxAttempts: cfg.MaxAttempts}, logger: logger, now: time.Now,
	}
}

// Execute processa o job. Retorna erro apenas quando a mensagem deve voltar para a fila
// (falha transitória antes da última tentativa, ou falha ao publicar o status).
// Falhas definitivas são informadas como FAILED e retornam nil (a mensagem é confirmada).
func (uc *ProcessVideo) Execute(ctx context.Context, job domain.Job, attempt int) (Result, error) {
	log := uc.logger.With("video_id", job.VideoID, "attempt", attempt)
	start := uc.now()

	if err := uc.status.Publish(ctx, job, domain.StatusChange{Status: domain.StatusProcessing}); err != nil {
		return Result{Outcome: domain.OutcomeRetry}, fmt.Errorf("publicando PROCESSING: %w", err)
	}
	log.Info("processamento iniciado")

	frames, err := uc.process(ctx, job)
	if err != nil {
		outcome := uc.policy.Decide(err, attempt)
		if outcome == domain.OutcomeRetry {
			log.Warn("falha transitória, será reprocessado", "error", err)
			return Result{Outcome: outcome}, err
		}
		reason := uc.policy.FailureReason(err, attempt)
		if perr := uc.status.Publish(ctx, job, domain.StatusChange{Status: domain.StatusFailed, Error: reason}); perr != nil {
			return Result{Outcome: domain.OutcomeRetry}, fmt.Errorf("publicando FAILED: %w", perr)
		}
		log.Error("processamento falhou", "reason", reason)
		return Result{Outcome: domain.OutcomeFailed}, nil
	}

	change := domain.StatusChange{Status: domain.StatusCompleted, ZipKey: job.ZipKey(), FrameCount: frames}
	if err := uc.status.Publish(ctx, job, change); err != nil {
		return Result{Outcome: domain.OutcomeRetry}, fmt.Errorf("publicando COMPLETED: %w", err)
	}
	elapsed := uc.now().Sub(start)
	log.Info("processamento concluído", "frames", frames, "duration_ms", elapsed.Milliseconds())
	return Result{Outcome: domain.OutcomeCompleted, Frames: frames, Duration: elapsed}, nil
}

// process baixa o vídeo, extrai os frames, gera o ZIP e o envia ao storage.
// Usa um diretório temporário exclusivo por job, removido ao final.
func (uc *ProcessVideo) process(ctx context.Context, job domain.Job) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, uc.cfg.JobTimeout)
	defer cancel()

	if err := os.MkdirAll(uc.cfg.WorkDir, 0o755); err != nil {
		return 0, fmt.Errorf("criando diretório de trabalho: %w", err)
	}
	tmp, err := os.MkdirTemp(uc.cfg.WorkDir, "job-*")
	if err != nil {
		return 0, fmt.Errorf("criando diretório temporário: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	videoPath := filepath.Join(tmp, "input"+job.InputExtension())
	if err := uc.download(ctx, job.StorageKey, videoPath); err != nil {
		return 0, err
	}

	framesDir := filepath.Join(tmp, "frames")
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return 0, err
	}
	frames, err := uc.extractor.Extract(ctx, videoPath, framesDir)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return 0, domain.ErrTimeout
		}
		return 0, err
	}
	if len(frames) == 0 {
		return 0, domain.ErrNoFrames
	}

	zipPath := filepath.Join(tmp, "frames.zip")
	if err := uc.archiver.Archive(frames, zipPath); err != nil {
		return 0, fmt.Errorf("compactando frames: %w", err)
	}
	if err := uc.upload(ctx, zipPath, job.ZipKey()); err != nil {
		return 0, err
	}
	return len(frames), nil
}

func (uc *ProcessVideo) download(ctx context.Context, key, dst string) error {
	rc, _, err := uc.storage.Get(ctx, key)
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

func (uc *ProcessVideo) upload(ctx context.Context, path, key string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if _, err := uc.storage.Put(ctx, key, f, info.Size(), "application/zip"); err != nil {
		return fmt.Errorf("enviando zip: %w", err)
	}
	return nil
}
