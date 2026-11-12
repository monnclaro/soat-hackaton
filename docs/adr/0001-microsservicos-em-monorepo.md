# ADR 0001 — Microsserviços em Go num Monorepo

**Status:** Aceito

## Contexto

O projeto base é um único binário Go (`main.go`) que recebe o upload, roda o ffmpeg dentro da requisição HTTP, grava o ZIP no disco local e serve o download. Ele não escala horizontalmente (o arquivo fica no disco de uma instância), segura a conexão do cliente durante todo o processamento e mistura responsabilidades. O enunciado pede microsserviços, escalabilidade e qualidade de software, e o projeto deve ser versionado no GitHub (repositório `soat-hackaton`).

## Decisão

Separar o sistema em quatro microsserviços com responsabilidades e ciclos de escala distintos, mais um gateway:

- **auth-service**: identidade (cadastro, login, JWT).
- **video-service**: API de vídeos (upload, status, download) e dono do estado dos vídeos.
- **processor-worker**: processamento pesado (ffmpeg + zip), sem API pública.
- **notification-service**: envio de e-mail.
- **gateway** (nginx): ponto único de entrada e UI.

Manter **Go** (mesma linguagem do projeto base e adequada a serviços de I/O concorrente), usando a stdlib `net/http` (roteamento com padrões do Go 1.22+) e `log/slog`.

Todos vivem num **monorepo** (`cmd/<serviço>` + `internal/<contexto>`), com um Dockerfile multi-stage parametrizado por `SERVICE`. Cada serviço gera sua própria imagem e tem Deployment/HPA independentes.

Internamente cada contexto segue arquitetura hexagonal: casos de uso (`service.go`) dependem de portas (interfaces); adaptadores (`postgres.go`, `cache.go`, `handler.go`, `async.go`) ficam na borda; `cmd/*/main.go` faz a composição.

## Alternativas descartadas

- **Manter o monólito, só tornando o processamento assíncrono com goroutines**: não sobrevive a reinício (perde o trabalho em andamento) e não escala além de uma máquina.
- **Um repositório por serviço**: aumenta o custo de CI e de versionamento de contratos (`internal/events`) sem benefício para um time pequeno; o enunciado aceita "link do(s) projeto(s)".
- **Framework web (gin)**: o projeto base usa gin, mas a stdlib já cobre roteamento, middlewares e graceful shutdown com menos dependências.

## Consequências

- Escala independente: o worker (CPU-bound) escala sem replicar a API, e vice-versa.
- Contratos de mensagens compartilhados em `internal/events` com tipagem em tempo de compilação; uma mudança incompatível aparece no build de todos os serviços.
- Um pipeline único publica cinco imagens; um deploy de um serviço não exige deploy dos outros, mas todos compartilham a mesma árvore de versão.
