<img src="https://r2cdn.perplexity.ai/pplx-full-logo-primary-dark%402x.png" style="height:64px;margin-right:32px"/>

# definisci l'analisi progettuale e il prompt per permettere a GitHub copilot di iniziare questo lavoro

non tralasciare nulla

Ecco un’analisi progettuale dettagliata e il prompt completo per GitHub Copilot per avviare lo sviluppo di un reverse proxy multi-backend in Go, con gestione autenticazione AD, HTTPS, firewall, load balancing e monitoraggio, compatibile con Rocky Linux.

***

## Analisi progettuale dettagliata

### Scopo

Sviluppare un software reverse proxy che:

- Riceve richieste HTTPS dagli applicativi client
- Autentica e autorizza utenti contro Active Directory, basando l’accesso su appartenenza a gruppi AD
- Instrada richieste a backend multipli: Ollama (su distincti server Rocky Linux) e API OpenAI
- Gestisce load balancing dinamico basato sul carico delle macchine Ollama e parametri configurabili per altri backend
- Offre logging strutturato, esportazione metriche per monitoraggio (Prometheus)
- Permette gestione e configurazione firewall integrata tramite script esterni (iptables/firewalld)
- Funziona come pacchetto standalone da eseguire su VM Rocky Linux


### Architettura

- **Server HTTPS**: connessione TLS terminata localmente, gestione certificati con LetsEncrypt (autocert)
- **Middleware autenticazione**:
    - Bind LDAP su AD per autenticare utente via username/password
    - Recupero gruppo di appartenenza su AD tramite query LDAP
    - Autorizzazione in base a lista gruppi configurata
- **Routing richieste**:
    - Smistamento richieste basato su path URL, es. `/ollama/*` e `/openai/*`
    - Possibilità di estendere per nuovi backend
- **Load balancing backend Ollama**:
    - Raccolta metriche carico (CPU, RAM) via REST API o agent custom
    - Selezione server meno carico o round-robin
- **Proxy HTTP**:
    - Uso di `httputil.ReverseProxy` per inoltro richieste e risposta trasparente
- **Logging e monitoraggio**:
    - Logging con `logrus` o `zap` con livelli e contesti
    - Endpoint `/metrics` per Prometheus
- **Configurazione e deploy**:
    - Configurazione via file YAML/JSON
    - Script di configurazione firewall shell esterni
    - Systemd unit per gestione ciclo vita


### Tecnologie principali

| Funzionalità | Tecnologie |
| :-- | :-- |
| Reverse proxy HTTP | Go `net/http/httputil` |
| HTTPS e TLS | Go `autocert` LetsEncrypt |
| LDAP Active Directory | `github.com/go-ldap/ldap/v3` |
| Load balancing | Algoritmi custom in Go |
| Monitoraggio | `gopsutil`, `prometheus/client_golang` |
| Logging | `logrus` o `zap` |
| Firewall management | Script Bash `firewalld` o `iptables` |


***

## Prompt completo per GitHub Copilot

```go
// Reverse Proxy multi-backend con autenticazione Active Directory e load balancing
// Funzionalità principali:
// - Termina HTTPS con certificati LetsEncrypt (autocert)
// - Middleware di autenticazione con bind LDAP su Active Directory
// - Verifica appartenenza a gruppi AD autorizzati per accesso
// - Routing richieste REST API tramite path:
//     /ollama/ per gruppo server Ollama con load balancing dinamico basato su carico CPU/RAM
//     /openai/ per richieste inoltrate all'API OpenAI con meccanismi di sicurezza
// - Logging strutturato e dettagliato
// - Esportazione metriche Prometheus su /metrics
// - Configurazione tramite file YAML/JSON esterno
// - Script o meccanismi esterni per configurazione firewall (iptables/firewalld)
// - Gestione errori e fallback sicuri
package main

import (
    "net/http"
    "net/http/httputil"
    "golang.org/x/crypto/acme/autocert"
    ldap "github.com/go-ldap/ldap/v3"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/sirupsen/logrus"
    // ... altre importazioni necessarie
)

// Config struct per backend, AD, certificati, autenticazione
type Config struct {
    AD struct {
        LDAPURL      string
        BindDN       string
        BindPassword string
        AllowedGroups []string
    }
    Backends struct {
        OllamaServers []string
        OpenAIEndpoint string
    }
    HTTPS struct {
        Domain string
        CacheDir string
    }
    Auth struct {
        Type string // "Basic" o "Token"
    }
    // ... altri parametri configurabili
}

// Funzione middleware per autenticazione LDAP con controllo gruppi AD
func LDAPAuthMiddleware(next http.Handler, cfg Config) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Estrazione credenziali da header
        // Bind LDAP su AD
        // Query gruppi utente con filtro memberOf
        // Controllo appartenenza gruppi autorizzati
        // Se autorizzato, invoke next.ServeHTTP
        // Altrimenti risposta 403 forbidden
    })
}

// Funzione per load balancing e selezione backend Ollama meno carico
func SelectOllamaBackend() string {
    // Raccogli metriche carico CPU e RAM e seleziona il migliore
    return "http://server1:11434"
}

// Handler che inoltra richieste verso Ollama o OpenAI in base a path
func ProxyHandler(cfg Config) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if strings.HasPrefix(r.URL.Path, "/ollama/") {
            // Inoltra richiesta al server Ollama scelto dal load balancer
        } else if strings.HasPrefix(r.URL.Path, "/openai/") {
            // Inoltra richiesta all'endpoint OpenAI con eventuale modifica header per autentificazione
        } else {
            http.NotFound(w, r)
        }
    })
}

func main() {
    // Carica configurazione da file
    // Setup logger
    // Setup autocert manager per TLS
    // Configura HTTP server con middleware LDAPAuthMiddleware e ProxyHandler
    // Configura endpoint /metrics per Prometheus
    // Avvia server HTTPS
}
```


***

### Nota per Copilot

Si prega di completare:

- Implementazione middleware LDAPBind, query memberOf e controllo gruppi
- Logica raccolta metriche di sistema e load balancing
- Proxying corretto e sicuro verso backend Ollama e OpenAI
- Gestione errori, configurazione dinamica e sicurezza su token/headers

***

Con questa analisi e prompt, GitHub Copilot potrà generare una base solida e completa per il progetto, facilitando lo sviluppo modulare e la futura estensione.

Se vuoi, posso aiutarti a scrivere parti di codice più dettagliate o boilerplate per iniziare!

