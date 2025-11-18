package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Manager gestisce le metriche Prometheus
type Manager struct {
	authAttempts   *prometheus.CounterVec
	authFailures   *prometheus.CounterVec
	proxyRequests  *prometheus.CounterVec
	proxyErrors    *prometheus.CounterVec
	proxyLatency   *prometheus.HistogramVec
	backendHealth  *prometheus.GaugeVec
}

// NewManager crea un nuovo manager delle metriche
func NewManager() *Manager {
	return &Manager{
		authAttempts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aiconnect_auth_attempts_total",
				Help: "Numero totale di tentativi di autenticazione",
			},
			[]string{"result"},
		),

		authFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aiconnect_auth_failures_total",
				Help: "Numero totale di autenticazioni fallite",
			},
			[]string{"reason"},
		),

		proxyRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aiconnect_proxy_requests_total",
				Help: "Numero totale di richieste proxy",
			},
			[]string{"backend"},
		),

		proxyErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aiconnect_proxy_errors_total",
				Help: "Numero totale di errori proxy",
			},
			[]string{"backend"},
		),

		proxyLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "aiconnect_proxy_latency_seconds",
				Help:    "Latenza richieste proxy in secondi",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"backend"},
		),

		backendHealth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "aiconnect_backend_health",
				Help: "Stato salute backend (1=healthy, 0=unhealthy)",
			},
			[]string{"backend", "server"},
		),
	}
}

// IncrementAuthAttempts incrementa il contatore tentativi autenticazione
func (m *Manager) IncrementAuthAttempts(success bool) {
	result := "failure"
	if success {
		result = "success"
	}
	m.authAttempts.WithLabelValues(result).Inc()
}

// IncrementAuthFailures incrementa il contatore fallimenti autenticazione
func (m *Manager) IncrementAuthFailures(reason string) {
	m.authFailures.WithLabelValues(reason).Inc()
}

// IncrementProxyRequests incrementa il contatore richieste proxy
func (m *Manager) IncrementProxyRequests(backend string) {
	m.proxyRequests.WithLabelValues(backend).Inc()
}

// IncrementProxyErrors incrementa il contatore errori proxy
func (m *Manager) IncrementProxyErrors(backend string) {
	m.proxyErrors.WithLabelValues(backend).Inc()
}

// RecordLatency registra la latenza di una richiesta proxy
func (m *Manager) RecordLatency(backend string, duration time.Duration) {
	m.proxyLatency.WithLabelValues(backend).Observe(duration.Seconds())
}

// SetBackendHealth imposta lo stato di salute di un backend
func (m *Manager) SetBackendHealth(backend, server string, healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	m.backendHealth.WithLabelValues(backend, server).Set(value)
}
