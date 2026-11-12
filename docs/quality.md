# Qualidade: testes, CI/CD e desenvolvimento

- [Testes](#testes)
- [CI/CD](#cicd)
- [Fluxo de desenvolvimento](#fluxo-de-desenvolvimento)

## Testes

| Tipo | O que cobre | Como rodar |
|---|---|---|
| **Unitários** | Por camada: `domain` (máquina de estados, validações, política de retentativa), `usecase` com fakes das portas (sucesso, falha definitiva/transitória, timeout, concorrência), `adapter` (handlers HTTP com `httptest`, consumidores, publisher, ffmpeg, ZIP, SMTP, bcrypt, JWT) e middlewares | `go test ./...` |
| **Arquitetura** | Regra de dependência da Clean Architecture: `domain` e `usecase` não importam adaptadores nem bibliotecas de infraestrutura ([internal/archtest](../internal/archtest/arch_test.go)) | `go test ./internal/archtest/` |
| **Integração** | Adaptadores contra infraestrutura **real**: repositórios Postgres e outbox, cache Redis, storage S3, publish/consume/requeue no RabbitMQ | ver abaixo |
| **E2E** | Fluxo completo pelo gateway: cadastro, login, 401 sem token, 3 uploads em paralelo + 1 corrompido, processamento, ZIP com frames, `409` ao baixar vídeo com falha, e-mail no Mailpit | `./scripts/e2e.sh` |

```bash
# Unitários
go test ./...

# Unitários + integração com cobertura
docker compose up -d --wait postgres redis rabbitmq s3
go test -tags=integration -coverpkg=./internal/... -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1        # total (gate do CI: 80%)
go tool cover -html=coverage.out                   # relatório navegável

# Ponta a ponta (sistema completo no ar)
docker compose up -d --build
./scripts/e2e.sh                                   # requer bash, curl e unzip
```

**Ferramentas de qualidade no CI:** `gofmt`, `go vet`, `golangci-lint`, race detector (`-race`) e gate de cobertura.

---

## CI/CD

Workflow: [.github/workflows/ci-cd.yml](../.github/workflows/ci-cd.yml).

```mermaid
flowchart LR
    PR[Pull Request / push] --> T[Lint & Testes<br/>gofmt · vet · golangci-lint<br/>unit + integração -race<br/>cobertura ≥ 80%]
    T --> E[E2E<br/>docker compose + e2e.sh]
    E -->|somente main| B[Build & push<br/>5 imagens no GHCR<br/>:sha e :latest]
    B -->|DEPLOY_ENABLED=true| D[Deploy K8s<br/>kubectl apply -k<br/>rollout status]
```

| Job | Quando | O que faz |
|---|---|---|
| **Lint & Testes** | PR e push | formatação, vet, golangci-lint, sobe Postgres/Redis/RabbitMQ/S3, testes com `-race` e integração, publica `coverage.out`, **falha abaixo de 80%** |
| **E2E** | após os testes | sobe o sistema completo (inclusive o build da UI) e executa `scripts/e2e.sh`; em caso de falha, imprime os logs |
| **Imagens** | push na `main` | build multi-stage e push de `auth-service`, `video-service`, `processor-worker`, `notification-service` e `gateway` para `ghcr.io/monnclaro/soat-hackaton/*` |
| **Deploy** | push na `main` + variável `DEPLOY_ENABLED=true` | fixa as imagens na SHA do commit, aplica `deploy/k8s/base` e aguarda o rollout |

**Para habilitar o deploy:** em *Settings → Secrets and variables → Actions*, crie a variável `DEPLOY_ENABLED=true` e o secret `KUBE_CONFIG` com o kubeconfig do cluster em base64 (`base64 -w0 ~/.kube/config`).

**Imagens:** multi-stage. Os serviços usam `distroless/static` (sem shell, usuário não-root UID 65532); o worker usa `alpine` com ffmpeg e usuário não-root; o gateway compila a UI num estágio Node e serve com nginx.

---

## Fluxo de desenvolvimento

Para rodar os serviços na sua máquina, veja a [opção B de running.md](running.md#opção-b--serviços-go-na-mão--infraestrutura-no-docker). Comandos usados antes de abrir um PR (os mesmos do CI):

```bash
gofmt -l .                                   # lista arquivos não formatados (gofmt -w . corrige)
go vet ./...                                 # análise estática
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
go test ./...                                # testes unitários
docker compose up -d --wait postgres redis rabbitmq s3
go test -tags=integration ./...              # unitários + integração
go build ./...                               # compila os 4 serviços
```

Para compilar binários: `go build -o bin/ ./cmd/...` (gera `bin/auth-service`, `bin/video-service` etc.).

**UI (`web/`)**: com os serviços rodando, edite `web/src` e recompile automaticamente:

```bash
cd web
npm ci
npm run watch        # recompila SCSS/HTML/JS para web/dist a cada alteração
```

No Compose, para ver as alterações, reconstrua o gateway: `docker compose up -d --build gateway`.
