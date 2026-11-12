# ADR 0012 — SeaweedFS como S3 no Ambiente Local

**Status:** Aceito

## Contexto

O ambiente local (Docker Compose, Kubernetes local e CI) precisa de um object storage compatível com S3 ([ADR 0004](./0004-object-storage-s3.md)). A escolha natural seria o MinIO, mas as imagens `minio/minio` deixaram de estar disponíveis publicamente no Docker Hub e no Quay (o pull retorna "access denied"/401).

## Decisão

Usar **SeaweedFS** (`chrislusf/seaweedfs`) com o gateway S3 (`-s3`, porta 8333) e credenciais definidas em `deploy/seaweedfs/s3.json`. O código usa o cliente `minio-go`, compatível com qualquer S3; a troca é só de configuração (`S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`).

## Alternativas descartadas

- **Imagem legada do MinIO (bitnamilegacy)**: congelada, sem atualizações de segurança.
- **LocalStack**: emula S3, mas é mais pesado e voltado a simular vários serviços AWS.
- **Compilar o MinIO a partir do código-fonte**: aumenta o tempo de build e a manutenção.

## Consequências

- Ambiente local e CI sobem sem credenciais externas.
- O SeaweedFS pode ser usado também em produção (self-hosted, inclusive no Kubernetes via `deploy/k8s/local/infra.yaml`), sem depender de nuvem. Se preferirem um serviço gerenciado, qualquer provedor compatível com S3 funciona só com configuração (ex.: Cloudflare R2, Backblaze B2, AWS S3).
- Os testes de integração (`TestS3Storage`) validam o adaptador contra uma implementação S3 real.
