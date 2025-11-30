package mdns

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/fzanti/aiconnect/internal/registry"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultHealthCheckInterval is the default interval between health checks
	DefaultHealthCheckInterval = 30 * time.Second
	// DefaultHealthCheckTimeout is the default timeout for health checks
	DefaultHealthCheckTimeout = 2 * time.Second
	// DefaultMaxHealthErrors is the default max consecutive errors before marking unhealthy
	DefaultMaxHealthErrors = 3
)

// HealthCheckerConfig contains configuration for the health checker
type HealthCheckerConfig struct {
	// CheckInterval is the interval between health checks
	CheckInterval time.Duration
	// CheckTimeout is the timeout for each health check
	CheckTimeout time.Duration
	// MaxErrors is the max consecutive errors before marking unhealthy
	MaxErrors int
}

// DefaultHealthCheckerConfig returns default health checker configuration
func DefaultHealthCheckerConfig() *HealthCheckerConfig {
	return &HealthCheckerConfig{
		CheckInterval: DefaultHealthCheckInterval,
		CheckTimeout:  DefaultHealthCheckTimeout,
		MaxErrors:     DefaultMaxHealthErrors,
	}
}

// HealthChecker performs periodic health checks on discovered nodes
type HealthChecker struct {
	config   *HealthCheckerConfig
	registry *registry.Registry
	log      *logrus.Logger
	client   *http.Client
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	running  bool
	mutex    sync.Mutex
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(config *HealthCheckerConfig, reg *registry.Registry, log *logrus.Logger) *HealthChecker {
	if config == nil {
		config = DefaultHealthCheckerConfig()
	}
	if log == nil {
		log = logrus.New()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &HealthChecker{
		config:   config,
		registry: reg,
		log:      log,
		client: &http.Client{
			// No timeout here - use context timeout instead
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the periodic health checking
func (h *HealthChecker) Start() {
	h.mutex.Lock()
	if h.running {
		h.mutex.Unlock()
		return
	}
	h.running = true
	h.mutex.Unlock()

	// Initial health check
	h.checkAll()

	// Start periodic checking
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		ticker := time.NewTicker(h.config.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-h.ctx.Done():
				return
			case <-ticker.C:
				h.checkAll()
			}
		}
	}()

	h.log.WithField("interval", h.config.CheckInterval).Info("Health checker started")
}

// Stop stops the health checker
func (h *HealthChecker) Stop() {
	h.mutex.Lock()
	if !h.running {
		h.mutex.Unlock()
		return
	}
	h.running = false
	h.mutex.Unlock()

	h.cancel()
	h.wg.Wait()
	h.log.Info("Health checker stopped")
}

// checkAll checks the health of all registered nodes
func (h *HealthChecker) checkAll() {
	nodes := h.registry.GetAllNodes()
	if len(nodes) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(n *registry.Node) {
			defer wg.Done()
			h.checkNode(n)
		}(node)
	}
	wg.Wait()
}

// checkNode checks the health of a single node
func (h *HealthChecker) checkNode(node *registry.Node) {
	var healthy bool
	var err error

	switch node.Type {
	case registry.NodeTypeOllama:
		healthy, err = h.checkOllama(node)
	case registry.NodeTypeVLLM, registry.NodeTypeOpenAI:
		healthy, err = h.checkOpenAICompatible(node)
	default:
		healthy, err = h.checkGeneric(node)
	}

	if healthy {
		h.registry.UpdateNodeStatus(node.Host, node.Port, registry.NodeStatusHealthy)
		h.log.WithFields(logrus.Fields{
			"name": node.Name,
			"host": node.Host,
			"port": node.Port,
			"type": node.Type,
		}).Debug("Node health check passed")
	} else {
		h.registry.IncrementErrorCount(node.Host, node.Port, h.config.MaxErrors)
		h.log.WithFields(logrus.Fields{
			"name":  node.Name,
			"host":  node.Host,
			"port":  node.Port,
			"type":  node.Type,
			"error": err,
		}).Debug("Node health check failed")
	}
}

// checkOllama checks the health of an Ollama node
func (h *HealthChecker) checkOllama(node *registry.Node) (bool, error) {
	url := fmt.Sprintf("http://%s:%d/api/tags", node.Host, node.Port)
	return h.doHealthCheck(url)
}

// checkOpenAICompatible checks the health of an OpenAI-compatible node (vLLM, OpenAI)
func (h *HealthChecker) checkOpenAICompatible(node *registry.Node) (bool, error) {
	scheme := "http"
	if node.Type == registry.NodeTypeOpenAI {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s:%d/v1/models", scheme, node.Host, node.Port)
	return h.doHealthCheck(url)
}

// checkGeneric performs a generic health check
func (h *HealthChecker) checkGeneric(node *registry.Node) (bool, error) {
	url := fmt.Sprintf("http://%s:%d/health", node.Host, node.Port)
	return h.doHealthCheck(url)
}

// doHealthCheck performs an HTTP GET request and checks for success
func (h *HealthChecker) doHealthCheck(url string) (bool, error) {
	ctx, cancel := context.WithTimeout(h.ctx, h.config.CheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}

	return false, fmt.Errorf("health check returned status %d", resp.StatusCode)
}

// NodesResponse represents the response for /internal/nodes endpoint
type NodesResponse struct {
	AIConnect struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"aiconnect"`
	DiscoveredNodes []*NodeInfo `json:"discovered_nodes"`
}

// NodeInfo represents a discovered node in the API response
type NodeInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Status   string `json:"status"`
	LastSeen string `json:"last_seen"`
}

// NodesHandler creates an HTTP handler for the /internal/nodes endpoint
func NodesHandler(reg *registry.Registry, host string, port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodes := reg.GetAllNodes()

		response := NodesResponse{}
		response.AIConnect.Host = host
		response.AIConnect.Port = port

		response.DiscoveredNodes = make([]*NodeInfo, 0, len(nodes))
		for _, node := range nodes {
			info := &NodeInfo{
				Name:     node.Name,
				Type:     string(node.Type),
				Host:     node.Host,
				Port:     node.Port,
				Status:   string(node.Status),
				LastSeen: node.LastSeen.Format(time.RFC3339),
			}
			response.DiscoveredNodes = append(response.DiscoveredNodes, info)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}
