package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config rappresenta la configurazione completa dell'applicazione
type Config struct {
	AD struct {
		Enabled       *bool    `yaml:"enabled"`
		LDAPURL       string   `yaml:"ldap_url"`
		BindDN        string   `yaml:"bind_dn"`
		BindPassword  string   `yaml:"bind_password"`
		BaseDN        string   `yaml:"base_dn"`
		AllowedGroups []string `yaml:"allowed_groups"`
		PublicPaths   []string `yaml:"public_paths"`
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

	applyDefaults(&cfg)
	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	// AD enabled defaults to true for backward compatibility
	if cfg.AD.Enabled == nil {
		defaultEnabled := true
		cfg.AD.Enabled = &defaultEnabled
	}
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
}

func Validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config nil")
	}
	applyDefaults(cfg)

	if strings.TrimSpace(cfg.HTTPS.Domain) == "" {
		return errors.New("https.domain obbligatorio")
	}
	if strings.TrimSpace(cfg.HTTPS.CacheDir) == "" {
		return errors.New("https.cache_dir obbligatorio")
	}
	if cfg.HTTPS.Port <= 0 {
		return errors.New("https.port non valido")
	}

	adEnabled := cfg.AD.Enabled == nil || *cfg.AD.Enabled
	if adEnabled {
		if strings.TrimSpace(cfg.AD.LDAPURL) == "" {
			return errors.New("ad.ldap_url obbligatorio (quando AD è abilitato)")
		}
		if strings.TrimSpace(cfg.AD.BaseDN) == "" {
			return errors.New("ad.base_dn obbligatorio (quando AD è abilitato)")
		}
		if len(cfg.AD.AllowedGroups) == 0 {
			return errors.New("ad.allowed_groups obbligatorio (quando AD è abilitato)")
		}
		// BindDN/BindPassword possono essere opzionali in alcune configurazioni (anonymous bind),
		// ma in ambienti AD tipici servono; li lasciamo configurabili nel wizard.
	}

	if len(cfg.Backends.OllamaServers) == 0 && len(cfg.Backends.VLLMServers) == 0 && strings.TrimSpace(cfg.Backends.OpenAIEndpoint) == "" {
		return errors.New("almeno un backend deve essere configurato (ollama_servers, vllm_servers o openai_endpoint)")
	}
	if strings.TrimSpace(cfg.Backends.OpenAIEndpoint) != "" && strings.TrimSpace(cfg.Backends.OpenAIAPIKey) == "" {
		return errors.New("openai_api_key obbligatoria quando openai_endpoint è configurato")
	}

	if IsPlaceholderConfig(cfg) {
		return errors.New("config sembra un esempio non compilato (placeholder)")
	}

	return nil
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("config nil")
	}
	applyDefaults(cfg)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("errore serializzazione YAML: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("errore creazione directory config: %w", err)
	}

	// Scrive atomico: file temporaneo + rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("errore scrittura config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("errore rename config: %w", err)
	}

	return nil
}

func IsPlaceholderConfig(cfg *Config) bool {
	if cfg == nil {
		return false
	}

	// Riconoscimento volutamente conservativo: match su valori esatti del config.example.yaml
	if strings.TrimSpace(cfg.AD.LDAPURL) == "ldap://ad.example.com:389" {
		return true
	}
	if strings.TrimSpace(cfg.AD.BindPassword) == "your-service-account-password" {
		return true
	}
	if strings.TrimSpace(cfg.AD.BaseDN) == "DC=example,DC=com" {
		return true
	}
	if strings.TrimSpace(cfg.Backends.OpenAIAPIKey) == "sk-your-openai-api-key-here" {
		return true
	}
	if strings.TrimSpace(cfg.HTTPS.Domain) == "aiconnect.example.com" {
		return true
	}
	return false
}
