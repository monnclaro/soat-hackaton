package processor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
)

// FFmpeg extrai frames usando o binário ffmpeg (mesma estratégia do projeto base: N frames por segundo em PNG).
type FFmpeg struct {
	Binary string
	FPS    float64
}

// Extract executa o ffmpeg e retorna os frames gerados em ordem.
func (f FFmpeg) Extract(ctx context.Context, videoPath, outDir string) ([]string, error) {
	bin := f.Binary
	if bin == "" {
		bin = "ffmpeg"
	}
	fps := f.FPS
	if fps <= 0 {
		fps = 1
	}
	pattern := filepath.Join(outDir, "frame_%05d.png")
	cmd := exec.CommandContext(ctx, bin,
		"-hide_banner", "-loglevel", "error",
		"-i", videoPath,
		"-vf", "fps="+strconv.FormatFloat(fps, 'f', -1, 64),
		"-y", pattern,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if _, ok := err.(*exec.ExitError); ok {
			// Vídeo inválido/corrompido: falha definitiva.
			return nil, &ProcessingError{Reason: "ffmpeg não conseguiu processar o vídeo: " + truncate(stderr.String(), 300)}
		}
		// Binário ausente ou erro do SO: falha transitória/infra.
		return nil, fmt.Errorf("executando ffmpeg: %w", err)
	}
	frames, err := filepath.Glob(filepath.Join(outDir, "frame_*.png"))
	if err != nil {
		return nil, err
	}
	sort.Strings(frames)
	return frames, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
