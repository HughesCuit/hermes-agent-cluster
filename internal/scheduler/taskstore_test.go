package scheduler

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// TestCreate_DuplicateID verifies that Create returns an error for duplicate IDs.
func TestCreate_DuplicateID(t *testing.T) {
	store := NewTaskStore()

	_, err := store.Create("task_dup", "First task", nil)
	if err != nil {
		t.Fatalf("first create should succeed: %v", err)
	}

	_, err = store.Create("task_dup", "Duplicate task", nil)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
}

// TestCreate_ConcurrentNoOverwrite verifies that concurrent creates with
// the same ID result in exactly one task, and concurrent creates with
// different IDs all succeed without data loss.
func TestCreate_ConcurrentNoOverwrite(t *testing.T) {
	const goroutines = 100

	// Phase 1: concurrent creates with the SAME ID — exactly one should succeed
	t.Run("same_id", func(t *testing.T) {
		store := NewTaskStore()
		var successes atomic.Int32
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				_, err := store.Create("task_shared", "Shared task", nil)
				if err == nil {
					successes.Add(1)
				}
			}()
		}
		wg.Wait()

		if successes.Load() != 1 {
			t.Errorf("expected exactly 1 successful create for shared ID, got %d", successes.Load())
		}
	})

	// Phase 2: concurrent creates with UNIQUE IDs — all should succeed
	t.Run("unique_ids", func(t *testing.T) {
		store := NewTaskStore()
		var successCount atomic.Int32
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func(n int) {
				defer wg.Done()
				id := fmt.Sprintf("task_%d", n)
				_, err := store.Create(id, "Task "+id, nil)
				if err == nil {
					successCount.Add(1)
				}
			}(i)
		}
		wg.Wait()

		if int(successCount.Load()) != goroutines {
			t.Errorf("expected %d successful creates for unique IDs, got %d", goroutines, successCount.Load())
		}

		// Verify final count matches successes (no overwrites)
		all := store.GetAll()
		if len(all) != goroutines {
			t.Errorf("expected %d tasks in store, got %d", goroutines, len(all))
		}
	})
}

// TestCreate_ConcurrentCountInvariant verifies that N concurrent creates with
// unique IDs always result in exactly N tasks (no data loss from races).
func TestCreate_ConcurrentCountInvariant(t *testing.T) {
	const goroutines = 500
	store := NewTaskStore()

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			id := GenerateID()
			_, err := store.Create(id, fmt.Sprintf("Task %d", n), nil)
			if err != nil {
				t.Errorf("unexpected error creating task %s: %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	all := store.GetAll()
	if len(all) != goroutines {
		t.Errorf("task count invariant violated: expected %d, got %d", goroutines, len(all))
	}
}

// TestGenerateID_Unique verifies that GenerateID produces unique values.
func TestGenerateID_Unique(t *testing.T) {
	const count = 1000
	seen := make(map[string]bool, count)
	for i := 0; i < count; i++ {
		id := GenerateID()
		if seen[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		seen[id] = true
	}
}
