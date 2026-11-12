package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/notification/domain"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
)

type fakeNotifier struct {
	calls []domain.Failure
	sent  bool
	err   error
}

func (f *fakeNotifier) Execute(_ context.Context, fl domain.Failure) (bool, error) {
	f.calls = append(f.calls, fl)
	return f.sent, f.err
}

func TestFailedVideoHandler(t *testing.T) {
	n := &fakeNotifier{sent: true}
	h := FailedVideoHandler(n)
	ctx := context.Background()

	if err := h(ctx, rabbitmq.Delivery{Body: []byte("x")}); !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid json must be permanent: %v", err)
	}

	completed, _ := json.Marshal(events.VideoStatusChanged{VideoID: "v1", Status: events.StatusCompleted})
	if err := h(ctx, rabbitmq.Delivery{Body: completed}); err != nil || len(n.calls) != 0 {
		t.Fatalf("non-failed events must be ignored: %v %d", err, len(n.calls))
	}

	failed, _ := json.Marshal(events.VideoStatusChanged{VideoID: "v1", Status: events.StatusFailed, UserEmail: "a@b.com", OriginalName: "a.mp4", Error: "corrompido"})
	if err := h(ctx, rabbitmq.Delivery{Body: failed}); err != nil {
		t.Fatal(err)
	}
	if got := n.calls[0]; got.UserEmail != "a@b.com" || got.Reason != "corrompido" || got.OriginalName != "a.mp4" {
		t.Fatalf("failure = %+v", got)
	}

	n.sent = false
	if err := h(ctx, rabbitmq.Delivery{Body: failed}); err != nil {
		t.Fatal(err)
	}

	n.err = errors.New("smtp down")
	if err := h(ctx, rabbitmq.Delivery{Body: failed}); err == nil || rabbitmq.IsPermanent(err) {
		t.Fatalf("send failure must be transient: %v", err)
	}
}
