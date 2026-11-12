# ADR 0003 — Transactional Outbox para Publicar o Pedido de Processamento

**Status:** Aceito

## Contexto

No upload, o video-service precisa (1) gravar o vídeo no banco como `PENDING` e (2) publicar `video.uploaded` no RabbitMQ. Fazer as duas coisas em sequência é um *dual write*:

- banco ok + broker fora → vídeo fica `PENDING` para sempre (requisição perdida);
- broker ok + banco falha → worker processa um vídeo que a API não conhece.

O requisito "não perder requisições" exclui as duas situações.

## Decisão

Usar o padrão **Transactional Outbox**:

1. Na mesma transação do `INSERT INTO videos`, gravar a mensagem em `outbox (id, routing_key, payload, published_at NULL)`.
2. O cliente recebe `202` somente após o commit.
3. Um **relay** em background (`OutboxRelay`, dentro do video-service) busca as pendentes com `SELECT ... FOR UPDATE SKIP LOCKED`, publica com *publisher confirms* e marca `published_at` apenas para as confirmadas.
4. Se o lote vier cheio, o relay roda de novo imediatamente para drenar picos; caso contrário, aguarda `OUTBOX_INTERVAL` (1s).

## Alternativas descartadas

- **Publicar direto após o commit**: perde a mensagem se o processo cair entre o commit e o publish, ou se o broker estiver fora.
- **Publicar antes do commit**: gera mensagens órfãs se a transação falhar.
- **CDC (Debezium)**: mesma garantia, mas adiciona Kafka Connect e mais infraestrutura do que o caso justifica.

## Consequências

- Garantia: todo upload confirmado ao cliente gera pelo menos uma mensagem de processamento, mesmo com o broker indisponível no momento do upload.
- `SKIP LOCKED` permite várias réplicas do video-service rodando o relay em paralelo sem publicar a mesma mensagem duas vezes no caminho normal.
- Se o processo cair depois do publish e antes do `UPDATE published_at`, a mensagem é publicada de novo (*at-least-once*); o worker é idempotente.
- Latência adicional de até ~1s entre o upload e o início do processamento.
- A tabela `outbox` cresce; em produção, um job deve expurgar as linhas publicadas antigas.
