# Uso do sistema

- [Interface web](#interface-web)
- [Referência da API](#referência-da-api)

## Interface web

1. Acesse **http://localhost:8080**.
2. Na aba **Criar conta**, informe nome, e-mail e senha (mínimo de 8 caracteres). Ao cadastrar, você já entra logado.
   Nas próximas vezes, use a aba **Entrar**. A sessão (token JWT) fica salva no navegador por 24h; quando expira, a tela volta para o login com um aviso.
3. Em **Enviar vídeos**, **arraste** um ou vários arquivos (mp4, avi, mov, mkv, webm, wmv, flv, m4v) para a área tracejada ou clique nela para selecionar.
   - Cada arquivo ganha uma **barra de progresso** própria e os envios seguem em paralelo.
   - Arquivos com formato inválido ou acima de 1 GB são recusados antes do envio.
4. Os **contadores** no topo mostram o total de vídeos, quantos estão na fila ou processando, concluídos e com falha.
5. A lista **Meus vídeos** se atualiza sozinha: a cada 3s enquanto houver vídeo em andamento (indicador "atualizando") e a cada 20s quando não houver. Ela pausa quando a aba fica em segundo plano.
   - **Na fila** (`PENDING`): aceito, aguardando um worker;
   - **Processando** (`PROCESSING`);
   - **Concluído** (`COMPLETED`): mostra a quantidade de frames e o botão **Baixar ZIP**;
   - **Falhou** (`FAILED`): mostra o motivo, e um e-mail é enviado.
6. Quando um vídeo termina ou falha, aparece um aviso no canto da tela.
7. Para ver o e-mail de falha, envie um arquivo inválido renomeado para `.mp4` e abra o **Mailpit** em http://localhost:8025.

A interface segue o tema claro/escuro do sistema operacional e funciona em telas de celular.

---

## Referência da API

Base: `http://localhost:8080`. Contrato completo em [openapi.yaml](openapi.yaml). Erros sempre seguem o formato `{"error": "mensagem"}`.

### Autenticação

#### `POST /auth/register`
```bash
curl -X POST localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"Ana","email":"ana@fiapx.com","password":"senha-segura"}'
```
```json
201 Created
{ "id": "0b6c…", "name": "Ana", "email": "ana@fiapx.com", "created_at": "2026-11-11T22:00:00Z" }
```
Erros: `400` JSON inválido · `409` e-mail já cadastrado · `422` validação (nome vazio, e-mail inválido, senha fora de 8–72 caracteres).

#### `POST /auth/login`
```bash
curl -X POST localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"ana@fiapx.com","password":"senha-segura"}'
```
```json
200 OK
{ "access_token": "eyJhbGciOi…", "token_type": "Bearer", "expires_at": "2026-11-12T22:00:00Z",
  "user": { "id": "0b6c…", "name": "Ana", "email": "ana@fiapx.com", "created_at": "…" } }
```
Erros: `401` e-mail ou senha inválidos.

#### `GET /auth/me` (autenticado)
Retorna o usuário do token.

### Vídeos (todas exigem `Authorization: Bearer <token>`)

```bash
TOKEN=$(curl -s -X POST localhost:8080/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"ana@fiapx.com","password":"senha-segura"}' | sed -E 's/.*"access_token":"([^"]+)".*/\1/')
```

#### `POST /api/videos`: enviar vídeo
```bash
curl -H "Authorization: Bearer $TOKEN" -F video=@aula.mp4 localhost:8080/api/videos
```
```json
202 Accepted
Location: /api/videos/7f3e…
{ "id": "7f3e…", "original_name": "aula.mp4", "status": "PENDING", "size_bytes": 1048576,
  "frame_count": 0, "created_at": "…", "updated_at": "…" }
```
Erros: `400` não é multipart ou sem o campo `video` · `401` sem token · `413` maior que `MAX_UPLOAD_BYTES` · `422` extensão não suportada ou arquivo vazio.

#### `GET /api/videos`: listar status
```json
200 OK
{ "videos": [
  { "id": "7f3e…", "original_name": "aula.mp4", "status": "COMPLETED", "frame_count": 120,
    "download_url": "/api/videos/7f3e…/download", … },
  { "id": "a1b2…", "original_name": "corrompido.mp4", "status": "FAILED",
    "error": "ffmpeg não conseguiu processar o vídeo: …", … }
] }
```

#### `GET /api/videos/{id}`: detalhe
`404` se não existir ou pertencer a outro usuário.

#### `GET /api/videos/{id}/download`: baixar o ZIP
```bash
curl -H "Authorization: Bearer $TOKEN" -o frames.zip localhost:8080/api/videos/<id>/download
```
`200` com `Content-Type: application/zip` e `Content-Disposition: attachment; filename="aula_frames.zip"` · `409` se ainda não foi concluído ou falhou · `404` se não existir.

### Endpoints operacionais (em cada serviço, sem passar pelo gateway)

| Rota | Uso |
|---|---|
| `GET /healthz` | Liveness: o processo está vivo |
| `GET /readyz` | Readiness: dependências essenciais acessíveis (Postgres e S3 no video-service; Postgres no auth-service; RabbitMQ/S3 nos consumidores). O Redis não entra, por ser só cache |
| `GET /metrics` | Métricas Prometheus |
