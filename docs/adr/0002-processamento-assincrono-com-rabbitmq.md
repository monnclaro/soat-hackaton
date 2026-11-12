# ADR 0002 — Processamento Assíncrono com RabbitMQ (Quorum Queues)

**Status:** Aceito

## Contexto

Extrair frames de um vídeo pode levar minutos. Requisitos: processar vários vídeos ao mesmo tempo e **não perder requisições em picos**. O processamento síncrono do projeto base prende a conexão do cliente e, sob pico, esgota CPU/memória da API, que passa a recusar novas requisições.

## Decisão

Desacoplar aceitação e processamento com mensageria:

- O video-service aceita o upload, persiste e retorna **202 Accepted**; o processamento vira uma mensagem `video.uploaded`.
- **RabbitMQ** com exchange topic `fiapx.events` e filas por consumidor (`processor.video-uploaded`, `video-service.status-changed`, `notification.video-failed`).
- Filas do tipo **quorum** (replicadas, persistidas em disco) com mensagens `persistent`.
- **Publisher confirms**: o publicador só considera a mensagem entregue após o ack do broker.
- **Ack manual** no consumidor, enviado apenas ao final do processamento; prefetch igual à concorrência do worker.
- O worker publica o progresso como eventos `video.status.{processing|completed|failed}`, que o video-service consome para atualizar o banco.

## Alternativas descartadas

- **Apache Kafka**: excelente para streams e replay, mas o caso aqui é *work queue* (cada vídeo processado uma vez, com ack individual, retentativa e DLQ). No Kafka isso exigiria gerenciar offsets/partições e implementar retry/DLQ manualmente, além de ser mais pesado de operar.
- **Fila em Redis (listas/streams)**: menos garantias de durabilidade e sem DLX/delivery-limit nativos.
- **Filas clássicas do RabbitMQ**: as quorum queues oferecem replicação, `x-delivery-limit` e o header `x-delivery-count`, usados na política de retentativas (ver [ADR 0008](./0008-retentativas-dlq-e-idempotencia.md)).

## Consequências

- Picos viram backlog na fila, não erros: a API continua respondendo e os workers drenam no ritmo deles.
- Entrega *at-least-once*: os consumidores precisam ser idempotentes ([ADR 0008](./0008-retentativas-dlq-e-idempotencia.md)).
- O status é eventualmente consistente; a UI consulta a listagem por polling.
- O RabbitMQ vira dependência crítica; em produção, usar Amazon MQ ou cluster de 3 nós.
