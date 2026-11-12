package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/processor/domain"
)

type memStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
	getErr  error
	putErr  error
}

func (s *memStorage) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return nil, 0, s.getErr
	}
	b, ok := s.objects[key]
	if !ok {
		return nil, 0, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

func (s *memStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) (int64, error) {
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

type fakeExtractor struct {
	frames int
	err    error
}

func (f fakeExtractor) Extract(_ context.Context, videoPath, outDir string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if _, err := os.Stat(videoPath); err != nil {
		return nil, fmt.Errorf("video not downloaded: %w", err)
	}
	var out []string
	for i := 1; i <= f.frames; i++ {
		p := filepath.Join(outDir, fmt.Sprintf("frame_%05d.png", i))
		if err := os.WriteFile(p, []byte("png"), 0o644); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

type extractorFunc func(ctx context.Context, videoPath, outDir string) ([]string, error)

func (f extractorFunc) Extract(ctx context.Context, v, o string) ([]string, error) {
	return f(ctx, v, o)
}

// listArchiver grava a lista de arquivos (o ZIP real é testado no adaptador archive).
type listArchiver struct{ err error }

func (a listArchiver) Archive(files []string, dest string) error {
	if a.err != nil {
		return a.err
	}
	var buf bytes.Buffer
	for _, f := range files {
		buf.WriteString(filepath.Base(f) + "\n")
	}
	return os.WriteFile(dest, buf.Bytes(), 0o644)
}

type capturePublisher struct {
	mu      sync.Mutex
	changes []domain.StatusChange
	failOn  domain.Status
}

func (c *capturePublisher) Publish(_ context.Context, _ domain.Job, ch domain.StatusChange) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch.Status == c.failOn {
		return errors.New("broker down")
	}
	c.changes = append(c.changes, ch)
	return nil
}

func (c *capturePublisher) statuses() []domain.Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	var s []domain.Status
	for _, ch := range c.changes {
		s = append(s, ch.Status)
	}
	return s
}

func (c *capturePublisher) last() domain.StatusChange {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.changes[len(c.changes)-1]
}

func job(id string) domain.Job {
	return domain.Job{VideoID: id, UserID: "u1", UserEmail: "u1@x.com", OriginalName: "aula.mp4", StorageKey: "videos/u1/" + id + ".mp4"}
}

func newUC(t *testing.T, ex FrameExtractor) (*ProcessVideo, *memStorage, *capturePublisher) {
	t.Helper()
	st := &memStorage{objects: map[string][]byte{}}
	pub := &capturePublisher{}
	uc := NewProcessVideo(st, ex, listArchiver{}, pub, Config{WorkDir: t.TempDir(), MaxAttempts: 3}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return uc, st, pub
}

func TestExecuteSuccess(t *testing.T) {
	uc, st, pub := newUC(t, fakeExtractor{frames: 3})
	j := job("v1")
	st.objects[j.StorageKey] = []byte("video")

	res, err := uc.Execute(context.Background(), j, 1)
	if err != nil || res.Outcome != domain.OutcomeCompleted || res.Frames != 3 {
		t.Fatalf("result = %+v %v", res, err)
	}
	got := pub.statuses()
	if len(got) != 2 || got[0] != domain.StatusProcessing || got[1] != domain.StatusCompleted {
		t.Fatalf("statuses = %v", got)
	}
	if last := pub.last(); last.ZipKey != "outputs/u1/v1.zip" || last.FrameCount != 3 {
		t.Fatalf("completed = %+v", last)
	}
	if string(st.objects["outputs/u1/v1.zip"]) != "frame_00001.png\nframe_00002.png\nframe_00003.png\n" {
		t.Fatalf("archive content = %q", st.objects["outputs/u1/v1.zip"])
	}
	if entries, _ := os.ReadDir(uc.cfg.WorkDir); len(entries) != 0 {
		t.Fatalf("temporary files must be removed, found %d", len(entries))
	}
}

func TestExecutePermanentFailure(t *testing.T) {
	for _, ex := range []FrameExtractor{
		fakeExtractor{err: &domain.ProcessingError{Reason: "vídeo corrompido"}},
		fakeExtractor{frames: 0},
	} {
		uc, st, pub := newUC(t, ex)
		j := job("v2")
		st.objects[j.StorageKey] = []byte("x")
		res, err := uc.Execute(context.Background(), j, 1)
		if err != nil || res.Outcome != domain.OutcomeFailed {
			t.Fatalf("permanent failure must be acked as failed: %+v %v", res, err)
		}
		if last := pub.last(); last.Status != domain.StatusFailed || last.Error == "" {
			t.Fatalf("failed = %+v", last)
		}
	}
}

func TestExecuteTransientFailureRetriesThenFails(t *testing.T) {
	uc, st, pub := newUC(t, fakeExtractor{frames: 1})
	j := job("v3")
	st.objects[j.StorageKey] = []byte("x")
	st.putErr = errors.New("storage timeout")

	res, err := uc.Execute(context.Background(), j, 1)
	if err == nil || res.Outcome != domain.OutcomeRetry {
		t.Fatalf("transient failure must retry: %+v %v", res, err)
	}
	if got := pub.statuses(); len(got) != 1 {
		t.Fatalf("no terminal status before the last attempt: %v", got)
	}

	res, err = uc.Execute(context.Background(), j, 3)
	if err != nil || res.Outcome != domain.OutcomeFailed || pub.last().Status != domain.StatusFailed {
		t.Fatalf("last attempt must fail: %+v %v", res, err)
	}
}

func TestExecuteOtherTransientErrors(t *testing.T) {
	uc, st, _ := newUC(t, fakeExtractor{frames: 1})
	st.getErr = errors.New("storage down")
	if _, err := uc.Execute(context.Background(), job("v4"), 1); err == nil {
		t.Fatal("download error must be transient")
	}

	uc, st, _ = newUC(t, fakeExtractor{frames: 1})
	uc.archiver = listArchiver{err: errors.New("disk full")}
	j := job("v5")
	st.objects[j.StorageKey] = []byte("x")
	if _, err := uc.Execute(context.Background(), j, 1); err == nil {
		t.Fatal("archive error must be transient")
	}
}

func TestExecutePublishErrors(t *testing.T) {
	for _, failOn := range []domain.Status{domain.StatusProcessing, domain.StatusCompleted, domain.StatusFailed} {
		ex := FrameExtractor(fakeExtractor{frames: 1})
		if failOn == domain.StatusFailed {
			ex = fakeExtractor{err: &domain.ProcessingError{Reason: "x"}}
		}
		uc, st, pub := newUC(t, ex)
		pub.failOn = failOn
		j := job("p")
		st.objects[j.StorageKey] = []byte("x")
		if res, err := uc.Execute(context.Background(), j, 1); err == nil || res.Outcome != domain.OutcomeRetry {
			t.Fatalf("publish %s failure must requeue: %+v %v", failOn, res, err)
		}
	}
}

func TestExecuteTimeoutIsPermanent(t *testing.T) {
	slow := extractorFunc(func(ctx context.Context, _, _ string) ([]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	uc, st, pub := newUC(t, slow)
	uc.cfg.JobTimeout = 20 * time.Millisecond
	j := job("v6")
	st.objects[j.StorageKey] = []byte("x")
	if res, err := uc.Execute(context.Background(), j, 1); err != nil || res.Outcome != domain.OutcomeFailed {
		t.Fatalf("timeout must fail the video: %+v %v", res, err)
	}
	if pub.last().Error != domain.ErrTimeout.Reason {
		t.Fatalf("reason = %q", pub.last().Error)
	}
}

func TestExecuteConcurrentJobs(t *testing.T) {
	uc, st, pub := newUC(t, fakeExtractor{frames: 2})
	const n = 20
	jobs := make([]domain.Job, n)
	for i := range n {
		jobs[i] = job(fmt.Sprintf("c%d", i))
		st.objects[jobs[i].StorageKey] = []byte("x")
	}
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := uc.Execute(context.Background(), j, 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	completed := 0
	for _, s := range pub.statuses() {
		if s == domain.StatusCompleted {
			completed++
		}
	}
	if completed != n {
		t.Fatalf("completed = %d, want %d", completed, n)
	}
}

func TestDefaults(t *testing.T) {
	uc := NewProcessVideo(nil, nil, nil, nil, Config{}, slog.Default())
	if uc.cfg.MaxAttempts != 3 || uc.cfg.JobTimeout != 30*time.Minute || uc.cfg.WorkDir == "" {
		t.Fatalf("defaults = %+v", uc.cfg)
	}
}
