package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

var ana = security.Principal{UserID: "11111111-1111-1111-1111-111111111111", Email: "ana@x.com", Name: "Ana"}

func newSvc() (*Service, *fakeRepo, *fakeCache, *fakeStorage) {
	repo, cache, st := newFakeRepo(), newFakeCache(), newFakeStorage()
	svc := NewService(repo, cache, st, discardLogger())
	n := 0
	svc.newID = func() string { n++; return fmt.Sprintf("id-%d", n) }
	return svc, repo, cache, st
}

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{events.StatusPending, events.StatusProcessing, true},
		{events.StatusPending, events.StatusCompleted, true},
		{events.StatusPending, events.StatusFailed, true},
		{events.StatusProcessing, events.StatusProcessing, true},
		{events.StatusProcessing, events.StatusCompleted, true},
		{events.StatusProcessing, events.StatusFailed, true},
		{events.StatusProcessing, events.StatusPending, false},
		{events.StatusCompleted, events.StatusProcessing, false},
		{events.StatusCompleted, events.StatusFailed, false},
		{events.StatusFailed, events.StatusCompleted, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.want {
			t.Errorf("%s → %s = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestValidateFileName(t *testing.T) {
	name, ext, err := ValidateFileName(`C:\videos\Aula 1.MP4`)
	if err != nil || name != "Aula 1.MP4" || ext != ".mp4" {
		t.Fatalf("got %q %q %v", name, ext, err)
	}
	if _, _, err := ValidateFileName("../../etc/passwd.mkv"); err != nil {
		t.Fatalf("path traversal should be stripped to base name, got %v", err)
	}
	for _, bad := range []string{"", "  ", "file.txt", "noext"} {
		if _, _, err := ValidateFileName(bad); err == nil {
			t.Errorf("ValidateFileName(%q) expected error", bad)
		}
	}
	if _, _, err := ValidateFileName(strings.Repeat("a", 300) + ".mp4"); !errors.Is(err, ErrInvalidFileName) {
		t.Fatalf("long name: %v", err)
	}
}

func TestZipFileName(t *testing.T) {
	if got := ZipFileName("aula.mp4"); got != "aula_frames.zip" {
		t.Fatal(got)
	}
	if got := ZipFileName(".mp4"); got != "video_frames.zip" {
		t.Fatal(got)
	}
}

func TestUploadPersistsVideoAndOutbox(t *testing.T) {
	svc, repo, cache, st := newSvc()
	v, err := svc.Upload(context.Background(), ana, "aula.mp4", strings.NewReader("conteudo"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != events.StatusPending || v.SizeBytes != 8 || v.UserID != ana.UserID {
		t.Fatalf("unexpected video %+v", v)
	}
	if _, ok := st.objects[v.StorageKey]; !ok || !strings.HasPrefix(v.StorageKey, "videos/"+ana.UserID+"/") {
		t.Fatalf("object not stored at %s", v.StorageKey)
	}
	if len(repo.outbox) != 1 || repo.outbox[0].RoutingKey != events.RoutingKeyVideoUploaded {
		t.Fatalf("outbox = %+v", repo.outbox)
	}
	var evt events.VideoUploaded
	if err := json.Unmarshal(repo.outbox[0].Payload, &evt); err != nil {
		t.Fatal(err)
	}
	if evt.VideoID != v.ID || evt.UserEmail != ana.Email || evt.StorageKey != v.StorageKey {
		t.Fatalf("event = %+v", evt)
	}
	if len(cache.invalidated) != 1 {
		t.Fatal("cache should be invalidated after upload")
	}
}

func TestUploadErrors(t *testing.T) {
	ctx := context.Background()

	svc, _, _, _ := newSvc()
	if _, err := svc.Upload(ctx, ana, "doc.pdf", strings.NewReader("x")); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("unsupported: %v", err)
	}

	svc, _, _, st := newSvc()
	if _, err := svc.Upload(ctx, ana, "a.mp4", strings.NewReader("")); !errors.Is(err, ErrEmptyFile) {
		t.Fatalf("empty: %v", err)
	}
	if len(st.deleted) != 1 {
		t.Fatal("empty object should be removed")
	}

	svc, _, _, st = newSvc()
	st.putErr = errors.New("s3 down")
	if _, err := svc.Upload(ctx, ana, "a.mp4", strings.NewReader("x")); err == nil {
		t.Fatal("expected storage error")
	}

	svc, repo, _, st := newSvc()
	repo.createErr = errors.New("db down")
	if _, err := svc.Upload(ctx, ana, "a.mp4", strings.NewReader("x")); err == nil {
		t.Fatal("expected repo error")
	}
	if len(st.objects) != 0 || len(st.deleted) != 1 {
		t.Fatal("orphan object must be compensated (deleted)")
	}
}

func TestListUsesCache(t *testing.T) {
	svc, repo, cache, _ := newSvc()
	ctx := context.Background()
	_, _ = svc.Upload(ctx, ana, "a.mp4", strings.NewReader("x"))
	_, _ = svc.Upload(ctx, security.Principal{UserID: "other"}, "b.mp4", strings.NewReader("y"))

	list, err := svc.List(ctx, ana.UserID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v %v", list, err)
	}
	if _, err := svc.List(ctx, ana.UserID); err != nil {
		t.Fatal(err)
	}
	if repo.listCalls != 1 {
		t.Fatalf("second list should hit cache, repo calls = %d", repo.listCalls)
	}

	// Cache indisponível não derruba a listagem.
	cache.err = errors.New("redis down")
	if list, err := svc.List(ctx, ana.UserID); err != nil || len(list) != 1 {
		t.Fatalf("list with cache down = %v %v", list, err)
	}

	repo.getErr = errors.New("db down")
	cache.err = nil
	cache.lists = map[string][]Video{}
	if _, err := svc.List(ctx, ana.UserID); err == nil {
		t.Fatal("expected repo error")
	}
}

func TestGetEnforcesOwnership(t *testing.T) {
	svc, _, _, _ := newSvc()
	ctx := context.Background()
	v, _ := svc.Upload(ctx, ana, "a.mp4", strings.NewReader("x"))
	if _, err := svc.Get(ctx, ana.UserID, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, "intruder", v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user should get not found, got %v", err)
	}
	if _, err := svc.Get(ctx, ana.UserID, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestApplyStatusAndDownload(t *testing.T) {
	svc, repo, cache, st := newSvc()
	ctx := context.Background()
	v, _ := svc.Upload(ctx, ana, "aula.mp4", strings.NewReader("x"))

	if _, _, _, err := svc.Download(ctx, ana.UserID, v.ID); !errors.Is(err, ErrNotReady) {
		t.Fatalf("pending download: %v", err)
	}

	apply := func(status string, extra func(*events.VideoStatusChanged)) {
		t.Helper()
		evt := events.VideoStatusChanged{VideoID: v.ID, Status: status, OccurredAt: time.Now()}
		if extra != nil {
			extra(&evt)
		}
		if err := svc.ApplyStatus(ctx, evt); err != nil {
			t.Fatal(err)
		}
	}

	apply(events.StatusProcessing, nil)
	if repo.videos[v.ID].Status != events.StatusProcessing {
		t.Fatal("should be processing")
	}
	zipKey := "outputs/" + ana.UserID + "/" + v.ID + ".zip"
	st.objects[zipKey] = []byte("PK-zip")
	apply(events.StatusCompleted, func(e *events.VideoStatusChanged) { e.ZipKey = zipKey; e.FrameCount = 12 })
	got := repo.videos[v.ID]
	if got.Status != events.StatusCompleted || got.FrameCount != 12 || got.ZipKey != zipKey {
		t.Fatalf("completed video = %+v", got)
	}

	// Evento atrasado/duplicado é ignorado (estado terminal).
	apply(events.StatusFailed, func(e *events.VideoStatusChanged) { e.Error = "late" })
	if repo.videos[v.ID].Status != events.StatusCompleted {
		t.Fatal("terminal state must not change")
	}
	if len(cache.invalidated) < 3 {
		t.Fatalf("cache should be invalidated on each applied change: %v", cache.invalidated)
	}

	rc, size, name, err := svc.Download(ctx, ana.UserID, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "PK-zip" || size != 6 || name != "aula_frames.zip" {
		t.Fatalf("download = %q %d %q", b, size, name)
	}
	if _, _, _, err := svc.Download(ctx, "intruder", v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("intruder download: %v", err)
	}
}

func TestApplyStatusFailedStoresError(t *testing.T) {
	svc, repo, _, _ := newSvc()
	ctx := context.Background()
	v, _ := svc.Upload(ctx, ana, "a.mp4", strings.NewReader("x"))
	err := svc.ApplyStatus(ctx, events.VideoStatusChanged{VideoID: v.ID, Status: events.StatusFailed, Error: "ffmpeg falhou"})
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.videos[v.ID]; got.Status != events.StatusFailed || got.ErrorMessage != "ffmpeg falhou" {
		t.Fatalf("failed video = %+v", got)
	}
	if err := svc.ApplyStatus(ctx, events.VideoStatusChanged{VideoID: "nope", Status: events.StatusFailed}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing video: %v", err)
	}
}

func TestStatusHandler(t *testing.T) {
	svc, repo, _, _ := newSvc()
	ctx := context.Background()
	v, _ := svc.Upload(ctx, ana, "a.mp4", strings.NewReader("x"))
	h := StatusHandler(svc)

	if err := h(ctx, rabbitmq.Delivery{Body: []byte("{bad")}); !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid json should be permanent: %v", err)
	}
	body, _ := json.Marshal(events.VideoStatusChanged{VideoID: "missing", Status: events.StatusProcessing})
	if err := h(ctx, rabbitmq.Delivery{Body: body}); !rabbitmq.IsPermanent(err) {
		t.Fatalf("unknown video should be permanent: %v", err)
	}
	body, _ = json.Marshal(events.VideoStatusChanged{VideoID: v.ID, Status: events.StatusProcessing})
	if err := h(ctx, rabbitmq.Delivery{Body: body}); err != nil {
		t.Fatal(err)
	}
	if repo.videos[v.ID].Status != events.StatusProcessing {
		t.Fatal("status not applied")
	}
	repo.getErr = errors.New("db down")
	if err := h(ctx, rabbitmq.Delivery{Body: body}); err == nil || rabbitmq.IsPermanent(err) {
		t.Fatalf("infra error should be transient (requeue): %v", err)
	}
}

func TestOutboxRelay(t *testing.T) {
	store := &memOutbox{}
	for i := range 5 {
		store.pending = append(store.pending, OutboxMessage{ID: fmt.Sprint(i), RoutingKey: "video.uploaded"})
	}
	pub := &fakePublisher{failAfter: 3}
	relay := NewOutboxRelay(store, pub, 10*time.Millisecond, 2, discardLogger())

	n, err := relay.RunOnce(context.Background())
	if n != 2 || err != nil {
		t.Fatalf("first batch = %d %v", n, err)
	}
	n, err = relay.RunOnce(context.Background())
	if n != 1 || err == nil {
		t.Fatalf("partial failure should report published count and error: %d %v", n, err)
	}
	if len(store.pending) != 2 {
		t.Fatalf("unpublished messages must remain pending, got %d", len(store.pending))
	}

	// Broker volta: o loop drena o restante.
	pub.failAfter = -1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	deadline := time.After(2 * time.Second)
	for {
		store.mu.Lock()
		left := len(store.pending)
		store.mu.Unlock()
		if left == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("relay did not drain outbox")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
	if len(pub.published) != 5 {
		t.Fatalf("published = %d", len(pub.published))
	}
}

func TestOutboxRelayLogsStoreError(t *testing.T) {
	store := &memOutbox{err: errors.New("db down")}
	relay := NewOutboxRelay(store, &fakePublisher{failAfter: -1}, time.Millisecond, 10, discardLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	relay.Run(ctx) // deve retornar quando o ctx expirar, sem pânico
}
