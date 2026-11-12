// Package videotest fornece implementações em memória das portas do video-service,
// usadas pelos testes das camadas usecase e adapter.
package videotest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"

	"github.com/monnclaro/soat-hackaton/internal/platform/storage"
	"github.com/monnclaro/soat-hackaton/internal/video/domain"
	"github.com/monnclaro/soat-hackaton/internal/video/usecase"
)

// DiscardLogger descarta os logs.
func DiscardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Repo implementa usecase.Repository em memória.
type Repo struct {
	mu        sync.Mutex
	Videos    map[string]domain.Video
	Outbox    []usecase.OutgoingMessage
	CreateErr error
	GetErr    error
	ListCalls int
}

// NewRepo cria o repositório vazio.
func NewRepo() *Repo { return &Repo{Videos: map[string]domain.Video{}} }

// CreateAndEnqueue implementa usecase.Repository.
func (r *Repo) CreateAndEnqueue(_ context.Context, v domain.Video, m usecase.OutgoingMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CreateErr != nil {
		return r.CreateErr
	}
	r.Videos[v.ID] = v
	r.Outbox = append(r.Outbox, m)
	return nil
}

// Get implementa usecase.Repository.
func (r *Repo) Get(_ context.Context, id string) (domain.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.GetErr != nil {
		return domain.Video{}, r.GetErr
	}
	v, ok := r.Videos[id]
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return v, nil
}

// ListByUser implementa usecase.Repository.
func (r *Repo) ListByUser(_ context.Context, userID string) ([]domain.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ListCalls++
	if r.GetErr != nil {
		return nil, r.GetErr
	}
	out := []domain.Video{}
	for _, v := range r.Videos {
		if v.UserID == userID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Update implementa usecase.Repository.
func (r *Repo) Update(_ context.Context, v domain.Video) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Videos[v.ID]; !ok {
		return domain.ErrNotFound
	}
	r.Videos[v.ID] = v
	return nil
}

// Video retorna uma cópia do vídeo armazenado.
func (r *Repo) Video(id string) domain.Video {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Videos[id]
}

// Cache implementa usecase.ListCache em memória.
type Cache struct {
	mu          sync.Mutex
	Lists       map[string][]domain.Video
	Invalidated []string
	Err         error
}

// NewCache cria o cache vazio.
func NewCache() *Cache { return &Cache{Lists: map[string][]domain.Video{}} }

// Get implementa usecase.ListCache.
func (c *Cache) Get(_ context.Context, userID string) ([]domain.Video, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Err != nil {
		return nil, false, c.Err
	}
	v, ok := c.Lists[userID]
	return v, ok, nil
}

// Set implementa usecase.ListCache.
func (c *Cache) Set(_ context.Context, userID string, v []domain.Video) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Err != nil {
		return c.Err
	}
	c.Lists[userID] = v
	return nil
}

// Invalidate implementa usecase.ListCache.
func (c *Cache) Invalidate(_ context.Context, userID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Invalidated = append(c.Invalidated, userID)
	delete(c.Lists, userID)
	return c.Err
}

// Storage implementa usecase.FileStorage em memória.
type Storage struct {
	mu      sync.Mutex
	Objects map[string][]byte
	PutErr  error
	Deleted []string
}

// NewStorage cria o storage vazio.
func NewStorage() *Storage { return &Storage{Objects: map[string][]byte{}} }

// Put implementa usecase.FileStorage.
func (s *Storage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) (int64, error) {
	if s.PutErr != nil {
		return 0, s.PutErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Objects[key] = b
	return int64(len(b)), nil
}

// Get implementa usecase.FileStorage.
func (s *Storage) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.Objects[key]
	if !ok {
		return nil, 0, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

// Delete implementa usecase.FileStorage.
func (s *Storage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Deleted = append(s.Deleted, key)
	delete(s.Objects, key)
	return nil
}

// Publisher simula o broker. FailAfter = -1 nunca falha.
type Publisher struct {
	mu        sync.Mutex
	Published []usecase.OutgoingMessage
	FailAfter int
}

// Publish registra a mensagem ou falha após FailAfter publicações.
func (p *Publisher) Publish(_ context.Context, topic string, body []byte, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.FailAfter >= 0 && len(p.Published) >= p.FailAfter {
		return errors.New("broker indisponível")
	}
	p.Published = append(p.Published, usecase.OutgoingMessage{ID: id, Topic: topic, Payload: body})
	return nil
}

// Count retorna quantas mensagens foram publicadas.
func (p *Publisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.Published)
}
