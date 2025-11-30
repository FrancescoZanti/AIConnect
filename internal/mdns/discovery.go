package mdns

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fzanti/aiconnect/internal/registry"
	"github.com/grandcat/zeroconf"
	"github.com/sirupsen/logrus"
)

const (
	// OllamaServiceType is the mDNS service type for Ollama
	OllamaServiceType = "_ollama._tcp"
	// OpenAIServiceType is the mDNS service type for OpenAI-compatible backends
	OpenAIServiceType = "_openai._tcp"
	// VLLMServiceType is the mDNS service type for vLLM
	VLLMServiceType = "_vllm._tcp"

	// DefaultDiscoveryTimeout is the default timeout for mDNS discovery
	DefaultDiscoveryTimeout = 5 * time.Second
	// DefaultDiscoveryInterval is the default interval between discovery scans
	DefaultDiscoveryInterval = 30 * time.Second
)

// DiscoveryConfig contains configuration for the mDNS discovery
type DiscoveryConfig struct {
	// ServiceTypes is a list of service types to discover
	ServiceTypes []string
	// Domain is the mDNS domain (default: "local")
	Domain string
	// DiscoveryInterval is the interval between discovery scans
	DiscoveryInterval time.Duration
	// DiscoveryTimeout is the timeout for each discovery scan
	DiscoveryTimeout time.Duration
}

// DefaultDiscoveryConfig returns default discovery configuration
func DefaultDiscoveryConfig() *DiscoveryConfig {
	return &DiscoveryConfig{
		ServiceTypes: []string{
			OllamaServiceType,
			OpenAIServiceType,
			VLLMServiceType,
		},
		Domain:            "local",
		DiscoveryInterval: DefaultDiscoveryInterval,
		DiscoveryTimeout:  DefaultDiscoveryTimeout,
	}
}

// Discovery handles mDNS discovery of LLM backends
type Discovery struct {
	config   *DiscoveryConfig
	registry *registry.Registry
	log      *logrus.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	running  bool
	mutex    sync.Mutex
}

// NewDiscovery creates a new mDNS discovery instance
func NewDiscovery(config *DiscoveryConfig, reg *registry.Registry, log *logrus.Logger) *Discovery {
	if config == nil {
		config = DefaultDiscoveryConfig()
	}
	if log == nil {
		log = logrus.New()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Discovery{
		config:   config,
		registry: reg,
		log:      log,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the mDNS discovery process
func (d *Discovery) Start() {
	d.mutex.Lock()
	if d.running {
		d.mutex.Unlock()
		return
	}
	d.running = true
	d.mutex.Unlock()

	// Initial discovery
	d.discover()

	// Start periodic discovery
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		ticker := time.NewTicker(d.config.DiscoveryInterval)
		defer ticker.Stop()

		for {
			select {
			case <-d.ctx.Done():
				return
			case <-ticker.C:
				d.discover()
			}
		}
	}()

	d.log.WithFields(logrus.Fields{
		"services": d.config.ServiceTypes,
		"interval": d.config.DiscoveryInterval,
	}).Info("mDNS discovery started")
}

// Stop stops the mDNS discovery process
func (d *Discovery) Stop() {
	d.mutex.Lock()
	if !d.running {
		d.mutex.Unlock()
		return
	}
	d.running = false
	d.mutex.Unlock()

	d.cancel()
	d.wg.Wait()
	d.log.Info("mDNS discovery stopped")
}

// discover performs a single discovery scan for all configured service types
func (d *Discovery) discover() {
	for _, serviceType := range d.config.ServiceTypes {
		d.discoverService(serviceType)
	}
}

// discoverService performs a single discovery scan for a specific service type
func (d *Discovery) discoverService(serviceType string) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		d.log.WithError(err).WithField("service", serviceType).Error("Failed to create mDNS resolver")
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)
	ctx, cancel := context.WithTimeout(d.ctx, d.config.DiscoveryTimeout)
	defer cancel()

	// Process discovered entries
	go func() {
		for entry := range entries {
			d.processEntry(serviceType, entry)
		}
	}()

	// Start browsing
	err = resolver.Browse(ctx, serviceType, d.config.Domain, entries)
	if err != nil {
		d.log.WithError(err).WithField("service", serviceType).Debug("mDNS browse error")
	}

	<-ctx.Done()
}

// processEntry processes a discovered mDNS entry
func (d *Discovery) processEntry(serviceType string, entry *zeroconf.ServiceEntry) {
	if entry == nil {
		return
	}

	nodeType := serviceTypeToNodeType(serviceType)
	if nodeType == "" {
		d.log.WithField("service", serviceType).Warn("Unknown service type")
		return
	}

	// Get the first IPv4 address
	var host string
	for _, addr := range entry.AddrIPv4 {
		host = addr.String()
		break
	}
	if host == "" {
		for _, addr := range entry.AddrIPv6 {
			host = addr.String()
			break
		}
	}
	if host == "" && entry.HostName != "" {
		host = strings.TrimSuffix(entry.HostName, ".")
	}
	if host == "" {
		d.log.WithField("instance", entry.Instance).Warn("No address found for discovered service")
		return
	}

	node := &registry.Node{
		Name:   entry.Instance,
		Type:   nodeType,
		Host:   host,
		Port:   entry.Port,
		Status: registry.NodeStatusUnknown,
	}

	d.registry.AddNode(node)

	d.log.WithFields(logrus.Fields{
		"name":    node.Name,
		"type":    node.Type,
		"host":    node.Host,
		"port":    node.Port,
		"service": serviceType,
	}).Debug("Discovered LLM backend via mDNS")
}

// serviceTypeToNodeType converts an mDNS service type to a registry node type
func serviceTypeToNodeType(serviceType string) registry.NodeType {
	switch serviceType {
	case OllamaServiceType:
		return registry.NodeTypeOllama
	case OpenAIServiceType:
		return registry.NodeTypeOpenAI
	case VLLMServiceType:
		return registry.NodeTypeVLLM
	default:
		return ""
	}
}

// NodeTypeToServiceType converts a registry node type to an mDNS service type
func NodeTypeToServiceType(nodeType registry.NodeType) string {
	switch nodeType {
	case registry.NodeTypeOllama:
		return OllamaServiceType
	case registry.NodeTypeOpenAI:
		return OpenAIServiceType
	case registry.NodeTypeVLLM:
		return VLLMServiceType
	default:
		return ""
	}
}

// GetServiceURL returns the full URL for a node based on its type
func GetServiceURL(node *registry.Node) string {
	switch node.Type {
	case registry.NodeTypeOllama:
		return fmt.Sprintf("http://%s:%d", node.Host, node.Port)
	case registry.NodeTypeVLLM:
		return fmt.Sprintf("http://%s:%d", node.Host, node.Port)
	case registry.NodeTypeOpenAI:
		return fmt.Sprintf("https://%s:%d", node.Host, node.Port)
	default:
		return fmt.Sprintf("http://%s:%d", node.Host, node.Port)
	}
}
