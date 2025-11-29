package loadbalancer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewVLLMLoadBalancer(t *testing.T) {
	servers := []string{"http://vllm1:8000", "http://vllm2:8000"}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	if lb == nil {
		t.Fatal("Expected non-nil load balancer")
	}

	if len(lb.servers) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(lb.servers))
	}

	if len(lb.metrics) != 2 {
		t.Errorf("Expected 2 metrics entries, got %d", len(lb.metrics))
	}

	// Verify initial metrics state
	for _, server := range servers {
		metrics, exists := lb.metrics[server]
		if !exists {
			t.Errorf("Expected metrics for server %s", server)
			continue
		}
		if !metrics.Available {
			t.Errorf("Expected server %s to be initially available", server)
		}
		if metrics.URL != server {
			t.Errorf("Expected URL %s, got %s", server, metrics.URL)
		}
	}
}

func TestVLLMLoadBalancer_SelectServer_RoundRobin(t *testing.T) {
	servers := []string{"http://vllm1:8000", "http://vllm2:8000", "http://vllm3:8000"}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	// Without metrics (LastCheck is zero or TotalWeight is 0), should use round-robin fallback
	// Due to map iteration order being non-deterministic, we just verify:
	// 1. Servers are selected without error
	// 2. All selected servers are from the available pool
	selectedServers := make(map[string]int)
	for i := 0; i < 6; i++ {
		server, err := lb.SelectServer()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		selectedServers[server]++
	}

	// Verify all selections are from the server pool
	for server := range selectedServers {
		found := false
		for _, s := range servers {
			if s == server {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Selected server %s is not in the server pool", server)
		}
	}

	// Verify we got some selections
	if len(selectedServers) == 0 {
		t.Error("No servers were selected")
	}
}

func TestVLLMLoadBalancer_SelectServer_WeightedLeastLoad(t *testing.T) {
	servers := []string{"http://vllm1:8000", "http://vllm2:8000"}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	// Set metrics manually to simulate weighted load
	lb.mutex.Lock()
	lb.metrics["http://vllm1:8000"] = &ServerMetrics{
		URL:         "http://vllm1:8000",
		CPUPercent:  80.0,
		RAMPercent:  70.0,
		TotalWeight: 150.0,
		Available:   true,
		LastCheck:   time.Now(),
	}
	lb.metrics["http://vllm2:8000"] = &ServerMetrics{
		URL:         "http://vllm2:8000",
		CPUPercent:  20.0,
		RAMPercent:  30.0,
		TotalWeight: 50.0,
		Available:   true,
		LastCheck:   time.Now(),
	}
	lb.mutex.Unlock()

	// Should always select vllm2 (lower weight)
	for i := 0; i < 5; i++ {
		server, err := lb.SelectServer()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if server != "http://vllm2:8000" {
			t.Errorf("Expected vllm2 (lower weight), got %s", server)
		}
	}
}

func TestVLLMLoadBalancer_SelectServer_NoAvailableServers(t *testing.T) {
	servers := []string{"http://vllm1:8000", "http://vllm2:8000"}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	// Mark all servers as unavailable
	lb.mutex.Lock()
	for _, server := range servers {
		lb.metrics[server].Available = false
	}
	lb.mutex.Unlock()

	_, err := lb.SelectServer()
	if err == nil {
		t.Error("Expected error when no servers available")
	}
}

func TestVLLMLoadBalancer_HandleServerError(t *testing.T) {
	servers := []string{"http://vllm1:8000"}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	serverURL := "http://vllm1:8000"
	testError := fmt.Errorf("connection refused")

	// Simulate consecutive errors
	for i := 1; i <= 3; i++ {
		lb.handleServerError(serverURL, testError)

		lb.mutex.RLock()
		metrics := lb.metrics[serverURL]
		errorCount := metrics.ErrorCount
		available := metrics.Available
		lb.mutex.RUnlock()

		if errorCount != i {
			t.Errorf("Expected error count %d, got %d", i, errorCount)
		}

		// Server should become unavailable after 3 consecutive errors
		if i < 3 && !available {
			t.Errorf("Server should still be available after %d errors", i)
		}
		if i >= 3 && available {
			t.Errorf("Server should be unavailable after %d errors", i)
		}
	}
}

func TestVLLMLoadBalancer_CheckServer_HealthAndMetrics(t *testing.T) {
	// Create a mock server that returns health and metrics
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/metrics":
			response := map[string]interface{}{
				"cpu_percent":                 45.5,
				"ram_percent":                 60.0,
				"gpu_count":                   2,
				"gpu_avg_utilization_percent": 30.0,
				"gpu_avg_memory_percent":      40.0,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	// Trigger server check
	lb.checkServer(mockServer.URL)

	// Verify metrics were updated
	lb.mutex.RLock()
	metrics := lb.metrics[mockServer.URL]
	lb.mutex.RUnlock()

	if !metrics.Available {
		t.Error("Expected server to be available")
	}
	if metrics.CPUPercent != 45.5 {
		t.Errorf("Expected CPU 45.5, got %f", metrics.CPUPercent)
	}
	if metrics.RAMPercent != 60.0 {
		t.Errorf("Expected RAM 60.0, got %f", metrics.RAMPercent)
	}
	if metrics.GPUCount != 2 {
		t.Errorf("Expected GPU count 2, got %d", metrics.GPUCount)
	}
	if metrics.GPUAvgUtil != 30.0 {
		t.Errorf("Expected GPU util 30.0, got %f", metrics.GPUAvgUtil)
	}
	if metrics.GPUAvgMemory != 40.0 {
		t.Errorf("Expected GPU memory 40.0, got %f", metrics.GPUAvgMemory)
	}

	// Check weight calculation: CPU + RAM + (GPU util * 1.5) + (GPU mem * 1.5)
	expectedWeight := 45.5 + 60.0 + (30.0 * 1.5) + (40.0 * 1.5)
	if metrics.TotalWeight != expectedWeight {
		t.Errorf("Expected weight %f, got %f", expectedWeight, metrics.TotalWeight)
	}
	if metrics.ErrorCount != 0 {
		t.Errorf("Expected error count 0, got %d", metrics.ErrorCount)
	}
}

func TestVLLMLoadBalancer_CheckServer_HealthOnlyNoMetrics(t *testing.T) {
	// Create a mock server that returns health but no metrics endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	lb.checkServer(mockServer.URL)

	lb.mutex.RLock()
	metrics := lb.metrics[mockServer.URL]
	lb.mutex.RUnlock()

	// Server should be available even without detailed metrics
	if !metrics.Available {
		t.Error("Expected server to be available (health check passed)")
	}
	if metrics.ErrorCount != 0 {
		t.Errorf("Expected error count 0, got %d", metrics.ErrorCount)
	}
}

func TestVLLMLoadBalancer_CheckServer_HealthFailure(t *testing.T) {
	// Create a mock server that returns health failure
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	lb.checkServer(mockServer.URL)

	lb.mutex.RLock()
	metrics := lb.metrics[mockServer.URL]
	lb.mutex.RUnlock()

	if metrics.ErrorCount != 1 {
		t.Errorf("Expected error count 1, got %d", metrics.ErrorCount)
	}
}

func TestVLLMLoadBalancer_CheckServer_NoGPU(t *testing.T) {
	// Create a mock server that returns metrics without GPU
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/metrics":
			response := map[string]interface{}{
				"cpu_percent":                 50.0,
				"ram_percent":                 40.0,
				"gpu_count":                   0,
				"gpu_avg_utilization_percent": 0.0,
				"gpu_avg_memory_percent":      0.0,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	lb.checkServer(mockServer.URL)

	lb.mutex.RLock()
	metrics := lb.metrics[mockServer.URL]
	lb.mutex.RUnlock()

	// Without GPU, weight should be just CPU + RAM
	expectedWeight := 50.0 + 40.0
	if metrics.TotalWeight != expectedWeight {
		t.Errorf("Expected weight %f (no GPU), got %f", expectedWeight, metrics.TotalWeight)
	}
}

func TestVLLMLoadBalancer_CheckServer_InvalidMetricsJSON(t *testing.T) {
	// Create a mock server that returns health OK but invalid JSON for metrics
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/metrics":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("not valid json"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	lb.checkServer(mockServer.URL)

	lb.mutex.RLock()
	metrics := lb.metrics[mockServer.URL]
	lb.mutex.RUnlock()

	// Server should still be available (health check passed)
	// Even if metrics JSON is invalid, the fallback logic makes server available
	if !metrics.Available {
		t.Error("Expected server to be available (health check passed, metrics fallback)")
	}
	if metrics.ErrorCount != 0 {
		t.Errorf("Expected error count 0 (health OK), got %d", metrics.ErrorCount)
	}
}

func TestVLLMLoadBalancer_GetMetrics(t *testing.T) {
	servers := []string{"http://vllm1:8000", "http://vllm2:8000"}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	// Set some metrics
	lb.mutex.Lock()
	lb.metrics["http://vllm1:8000"].CPUPercent = 50.0
	lb.metrics["http://vllm1:8000"].RAMPercent = 60.0
	lb.mutex.Unlock()

	metrics := lb.GetMetrics()

	if len(metrics) != 2 {
		t.Errorf("Expected 2 metrics entries, got %d", len(metrics))
	}

	// Verify data is copied (not reference to original)
	lb.mutex.Lock()
	lb.metrics["http://vllm1:8000"].CPUPercent = 100.0
	lb.mutex.Unlock()

	if metrics["http://vllm1:8000"].CPUPercent != 50.0 {
		t.Error("GetMetrics should return a copy, not reference to original data")
	}
}

func TestVLLMLoadBalancer_SelectServer_OnlyAvailableServerWithMetrics(t *testing.T) {
	servers := []string{"http://vllm1:8000", "http://vllm2:8000"}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	// Set vllm1 as unavailable, vllm2 with metrics
	lb.mutex.Lock()
	lb.metrics["http://vllm1:8000"].Available = false
	lb.metrics["http://vllm2:8000"] = &ServerMetrics{
		URL:         "http://vllm2:8000",
		CPUPercent:  30.0,
		RAMPercent:  40.0,
		TotalWeight: 70.0,
		Available:   true,
		LastCheck:   time.Now(),
	}
	lb.mutex.Unlock()

	server, err := lb.SelectServer()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if server != "http://vllm2:8000" {
		t.Errorf("Expected vllm2 (only available), got %s", server)
	}
}

func TestVLLMLoadBalancer_ErrorCountReset(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/metrics":
			response := map[string]interface{}{
				"cpu_percent": 50.0,
				"ram_percent": 50.0,
				"gpu_count":   0,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	testError := fmt.Errorf("test error")
	// Simulate some errors
	lb.handleServerError(mockServer.URL, testError)
	lb.handleServerError(mockServer.URL, testError)

	lb.mutex.RLock()
	errorCountBefore := lb.metrics[mockServer.URL].ErrorCount
	lb.mutex.RUnlock()

	if errorCountBefore != 2 {
		t.Errorf("Expected error count 2 before recovery, got %d", errorCountBefore)
	}

	// Successful check should reset error count
	lb.checkServer(mockServer.URL)

	lb.mutex.RLock()
	errorCountAfter := lb.metrics[mockServer.URL].ErrorCount
	lb.mutex.RUnlock()

	if errorCountAfter != 0 {
		t.Errorf("Expected error count 0 after successful check, got %d", errorCountAfter)
	}
}

func TestVLLMLoadBalancer_SelectServer_FallbackWithZeroWeight(t *testing.T) {
	servers := []string{"http://vllm1:8000", "http://vllm2:8000"}
	log := newTestLogger()

	lb := NewVLLMLoadBalancer(servers, 30, log)

	// Set metrics with TotalWeight = 0 (should trigger round-robin fallback in vLLM)
	lb.mutex.Lock()
	lb.metrics["http://vllm1:8000"] = &ServerMetrics{
		URL:         "http://vllm1:8000",
		TotalWeight: 0,
		Available:   true,
		LastCheck:   time.Now(),
	}
	lb.metrics["http://vllm2:8000"] = &ServerMetrics{
		URL:         "http://vllm2:8000",
		TotalWeight: 0,
		Available:   true,
		LastCheck:   time.Now(),
	}
	lb.mutex.Unlock()

	// With TotalWeight = 0 for both, vLLM's SelectServer uses round-robin fallback
	// Due to map iteration order being non-deterministic, we just verify:
	// 1. Servers are selected without error
	// 2. All selected servers are from the available pool
	selectedServers := make(map[string]int)
	for i := 0; i < 4; i++ {
		server, err := lb.SelectServer()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		selectedServers[server]++
	}

	// Verify all selections are from the server pool
	for server := range selectedServers {
		found := false
		for _, s := range servers {
			if s == server {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Selected server %s is not in the server pool", server)
		}
	}

	// Verify at least one server was selected
	if len(selectedServers) == 0 {
		t.Error("No servers were selected")
	}
}
