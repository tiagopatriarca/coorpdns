# ==========================================
# CoorpDNS - Multi-Stage Dockerfile
# ==========================================

# Estágio 1: Build da aplicação Go
FROM golang:1.22-alpine AS builder

# Dependências necessárias para compilar CGO (SQLite)
RUN apk add --no-cache gcc musl-dev git

WORKDIR /build

# Copia todo o código fonte
COPY . .

# Garante dependências resolvidas e go.sum íntegro
RUN go mod tidy

# Compilação otimizada do binário CoorpDNS
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /build/coorpdns ./cmd/coorpdns

# Compilação cruzada do agente oficial para Windows (com autocura de DNS e resiliência)
RUN mkdir -p /build/downloads && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o /build/downloads/coorpdns-agent.exe ./cmd/agent

# Estágio 2: Imagem final mínima e segura
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Cria diretórios de persistência
RUN mkdir -p /app/data /app/config /app/downloads

# Copia binários do builder
COPY --from=builder /build/coorpdns /app/coorpdns
COPY --from=builder /build/downloads/coorpdns-agent.exe /app/downloads/coorpdns-agent.exe

# Expõe portas DNS (53 UDP/TCP) e Painel Web (8080)
EXPOSE 53/udp 53/tcp 8080/tcp

VOLUME ["/app/data", "/app/config"]

# Healthcheck para monitoramento em produção
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -q -O - http://127.0.0.1:8080/api/v1/status || exit 1

ENTRYPOINT ["/app/coorpdns"]
CMD ["-config", "/app/config/config.yaml"]
