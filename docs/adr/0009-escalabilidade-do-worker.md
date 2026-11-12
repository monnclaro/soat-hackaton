# ADR 0009 — Escalabilidade do Worker (Concorrência + HPA/KEDA)

**Status:** Aceito

## Contexto

Requisito: "processar mais de um vídeo ao mesmo tempo" e "arquitetura que permita ser escalada". O ffmpeg é CPU-bound e a duração do job varia muito com o tamanho do vídeo.

## Decisão

Escalar em dois níveis:

1. **Dentro do pod**: `WORKER_CONCURRENCY` goroutines consumindo a mesma fila, com prefetch igual à concorrência, para que o broker não entregue mais mensagens do que o pod processa.
2. **Entre pods**: réplicas são *competing consumers* na mesma fila.
   - **HPA por CPU** (60%, de 2 a 10 réplicas, scale-down estabilizado em 5 min) na base K8s;
   - **KEDA por tamanho da fila** (opcional, `deploy/k8s/optional`), que reage ao pico antes de a CPU saturar.
- **Graceful shutdown**: ao receber SIGTERM, o worker cancela o consumo (para de receber), termina os vídeos em andamento e sai. `terminationGracePeriodSeconds: 600`. Se o pod morrer antes, a mensagem sem ack é reentregue a outra réplica.
- `JOB_TIMEOUT` (30 min) limita vídeos patológicos.
- O video-service e o auth-service também têm HPA (stateless).

## Alternativas descartadas

- **Um vídeo por pod (Kubernetes Job por vídeo)**: isolamento máximo, mas alto overhead de agendamento para vídeos curtos e mais complexidade de orquestração.
- **Somente concorrência interna**: limitada a uma máquina.
- **Escalar só por CPU**: reage tarde a um pico de uploads, por isso o KEDA é oferecido.

## Consequências

- No compose: 2 réplicas × 2 = 4 vídeos simultâneos (`docker compose up --scale processor-worker=N` amplia). No E2E, os vídeos foram distribuídos entre as réplicas.
- O custo escala com a demanda; no mínimo 2 réplicas para disponibilidade.
- Deploys do worker podem demorar até o fim dos jobs em andamento.
