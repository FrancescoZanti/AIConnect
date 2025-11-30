package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config rappresenta la configurazione completa dell'applicazione
type Config struct {
	AD struct {
		LDAPURL       string   `yaml:"ldap_url"`
		BindDN        string   `yaml:"bind_dn"`
		BindPassword  string   `yaml:"bind_password"`
		BaseDN        string   `yaml:"base_dn"`
		AllowedGroups []string `yaml:"allowed_groups"`
	} `yaml:"ad"`

	Backends struct {
		OllamaServers  []string `yaml:"ollama_servers"`
		VLLMServers    []string `yaml:"vllm_servers"`
		OpenAIEndpoint string   `yaml:"openai_endpoint"`
		OpenAIAPIKey   string   `yaml:"openai_api_key"`
	} `yaml:"backends"`

	HTTPS struct {
		Domain   string `yaml:"domain"`
		CacheDir string `yaml:"cache_dir"`
		Port     int    `yaml:"port"`
	} `yaml:"https"`

	Monitoring struct {
		HealthCheckInterval int `yaml:"health_check_interval"`
		MetricsPort         int `yaml:"metrics_port"`
	} `yaml:"monitoring"`

	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"logging"`

	MDNS struct {
		Enabled           bool     `yaml:"enabled"`
		ServiceName       string   `yaml:"service_name"`
		Version           string   `yaml:"version"`
		Capabilities      string   `yaml:"capabilities"`
		DiscoveryEnabled  bool     `yaml:"discovery_enabled"`
		DiscoveryInterval int      `yaml:"discovery_interval"`
		DiscoveryTimeout  int      `yaml:"discovery_timeout"`
		ServiceTypes      []string `yaml:"service_types"`
	} `yaml:"mdns"`
}

// Load carica la configurazione dal file YAML specificato
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("errore lettura file config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("errore parsing YAML: %w", err)
	}

	// Valori di default
	if cfg.HTTPS.Port == 0 {
		cfg.HTTPS.Port = 443
	}
	if cfg.Monitoring.HealthCheckInterval == 0 {
		cfg.Monitoring.HealthCheckInterval = 30
	}
	if cfg.Monitoring.MetricsPort == 0 {
		cfg.Monitoring.MetricsPort = 9090
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}

	// mDNS defaults
	if cfg.MDNS.ServiceName == "" {
		cfg.MDNS.ServiceName = "AIConnect Orchestrator"
	}
	if cfg.MDNS.Version == "" {
		cfg.MDNS.Version = "1.0.0"
	}
	if cfg.MDNS.Capabilities == "" {
		cfg.MDNS.Capabilities = "ollama,vllm,openai"
	}
	if cfg.MDNS.DiscoveryInterval == 0 {
		cfg.MDNS.DiscoveryInterval = 30
	}
	if cfg.MDNS.DiscoveryTimeout == 0 {
		cfg.MDNS.DiscoveryTimeout = 5
	}
	if len(cfg.MDNS.ServiceTypes) == 0 {
		cfg.MDNS.ServiceTypes = []string{"_ollama._tcp", "_openai._tcp", "_vllm._tcp"}
	}

	return &cfg, nil
}
