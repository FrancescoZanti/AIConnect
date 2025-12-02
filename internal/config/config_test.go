package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ADEnabledDefaultsToTrue(t *testing.T) {
	// Create a temporary config file without 'enabled' field
	content := `
ad:
  ldap_url: "ldap://test.example.com:389"
  bind_dn: "CN=test,DC=example,DC=com"
  bind_password: "testpass"
  base_dn: "DC=example,DC=com"
  allowed_groups:
    - "CN=TestGroup,DC=example,DC=com"

backends:
  ollama_servers:
    - "http://ollama1:11434"
  openai_endpoint: "https://api.openai.com/v1"
  openai_api_key: "test-key"

https:
  domain: "test.example.com"
  cache_dir: "/tmp/test-cache"
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// AD.Enabled should default to true for backward compatibility
	if cfg.AD.Enabled == nil {
		t.Fatal("AD.Enabled should not be nil after loading")
	}
	if !*cfg.AD.Enabled {
		t.Error("AD.Enabled should default to true when not specified")
	}
}

func TestLoad_ADEnabledExplicitlyFalse(t *testing.T) {
	// Create a temporary config file with 'enabled: false'
	content := `
ad:
  enabled: false
  ldap_url: "ldap://test.example.com:389"
  bind_dn: "CN=test,DC=example,DC=com"
  bind_password: "testpass"
  base_dn: "DC=example,DC=com"

backends:
  ollama_servers:
    - "http://ollama1:11434"
  openai_endpoint: "https://api.openai.com/v1"
  openai_api_key: "test-key"

https:
  domain: "test.example.com"
  cache_dir: "/tmp/test-cache"
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// AD.Enabled should be explicitly false
	if cfg.AD.Enabled == nil {
		t.Fatal("AD.Enabled should not be nil after loading")
	}
	if *cfg.AD.Enabled {
		t.Error("AD.Enabled should be false when explicitly set to false")
	}
}

func TestLoad_ADEnabledExplicitlyTrue(t *testing.T) {
	// Create a temporary config file with 'enabled: true'
	content := `
ad:
  enabled: true
  ldap_url: "ldap://test.example.com:389"
  bind_dn: "CN=test,DC=example,DC=com"
  bind_password: "testpass"
  base_dn: "DC=example,DC=com"

backends:
  ollama_servers:
    - "http://ollama1:11434"
  openai_endpoint: "https://api.openai.com/v1"
  openai_api_key: "test-key"

https:
  domain: "test.example.com"
  cache_dir: "/tmp/test-cache"
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// AD.Enabled should be explicitly true
	if cfg.AD.Enabled == nil {
		t.Fatal("AD.Enabled should not be nil after loading")
	}
	if !*cfg.AD.Enabled {
		t.Error("AD.Enabled should be true when explicitly set to true")
	}
}

func TestLoad_PublicPaths(t *testing.T) {
	// Create a temporary config file with public_paths
	content := `
ad:
  enabled: true
  ldap_url: "ldap://test.example.com:389"
  bind_dn: "CN=test,DC=example,DC=com"
  bind_password: "testpass"
  base_dn: "DC=example,DC=com"
  public_paths:
    - "/ollama/*"
    - "/health"
    - "/vllm/"

backends:
  ollama_servers:
    - "http://ollama1:11434"
  openai_endpoint: "https://api.openai.com/v1"
  openai_api_key: "test-key"

https:
  domain: "test.example.com"
  cache_dir: "/tmp/test-cache"
`
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	expectedPaths := []string{"/ollama/*", "/health", "/vllm/"}
	if len(cfg.AD.PublicPaths) != len(expectedPaths) {
		t.Errorf("Expected %d public paths, got %d", len(expectedPaths), len(cfg.AD.PublicPaths))
	}

	for i, expected := range expectedPaths {
		if i < len(cfg.AD.PublicPaths) && cfg.AD.PublicPaths[i] != expected {
			t.Errorf("Expected public path %d to be %q, got %q", i, expected, cfg.AD.PublicPaths[i])
		}
	}
}
