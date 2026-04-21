package ws

import "sync"

// LogLine represents a single build log line for broadcasting.
type LogLine struct {
	DeploymentID string `json:"deployment_id"`
	LineNumber   int    `json:"line_number"`
	Content      string `json:"content"`
	Stream       string `json:"stream"`
}

// maxBufferedLines caps per-topic replay buffer size.
const maxBufferedLines = 500

// Hub manages WebSocket subscribers per deployment.
// It also keeps a bounded replay buffer per topic so that subscribers which
// connect after a publisher has started still see the session from the start.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan LogLine]struct{}
	buffer      map[string][]LogLine
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]map[chan LogLine]struct{}),
		buffer:      make(map[string][]LogLine),
	}
}

// Subscribe creates a channel that receives log lines for a topic. The channel
// is pre-seeded with any buffered events for that topic so late subscribers
// catch up on events published before they connected.
func (h *Hub) Subscribe(topic string) chan LogLine {
	h.mu.Lock()
	defer h.mu.Unlock()

	buffered := h.buffer[topic]
	// Size channel to comfortably hold the replay plus headroom for new events.
	ch := make(chan LogLine, len(buffered)+64)
	for _, line := range buffered {
		ch <- line
	}

	if h.subscribers[topic] == nil {
		h.subscribers[topic] = make(map[chan LogLine]struct{})
	}
	h.subscribers[topic][ch] = struct{}{}
	return ch
}

// Unsubscribe removes a channel from a topic's subscribers. When the last
// subscriber leaves we also drop the replay buffer to avoid unbounded growth.
func (h *Hub) Unsubscribe(topic string, ch chan LogLine) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subs, ok := h.subscribers[topic]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(h.subscribers, topic)
			delete(h.buffer, topic)
		}
	}
	close(ch)
}

// ResetBuffer clears the replay buffer for a topic. Publishers should call this
// when starting a new session so stale events from a prior session don't leak
// into the new one.
func (h *Hub) ResetBuffer(topic string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.buffer, topic)
}

// Publish appends the line to the topic's replay buffer and fan-outs to every
// live subscriber. Slow subscribers are dropped silently.
func (h *Hub) Publish(line LogLine) {
	h.mu.Lock()
	defer h.mu.Unlock()

	buf := h.buffer[line.DeploymentID]
	buf = append(buf, line)
	if len(buf) > maxBufferedLines {
		buf = buf[len(buf)-maxBufferedLines:]
	}
	h.buffer[line.DeploymentID] = buf

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
