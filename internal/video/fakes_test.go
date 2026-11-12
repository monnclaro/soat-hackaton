package video

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"

	"github.com/monnclaro/soat-hackaton/internal/platform/storage"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type fakeRepo struct {
	mu        sync.Mutex
	videos    map[string]Video
	outbox    []OutboxMessage
	createErr error
	getErr    error
	listCalls int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{videos: map[string]Video{}} }

func (f *fakeRepo) CreateWithOutbox(_ context.Context, v Video, m OutboxMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	f.videos[v.ID] = v
	f.outbox = append(f.outbox, m)
	return nil
}

func (f *fakeRepo) Get(_ context.Context, id string) (Video, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return Video{}, f.getErr
	}
	v, ok := f.videos[id]
	if !ok {
		return Video{}, ErrNotFound
	}
	return v, nil
}

func (f *fakeRepo) ListByUser(_ context.Context, userID string) ([]Video, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	out := []Video{}
	for _, v := range f.videos {
		if v.UserID == userID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepo) Update(_ context.Context, v Video) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.videos[v.ID]; !ok {
		return ErrNotFound
	}
	f.videos[v.ID] = v
	return nil
}

type fakeCache struct {
	mu          sync.Mutex
	lists       map[string][]Video
	invalidated []string
	err         error
}

func newFakeCache() *fakeCache { return &fakeCache{lists: map[string][]Video{}} }

func (c *fakeCache) GetList(_ context.Context, userID string) ([]Video, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, false, c.err
	}
	v, ok := c.lists[userID]
	return v, ok, nil
}

func (c *fakeCache) SetList(_ context.Context, userID string, v []Video) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.lists[userID] = v
	return nil
}

func (c *fakeCache) Invalidate(_ context.Context, userID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalidated = append(c.invalidated, userID)
	delete(c.lists, userID)
	return c.err
}

type fakeStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
	putErr  error
	deleted []string
}

func newFakeStorage() *fakeStorage { return &fakeStorage{objects: map[string][]byte{}} }

func (s *fakeStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) (int64, error) {
	if s.putErr != nil {
		return 0, s.putErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = b
	return int64(len(b)), nil
}

func (s *fakeStorage) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.objects[key]
	if !ok {
		return nil, 0, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

func (s *fakeStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, key)
	delete(s.objects, key)
	return nil
}

type fakePublisher struct {
	mu        sync.Mutex
	published []OutboxMessage
	failAfter int // -1 = nunca falha
}

func (p *fakePublisher) Publish(_ context.Context, rk string, body []byte, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failAfter >= 0 && len(p.published) >= p.failAfter {
		return errors.New("broker indisponível")
	}
	p.published = append(p.published, OutboxMessage{ID: id, RoutingKey: rk, Payload: body})
	return nil
}

// memOutbox simula o DispatchPending do Postgres.
type memOutbox struct {
	mu      sync.Mutex
	pending []OutboxMessage
	err     error
}

func (o *memOutbox) DispatchPending(ctx context.Context, limit int, publish func(context.Context, OutboxMessage) error) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.err != nil {
		return 0, o.err
	}
	n := 0
	for n < limit && len(o.pending) > 0 {
		if err := publish(ctx, o.pending[0]); err != nil {
			return n, err
		}
		o.pending = o.pending[1:]
		n++
	}
	return n, nil
}
