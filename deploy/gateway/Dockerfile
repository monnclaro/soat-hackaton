# Gateway: nginx + UI web. Build a partir da raiz do repositório:
#   docker build -f deploy/gateway/Dockerfile -t fiapx/gateway .

# 1) Compila a UI (SCSS → CSS, cópia de HTML/JS para dist/).
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/build.mjs ./
COPY web/src ./src
RUN npm run build

# 2) nginx servindo a UI e fazendo proxy para os serviços.
FROM nginx:1.30-alpine
ENV NGINX_ENTRYPOINT_LOCAL_RESOLVERS=1 \
    AUTH_UPSTREAM=auth-service:8081 \
    VIDEO_UPSTREAM=video-service:8082
RUN rm -f /etc/nginx/conf.d/default.conf
COPY deploy/gateway/default.conf.template /etc/nginx/templates/default.conf.template
COPY --from=web /web/dist/ /usr/share/nginx/html/
EXPOSE 8080
