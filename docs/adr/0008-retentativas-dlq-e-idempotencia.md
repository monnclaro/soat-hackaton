# ADR 0008 — Retentativas, DLQ e Consumidores Idempotentes

**Status:** Aceito

## Contexto

Com entrega *at-least-once* ([ADR 0002](./0002-processamento-assincrono-com-rabbitmq.md), [ADR 0003](./0003-transactional-outbox.md)), mensagens podem chegar duplicadas ou fora de ordem. Falhas também têm naturezas diferentes:

- **definitivas**: vídeo corrompido, nenhum frame extraído, timeout; repetir não resolve;
- **transitórias**: S3, banco, SMTP ou broker momentaneamente indisponíveis;
- **mensagem inválida**: JSON malformado (*poison message*).

## Decisão

Classificar o erro no handler e decidir o destino da mensagem (`internal/platform/rabbitmq`):

| Resultado do handler | Ação |
|---|---|
| `nil` | `ack` |
| erro comum (transitório) | backoff (1s, 4s, 9s... até 30s) e `nack` com requeue |
| `rabbitmq.Permanent(err)` | `nack` sem requeue → DLX → `fiapx.dead-letter` |

- As quorum queues têm `x-delivery-limit` (`RABBITMQ_DELIVERY_LIMIT`, 5): além do limite, a mensagem vai para a DLQ automaticamente.
- No worker, falha definitiva (`ProcessingError`) **publica `FAILED` e dá ack**; o usuário é notificado. Falha transitória é reentregue até `MAX_ATTEMPTS` (lido de `x-delivery-count`); na última tentativa o vídeo vira `FAILED`, para nunca ficar preso em `PROCESSING`.
- **Idempotência**:
  - video-service: máquina de estados `PENDING → PROCESSING → COMPLETED|FAILED`, com estados terminais; eventos duplicados ou atrasados são ignorados;
  - worker: chave do ZIP determinística (`outputs/<user>/<video>.zip`), então reprocessar sobrescreve o mesmo objeto;
  - eventos têm `message_id` determinístico (`<video_id>-<status>`), que permite deduplicar.
- Alerta no Prometheus quando há mensagens na DLQ.

## Alternativas descartadas

- **Requeue imediato sem backoff**: esgotava o `x-delivery-limit` em milissegundos. O E2E reproduziu isso com o SMTP falhando, e o backoff foi adotado.
- **Filas de retry com TTL (delayed queues)**: backoff sem ocupar o consumidor, mas mais filas e bindings para manter; o `sleep` limitado a 30s é suficiente no volume atual.
- **Exactly-once**: não é garantido por RabbitMQ; idempotência é a solução padrão.

## Consequências

- Nenhum vídeo fica eternamente em `PROCESSING`: todos terminam em `COMPLETED` ou `FAILED`.
- Enquanto um consumidor espera o backoff, aquele slot de concorrência fica ocupado.
- A notificação de falha pode, em caso raro de reentrega, enviar e-mail duplicado (aceitável).
- Mensagens na DLQ exigem análise manual (reprocessar pela UI do RabbitMQ).
