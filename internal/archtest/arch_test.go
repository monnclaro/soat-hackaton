// Package archtest verifica automaticamente a regra de dependência da Clean Architecture:
// as camadas internas (domain e usecase) não podem depender das externas (adapter, platform)
// nem de bibliotecas de infraestrutura (banco, broker, HTTP, criptografia, métricas...).
package archtest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/monnclaro/soat-hackaton/"

var services = []string{"auth", "video", "processor", "notification"}

// Importações proibidas em cada camada interna (prefixos).
var forbidden = map[string][]string{
	"domain": {
		module + "internal/", // o domínio não depende de nenhum outro pacote do projeto
		"net/http", "database/sql", "os/exec",
		"github.com/", "golang.org/x/",
	},
	"usecase": {
		module + "internal/platform/",
		"net/http", "net/smtp", "database/sql", "os/exec",
		"github.com/jackc/", "github.com/redis/", "github.com/rabbitmq/", "github.com/minio/",
		"github.com/prometheus/", "github.com/golang-jwt/", "golang.org/x/crypto",
	},
}

func TestDependencyRule(t *testing.T) {
	root := filepath.Join("..")
	checked := 0
	for _, svc := range services {
		for layer, rules := range forbidden {
			dir := filepath.Join(root, svc, layer)
			files, err := filepath.Glob(filepath.Join(dir, "*.go"))
			if err != nil {
				t.Fatal(err)
			}
			if len(files) == 0 {
				t.Errorf("%s/%s não existe ou está vazio", svc, layer)
				continue
			}
			for _, file := range files {
				if strings.HasSuffix(file, "_test.go") {
					continue
				}
				checked++
				for _, imp := range imports(t, file) {
					if bad := violates(imp, svc, layer, rules); bad {
						t.Errorf("%s importa %q: a camada %s não pode depender disso", rel(file), imp, layer)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("nenhum arquivo verificado")
	}
}

func violates(imp, svc, layer string, rules []string) bool {
	// usecase pode depender do próprio domain e do contrato de eventos.
	if layer == "usecase" && (imp == module+"internal/"+svc+"/domain" || imp == module+"internal/events") {
		return false
	}
	// usecase nunca depende de adapters (de nenhum serviço).
	if layer == "usecase" && strings.Contains(imp, "/adapter") {
		return true
	}
	// Bibliotecas utilitárias sem infraestrutura são permitidas.
	if imp == "github.com/google/uuid" {
		return false
	}
	for _, prefix := range rules {
		if strings.HasPrefix(imp, prefix) {
			return true
		}
	}
	return false
}

func imports(t *testing.T, file string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, spec := range f.Imports {
		p, _ := strconv.Unquote(spec.Path.Value)
		out = append(out, p)
	}
	return out
}

func rel(path string) string {
	wd, _ := os.Getwd()
	if r, err := filepath.Rel(filepath.Join(wd, ".."), path); err == nil {
		return "internal/" + filepath.ToSlash(r)
	}
	return path
}

func TestViolatesSelfCheck(t *testing.T) {
	cases := []struct {
		imp, svc, layer string
		want            bool
	}{
		{module + "internal/video/domain", "video", "usecase", false},
		{module + "internal/events", "video", "usecase", false},
		{module + "internal/video/adapter/postgres", "video", "usecase", true},
		{"github.com/jackc/pgx/v5", "video", "usecase", true},
		{"github.com/google/uuid", "auth", "usecase", false},
		{module + "internal/events", "video", "domain", true},
		{"net/http", "video", "domain", true},
		{"time", "video", "domain", false},
	}
	for _, c := range cases {
		if got := violates(c.imp, c.svc, c.layer, forbidden[c.layer]); got != c.want {
			t.Errorf("violates(%q, %s/%s) = %v, want %v", c.imp, c.svc, c.layer, got, c.want)
		}
	}
}
