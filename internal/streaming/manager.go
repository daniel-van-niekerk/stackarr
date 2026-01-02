package streaming

import (
	"sync"
	"time"
)

// ProgressEvent represents a single progress update
type ProgressEvent struct {
	Type      string      `json:"type"`       // image_pull, layer_download, container_create, container_start, complete, error
	Message   string      `json:"message"`    // Human-readable message
	Timestamp string      `json:"timestamp"`  // RFC3339 timestamp
	Data      interface{} `json:"data"`       // Type-specific data
}

// LayerProgress represents progress for a single image layer
type LayerProgress struct {
	LayerID  string `json:"layer_id"`
	Status   string `json:"status"`      // pulling, downloading, extracting, complete
	Current  int64  `json:"current"`     // Bytes downloaded/extracted
	Total    int64  `json:"total"`       // Total bytes
	Progress int    `json:"progress"`    // Percentage (0-100)
}

// OperationProgress tracks progress for a single operation
type OperationProgress struct {
	ID        string
	Clients   map[chan ProgressEvent]bool
	mutex     sync.RWMutex
	done      bool
	createdAt time.Time
}

// ProgressManager manages SSE connections and progress events
type ProgressManager struct {
	operations map[string]*OperationProgress
	mutex      sync.RWMutex
}

// NewProgressManager creates a new progress manager
func NewProgressManager() *ProgressManager {
	return &ProgressManager{
		operations: make(map[string]*OperationProgress),
	}
}

// CreateOperation creates a new operation and returns it
func (pm *ProgressManager) CreateOperation(operationID string) *OperationProgress {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	op := &OperationProgress{
		ID:        operationID,
		Clients:   make(map[chan ProgressEvent]bool),
		done:      false,
		createdAt: time.Now(),
	}

	pm.operations[operationID] = op
	return op
}

// Subscribe adds a client to an operation and returns a channel for events
// Returns error if operation doesn't exist or is already complete
func (pm *ProgressManager) Subscribe(operationID string) (chan ProgressEvent, error) {
	pm.mutex.RLock()
	op, exists := pm.operations[operationID]
	pm.mutex.RUnlock()

	if !exists {
		// Return empty error interface (nil)
		return nil, ErrOperationNotFound
	}

	op.mutex.Lock()
	defer op.mutex.Unlock()

	if op.done {
		return nil, ErrOperationComplete
	}

	ch := make(chan ProgressEvent, 100)
	op.Clients[ch] = true
	return ch, nil
}

// Unsubscribe removes a client channel from an operation
func (pm *ProgressManager) Unsubscribe(operationID string, ch chan ProgressEvent) {
	pm.mutex.RLock()
	op, exists := pm.operations[operationID]
	pm.mutex.RUnlock()

	if !exists {
		return
	}

	op.mutex.Lock()
	defer op.mutex.Unlock()

	if _, ok := op.Clients[ch]; ok {
		delete(op.Clients, ch)
		close(ch)
	}
}

// Publish sends an event to all subscribers of an operation
func (pm *ProgressManager) Publish(operationID string, event ProgressEvent) {
	pm.mutex.RLock()
	op, exists := pm.operations[operationID]
	pm.mutex.RUnlock()

	if !exists {
		return
	}

	op.mutex.RLock()
	defer op.mutex.RUnlock()

	for ch := range op.Clients {
		select {
		case ch <- event:
		default:
			// Channel full, skip this client (they should be keeping up)
		}
	}
}

// Complete marks an operation as complete and closes all client channels
func (pm *ProgressManager) Complete(operationID string) {
	pm.mutex.RLock()
	op, exists := pm.operations[operationID]
	pm.mutex.RUnlock()

	if !exists {
		return
	}

	op.mutex.Lock()
	defer op.mutex.Unlock()

	op.done = true

	// Close all client channels
	for ch := range op.Clients {
		close(ch)
	}
	op.Clients = make(map[chan ProgressEvent]bool)
}

// CleanupOld removes operations older than maxAge
func (pm *ProgressManager) CleanupOld(maxAge time.Duration) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	now := time.Now()
	for id, op := range pm.operations {
		if op.done && now.Sub(op.createdAt) > maxAge {
			delete(pm.operations, id)
		}
	}
}

// Error types
type operationError string

const (
	ErrOperationNotFound operationError = "operation not found"
	ErrOperationComplete operationError = "operation already complete"
)

func (e operationError) Error() string {
	return string(e)
}
