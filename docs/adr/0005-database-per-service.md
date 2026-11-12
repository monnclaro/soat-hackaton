# ADR 0005 — Database-per-Service no PostgreSQL

**Status:** Aceito

## Contexto

O sistema precisa persistir usuários e vídeos. Microsserviços que compartilham tabelas ficam acoplados por schema: uma migração de um quebra o outro, e não dá para escalar ou trocar o banco de um deles isoladamente.

## Decisão

- **PostgreSQL** como banco relacional (recomendado pelo enunciado; transações são necessárias para o outbox, [ADR 0003](./0003-transactional-outbox.md)).
- Um banco por serviço: `fiapx_auth` (auth-service: `users`) e `fiapx_videos` (video-service: `videos`, `outbox`). Só o serviço dono acessa o seu banco.
- Sem FK entre bancos: `videos.user_id` é uma referência lógica. O e-mail do usuário é **desnormalizado** em `videos.user_email` (vem do JWT no upload) para que a notificação não dependa de consultar o auth-service.
- Schema versionado em dois lugares, com o mesmo DDL idempotente:
  - [scripts/db/init.sql](../../scripts/db/init.sql): script de criação (entregável), executado pelo container do Postgres;
  - `internal/*/migrations.sql`: embutido no binário e aplicado na subida com `pg_advisory_lock` (seguro com várias réplicas subindo juntas).
- notification-service e processor-worker não têm banco: recebem tudo de que precisam nos eventos.

## Alternativas descartadas

- **Banco único compartilhado**: mais simples, mas acopla os serviços.
- **NoSQL (MongoDB/DynamoDB)**: o modelo é pequeno e relacional, e o outbox precisa de transação atômica entre vídeo e mensagem.
- **Ferramenta de migração (golang-migrate/Flyway)**: com duas tabelas por serviço, DDL idempotente é suficiente; vale migrar para uma ferramenta quando surgirem alterações de schema.

## Consequências

- Em ambiente local os dois bancos ficam na mesma instância Postgres (baixo custo); em produção podem ser instâncias separadas.
- A troca de e-mail de um usuário não é refletida em vídeos antigos (aceitável: o e-mail é usado apenas para notificar o próprio upload).
- O schema existe em dois arquivos; o CI verifica que a cópia do K8s (`deploy/k8s/local/init.sql`) é idêntica ao script.
