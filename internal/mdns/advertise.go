package mdns

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/grandcat/zeroconf"
	"github.com/sirupsen/logrus"
)

const (
	// AIConnectServiceType is the mDNS service type for AIConnect orchestrator
	AIConnectServiceType = "_aiconnect._tcp"

	// DefaultPort is the default port for AIConnect
	DefaultPort = 443
)

// AdvertiserConfig contains configuration for the mDNS advertiser
type AdvertiserConfig struct {
	// ServiceName is the name to advertise (e.g., "AIConnect Orchestrator")
	ServiceName string
	// Port is the port AIConnect is listening on
	Port int
	// Domain is the mDNS domain (default: "local.")
	Domain string
	// Version is the version of AIConnect
	Version string
	// Capabilities is a comma-separated list of supported backends
	Capabilities string
}

// DefaultAdvertiserConfig returns default advertiser configuration
func DefaultAdvertiserConfig() *AdvertiserConfig {
	return &AdvertiserConfig{
		ServiceName:  "AIConnect Orchestrator",
		Port:         DefaultPort,
		Domain:       "local.",
		Version:      "1.0.0",
		Capabilities: "ollama,vllm,openai",
	}
}

// Advertiser handles mDNS advertisement of AIConnect
type Advertiser struct {
	config *AdvertiserConfig
	log    *logrus.Logger
	server *zeroconf.Server
	ctx    context.Context
	cancel context.CancelFunc
}

// NewAdvertiser creates a new mDNS advertiser
func NewAdvertiser(config *AdvertiserConfig, log *logrus.Logger) *Advertiser {
	if config == nil {
		config = DefaultAdvertiserConfig()
	}
	if log == nil {
		log = logrus.New()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Advertiser{
		config: config,
		log:    log,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins advertising AIConnect via mDNS
func (a *Advertiser) Start() error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "aiconnect"
	}

	// Get local IPs for mDNS
	ips, err := getLocalIPs()
	if err != nil {
		a.log.WithError(err).Warn("Could not determine local IPs, using default")
	}

	// Build TXT records
	txtRecords := []string{
		fmt.Sprintf("version=%s", a.config.Version),
		fmt.Sprintf("capabilities=%s", a.config.Capabilities),
	}

	// Register the service
	server, err := zeroconf.Register(
		a.config.ServiceName, // Service instance name
		AIConnectServiceType, // Service type
		a.config.Domain,      // Domain
		a.config.Port,        // Port
		txtRecords,           // TXT records
		getInterfaces(ips),   // Interfaces to register on
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %w", err)
	}

	a.server = server

	a.log.WithFields(logrus.Fields{
		"service":      a.config.ServiceName,
		"type":         AIConnectServiceType,
		"port":         a.config.Port,
		"hostname":     hostname,
		"capabilities": a.config.Capabilities,
	}).Info("mDNS advertisement started")

	return nil
}

// Stop stops the mDNS advertisement
func (a *Advertiser) Stop() {
	a.cancel()
	if a.server != nil {
		a.server.Shutdown()
		a.log.Info("mDNS advertisement stopped")
	}
}

// getLocalIPs returns all local non-loopback IP addresses
func getLocalIPs() ([]net.IP, error) {
	var ips []net.IP

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP)
			}
		}
	}

	return ips, nil
}

// GetLocalIPs returns all local non-loopback IPv4 addresses as strings
func GetLocalIPs() []string {
	ips, err := getLocalIPs()
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(ips))
	for _, ip := range ips {
		result = append(result, ip.String())
	}
	return result
}

// getInterfaces returns network interfaces for mDNS registration
func getInterfaces(ips []net.IP) []net.Interface {
	if len(ips) == 0 {
		return nil // Use default interfaces
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var result []net.Interface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				for _, ip := range ips {
					if ipnet.IP.Equal(ip) {
						result = append(result, iface)
						break
					}
				}
			}
		}
	}

	return result
}
