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

// ServerMetrics contiene le metriche di carico di un server
type ServerMetrics struct {
	URL              string
	CPUPercent       float64
	RAMPercent       float64
	GPUCount         int
	GPUAvgUtil       float64
	GPUAvgMemory     float64
	Available        bool
	LastCheck        time.Time
	ErrorCount       int
	TotalWeight      float64 // Carico totale calcolato
}

// OllamaLoadBalancer gestisce il load balancing tra server Ollama
type OllamaLoadBalancer struct {
	servers         []string
	metrics         map[string]*ServerMetrics
	mutex           sync.RWMutex
	log             *logrus.Logger
	checkInterval   time.Duration
	maxConsecErrors int
	roundRobinIndex int
}

// NewOllamaLoadBalancer crea un nuovo load balancer
func NewOllamaLoadBalancer(servers []string, checkInterval int, log *logrus.Logger) *OllamaLoadBalancer {
	lb := &OllamaLoadBalancer{
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
func (lb *OllamaLoadBalancer) Start() {
	// Controllo iniziale
	lb.checkAllServers()

	// Avvia polling periodico
	ticker := time.NewTicker(lb.checkInterval)
	go func() {
		for range ticker.C {
			lb.checkAllServers()
		}
	}()

	lb.log.WithField("interval", lb.checkInterval).Info("Load balancer Ollama avviato")
}

// checkAllServers controlla lo stato di tutti i server
func (lb *OllamaLoadBalancer) checkAllServers() {
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

// checkServer controlla metriche di un singolo server
func (lb *OllamaLoadBalancer) checkServer(serverURL string) {
	metricsURL := fmt.Sprintf("%s/metrics", serverURL)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(metricsURL)
	if err != nil {
		lb.handleServerError(serverURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		lb.handleServerError(serverURL, fmt.Errorf("status code: %d", resp.StatusCode))
		return
	}

	// Parse JSON response
	var data struct {
		CPUPercent    float64 `json:"cpu_percent"`
		RAMPercent    float64 `json:"ram_percent"`
		GPUCount      int     `json:"gpu_count"`
		GPUAvgUtil    float64 `json:"gpu_avg_utilization_percent"`
		GPUAvgMemory  float64 `json:"gpu_avg_memory_percent"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		lb.handleServerError(serverURL, err)
		return
	}

	// Aggiorna metriche
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	metrics := lb.metrics[serverURL]
	metrics.CPUPercent = data.CPUPercent
	metrics.RAMPercent = data.RAMPercent
	metrics.GPUCount = data.GPUCount
	metrics.GPUAvgUtil = data.GPUAvgUtil
	metrics.GPUAvgMemory = data.GPUAvgMemory
	
	// Calcola peso totale: CPU + RAM + (GPU util * 1.5) + (GPU mem * 1.5)
	// GPU ha peso maggiore perché più critica per inferenza AI
	gpuWeight := 0.0
	if data.GPUCount > 0 {
		gpuWeight = (data.GPUAvgUtil * 1.5) + (data.GPUAvgMemory * 1.5)
	}
	metrics.TotalWeight = data.CPUPercent + data.RAMPercent + gpuWeight
	
	metrics.Available = true
	metrics.LastCheck = time.Now()
	metrics.ErrorCount = 0

	lb.log.WithFields(logrus.Fields{
		"server":    serverURL,
		"cpu":       data.CPUPercent,
		"ram":       data.RAMPercent,
		"gpu_count": data.GPUCount,
		"gpu_util":  data.GPUAvgUtil,
		"gpu_mem":   data.GPUAvgMemory,
		"weight":    metrics.TotalWeight,
	}).Debug("Metriche server aggiornate")
}

// handleServerError gestisce errori di comunicazione con il server
func (lb *OllamaLoadBalancer) handleServerError(serverURL string, err error) {
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
		}).Warn("Server marcato non disponibile")
	} else {
		lb.log.WithFields(logrus.Fields{
			"server":      serverURL,
			"error_count": metrics.ErrorCount,
			"error":       err.Error(),
		}).Debug("Errore comunicazione server")
	}
}

// SelectServer seleziona il server migliore usando weighted least-load
func (lb *OllamaLoadBalancer) SelectServer() (string, error) {
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
		return "", fmt.Errorf("nessun server Ollama disponibile")
	}

	// Se abbiamo metriche valide, usa weighted least-load
	var hasMetrics bool
	for _, m := range availableServers {
		if !m.LastCheck.IsZero() {
			hasMetrics = true
			break
		}
	}

	if hasMetrics {
		// Trova server con carico minore
		minWeight := math.MaxFloat64
		var selectedServer string

		for _, m := range availableServers {
			if !m.LastCheck.IsZero() && m.TotalWeight < minWeight {
				minWeight = m.TotalWeight
				selectedServer = m.URL
			}
		}

		if selectedServer != "" {
			lb.log.WithFields(logrus.Fields{
				"server": selectedServer,
				"weight": minWeight,
			}).Debug("Server selezionato (weighted least-load)")
			return selectedServer, nil
		}
	}

	// Fallback: round-robin
	selected := availableServers[lb.roundRobinIndex%len(availableServers)]
	lb.roundRobinIndex++

	lb.log.WithField("server", selected.URL).Debug("Server selezionato (round-robin fallback)")
	return selected.URL, nil
}

// GetMetrics restituisce le metriche correnti (per debugging/monitoring)
func (lb *OllamaLoadBalancer) GetMetrics() map[string]*ServerMetrics {
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
