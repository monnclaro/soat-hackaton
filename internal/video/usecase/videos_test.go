package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/video/domain"
	"github.com/monnclaro/soat-hackaton/internal/video/usecase"
	"github.com/monnclaro/soat-hackaton/internal/video/videotest"
)

var ana = usecase.Owner{UserID: "11111111-1111-1111-1111-111111111111", Email: "ana@x.com", Name: "Ana"}

func newVideos() (*usecase.Videos, *videotest.Repo, *videotest.Cache, *videotest.Storage) {
	repo, cache, st := videotest.NewRepo(), videotest.NewCache(), videotest.NewStorage()
	return usecase.NewVideos(repo, cache, st, videotest.DiscardLogger()), repo, cache, st
}

func TestUploadPersistsVideoAndEnqueuesProcessing(t *testing.T) {
	uc, repo, cache, st := newVideos()
	v, err := uc.Upload(context.Background(), ana, "aula.mp4", strings.NewReader("conteudo"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != domain.StatusPending || v.SizeBytes != 8 || v.UserID != ana.UserID || v.UserEmail != ana.Email {
		t.Fatalf("video = %+v", v)
	}
	if _, ok := st.Objects[v.StorageKey]; !ok || !strings.HasPrefix(v.StorageKey, "videos/"+ana.UserID+"/") {
		t.Fatalf("object not stored at %s", v.StorageKey)
	}
	if len(repo.Outbox) != 1 || repo.Outbox[0].Topic != events.RoutingKeyVideoUploaded {
		t.Fatalf("outbox = %+v", repo.Outbox)
	}
	var evt events.VideoUploaded
	if err := json.Unmarshal(repo.Outbox[0].Payload, &evt); err != nil {
		t.Fatal(err)
	}
	if evt.VideoID != v.ID || evt.UserEmail != ana.Email || evt.UserName != ana.Name || evt.StorageKey != v.StorageKey {
		t.Fatalf("event = %+v", evt)
	}
	if len(cache.Invalidated) != 1 {
		t.Fatal("cache must be invalidated after upload")
	}
}

func TestUploadErrors(t *testing.T) {
	ctx := context.Background()

	uc, _, _, _ := newVideos()
	if _, err := uc.Upload(ctx, ana, "doc.pdf", strings.NewReader("x")); !errors.Is(err, domain.ErrUnsupportedType) {
		t.Fatalf("unsupported: %v", err)
	}

	uc, _, _, st := newVideos()
	if _, err := uc.Upload(ctx, ana, "a.mp4", strings.NewReader("")); !errors.Is(err, domain.ErrEmptyFile) {
		t.Fatalf("empty: %v", err)
	}
	if len(st.Deleted) != 1 {
		t.Fatal("empty object must be removed")
	}

	uc, _, _, st = newVideos()
	st.PutErr = errors.New("storage down")
	if _, err := uc.Upload(ctx, ana, "a.mp4", strings.NewReader("x")); err == nil {
		t.Fatal("expected storage error")
	}

	uc, repo, _, st := newVideos()
	repo.CreateErr = errors.New("db down")
	if _, err := uc.Upload(ctx, ana, "a.mp4", strings.NewReader("x")); err == nil {
		t.Fatal("expected repository error")
	}
	if len(st.Objects) != 0 || len(st.Deleted) != 1 {
		t.Fatal("orphan object must be compensated (deleted)")
	}
}

func TestListUsesCacheAndSurvivesCacheFailure(t *testing.T) {
	uc, repo, cache, _ := newVideos()
	ctx := context.Background()
	_, _ = uc.Upload(ctx, ana, "a.mp4", strings.NewReader("x"))
	_, _ = uc.Upload(ctx, usecase.Owner{UserID: "other"}, "b.mp4", strings.NewReader("y"))

	if list, err := uc.List(ctx, ana.UserID); err != nil || len(list) != 1 {
		t.Fatalf("list = %v %v", list, err)
	}
	_, _ = uc.List(ctx, ana.UserID)
	if repo.ListCalls != 1 {
		t.Fatalf("second list should hit the cache, repo calls = %d", repo.ListCalls)
	}

	cache.Err = errors.New("redis down")
	if list, err := uc.List(ctx, ana.UserID); err != nil || len(list) != 1 {
		t.Fatalf("list with cache down = %v %v", list, err)
	}

	cache.Err = nil
	cache.Lists = map[string][]domain.Video{}
	repo.GetErr = errors.New("db down")
	if _, err := uc.List(ctx, ana.UserID); err == nil {
		t.Fatal("expected repository error")
	}
}

func TestGetEnforcesOwnership(t *testing.T) {
	uc, _, _, _ := newVideos()
	ctx := context.Background()
	v, _ := uc.Upload(ctx, ana, "a.mp4", strings.NewReader("x"))
	if _, err := uc.Get(ctx, ana.UserID, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Get(ctx, "intruder", v.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other user must get not found: %v", err)
	}
	if _, err := uc.Get(ctx, ana.UserID, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestApplyStatusAndDownload(t *testing.T) {
	uc, repo, cache, st := newVideos()
	ctx := context.Background()
	v, _ := uc.Upload(ctx, ana, "aula.mp4", strings.NewReader("x"))

	if _, _, _, err := uc.Download(ctx, ana.UserID, v.ID); !errors.Is(err, domain.ErrNotReady) {
		t.Fatalf("pending download: %v", err)
	}

	if applied, err := uc.ApplyStatus(ctx, v.ID, domain.StatusChange{Status: domain.StatusProcessing}); !applied || err != nil {
		t.Fatalf("processing: %v %v", applied, err)
	}
	zipKey := "outputs/" + ana.UserID + "/" + v.ID + ".zip"
	st.Objects[zipKey] = []byte("PK-zip")
	if applied, _ := uc.ApplyStatus(ctx, v.ID, domain.StatusChange{Status: domain.StatusCompleted, ZipKey: zipKey, FrameCount: 12}); !applied {
		t.Fatal("completed should apply")
	}
	if got := repo.Video(v.ID); got.Status != domain.StatusCompleted || got.FrameCount != 12 {
		t.Fatalf("completed video = %+v", got)
	}

	// Evento atrasado/duplicado é ignorado.
	if applied, err := uc.ApplyStatus(ctx, v.ID, domain.StatusChange{Status: domain.StatusFailed}); applied || err != nil {
		t.Fatalf("late event must be ignored: %v %v", applied, err)
	}
	if len(cache.Invalidated) < 3 {
		t.Fatalf("cache must be invalidated on each applied change: %v", cache.Invalidated)
	}

	rc, size, name, err := uc.Download(ctx, ana.UserID, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "PK-zip" || size != 6 || name != "aula_frames.zip" {
		t.Fatalf("download = %q %d %q", b, size, name)
	}
	if _, _, _, err := uc.Download(ctx, "intruder", v.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("intruder download: %v", err)
	}

	delete(st.Objects, zipKey)
	if _, _, _, err := uc.Download(ctx, ana.UserID, v.ID); err == nil {
		t.Fatal("missing zip must fail")
	}
}

func TestApplyStatusErrors(t *testing.T) {
	uc, repo, _, _ := newVideos()
	ctx := context.Background()
	if _, err := uc.ApplyStatus(ctx, "missing", domain.StatusChange{Status: domain.StatusFailed}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing video: %v", err)
	}

	v, _ := uc.Upload(ctx, ana, "a.mp4", strings.NewReader("x"))
	delete(repo.Videos, v.ID) // some entre o Get e o Update
	repo.Videos["ghost"] = v
	if _, err := uc.ApplyStatus(ctx, "ghost", domain.StatusChange{Status: domain.StatusFailed}); err == nil {
		t.Fatal("update error must propagate")
	}
}
