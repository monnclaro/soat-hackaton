package usecase

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/monnclaro/soat-hackaton/internal/notification/domain"
)

type fakeSender struct {
	sent []domain.Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, m domain.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

func TestNotifyFailure(t *testing.T) {
	s := &fakeSender{}
	uc := NewNotifyFailure(s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	if sent, err := uc.Execute(ctx, domain.Failure{VideoID: "v1"}); sent || err != nil {
		t.Fatalf("no e-mail must be skipped: %v %v", sent, err)
	}

	f := domain.Failure{VideoID: "v1", UserEmail: "ana@x.com", OriginalName: "a.mp4", Reason: "corrompido"}
	if sent, err := uc.Execute(ctx, f); !sent || err != nil || len(s.sent) != 1 || s.sent[0].To != "ana@x.com" {
		t.Fatalf("send: %v %v %+v", sent, err, s.sent)
	}

	s.err = errors.New("smtp down")
	if _, err := uc.Execute(ctx, f); err == nil {
		t.Fatal("send failure must return error (requeue)")
	}
}
