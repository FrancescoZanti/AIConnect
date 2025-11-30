package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/fzanti/aiconnect/internal/auth"
	"github.com/fzanti/aiconnect/internal/config"
	"github.com/fzanti/aiconnect/internal/loadbalancer"
	"github.com/fzanti/aiconnect/internal/mdns"
	"github.com/fzanti/aiconnect/internal/metrics"
	"github.com/fzanti/aiconnect/internal/proxy"
	"github.com/fzanti/aiconnect/internal/registry"
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

	// Initialize node registry for mDNS discovery
	nodeRegistry := registry.NewRegistry()

	// Initialize mDNS advertiser if enabled
	var mdnsAdvertiser *mdns.Advertiser
	if cfg.MDNS.Enabled {
		advertiserConfig := &mdns.AdvertiserConfig{
			ServiceName:  cfg.MDNS.ServiceName,
			Port:         cfg.HTTPS.Port,
			Domain:       "local.",
			Version:      cfg.MDNS.Version,
			Capabilities: cfg.MDNS.Capabilities,
		}
		mdnsAdvertiser = mdns.NewAdvertiser(advertiserConfig, log)
		if err := mdnsAdvertiser.Start(); err != nil {
			log.WithError(err).Warn("Failed to start mDNS advertiser")
		} else {
			defer mdnsAdvertiser.Stop()
		}
	}

	// Initialize mDNS discovery if enabled
	var mdnsDiscovery *mdns.Discovery
	var healthChecker *mdns.HealthChecker
	if cfg.MDNS.DiscoveryEnabled {
		discoveryConfig := &mdns.DiscoveryConfig{
			ServiceTypes:      cfg.MDNS.ServiceTypes,
			Domain:            "local",
			DiscoveryInterval: time.Duration(cfg.MDNS.DiscoveryInterval) * time.Second,
			DiscoveryTimeout:  time.Duration(cfg.MDNS.DiscoveryTimeout) * time.Second,
		}
		mdnsDiscovery = mdns.NewDiscovery(discoveryConfig, nodeRegistry, log)
		mdnsDiscovery.Start()
		defer mdnsDiscovery.Stop()

		// Initialize health checker for discovered nodes
		healthConfig := &mdns.HealthCheckerConfig{
			CheckInterval: time.Duration(cfg.Monitoring.HealthCheckInterval) * time.Second,
			CheckTimeout:  2 * time.Second,
			MaxErrors:     3,
		}
		healthChecker = mdns.NewHealthChecker(healthConfig, nodeRegistry, log)
		healthChecker.Start()
		defer healthChecker.Stop()

		// Register event callback for logging
		nodeRegistry.OnEvent(func(e registry.Event) {
			log.WithFields(logrus.Fields{
				"event": e.Type,
				"node":  e.Node.Name,
				"host":  e.Node.Host,
				"port":  e.Node.Port,
				"type":  e.Node.Type,
			}).Info("Registry event")
		})
	}

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

	// Nodes endpoint for topology discovery (unauthenticated for MatePro compatibility)
	// Get local host for the response
	localHost := getLocalHost()
	mux.HandleFunc("/internal/nodes", mdns.NodesHandler(nodeRegistry, localHost, cfg.HTTPS.Port))

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

// getLocalHost returns the local host IP address
func getLocalHost() string {
	// Try to get the preferred outbound IP
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "127.0.0.1"
}
