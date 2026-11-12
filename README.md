# FIAP X — Sistema de Processamento de Vídeos

> **Hackathon POSTECH SOAT — Fase 5.** O usuário envia vídeos, o sistema extrai os frames (1 por segundo) e devolve um **.zip com as imagens**. É uma evolução do projeto base, reescrito como **microsserviços em Go com mensageria**: processa vários vídeos em paralelo, não perde requisições em picos, exige login e mostra o status de cada vídeo. Quando o processamento falha, o usuário recebe um e-mail.

[![CI-CD](https://github.com/monnclaro/soat-hackaton/actions/workflows/ci-cd.yml/badge.svg)](https://github.com/monnclaro/soat-hackaton/actions/workflows/ci-cd.yml)

| Documento | Conteúdo |
|---|---|
| 📐 [docs/architecture.md](docs/architecture.md) | Arquitetura, diagramas de sequência, máquina de estados, modelo de dados |
| 🧭 [docs/adr/](docs/adr/README.md) | 12 Architecture Decision Records (contexto, alternativas e consequências) |
| 📄 [docs/openapi.yaml](docs/openapi.yaml) | Contrato OpenAPI 3 da API |
| 🗄️ [scripts/db/init.sql](scripts/db/init.sql) | Script de criação dos bancos de dados |
| 🧪 [scripts/e2e.sh](scripts/e2e.sh) | Teste ponta a ponta do fluxo completo |

---

## Sumário

1. [O problema e a solução](#1-o-problema-e-a-solução)
2. [Requisitos atendidos](#2-requisitos-atendidos)
3. [Arquitetura](#3-arquitetura)
4. [Microsserviços](#4-microsserviços)
5. [Stack tecnológica](#5-stack-tecnológica)
6. [Como executar (Docker Compose)](#6-como-executar-docker-compose)
7. [Usando o sistema pela interface web](#7-usando-o-sistema-pela-interface-web)
8. [Referência da API](#8-referência-da-api)
9. [Ciclo de vida de um vídeo](#9-ciclo-de-vida-de-um-vídeo)
10. [Mensageria](#10-mensageria)
11. [Banco de dados](#11-banco-de-dados)
12. [Resiliência: o que acontece quando algo falha](#12-resiliência-o-que-acontece-quando-algo-falha)
13. [Escalabilidade](#13-escalabilidade)
14. [Observabilidade](#14-observabilidade)
15. [Testes e qualidade](#15-testes-e-qualidade)
16. [CI/CD](#16-cicd)
17. [Kubernetes](#17-kubernetes)
18. [Configuração (variáveis de ambiente)](#18-configuração-variáveis-de-ambiente)
19. [Estrutura do repositório](#19-estrutura-do-repositório)
20. [Desenvolvimento local sem Docker](#20-desenvolvimento-local-sem-docker)
21. [Solução de problemas](#21-solução-de-problemas)
22. [Roteiro sugerido para o vídeo de apresentação](#22-roteiro-sugerido-para-o-vídeo-de-apresentação)

---

## 1. O problema e a solução

O projeto base apresentado aos investidores (`projeto-fiapx`) era um único arquivo `main.go` que:

| Problema no projeto base | Consequência | Como foi resolvido |
|---|---|---|
| Rodava o ffmpeg **dentro da requisição HTTP** | O cliente esperava minutos; poucos uploads simultâneos derrubavam o servidor | Upload responde `202` na hora e o processamento é assíncrono via RabbitMQ |
| Gravava vídeos e ZIPs no **disco local** | Impossível ter mais de uma instância; arquivos somem ao recriar o container | Object storage S3 compartilhado entre API e workers |
| **Sem autenticação** | Qualquer um via e baixava os arquivos de todos | Cadastro/login, JWT, cada usuário só acessa os próprios vídeos |
| **Sem banco de dados** | Nenhum histórico; a listagem era um `ls` na pasta | PostgreSQL com status de cada vídeo |
| Erro só aparecia na resposta HTTP | Se o usuário fechasse a página, nunca saberia | Notificação por e-mail em caso de falha |
| Sem testes, sem CI, Dockerfile com `go run` | Nenhuma garantia de qualidade | Testes unitários/integração/E2E, gate de cobertura, pipeline completo, imagens mínimas não-root |

---

## 2. Requisitos atendidos

### Funcionais

| Requisito do enunciado | Como é atendido | Onde ver |
|---|---|---|
| Processar mais de um vídeo ao mesmo tempo | Cada worker processa `WORKER_CONCURRENCY` vídeos em paralelo e várias réplicas competem pela mesma fila | [processor](internal/processor), [ADR 0009](docs/adr/0009-escalabilidade-do-worker.md) |
| Em picos, não perder requisições | Transactional outbox, publisher confirms, quorum queues duráveis, ack só ao final, reentrega e DLQ | [ADR 0002](docs/adr/0002-processamento-assincrono-com-rabbitmq.md), [ADR 0003](docs/adr/0003-transactional-outbox.md) |
| Protegido por usuário e senha | auth-service (bcrypt + JWT); todas as rotas `/api` exigem token | [auth](internal/auth), [ADR 0006](docs/adr/0006-jwt-simetrico-compartilhado.md) |
| Listagem de status dos vídeos do usuário | `GET /api/videos` com cache Redis | [video](internal/video), [ADR 0007](docs/adr/0007-cache-aside-redis.md) |
| Notificar o usuário em caso de erro | Evento `video.status.failed` → notification-service → e-mail | [notification](internal/notification), [ADR 0010](docs/adr/0010-notificacao-por-eventos.md) |

### Técnicos

| Requisito | Como é atendido |
|---|---|
| Persistir os dados | PostgreSQL (um banco por serviço), S3 para arquivos, Redis com AOF |
| Arquitetura escalável | Serviços stateless, storage compartilhado, mensageria, HPA (e KEDA opcional) no Kubernetes |
| Versionado no GitHub | Este repositório |
| Testes que garantam a qualidade | Unitários + integração com infra real + E2E; gate de **80% de cobertura** (atual: ~86%) |
| CI/CD | GitHub Actions: lint → testes → E2E → imagens no GHCR → deploy Kubernetes |

### Stack recomendada × utilizada

| Recomendado | Utilizado |
|---|---|
| Docker + Kubernetes ou Docker Compose | **Ambos**: Compose para local/CI, Kustomize para K8s |
| RabbitMQ, Kafka ou similar | **RabbitMQ 4** |
| PostgreSQL + Redis | **PostgreSQL 17 + Redis 7** |
| Prometheus + Grafana, ELK... | **Prometheus + Grafana** (dashboard e alertas como código) |
| GitHub Actions | **GitHub Actions** |

---

## 3. Arquitetura

```mermaid
flowchart LR
    U[Usuário / Navegador] -->|HTTP :8080| GW[Gateway nginx<br/>UI + reverse proxy]
    GW -->|/auth/*| AUTH[auth-service :8081]
    GW -->|/api/*| VID[video-service :8082]

    AUTH --> PGA[(PostgreSQL<br/>fiapx_auth)]
    VID --> PGV[(PostgreSQL<br/>fiapx_videos + outbox)]
    VID --> R[(Redis<br/>cache)]
    VID -->|vídeo original| S3[(Object storage S3)]

    VID -->|outbox relay<br/>video.uploaded| MQ{{RabbitMQ<br/>fiapx.events}}
    MQ --> W1[processor-worker #1]
    MQ --> W2[processor-worker #N]
    W1 & W2 -->|baixa vídeo / grava ZIP| S3
    W1 & W2 -->|video.status.*| MQ
    MQ -->|status| VID
    MQ -->|falhas| NOT[notification-service :8084]
    NOT -->|SMTP| MAIL[E-mail]

    PROM[Prometheus] -.->|/metrics| AUTH & VID & W1 & W2 & NOT & MQ
    GRAF[Grafana] -.-> PROM
```

**Ideia central:** a API **aceita** o vídeo rápido e de forma durável; o **processamento** acontece depois, em workers que escalam de forma independente. Entre os dois há uma fila, que absorve os picos.

Para os diagramas de sequência (upload, processamento, falha), a máquina de estados e o modelo de dados, veja [docs/architecture.md](docs/architecture.md).

---

## 4. Microsserviços

### gateway (nginx)
- Ponto único de entrada na porta **8080**; serve a **UI web** (`web/index.html`).
- Roteia `/auth/*` → auth-service e `/api/*` → video-service.
- Upload em **streaming** (`proxy_request_buffering off`), limite de 1 GiB.
- Re-resolve o DNS dos serviços a cada 10s, então réplicas novas entram no balanceamento.

### auth-service — [internal/auth](internal/auth)
- `POST /auth/register`, `POST /auth/login`, `GET /auth/me`.
- Senhas com **bcrypt**; e-mail normalizado (minúsculas, sem espaços) e único.
- Emite **JWT HS256** com `sub`, `email`, `name` e expiração (24h por padrão).
- Banco: `fiapx_auth.users`.

### video-service — [internal/video](internal/video)
- `POST /api/videos`: recebe o multipart e faz **stream direto para o S3**, sem guardar em disco ou memória. Grava vídeo + mensagem de outbox na **mesma transação** e responde `202`.
- `GET /api/videos`: lista os vídeos do usuário (**cache Redis**, TTL 30s, invalidado a cada mudança).
- `GET /api/videos/{id}` e `GET /api/videos/{id}/download`: stream do ZIP.
- **Outbox relay**: publica as mensagens pendentes no RabbitMQ com confirmação do broker.
- **Consumidor** `video.status.*`: atualiza o status respeitando a máquina de estados (idempotente).
- Banco: `fiapx_videos.videos` e `fiapx_videos.outbox`.

### processor-worker — [internal/processor](internal/processor)
- Consome `processor.video-uploaded` com `WORKER_CONCURRENCY` goroutines.
- Para cada vídeo: publica `PROCESSING`, baixa do S3 para um diretório temporário, roda `ffmpeg -vf fps=1` (PNG), compacta, envia o ZIP ao S3 e publica `COMPLETED` (ou `FAILED`).
- Diferencia **falha definitiva** (vídeo inválido, sem frames, timeout), que marca `FAILED`, de **falha transitória** (S3 ou broker), que gera retentativa com backoff.
- Graceful shutdown: termina os vídeos em andamento antes de sair.

### notification-service — [internal/notification](internal/notification)
- Consome `notification.video-failed` e envia um e-mail com o nome do arquivo, o motivo e o ID do vídeo.
- SMTP configurável: Mailpit no ambiente local, Amazon SES ou outro provedor em produção.

---

## 5. Stack tecnológica

| Componente | Tecnologia | Por quê |
|---|---|---|
| Linguagem | **Go 1.26** (stdlib `net/http`, `log/slog`) | Mesma do projeto base; ótima para I/O concorrente; binários pequenos |
| Mensageria | **RabbitMQ 4.1** (quorum queues) | Work queue com ack manual, DLX, delivery-limit |
| Banco | **PostgreSQL 17** | Transações (outbox), confiável, recomendado |
| Cache | **Redis 7** | Alivia o polling da listagem |
| Arquivos | **S3** (SeaweedFS local / AWS S3) | Compartilhado entre API e workers ([ADR 0004](docs/adr/0004-object-storage-s3.md)) |
| Processamento | **ffmpeg** | Mesmo motor do projeto base |
| E-mail (local) | **Mailpit** | Captura os e-mails com UI web |
| Gateway | **nginx 1.27** | Proxy reverso + arquivos estáticos |
| Métricas | **Prometheus 3 + Grafana 12** | Recomendado; provisionado como código |
| Containers | **Docker, Compose, Kubernetes (Kustomize)** | Local, CI e produção com os mesmos artefatos |
| CI/CD | **GitHub Actions + GHCR** | Recomendado |

Bibliotecas Go: `pgx/v5`, `amqp091-go`, `go-redis/v9`, `minio-go/v7` (cliente S3 genérico), `golang-jwt/v5`, `x/crypto/bcrypt`, `prometheus/client_golang`, `google/uuid`.

---

## 6. Como executar (Docker Compose)

### Pré-requisitos
- **Docker Desktop** (ou Docker Engine) com **Compose v2**.
- Portas livres: 8080, 3000, 9090, 15672, 8025, 5432, 6379, 5672, 8333.

### Subir tudo

```bash
git clone https://github.com/monnclaro/soat-hackaton.git
cd soat-hackaton
docker compose up -d --build
```

Na primeira vez, as imagens levam alguns minutos para baixar e compilar. Para acompanhar:

```bash
docker compose ps          # todos devem estar "Up" / "healthy"
docker compose logs -f video-service processor-worker
```

### Endereços

| Serviço | URL | Credenciais |
|---|---|---|
| **Aplicação (UI + API)** | http://localhost:8080 | crie uma conta na tela |
| Grafana (dashboard) | http://localhost:3000 | `admin` / `admin` |
| Prometheus | http://localhost:9090 | — |
| RabbitMQ Management | http://localhost:15672 | `guest` / `guest` |
| Mailpit (e-mails enviados) | http://localhost:8025 | — |
| S3 (SeaweedFS) | http://localhost:8333 | `fiapx-access-key` / `fiapx-secret-key` |
| PostgreSQL | `localhost:5432` | `fiapx` / `fiapx` |

### Comandos úteis

```bash
docker compose up -d --scale processor-worker=4   # 4 workers × 2 = 8 vídeos simultâneos
docker compose logs -f processor-worker           # acompanhar o processamento
docker compose down                                # parar (mantém os dados)
docker compose down -v                             # parar e apagar todos os dados
```

> **Já tem um Postgres local na porta 5432?** Suba com `POSTGRES_HOST_PORT=55432 docker compose up -d --build`. Os serviços continuam usando a porta interna 5432; só a porta exposta no host muda.

---

## 7. Usando o sistema pela interface web

1. Acesse **http://localhost:8080**.
2. Em **Criar conta**, informe nome, e-mail e senha (mínimo de 8 caracteres) e clique em **Cadastrar**.
3. Em **Entrar**, faça login.
4. Em **Enviar vídeos**, selecione **um ou vários** arquivos (mp4, avi, mov, mkv, webm, wmv, flv, m4v) e clique em **Enviar**. Os uploads seguem em paralelo.
5. A tabela **Meus vídeos** se atualiza sozinha a cada 4s:
   - **Na fila** (`PENDING`): aceito, aguardando um worker;
   - **Processando** (`PROCESSING`);
   - **Concluído** (`COMPLETED`): aparece o botão **Baixar ZIP**;
   - **Falhou** (`FAILED`): mostra o motivo, e um e-mail é enviado.
6. Para ver o e-mail de falha, envie um arquivo inválido renomeado para `.mp4` e abra o **Mailpit** em http://localhost:8025.

---

## 8. Referência da API

Base: `http://localhost:8080`. Contrato completo em [docs/openapi.yaml](docs/openapi.yaml). Erros sempre seguem o formato `{"error": "mensagem"}`.

### Autenticação

#### `POST /auth/register`
```bash
curl -X POST localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"Ana","email":"ana@fiapx.com","password":"senha-segura"}'
```
```json
201 Created
{ "id": "0b6c…", "name": "Ana", "email": "ana@fiapx.com", "created_at": "2026-11-11T22:00:00Z" }
```
Erros: `400` JSON inválido · `409` e-mail já cadastrado · `422` validação (nome vazio, e-mail inválido, senha fora de 8–72 caracteres).

#### `POST /auth/login`
```bash
curl -X POST localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"ana@fiapx.com","password":"senha-segura"}'
```
```json
200 OK
{ "access_token": "eyJhbGciOi…", "token_type": "Bearer", "expires_at": "2026-11-12T22:00:00Z",
  "user": { "id": "0b6c…", "name": "Ana", "email": "ana@fiapx.com", "created_at": "…" } }
```
Erros: `401` e-mail ou senha inválidos.

#### `GET /auth/me` (autenticado)
Retorna o usuário do token.

### Vídeos (todas exigem `Authorization: Bearer <token>`)

```bash
TOKEN=$(curl -s -X POST localhost:8080/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"ana@fiapx.com","password":"senha-segura"}' | sed -E 's/.*"access_token":"([^"]+)".*/\1/')
```

#### `POST /api/videos`: enviar vídeo
```bash
curl -H "Authorization: Bearer $TOKEN" -F video=@aula.mp4 localhost:8080/api/videos
```
```json
202 Accepted
Location: /api/videos/7f3e…
{ "id": "7f3e…", "original_name": "aula.mp4", "status": "PENDING", "size_bytes": 1048576,
  "frame_count": 0, "created_at": "…", "updated_at": "…" }
```
Erros: `400` não é multipart ou sem o campo `video` · `401` sem token · `413` maior que `MAX_UPLOAD_BYTES` · `422` extensão não suportada ou arquivo vazio.

#### `GET /api/videos`: listar status
```json
200 OK
{ "videos": [
  { "id": "7f3e…", "original_name": "aula.mp4", "status": "COMPLETED", "frame_count": 120,
    "download_url": "/api/videos/7f3e…/download", … },
  { "id": "a1b2…", "original_name": "corrompido.mp4", "status": "FAILED",
    "error": "ffmpeg não conseguiu processar o vídeo: …", … }
] }
```

#### `GET /api/videos/{id}`: detalhe
`404` se não existir ou pertencer a outro usuário.

#### `GET /api/videos/{id}/download`: baixar o ZIP
```bash
curl -H "Authorization: Bearer $TOKEN" -o frames.zip localhost:8080/api/videos/<id>/download
```
`200` com `Content-Type: application/zip` e `Content-Disposition: attachment; filename="aula_frames.zip"` · `409` se ainda não foi concluído ou falhou · `404` se não existir.

### Endpoints operacionais (em cada serviço, sem passar pelo gateway)

| Rota | Uso |
|---|---|
| `GET /healthz` | Liveness: o processo está vivo |
| `GET /readyz` | Readiness: dependências (Postgres, Redis, S3, RabbitMQ) acessíveis |
| `GET /metrics` | Métricas Prometheus |

---

## 9. Ciclo de vida de um vídeo

```mermaid
stateDiagram-v2
    [*] --> PENDING: upload aceito (202)
    PENDING --> PROCESSING: worker iniciou
    PROCESSING --> COMPLETED: ZIP gerado
    PROCESSING --> FAILED: vídeo inválido / timeout / tentativas esgotadas
    PENDING --> FAILED
    COMPLETED --> [*]
    FAILED --> [*]
```

1. **Upload**: o vídeo vai para o S3 em `videos/<user>/<id>.<ext>`; a linha em `videos` (PENDING) e a mensagem em `outbox` são gravadas na mesma transação.
2. **Outbox relay** (a cada 1s, ou imediatamente sob carga): publica `video.uploaded` e marca a mensagem como publicada.
3. **Worker**: publica `PROCESSING`, extrai os frames, gera o ZIP em `outputs/<user>/<id>.zip` e publica `COMPLETED` com `frame_count`.
4. **video-service**: consome o evento, atualiza o banco e invalida o cache.
5. **Em caso de falha**: o worker publica `FAILED` com o motivo, o video-service grava o erro e o notification-service envia o e-mail.

Os estados `COMPLETED` e `FAILED` são **terminais**: eventos duplicados ou atrasados são ignorados.

---

## 10. Mensageria

Exchange **`fiapx.events`** (topic). Todas as filas são **quorum queues** com dead-letter para `fiapx.dlx` e `x-delivery-limit` configurável.

| Fila | Routing key | Produtor | Consumidor |
|---|---|---|---|
| `processor.video-uploaded` | `video.uploaded` | video-service (outbox) | processor-worker |
| `video-service.status-changed` | `video.status.*` | processor-worker | video-service |
| `notification.video-failed` | `video.status.failed` | processor-worker | notification-service |
| `fiapx.dead-letter` | `#` (via `fiapx.dlx`) | RabbitMQ | operação (análise manual) |

Contratos em [internal/events/events.go](internal/events/events.go). Exemplo de `video.status.failed`:

```json
{
  "video_id": "a1b2…", "user_id": "0b6c…", "user_email": "ana@fiapx.com", "user_name": "Ana",
  "original_name": "corrompido.mp4", "status": "FAILED",
  "error": "ffmpeg não conseguiu processar o vídeo: …", "occurred_at": "2026-11-11T22:03:55Z"
}
```

Você pode inspecionar as filas, as mensagens e a DLQ no **RabbitMQ Management** (http://localhost:15672 → *Queues*).

---

## 11. Banco de dados

**Database-per-service** ([ADR 0005](docs/adr/0005-database-per-service.md)):

| Banco | Dono | Tabelas |
|---|---|---|
| `fiapx_auth` | auth-service | `users` (id, name, email único, password_hash bcrypt, created_at) |
| `fiapx_videos` | video-service | `videos` (status, chaves no S3, tamanho, frames, erro, datas); `outbox` (mensagens a publicar) |

- **Script de criação:** [scripts/db/init.sql](scripts/db/init.sql). O container do Postgres o executa automaticamente na primeira subida.
- **Migrações automáticas:** cada serviço também aplica o seu DDL idempotente (`internal/*/migrations.sql`) ao iniciar, com `pg_advisory_lock` para várias réplicas. Isso permite usar um banco gerenciado (RDS) sem rodar o script manualmente.

Para consultar:
```bash
docker compose exec postgres psql -U fiapx -d fiapx_videos \
  -c "SELECT original_name, status, frame_count, error_message FROM videos ORDER BY created_at DESC LIMIT 10;"
```

---

## 12. Resiliência: o que acontece quando algo falha

| Cenário | Comportamento |
|---|---|
| **Pico de uploads** | A API só grava e responde `202`; os vídeos se acumulam na fila e os workers drenam no ritmo deles. Nada é rejeitado por falta de capacidade de processamento. |
| **RabbitMQ fora do ar no upload** | O upload continua funcionando: a mensagem fica no `outbox` e o relay publica quando o broker voltar. |
| **Worker morre no meio de um vídeo** | A mensagem não recebeu ack, então o RabbitMQ a reentrega a outra réplica. |
| **Deploy / scale-down do worker** | SIGTERM: o worker para de pegar vídeos novos e termina os atuais (até 10 min no K8s). |
| **Vídeo corrompido** | ffmpeg falha, o status vira `FAILED` com o motivo e o usuário recebe e-mail. Não há retentativa, porque não adiantaria. |
| **S3 instável durante o processamento** | Retentativa com backoff (1s, 4s, 9s...); após `MAX_ATTEMPTS`, o status vira `FAILED`. Nunca fica preso em `PROCESSING`. |
| **SMTP fora do ar** | E-mail reentregue com backoff; após o limite, vai para a DLQ. O processamento não é afetado. |
| **Mensagem malformada** | Vai direto para a DLQ (alerta no Prometheus). |
| **Mensagem duplicada** | Consumidores idempotentes: a máquina de estados ignora duplicatas e o ZIP tem chave determinística. |
| **Redis fora do ar** | A listagem ignora o cache e lê do Postgres. |
| **Conexão com o RabbitMQ cai** | O serviço encerra (crash-only) e o Docker/Kubernetes o reinicia. |

Detalhes em [ADR 0008](docs/adr/0008-retentativas-dlq-e-idempotencia.md).

---

## 13. Escalabilidade

| Componente | Como escala |
|---|---|
| processor-worker | `WORKER_CONCURRENCY` por pod × número de réplicas. Compose: `--scale processor-worker=N`. K8s: HPA por CPU (2–10) ou KEDA pela fila |
| video-service | Stateless; várias réplicas (HPA 2–6). O outbox relay usa `SKIP LOCKED` e roda em todas sem duplicar |
| auth-service | Stateless; JWT validado localmente (HPA 2–5) |
| gateway | Stateless; 2 réplicas no K8s |
| Postgres/Redis/RabbitMQ/S3 | Em produção, serviços gerenciados (RDS, ElastiCache, Amazon MQ, S3) |

**Demonstração de paralelismo:** envie vários vídeos pela UI e rode `docker compose logs -f processor-worker`. Cada réplica (`processor-worker-1`, `-2`) processa vídeos diferentes ao mesmo tempo. O painel *Vídeos em processamento agora* do Grafana mostra o total.

---

## 14. Observabilidade

### Grafana: http://localhost:3000 (admin/admin)
Dashboard **FIAP X → FIAP X - Processamento de Vídeos**, provisionado automaticamente:
- vídeos em processamento agora, fila aguardando, mensagens na DLQ, réplicas do worker ativas;
- uploads × processamentos por minuto (aceitos, concluídos, falhos, retentativas);
- profundidade das filas (ready/unacked);
- duração do processamento (p50/p95);
- requisições HTTP por rota/status e latência p95;
- notificações enviadas.

### Métricas principais

| Métrica | Serviço |
|---|---|
| `http_requests_total{route,method,status}`, `http_request_duration_seconds` | todos |
| `fiapx_video_uploads_total{result}` | video-service |
| `fiapx_video_status_updates_total{status,applied}` | video-service |
| `fiapx_processor_jobs_total{result}` | processor-worker |
| `fiapx_processor_job_duration_seconds`, `fiapx_processor_jobs_in_flight`, `fiapx_processor_frames_extracted_total` | processor-worker |
| `fiapx_notifications_total{result}` | notification-service |
| `rabbitmq_queue_messages_ready`, `rabbitmq_queue_messages_unacked` | RabbitMQ |

### Alertas ([deploy/prometheus/alerts.yml](deploy/prometheus/alerts.yml))
`VideoQueueBacklog` (fila acima de 50 por 5 min), `DeadLetterMessages` (DLQ maior que 0), `HighProcessingFailureRate` (mais de 20% de falhas), `ServiceDown`. Veja em http://localhost:9090/alerts.

### Logs
JSON estruturado em todos os serviços (`service`, `video_id`, `attempt`, `message_id`, `duration_ms`):
```bash
docker compose logs -f processor-worker | grep video_id
```

---

## 15. Testes e qualidade

| Tipo | O que cobre | Como rodar |
|---|---|---|
| **Unitários** | Regras de domínio (máquina de estados, validações), casos de uso com fakes, handlers HTTP (`httptest`), worker (sucesso, falha definitiva/transitória, timeout, concorrência), notificação, JWT, middlewares | `go test ./...` |
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

## 16. CI/CD

Workflow: [.github/workflows/ci-cd.yml](.github/workflows/ci-cd.yml).

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
| **E2E** | após os testes | sobe o sistema completo e executa `scripts/e2e.sh`; em caso de falha, imprime os logs |
| **Imagens** | push na `main` | build multi-stage e push de `auth-service`, `video-service`, `processor-worker`, `notification-service` e `gateway` para `ghcr.io/monnclaro/soat-hackaton/*` |
| **Deploy** | push na `main` + variável `DEPLOY_ENABLED=true` | fixa as imagens na SHA do commit, aplica `deploy/k8s/base` e aguarda o rollout |

**Para habilitar o deploy:** em *Settings → Secrets and variables → Actions*, crie a variável `DEPLOY_ENABLED=true` e o secret `KUBE_CONFIG` com o kubeconfig do cluster em base64 (`base64 -w0 ~/.kube/config`).

**Imagens:** multi-stage. Os serviços usam `distroless/static` (sem shell, usuário não-root UID 65532); o worker usa `alpine` com ffmpeg e usuário não-root.

---

## 17. Kubernetes

Manifests com Kustomize em [deploy/k8s](deploy/k8s):

| Pasta | Conteúdo |
|---|---|
| `base/` | Namespace `fiapx`, ConfigMap/Secret, Deployments + Services + HPAs dos 4 serviços e do gateway (Service `LoadBalancer`) |
| `local/` | `base` + infraestrutura em pods (Postgres, Redis, RabbitMQ, SeaweedFS, Mailpit), para minikube/kind |
| `optional/` | `ScaledObject` do KEDA para escalar o worker pela fila |

Boas práticas aplicadas: probes de liveness/readiness, requests/limits, `runAsNonRoot`, `readOnlyRootFilesystem`, `terminationGracePeriodSeconds` longo no worker, Service headless para o Prometheus descobrir cada réplica do worker.

### Rodando no minikube

```bash
minikube start --cpus=4 --memory=8g
minikube addons enable metrics-server        # necessário para o HPA

# Opção A: imagens publicadas pelo CI no GHCR (torne os pacotes públicos no GitHub
# ou crie um imagePullSecret)
kubectl apply -k deploy/k8s/local

# Opção B: imagens construídas localmente dentro do minikube
eval $(minikube docker-env)
for s in auth-service video-service notification-service; do
  docker build --build-arg SERVICE=$s --target service -t ghcr.io/monnclaro/soat-hackaton/$s:latest .
done
docker build --build-arg SERVICE=processor-worker --target worker -t ghcr.io/monnclaro/soat-hackaton/processor-worker:latest .
docker build -f deploy/gateway/Dockerfile -t ghcr.io/monnclaro/soat-hackaton/gateway:latest .
kubectl apply -k deploy/k8s/local
kubectl -n fiapx patch deploy auth-service video-service processor-worker notification-service gateway \
  --type=json -p='[{"op":"add","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Never"}]' 2>/dev/null || true

kubectl -n fiapx get pods -w
minikube service -n fiapx gateway             # abre a aplicação
```

**Em nuvem (AWS):** aplique `deploy/k8s/base` no EKS e aponte os Secrets para RDS PostgreSQL, ElastiCache, Amazon MQ (RabbitMQ), S3 e SES.

---

## 18. Configuração (variáveis de ambiente)

### Comuns

| Variável | Descrição | Padrão |
|---|---|---|
| `PORT` | Porta HTTP do serviço | 8081 / 8082 / 8083 / 8084 |
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `RABBITMQ_URL` | Conexão AMQP | `amqp://guest:guest@localhost:5672/` |
| `RABBITMQ_DELIVERY_LIMIT` | Reentregas antes da DLQ | `5` |

### auth-service

| Variável | Descrição | Padrão |
|---|---|---|
| `DATABASE_URL` | Postgres `fiapx_auth` | `postgres://fiapx:fiapx@localhost:5432/fiapx_auth?sslmode=disable` |
| `JWT_SECRET` | Segredo HS256 (**mín. 32 caracteres**, igual ao do video-service) | — obrigatório |
| `JWT_TTL` | Validade do token | `24h` |
| `BCRYPT_COST` | Custo do bcrypt (0 = padrão 10) | `0` |

### video-service

| Variável | Descrição | Padrão |
|---|---|---|
| `DATABASE_URL` | Postgres `fiapx_videos` | `postgres://fiapx:fiapx@localhost:5432/fiapx_videos?sslmode=disable` |
| `JWT_SECRET` | Mesmo segredo do auth-service | — obrigatório |
| `REDIS_ADDR` / `REDIS_PASSWORD` | Redis | `localhost:6379` / vazio |
| `CACHE_TTL` | TTL da listagem em cache | `30s` |
| `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `S3_REGION`, `S3_USE_SSL` | Object storage | SeaweedFS local, bucket `fiapx-videos` |
| `MAX_UPLOAD_BYTES` | Tamanho máximo do vídeo | `1073741824` (1 GiB) |
| `OUTBOX_INTERVAL` / `OUTBOX_BATCH` | Frequência e lote do relay | `1s` / `100` |
| `CONSUMER_CONCURRENCY` | Consumidores de status | `4` |

### processor-worker

| Variável | Descrição | Padrão |
|---|---|---|
| `WORKER_CONCURRENCY` | Vídeos processados em paralelo por réplica | `2` |
| `FRAME_FPS` | Frames extraídos por segundo de vídeo | `1` |
| `JOB_TIMEOUT` | Tempo máximo por vídeo | `30m` |
| `MAX_ATTEMPTS` | Tentativas em falhas transitórias antes de `FAILED` | `3` |
| `WORK_DIR` | Diretório temporário | `/work` |
| `FFMPEG_BIN` | Caminho do ffmpeg | `ffmpeg` |
| `S3_*` | Mesmas do video-service | — |

### notification-service

| Variável | Descrição | Padrão |
|---|---|---|
| `SMTP_ADDR` | Servidor SMTP `host:porta` | `localhost:1025` |
| `SMTP_FROM` | Remetente | `FIAP X <no-reply@fiapx.local>` |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | Autenticação (opcional) | vazio |
| `CONSUMER_CONCURRENCY` | Consumidores | `2` |

> No compose, `JWT_SECRET` pode ser sobrescrito com `JWT_SECRET=... docker compose up -d`. **Troque o padrão em qualquer ambiente real.**

---

## 19. Estrutura do repositório

```
.
├── cmd/                              # entrypoints (composição + ciclo de vida)
│   ├── auth-service/
│   ├── video-service/
│   ├── processor-worker/
│   └── notification-service/
├── internal/
│   ├── auth/                         # cadastro, login, JWT, repositório Postgres
│   ├── video/                        # domínio, casos de uso, HTTP, outbox, cache, consumidor
│   ├── processor/                    # orquestração do job, ffmpeg, zip
│   ├── notification/                 # regra de notificação e envio SMTP
│   ├── events/                       # contratos das mensagens
│   ├── integration/                  # testes de integração (build tag "integration")
│   └── platform/                     # infraestrutura compartilhada
│       ├── config/  httpx/  logging/
│       ├── postgres/  rabbitmq/  storage/
│       └── security/                 # JWT e contexto do usuário
├── web/index.html                    # UI (servida pelo gateway)
├── deploy/
│   ├── gateway/                      # Dockerfile + template nginx
│   ├── prometheus/                   # scrape + alertas
│   ├── grafana/                      # datasource + dashboard provisionados
│   ├── seaweedfs/                    # credenciais S3 locais
│   └── k8s/                          # base / local / optional (Kustomize)
├── scripts/
│   ├── db/init.sql                   # criação dos bancos (entregável)
│   └── e2e.sh                        # teste ponta a ponta
├── docs/
│   ├── architecture.md
│   ├── adr/
│   └── openapi.yaml
├── .github/workflows/ci-cd.yml
├── Dockerfile                        # multi-stage para os 4 serviços
└── docker-compose.yml
```

Cada contexto segue **arquitetura hexagonal**: `service.go` (casos de uso) depende apenas de interfaces; `handler.go`/`async.go` são adaptadores de entrada; `postgres.go`/`cache.go` são adaptadores de saída.

---

## 20. Desenvolvimento local sem Docker

Requer Go 1.26+ e ffmpeg no `PATH` (só para o worker). A infraestrutura pode vir do compose:

```bash
docker compose up -d postgres redis rabbitmq s3 mailpit
export JWT_SECRET=dev-secret-com-pelo-menos-32-caracteres
go run ./cmd/auth-service &
go run ./cmd/video-service &
go run ./cmd/processor-worker &
go run ./cmd/notification-service &
```

Os padrões das variáveis já apontam para `localhost`. Sem o gateway, chame os serviços direto: `localhost:8081/auth/...` e `localhost:8082/api/...`.

---

## 21. Solução de problemas

| Sintoma | Causa provável | Solução |
|---|---|---|
| `password authentication failed for user "fiapx"` ao conectar do host | Outro Postgres local ocupa a 5432 | `POSTGRES_HOST_PORT=55432 docker compose up -d` e use a porta 55432 |
| `502 Bad Gateway` logo após subir | Serviços ainda iniciando (aguardam dependências) | Aguarde alguns segundos; `docker compose ps` |
| Vídeo parado em `PENDING` | Workers fora do ar ou fila parada | `docker compose logs processor-worker`; veja a fila no RabbitMQ |
| Vídeo `FAILED` com "ffmpeg não conseguiu processar" | Arquivo corrompido ou codec não suportado | Teste o arquivo localmente com `ffmpeg -i arquivo` |
| E-mail não aparece no Mailpit | notification-service reiniciando ou SMTP incorreto | `docker compose logs notification-service`; confira a DLQ |
| `413` no upload | Arquivo maior que `MAX_UPLOAD_BYTES` | Aumente a variável (e `client_max_body_size` no gateway) |
| `401` nas rotas `/api` | Token ausente/expirado ou `JWT_SECRET` diferente entre auth e video | Faça login de novo; confira o segredo |
| Recomeçar do zero | — | `docker compose down -v && docker compose up -d --build` |

---

## 22. Roteiro sugerido para o vídeo de apresentação

Máximo de 10 minutos, conforme o enunciado:

1. **(1 min) Problema**: mostrar o `main.go` do projeto base e os problemas da [seção 1](#1-o-problema-e-a-solução).
2. **(2 min) Arquitetura**: diagrama da [seção 3](#3-arquitetura) e as decisões principais (fila, outbox, S3, database-per-service), citando os [ADRs](docs/adr/README.md).
3. **(3 min) Demonstração** em http://localhost:8080:
   - criar conta e fazer login;
   - enviar 3–4 vídeos de uma vez + 1 arquivo inválido;
   - mostrar os status mudando e baixar um ZIP;
   - abrir o Mailpit com o e-mail de falha.
4. **(1,5 min) Escalabilidade e resiliência**: `docker compose up -d --scale processor-worker=4`, logs das réplicas dividindo o trabalho, fila no RabbitMQ e dashboard do Grafana.
5. **(1,5 min) Qualidade e CI/CD**: Actions verde (lint, testes, cobertura, E2E, imagens), testes de integração e `scripts/e2e.sh`.
6. **(1 min) Documentação**: `docs/architecture.md`, ADRs, OpenAPI e `scripts/db/init.sql`.
