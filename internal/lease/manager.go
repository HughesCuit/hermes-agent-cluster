package lease

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// LeaseStatus represents the state of a lease.
type LeaseStatus string

const (
	LeaseActive   LeaseStatus = "active"
	LeaseExpired  LeaseStatus = "expired"
	LeaseRevoked  LeaseStatus = "revoked"
)

// Lease represents a task lease held by a node.
type Lease struct {
	ID        string      `json:"id"`
	TaskID    string      `json:"task_id"`
	NodeID    string      `json:"node_id"`
	CreatedAt time.Time   `json:"created_at"`
	ExpiresAt time.Time   `json:"expires_at"`
	Status    LeaseStatus `json:"status"`
}

// Manager handles lease lifecycle.
type Manager struct {
	mu        sync.RWMutex
	leases    map[string]*Lease        // id -> lease
	taskIndex map[string]string         // task_id -> lease_id (active lease per task)
	callback  func(taskID, nodeID string) // called when lease expires or revoked
}

// NewManager creates a lease manager.
func NewManager() *Manager {
	return &Manager{
		leases:    make(map[string]*Lease),
		taskIndex: make(map[string]string),
	}
}

// SetExpiryCallback sets the callback invoked on lease expiry/revocation.
func (m *Manager) SetExpiryCallback(fn func(taskID, nodeID string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callback = fn
}

// Create creates a new lease for a task. Only one active lease per task.
func (m *Manager) Create(taskID, nodeID string, ttl time.Duration) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existingID, ok := m.taskIndex[taskID]; ok {
		if existing, ok2 := m.leases[existingID]; ok2 && existing.Status == LeaseActive {
			return nil, fmt.Errorf("task %s already has an active lease held by %s", taskID, existing.NodeID)
		}
	}

	id := fmt.Sprintf("lease_%s_%s", taskID, nodeID)
	now := time.Now()
	l := &Lease{
		ID:        id,
		TaskID:    taskID,
		NodeID:    nodeID,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		Status:    LeaseActive,
	}
	m.leases[id] = l
	m.taskIndex[taskID] = id
	return l, nil
}

// Get returns a lease by ID.
func (m *Manager) Get(id string) (*Lease, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, ok := m.leases[id]
	if !ok {
		return nil, false
	}
	cp := *l
	return &cp, true
}

// GetActiveForTask returns the active lease for a task.
func (m *Manager) GetActiveForTask(taskID string) (*Lease, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lid, ok := m.taskIndex[taskID]
	if !ok {
		return nil, false
	}
	l, ok := m.leases[lid]
	if !ok || l.Status != LeaseActive {
		return nil, false
	}
	cp := *l
	return &cp, true
}

// Renew extends a lease's expiry. Only the holder can renew.
func (m *Manager) Renew(leaseID, nodeID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.leases[leaseID]
	if !ok {
		return fmt.Errorf("lease %s not found", leaseID)
	}
	if l.NodeID != nodeID {
		return fmt.Errorf("only holder %s can renew lease %s", l.NodeID, leaseID)
	}
	if l.Status != LeaseActive {
		return fmt.Errorf("lease %s is not active", leaseID)
	}
	l.ExpiresAt = time.Now().Add(ttl)
	return nil
}

// Revoke revokes a lease and triggers the callback.
func (m *Manager) Revoke(leaseID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.leases[leaseID]
	if !ok {
		return fmt.Errorf("lease %s not found", leaseID)
	}
	if l.Status != LeaseActive {
		return nil // already expired/revoked
	}
	l.Status = LeaseRevoked
	if m.callback != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("lease callback panic: %v", r)
				}
			}()
			m.callback(l.TaskID, l.NodeID)
		}()
	}
	return nil
}

// RevokeAllForNode revokes all active leases for a node.
func (m *Manager) RevokeAllForNode(nodeID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var revoked []string
	for _, l := range m.leases {
		if l.NodeID == nodeID && l.Status == LeaseActive {
			l.Status = LeaseRevoked
			revoked = append(revoked, l.TaskID)
			if m.callback != nil {
				leaseCopy := *l
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("lease callback panic: %v", r)
						}
					}()
					m.callback(leaseCopy.TaskID, leaseCopy.NodeID)
				}()
			}
		}
	}
	return revoked
}

// GetActiveLeases returns all active leases.
func (m *Manager) GetActiveLeases() []*Lease {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Lease
	for _, l := range m.leases {
		if l.Status == LeaseActive {
			cp := *l
			result = append(result, &cp)
		}
	}
	return result
}

// CheckExpiry scans all leases and expires those past their TTL.
// Returns list of expired task IDs.
func (m *Manager) CheckExpiry() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	var expired []string
	for _, l := range m.leases {
		if l.Status == LeaseActive && now.After(l.ExpiresAt) {
			l.Status = LeaseExpired
			expired = append(expired, l.TaskID)
			if m.callback != nil {
				leaseCopy := *l
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("lease callback panic: %v", r)
						}
					}()
					m.callback(leaseCopy.TaskID, leaseCopy.NodeID)
				}()
			}
		}
	}
	return expired
}

// StartExpiryScanner runs a background goroutine that checks lease expiry.
func (m *Manager) StartExpiryScanner(interval time.Duration, stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.CheckExpiry()
			case <-stopCh:
				return
			}
		}
	}()
}
