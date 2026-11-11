# syntax=docker/dockerfile:1
# Dockerfile único para todos os microsserviços (monorepo).
#   docker build --build-arg SERVICE=auth-service --target service -t fiapx/auth-service .
#   docker build --build-arg SERVICE=processor-worker --target worker -t fiapx/processor-worker .

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG SERVICE
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    test -n "$SERVICE" && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${SERVICE}

# Serviços HTTP/consumidores sem dependências nativas: imagem mínima, sem shell, usuário não-root.
FROM gcr.io/distroless/static-debian12:nonroot AS service
COPY --from=build /out/app /app
# UID numérico: exigido pelo runAsNonRoot do Kubernetes.
USER 65532:65532
ENTRYPOINT ["/app"]

# Worker precisa do ffmpeg.
FROM alpine:3.22 AS worker
RUN apk add --no-cache ffmpeg ca-certificates && adduser -D -u 10001 app && mkdir -p /work && chown app /work
COPY --from=build /out/app /app
USER app
ENV WORK_DIR=/work
ENTRYPOINT ["/app"]
