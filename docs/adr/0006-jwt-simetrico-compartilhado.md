# ADR 0006 — Autenticação com JWT Simétrico Compartilhado

**Status:** Aceito

## Contexto

O sistema deve ser protegido por usuário e senha. Várias réplicas do video-service precisam validar a identidade em toda requisição sem criar gargalo no auth-service.

## Decisão

- O **auth-service** guarda a senha com **bcrypt** e, no login, emite um **JWT HS256** com `sub` (id), `email`, `name`, `iss=fiapx-auth` e expiração (`JWT_TTL`, 24h por padrão).
- O **video-service** valida o token localmente (assinatura, algoritmo fixo HS256, issuer e expiração) com o mesmo `JWT_SECRET` (mínimo de 32 caracteres, injetado via Secret).
- Toda rota `/api/*` exige `Authorization: Bearer`. Cada consulta filtra pelo `sub` do token, e um vídeo de outro usuário retorna **404** (não 403), para não revelar a existência do recurso.
- Login com e-mail inexistente ou senha errada retorna a mesma mensagem genérica.

## Alternativas descartadas

- **Sessão com estado (Redis)**: exige consultar o store a cada requisição e acopla os serviços a ele.
- **RS256 com JWKS**: melhor quando há muitos consumidores ou terceiros validando tokens; aqui só um serviço valida, e o segredo compartilhado é mais simples de operar.
- **Validar o token no gateway**: o nginx OSS não valida JWT nativamente; a validação no serviço também garante a autorização por dono do recurso.

## Consequências

- Validação sem I/O: escala com as réplicas do video-service.
- Não há revogação imediata; um token roubado vale até expirar. A mitigação é um TTL curto (configurável). Uma denylist em Redis fica como evolução.
- A rotação do `JWT_SECRET` invalida todos os tokens ativos e exige redeploy coordenado de auth e video.
