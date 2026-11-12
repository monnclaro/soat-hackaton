# FIAP X — Sistema de Processamento de Vídeos

Hackathon POSTECH SOAT, Fase 5. Evolução do projeto base (um monólito Go que processava o vídeo dentro da requisição HTTP) para uma arquitetura de **microsserviços em Go com mensageria**. Ela processa vários vídeos em paralelo, não perde requisições em picos, tem autenticação, status por usuário e notificação de falha.

📐 **Arquitetura detalhada:** [docs/architecture.md](docs/architecture.md) · 🧭 **ADRs:** [docs/adr](docs/adr/README.md) · 📄 **Contrato da API:** [docs/openapi.yaml](docs/openapi.yaml) · 🗄️ **Script do banco:** [scripts/db/init.sql](scripts/db/init.sql)

```mermaid
flowchart LR
    U[UI / Cliente] --> GW[Gateway]
    GW --> AUTH[auth-service] --> PG[(Postgres)]
    GW --> VID[video-service] --> PG & R[(Redis)] & S3[(S3)]
    VID -- outbox --> MQ{{RabbitMQ}}
    MQ --> W[processor-worker xN] --> S3
    W -- status --> MQ --> VID
    MQ --> NOT[notification-service] --> MAIL[E-mail]
```

## Stack

| Camada | Tecnologia |
|---|---|
| Linguagem | Go 1.26 (`net/http`, `log/slog`) |
| Mensageria | RabbitMQ 4 (quorum queues, DLQ, publisher confirms) |
| Banco / cache | PostgreSQL 17 (database-per-service) + Redis 7 |
| Arquivos | Object storage S3 (SeaweedFS local, AWS S3 em produção) |
| Processamento | ffmpeg (1 frame/s, configurável) → ZIP |
| Containers | Docker, Docker Compose, Kubernetes (Kustomize, HPA, KEDA opcional) |
| Monitoramento | Prometheus + Grafana (dashboard e alertas provisionados) |
| CI/CD | GitHub Actions → GHCR → Kubernetes |

## Como executar

Pré-requisito: Docker com Compose v2.

```bash
docker compose up -d --build
```

| Serviço | URL | Credenciais |
|---|---|---|
| **UI + API (gateway)** | http://localhost:8080 | crie uma conta na tela |
| Grafana | http://localhost:3000 | admin / admin |
| Prometheus | http://localhost:9090 | — |
| RabbitMQ | http://localhost:15672 | guest / guest |
| Mailpit (e-mails enviados) | http://localhost:8025 | — |

> Se já houver um Postgres local na porta 5432, use `POSTGRES_HOST_PORT=55432 docker compose up -d --build`.

Para escalar o processamento: `docker compose up -d --scale processor-worker=4`.

### Usando a API (curl)

```bash
curl -X POST localhost:8080/auth/register -H 'Content-Type: application/json' \
  -d '{"name":"Ana","email":"ana@fiapx.com","password":"senha-segura"}'

TOKEN=$(curl -s -X POST localhost:8080/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"ana@fiapx.com","password":"senha-segura"}' | sed -E 's/.*"access_token":"([^"]+)".*/\1/')

curl -H "Authorization: Bearer $TOKEN" -F video=@meu-video.mp4 localhost:8080/api/videos   # 202 Accepted
curl -H "Authorization: Bearer $TOKEN" localhost:8080/api/videos                           # status
curl -H "Authorization: Bearer $TOKEN" -o frames.zip localhost:8080/api/videos/<id>/download
```

## Testes

```bash
go test ./...                                           # unitários
docker compose up -d --wait postgres redis rabbitmq s3
go test -tags=integration ./...                         # + integração (Postgres, Redis, RabbitMQ, S3 reais)
docker compose up -d --build && ./scripts/e2e.sh        # ponta a ponta pelo gateway
```

O E2E cadastra um usuário, envia 3 vídeos válidos em paralelo e 1 corrompido, e espera o processamento. Depois valida o ZIP, o status `FAILED` e o e-mail de falha no Mailpit.

## CI/CD ([.github/workflows/ci-cd.yml](.github/workflows/ci-cd.yml))

1. **Lint & Testes**: gofmt, go vet, golangci-lint, testes unitários + integração com `-race` e **gate de cobertura de 80%**.
2. **E2E**: sobe o sistema completo com Docker Compose e roda `scripts/e2e.sh`.
3. **Imagens** (push na `main`): build multi-stage e publicação de 5 imagens no GHCR (`:sha` e `:latest`).
4. **Deploy** (push na `main`, opcional): `kubectl apply -k deploy/k8s/base` com as imagens fixadas na SHA. Para habilitar, defina a variável de repositório `DEPLOY_ENABLED=true` e o secret `KUBE_CONFIG` (kubeconfig em base64).

## Kubernetes

```bash
kubectl apply -k deploy/k8s/local    # minikube/kind: aplicações + infraestrutura
kubectl apply -k deploy/k8s/base     # somente aplicações (infra gerenciada na nuvem)
```

## Estrutura

```
cmd/                       entrypoints dos 4 microsserviços
internal/auth              cadastro/login/JWT
internal/video             upload, status, download, outbox, cache
internal/processor         worker ffmpeg + zip
internal/notification      e-mail de falha
internal/events            contratos das mensagens
internal/platform          httpx, rabbitmq, postgres, storage, security, config, logging
internal/integration       testes de integração (build tag "integration")
web/                       UI (servida pelo gateway)
deploy/                    gateway, prometheus, grafana, seaweedfs, k8s
scripts/                   init.sql do banco e e2e.sh
docs/                      arquitetura, ADRs e OpenAPI
```

## Configuração (variáveis de ambiente)

| Variável | Serviço | Padrão |
|---|---|---|
| `JWT_SECRET` (≥ 32 caracteres) | auth, video | — (obrigatória) |
| `JWT_TTL` | auth | `24h` |
| `DATABASE_URL` | auth, video | — |
| `RABBITMQ_URL` | video, worker, notification | `amqp://guest:guest@localhost:5672/` |
| `REDIS_ADDR`, `CACHE_TTL` | video | `localhost:6379`, `30s` |
| `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `S3_USE_SSL` | video, worker | SeaweedFS local |
| `MAX_UPLOAD_BYTES` | video | 1 GiB |
| `WORKER_CONCURRENCY` | worker | `2` |
| `FRAME_FPS`, `JOB_TIMEOUT`, `MAX_ATTEMPTS` | worker | `1`, `30m`, `3` |
| `RABBITMQ_DELIVERY_LIMIT` | todos (consumidores) | `5` |
| `SMTP_ADDR`, `SMTP_FROM`, `SMTP_USERNAME`, `SMTP_PASSWORD` | notification | Mailpit |
