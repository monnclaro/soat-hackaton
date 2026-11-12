// Package video implementa o ciclo de vida dos vídeos: upload, enfileiramento (outbox),
// atualização de status, listagem por usuário e download do ZIP de frames.
package video

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
)

// Erros de domínio.
var (
	ErrNotFound         = errors.New("vídeo não encontrado")
	ErrNotReady         = errors.New("vídeo ainda não foi processado")
	ErrUnsupportedType  = errors.New("formato não suportado (use mp4, avi, mov, mkv, webm, wmv, flv ou m4v)")
	ErrEmptyFile        = errors.New("arquivo vazio")
	ErrInvalidFileName  = errors.New("nome de arquivo inválido")
	ErrInvalidState     = errors.New("transição de status inválida")
	allowedVideoFormats = map[string]bool{
		".mp4": true, ".avi": true, ".mov": true, ".mkv": true,
		".webm": true, ".wmv": true, ".flv": true, ".m4v": true,
	}
)

// Video é o agregado de vídeo. Os campos JSON são usados pelo cache.
type Video struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	UserEmail    string    `json:"user_email"`
	OriginalName string    `json:"original_name"`
	StorageKey   string    `json:"storage_key"`
	ZipKey       string    `json:"zip_key,omitempty"`
	SizeBytes    int64     `json:"size_bytes"`
	FrameCount   int       `json:"frame_count"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CanTransition define a máquina de estados do vídeo.
// PENDING → PROCESSING → COMPLETED | FAILED. PROCESSING → PROCESSING é aceito (reentrega da mensagem).
// COMPLETED e FAILED são terminais (eventos atrasados/duplicados são ignorados).
func CanTransition(from, to string) bool {
	switch from {
	case events.StatusPending:
		return to == events.StatusProcessing || to == events.StatusCompleted || to == events.StatusFailed
	case events.StatusProcessing:
		return to == events.StatusProcessing || to == events.StatusCompleted || to == events.StatusFailed
	default:
		return false
	}
}

// ValidateFileName valida o nome e a extensão, retornando o nome limpo e a extensão em minúsculas.
func ValidateFileName(name string) (string, string, error) {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "" || name == "." || name == "/" || len(name) > 255 {
		return "", "", ErrInvalidFileName
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedVideoFormats[ext] {
		return "", "", ErrUnsupportedType
	}
	return name, ext, nil
}

// ZipFileName gera o nome do arquivo para download (ex.: "aula.mp4" → "aula_frames.zip").
func ZipFileName(originalName string) string {
	base := strings.TrimSuffix(originalName, filepath.Ext(originalName))
	if base == "" {
		base = "video"
	}
	return base + "_frames.zip"
}
