# Roteiro sugerido para o vídeo de apresentação

Máximo de 10 minutos, conforme o enunciado:

1. **(1 min) Problema**: mostrar o `main.go` do projeto base e a tabela "O problema e a solução" do [README](../README.md#o-problema-e-a-solução).
2. **(2 min) Arquitetura**: diagrama do [README](../README.md#arquitetura) e as decisões principais (fila, outbox, armazenamento compatível com S3, database-per-service), citando os [ADRs](adr/README.md).
3. **(3 min) Demonstração** em http://localhost:8080:
   - criar conta (já entra logado);
   - arrastar 3–4 vídeos de uma vez + 1 arquivo inválido;
   - mostrar os contadores e os status mudando e baixar um ZIP;
   - abrir o Mailpit com o e-mail de falha.
4. **(1,5 min) Escalabilidade e resiliência**: `docker compose up -d --scale processor-worker=4`, logs das réplicas dividindo o trabalho, fila no RabbitMQ e dashboard do Grafana ([how-it-works.md](how-it-works.md#escalabilidade)).
5. **(1,5 min) Qualidade e CI/CD**: Actions verde (lint, testes, cobertura, E2E, imagens), testes de integração e `scripts/e2e.sh` ([quality.md](quality.md)).
6. **(1 min) Documentação**: [architecture.md](architecture.md), ADRs, [openapi.yaml](openapi.yaml) e [scripts/db/init.sql](../scripts/db/init.sql).
