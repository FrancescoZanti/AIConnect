# AIConnect - Guida Docker e Podman

Questa guida descrive come eseguire AIConnect utilizzando container con Docker o Podman.

## Indice

- [Prerequisiti](#prerequisiti)
- [Build dell'Immagine](#build-dellimmagine)
- [Esecuzione con Docker](#esecuzione-con-docker)
- [Esecuzione con Podman](#esecuzione-con-podman)
- [Docker Compose / Podman Compose](#docker-compose--podman-compose)
- [Configurazione](#configurazione)
- [Volumi e Persistenza](#volumi-e-persistenza)
- [Networking](#networking)
- [Troubleshooting](#troubleshooting)

## Prerequisiti

### Docker
```bash
# Rocky Linux / RHEL / CentOS
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
```

### Podman
```bash
# Rocky Linux / RHEL / CentOS (Podman è preinstallato)
sudo dnf install -y podman podman-compose
```

## Build dell'Immagine

AIConnect utilizza un `Containerfile` multi-stage ottimizzato per produzione.

### Con Podman (raccomandato su Rocky Linux)
```bash
podman build -t aiconnect:latest -f Containerfile .
```

### Con Docker
```bash
docker build -t aiconnect:latest -f Containerfile .
```

### Usando Make
```bash
# Podman (default)
make container-build

# Docker
make docker-build
```

## Esecuzione con Docker

### Run Base
```bash
docker run -d \
  --name aiconnect \
  -p 443:443 \
  -p 9090:9090 \
  -v ./config.yaml:/etc/aiconnect/config.yaml:ro \
  -v aiconnect-certs:/var/cache/aiconnect \
  aiconnect:latest
```

### Con Variabili d'Ambiente
```bash
docker run -d \
  --name aiconnect \
  -p 443:443 \
  -p 9090:9090 \
  -e AICONNECT_CONFIG=/etc/aiconnect/config.yaml \
  -v ./config.yaml:/etc/aiconnect/config.yaml:ro \
  -v aiconnect-certs:/var/cache/aiconnect \
  aiconnect:latest
```

### Comandi Utili
```bash
# Visualizza log
docker logs -f aiconnect

# Ferma container
docker stop aiconnect

# Rimuovi container
docker rm aiconnect

# Accedi al container
docker exec -it aiconnect /bin/sh
```

## Esecuzione con Podman

### Run Base
```bash
podman run -d \
  --name aiconnect \
  -p 443:443 \
  -p 9090:9090 \
  -v ./config.yaml:/etc/aiconnect/config.yaml:ro \
  -v aiconnect-certs:/var/cache/aiconnect \
  aiconnect:latest
```

### Con SELinux (Rocky Linux)
Su sistemi con SELinux abilitato, usa il flag `:Z` per i volumi:
```bash
podman run -d \
  --name aiconnect \
  -p 443:443 \
  -p 9090:9090 \
  -v ./config.yaml:/etc/aiconnect/config.yaml:ro,Z \
  -v aiconnect-certs:/var/cache/aiconnect:Z \
  aiconnect:latest
```

### Rootless Podman
Podman supporta l'esecuzione senza root. Per binding su porte privilegiate (<1024):
```bash
# Opzione 1: Usa porte non privilegiate
podman run -d \
  --name aiconnect \
  -p 8443:443 \
  -p 9090:9090 \
  -v ./config.yaml:/etc/aiconnect/config.yaml:ro \
  aiconnect:latest

# Opzione 2: Abilita binding porte privilegiate per utente
sudo sysctl -w net.ipv4.ip_unprivileged_port_start=443
```

### Comandi Utili
```bash
# Visualizza log
podman logs -f aiconnect

# Ferma container
podman stop aiconnect

# Rimuovi container
podman rm aiconnect

# Genera systemd unit per autostart
podman generate systemd --name aiconnect --files --new
sudo mv container-aiconnect.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable container-aiconnect
```

## Docker Compose / Podman Compose

Nella root del progetto è disponibile un file `compose.yaml` compatibile con entrambi.

### Con Docker Compose
```bash
# Avvia
docker compose up -d

# Log
docker compose logs -f

# Ferma
docker compose down

# Ferma e rimuovi volumi
docker compose down -v
```

### Con Podman Compose
```bash
# Avvia
podman-compose up -d

# Log
podman-compose logs -f

# Ferma
podman-compose down

# Ferma e rimuovi volumi
podman-compose down -v
```

### Esempio compose.yaml

```yaml
services:
  aiconnect:
    build:
      context: .
      dockerfile: Containerfile
    image: aiconnect:latest
    container_name: aiconnect
    ports:
      - "443:443"
      - "9090:9090"
    volumes:
      - ./config.yaml:/etc/aiconnect/config.yaml:ro
      - aiconnect-certs:/var/cache/aiconnect
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:9090/metrics"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 5s

volumes:
  aiconnect-certs:
```

## Configurazione

### File di Configurazione

Crea un file `config.yaml` basato su `config.example.yaml`:

```bash
cp config.example.yaml config.yaml
nano config.yaml
```

### Configurazione Minima

```yaml
ad:
  enabled: false  # Disabilita autenticazione AD per test

backends:
  ollama_servers:
    - "http://host.docker.internal:11434"
  openai_endpoint: "https://api.openai.com/v1"
  openai_api_key: "sk-..."

https:
  domain: "localhost"
  cache_dir: "/var/cache/aiconnect/autocert"
  port: 443

monitoring:
  metrics_port: 9090
```

### Accesso all'Host

Per accedere a servizi sull'host dal container:

**Docker:**
```yaml
backends:
  ollama_servers:
    - "http://host.docker.internal:11434"
```

**Podman:**
```yaml
backends:
  ollama_servers:
    - "http://host.containers.internal:11434"
```

## Volumi e Persistenza

### Volumi Necessari

| Path nel Container | Descrizione | Persistenza |
|-------------------|-------------|-------------|
| `/etc/aiconnect/config.yaml` | File di configurazione | Obbligatorio |
| `/var/cache/aiconnect` | Cache certificati TLS | Raccomandato |

### Backup Certificati

I certificati LetsEncrypt vengono salvati in `/var/cache/aiconnect/autocert`. Per backup:

```bash
# Docker
docker cp aiconnect:/var/cache/aiconnect ./backup-certs

# Podman
podman cp aiconnect:/var/cache/aiconnect ./backup-certs
```

## Networking

### Network Bridge Dedicato

Per ambienti multi-container:

```bash
# Docker
docker network create aiconnect-network
docker run -d --network aiconnect-network --name aiconnect ...

# Podman
podman network create aiconnect-network
podman run -d --network aiconnect-network --name aiconnect ...
```

### Connessione a Backend Ollama in Container

Se Ollama gira in un container separato:

```bash
# Crea network condiviso
podman network create ai-network

# Avvia Ollama
podman run -d --network ai-network --name ollama ollama/ollama

# Avvia AIConnect
podman run -d --network ai-network --name aiconnect \
  -v ./config.yaml:/etc/aiconnect/config.yaml:ro \
  aiconnect:latest
```

Nella configurazione:
```yaml
backends:
  ollama_servers:
    - "http://ollama:11434"
```

## Troubleshooting

### Container Non Parte

```bash
# Verifica log di avvio
docker logs aiconnect
podman logs aiconnect

# Verifica file config esiste e ha permessi corretti
ls -la ./config.yaml
```

### Errore "Permission Denied" su Volumi (Podman/SELinux)

```bash
# Usa flag :Z per contesti SELinux
podman run -v ./config.yaml:/etc/aiconnect/config.yaml:ro,Z ...

# O disabilita SELinux temporaneamente (non raccomandato in produzione)
sudo setenforce 0
```

### Porta 443 Già in Uso

```bash
# Verifica processi sulla porta
sudo ss -tlnp | grep :443

# Usa porta alternativa
docker run -p 8443:443 ...
```

### Connessione a Backend Fallisce

```bash
# Verifica DNS/rete dal container
docker exec aiconnect ping ollama1.example.com
docker exec aiconnect wget -O- http://ollama1.example.com:11434/metrics

# Verifica firewall host
sudo firewall-cmd --list-all
```

### Health Check Fallisce

```bash
# Verifica manualmente endpoint metriche
docker exec aiconnect wget -O- http://localhost:9090/metrics

# Controlla configurazione monitoring
grep -A2 "monitoring:" config.yaml
```

## Immagini Pre-built

Le immagini sono disponibili su GitHub Container Registry:

```bash
# Pull ultima versione
docker pull ghcr.io/francescozanti/aiconnect:latest
podman pull ghcr.io/francescozanti/aiconnect:latest

# Pull versione specifica
docker pull ghcr.io/francescozanti/aiconnect:v1.0.0
```

## Risorse

- [Containerfile](../Containerfile) - Definizione build container
- [compose.yaml](../compose.yaml) - File compose per orchestrazione
- [config.example.yaml](../config.example.yaml) - Configurazione di esempio
- [README.md](../README.md) - Documentazione principale
