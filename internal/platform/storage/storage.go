// Package storage implementa armazenamento de objetos S3-compatível (SeaweedFS, Garage, Ceph, MinIO, Cloudflare R2, AWS S3...).
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrNotFound indica objeto inexistente.
var ErrNotFound = errors.New("objeto não encontrado")

// Config de acesso ao bucket.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	Region    string
}

// S3 é o cliente de armazenamento.
type S3 struct {
	client *minio.Client
	bucket string
}

// New cria o cliente e garante que o bucket existe (com retentativas).
func New(ctx context.Context, cfg Config, logger *slog.Logger) (*S3, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("criando cliente s3: %w", err)
	}
	s := &S3{client: client, bucket: cfg.Bucket}
	backoff := 500 * time.Millisecond
	for {
		err = s.ensureBucket(ctx, cfg.Region)
		if err == nil {
			return s, nil
		}
		logger.Warn("storage indisponível, tentando novamente", "error", err, "retry_in", backoff.String())
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("conectando ao storage: %w", err)
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 10*time.Second)
	}
}

func (s *S3) ensureBucket(ctx context.Context, region string) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	err = s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region})
	if err != nil {
		// Outra réplica pode ter criado o bucket em paralelo.
		if resp := minio.ToErrorResponse(err); resp.Code == "BucketAlreadyOwnedByYou" || resp.Code == "BucketAlreadyExists" {
			return nil
		}
	}
	return err
}

// Put envia o conteúdo. size = -1 quando desconhecido (upload multipart em streaming).
func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (int64, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return 0, fmt.Errorf("enviando %s: %w", key, err)
	}
	return info.Size, nil
}

// Get abre o objeto para leitura. O chamador deve fechar o reader.
func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("lendo %s: %w", key, err)
	}
	st, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("lendo %s: %w", key, err)
	}
	return obj, st.Size, nil
}

// Delete remove o objeto (idempotente).
func (s *S3) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// Ping verifica se o bucket está acessível (readiness).
func (s *S3) Ping(ctx context.Context) error {
	_, err := s.client.BucketExists(ctx, s.bucket)
	return err
}
