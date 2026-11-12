// Package usecase contém o caso de uso NotifyFailure. Depende apenas do domínio e da porta Sender.
package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/monnclaro/soat-hackaton/internal/notification/domain"
)

// Sender envia a mensagem (SMTP, SES, SendGrid... implementado em adapter/).
type Sender interface {
	Send(ctx context.Context, msg domain.Message) error
}

// NotifyFailure avisa o usuário que o processamento de um vídeo falhou.
type NotifyFailure struct {
	sender Sender
	logger *slog.Logger
}

// NewNotifyFailure cria o caso de uso.
func NewNotifyFailure(sender Sender, logger *slog.Logger) *NotifyFailure {
	return &NotifyFailure{sender: sender, logger: logger}
}

// Execute envia o e-mail. Retorna sent = false quando não há destinatário.
// Erros de envio são retornados para que a mensagem seja reentregue.
func (uc *NotifyFailure) Execute(ctx context.Context, f domain.Failure) (bool, error) {
	if !f.Notifiable() {
		uc.logger.Warn("falha sem e-mail do usuário, notificação ignorada", "video_id", f.VideoID)
		return false, nil
	}
	if err := uc.sender.Send(ctx, domain.FailureMessage(f)); err != nil {
		return false, fmt.Errorf("enviando e-mail: %w", err)
	}
	uc.logger.Info("usuário notificado", "video_id", f.VideoID, "to", f.UserEmail)
	return true, nil
}
