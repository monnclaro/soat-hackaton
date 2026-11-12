package video

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

// OutboxMessage é uma mensagem a ser publicada no broker, gravada na mesma transação do vídeo.
type OutboxMessage struct {
	ID         string
	RoutingKey string
	Payload    []byte
	CreatedAt  time.Time
}

// Repository é a porta de persistência.
type Repository interface {
	// CreateWithOutbox grava o vídeo e a mensagem de outbox atomicamente.
	CreateWithOutbox(ctx context.Context, v Video, msg OutboxMessage) error
	Get(ctx context.Context, id string) (Video, error)
	ListByUser(ctx context.Context, userID string) ([]Video, error)
	Update(ctx context.Context, v Video) error
}

// Cache é a porta de cache da listagem por usuário.
type Cache interface {
	GetList(ctx context.Context, userID string) ([]Video, bool, error)
	SetList(ctx context.Context, userID string, videos []Video) error
	Invalidate(ctx context.Context, userID string) error
}

// Storage é a porta de armazenamento de objetos.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (int64, error)
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, key string) error
}

var (
	uploadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiapx_video_uploads_total",
		Help: "Uploads de vídeo recebidos por resultado.",
	}, []string{"result"})
	statusUpdates = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiapx_video_status_updates_total",
		Help: "Atualizações de status aplicadas/ignoradas.",
	}, []string{"status", "applied"})
)

// Service contém os casos de uso de vídeo.
type Service struct {
	repo    Repository
	cache   Cache
	storage Storage
	logger  *slog.Logger
	now     func() time.Time
	newID   func() string
}

// NewService cria o serviço.
func NewService(repo Repository, cache Cache, storage Storage, logger *slog.Logger) *Service {
	return &Service{repo: repo, cache: cache, storage: storage, logger: logger, now: time.Now, newID: uuid.NewString}
}

// Upload armazena o vídeo, registra-o como PENDING e enfileira o processamento (via outbox).
// A requisição só é confirmada ao cliente após o vídeo e a mensagem estarem persistidos,
// garantindo que nenhum pedido seja perdido em picos (o processamento é assíncrono).
func (s *Service) Upload(ctx context.Context, p security.Principal, fileName string, content io.Reader) (Video, error) {
	name, ext, err := ValidateFileName(fileName)
	if err != nil {
		uploadsTotal.WithLabelValues("rejected").Inc()
		return Video{}, err
	}
	id := s.newID()
	key := fmt.Sprintf("videos/%s/%s%s", p.UserID, id, ext)

	size, err := s.storage.Put(ctx, key, content, -1, "application/octet-stream")
	if err != nil {
		uploadsTotal.WithLabelValues("error").Inc()
		return Video{}, fmt.Errorf("armazenando vídeo: %w", err)
	}
	if size == 0 {
		_ = s.storage.Delete(context.WithoutCancel(ctx), key)
		uploadsTotal.WithLabelValues("rejected").Inc()
		return Video{}, ErrEmptyFile
	}

	now := s.now().UTC()
	v := Video{
		ID: id, UserID: p.UserID, UserEmail: p.Email, OriginalName: name, StorageKey: key,
		SizeBytes: size, Status: events.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	payload, err := json.Marshal(events.VideoUploaded{
		VideoID: id, UserID: p.UserID, UserEmail: p.Email, UserName: p.Name,
		OriginalName: name, StorageKey: key, UploadedAt: now,
	})
	if err != nil {
		return Video{}, err
	}
	msg := OutboxMessage{ID: s.newID(), RoutingKey: events.RoutingKeyVideoUploaded, Payload: payload, CreatedAt: now}
	if err := s.repo.CreateWithOutbox(ctx, v, msg); err != nil {
		// Compensação: remove o objeto órfão.
		_ = s.storage.Delete(context.WithoutCancel(ctx), key)
		uploadsTotal.WithLabelValues("error").Inc()
		return Video{}, fmt.Errorf("registrando vídeo: %w", err)
	}
	s.invalidate(ctx, p.UserID)
	uploadsTotal.WithLabelValues("accepted").Inc()
	return v, nil
}

// List retorna os vídeos do usuário (cache-aside com Redis).
func (s *Service) List(ctx context.Context, userID string) ([]Video, error) {
	if cached, ok, err := s.cache.GetList(ctx, userID); err != nil {
		s.logger.Warn("falha ao ler cache", "error", err)
	} else if ok {
		return cached, nil
	}
	videos, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetList(ctx, userID, videos); err != nil {
		s.logger.Warn("falha ao gravar cache", "error", err)
	}
	return videos, nil
}

// Get retorna um vídeo do usuário (vídeos de outros usuários retornam ErrNotFound).
func (s *Service) Get(ctx context.Context, userID, id string) (Video, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return Video{}, err
	}
	if v.UserID != userID {
		return Video{}, ErrNotFound
	}
	return v, nil
}

// Download abre o ZIP de frames de um vídeo concluído.
func (s *Service) Download(ctx context.Context, userID, id string) (io.ReadCloser, int64, string, error) {
	v, err := s.Get(ctx, userID, id)
	if err != nil {
		return nil, 0, "", err
	}
	if v.Status != events.StatusCompleted || v.ZipKey == "" {
		return nil, 0, "", ErrNotReady
	}
	rc, size, err := s.storage.Get(ctx, v.ZipKey)
	if err != nil {
		return nil, 0, "", err
	}
	return rc, size, ZipFileName(v.OriginalName), nil
}

// ApplyStatus aplica um evento de mudança de status vindo do processor-worker.
// É idempotente: eventos duplicados ou fora de ordem que violam a máquina de estados são ignorados.
func (s *Service) ApplyStatus(ctx context.Context, evt events.VideoStatusChanged) error {
	v, err := s.repo.Get(ctx, evt.VideoID)
	if err != nil {
		return err
	}
	if !CanTransition(v.Status, evt.Status) {
		statusUpdates.WithLabelValues(evt.Status, "false").Inc()
		s.logger.Info("evento de status ignorado", "video_id", v.ID, "from", v.Status, "to", evt.Status)
		return nil
	}
	v.Status = evt.Status
	v.UpdatedAt = s.now().UTC()
	switch evt.Status {
	case events.StatusCompleted:
		v.ZipKey = evt.ZipKey
		v.FrameCount = evt.FrameCount
		v.ErrorMessage = ""
	case events.StatusFailed:
		v.ErrorMessage = evt.Error
	}
	if err := s.repo.Update(ctx, v); err != nil {
		return err
	}
	s.invalidate(ctx, v.UserID)
	statusUpdates.WithLabelValues(evt.Status, "true").Inc()
	return nil
}

func (s *Service) invalidate(ctx context.Context, userID string) {
	if err := s.cache.Invalidate(ctx, userID); err != nil {
		s.logger.Warn("falha ao invalidar cache", "error", err)
	}
}
