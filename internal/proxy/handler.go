package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/fzanti/aiconnect/internal/config"
	"github.com/fzanti/aiconnect/internal/loadbalancer"
	"github.com/fzanti/aiconnect/internal/metrics"
	"github.com/sirupsen/logrus"
)

// Handler gestisce il routing e il proxying delle richieste
type Handler struct {
	cfg            *config.Config
	log            *logrus.Logger
	ollamaLB       *loadbalancer.OllamaLoadBalancer
	vllmLB         *loadbalancer.VLLMLoadBalancer
	openaiProxy    *httputil.ReverseProxy
	metricsManager *metrics.Manager
}

// NewHandler crea un nuovo proxy handler
func NewHandler(cfg *config.Config, log *logrus.Logger, ollamaLB *loadbalancer.OllamaLoadBalancer, vllmLB *loadbalancer.VLLMLoadBalancer, mm *metrics.Manager) *Handler {
	// Configura proxy per OpenAI
	openaiURL, _ := url.Parse(cfg.Backends.OpenAIEndpoint)
	openaiProxy := httputil.NewSingleHostReverseProxy(openaiURL)

	// Modifica richieste OpenAI per aggiungere API key
	openaiProxy.Director = func(req *http.Request) {
		req.URL.Scheme = openaiURL.Scheme
		req.URL.Host = openaiURL.Host
		req.Host = openaiURL.Host

		// Sostituisci header Authorization con API key OpenAI
		req.Header.Del("Authorization")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.Backends.OpenAIAPIKey))

		// Mantieni X-Forwarded-User per audit
		if user := req.Header.Get("X-Forwarded-User"); user != "" {
			req.Header.Set("X-Forwarded-User", user)
		}

		log.WithFields(logrus.Fields{
			"user":   req.Header.Get("X-Forwarded-User"),
			"path":   req.URL.Path,
			"method": req.Method,
		}).Debug("Proxying richiesta OpenAI")
	}

	return &Handler{
		cfg:            cfg,
		log:            log,
		ollamaLB:       ollamaLB,
		vllmLB:         vllmLB,
		openaiProxy:    openaiProxy,
		metricsManager: mm,
	}
}

// ServeHTTP implementa http.Handler per gestire le richieste
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Routing basato su path
	if strings.HasPrefix(r.URL.Path, "/ollama/") {
		h.handleOllama(w, r, start)
	} else if strings.HasPrefix(r.URL.Path, "/vllm/") {
		h.handleVLLM(w, r, start)
	} else if strings.HasPrefix(r.URL.Path, "/openai/") {
		h.handleOpenAI(w, r, start)
	} else {
		h.log.WithField("path", r.URL.Path).Warn("Path non riconosciuto")
		http.NotFound(w, r)
	}
}

// handleOllama gestisce richieste per backend Ollama
func (h *Handler) handleOllama(w http.ResponseWriter, r *http.Request, start time.Time) {
	// Seleziona server tramite load balancer
	serverURL, err := h.ollamaLB.SelectServer()
	if err != nil {
		h.log.WithError(err).Error("Impossibile selezionare server Ollama")
		h.metricsManager.IncrementProxyErrors("ollama")
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Crea proxy per il server selezionato
	targetURL, _ := url.Parse(serverURL)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Configura director per modificare richiesta
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host

		// Rimuovi prefisso /ollama/ dal path
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/ollama")
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}

		// Rimuovi header Authorization (già autenticato)
		req.Header.Del("Authorization")

		// Mantieni X-Forwarded-* headers
		if user := req.Header.Get("X-Forwarded-User"); user != "" {
			req.Header.Set("X-Forwarded-User", user)
		}
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
		req.Header.Set("X-Forwarded-Proto", "https")

		h.log.WithFields(logrus.Fields{
			"user":   req.Header.Get("X-Forwarded-User"),
			"server": serverURL,
			"path":   req.URL.Path,
			"method": req.Method,
		}).Debug("Proxying richiesta Ollama")
	}

	// Gestione errori proxy
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		h.log.WithFields(logrus.Fields{
			"server": serverURL,
			"error":  err.Error(),
		}).Error("Errore proxy Ollama")
		h.metricsManager.IncrementProxyErrors("ollama")
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Esegui proxy
	proxy.ServeHTTP(w, r)

	// Registra latenza
	duration := time.Since(start)
	h.metricsManager.RecordLatency("ollama", duration)
	h.log.WithFields(logrus.Fields{
		"server":   serverURL,
		"duration": duration.Milliseconds(),
	}).Info("Richiesta Ollama completata")
}

// handleOpenAI gestisce richieste per backend OpenAI
func (h *Handler) handleOpenAI(w http.ResponseWriter, r *http.Request, start time.Time) {
	// Rimuovi prefisso /openai/ dal path
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/openai")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}

	// Gestione errori
	h.openaiProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		h.log.WithError(err).Error("Errore proxy OpenAI")
		h.metricsManager.IncrementProxyErrors("openai")
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Esegui proxy
	h.openaiProxy.ServeHTTP(w, r)

	// Registra latenza
	duration := time.Since(start)
	h.metricsManager.RecordLatency("openai", duration)
	h.log.WithField("duration", duration.Milliseconds()).Info("Richiesta OpenAI completata")
}

// handleVLLM gestisce richieste per backend vLLM
func (h *Handler) handleVLLM(w http.ResponseWriter, r *http.Request, start time.Time) {
	// Seleziona server tramite load balancer
	serverURL, err := h.vllmLB.SelectServer()
	if err != nil {
		h.log.WithError(err).Error("Impossibile selezionare server vLLM")
		h.metricsManager.IncrementProxyErrors("vllm")
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Crea proxy per il server selezionato
	targetURL, _ := url.Parse(serverURL)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Configura director per modificare richiesta
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.Host = targetURL.Host

		// Rimuovi prefisso /vllm/ dal path
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/vllm")
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}

		// Rimuovi header Authorization (già autenticato)
		req.Header.Del("Authorization")

		// Mantieni X-Forwarded-* headers
		if user := req.Header.Get("X-Forwarded-User"); user != "" {
			req.Header.Set("X-Forwarded-User", user)
		}
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
		req.Header.Set("X-Forwarded-Proto", "https")

		h.log.WithFields(logrus.Fields{
			"user":   req.Header.Get("X-Forwarded-User"),
			"server": serverURL,
			"path":   req.URL.Path,
			"method": req.Method,
		}).Debug("Proxying richiesta vLLM")
	}

	// Gestione errori proxy
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		h.log.WithFields(logrus.Fields{
			"server": serverURL,
			"error":  err.Error(),
		}).Error("Errore proxy vLLM")
		h.metricsManager.IncrementProxyErrors("vllm")
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	// Esegui proxy
	proxy.ServeHTTP(w, r)

	// Registra latenza
	duration := time.Since(start)
	h.metricsManager.RecordLatency("vllm", duration)
	h.log.WithFields(logrus.Fields{
		"server":   serverURL,
		"duration": duration.Milliseconds(),
	}).Info("Richiesta vLLM completata")
}
