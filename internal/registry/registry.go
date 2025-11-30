package registry

import (
	"fmt"
	"sync"
	"time"
)

// NodeType represents the type of LLM backend
type NodeType string

const (
	NodeTypeOllama NodeType = "ollama"
	NodeTypeVLLM   NodeType = "vllm"
	NodeTypeOpenAI NodeType = "openai"
)

// NodeStatus represents the health status of a node
type NodeStatus string

const (
	NodeStatusHealthy     NodeStatus = "healthy"
	NodeStatusUnreachable NodeStatus = "unreachable"
	NodeStatusUnknown     NodeStatus = "unknown"
)

// Node represents a discovered LLM backend
type Node struct {
	Name     string     `json:"name"`
	Type     NodeType   `json:"type"`
	Host     string     `json:"host"`
	Port     int        `json:"port"`
	Status   NodeStatus `json:"status"`
	LastSeen time.Time  `json:"last_seen"`
	// Internal tracking
	ErrorCount int `json:"-"`
}

// EventType represents the type of registry event
type EventType string

const (
	EventNodeDiscovered EventType = "NodeDiscovered"
	EventNodeLost       EventType = "NodeLost"
	EventHealthOK       EventType = "HealthOK"
	EventHealthFail     EventType = "HealthFail"
)

// Event represents a registry event
type Event struct {
	Type      EventType
	Node      *Node
	Timestamp time.Time
}

// EventCallback is a function called when an event occurs
type EventCallback func(Event)

// Registry manages discovered LLM nodes
type Registry struct {
	nodes     map[string]*Node // key is "host:port"
	mutex     sync.RWMutex
	callbacks []EventCallback
}

// NewRegistry creates a new node registry
func NewRegistry() *Registry {
	return &Registry{
		nodes:     make(map[string]*Node),
		callbacks: make([]EventCallback, 0),
	}
}

// nodeKey generates a unique key for a node
func nodeKey(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}

// nodeKeyFromNode generates a unique key from a node
func nodeKeyFromNode(n *Node) string {
	return fmt.Sprintf("%s:%d", n.Host, n.Port)
}

// OnEvent registers an event callback
func (r *Registry) OnEvent(callback EventCallback) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.callbacks = append(r.callbacks, callback)
}

// emit emits an event to all registered callbacks
func (r *Registry) emit(eventType EventType, node *Node) {
	event := Event{
		Type:      eventType,
		Node:      node,
		Timestamp: time.Now(),
	}
	r.mutex.RLock()
	callbacks := make([]EventCallback, len(r.callbacks))
	copy(callbacks, r.callbacks)
	r.mutex.RUnlock()
	for _, cb := range callbacks {
		go func(callback EventCallback) {
			defer func() {
				if rec := recover(); rec != nil {
					// Log panic in callback but don't crash
					// Callback error is silently ignored to prevent application crash
				}
			}()
			callback(event)
		}(cb)
	}
}

// AddNode adds or updates a node in the registry
func (r *Registry) AddNode(node *Node) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	key := nodeKeyFromNode(node)
	_, exists := r.nodes[key]

	node.LastSeen = time.Now()
	if node.Status == "" {
		node.Status = NodeStatusUnknown
	}
	r.nodes[key] = node

	if !exists {
		r.emit(EventNodeDiscovered, node)
	}
}

// RemoveNode removes a node from the registry
func (r *Registry) RemoveNode(host string, port int) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	key := nodeKey(host, port)
	if node, exists := r.nodes[key]; exists {
		delete(r.nodes, key)
		r.emit(EventNodeLost, node)
	}
}

// UpdateNodeStatus updates the status of a node
func (r *Registry) UpdateNodeStatus(host string, port int, status NodeStatus) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	key := nodeKey(host, port)
	if node, exists := r.nodes[key]; exists {
		oldStatus := node.Status
		node.Status = status
		node.LastSeen = time.Now()

		if status == NodeStatusHealthy && oldStatus != NodeStatusHealthy {
			node.ErrorCount = 0
			r.emit(EventHealthOK, node)
		} else if status == NodeStatusUnreachable && oldStatus == NodeStatusHealthy {
			r.emit(EventHealthFail, node)
		}
	}
}

// IncrementErrorCount increments the error count for a node
// Returns true if the node should be marked as unreachable
func (r *Registry) IncrementErrorCount(host string, port int, maxErrors int) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	key := nodeKey(host, port)
	if node, exists := r.nodes[key]; exists {
		node.ErrorCount++
		if node.ErrorCount >= maxErrors {
			node.Status = NodeStatusUnreachable
			r.emit(EventHealthFail, node)
			return true
		}
	}
	return false
}

// GetNode returns a node by host and port
func (r *Registry) GetNode(host string, port int) (*Node, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	key := nodeKey(host, port)
	node, exists := r.nodes[key]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	nodeCopy := *node
	return &nodeCopy, true
}

// GetAllNodes returns all registered nodes
func (r *Registry) GetAllNodes() []*Node {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	nodes := make([]*Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		// Return a copy to avoid race conditions
		nodeCopy := *node
		nodes = append(nodes, &nodeCopy)
	}
	return nodes
}

// GetNodesByType returns all nodes of a specific type
func (r *Registry) GetNodesByType(nodeType NodeType) []*Node {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if node.Type == nodeType {
			nodeCopy := *node
			nodes = append(nodes, &nodeCopy)
		}
	}
	return nodes
}

// GetHealthyNodes returns all healthy nodes
func (r *Registry) GetHealthyNodes() []*Node {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if node.Status == NodeStatusHealthy {
			nodeCopy := *node
			nodes = append(nodes, &nodeCopy)
		}
	}
	return nodes
}

// GetHealthyNodesByType returns all healthy nodes of a specific type
func (r *Registry) GetHealthyNodesByType(nodeType NodeType) []*Node {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if node.Type == nodeType && node.Status == NodeStatusHealthy {
			nodeCopy := *node
			nodes = append(nodes, &nodeCopy)
		}
	}
	return nodes
}

// Count returns the number of registered nodes
func (r *Registry) Count() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.nodes)
}

// Clear removes all nodes from the registry
func (r *Registry) Clear() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.nodes = make(map[string]*Node)
}
