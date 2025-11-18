# AIConnect - Reverse Proxy per AI Backends

## Architettura
Reverse proxy HTTPS in Go per instradare richieste AI a backend multipli con autenticazione AD e load balancing.

**Componenti principali**:
- **HTTPS Server**: TLS con LetsEncrypt autocert
- **Autenticazione**: LDAP bind su Active Directory con controllo gruppi
- **Routing**: Path-based (`/ollama/*`, `/openai/*`)
- **Load Balancing**: Selezione dinamica backend Ollama basata su metriche CPU/RAM
- **Monitoraggio**: Prometheus `/metrics` endpoint

## Stack Tecnologico
```go
// Core dependencies
"net/http/httputil"              // Reverse proxy
"golang.org/x/crypto/acme/autocert"  // LetsEncrypt
"github.com/go-ldap/ldap/v3"     // Active Directory
"github.com/prometheus/client_golang/prometheus" // Metriche
"github.com/sirupsen/logrus"     // Logging strutturato
"github.com/shirou/gopsutil"     // Metriche sistema
```

## Struttura Config (YAML/JSON)
```go
type Config struct {
    AD struct {
        LDAPURL       string   // es. "ldap://ad.example.com:389"
        BindDN        string   // es. "CN=service,OU=Users,DC=example,DC=com"
        BindPassword  string
        AllowedGroups []string // Gruppi AD autorizzati
    }
    Backends struct {
        OllamaServers  []string // ["http://ollama1:11434", "http://ollama2:11434"]
        OpenAIEndpoint string   // "https://api.openai.com/v1"
        OpenAIAPIKey   string   // Chiave condivisa per tutti gli utenti
    }
    HTTPS struct {
        Domain   string // Per LetsEncrypt
        CacheDir string // Dove salvare certificati
    }
    Monitoring struct {
        HealthCheckInterval int // Secondi tra health checks (default: 30)
        MetricsPort         int // Porta per endpoint Prometheus (default: 9090)
    }
}
```
Config location: `/etc/aiconnect/config.yaml`

## Pattern Implementativi

### Middleware Autenticazione
- Estrarre credenziali da `Authorization: Basic` header
- LDAP bind con username/password utente
- Query `memberOf` per recuperare gruppi AD
- Controllare intersezione con `AllowedGroups`
- Risposta 403 se non autorizzato

### Load Balancing Ollama
- Polling ogni 30s verso API REST dedicata su ogni server Ollama per metriche CPU/RAM/GPU
- Endpoint metriche: `GET http://ollama-server:11434/metrics` (ritorna JSON con cpu_percent, ram_percent, gpu_*)
- Algoritmo weighted least-load: `weight = cpu + ram + (gpu_util * 1.5) + (gpu_mem * 1.5)`
- GPU ha peso maggiore (1.5x) perché più critica per inferenza AI
- Fallback a round-robin se API metriche non risponde
- Health check ogni 30s: escludere server non raggiungibili o con errori consecutivi (>3)

### Proxying Sicuro
- Usare `httputil.NewSingleHostReverseProxy` per ogni backend
- Modificare header: rimuovere `Authorization` prima di inoltrare a Ollama
- Per OpenAI: sostituire header con `Authorization: Bearer <OpenAIAPIKey>` (chiave condivisa da config)
- Preservare `X-Forwarded-*` headers per tracciabilità
- Aggiungere `X-Forwarded-User` con username autenticato per audit

## Deployment su Rocky Linux
```bash
# Build
go build -o aiconnect main.go

# Installazione systemd
sudo cp aiconnect /usr/local/bin/
sudo cp aiconnect.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now aiconnect

# Firewall (firewalld)
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

## Logging e Debugging
- Usare `logrus.WithFields` per contesto (user, backend, latency)
- Log level configurabile via config
- Formato JSON per parsing automatico
- Metriche Prometheus: contatori per autenticazioni fallite, latenza per backend, errori proxy

## Riferimenti
- Spec completa: `AiConnect.md`
- Target OS: Rocky Linux (RHEL-compatible)
