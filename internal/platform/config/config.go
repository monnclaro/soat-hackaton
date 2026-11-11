// Package config lê configuração de variáveis de ambiente (12-factor).
package config

import (
	"os"
	"strconv"
	"time"
)

// String retorna a variável de ambiente ou o valor padrão.
func String(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// Int retorna a variável de ambiente convertida para int ou o padrão.
func Int(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Int64 retorna a variável de ambiente convertida para int64 ou o padrão.
func Int64(key string, def int64) int64 {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

// Duration retorna a variável de ambiente como time.Duration (ex.: "30s") ou o padrão.
func Duration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
