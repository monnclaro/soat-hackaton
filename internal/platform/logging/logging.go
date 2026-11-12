// Package logging configura logs estruturados em JSON (coletáveis por qualquer stack de logs).
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New cria um logger JSON com o atributo "service" e define-o como padrão.
func New(service, level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})).With("service", service)
	slog.SetDefault(logger)
	return logger
}
