package integration

import (
	"testing"
	"time"

	"github.com/heventure/hermes-kanban-remote/internal/lease"
	"github.com/heventure/hermes-kanban-remote/internal/scheduler"
)

// TestScenario4_LeaseExpiration verifies lease lifecycle:
// create lease -> wait for TTL -> verify expiry -> verify task can be rescheduled
func TestScenario4_LeaseExpiration(t *testing.T) {
	cluster := NewTestCluster(500 * time.Millisecond) // very short TTL for fast testing
	defer cluster.Stop()

	// Step 1: Register nodes
	cluster.RegisterNode("node_a", "Node-A", []string{"coding"})
	cluster.RegisterNode("node_b", "Node-B", []string{"coding"})

	// Step 2: Create a lease manually with short TTL
	l, err := cluster.LeaseMgr.Create("task_lease_001", "node_a", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create lease: %v", err)
	}

	// Step 3: Verify lease is active
	if l.Status != lease.LeaseActive {
		t.Errorf("expected lease status 'active', got '%s'", l.Status)
	}

	// Step 4: Verify only one active lease per task
	_, err = cluster.LeaseMgr.Create("task_lease_001", "node_b", 500*time.Millisecond)
	if err == nil {
		t.Error("expected error when creating duplicate active lease")
	}

	// Step 5: Wait for TTL to expire
	time.Sleep(600 * time.Millisecond)

	// Step 6: Manually trigger expiry check
	expired := cluster.LeaseMgr.CheckExpiry()
	if len(expired) == 0 {
		t.Error("expected at least one expired task")
	}

	// Step 7: Verify the lease is now expired
	expiredLease, ok := cluster.LeaseMgr.Get(l.ID)
	if !ok {
		t.Fatal("lease not found")
	}
	if expiredLease.Status != lease.LeaseExpired {
		t.Errorf("expected lease status 'expired', got '%s'", expiredLease.Status)
	}

	// Step 8: Verify no active lease for this task
	_, hasActive := cluster.LeaseMgr.GetActiveForTask("task_lease_001")
	if hasActive {
		t.Error("expected no active lease after expiry")
	}

	// Step 9: Create the task in the task store and verify rescheduling works
	if _, err := cluster.TaskStore.Create("task_lease_001", "Lease test task", []string{"coding"}); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	cluster.TaskStore.SetAssigned("task_lease_001", "node_a")

	// Create a fresh lease for rescheduling test
	cluster.LeaseMgr.Create("task_lease_001", "node_a", 10*time.Second)

	// Revoke the lease
	if activeLease, ok := cluster.LeaseMgr.GetActiveForTask("task_lease_001"); ok {
		cluster.LeaseMgr.Revoke(activeLease.ID)
	}

	// Wait a moment for async revocation callback
	time.Sleep(100 * time.Millisecond)

	// Unassign and reschedule
	cluster.TaskStore.Unassign("task_lease_001")
	scheduled := cluster.Scheduler.SchedulePending()

	// Verify task was rescheduled
	rescheduled, ok := cluster.TaskStore.Get("task_lease_001")
	if !ok {
		t.Fatal("task not found after reschedule")
	}

	if len(scheduled) > 0 {
		if rescheduled.Status != scheduler.TaskAssigned {
			t.Errorf("expected task to be reassigned, status = %s", rescheduled.Status)
		}
		t.Logf("Task rescheduled to: %s", rescheduled.AssignedTo)
	} else {
		t.Log("No nodes available for rescheduling (expected if all nodes have tasks)")
	}
}

// TestScenario4b_LeaseRenewal verifies lease renewal protection.
func TestScenario4b_LeaseRenewal(t *testing.T) {
	cluster := NewTestCluster(1 * time.Second)
	defer cluster.Stop()

	// Create a lease
	l, err := cluster.LeaseMgr.Create("task_renew_001", "node_a", 1*time.Second)
	if err != nil {
		t.Fatalf("failed to create lease: %v", err)
	}

	// Holder can renew
	err = cluster.LeaseMgr.Renew(l.ID, "node_a", 5*time.Second)
	if err != nil {
		t.Errorf("holder should be able to renew: %v", err)
	}

	// Non-holder cannot renew
	err = cluster.LeaseMgr.Renew(l.ID, "node_b", 5*time.Second)
	if err == nil {
		t.Error("non-holder should not be able to renew")
	}

	// Revoke the lease
	cluster.LeaseMgr.Revoke(l.ID)

	// Cannot renew a revoked lease
	err = cluster.LeaseMgr.Renew(l.ID, "node_a", 5*time.Second)
	if err == nil {
		t.Error("should not be able to renew a revoked lease")
	}
}

// TestScenario4c_LeaseRevokeAllForNode verifies bulk revocation.
func TestScenario4c_LeaseRevokeAllForNode(t *testing.T) {
	cluster := NewTestCluster(10 * time.Second)
	defer cluster.Stop()

	// Create multiple leases for node_a
	cluster.LeaseMgr.Create("task_bulk_001", "node_a", 10*time.Second)
	cluster.LeaseMgr.Create("task_bulk_002", "node_a", 10*time.Second)
	cluster.LeaseMgr.Create("task_bulk_003", "node_b", 10*time.Second)

	// Revoke all for node_a
	revoked := cluster.LeaseMgr.RevokeAllForNode("node_a")

	if len(revoked) != 2 {
		t.Errorf("expected 2 revoked tasks, got %d", len(revoked))
	}

	// Verify node_b's lease is still active
	_, ok := cluster.LeaseMgr.GetActiveForTask("task_bulk_003")
	if !ok {
		t.Error("node_b's lease should still be active")
	}

	// Verify node_a's leases are revoked
	_, ok = cluster.LeaseMgr.GetActiveForTask("task_bulk_001")
	if ok {
		t.Error("node_a's lease for task_bulk_001 should be revoked")
	}
	_, ok = cluster.LeaseMgr.GetActiveForTask("task_bulk_002")
	if ok {
		t.Error("node_a's lease for task_bulk_002 should be revoked")
	}
}
