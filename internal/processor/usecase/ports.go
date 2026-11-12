// Package usecase contém o caso de uso ProcessVideo: baixar o vídeo, extrair os frames,
// compactar, guardar o ZIP e publicar o resultado. Depende apenas do domínio e das portas abaixo.
package usecase

import (
	"context"
	"io"

	"github.com/monnclaro/soat-hackaton/internal/processor/domain"
)

// FileStorage lê o vídeo original e grava o ZIP.
type FileStorage interface {
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (int64, error)
}

// FrameExtractor extrai frames do vídeo para um diretório e retorna os arquivos gerados, em ordem.
// Deve retornar *domain.ProcessingError quando o vídeo for inválido.
type FrameExtractor interface {
	Extract(ctx context.Context, videoPath, outDir string) ([]string, error)
}

// Archiver compacta arquivos em um único pacote (ZIP).
type Archiver interface {
	Archive(files []string, destPath string) error
}

// StatusPublisher informa o andamento do job ao restante do sistema.
type StatusPublisher interface {
	Publish(ctx context.Context, job domain.Job, change domain.StatusChange) error
}
