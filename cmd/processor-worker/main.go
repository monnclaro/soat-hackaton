// processor-worker: consome a fila de vídeos e processa vários em paralelo (WORKER_CONCURRENCY por réplica).
// Escala horizontalmente: cada réplica é mais um consumidor competindo pela mesma fila.
// Este arquivo é a raiz de composição: cria os adaptadores e os injeta no caso de uso ProcessVideo.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/platform/config"
	"github.com/monnclaro/soat-hackaton/internal/platform/httpx"
	"github.com/monnclaro/soat-hackaton/internal/platform/logging"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
	"github.com/monnclaro/soat-hackaton/internal/platform/storage"
	"github.com/monnclaro/soat-hackaton/internal/processor/adapter/archive"
	"github.com/monnclaro/soat-hackaton/internal/processor/adapter/ffmpeg"
	"github.com/monnclaro/soat-hackaton/internal/processor/adapter/messaging"
	"github.com/monnclaro/soat-hackaton/internal/processor/usecase"
)

func main() {
	log := logging.New("processor-worker", config.String("LOG_LEVEL", "info"))
	if err := run(log); err != nil {
		log.Error("processor-worker encerrado com erro", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	st, err := storage.New(startCtx, storage.Config{
		Endpoint:  config.String("S3_ENDPOINT", "localhost:8333"),
		AccessKey: config.String("S3_ACCESS_KEY", "fiapx-access-key"),
		SecretKey: config.String("S3_SECRET_KEY", "fiapx-secret-key"),
		Bucket:    config.String("S3_BUCKET", "fiapx-videos"),
		UseSSL:    config.String("S3_USE_SSL", "false") == "true",
		Region:    config.String("S3_REGION", "us-east-1"),
	}, log)
	if err != nil {
		return err
	}

	conn, err := rabbitmq.Dial(startCtx, config.String("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"), log)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	deliveryLimit := config.Int("RABBITMQ_DELIVERY_LIMIT", 5)
	if err := rabbitmq.Setup(conn, deliveryLimit); err != nil {
		return err
	}
	pub, err := rabbitmq.NewPublisher(conn)
	if err != nil {
		return err
	}
	defer func() { _ = pub.Close() }()

	fps, _ := strconv.ParseFloat(config.String("FRAME_FPS", "1"), 64)
	// Adaptadores → caso de uso.
	processVideo := usecase.NewProcessVideo(
		st,
		ffmpeg.Extractor{Binary: config.String("FFMPEG_BIN", "ffmpeg"), FPS: fps},
		archive.Zip{},
		messaging.NewStatusPublisher(pub),
		usecase.Config{
			WorkDir:     config.String("WORK_DIR", os.TempDir()+"/fiapx"),
			JobTimeout:  config.Duration("JOB_TIMEOUT", 30*time.Minute),
			MaxAttempts: config.Int("MAX_ATTEMPTS", 3),
		},
		log,
	)

	mux := http.NewServeMux()
	httpx.RegisterOps(mux, func(ctx context.Context) error {
		if conn.IsClosed() {
			return errors.New("rabbitmq desconectado")
		}
		return st.Ping(ctx)
	})

	ctx, cancelAll := context.WithCancelCause(ctx)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		concurrency := config.Int("WORKER_CONCURRENCY", 2)
		if err := rabbitmq.Consume(ctx, conn, rabbitmq.QueueVideoUploaded, concurrency, log, messaging.VideoUploadedHandler(processVideo)); err != nil {
			cancelAll(err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := httpx.Serve(ctx, log, ":"+config.String("PORT", "8083"), httpx.Observe(log, mux)); err != nil {
			cancelAll(err)
		}
	}()
	<-ctx.Done()
	wg.Wait()
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	log.Info("processor-worker finalizado")
	return nil
}
