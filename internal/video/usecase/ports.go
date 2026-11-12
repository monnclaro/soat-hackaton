// Package usecase contém os casos de uso de vídeo: enviar, listar, consultar, baixar
// e aplicar o resultado do processamento. Depende apenas do domínio, do contrato de
// eventos (internal/events) e das portas declaradas aqui.
package usecase

import (
	"context"
	"io"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/video/domain"
)

// OutgoingMessage é um evento a ser publicado de forma assíncrona e confiável
// (gravado junto com o vídeo, na mesma transação).
type OutgoingMessage struct {
	ID        string
	Topic     string // routing key, ex.: "video.uploaded"
	Payload   []byte
	CreatedAt time.Time
}

// Repository persiste vídeos.
type Repository interface {
	// CreateAndEnqueue grava o vídeo e a mensagem de forma atômica.
	CreateAndEnqueue(ctx context.Context, v domain.Video, msg OutgoingMessage) error
	// Get retorna domain.ErrNotFound se não existir.
	Get(ctx context.Context, id string) (domain.Video, error)
	ListByUser(ctx context.Context, userID string) ([]domain.Video, error)
	Update(ctx context.Context, v domain.Video) error
}

// ListCache guarda a listagem de vídeos por usuário (opcional para a corretude).
type ListCache interface {
	Get(ctx context.Context, userID string) ([]domain.Video, bool, error)
	Set(ctx context.Context, userID string, videos []domain.Video) error
	Invalidate(ctx context.Context, userID string) error
}

// FileStorage guarda os arquivos (vídeos e ZIPs).
type FileStorage interface {
	// Put envia o conteúdo (size = -1 quando desconhecido) e retorna os bytes gravados.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (int64, error)
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, key string) error
}
