package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/fzanti/aiconnect/internal/auth"
	"github.com/fzanti/aiconnect/internal/config"
	"github.com/fzanti/aiconnect/internal/loadbalancer"
	"github.com/fzanti/aiconnect/internal/metrics"
	"github.com/fzanti/aiconnect/internal/proxy"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/acme/autocert"
)

func main() {
	// Setup logger
	log := logrus.New()

	// Get config path from environment or default
	configPath := os.Getenv("AICONNECT_CONFIG")
	if configPath == "" {
		configPath = "/etc/aiconnect/config.yaml"
	}

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.WithError(err).Fatal("Impossibile caricare configurazione")
	}

	// Configure logger based on config
	level, err := logrus.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	log.SetLevel(level)

	if cfg.Logging.Format == "json" {
		log.SetFormatter(&logrus.JSONFormatter{})
	} else {
		log.SetFormatter(&logrus.TextFormatter{})
	}

	log.Info("AIConnect in avvio...")

	// Initialize metrics manager
	metricsManager := metrics.NewManager()

	// Initialize Ollama load balancer
	ollamaLB := loadbalancer.NewOllamaLoadBalancer(
		cfg.Backends.OllamaServers,
		cfg.Monitoring.HealthCheckInterval,
		log,
	)
	ollamaLB.Start()

	// Initialize vLLM load balancer
	vllmLB := loadbalancer.NewVLLMLoadBalancer(
		cfg.Backends.VLLMServers,
		cfg.Monitoring.HealthCheckInterval,
		log,
	)
	vllmLB.Start()

	// Create proxy handler
	proxyHandler := proxy.NewHandler(cfg, log, ollamaLB, vllmLB, metricsManager)

	// Wrap with authentication middleware
	authHandler := auth.LDAPAuthMiddleware(cfg, log)(proxyHandler)

	// Setup HTTP mux
	mux := http.NewServeMux()
	mux.Handle("/ollama/", authHandler)
	mux.Handle("/vllm/", authHandler)
	mux.Handle("/openai/", authHandler)

	// Health check endpoint (unauthenticated)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	// Start metrics server on separate port
	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())

		metricsAddr := fmt.Sprintf(":%d", cfg.Monitoring.MetricsPort)
		log.WithField("address", metricsAddr).Info("Server metriche in ascolto")

		if err := http.ListenAndServe(metricsAddr, metricsMux); err != nil {
			log.WithError(err).Fatal("Errore server metriche")
		}
	}()

	// Configure autocert manager for LetsEncrypt
	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.HTTPS.Domain),
		Cache:      autocert.DirCache(cfg.HTTPS.CacheDir),
	}

	// Configure HTTPS server
	httpsAddr := fmt.Sprintf(":%d", cfg.HTTPS.Port)
	server := &http.Server{
		Addr:      httpsAddr,
		Handler:   mux,
		TLSConfig: certManager.TLSConfig(),
	}

	log.WithFields(logrus.Fields{
		"address": httpsAddr,
		"domain":  cfg.HTTPS.Domain,
	}).Info("Server HTTPS in avvio")

	// Start HTTPS server
	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.WithError(err).Fatal("Errore server HTTPS")
	}
}
