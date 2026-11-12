// Package ffmpeg implementa a porta FrameExtractor executando o binário ffmpeg.
package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/monnclaro/soat-hackaton/internal/processor/domain"
)

// Extractor extrai N frames por segundo em PNG (mesma estratégia do projeto base).
type Extractor struct {
	Binary string  // caminho do ffmpeg (padrão: "ffmpeg" do PATH)
	FPS    float64 // frames por segundo de vídeo (padrão: 1)
}

// Extract executa o ffmpeg e retorna os frames gerados em ordem.
func (e Extractor) Extract(ctx context.Context, videoPath, outDir string) ([]string, error) {
	bin := e.Binary
	if bin == "" {
		bin = "ffmpeg"
	}
	fps := e.FPS
	if fps <= 0 {
		fps = 1
	}
	cmd := exec.CommandContext(ctx, bin,
		"-hide_banner", "-loglevel", "error",
		"-i", videoPath,
		"-vf", "fps="+strconv.FormatFloat(fps, 'f', -1, 64),
		"-y", filepath.Join(outDir, "frame_%05d.png"),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// O ffmpeg rodou e recusou o arquivo: vídeo inválido/corrompido (falha definitiva).
			return nil, &domain.ProcessingError{Reason: "ffmpeg não conseguiu processar o vídeo: " + truncate(stderr.String(), 300)}
		}
		// Binário ausente ou erro do sistema operacional: falha de infraestrutura (transitória).
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
