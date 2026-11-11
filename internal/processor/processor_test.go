package processor

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
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
		if err := os.WriteFile(p, []byte(fmt.Sprintf("png-%d", i)), 0o644); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

type capturePublisher struct {
	mu     sync.Mutex
	events []events.VideoStatusChanged
	keys   []string
	err    error
}

func (c *capturePublisher) Publish(_ context.Context, rk string, body []byte, _ string) error {
	if c.err != nil {
		return c.err
	}
	var e events.VideoStatusChanged
	if err := json.Unmarshal(body, &e); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	c.keys = append(c.keys, rk)
	return nil
}

func (c *capturePublisher) statuses() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var s []string
	for _, e := range c.events {
		s = append(s, e.Status)
	}
	return s
}

func uploaded(id string) events.VideoUploaded {
	return events.VideoUploaded{
		VideoID: id, UserID: "u1", UserEmail: "u1@x.com", OriginalName: "aula.mp4",
		StorageKey: "videos/u1/" + id + ".mp4", UploadedAt: time.Now(),
	}
}

func newProcessor(t *testing.T, ex FrameExtractor) (*Processor, *memStorage, *capturePublisher) {
	t.Helper()
	st := &memStorage{objects: map[string][]byte{}}
	pub := &capturePublisher{}
	p := New(st, ex, pub, Config{WorkDir: t.TempDir(), MaxAttempts: 3}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return p, st, pub
}

func TestHandleSuccessProducesZip(t *testing.T) {
	p, st, pub := newProcessor(t, fakeExtractor{frames: 3})
	evt := uploaded("v1")
	st.objects[evt.StorageKey] = []byte("video-bytes")

	if err := p.Handle(context.Background(), evt, 1); err != nil {
		t.Fatal(err)
	}
	got := pub.statuses()
	if len(got) != 2 || got[0] != events.StatusProcessing || got[1] != events.StatusCompleted {
		t.Fatalf("statuses = %v", got)
	}
	if pub.keys[1] != "video.status.completed" {
		t.Fatalf("routing key = %s", pub.keys[1])
	}
	done := pub.events[1]
	if done.FrameCount != 3 || done.ZipKey != "outputs/u1/v1.zip" || done.UserEmail != "u1@x.com" {
		t.Fatalf("completed event = %+v", done)
	}

	zr, err := zip.NewReader(bytes.NewReader(st.objects[done.ZipKey]), int64(len(st.objects[done.ZipKey])))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 3 || zr.File[0].Name != "frame_00001.png" {
		t.Fatalf("zip entries = %d first=%s", len(zr.File), zr.File[0].Name)
	}

	entries, _ := os.ReadDir(p.cfg.WorkDir)
	if len(entries) != 0 {
		t.Fatalf("temp dirs must be cleaned, found %d", len(entries))
	}
}

func TestHandlePermanentFailureMarksFailed(t *testing.T) {
	for _, ex := range []fakeExtractor{
		{err: &ProcessingError{Reason: "vídeo corrompido"}},
		{frames: 0},
	} {
		p, st, pub := newProcessor(t, ex)
		evt := uploaded("v2")
		st.objects[evt.StorageKey] = []byte("x")
		if err := p.Handle(context.Background(), evt, 1); err != nil {
			t.Fatalf("permanent failure must be acked, got %v", err)
		}
		got := pub.statuses()
		if len(got) != 2 || got[1] != events.StatusFailed || pub.events[1].Error == "" {
			t.Fatalf("statuses = %v events=%+v", got, pub.events)
		}
		if pub.keys[1] != "video.status.failed" {
			t.Fatalf("routing key = %s", pub.keys[1])
		}
	}
}

func TestHandleTransientFailureRetriesThenFails(t *testing.T) {
	p, st, pub := newProcessor(t, fakeExtractor{frames: 1})
	evt := uploaded("v3")
	st.objects[evt.StorageKey] = []byte("x")
	st.putErr = errors.New("s3 timeout")

	if err := p.Handle(context.Background(), evt, 1); err == nil {
		t.Fatal("transient failure should return error (requeue)")
	}
	if got := pub.statuses(); len(got) != 1 || got[0] != events.StatusProcessing {
		t.Fatalf("no terminal status before last attempt: %v", got)
	}

	if err := p.Handle(context.Background(), evt, 3); err != nil {
		t.Fatalf("last attempt must ack, got %v", err)
	}
	got := pub.statuses()
	if got[len(got)-1] != events.StatusFailed {
		t.Fatalf("last attempt should mark failed: %v", got)
	}
}

func TestHandleDownloadErrorIsTransient(t *testing.T) {
	p, st, _ := newProcessor(t, fakeExtractor{frames: 1})
	st.getErr = errors.New("s3 down")
	if err := p.Handle(context.Background(), uploaded("v4"), 1); err == nil {
		t.Fatal("expected transient error")
	}
}

func TestHandlePublishErrorRequeues(t *testing.T) {
	p, st, pub := newProcessor(t, fakeExtractor{frames: 1})
	evt := uploaded("v5")
	st.objects[evt.StorageKey] = []byte("x")
	pub.err = errors.New("broker down")
	if err := p.Handle(context.Background(), evt, 1); err == nil {
		t.Fatal("publish failure must requeue")
	}
}

func TestHandleTimeoutIsPermanent(t *testing.T) {
	slow := extractorFunc(func(ctx context.Context, _, _ string) ([]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	p, st, pub := newProcessor(t, slow)
	p.cfg.JobTimeout = 20 * time.Millisecond
	evt := uploaded("v6")
	st.objects[evt.StorageKey] = []byte("x")
	if err := p.Handle(context.Background(), evt, 1); err != nil {
		t.Fatal(err)
	}
	if got := pub.statuses(); got[len(got)-1] != events.StatusFailed {
		t.Fatalf("timeout should fail the video: %v", got)
	}
}

type extractorFunc func(ctx context.Context, videoPath, outDir string) ([]string, error)

func (f extractorFunc) Extract(ctx context.Context, v, o string) ([]string, error) {
	return f(ctx, v, o)
}

func TestConcurrentJobs(t *testing.T) {
	p, st, pub := newProcessor(t, fakeExtractor{frames: 2})
	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		evt := uploaded(fmt.Sprintf("c%d", i))
		st.objects[evt.StorageKey] = []byte("x")
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Handle(context.Background(), evt, 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	completed := 0
	for _, s := range pub.statuses() {
		if s == events.StatusCompleted {
			completed++
		}
	}
	if completed != n {
		t.Fatalf("completed = %d, want %d", completed, n)
	}
}

func TestHandlerAdapter(t *testing.T) {
	p, st, pub := newProcessor(t, fakeExtractor{frames: 1})
	h := Handler(p)
	if err := h(context.Background(), rabbitmq.Delivery{Body: []byte("{}")}); !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid message should be permanent: %v", err)
	}
	evt := uploaded("h1")
	st.objects[evt.StorageKey] = []byte("x")
	body, _ := json.Marshal(evt)
	if err := h(context.Background(), rabbitmq.Delivery{Body: body, Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	if got := pub.statuses(); got[len(got)-1] != events.StatusCompleted {
		t.Fatalf("statuses = %v", got)
	}
}

func TestNewDefaults(t *testing.T) {
	p := New(nil, nil, nil, Config{}, slog.Default())
	if p.cfg.MaxAttempts != 3 || p.cfg.JobTimeout != 30*time.Minute {
		t.Fatalf("defaults = %+v", p.cfg)
	}
}

func TestZipFilesErrors(t *testing.T) {
	if err := ZipFiles([]string{filepath.Join(t.TempDir(), "missing.png")}, io.Discard); err == nil {
		t.Fatal("expected error for missing file")
	}
	if err := ZipFilesToPath(nil, filepath.Join(t.TempDir(), "no", "dir", "x.zip")); err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestFFmpegExtractErrors(t *testing.T) {
	dir := t.TempDir()
	// Binário inexistente → erro de infraestrutura (transitório).
	_, err := FFmpeg{Binary: filepath.Join(dir, "no-ffmpeg")}.Extract(context.Background(), "in.mp4", dir)
	var perr *ProcessingError
	if err == nil || errors.As(err, &perr) {
		t.Fatalf("missing binary should be infra error, got %v", err)
	}

	// Binário que termina com código != 0 → falha definitiva do vídeo.
	// Usa o próprio binário de teste: ele rejeita as flags do ffmpeg e sai com código 2.
	self, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	_, err = FFmpeg{Binary: self, FPS: 2}.Extract(context.Background(), "in.mp4", dir)
	if !errors.As(err, &perr) {
		t.Fatalf("non-zero exit should be ProcessingError, got %v", err)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("abc", 5) != "abc" || truncate("abcdef", 3) != "abc..." {
		t.Fatal("truncate")
	}
}
