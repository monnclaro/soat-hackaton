package ffmpeg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/monnclaro/soat-hackaton/internal/processor/domain"
)

func TestExtractErrors(t *testing.T) {
	dir := t.TempDir()
	var perr *domain.ProcessingError

	// Binário inexistente → erro de infraestrutura (transitório).
	_, err := Extractor{Binary: filepath.Join(dir, "no-ffmpeg")}.Extract(context.Background(), "in.mp4", dir)
	if err == nil || errors.As(err, &perr) {
		t.Fatalf("missing binary must be an infra error, got %v", err)
	}

	// Binário que termina com código != 0 → falha definitiva do vídeo.
	// Usa o próprio binário de teste: ele rejeita as flags do ffmpeg e sai com código 2.
	self, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	_, err = Extractor{Binary: self, FPS: 2}.Extract(context.Background(), "in.mp4", dir)
	if !errors.As(err, &perr) {
		t.Fatalf("non-zero exit must be ProcessingError, got %v", err)
	}

	// Contexto cancelado → devolve o erro do contexto.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Extractor{Binary: self}).Extract(ctx, "in.mp4", dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context: %v", err)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("abc", 5) != "abc" || truncate("abcdef", 3) != "abc..." {
		t.Fatal("truncate")
	}
}
