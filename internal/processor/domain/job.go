// Package domain contém as regras do processamento de vídeos: o job, os possíveis
// resultados e a política de retentativa. Não depende de outras camadas.
package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Job é o pedido de processamento de um vídeo.
type Job struct {
	VideoID      string
	UserID       string
	UserEmail    string
	UserName     string
	OriginalName string
	StorageKey   string // onde está o vídeo original
}

// ZipKey define onde o ZIP de frames será guardado. É determinística: reprocessar o
// mesmo vídeo sobrescreve o mesmo arquivo (idempotência).
func (j Job) ZipKey() string { return "outputs/" + j.UserID + "/" + j.VideoID + ".zip" }

// InputExtension é a extensão do arquivo original (ajuda o ffmpeg a detectar o formato).
func (j Job) InputExtension() string { return strings.ToLower(filepath.Ext(j.OriginalName)) }

// Status publicado durante o processamento.
type Status string

// Estados publicados pelo worker.
const (
	StatusProcessing Status = "PROCESSING"
	StatusCompleted  Status = "COMPLETED"
	StatusFailed     Status = "FAILED"
)

// StatusChange é o resultado informado ao restante do sistema.
type StatusChange struct {
	Status     Status
	ZipKey     string
	FrameCount int
	Error      string
}

// ProcessingError é uma falha definitiva do vídeo (arquivo corrompido, sem frames, tempo esgotado).
// Tentar de novo não resolve: o vídeo deve ser marcado como FAILED e o usuário notificado.
type ProcessingError struct{ Reason string }

func (e *ProcessingError) Error() string { return e.Reason }

// ErrNoFrames indica que o vídeo não gerou nenhum frame.
var ErrNoFrames = &ProcessingError{Reason: "nenhum frame foi extraído do vídeo"}

// ErrTimeout indica que o processamento excedeu o tempo limite.
var ErrTimeout = &ProcessingError{Reason: "tempo limite de processamento excedido"}

// Outcome é o desfecho de uma tentativa de processamento.
type Outcome string

// Desfechos possíveis.
const (
	OutcomeCompleted Outcome = "completed"
	OutcomeFailed    Outcome = "failed" // definitivo: marca FAILED e notifica
	OutcomeRetry     Outcome = "retry"  // transitório: a mensagem volta para a fila
)

// RetryPolicy decide entre tentar de novo ou desistir.
type RetryPolicy struct{ MaxAttempts int }

// Decide classifica um erro: falha definitiva ou última tentativa → Failed; senão → Retry.
func (p RetryPolicy) Decide(err error, attempt int) Outcome {
	var perr *ProcessingError
	if errors.As(err, &perr) || attempt >= p.MaxAttempts {
		return OutcomeFailed
	}
	return OutcomeRetry
}

// FailureReason gera a mensagem mostrada ao usuário.
func (p RetryPolicy) FailureReason(err error, attempt int) string {
	var perr *ProcessingError
	if errors.As(err, &perr) {
		return perr.Reason
	}
	return fmt.Sprintf("falha após %d tentativas: %s", attempt, err.Error())
}
