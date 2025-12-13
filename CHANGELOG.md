# Changelog

<<<<<<< HEAD
Tutte le modifiche rilevanti a questo progetto vengono documentate in questo file.

Il formato è basato su [Keep a Changelog](https://keepachangelog.com/it/1.1.0/) e questo progetto segue il versioning SemVer.

## [Unreleased]

## [0.0.1] - 2025-12-13

### Added

- Release automatica via GitHub Actions basata sul file `VERSION`.
- Build e pubblicazione di pacchetti RPM (Fedora e RHEL 10) come asset della GitHub Release.
- Build e pubblicazione dell’immagine container su GHCR con tag `v<versione>` e `latest`.
=======
Tutte le modifiche significative al progetto AIConnect sono documentate in questo file.

Il formato è basato su [Keep a Changelog](https://keepachangelog.com/it/1.0.0/),
e questo progetto aderisce al [Semantic Versioning](https://semver.org/lang/it/).

## [Unreleased]

### Aggiunto
- Documentazione per Docker e Podman (`docs/docker.md`)
- File `compose.yaml` compatibile con Docker e Podman
- Questo file CHANGELOG.md

## [1.0.0] - 2025-12-02

### Aggiunto
- Autenticazione LDAP/Active Directory opzionale (può essere disabilitata via configurazione)
- Supporto per path pubblici configurabili (endpoint accessibili senza autenticazione)
- Backend vLLM oltre a Ollama e OpenAI
- Supporto mDNS per auto-discovery di backend LLM
- Containerfile multi-stage per build ottimizzata (compatibile con Docker e Podman)
- Makefile con target per container build/run
- Workflow GitHub Actions per build automatica container image
- Pubblicazione automatica su GitHub Container Registry (ghcr.io)

### Caratteristiche principali
- **Reverse Proxy HTTPS**: Certificati TLS automatici via LetsEncrypt (autocert)
- **Autenticazione AD**: Bind LDAP con verifica appartenenza a gruppi autorizzati
- **Routing Dinamico**: Instradamento basato su path URL (`/ollama/*`, `/openai/*`, `/vllm/*`)
- **Load Balancing Intelligente**: Selezione automatica del server meno carico basata su metriche CPU, RAM e GPU
- **Monitoraggio Prometheus**: Esposizione metriche su porta 9090
- **Sicurezza**: Gestione header HTTP, API key injection, audit logging
- **mDNS Discovery**: Auto-discovery di backend LLM nella rete locale

### Struttura Progetto
```
aiconnect/
├── cmd/aiconnect/           # Main application
├── internal/
│   ├── auth/                # Autenticazione LDAP
│   ├── config/              # Caricamento configurazione
│   ├── loadbalancer/        # Load balancing Ollama/vLLM
│   ├── mdns/                # mDNS advertisement e discovery
│   ├── metrics/             # Metriche Prometheus
│   ├── proxy/               # Reverse proxy handler
│   └── registry/            # Registry backend dinamico
├── deployment/              # Systemd service e script installazione
├── tools/ollama-metrics/    # Server metriche per backend Ollama
├── Containerfile            # Build container image
├── Makefile                 # Build automation
└── config.example.yaml      # Configurazione di esempio
```

### Dipendenze
- Go 1.21+
- `github.com/go-ldap/ldap/v3` - Client LDAP
- `github.com/prometheus/client_golang` - Metriche Prometheus
- `github.com/grandcat/zeroconf` - mDNS
- `github.com/sirupsen/logrus` - Logging strutturato
- `gopkg.in/yaml.v3` - Parsing configurazione YAML

## Tipi di Modifiche

- `Aggiunto` per nuove funzionalità
- `Modificato` per modifiche a funzionalità esistenti
- `Deprecato` per funzionalità che saranno rimosse nelle prossime release
- `Rimosso` per funzionalità rimosse
- `Corretto` per bug fix
- `Sicurezza` per vulnerabilità corrette
>>>>>>> e75cb204268e9d96975a557aa6c104ec63d09d6f
