// Package domain contém as entidades e regras de negócio de identidade.
// Não depende de nenhuma outra camada nem de bibliotecas de infraestrutura.
package domain

import (
	"errors"
	"net/mail"
	"strings"
	"time"
)

// Erros de domínio.
var (
	ErrEmailTaken         = errors.New("e-mail já cadastrado")
	ErrInvalidCredentials = errors.New("e-mail ou senha inválidos")
	ErrUserNotFound       = errors.New("usuário não encontrado")
)

// ValidationError indica dados de entrada que violam as regras do domínio.
type ValidationError struct{ Msg string }

func (e ValidationError) Error() string { return e.Msg }

// User é a entidade de usuário. A senha nunca é guardada em texto puro.
type User struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// Limites das regras de cadastro.
const (
	MaxNameLength     = 120
	MaxEmailLength    = 254
	MinPasswordLength = 8
	MaxPasswordLength = 72 // limite do bcrypt
)

// NormalizeEmail padroniza o e-mail (minúsculas, sem espaços) para garantir unicidade.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Registration são os dados de cadastro já validados e normalizados.
type Registration struct {
	Name     string
	Email    string
	Password string
}

// NewRegistration valida e normaliza os dados de cadastro.
func NewRegistration(name, email, password string) (Registration, error) {
	name = strings.TrimSpace(name)
	email = NormalizeEmail(email)
	if name == "" || len(name) > MaxNameLength {
		return Registration{}, ValidationError{"nome é obrigatório (até 120 caracteres)"}
	}
	if _, err := mail.ParseAddress(email); err != nil || len(email) > MaxEmailLength {
		return Registration{}, ValidationError{"e-mail inválido"}
	}
	if len(password) < MinPasswordLength || len(password) > MaxPasswordLength {
		return Registration{}, ValidationError{"senha deve ter entre 8 e 72 caracteres"}
	}
	return Registration{Name: name, Email: email, Password: password}, nil
}
