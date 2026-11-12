# ADR 0010 — Notificação de Falha Orientada a Eventos (E-mail)

**Status:** Aceito

## Contexto

Requisito: "em caso de erro, um usuário pode ser notificado (e-mail ou outro meio)". A notificação não deve atrasar nem fazer falhar o processamento, e o canal pode mudar (SMS, push, webhook).

## Decisão

- Um **notification-service** dedicado consome a fila `notification.video-failed`, ligada à routing key `video.status.failed`. É o mesmo evento que atualiza o status no video-service (*pub/sub*: cada interessado tem a sua fila).
- O evento já carrega `user_email`, `user_name`, `original_name` e `error`, então o serviço não consulta nenhum outro serviço nem banco.
- O envio usa a porta `Sender`; a implementação atual é **SMTP** (Mailpit no ambiente local, Amazon SES/SMTP em produção, configurado por `SMTP_*`).
- O envelope SMTP usa o endereço puro (`MAIL FROM`/`RCPT TO`), e os cabeçalhos são sanitizados contra injeção de CRLF.
- Falha de envio é transitória: retentativa com backoff e DLQ após o limite ([ADR 0008](./0008-retentativas-dlq-e-idempotencia.md)).

## Alternativas descartadas

- **Worker enviar o e-mail diretamente**: acopla processamento e canal de comunicação; um SMTP lento atrasaria a fila de vídeos.
- **video-service enviar ao aplicar `FAILED`**: mistura responsabilidades na API.
- **SNS/SES direto via SDK**: prende o código ao provedor; o SMTP funciona com qualquer um.

## Consequências

- Novos canais ou novos gatilhos (ex.: avisar conclusão) são adicionados sem tocar no worker, criando outra fila/binding ou outro `Sender`.
- Indisponibilidade do SMTP não afeta o processamento; os e-mails ficam represados na fila.
- Possível e-mail duplicado em reentregas (aceitável).
