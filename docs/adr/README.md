# Architecture Decision Records

| ADR | Decisão | Status |
|---|---|---|
| [0001](./0001-microsservicos-em-monorepo.md) | Microsserviços em Go num monorepo | Aceito |
| [0002](./0002-processamento-assincrono-com-rabbitmq.md) | Processamento assíncrono com RabbitMQ (quorum queues) | Aceito |
| [0003](./0003-transactional-outbox.md) | Transactional outbox para publicar o pedido de processamento | Aceito |
| [0004](./0004-object-storage-s3.md) | Object storage S3 no lugar do disco local | Aceito |
| [0005](./0005-database-per-service.md) | Database-per-service no PostgreSQL | Aceito |
| [0006](./0006-jwt-simetrico-compartilhado.md) | Autenticação com JWT simétrico compartilhado | Aceito |
| [0007](./0007-cache-aside-redis.md) | Cache-aside em Redis para a listagem de status | Aceito |
| [0008](./0008-retentativas-dlq-e-idempotencia.md) | Retentativas, DLQ e consumidores idempotentes | Aceito |
| [0009](./0009-escalabilidade-do-worker.md) | Escalabilidade do worker (concorrência + HPA/KEDA) | Aceito |
| [0010](./0010-notificacao-por-eventos.md) | Notificação de falha orientada a eventos (e-mail) | Aceito |
| [0011](./0011-observabilidade-prometheus-grafana.md) | Observabilidade com Prometheus + Grafana | Aceito |
| [0012](./0012-seaweedfs-no-ambiente-local.md) | SeaweedFS como S3 no ambiente local | Aceito |
