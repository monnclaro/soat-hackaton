// Package domain contém a entidade Video e suas regras de negócio
// (máquina de estados, validação de arquivo, nomes de saída). Não depende de outras camadas.
package domain

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// Status do processamento de um vídeo.
type Status string

// Estados possíveis.
const (
	StatusPending    Status = "PENDING"
	StatusProcessing Status = "PROCESSING"
	StatusCompleted  Status = "COMPLETED"
	StatusFailed     Status = "FAILED"
)

// Erros de domínio.
var (
	ErrNotFound        = errors.New("vídeo não encontrado")
	ErrNotReady        = errors.New("vídeo ainda não foi processado")
	ErrUnsupportedType = errors.New("formato não suportado (use mp4, avi, mov, mkv, webm, wmv, flv ou m4v)")
	ErrEmptyFile       = errors.New("arquivo vazio")
	ErrInvalidFileName = errors.New("nome de arquivo inválido")
)

var allowedFormats = map[string]bool{
	".mp4": true, ".avi": true, ".mov": true, ".mkv": true,
	".webm": true, ".wmv": true, ".flv": true, ".m4v": true,
}

// Video é a entidade central do serviço.
type Video struct {
	ID           string
	UserID       string
	UserEmail    string
	OriginalName string
	StorageKey   string // onde está o vídeo original no object storage
	ZipKey       string // onde está o ZIP de frames (quando COMPLETED)
	SizeBytes    int64
	FrameCount   int
	Status       Status
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CanTransition define a máquina de estados:
// PENDING → PROCESSING → COMPLETED | FAILED. PROCESSING → PROCESSING é aceito (reentrega da mensagem).
// COMPLETED e FAILED são terminais.
func CanTransition(from, to Status) bool {
	switch from {
	case StatusPending, StatusProcessing:
		return to == StatusProcessing || to == StatusCompleted || to == StatusFailed
	default:
		return false
	}
}

// StatusChange é o resultado do processamento informado ao vídeo.
type StatusChange struct {
	Status     Status
	ZipKey     string
	FrameCount int
	Error      string
}

// Apply aplica a mudança de status se a máquina de estados permitir.
// Retorna false (sem alterar nada) para eventos duplicados ou fora de ordem.
func (v *Video) Apply(c StatusChange, now time.Time) bool {
	if !CanTransition(v.Status, c.Status) {
		return false
	}
	v.Status = c.Status
	v.UpdatedAt = now
	switch c.Status {
	case StatusCompleted:
		v.ZipKey = c.ZipKey
		v.FrameCount = c.FrameCount
		v.ErrorMessage = ""
	case StatusFailed:
		v.ErrorMessage = c.Error
	}
	return true
}

// OwnedBy indica se o vídeo pertence ao usuário.
func (v Video) OwnedBy(userID string) bool { return v.UserID == userID }

// Downloadable indica se o ZIP já pode ser baixado.
func (v Video) Downloadable() bool { return v.Status == StatusCompleted && v.ZipKey != "" }

// ValidateFileName valida o nome e a extensão, retornando o nome limpo (sem diretórios)
// e a extensão em minúsculas.
func ValidateFileName(name string) (string, string, error) {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "" || name == "." || name == "/" || len(name) > 255 {
		return "", "", ErrInvalidFileName
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedFormats[ext] {
		return "", "", ErrUnsupportedType
	}
	return name, ext, nil
}

// StorageKey define onde o vídeo original é guardado.
func StorageKey(userID, videoID, ext string) string {
	return "videos/" + userID + "/" + videoID + ext
}

// ZipFileName gera o nome do arquivo para download (ex.: "aula.mp4" → "aula_frames.zip").
func ZipFileName(originalName string) string {
	base := strings.TrimSuffix(originalName, filepath.Ext(originalName))
	if base == "" {
		base = "video"
	}
	return base + "_frames.zip"
}
