# ADR 0014 — Front-end em React + TypeScript

**Status:** Aceito (substitui a UI em HTML/JavaScript sem framework)

## Contexto

A primeira versão da interface era HTML + SCSS + JavaScript (módulos ES) sem framework. Ela atendia ao fluxo, mas a tela tem estado de verdade: sessão, vários uploads simultâneos com progresso, lista com atualização automática, contadores e avisos. Controlar esse estado manipulando o DOM direto deixa o código difícil de evoluir e de testar. Também não havia testes automatizados da interface.

## Decisão

Reescrever a UI em **React 19 + TypeScript**, com build pelo **Vite**, mantendo os estilos em **SCSS**:

- **Componentes:** `AuthPage` (abas de login/cadastro), `Dashboard`, `Stats`, `UploadPanel`, `VideoList`/`VideoCard`, `Topbar`, `PasswordField`.
- **Estado compartilhado** via Context: `SessionProvider` (token + usuário no `localStorage`) e `ToastProvider` (avisos).
- **Hook `useVideos`:** listagem com atualização adaptativa (3s com vídeos em andamento, 20s sem, pausada em segundo plano) e detecção de mudança de status.
- **Cliente da API tipado** (`src/api`), espelhando o `docs/openapi.yaml`.
- **Testes** com Vitest + Testing Library (fluxo de login/cadastro, sessão expirada, envio com progresso, recusa de arquivos, download), incluídos no CI.
- O gateway compila a UI num estágio Node do Dockerfile e o nginx serve o resultado. Os assets com hash ficam em cache longo e o `index.html` é sempre revalidado.

## Alternativas descartadas

- **Manter JavaScript sem framework:** menos dependências, mas sem tipagem e com testes de UI difíceis.
- **Next.js:** renderização no servidor e rotas de API não são necessárias. A UI é uma SPA atrás do gateway, e o Next exigiria um servidor Node a mais em produção.
- **Vue/Angular:** atenderiam igualmente; React foi escolhido por ser o mais usado no mercado e pela familiaridade do time.

## Consequências

- A tela ganha tipagem de ponta a ponta com a API e testes automatizados no pipeline (job "Front-end (React)").
- Entram dependências npm e uma etapa de build (Node 22 no Docker e no CI).
- Em desenvolvimento, `npm run dev` sobe o Vite com recarga instantânea e encaminha `/auth` e `/api` para o gateway.
