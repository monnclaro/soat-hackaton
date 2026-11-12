package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
	"github.com/monnclaro/soat-hackaton/internal/processor/domain"
	"github.com/monnclaro/soat-hackaton/internal/processor/usecase"
)

type fakeExecutor struct {
	got     domain.Job
	attempt int
	res     usecase.Result
	err     error
}

func (f *fakeExecutor) Execute(_ context.Context, j domain.Job, attempt int) (usecase.Result, error) {
	f.got, f.attempt = j, attempt
	return f.res, f.err
}

func TestVideoUploadedHandler(t *testing.T) {
	ex := &fakeExecutor{res: usecase.Result{Outcome: domain.OutcomeCompleted, Frames: 5, Duration: time.Second}}
	h := VideoUploadedHandler(ex)

	if err := h(context.Background(), rabbitmq.Delivery{Body: []byte("{}")}); !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid message must be permanent: %v", err)
	}

	body, _ := json.Marshal(events.VideoUploaded{VideoID: "v1", UserID: "u1", UserEmail: "a@b.com", OriginalName: "a.mp4", StorageKey: "videos/u1/v1.mp4"})
	if err := h(context.Background(), rabbitmq.Delivery{Body: body, Attempt: 2}); err != nil {
		t.Fatal(err)
	}
	if ex.got.VideoID != "v1" || ex.got.StorageKey != "videos/u1/v1.mp4" || ex.got.UserEmail != "a@b.com" || ex.attempt != 2 {
		t.Fatalf("job = %+v attempt=%d", ex.got, ex.attempt)
	}

	ex.res, ex.err = usecase.Result{Outcome: domain.OutcomeRetry}, errors.New("transient")
	if err := h(context.Background(), rabbitmq.Delivery{Body: body}); err == nil || rabbitmq.IsPermanent(err) {
		t.Fatalf("transient error must requeue: %v", err)
	}
}

type captureBroker struct {
	key, id string
	body    []byte
	err     error
}

func (b *captureBroker) Publish(_ context.Context, key string, body []byte, id string) error {
	b.key, b.body, b.id = key, body, id
	return b.err
}

func TestStatusPublisher(t *testing.T) {
	b := &captureBroker{}
	p := NewStatusPublisher(b)
	j := domain.Job{VideoID: "v1", UserID: "u1", UserEmail: "a@b.com", UserName: "Ana", OriginalName: "a.mp4"}

	if err := p.Publish(context.Background(), j, domain.StatusChange{Status: domain.StatusFailed, Error: "corrompido"}); err != nil {
		t.Fatal(err)
	}
	if b.key != "video.status.failed" || b.id != "v1-failed" {
		t.Fatalf("key=%s id=%s", b.key, b.id)
	}
	var evt events.VideoStatusChanged
	_ = json.Unmarshal(b.body, &evt)
	if evt.Status != "FAILED" || evt.Error != "corrompido" || evt.UserEmail != "a@b.com" || evt.UserName != "Ana" || evt.OccurredAt.IsZero() {
		t.Fatalf("event = %+v", evt)
	}

	_ = p.Publish(context.Background(), j, domain.StatusChange{Status: domain.StatusCompleted, ZipKey: "z.zip", FrameCount: 3})
	_ = json.Unmarshal(b.body, &evt)
	if b.key != "video.status.completed" || evt.ZipKey != "z.zip" || evt.FrameCount != 3 {
		t.Fatalf("completed event = %+v", evt)
	}

	b.err = errors.New("broker down")
	if err := p.Publish(context.Background(), j, domain.StatusChange{Status: domain.StatusProcessing}); err == nil {
		t.Fatal("broker error must propagate")
	}
}
