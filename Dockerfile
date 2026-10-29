# ==========================================
# CoorpDNS - Multi-Stage Hardened Dockerfile
# ==========================================

# Estágio 1: Build da aplicação Go
FROM golang:1.26-alpine AS builder

# Dependências necessárias para compilar CGO (SQLite)
RUN apk add --no-cache gcc musl-dev git

WORKDIR /build

# Copia todo o código fonte
COPY . .

# Garante dependências resolvidas e verifica integridade criptográfica do go.sum
RUN go mod download && go mod verify

# Compilação otimizada do binário CoorpDNS
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /build/coorpdns ./cmd/coorpdns

# Compilação cruzada dos agentes oficiais para Windows e Linux (com autocura de DNS e resiliência)
RUN mkdir -p /build/downloads && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o /build/downloads/coorpdns-agent.exe ./cmd/agent && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /build/downloads/coorpdns-agent-linux ./cmd/agent

# Estágio 2: Imagem final mínima e endurecida (Non-Root + Least Privilege)
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata libcap && \
    addgroup -S -g 10001 coorpdns && \
    adduser -S -D -u 10001 -G coorpdns coorpdns

WORKDIR /app

# Cria diretórios de persistência com permissões restritas ao usuário não-root
RUN mkdir -p /app/data /app/config /app/downloads

# Copia binários do builder
COPY --from=builder /build/coorpdns /app/coorpdns
COPY --from=builder /build/downloads/coorpdns-agent.exe /app/downloads/coorpdns-agent.exe
COPY --from=builder /build/downloads/coorpdns-agent-linux /app/downloads/coorpdns-agent-linux

# Concede exclusivamente a capability para bind na porta 53 sem precisar executar como root
RUN setcap 'cap_net_bind_service=+ep' /app/coorpdns && \
    chown -R coorpdns:coorpdns /app && \
    chmod 0750 /app/data /app/config

USER coorpdns

# Expõe portas DNS (53 UDP/TCP) e Painel Web (8080)
EXPOSE 53/udp 53/tcp 8080/tcp

VOLUME ["/app/data", "/app/config"]

# Healthcheck para monitoramento em produção
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -q -O - http://127.0.0.1:8080/api/v1/status || exit 1

ENTRYPOINT ["/app/coorpdns"]
CMD ["-config", "/app/config/config.yaml"]
