# Gateway: nginx + UI web. Build a partir da raiz do repositório:
#   docker build -f deploy/gateway/Dockerfile -t fiapx/gateway .
FROM nginx:1.27-alpine
ENV NGINX_ENTRYPOINT_LOCAL_RESOLVERS=1 \
    AUTH_UPSTREAM=auth-service:8081 \
    VIDEO_UPSTREAM=video-service:8082
RUN rm -f /etc/nginx/conf.d/default.conf
COPY deploy/gateway/default.conf.template /etc/nginx/templates/default.conf.template
COPY web/ /usr/share/nginx/html/
EXPOSE 8080
