package registry

import (
	"sync"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("Expected non-nil registry")
	}
	if reg.Count() != 0 {
		t.Errorf("Expected empty registry, got %d nodes", reg.Count())
	}
}

func TestRegistry_AddNode(t *testing.T) {
	reg := NewRegistry()

	node := &Node{
		Name: "test-node",
		Type: NodeTypeOllama,
		Host: "192.168.1.100",
		Port: 11434,
	}

	reg.AddNode(node)

	if reg.Count() != 1 {
		t.Errorf("Expected 1 node, got %d", reg.Count())
	}

	retrieved, exists := reg.GetNode("192.168.1.100", 11434)
	if !exists {
		t.Fatal("Expected node to exist")
	}
	if retrieved.Name != "test-node" {
		t.Errorf("Expected name 'test-node', got '%s'", retrieved.Name)
	}
	if retrieved.Type != NodeTypeOllama {
		t.Errorf("Expected type Ollama, got '%s'", retrieved.Type)
	}
}

func TestRegistry_AddNode_UpdatesExisting(t *testing.T) {
	reg := NewRegistry()

	node1 := &Node{
		Name:   "test-node",
		Type:   NodeTypeOllama,
		Host:   "192.168.1.100",
		Port:   11434,
		Status: NodeStatusUnknown,
	}
	reg.AddNode(node1)

	// Update with same host:port but different status
	node2 := &Node{
		Name:   "test-node-updated",
		Type:   NodeTypeOllama,
		Host:   "192.168.1.100",
		Port:   11434,
		Status: NodeStatusHealthy,
	}
	reg.AddNode(node2)

	if reg.Count() != 1 {
		t.Errorf("Expected 1 node (updated), got %d", reg.Count())
	}

	retrieved, _ := reg.GetNode("192.168.1.100", 11434)
	if retrieved.Name != "test-node-updated" {
		t.Errorf("Expected updated name, got '%s'", retrieved.Name)
	}
}

func TestRegistry_RemoveNode(t *testing.T) {
	reg := NewRegistry()

	node := &Node{
		Name: "test-node",
		Type: NodeTypeOllama,
		Host: "192.168.1.100",
		Port: 11434,
	}
	reg.AddNode(node)

	reg.RemoveNode("192.168.1.100", 11434)

	if reg.Count() != 0 {
		t.Errorf("Expected 0 nodes after removal, got %d", reg.Count())
	}

	_, exists := reg.GetNode("192.168.1.100", 11434)
	if exists {
		t.Error("Expected node to not exist after removal")
	}
}

func TestRegistry_UpdateNodeStatus(t *testing.T) {
	reg := NewRegistry()

	node := &Node{
		Name:   "test-node",
		Type:   NodeTypeOllama,
		Host:   "192.168.1.100",
		Port:   11434,
		Status: NodeStatusUnknown,
	}
	reg.AddNode(node)

	reg.UpdateNodeStatus("192.168.1.100", 11434, NodeStatusHealthy)

	retrieved, _ := reg.GetNode("192.168.1.100", 11434)
	if retrieved.Status != NodeStatusHealthy {
		t.Errorf("Expected status Healthy, got '%s'", retrieved.Status)
	}
}

func TestRegistry_IncrementErrorCount(t *testing.T) {
	reg := NewRegistry()

	node := &Node{
		Name:   "test-node",
		Type:   NodeTypeOllama,
		Host:   "192.168.1.100",
		Port:   11434,
		Status: NodeStatusHealthy,
	}
	reg.AddNode(node)

	// First two errors should not mark as unreachable
	shouldRemove := reg.IncrementErrorCount("192.168.1.100", 11434, 3)
	if shouldRemove {
		t.Error("Should not mark unreachable after 1 error")
	}

	shouldRemove = reg.IncrementErrorCount("192.168.1.100", 11434, 3)
	if shouldRemove {
		t.Error("Should not mark unreachable after 2 errors")
	}

	// Third error should mark as unreachable
	shouldRemove = reg.IncrementErrorCount("192.168.1.100", 11434, 3)
	if !shouldRemove {
		t.Error("Should mark unreachable after 3 errors")
	}

	retrieved, _ := reg.GetNode("192.168.1.100", 11434)
	if retrieved.Status != NodeStatusUnreachable {
		t.Errorf("Expected status Unreachable, got '%s'", retrieved.Status)
	}
}

func TestRegistry_GetAllNodes(t *testing.T) {
	reg := NewRegistry()

	nodes := []*Node{
		{Name: "node1", Type: NodeTypeOllama, Host: "192.168.1.100", Port: 11434},
		{Name: "node2", Type: NodeTypeVLLM, Host: "192.168.1.101", Port: 8000},
		{Name: "node3", Type: NodeTypeOpenAI, Host: "192.168.1.102", Port: 443},
	}

	for _, n := range nodes {
		reg.AddNode(n)
	}

	allNodes := reg.GetAllNodes()
	if len(allNodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(allNodes))
	}
}

func TestRegistry_GetNodesByType(t *testing.T) {
	reg := NewRegistry()

	nodes := []*Node{
		{Name: "ollama1", Type: NodeTypeOllama, Host: "192.168.1.100", Port: 11434},
		{Name: "ollama2", Type: NodeTypeOllama, Host: "192.168.1.101", Port: 11434},
		{Name: "vllm1", Type: NodeTypeVLLM, Host: "192.168.1.102", Port: 8000},
	}

	for _, n := range nodes {
		reg.AddNode(n)
	}

	ollamaNodes := reg.GetNodesByType(NodeTypeOllama)
	if len(ollamaNodes) != 2 {
		t.Errorf("Expected 2 Ollama nodes, got %d", len(ollamaNodes))
	}

	vllmNodes := reg.GetNodesByType(NodeTypeVLLM)
	if len(vllmNodes) != 1 {
		t.Errorf("Expected 1 vLLM node, got %d", len(vllmNodes))
	}
}

func TestRegistry_GetHealthyNodes(t *testing.T) {
	reg := NewRegistry()

	nodes := []*Node{
		{Name: "node1", Type: NodeTypeOllama, Host: "192.168.1.100", Port: 11434, Status: NodeStatusHealthy},
		{Name: "node2", Type: NodeTypeOllama, Host: "192.168.1.101", Port: 11434, Status: NodeStatusUnreachable},
		{Name: "node3", Type: NodeTypeOllama, Host: "192.168.1.102", Port: 11434, Status: NodeStatusHealthy},
	}

	for _, n := range nodes {
		reg.AddNode(n)
	}

	healthyNodes := reg.GetHealthyNodes()
	if len(healthyNodes) != 2 {
		t.Errorf("Expected 2 healthy nodes, got %d", len(healthyNodes))
	}
}

func TestRegistry_GetHealthyNodesByType(t *testing.T) {
	reg := NewRegistry()

	nodes := []*Node{
		{Name: "ollama1", Type: NodeTypeOllama, Host: "192.168.1.100", Port: 11434, Status: NodeStatusHealthy},
		{Name: "ollama2", Type: NodeTypeOllama, Host: "192.168.1.101", Port: 11434, Status: NodeStatusUnreachable},
		{Name: "vllm1", Type: NodeTypeVLLM, Host: "192.168.1.102", Port: 8000, Status: NodeStatusHealthy},
	}

	for _, n := range nodes {
		reg.AddNode(n)
	}

	healthyOllama := reg.GetHealthyNodesByType(NodeTypeOllama)
	if len(healthyOllama) != 1 {
		t.Errorf("Expected 1 healthy Ollama node, got %d", len(healthyOllama))
	}
}

func TestRegistry_OnEvent(t *testing.T) {
	reg := NewRegistry()

	var receivedEvents []Event
	var mutex sync.Mutex

	reg.OnEvent(func(e Event) {
		mutex.Lock()
		receivedEvents = append(receivedEvents, e)
		mutex.Unlock()
	})

	node := &Node{
		Name:   "test-node",
		Type:   NodeTypeOllama,
		Host:   "192.168.1.100",
		Port:   11434,
		Status: NodeStatusUnknown,
	}

	// Add node - should trigger NodeDiscovered
	reg.AddNode(node)
	time.Sleep(10 * time.Millisecond) // Allow goroutine to execute

	mutex.Lock()
	if len(receivedEvents) != 1 {
		t.Errorf("Expected 1 event, got %d", len(receivedEvents))
	}
	if len(receivedEvents) > 0 && receivedEvents[0].Type != EventNodeDiscovered {
		t.Errorf("Expected NodeDiscovered event, got %s", receivedEvents[0].Type)
	}
	mutex.Unlock()

	// Update status to healthy - should trigger HealthOK
	reg.UpdateNodeStatus("192.168.1.100", 11434, NodeStatusHealthy)
	time.Sleep(10 * time.Millisecond)

	mutex.Lock()
	if len(receivedEvents) != 2 {
		t.Errorf("Expected 2 events, got %d", len(receivedEvents))
	}
	if len(receivedEvents) > 1 && receivedEvents[1].Type != EventHealthOK {
		t.Errorf("Expected HealthOK event, got %s", receivedEvents[1].Type)
	}
	mutex.Unlock()
}

func TestRegistry_Clear(t *testing.T) {
	reg := NewRegistry()

	nodes := []*Node{
		{Name: "node1", Type: NodeTypeOllama, Host: "192.168.1.100", Port: 11434},
		{Name: "node2", Type: NodeTypeVLLM, Host: "192.168.1.101", Port: 8000},
	}

	for _, n := range nodes {
		reg.AddNode(n)
	}

	if reg.Count() != 2 {
		t.Errorf("Expected 2 nodes before clear, got %d", reg.Count())
	}

	reg.Clear()

	if reg.Count() != 0 {
		t.Errorf("Expected 0 nodes after clear, got %d", reg.Count())
	}
}

func TestRegistry_Concurrency(t *testing.T) {
	reg := NewRegistry()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := &Node{
				Name: "node",
				Type: NodeTypeOllama,
				Host: "192.168.1.100",
				Port: 11434 + i,
			}
			reg.AddNode(node)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.GetAllNodes()
			reg.GetHealthyNodes()
			reg.Count()
		}()
	}

	wg.Wait()

	// Verify no panics occurred and data is consistent
	if reg.Count() != numGoroutines {
		t.Errorf("Expected %d nodes, got %d", numGoroutines, reg.Count())
	}
}
