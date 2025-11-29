# Containerfile per AIConnect
# Compatibile con Podman e Docker
#
# Build:
#   podman build -t aiconnect .
#   docker build -t aiconnect -f Containerfile .
#
# Run:
#   podman run -d -p 443:443 -p 9090:9090 -v /path/to/config.yaml:/etc/aiconnect/config.yaml:ro aiconnect
#   docker run -d -p 443:443 -p 9090:9090 -v /path/to/config.yaml:/etc/aiconnect/config.yaml:ro aiconnect

# ===== Stage 1: Build =====
FROM docker.io/library/golang:1.21-alpine AS builder

# Installa dipendenze di build
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copia go.mod e go.sum per cache delle dipendenze
COPY go.mod go.sum ./
RUN go mod download

# Copia il codice sorgente
COPY . .

# Build del binario statico
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o aiconnect \
    ./cmd/aiconnect

# ===== Stage 2: Runtime =====
FROM docker.io/library/alpine:3.19

# Labels OCI standard
LABEL org.opencontainers.image.title="AIConnect"
LABEL org.opencontainers.image.description="Reverse proxy HTTPS per AI backends con autenticazione AD"
LABEL org.opencontainers.image.source="https://github.com/fzanti/aiconnect"
LABEL org.opencontainers.image.vendor="AIConnect"

# Installa certificati CA per connessioni TLS
RUN apk add --no-cache ca-certificates tzdata

# Crea utente non-root per sicurezza
RUN addgroup -g 1000 aiconnect && \
    adduser -u 1000 -G aiconnect -s /sbin/nologin -D aiconnect

# Crea directories necessarie
RUN mkdir -p /etc/aiconnect /var/cache/aiconnect/autocert && \
    chown -R aiconnect:aiconnect /var/cache/aiconnect

# Copia binario dal builder
COPY --from=builder /build/aiconnect /usr/local/bin/aiconnect

# Copia config di esempio (opzionale, può essere sovrascritto con volume)
COPY --chown=aiconnect:aiconnect config.example.yaml /etc/aiconnect/config.example.yaml

# Imposta variabili di ambiente
ENV AICONNECT_CONFIG=/etc/aiconnect/config.yaml

# Esponi porte
# 443: HTTPS proxy
# 9090: Prometheus metrics
EXPOSE 443 9090

# Imposta utente non-root
USER aiconnect

# Volume per configurazione e cache certificati
VOLUME ["/etc/aiconnect", "/var/cache/aiconnect"]

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9090/metrics || exit 1

# Comando di avvio
ENTRYPOINT ["/usr/local/bin/aiconnect"]
