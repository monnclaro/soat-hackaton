// notification-service: envia e-mail ao usuário quando o processamento de um vídeo falha.
// Este arquivo é a raiz de composição: cria os adaptadores e os injeta no caso de uso NotifyFailure.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/notification/adapter/messaging"
	"github.com/monnclaro/soat-hackaton/internal/notification/adapter/smtp"
	"github.com/monnclaro/soat-hackaton/internal/notification/usecase"
	"github.com/monnclaro/soat-hackaton/internal/platform/config"
	"github.com/monnclaro/soat-hackaton/internal/platform/httpx"
	"github.com/monnclaro/soat-hackaton/internal/platform/logging"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
)

func main() {
	log := logging.New("notification-service", config.String("LOG_LEVEL", "info"))
	if err := run(log); err != nil {
		log.Error("notification-service encerrado com erro", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	conn, err := rabbitmq.Dial(startCtx, config.String("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"), log)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := rabbitmq.Setup(conn, config.Int("RABBITMQ_DELIVERY_LIMIT", 5)); err != nil {
		return err
	}

	// Adaptador SMTP → caso de uso.
	sender := smtp.New(
		config.String("SMTP_ADDR", "localhost:1025"),
		config.String("SMTP_FROM", "FIAP X <no-reply@fiapx.local>"),
		config.String("SMTP_USERNAME", ""),
		config.String("SMTP_PASSWORD", ""),
	)
	notifyFailure := usecase.NewNotifyFailure(sender, log)

	mux := http.NewServeMux()
	httpx.RegisterOps(mux, func(context.Context) error {
		if conn.IsClosed() {
			return errors.New("rabbitmq desconectado")
		}
		return nil
	})

	ctx, cancelAll := context.WithCancelCause(ctx)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := rabbitmq.Consume(ctx, conn, rabbitmq.QueueNotifications, config.Int("CONSUMER_CONCURRENCY", 2), log, messaging.FailedVideoHandler(notifyFailure)); err != nil {
			cancelAll(err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := httpx.Serve(ctx, log, ":"+config.String("PORT", "8084"), httpx.Observe(log, mux)); err != nil {
			cancelAll(err)
		}
	}()
	<-ctx.Done()
	wg.Wait()
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	log.Info("notification-service finalizado")
	return nil
}
