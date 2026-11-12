# Como executar

Há quatro formas de rodar o projeto. Escolha conforme o objetivo:

| Opção | Quando usar | O que precisa instalar |
|---|---|---|
| [A. Docker Compose](#opção-a--docker-compose-recomendado) (recomendado) | Demonstração, avaliação, testar o sistema completo | Docker |
| [B. Serviços na mão + infra no Docker](#opção-b--serviços-go-na-mão--infraestrutura-no-docker) | Desenvolver/depurar os serviços Go (breakpoints, `go run`) | Docker, Go, ffmpeg |
| [C. Tudo na mão, sem Docker](#opção-c--tudo-na-mão-sem-docker) | Ambiente sem Docker | Go, ffmpeg, Node.js, PostgreSQL, Redis, RabbitMQ, SeaweedFS, Mailpit |
| [D. Kubernetes](#opção-d--kubernetes-minikube) | Demonstrar HPA, réplicas e deploy | Docker, minikube, kubectl |

Em todas as opções, comece clonando o repositório:

```bash
git clone https://github.com/monnclaro/soat-hackaton.git
cd soat-hackaton
```

## Portas usadas

| Porta | Componente | Porta | Componente |
|---|---|---|---|
| 8080 | Gateway (UI + API) | 5432 | PostgreSQL |
| 8081 | auth-service | 6379 | Redis |
| 8082 | video-service | 5672 / 15672 / 15692 | RabbitMQ (AMQP / painel / métricas) |
| 8083 | processor-worker | 8333 | SeaweedFS (S3) |
| 8084 | notification-service | 1025 / 8025 | Mailpit (SMTP / painel) |
| 3000 | Grafana | 9090 | Prometheus |

---

## Opção A — Docker Compose (recomendado)

Sobe **tudo** (4 microsserviços, gateway, PostgreSQL, Redis, RabbitMQ, SeaweedFS, Mailpit, Prometheus e Grafana) com um comando.

**Pré-requisitos:** Docker Desktop (Windows/macOS) ou Docker Engine (Linux) com **Compose v2**. Confira com `docker compose version`.

**Passo 1. Subir o ambiente**

```bash
docker compose up -d --build
```

Na primeira vez leva alguns minutos (download das imagens e compilação dos serviços Go e da UI).

> **Já tem um PostgreSQL instalado na máquina, ocupando a porta 5432?** Use outra porta no host:
> - bash: `POSTGRES_HOST_PORT=55432 docker compose up -d --build`
> - PowerShell: `$env:POSTGRES_HOST_PORT=55432; docker compose up -d --build`

**Passo 2. Conferir se está tudo no ar**

```bash
docker compose ps
```

Todos os containers devem estar `Up`, e os de infraestrutura `(healthy)`. Os serviços Go aguardam as dependências e se conectam sozinhos; se algum reiniciar uma ou duas vezes no início, é normal.

**Passo 3. Abrir a aplicação**

| Serviço | URL | Credenciais |
|---|---|---|
| **Aplicação (UI + API)** | http://localhost:8080 | crie uma conta na tela |
| Grafana (dashboard) | http://localhost:3000 | `admin` / `admin` |
| Prometheus | http://localhost:9090 | — |
| RabbitMQ Management | http://localhost:15672 | `guest` / `guest` |
| Mailpit (e-mails enviados) | http://localhost:8025 | — |
| S3 (SeaweedFS) | http://localhost:8333 | `fiapx-access-key` / `fiapx-secret-key` |
| PostgreSQL | `localhost:5432` | `fiapx` / `fiapx` |

Veja [usage.md](usage.md) para usar o sistema.

**Passo 4 (opcional). Validar automaticamente com o teste ponta a ponta**

```bash
./scripts/e2e.sh        # Linux/macOS/Git Bash; requer curl e unzip
```

**Comandos do dia a dia**

```bash
docker compose logs -f processor-worker            # acompanhar o processamento
docker compose logs -f video-service               # logs da API
docker compose up -d --scale processor-worker=4    # 4 workers × 2 = 8 vídeos simultâneos
docker compose up -d --build video-service         # recompilar só um serviço após mudar o código
docker compose restart notification-service        # reiniciar um serviço
docker compose exec postgres psql -U fiapx -d fiapx_videos   # abrir o banco
docker compose stop                                # pausar (mantém containers e dados)
docker compose down                                # remover containers (mantém os dados)
docker compose down -v                             # remover tudo, inclusive os dados
```

---

## Opção B — Serviços Go na mão + infraestrutura no Docker

Ideal para desenvolver: a infraestrutura sobe no Docker e os microsserviços rodam direto na sua máquina com `go run` (ou pelo debugger da IDE).

**Pré-requisitos**

| Ferramenta | Versão | Instalação |
|---|---|---|
| Go | 1.26+ | https://go.dev/dl · `winget install GoLang.Go` · `brew install go` |
| ffmpeg | qualquer recente | `winget install Gyan.FFmpeg` · `brew install ffmpeg` · `sudo apt install ffmpeg` |
| Docker | Compose v2 | ver opção A |

Confira com `go version` e `ffmpeg -version`. O ffmpeg só é necessário para o processor-worker.

**Passo 1. Subir só a infraestrutura**

```bash
docker compose up -d --wait postgres redis rabbitmq s3 mailpit
```

O PostgreSQL já sobe com os bancos `fiapx_auth` e `fiapx_videos` criados por [scripts/db/init.sql](../scripts/db/init.sql).

**Passo 2. Baixar as dependências Go**

```bash
go mod download
```

**Passo 3. Definir o segredo do JWT** (o mesmo para auth-service e video-service; mínimo de 32 caracteres)

- bash/zsh: `export JWT_SECRET=dev-secret-com-pelo-menos-32-caracteres`
- PowerShell: `$env:JWT_SECRET="dev-secret-com-pelo-menos-32-caracteres"`

Todas as outras variáveis já têm padrão apontando para `localhost` (ver [operations.md](operations.md#configuração-variáveis-de-ambiente)). Se o Postgres estiver em outra porta (ex.: 55432), defina também `DATABASE_URL` para cada serviço.

**Passo 4. Subir os 4 microsserviços, cada um em um terminal**

```bash
# Terminal 1
go run ./cmd/auth-service            # :8081
# Terminal 2
go run ./cmd/video-service           # :8082
# Terminal 3
go run ./cmd/processor-worker        # :8083
# Terminal 4
go run ./cmd/notification-service    # :8084
```

Lembre de definir `JWT_SECRET` em cada terminal (passo 3). Cada serviço mostra `servidor HTTP iniciado` quando está pronto.

> Para rodar dois workers na mão (paralelismo entre réplicas), abra outro terminal com outra porta: `PORT=8085 go run ./cmd/processor-worker` (PowerShell: `$env:PORT=8085; go run ./cmd/processor-worker`).

**Passo 5. Subir o gateway (UI) apontando para os serviços da sua máquina**

O gateway serve a UI e encaminha `/auth` e `/api`. Rode a imagem dele apontando para o host:

- **Windows / macOS (Docker Desktop):**
  ```bash
  docker build -f deploy/gateway/Dockerfile -t fiapx/gateway:local .
  docker run --rm -p 8080:8080 \
    -e AUTH_UPSTREAM=host.docker.internal:8081 \
    -e VIDEO_UPSTREAM=host.docker.internal:8082 \
    fiapx/gateway:local
  ```
- **Linux:**
  ```bash
  docker build -f deploy/gateway/Dockerfile -t fiapx/gateway:local .
  docker run --rm --network host \
    -e AUTH_UPSTREAM=127.0.0.1:8081 -e VIDEO_UPSTREAM=127.0.0.1:8082 \
    fiapx/gateway:local
  ```

Abra http://localhost:8080. Sem o gateway, a API continua acessível direto em `http://localhost:8081/auth/...` e `http://localhost:8082/api/...` (exemplos `curl` em [usage.md](usage.md#referência-da-api)).

**Passo 6 (opcional). Teste ponta a ponta**

O `e2e.sh` gera o vídeo de teste com o ffmpeg do container do worker. Na opção B, use a opção A para rodá-lo, ou faça o fluxo pela UI.

**Para parar:** `Ctrl+C` em cada terminal e `docker compose down` para a infraestrutura.

---

## Opção C — Tudo na mão, sem Docker

Cada componente é instalado e executado diretamente no sistema operacional.

**Passo 1. Instalar as dependências**

| Componente | Linux (Debian/Ubuntu) | macOS (Homebrew) | Windows |
|---|---|---|---|
| Go 1.26+ | https://go.dev/dl | `brew install go` | `winget install GoLang.Go` |
| ffmpeg | `sudo apt install ffmpeg` | `brew install ffmpeg` | `winget install Gyan.FFmpeg` |
| PostgreSQL 17 | `sudo apt install postgresql` | `brew install postgresql@17` | `winget install PostgreSQL.PostgreSQL.17` |
| Redis 7 | `sudo apt install redis-server` | `brew install redis` | via WSL (`sudo apt install redis-server`) ou Memurai |
| RabbitMQ 4 | [guia oficial](https://www.rabbitmq.com/docs/install-debian) | `brew install rabbitmq` | `choco install rabbitmq` (instala o Erlang junto) |
| SeaweedFS (S3) | binário `weed` em https://github.com/seaweedfs/seaweedfs/releases | `brew install seaweedfs` | binário `weed.exe` nas releases |
| Mailpit (SMTP de teste) | binário em https://github.com/axllent/mailpit/releases | `brew install mailpit` | binário `mailpit.exe` nas releases |
| Node.js 20+ (para a UI) | https://nodejs.org | `brew install node` | `winget install OpenJS.NodeJS.LTS` |
| nginx (para a UI) | `sudo apt install nginx` | `brew install nginx` | https://nginx.org/en/download.html |

**Passo 2. Criar o usuário e os bancos no PostgreSQL**

```bash
# como superusuário do Postgres (Linux: sudo -u postgres psql ...)
psql -U postgres -c "CREATE USER fiapx WITH PASSWORD 'fiapx' CREATEDB;"
# executa o script de criação (cria fiapx_auth e fiapx_videos com as tabelas)
psql -U fiapx -h localhost -d postgres -f scripts/db/init.sql
```

> Os serviços também criam as tabelas sozinhos ao subir (migrações idempotentes), mas os **bancos** precisam existir.

**Passo 3. Iniciar a infraestrutura** (cada um em um terminal, ou como serviço do SO)

```bash
# Redis (se não estiver rodando como serviço)
redis-server

# RabbitMQ (se não estiver rodando como serviço). O usuário guest/guest funciona via localhost.
rabbitmq-server

# SeaweedFS com API S3 na porta 8333, usando as credenciais do projeto
mkdir -p ./data/seaweedfs
weed server -dir=./data/seaweedfs -s3 -s3.port=8333 -s3.config=deploy/seaweedfs/s3.json

# Mailpit (SMTP em :1025, painel em http://localhost:8025)
mailpit
```

Confira: Redis com `redis-cli ping` (deve responder `PONG`); RabbitMQ em http://localhost:15672 (o painel requer `rabbitmq-plugins enable rabbitmq_management`); SeaweedFS com `curl http://localhost:8333/healthz`; Mailpit em http://localhost:8025.

**Passo 4. Subir os microsserviços**

Exatamente como nos **passos 2 a 4 da [opção B](#opção-b--serviços-go-na-mão--infraestrutura-no-docker)**: `go mod download`, definir `JWT_SECRET` e `go run ./cmd/<serviço>` em quatro terminais. Os valores padrão das variáveis já apontam para `localhost` e as portas acima.

Exemplo completo em bash, com todas as variáveis explícitas:

```bash
export JWT_SECRET=dev-secret-com-pelo-menos-32-caracteres
export RABBITMQ_URL=amqp://guest:guest@localhost:5672/
export S3_ENDPOINT=localhost:8333 S3_ACCESS_KEY=fiapx-access-key S3_SECRET_KEY=fiapx-secret-key

DATABASE_URL="postgres://fiapx:fiapx@localhost:5432/fiapx_auth?sslmode=disable"   go run ./cmd/auth-service &
DATABASE_URL="postgres://fiapx:fiapx@localhost:5432/fiapx_videos?sslmode=disable" REDIS_ADDR=localhost:6379 go run ./cmd/video-service &
go run ./cmd/processor-worker &
SMTP_ADDR=localhost:1025 go run ./cmd/notification-service &
```

Equivalente em PowerShell (um serviço por janela):

```powershell
$env:JWT_SECRET="dev-secret-com-pelo-menos-32-caracteres"
$env:DATABASE_URL="postgres://fiapx:fiapx@localhost:5432/fiapx_videos?sslmode=disable"
go run ./cmd/video-service
```

**Passo 5. Servir a UI**

Compile a UI e sirva com um nginx local usando a mesma configuração do gateway:

1. Compile a UI:
   ```bash
   cd web
   npm ci
   npm run build        # gera web/dist (SCSS compilado + HTML + JS)
   cd ..
   ```
2. Copie [deploy/gateway/default.conf.template](../deploy/gateway/default.conf.template) para a pasta de configuração do nginx (ex.: `/etc/nginx/conf.d/fiapx.conf`).
3. Substitua `${NGINX_LOCAL_RESOLVERS}` por `127.0.0.1`, `${AUTH_UPSTREAM}` por `127.0.0.1:8081` e `${VIDEO_UPSTREAM}` por `127.0.0.1:8082`.
4. Ajuste `root` para o caminho absoluto da pasta `web/dist` do repositório.
5. Rode `nginx -s reload` e acesse http://localhost:8080.

Sem nginx, use a API diretamente nas portas 8081/8082 (exemplos `curl` em [usage.md](usage.md#referência-da-api)).

**Passo 6. Monitoramento (opcional)**

Instale Prometheus e Grafana e use [deploy/prometheus/prometheus.yml](../deploy/prometheus/prometheus.yml), trocando os alvos (`auth-service:8081` etc.) por `localhost:<porta>`. Importe o dashboard [deploy/grafana/dashboards/fiapx.json](../deploy/grafana/dashboards/fiapx.json) no Grafana.

---

## Opção D — Kubernetes (minikube)

Resumo (passo a passo completo, com imagens do GHCR ou construídas localmente, em [operations.md](operations.md#kubernetes)):

```bash
minikube start --cpus=4 --memory=8g
minikube addons enable metrics-server
kubectl apply -k deploy/k8s/local
kubectl -n fiapx get pods -w
minikube service -n fiapx gateway
```

---

## Solução de problemas

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
