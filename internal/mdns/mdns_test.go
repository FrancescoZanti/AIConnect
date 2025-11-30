package mdns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fzanti/aiconnect/internal/registry"
)

func TestNodesHandler_EmptyRegistry(t *testing.T) {
	reg := registry.NewRegistry()

	handler := NodesHandler(reg, "10.0.0.10", 9000)

	req, err := http.NewRequest("GET", "/internal/nodes", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, status)
	}

	var response NodesResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.AIConnect.Host != "10.0.0.10" {
		t.Errorf("Expected host '10.0.0.10', got '%s'", response.AIConnect.Host)
	}
	if response.AIConnect.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", response.AIConnect.Port)
	}
	if len(response.DiscoveredNodes) != 0 {
		t.Errorf("Expected 0 discovered nodes, got %d", len(response.DiscoveredNodes))
	}
}

func TestNodesHandler_WithNodes(t *testing.T) {
	reg := registry.NewRegistry()

	// Add some test nodes
	nodes := []*registry.Node{
		{
			Name:     "ollama-1",
			Type:     registry.NodeTypeOllama,
			Host:     "10.0.0.21",
			Port:     11434,
			Status:   registry.NodeStatusHealthy,
			LastSeen: time.Now(),
		},
		{
			Name:     "vllm-1",
			Type:     registry.NodeTypeVLLM,
			Host:     "10.0.0.22",
			Port:     8000,
			Status:   registry.NodeStatusHealthy,
			LastSeen: time.Now(),
		},
		{
			Name:     "ollama-2",
			Type:     registry.NodeTypeOllama,
			Host:     "10.0.0.23",
			Port:     11434,
			Status:   registry.NodeStatusUnreachable,
			LastSeen: time.Now(),
		},
	}

	for _, n := range nodes {
		reg.AddNode(n)
	}

	handler := NodesHandler(reg, "10.0.0.10", 9000)

	req, err := http.NewRequest("GET", "/internal/nodes", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, status)
	}

	var response NodesResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.DiscoveredNodes) != 3 {
		t.Errorf("Expected 3 discovered nodes, got %d", len(response.DiscoveredNodes))
	}

	// Find ollama-1 node
	var foundOllama1 bool
	for _, n := range response.DiscoveredNodes {
		if n.Name == "ollama-1" {
			foundOllama1 = true
			if n.Type != "ollama" {
				t.Errorf("Expected type 'ollama', got '%s'", n.Type)
			}
			if n.Host != "10.0.0.21" {
				t.Errorf("Expected host '10.0.0.21', got '%s'", n.Host)
			}
			if n.Port != 11434 {
				t.Errorf("Expected port 11434, got %d", n.Port)
			}
			if n.Status != "healthy" {
				t.Errorf("Expected status 'healthy', got '%s'", n.Status)
			}
			break
		}
	}
	if !foundOllama1 {
		t.Error("Expected to find ollama-1 node")
	}
}

func TestNodesHandler_ContentType(t *testing.T) {
	reg := registry.NewRegistry()
	handler := NodesHandler(reg, "localhost", 443)

	req, err := http.NewRequest("GET", "/internal/nodes", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestGetServiceURL(t *testing.T) {
	testCases := []struct {
		name     string
		node     *registry.Node
		expected string
	}{
		{
			name:     "Ollama node",
			node:     &registry.Node{Type: registry.NodeTypeOllama, Host: "192.168.1.100", Port: 11434},
			expected: "http://192.168.1.100:11434",
		},
		{
			name:     "vLLM node",
			node:     &registry.Node{Type: registry.NodeTypeVLLM, Host: "192.168.1.101", Port: 8000},
			expected: "http://192.168.1.101:8000",
		},
		{
			name:     "OpenAI node",
			node:     &registry.Node{Type: registry.NodeTypeOpenAI, Host: "api.openai.com", Port: 443},
			expected: "https://api.openai.com:443",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetServiceURL(tc.node)
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestNodeTypeToServiceType(t *testing.T) {
	testCases := []struct {
		nodeType    registry.NodeType
		serviceType string
	}{
		{registry.NodeTypeOllama, OllamaServiceType},
		{registry.NodeTypeVLLM, VLLMServiceType},
		{registry.NodeTypeOpenAI, OpenAIServiceType},
		{"unknown", ""},
	}

	for _, tc := range testCases {
		result := NodeTypeToServiceType(tc.nodeType)
		if result != tc.serviceType {
			t.Errorf("Expected '%s' for type '%s', got '%s'", tc.serviceType, tc.nodeType, result)
		}
	}
}

func TestServiceTypeToNodeType(t *testing.T) {
	testCases := []struct {
		serviceType string
		nodeType    registry.NodeType
	}{
		{OllamaServiceType, registry.NodeTypeOllama},
		{VLLMServiceType, registry.NodeTypeVLLM},
		{OpenAIServiceType, registry.NodeTypeOpenAI},
		{"_unknown._tcp", ""},
	}

	for _, tc := range testCases {
		result := serviceTypeToNodeType(tc.serviceType)
		if result != tc.nodeType {
			t.Errorf("Expected '%s' for service '%s', got '%s'", tc.nodeType, tc.serviceType, result)
		}
	}
}
