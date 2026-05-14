package scheduler

import (
	"fmt"
	"sync"
	"time"
)

// TaskStatus represents the state of a task.
type TaskStatus string

const (
	TaskReady     TaskStatus = "ready"
	TaskAssigned  TaskStatus = "assigned"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
)

// Task represents a schedulable unit of work.
type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Requires    []string   `json:"requires"`
	Status      TaskStatus `json:"status"`
	AssignedTo  string     `json:"assigned_to,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Version     int64      `json:"version"`
	FailReason  string     `json:"fail_reason,omitempty"`
}

// TaskStore is an in-memory store for tasks.
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

// NewTaskStore creates a new task store.
func NewTaskStore() *TaskStore {
	return &TaskStore{tasks: make(map[string]*Task)}
}

// Create adds a new task.
func (s *TaskStore) Create(id, title string, requires []string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	t := &Task{
		ID:        id,
		Title:     title,
		Requires:  requires,
		Status:    TaskReady,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   1,
	}
	s.tasks[id] = t
	return t
}

// Get returns a task by ID.
func (s *TaskStore) Get(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, false
	}
	cp := *t
	return &cp, true
}

// Update modifies a task.
func (s *TaskStore) Update(t *Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.UpdatedAt = time.Now()
	t.Version++
	cp := *t
	s.tasks[t.ID] = &cp
}

// SetStatus changes a task's status.
func (s *TaskStore) SetStatus(id string, status TaskStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	t.Status = status
	t.UpdatedAt = time.Now()
	t.Version++
	return nil
}

// SetAssigned marks a task as assigned to a node.
func (s *TaskStore) SetAssigned(id, nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	t.Status = TaskAssigned
	t.AssignedTo = nodeID
	t.UpdatedAt = time.Now()
	t.Version++
	return nil
}

// Unassign releases a task back to ready.
func (s *TaskStore) Unassign(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	t.Status = TaskReady
	t.AssignedTo = ""
	t.UpdatedAt = time.Now()
	t.Version++
	return nil
}

// GetReady returns all tasks in Ready status.
func (s *TaskStore) GetReady() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Task
	for _, t := range s.tasks {
		if t.Status == TaskReady {
			cp := *t
			result = append(result, &cp)
		}
	}
	return result
}

// GetAll returns all tasks.
func (s *TaskStore) GetAll() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		cp := *t
		result = append(result, &cp)
	}
	return result
}
