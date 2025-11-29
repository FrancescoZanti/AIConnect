package loadbalancer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func newTestLogger() *logrus.Logger {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel) // Suppress logs during tests
	return log
}

func TestNewOllamaLoadBalancer(t *testing.T) {
	servers := []string{"http://server1:11434", "http://server2:11434"}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

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

func TestOllamaLoadBalancer_SelectServer_RoundRobin(t *testing.T) {
	servers := []string{"http://server1:11434", "http://server2:11434", "http://server3:11434"}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

	// Without metrics (LastCheck is zero), should use round-robin fallback
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

func TestOllamaLoadBalancer_SelectServer_WeightedLeastLoad(t *testing.T) {
	servers := []string{"http://server1:11434", "http://server2:11434"}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

	// Set metrics manually to simulate weighted load
	lb.mutex.Lock()
	lb.metrics["http://server1:11434"] = &ServerMetrics{
		URL:         "http://server1:11434",
		CPUPercent:  80.0,
		RAMPercent:  70.0,
		TotalWeight: 150.0,
		Available:   true,
		LastCheck:   time.Now(),
	}
	lb.metrics["http://server2:11434"] = &ServerMetrics{
		URL:         "http://server2:11434",
		CPUPercent:  20.0,
		RAMPercent:  30.0,
		TotalWeight: 50.0,
		Available:   true,
		LastCheck:   time.Now(),
	}
	lb.mutex.Unlock()

	// Should always select server2 (lower weight)
	for i := 0; i < 5; i++ {
		server, err := lb.SelectServer()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if server != "http://server2:11434" {
			t.Errorf("Expected server2 (lower weight), got %s", server)
		}
	}
}

func TestOllamaLoadBalancer_SelectServer_NoAvailableServers(t *testing.T) {
	servers := []string{"http://server1:11434", "http://server2:11434"}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

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

func TestOllamaLoadBalancer_HandleServerError(t *testing.T) {
	servers := []string{"http://server1:11434"}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

	serverURL := "http://server1:11434"
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

func TestOllamaLoadBalancer_CheckServer_Success(t *testing.T) {
	// Create a mock server that returns metrics
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		response := map[string]interface{}{
			"cpu_percent":                 45.5,
			"ram_percent":                 60.0,
			"gpu_count":                   2,
			"gpu_avg_utilization_percent": 30.0,
			"gpu_avg_memory_percent":      40.0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

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
	// Expected: 45.5 + 60.0 + (30.0 * 1.5) + (40.0 * 1.5) = 45.5 + 60.0 + 45.0 + 60.0 = 210.5
	expectedWeight := 45.5 + 60.0 + (30.0 * 1.5) + (40.0 * 1.5)
	if metrics.TotalWeight != expectedWeight {
		t.Errorf("Expected weight %f, got %f", expectedWeight, metrics.TotalWeight)
	}
	if metrics.ErrorCount != 0 {
		t.Errorf("Expected error count 0, got %d", metrics.ErrorCount)
	}
}

func TestOllamaLoadBalancer_CheckServer_NoGPU(t *testing.T) {
	// Create a mock server that returns metrics without GPU
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		response := map[string]interface{}{
			"cpu_percent":                 50.0,
			"ram_percent":                 40.0,
			"gpu_count":                   0,
			"gpu_avg_utilization_percent": 0.0,
			"gpu_avg_memory_percent":      0.0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

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

func TestOllamaLoadBalancer_CheckServer_HTTPError(t *testing.T) {
	// Create a mock server that returns an error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

	lb.checkServer(mockServer.URL)

	lb.mutex.RLock()
	metrics := lb.metrics[mockServer.URL]
	lb.mutex.RUnlock()

	if metrics.ErrorCount != 1 {
		t.Errorf("Expected error count 1, got %d", metrics.ErrorCount)
	}
}

func TestOllamaLoadBalancer_CheckServer_InvalidJSON(t *testing.T) {
	// Create a mock server that returns invalid JSON
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

	lb.checkServer(mockServer.URL)

	lb.mutex.RLock()
	metrics := lb.metrics[mockServer.URL]
	lb.mutex.RUnlock()

	if metrics.ErrorCount != 1 {
		t.Errorf("Expected error count 1 for invalid JSON, got %d", metrics.ErrorCount)
	}
}

func TestOllamaLoadBalancer_GetMetrics(t *testing.T) {
	servers := []string{"http://server1:11434", "http://server2:11434"}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

	// Set some metrics
	lb.mutex.Lock()
	lb.metrics["http://server1:11434"].CPUPercent = 50.0
	lb.metrics["http://server1:11434"].RAMPercent = 60.0
	lb.mutex.Unlock()

	metrics := lb.GetMetrics()

	if len(metrics) != 2 {
		t.Errorf("Expected 2 metrics entries, got %d", len(metrics))
	}

	// Verify data is copied (not reference to original)
	lb.mutex.Lock()
	lb.metrics["http://server1:11434"].CPUPercent = 100.0
	lb.mutex.Unlock()

	if metrics["http://server1:11434"].CPUPercent != 50.0 {
		t.Error("GetMetrics should return a copy, not reference to original data")
	}
}

func TestOllamaLoadBalancer_SelectServer_OnlyAvailableServerWithMetrics(t *testing.T) {
	servers := []string{"http://server1:11434", "http://server2:11434"}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

	// Set server1 as unavailable, server2 with metrics
	lb.mutex.Lock()
	lb.metrics["http://server1:11434"].Available = false
	lb.metrics["http://server2:11434"] = &ServerMetrics{
		URL:         "http://server2:11434",
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
	if server != "http://server2:11434" {
		t.Errorf("Expected server2 (only available), got %s", server)
	}
}

func TestOllamaLoadBalancer_ErrorCountReset(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"cpu_percent": 50.0,
			"ram_percent": 50.0,
			"gpu_count":   0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	servers := []string{mockServer.URL}
	log := newTestLogger()

	lb := NewOllamaLoadBalancer(servers, 30, log)

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
