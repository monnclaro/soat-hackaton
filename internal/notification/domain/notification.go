// Package domain contém as regras da notificação: quando notificar e o conteúdo do e-mail.
// Não depende de outras camadas.
package domain

import (
	"fmt"
	"time"
)

// Failure descreve um vídeo cujo processamento falhou.
type Failure struct {
	VideoID      string
	UserEmail    string
	UserName     string
	OriginalName string
	Reason       string
	OccurredAt   time.Time
}

// Notifiable indica se há para quem enviar.
func (f Failure) Notifiable() bool { return f.UserEmail != "" }

// Message é a notificação a ser enviada.
type Message struct {
	To      string
	Subject string
	Body    string
}

// FailureMessage monta o e-mail de falha.
func FailureMessage(f Failure) Message {
	name := f.UserName
	if name == "" {
		name = "usuário"
	}
	reason := f.Reason
	if reason == "" {
		reason = "erro desconhecido"
	}
	body := fmt.Sprintf(`Olá, %s.

Infelizmente não conseguimos processar o seu vídeo "%s".

Motivo: %s
ID do vídeo: %s
Data: %s

Verifique se o arquivo é um vídeo válido e tente enviá-lo novamente.

Equipe FIAP X`, name, f.OriginalName, reason, f.VideoID, f.OccurredAt.Format(time.RFC1123))
	return Message{
		To:      f.UserEmail,
		Subject: fmt.Sprintf("FIAP X - Falha ao processar o vídeo %s", f.OriginalName),
		Body:    body,
	}
}
