# ADR 0013 — Clean Architecture em Cada Microsserviço

**Status:** Aceito (substitui a organização "um pacote por serviço" descrita na ADR 0001)

## Contexto

Na primeira versão, cada serviço era um único pacote Go (`service.go`, `handler.go`, `postgres.go`...). Os casos de uso já dependiam só de interfaces, mas as camadas estavam separadas apenas por arquivo: nada impedia que um caso de uso passasse a importar o Postgres ou o RabbitMQ. A separação também não era visível para quem lia a estrutura de pastas.

Clean Architecture é conteúdo do curso e foi usada nas fases anteriores (`Domain`, `Application`, `Infrastructure`, `Api` no `soat-tech-challenge`).

## Decisão

Cada microsserviço é organizado nas camadas da Clean Architecture, com uma pasta por camada:

```
internal/<serviço>/
  domain/      Entities: entidades e regras de negócio puras (sem dependências)
  usecase/     Use Cases: orquestram o domínio; declaram as portas (interfaces) que precisam
  adapter/     Interface Adapters + Frameworks & Drivers: HTTP, mensageria, Postgres, Redis,
               bcrypt, JWT, ffmpeg, ZIP, SMTP — implementam as portas ou chamam os casos de uso
cmd/<serviço>/main.go   raiz de composição: instancia os adaptadores e injeta nos casos de uso
```

Componentes compartilhados:
- `internal/platform`: drivers reutilizados por vários serviços (cliente S3, conexão RabbitMQ, pool Postgres, middlewares HTTP, JWT, config, logs);
- `internal/events`: contrato das mensagens entre serviços (fronteira da aplicação).

**Regra de dependência:** as dependências apontam sempre para dentro.
- `domain` não importa nada do projeto nem bibliotecas externas;
- `usecase` importa só o próprio `domain`, `internal/events` e utilitários puros (ex.: `uuid`);
- `adapter` importa `usecase` e `domain`;
- inversão de dependência: o caso de uso declara a interface (ex.: `UserRepository`, `FileStorage`, `StatusPublisher`) e o adaptador a implementa.

A regra é **verificada automaticamente** por [internal/archtest](../../internal/archtest/arch_test.go), que falha o CI se `domain` ou `usecase` importarem adaptadores, `platform` ou bibliotecas de infraestrutura (pgx, redis, amqp, S3, Prometheus, JWT, bcrypt, `net/http`).

Consequências práticas no código:
- regras que estavam no caso de uso foram para o domínio (`Video.Apply` para a máquina de estados, `RetryPolicy` para a decisão de retentativa, `NewRegistration` para a validação de cadastro);
- bcrypt e JWT viraram adaptadores das portas `PasswordHasher` e `TokenIssuer`;
- métricas Prometheus saíram dos casos de uso e foram para os adaptadores (HTTP e consumidores), que leem o resultado retornado pelo caso de uso;
- o worker ganhou as portas `FrameExtractor` (ffmpeg), `Archiver` (ZIP) e `StatusPublisher` (RabbitMQ).

## Alternativas descartadas

- **Manter um pacote por serviço**: menos pastas, mas a separação depende de disciplina e não aparece na estrutura.
- **Camadas globais (`internal/domain`, `internal/usecase` com todos os serviços juntos)**: mistura contextos de serviços diferentes e acopla os microsserviços.
- **Um módulo Go por camada**: isolamento ainda mais forte, mas complexidade de versionamento desproporcional ao projeto.

## Consequências

- Os casos de uso são testados com fakes das portas, sem banco, broker nem rede (`go test ./...` roda em segundos).
- Trocar tecnologia (ex.: outro broker, outro storage, SES no lugar de SMTP) é escrever um novo adaptador, sem tocar no domínio nem nos casos de uso.
- Mais pacotes e mais arquivos; a navegação é guiada pela mesma estrutura nos quatro serviços.
