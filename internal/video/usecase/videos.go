package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/video/domain"
)

// Owner identifica o usuário autenticado que executa a ação.
type Owner struct {
	UserID string
	Email  string
	Name   string
}

// Videos agrupa os casos de uso de vídeo.
type Videos struct {
	repo    Repository
	cache   ListCache
	storage FileStorage
	logger  *slog.Logger
	now     func() time.Time
	newID   func() string
}

// NewVideos cria os casos de uso com as dependências injetadas.
func NewVideos(repo Repository, cache ListCache, storage FileStorage, logger *slog.Logger) *Videos {
	return &Videos{repo: repo, cache: cache, storage: storage, logger: logger, now: time.Now, newID: uuid.NewString}
}

// Upload guarda o vídeo, registra-o como PENDING e enfileira o processamento.
// Só retorna sucesso depois que o vídeo e a mensagem estão persistidos: em picos, a requisição
// nunca se perde (o processamento acontece depois, de forma assíncrona).
func (uc *Videos) Upload(ctx context.Context, owner Owner, fileName string, content io.Reader) (domain.Video, error) {
	name, ext, err := domain.ValidateFileName(fileName)
	if err != nil {
		return domain.Video{}, err
	}
	id := uc.newID()
	key := domain.StorageKey(owner.UserID, id, ext)

	size, err := uc.storage.Put(ctx, key, content, -1, "application/octet-stream")
	if err != nil {
		return domain.Video{}, fmt.Errorf("armazenando vídeo: %w", err)
	}
	if size == 0 {
		_ = uc.storage.Delete(context.WithoutCancel(ctx), key)
		return domain.Video{}, domain.ErrEmptyFile
	}

	now := uc.now().UTC()
	v := domain.Video{
		ID: id, UserID: owner.UserID, UserEmail: owner.Email, OriginalName: name, StorageKey: key,
		SizeBytes: size, Status: domain.StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	payload, err := json.Marshal(events.VideoUploaded{
		VideoID: id, UserID: owner.UserID, UserEmail: owner.Email, UserName: owner.Name,
		OriginalName: name, StorageKey: key, UploadedAt: now,
	})
	if err != nil {
		return domain.Video{}, err
	}
	msg := OutgoingMessage{ID: uc.newID(), Topic: events.RoutingKeyVideoUploaded, Payload: payload, CreatedAt: now}
	if err := uc.repo.CreateAndEnqueue(ctx, v, msg); err != nil {
		// Compensação: remove o arquivo que ficaria órfão.
		_ = uc.storage.Delete(context.WithoutCancel(ctx), key)
		return domain.Video{}, fmt.Errorf("registrando vídeo: %w", err)
	}
	uc.invalidate(ctx, owner.UserID)
	return v, nil
}

// List retorna os vídeos do usuário, usando o cache quando disponível (cache-aside).
func (uc *Videos) List(ctx context.Context, userID string) ([]domain.Video, error) {
	if cached, ok, err := uc.cache.Get(ctx, userID); err != nil {
		uc.logger.Warn("falha ao ler cache", "error", err)
	} else if ok {
		return cached, nil
	}
	videos, err := uc.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := uc.cache.Set(ctx, userID, videos); err != nil {
		uc.logger.Warn("falha ao gravar cache", "error", err)
	}
	return videos, nil
}

// Get retorna um vídeo do usuário. Vídeos de outros usuários retornam domain.ErrNotFound.
func (uc *Videos) Get(ctx context.Context, userID, id string) (domain.Video, error) {
	v, err := uc.repo.Get(ctx, id)
	if err != nil {
		return domain.Video{}, err
	}
	if !v.OwnedBy(userID) {
		return domain.Video{}, domain.ErrNotFound
	}
	return v, nil
}

// Download abre o ZIP de frames de um vídeo concluído.
func (uc *Videos) Download(ctx context.Context, userID, id string) (io.ReadCloser, int64, string, error) {
	v, err := uc.Get(ctx, userID, id)
	if err != nil {
		return nil, 0, "", err
	}
	if !v.Downloadable() {
		return nil, 0, "", domain.ErrNotReady
	}
	rc, size, err := uc.storage.Get(ctx, v.ZipKey)
	if err != nil {
		return nil, 0, "", err
	}
	return rc, size, domain.ZipFileName(v.OriginalName), nil
}

// ApplyStatus aplica o resultado informado pelo processor-worker.
// É idempotente: eventos duplicados ou fora de ordem são ignorados (applied = false).
func (uc *Videos) ApplyStatus(ctx context.Context, videoID string, change domain.StatusChange) (bool, error) {
	v, err := uc.repo.Get(ctx, videoID)
	if err != nil {
		return false, err
	}
	if !v.Apply(change, uc.now().UTC()) {
		uc.logger.Info("mudança de status ignorada", "video_id", v.ID, "from", v.Status, "to", change.Status)
		return false, nil
	}
	if err := uc.repo.Update(ctx, v); err != nil {
		return false, err
	}
	uc.invalidate(ctx, v.UserID)
	return true, nil
}

func (uc *Videos) invalidate(ctx context.Context, userID string) {
	if err := uc.cache.Invalidate(ctx, userID); err != nil {
		uc.logger.Warn("falha ao invalidar cache", "error", err)
	}
}
