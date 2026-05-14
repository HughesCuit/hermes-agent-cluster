package recovery

import (
	"fmt"
	"sync"
	"time"
)

// RecoveryEvent represents a single recovery action.
type RecoveryEvent struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	NodeID    string    `json:"node_id"`
	Action    string    `json:"action"` // "revoke_lease", "reschedule", "mark_failed"
	Status    string    `json:"status"` // "completed", "partial", "failed"
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Log stores recovery events.
type Log struct {
	mu     sync.RWMutex
	events []RecoveryEvent
}

// NewLog creates a new recovery log.
func NewLog() *Log {
	return &Log{}
}

// Append adds a recovery event.
func (l *Log) Append(e RecoveryEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e.Timestamp = time.Now()
	if e.ID == "" {
		e.ID = fmt.Sprintf("recovery_%d", len(l.events)+1)
	}
	l.events = append(l.events, e)
}

// GetEvents returns all events.
func (l *Log) GetEvents() []RecoveryEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]RecoveryEvent, len(l.events))
	copy(result, l.events)
	return result
}

// Stats returns recovery statistics.
func (l *Log) Stats() map[string]int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	stats := map[string]int{"total": 0, "completed": 0, "partial": 0, "failed": 0}
	for _, e := range l.events {
		stats["total"]++
		stats[e.Status]++
	}
	return stats
}
