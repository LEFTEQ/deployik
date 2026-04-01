package ws

import "sync"

// LogLine represents a single build log line for broadcasting.
type LogLine struct {
	DeploymentID string `json:"deployment_id"`
	LineNumber   int    `json:"line_number"`
	Content      string `json:"content"`
	Stream       string `json:"stream"`
}

// Hub manages WebSocket subscribers per deployment.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan LogLine]struct{}
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]map[chan LogLine]struct{}),
	}
}

// Subscribe creates a channel that receives log lines for a deployment.
func (h *Hub) Subscribe(deploymentID string) chan LogLine {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan LogLine, 64)
	if h.subscribers[deploymentID] == nil {
		h.subscribers[deploymentID] = make(map[chan LogLine]struct{})
	}
	h.subscribers[deploymentID][ch] = struct{}{}
	return ch
}

// Unsubscribe removes a channel from a deployment's subscribers.
func (h *Hub) Unsubscribe(deploymentID string, ch chan LogLine) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subs, ok := h.subscribers[deploymentID]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(h.subscribers, deploymentID)
		}
	}
	close(ch)
}

// Publish sends a log line to all subscribers of a deployment.
func (h *Hub) Publish(line LogLine) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if subs, ok := h.subscribers[line.DeploymentID]; ok {
		for ch := range subs {
			select {
			case ch <- line:
			default:
				// Drop if subscriber is too slow
			}
		}
	}
}
