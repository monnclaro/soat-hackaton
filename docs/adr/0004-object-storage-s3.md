# ADR 0004 — Object Storage Compatível com S3 no Lugar do Disco Local

**Status:** Aceito

## Contexto

O projeto base grava os vídeos em `./uploads` e os ZIPs em `./outputs`, no disco do próprio processo. Com API e workers em réplicas diferentes (e pods efêmeros no Kubernetes), um arquivo gravado por uma instância não é visível para outra e some quando o pod é recriado.

## Decisão

Armazenar vídeos e ZIPs em **object storage compatível com a API S3**. "S3" aqui é o protocolo, não um fornecedor: o código usa um cliente S3 genérico e roda com qualquer implementação (SeaweedFS self-hosted por padrão). Estrutura dos objetos:

- `videos/<user_id>/<video_id>.<ext>`: vídeo original, gravado pelo video-service.
- `outputs/<user_id>/<video_id>.zip`: frames, gravado pelo worker.

O **upload é feito em streaming**: o handler lê o `multipart/form-data` com `MultipartReader` e repassa o corpo direto ao `PutObject`, sem bufferizar o vídeo em memória ou disco da API. O gateway também não bufferiza (`proxy_request_buffering off`).

O download passa pelo video-service, que valida o dono do vídeo e faz stream do objeto, sem expor o bucket.

O worker usa um diretório temporário por job (`emptyDir` no K8s), removido ao final.

## Alternativas descartadas

- **Volume compartilhado (NFS/EFS)**: funciona, mas acopla os serviços a um sistema de arquivos de rede, é mais caro e escala pior que S3.
- **Guardar os arquivos no banco (bytea)**: inviável para vídeos de centenas de MB.
- **URL pré-assinada para upload direto ao S3**: reduz tráfego na API, mas exige expor o storage ao cliente e um fluxo de confirmação em duas etapas. Fica como evolução.

## Consequências

- API e workers ficam stateless e escalam livremente.
- Vídeos de até `MAX_UPLOAD_BYTES` (1 GiB por padrão) não pressionam a memória da API.
- O projeto não depende de nenhuma nuvem: o padrão é o SeaweedFS self-hosted, e qualquer serviço compatível com S3 (Garage, Ceph, Cloudflare R2, Backblaze B2, AWS S3...) funciona trocando `S3_ENDPOINT`/credenciais ([ADR 0012](./0012-seaweedfs-no-ambiente-local.md)).
- Se o registro no banco falhar após o upload, o objeto é removido (compensação).
