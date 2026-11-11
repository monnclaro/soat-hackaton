package video

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implementa Cache (cache-aside) com Redis.
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisCache cria o cache. O TTL curto limita a janela de dados obsoletos.
func NewRedisCache(client *redis.Client, ttl time.Duration) *RedisCache {
	return &RedisCache{client: client, ttl: ttl}
}

func listKey(userID string) string { return "fiapx:videos:user:" + userID }

// GetList retorna a listagem em cache, se existir.
func (c *RedisCache) GetList(ctx context.Context, userID string) ([]Video, bool, error) {
	raw, err := c.client.Get(ctx, listKey(userID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var videos []Video
	if err := json.Unmarshal(raw, &videos); err != nil {
		return nil, false, err
	}
	return videos, true, nil
}

// SetList grava a listagem com TTL.
func (c *RedisCache) SetList(ctx context.Context, userID string, videos []Video) error {
	raw, err := json.Marshal(videos)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, listKey(userID), raw, c.ttl).Err()
}

// Invalidate remove a listagem do usuário do cache.
func (c *RedisCache) Invalidate(ctx context.Context, userID string) error {
	return c.client.Del(ctx, listKey(userID)).Err()
}
