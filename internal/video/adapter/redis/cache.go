// Package redis implementa a porta ListCache (cache-aside da listagem por usuário) com Redis.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/monnclaro/soat-hackaton/internal/video/domain"
)

// ListCache guarda a listagem de vídeos de cada usuário com TTL curto.
type ListCache struct {
	client *goredis.Client
	ttl    time.Duration
}

// NewListCache cria o cache.
func NewListCache(client *goredis.Client, ttl time.Duration) *ListCache {
	return &ListCache{client: client, ttl: ttl}
}

// cachedVideo é a representação serializada (o domínio não conhece JSON).
type cachedVideo struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	UserEmail    string    `json:"user_email"`
	OriginalName string    `json:"original_name"`
	StorageKey   string    `json:"storage_key"`
	ZipKey       string    `json:"zip_key,omitempty"`
	SizeBytes    int64     `json:"size_bytes"`
	FrameCount   int       `json:"frame_count"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func key(userID string) string { return "fiapx:videos:user:" + userID }

// Get retorna a listagem em cache, se existir.
func (c *ListCache) Get(ctx context.Context, userID string) ([]domain.Video, bool, error) {
	raw, err := c.client.Get(ctx, key(userID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var items []cachedVideo
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, false, err
	}
	out := make([]domain.Video, 0, len(items))
	for _, i := range items {
		out = append(out, domain.Video{
			ID: i.ID, UserID: i.UserID, UserEmail: i.UserEmail, OriginalName: i.OriginalName,
			StorageKey: i.StorageKey, ZipKey: i.ZipKey, SizeBytes: i.SizeBytes, FrameCount: i.FrameCount,
			Status: domain.Status(i.Status), ErrorMessage: i.ErrorMessage, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
		})
	}
	return out, true, nil
}

// Set grava a listagem com TTL.
func (c *ListCache) Set(ctx context.Context, userID string, videos []domain.Video) error {
	items := make([]cachedVideo, 0, len(videos))
	for _, v := range videos {
		items = append(items, cachedVideo{
			ID: v.ID, UserID: v.UserID, UserEmail: v.UserEmail, OriginalName: v.OriginalName,
			StorageKey: v.StorageKey, ZipKey: v.ZipKey, SizeBytes: v.SizeBytes, FrameCount: v.FrameCount,
			Status: string(v.Status), ErrorMessage: v.ErrorMessage, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
		})
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key(userID), raw, c.ttl).Err()
}

// Invalidate remove a listagem do usuário.
func (c *ListCache) Invalidate(ctx context.Context, userID string) error {
	return c.client.Del(ctx, key(userID)).Err()
}
