package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
	"github.com/monnclaro/soat-hackaton/internal/video/domain"
	"github.com/monnclaro/soat-hackaton/internal/video/usecase"
	"github.com/monnclaro/soat-hackaton/internal/video/videotest"
)

func TestStatusHandler(t *testing.T) {
	repo := videotest.NewRepo()
	videos := usecase.NewVideos(repo, videotest.NewCache(), videotest.NewStorage(), videotest.DiscardLogger())
	v, _ := videos.Upload(context.Background(), usecase.Owner{UserID: "u1"}, "a.mp4", strings.NewReader("x"))
	h := StatusHandler(videos)
	ctx := context.Background()

	if err := h(ctx, rabbitmq.Delivery{Body: []byte("{bad")}); !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid json must be permanent: %v", err)
	}
	body, _ := json.Marshal(events.VideoStatusChanged{VideoID: "missing", Status: events.StatusProcessing})
	if err := h(ctx, rabbitmq.Delivery{Body: body}); !rabbitmq.IsPermanent(err) {
		t.Fatalf("unknown video must be permanent: %v", err)
	}

	body, _ = json.Marshal(events.VideoStatusChanged{VideoID: v.ID, Status: events.StatusCompleted, ZipKey: "z.zip", FrameCount: 4})
	if err := h(ctx, rabbitmq.Delivery{Body: body}); err != nil {
		t.Fatal(err)
	}
	if got := repo.Video(v.ID); got.Status != domain.StatusCompleted || got.ZipKey != "z.zip" || got.FrameCount != 4 {
		t.Fatalf("status not applied: %+v", got)
	}

	repo.GetErr = errors.New("db down")
	if err := h(ctx, rabbitmq.Delivery{Body: body}); err == nil || rabbitmq.IsPermanent(err) {
		t.Fatalf("infra error must be transient (requeue): %v", err)
	}
}

// memOutbox simula o DispatchPending do Postgres.
type memOutbox struct {
	mu      sync.Mutex
	pending []usecase.OutgoingMessage
	err     error
}

func (o *memOutbox) DispatchPending(ctx context.Context, limit int, publish func(context.Context, usecase.OutgoingMessage) error) (int, error) {
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

func (o *memOutbox) left() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.pending)
}

func TestOutboxRelay(t *testing.T) {
	store := &memOutbox{}
	for i := range 5 {
		store.pending = append(store.pending, usecase.OutgoingMessage{ID: fmt.Sprint(i), Topic: events.RoutingKeyVideoUploaded})
	}
	pub := &videotest.Publisher{FailAfter: 3}
	relay := NewOutboxRelay(store, pub, 10*time.Millisecond, 2, videotest.DiscardLogger())

	if n, err := relay.RunOnce(context.Background()); n != 2 || err != nil {
		t.Fatalf("first batch = %d %v", n, err)
	}
	if n, err := relay.RunOnce(context.Background()); n != 1 || err == nil {
		t.Fatalf("partial failure must report count and error: %d %v", n, err)
	}
	if store.left() != 2 {
		t.Fatalf("unpublished messages must stay pending, got %d", store.left())
	}

	// O broker volta: o loop drena o restante.
	pub.FailAfter = -1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	deadline := time.After(2 * time.Second)
	for store.left() > 0 {
		select {
		case <-deadline:
			t.Fatal("relay did not drain the outbox")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
	if pub.Count() != 5 {
		t.Fatalf("published = %d", pub.Count())
	}
}

func TestOutboxRelayStoreError(t *testing.T) {
	relay := NewOutboxRelay(&memOutbox{err: errors.New("db down")}, &videotest.Publisher{FailAfter: -1}, time.Millisecond, 10, videotest.DiscardLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	relay.Run(ctx) // deve retornar quando o ctx expirar
}
