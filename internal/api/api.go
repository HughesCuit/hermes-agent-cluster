package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/heventure/hermes-kanban-remote/internal/cluster"
	"github.com/heventure/hermes-kanban-remote/internal/lease"
	"github.com/heventure/hermes-kanban-remote/internal/recovery"
	"github.com/heventure/hermes-kanban-remote/internal/scheduler"
	"github.com/heventure/hermes-kanban-remote/internal/sync"
)

// Server holds all dependencies for the API.
type Server struct {
	Router     *chi.Mux
	Registry   *cluster.Registry
	Scheduler  *scheduler.Scheduler
	LeaseMgr   *lease.Manager
	Recovery   *recovery.Detector
	Log        *recovery.Log
	StateStore *sync.StateStore
	Receiver   *sync.FollowerReceiver
}

// NewServer creates a new API server.
func NewServer(
	registry *cluster.Registry,
	sched *scheduler.Scheduler,
	leaseMgr *lease.Manager,
	detector *recovery.Detector,
	recLog *recovery.Log,
	stateStore *sync.StateStore,
	receiver *sync.FollowerReceiver,
) *Server {
	s := &Server{
		Router:     chi.NewRouter(),
		Registry:   registry,
		Scheduler:  sched,
		LeaseMgr:   leaseMgr,
		Recovery:   detector,
		Log:        recLog,
		StateStore: stateStore,
		Receiver:   receiver,
	}
	s.Router.Use(middleware.Logger)
	s.Router.Use(middleware.Recoverer)
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.Router.Route("/api/v1", func(r chi.Router) {
		// Node management
		r.Post("/nodes/join", s.handleJoin)
		r.Post("/nodes/heartbeat", s.handleHeartbeat)
		r.Get("/nodes", s.handleListNodes)

		// Task management
		r.Post("/tasks", s.handleSubmitTask)
		r.Get("/tasks", s.handleListTasks)
		r.Post("/tasks/{id}/complete", s.handleCompleteTask)

		// Lease management
		r.Post("/leases", s.handleCreateLease)
		r.Delete("/leases/{id}", s.handleRevokeLease)
		r.Get("/leases", s.handleListLeases)

		// Sync
		r.Post("/sync/receive", s.handleSyncReceive)
		r.Get("/sync/status", s.handleSyncStatus)

		// Recovery
		r.Post("/recovery/trigger", s.handleRecoveryTrigger)
		r.Get("/recovery/log", s.handleRecoveryLog)
		r.Get("/recovery/stats", s.handleRecoveryStats)
	})
}

// --- Node handlers ---

type joinRequest struct {
	NodeName     string   `json:"node_name"`
	Capabilities []string `json:"capabilities"`
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	nodeID := "node_" + req.NodeName
	node := &cluster.Node{
		ID:           nodeID,
		Name:         req.NodeName,
		Capabilities: req.Capabilities,
	}
	s.Registry.Register(node)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"node_id": nodeID, "status": "registered"})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	s.Registry.UpdateHeartbeat(req.NodeID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes := s.Registry.GetAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

// --- Task handlers ---

type submitTaskRequest struct {
	Title    string   `json:"title"`
	Requires []string `json:"requires"`
}

func (s *Server) handleSubmitTask(w http.ResponseWriter, r *http.Request) {
	var req submitTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	taskID := "task_" + time.Now().Format("150405.000")
	task := s.Scheduler.GetTaskStore().Create(taskID, req.Title, req.Requires)
	// Try to schedule immediately
	scheduled := s.Scheduler.SchedulePending()
	_ = scheduled
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks := s.Scheduler.GetTaskStore().GetAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func (s *Server) handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	s.Scheduler.GetTaskStore().SetStatus(taskID, scheduler.TaskCompleted)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
}

// --- Lease handlers ---

func (s *Server) handleCreateLease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID string `json:"task_id"`
		NodeID string `json:"node_id"`
		TTL    int    `json:"ttl_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	l, err := s.LeaseMgr.Create(req.TaskID, req.NodeID, time.Duration(req.TTL)*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(l)
}

func (s *Server) handleRevokeLease(w http.ResponseWriter, r *http.Request) {
	leaseID := chi.URLParam(r, "id")
	if err := s.LeaseMgr.Revoke(leaseID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}

func (s *Server) handleListLeases(w http.ResponseWriter, r *http.Request) {
	leases := s.LeaseMgr.GetActiveLeases()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(leases)
}

// --- Sync handlers ---

func (s *Server) handleSyncReceive(w http.ResponseWriter, r *http.Request) {
	var msg sync.SyncMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	applied := s.Receiver.HandleSyncMessage(msg)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"applied": applied})
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"version": s.StateStore.Version()})
}

// --- Recovery handlers ---

func (s *Server) handleRecoveryTrigger(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	go s.Recovery.NotifyOffline(req.NodeID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func (s *Server) handleRecoveryLog(w http.ResponseWriter, r *http.Request) {
	events := s.Log.GetEvents()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (s *Server) handleRecoveryStats(w http.ResponseWriter, r *http.Request) {
	stats := s.Log.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// ListenAndServe starts the server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.Router)
}
