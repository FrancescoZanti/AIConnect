# AIConnect - Reverse Proxy per AI Backends

Reverse proxy HTTPS in Go per l'instradamento intelligente di richieste AI verso backend multipli (Ollama, OpenAI) con autenticazione Active Directory integrata, load balancing dinamico basato su metriche di sistema e monitoraggio Prometheus.

## Caratteristiche Principali

- **Autenticazione Active Directory**: Bind LDAP con verifica appartenenza a gruppi autorizzati
- **Routing Dinamico**: Instradamento basato su path URL (`/ollama/*`, `/openai/*`)
- **Load Balancing Intelligente**: Selezione automatica del server Ollama meno carico basata su metriche CPU, RAM e GPU in tempo reale
- **HTTPS Automatico**: Certificati TLS gestiti automaticamente tramite LetsEncrypt (autocert)
- **Monitoraggio e Osservabilità**: Esposizione metriche Prometheus per monitoring centralizzato
- **Sicurezza Avanzata**: Gestione header HTTP, API key injection, audit logging con tracciamento utenti

## Architettura di Sistema

```text
Internet → HTTPS (443) → AIConnect → AD Auth → Load Balancer → Ollama Servers (N)
                                              ↘
                                                → OpenAI API
```

### Componenti Architetturali

- **HTTPS Server**: Server TLS con gestione automatica certificati LetsEncrypt, porta 443
- **Middleware Autenticazione**: LDAP bind su Active Directory con verifica attributo `memberOf`
- **Load Balancer**: Polling periodico (intervallo 30s) delle metriche backend con algoritmo weighted least-load
- **Proxy Handler**: Reverse proxy HTTP con modifica header e instradamento path-based
- **Metrics Server**: Endpoint Prometheus dedicato su porta 9090 per telemetria applicativa

## Prerequisiti

- Rocky Linux 8/9 (o RHEL-compatible)
- Go 1.21+ (per build)
- Accesso a Active Directory LDAP
- Server Ollama con endpoint metriche `GET /metrics` (JSON: `cpu_percent`, `ram_percent`)
- API key OpenAI
- Dominio configurato per LetsEncrypt

## Installazione

### 1. Build

```bash
git clone https://github.com/fzanti/aiconnect.git
cd aiconnect
go build -o aiconnect ./cmd/aiconnect
```

### 2. Configurazione

```bash
# Copia esempio config
sudo cp config.example.yaml /etc/aiconnect/config.yaml

# Modifica con i tuoi parametri
sudo nano /etc/aiconnect/config.yaml
```

**Parametri principali:**

```yaml
ad:
  ldap_url: "ldap://ad.example.com:389"
  bind_dn: "CN=service,OU=Users,DC=example,DC=com"
  bind_password: "password"
  base_dn: "DC=example,DC=com"
  allowed_groups:
    - "CN=AI-Users,OU=Groups,DC=example,DC=com"

backends:
  ollama_servers:
    - "http://ollama1:11434"
    - "http://ollama2:11434"
  openai_endpoint: "https://api.openai.com/v1"
  openai_api_key: "sk-..."

https:
  domain: "aiconnect.example.com"
  cache_dir: "/var/cache/aiconnect/autocert"
```

### 3. Installazione su Sistema

```bash
# Script automatico (richiede root)
sudo bash deployment/install.sh

# O manualmente:
sudo cp aiconnect /usr/local/bin/
sudo cp deployment/aiconnect.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now aiconnect
```

### 4. Firewall

```bash
# firewalld (Rocky Linux)
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --reload
```

## Utilizzo

### Avvio/Stop

```bash
# Avvia servizio
sudo systemctl start aiconnect

# Controlla stato
sudo systemctl status aiconnect

# Log in tempo reale
sudo journalctl -u aiconnect -f

# Stop
sudo systemctl stop aiconnect
```

### Test API

```bash
# Richiesta a backend Ollama
curl -X POST https://aiconnect.example.com/ollama/api/generate \
  -u "username:password" \
  -H "Content-Type: application/json" \
  -d '{"model":"llama2","prompt":"Hello!"}'

# Richiesta a OpenAI
curl -X POST https://aiconnect.example.com/openai/chat/completions \
  -u "username:password" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"Hello!"}]}'
```

### Metriche Prometheus

```bash
# Visualizza metriche
curl http://localhost:9090/metrics

# Metriche disponibili:
# - aiconnect_auth_attempts_total
# - aiconnect_auth_failures_total
# - aiconnect_proxy_requests_total
# - aiconnect_proxy_errors_total
# - aiconnect_proxy_latency_seconds
# - aiconnect_backend_health
```

## Sistema di Load Balancing

Il load balancer richiede che ogni server Ollama esponga un endpoint HTTP per la raccolta delle metriche di sistema:

```bash
# Endpoint richiesto su ogni server Ollama
GET http://ollama-server:11434/metrics

# Risposta attesa (formato JSON):
{
  "cpu_percent": 45.2,
  "ram_percent": 62.8,
  "gpu_count": 2,
  "gpu_avg_utilization_percent": 67.5,
  "gpu_avg_memory_percent": 82.3
}
```

**Algoritmo di Selezione:**

1. Polling periodico (30s) di tutti i server backend configurati
2. Calcolo peso di carico: `weight = cpu + ram + (gpu_util × 1.5) + (gpu_mem × 1.5)`
3. Selezione server con peso minore (least-loaded)
4. Fallback automatico a round-robin se endpoint metriche non risponde
5. Health checking: esclusione automatica server con oltre 3 errori consecutivi

Le metriche GPU hanno peso maggiorato (fattore 1.5x) in quanto l'inferenza di modelli AI è principalmente GPU-intensive e una GPU sovraccarica impatta significativamente le performance.

## Sicurezza e Conformità

### Gestione Header HTTP

- **Backend Ollama**: Rimozione completa header `Authorization` prima dell'inoltro per prevenire exposure credenziali
- **Backend OpenAI**: Sostituzione header con `Authorization: Bearer <api_key>` centralizzata da configurazione
- **Audit Trail**: Aggiunta automatica header `X-Forwarded-User` contenente username autenticato per tracciabilità
- **Preservazione Context**: Mantenimento header `X-Forwarded-*` standard per chain of trust

### Permessi Filesystem

```bash
/etc/aiconnect/config.yaml    # 600 (root:root) - Contiene credenziali sensibili
/var/cache/aiconnect/autocert # 700 (aiconnect:aiconnect) - Cache certificati TLS
/usr/local/bin/aiconnect      # 755 (root:root) - Binario eseguibile
```

### Hardening Systemd

Il service file include direttive di security hardening:

- `NoNewPrivileges=true`: Previene privilege escalation
- `PrivateTmp=true`: Filesystem /tmp isolato
- `ProtectSystem=strict`: Filesystem di sistema read-only
- `ProtectHome=true`: Home directory inaccessibili
- `AmbientCapabilities=CAP_NET_BIND_SERVICE`: Binding porte privilegiate senza root

## Risoluzione Problemi

### Diagnostica Autenticazione LDAP

Verifica connettività e bind LDAP verso Active Directory:

```bash
# Test bind LDAP con account di servizio
ldapsearch -H ldap://ad.example.com:389 \
  -D "CN=service,OU=Users,DC=example,DC=com" \
  -W -b "DC=example,DC=com" \
  "(sAMAccountName=username)"

# Verifica gruppi utente
ldapsearch -H ldap://ad.example.com:389 \
  -D "CN=service,OU=Users,DC=example,DC=com" \
  -W -b "DC=example,DC=com" \
  "(sAMAccountName=username)" memberOf
```

### Problemi Certificati TLS

Verifica stato certificati LetsEncrypt e cache:

```bash
# Ispezione cache certificati
ls -la /var/cache/aiconnect/autocert/

# Correzione permessi
sudo chown -R aiconnect:aiconnect /var/cache/aiconnect/
sudo chmod 700 /var/cache/aiconnect/autocert

# Verifica binding porta 80 per ACME challenge
sudo netstat -tlnp | grep :80
sudo ss -tlnp | grep :80
```

### Backend Ollama Non Disponibile

Diagnostica connettività e health checking backend:

```bash
# Test diretto endpoint metriche
curl -v http://ollama1:11434/metrics

# Analisi log errori backend
sudo journalctl -u aiconnect | grep "Server marcato non disponibile"

# Verifica network connectivity
ping ollama1.example.com
telnet ollama1.example.com 11434
```

## Configurazione Avanzata

### Logging e Diagnostica

Configurazione livelli log e formato output:

```yaml
logging:
  level: "debug"    # Opzioni: debug, info, warn, error
  format: "json"    # Opzioni: json, text
```

Livello `debug` produce output verboso utile per troubleshooting ma con overhead performance. Per produzione si raccomanda `info` o `warn`.

### Timeouts HTTP

Modifica timeouts server in `cmd/aiconnect/main.go` per adattare a latenze backend:

```go
httpsServer := &http.Server{
    ReadTimeout:  30 * time.Second,   // Timeout lettura richiesta client
    WriteTimeout: 30 * time.Second,   // Timeout scrittura risposta client
    IdleTimeout:  120 * time.Second,  // Timeout connessioni keep-alive
}
```

Per backend AI con inferenza lenta, incrementare `WriteTimeout` a 60-120s.

### Intervallo Health Check

Modifica intervallo polling metriche in configurazione:

```yaml
monitoring:
  health_check_interval: 15  # Polling più frequente (secondi)
```

Intervalli brevi (10-15s) migliorano reattività ma aumentano carico rete. Default 30s è bilanciato per la maggior parte degli scenari.

## Sviluppo

### Struttura Progetto

```text
aiconnect/
├── cmd/
│   └── aiconnect/         # Main application
│       └── main.go
├── internal/
│   ├── auth/              # LDAP authentication
│   ├── config/            # Configuration loading
│   ├── loadbalancer/      # Ollama load balancing
│   ├── metrics/           # Prometheus metrics
│   └── proxy/             # Reverse proxy handler
├── deployment/
│   ├── aiconnect.service  # Systemd service
│   └── install.sh         # Installation script
├── config.example.yaml    # Configuration example
├── go.mod
└── README.md
```

### Test Locale

```bash
# Build
go build -o aiconnect ./cmd/aiconnect

# Run con config custom
AICONNECT_CONFIG=./config.yaml ./aiconnect
```

## Versioning, Release e Packaging

Questo repository usa un file `VERSION` come sorgente della versione.

### Come pubblicare una nuova versione

1. Aggiorna il valore in `VERSION` (es. `0.0.2`).
2. Esegui commit e push su `main`.
3. Le GitHub Actions generano automaticamente:

    - una GitHub Release con tag `v<versione>` e asset RPM/SRPM per Fedora e RHEL 10
    - un’immagine container su GHCR con tag `v<versione>` e `latest`

### Workflow CI

- Release RPM: `.github/workflows/release.yml`
- Immagine container: `.github/workflows/container-image.yml`

### Changelog

Le modifiche sono tracciate in `CHANGELOG.md`.

## Licenza

MIT License

## Supporto

Per problemi o domande, aprire una issue su GitHub.
