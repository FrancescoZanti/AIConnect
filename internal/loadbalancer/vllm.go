package loadbalancer

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// VLLMLoadBalancer gestisce il load balancing tra server vLLM
type VLLMLoadBalancer struct {
	servers         []string
	metrics         map[string]*ServerMetrics
	mutex           sync.RWMutex
	log             *logrus.Logger
	checkInterval   time.Duration
	maxConsecErrors int
	roundRobinIndex int
}

// NewVLLMLoadBalancer crea un nuovo load balancer per vLLM
func NewVLLMLoadBalancer(servers []string, checkInterval int, log *logrus.Logger) *VLLMLoadBalancer {
	lb := &VLLMLoadBalancer{
		servers:         servers,
		metrics:         make(map[string]*ServerMetrics),
		log:             log,
		checkInterval:   time.Duration(checkInterval) * time.Second,
		maxConsecErrors: 3,
		roundRobinIndex: 0,
	}

	// Inizializza metriche per ogni server
	for _, server := range servers {
		lb.metrics[server] = &ServerMetrics{
			URL:       server,
			Available: true,
		}
	}

	return lb
}

// Start avvia il monitoraggio periodico dei server
func (lb *VLLMLoadBalancer) Start() {
	// Controllo iniziale
	lb.checkAllServers()

	// Avvia polling periodico
	ticker := time.NewTicker(lb.checkInterval)
	go func() {
		for range ticker.C {
			lb.checkAllServers()
		}
	}()

	lb.log.WithField("interval", lb.checkInterval).Info("Load balancer vLLM avviato")
}

// checkAllServers controlla lo stato di tutti i server
func (lb *VLLMLoadBalancer) checkAllServers() {
	var wg sync.WaitGroup

	for _, server := range lb.servers {
		wg.Add(1)
		go func(serverURL string) {
			defer wg.Done()
			lb.checkServer(serverURL)
		}(server)
	}

	wg.Wait()
}

// checkServer controlla metriche di un singolo server vLLM
func (lb *VLLMLoadBalancer) checkServer(serverURL string) {
	// vLLM espone metriche su /metrics in formato Prometheus o /health
	// Proviamo prima con /health per verificare disponibilità
	healthURL := fmt.Sprintf("%s/health", serverURL)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(healthURL)
	if err != nil {
		lb.handleServerError(serverURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		lb.handleServerError(serverURL, fmt.Errorf("status code: %d", resp.StatusCode))
		return
	}

	// Tenta di ottenere metriche dettagliate da /metrics (formato JSON custom)
	metricsURL := fmt.Sprintf("%s/metrics", serverURL)
	metricsResp, err := client.Get(metricsURL)
	if err == nil && metricsResp.StatusCode == http.StatusOK {
		defer metricsResp.Body.Close()

		// Parse JSON response (se disponibile)
		var data struct {
			CPUPercent   float64 `json:"cpu_percent"`
			RAMPercent   float64 `json:"ram_percent"`
			GPUCount     int     `json:"gpu_count"`
			GPUAvgUtil   float64 `json:"gpu_avg_utilization_percent"`
			GPUAvgMemory float64 `json:"gpu_avg_memory_percent"`
		}

		if err := json.NewDecoder(metricsResp.Body).Decode(&data); err == nil {
			// Aggiorna metriche dettagliate
			lb.mutex.Lock()
			metrics := lb.metrics[serverURL]
			metrics.CPUPercent = data.CPUPercent
			metrics.RAMPercent = data.RAMPercent
			metrics.GPUCount = data.GPUCount
			metrics.GPUAvgUtil = data.GPUAvgUtil
			metrics.GPUAvgMemory = data.GPUAvgMemory

			// Calcola peso totale: CPU + RAM + (GPU util * 1.5) + (GPU mem * 1.5)
			gpuWeight := 0.0
			if data.GPUCount > 0 {
				gpuWeight = (data.GPUAvgUtil * 1.5) + (data.GPUAvgMemory * 1.5)
			}
			metrics.TotalWeight = data.CPUPercent + data.RAMPercent + gpuWeight

			metrics.Available = true
			metrics.LastCheck = time.Now()
			metrics.ErrorCount = 0
			lb.mutex.Unlock()

			lb.log.WithFields(logrus.Fields{
				"server":    serverURL,
				"cpu":       data.CPUPercent,
				"ram":       data.RAMPercent,
				"gpu_count": data.GPUCount,
				"gpu_util":  data.GPUAvgUtil,
				"gpu_mem":   data.GPUAvgMemory,
				"weight":    metrics.TotalWeight,
			}).Debug("Metriche server vLLM aggiornate")
			return
		}
	}

	// Fallback: server è disponibile ma senza metriche dettagliate
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	metrics := lb.metrics[serverURL]
	metrics.Available = true
	metrics.LastCheck = time.Now()
	metrics.ErrorCount = 0

	lb.log.WithField("server", serverURL).Debug("Server vLLM disponibile (senza metriche dettagliate)")
}

// handleServerError gestisce errori di comunicazione con il server
func (lb *VLLMLoadBalancer) handleServerError(serverURL string, err error) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	metrics := lb.metrics[serverURL]
	metrics.ErrorCount++
	metrics.LastCheck = time.Now()

	if metrics.ErrorCount >= lb.maxConsecErrors {
		metrics.Available = false
		lb.log.WithFields(logrus.Fields{
			"server":      serverURL,
			"error_count": metrics.ErrorCount,
			"error":       err.Error(),
		}).Warn("Server vLLM marcato non disponibile")
	} else {
		lb.log.WithFields(logrus.Fields{
			"server":      serverURL,
			"error_count": metrics.ErrorCount,
			"error":       err.Error(),
		}).Debug("Errore comunicazione server vLLM")
	}
}

// SelectServer seleziona il server migliore usando weighted least-load
func (lb *VLLMLoadBalancer) SelectServer() (string, error) {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	// Trova server disponibili
	var availableServers []*ServerMetrics
	for _, metrics := range lb.metrics {
		if metrics.Available {
			availableServers = append(availableServers, metrics)
		}
	}

	if len(availableServers) == 0 {
		return "", fmt.Errorf("nessun server vLLM disponibile")
	}

	// Se abbiamo metriche valide, usa weighted least-load
	var hasMetrics bool
	for _, m := range availableServers {
		if !m.LastCheck.IsZero() && m.TotalWeight > 0 {
			hasMetrics = true
			break
		}
	}

	if hasMetrics {
		// Trova server con carico minore
		minWeight := math.MaxFloat64
		var selectedServer string

		for _, m := range availableServers {
			if !m.LastCheck.IsZero() && m.TotalWeight > 0 && m.TotalWeight < minWeight {
				minWeight = m.TotalWeight
				selectedServer = m.URL
			}
		}

		if selectedServer != "" {
			lb.log.WithFields(logrus.Fields{
				"server": selectedServer,
				"weight": minWeight,
			}).Debug("Server vLLM selezionato (weighted least-load)")
			return selectedServer, nil
		}
	}

	// Fallback: round-robin
	selected := availableServers[lb.roundRobinIndex%len(availableServers)]
	lb.roundRobinIndex++

	lb.log.WithField("server", selected.URL).Debug("Server vLLM selezionato (round-robin fallback)")
	return selected.URL, nil
}

// GetMetrics restituisce le metriche correnti (per debugging/monitoring)
func (lb *VLLMLoadBalancer) GetMetrics() map[string]*ServerMetrics {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	// Copia per evitare race conditions
	result := make(map[string]*ServerMetrics)
	for k, v := range lb.metrics {
		metricsCopy := *v
		result[k] = &metricsCopy
	}

	return result
}
