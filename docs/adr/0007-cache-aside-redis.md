# ADR 0007 — Cache-Aside em Redis para a Listagem de Status

**Status:** Aceito

## Contexto

A listagem de status (`GET /api/videos`) é a rota mais chamada: a UI faz polling a cada 4s enquanto há vídeos em processamento. Com muitos usuários isso vira carga de leitura constante no Postgres para dados que mudam poucas vezes por vídeo.

## Decisão

- **Cache-aside** no video-service: lê `fiapx:videos:user:<id>` no Redis; em caso de *miss*, consulta o Postgres e grava no cache com **TTL curto** (`CACHE_TTL`, 30s).
- **Invalidação explícita** da chave do usuário a cada upload e a cada mudança de status aplicada.
- **Falhas do Redis não derrubam a API**: erro de cache é logado e a leitura segue para o banco.

## Alternativas descartadas

- **Sem cache**: aceitável no volume atual, mas o polling cresce linearmente com os usuários ativos.
- **Write-through (atualizar o cache em vez de invalidar)**: mais complexo e sujeito a escrever listas inconsistentes com eventos fora de ordem.
- **Server-Sent Events/WebSocket no lugar de polling**: melhor experiência, mas exige conexões persistentes e *fan-out* entre réplicas. Fica como evolução.

## Consequências

- Polling repetido é atendido pelo Redis enquanto nada muda.
- Numa corrida rara (leitura do banco → invalidação → gravação do cache antigo), o usuário pode ver um status desatualizado por até `CACHE_TTL`.
- O Redis roda com AOF no compose; perder o cache é inofensivo.
