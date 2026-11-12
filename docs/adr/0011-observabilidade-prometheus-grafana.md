# ADR 0011 — Observabilidade com Prometheus + Grafana

**Status:** Aceito

## Contexto

Num sistema assíncrono, "a API respondeu 202" não diz se os vídeos estão sendo processados. É preciso enxergar backlog da fila, throughput, falhas, latência e saúde de cada réplica.

## Decisão

- Cada serviço expõe **`/metrics`** (client_golang) com métricas HTTP (`http_requests_total`, `http_request_duration_seconds`, rotuladas pelo *pattern* da rota para evitar alta cardinalidade) e de negócio:
  - `fiapx_video_uploads_total{result}`
  - `fiapx_processor_jobs_total{result}`, `fiapx_processor_job_duration_seconds`, `fiapx_processor_jobs_in_flight`, `fiapx_processor_frames_extracted_total`
  - `fiapx_video_status_updates_total`, `fiapx_notifications_total{result}`
- O RabbitMQ expõe métricas de filas pelo plugin `rabbitmq_prometheus` (`:15692`).
- **Prometheus** coleta tudo (descoberta por DNS das réplicas do worker) e avalia alertas: backlog da fila, mensagens na DLQ, taxa de falha > 20%, serviço fora do ar.
- **Grafana** com datasource e dashboard provisionados como código (`deploy/grafana`).
- **Logs** estruturados em JSON (`log/slog`) com `service`, `video_id`, `attempt` e `message_id`, prontos para Loki/ELK/CloudWatch.
- **Health checks** separados: `/healthz` (liveness, sem dependências) e `/readyz` (readiness, checa Postgres/Redis/S3/broker).

## Alternativas descartadas

- **ELK Stack**: forte para logs, mais pesado para métricas e alertas; os logs JSON já permitem ingestão futura.
- **APM SaaS (New Relic/Datadog)**: usado em fases anteriores, mas depende de conta e licença; Prometheus + Grafana roda localmente para a demonstração.

## Consequências

- O dashboard mostra em tempo real os vídeos em processamento, a fila, a DLQ e as réplicas ativas, útil na apresentação.
- Os mesmos alertas servem de critério para escalar (o backlog da fila é o mesmo sinal usado pelo KEDA).
- Tracing distribuído (OpenTelemetry) não foi implementado; os IDs de vídeo e mensagem nos logs permitem correlação manual.
