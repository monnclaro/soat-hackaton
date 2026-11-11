// Package events define os contratos das mensagens trocadas entre os serviços via RabbitMQ.
package events

import "time"

// Status de processamento de um vídeo.
const (
	StatusPending    = "PENDING"
	StatusProcessing = "PROCESSING"
	StatusCompleted  = "COMPLETED"
	StatusFailed     = "FAILED"
)

// RoutingKeyVideoUploaded é publicada pelo video-service quando um upload é aceito.
const RoutingKeyVideoUploaded = "video.uploaded"

// StatusRoutingKey retorna a routing key do evento de mudança de status (ex.: video.status.failed).
func StatusRoutingKey(status string) string {
	switch status {
	case StatusProcessing:
		return "video.status.processing"
	case StatusCompleted:
		return "video.status.completed"
	case StatusFailed:
		return "video.status.failed"
	default:
		return "video.status.unknown"
	}
}

// VideoUploaded é o comando de processamento enviado ao processor-worker.
type VideoUploaded struct {
	VideoID      string    `json:"video_id"`
	UserID       string    `json:"user_id"`
	UserEmail    string    `json:"user_email"`
	UserName     string    `json:"user_name"`
	OriginalName string    `json:"original_name"`
	StorageKey   string    `json:"storage_key"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

// VideoStatusChanged é publicado pelo processor-worker a cada transição de status.
type VideoStatusChanged struct {
	VideoID      string    `json:"video_id"`
	UserID       string    `json:"user_id"`
	UserEmail    string    `json:"user_email"`
	UserName     string    `json:"user_name"`
	OriginalName string    `json:"original_name"`
	Status       string    `json:"status"`
	ZipKey       string    `json:"zip_key,omitempty"`
	FrameCount   int       `json:"frame_count,omitempty"`
	Error        string    `json:"error,omitempty"`
	OccurredAt   time.Time `json:"occurred_at"`
}
