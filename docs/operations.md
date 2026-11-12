# Operação

- [Observabilidade](#observabilidade)
- [Kubernetes](#kubernetes)
- [Configuração (variáveis de ambiente)](#configuração-variáveis-de-ambiente)
- [Armazenamento de arquivos](#armazenamento-de-arquivos-sem-dependência-de-nuvem)

## Observabilidade

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

### Alertas
Definidos em [deploy/prometheus/alerts.yml](../deploy/prometheus/alerts.yml): `VideoQueueBacklog` (fila acima de 50 por 5 min), `DeadLetterMessages` (DLQ maior que 0), `HighProcessingFailureRate` (mais de 20% de falhas), `ServiceDown`. Veja em http://localhost:9090/alerts.

### Logs
JSON estruturado em todos os serviços (`service`, `video_id`, `attempt`, `message_id`, `duration_ms`):
```bash
docker compose logs -f processor-worker | grep video_id
```

---

## Kubernetes

Manifests com Kustomize em [deploy/k8s](../deploy/k8s):

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

# Opção 1: imagens publicadas pelo CI no GHCR (torne os pacotes públicos no GitHub
# ou crie um imagePullSecret)
kubectl apply -k deploy/k8s/local

# Opção 2: imagens construídas localmente dentro do minikube
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

**Sem dependência de nuvem:** `deploy/k8s/local` roda tudo self-hosted em qualquer Kubernetes (on-premises, k3s ou o Kubernetes gerenciado de qualquer provedor). Para usar serviços gerenciados, aplique só `deploy/k8s/base` e aponte o ConfigMap/Secret para eles (ver [architecture.md](architecture.md#10-produção-agnóstico-de-nuvem)).

---

## Configuração (variáveis de ambiente)

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

## Armazenamento de arquivos: sem dependência de nuvem

"S3" aqui é o **protocolo** de object storage, não a AWS. O código usa um cliente S3 genérico; o padrão é o **SeaweedFS** (open source, self-hosted), que sobe junto no Compose e no Kubernetes local. Qualquer outra implementação compatível funciona **sem alterar código**, só com variáveis:

| Opção | Tipo | Exemplo de configuração |
|---|---|---|
| **SeaweedFS** (padrão) | Self-hosted | `S3_ENDPOINT=s3:8333`, `S3_USE_SSL=false` |
| Garage / Ceph RGW / MinIO | Self-hosted | `S3_ENDPOINT=<host>:<porta>` |
| Cloudflare R2 | Gerenciado | `S3_ENDPOINT=<account-id>.r2.cloudflarestorage.com`, `S3_REGION=auto`, `S3_USE_SSL=true` |
| Backblaze B2 | Gerenciado | `S3_ENDPOINT=s3.<região>.backblazeb2.com`, `S3_USE_SSL=true` |
| DigitalOcean Spaces / Wasabi / AWS S3 | Gerenciado | endpoint do provedor, `S3_USE_SSL=true` |

Por que existe um storage compartilhado: o vídeo chega no video-service, mas é processado pelo worker em outro container ou máquina, e o ZIP gerado pelo worker é baixado pelo video-service. Disco local não funciona com várias réplicas ([ADR 0004](adr/0004-object-storage-s3.md)).
