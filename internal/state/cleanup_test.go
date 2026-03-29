package state

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDeletePlan(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create a plan
	plan := &Plan{
		ID:             "plan-delete-test",
		AppDescription: "Test app",
		Status:         PlanStatusCreated,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}

	err = store.CreatePlan(plan)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Verify it exists
	_, err = store.GetPlan(plan.ID)
	if err != nil {
		t.Fatalf("GetPlan before delete: %v", err)
	}

	// Delete it
	err = store.DeletePlan(plan.ID)
	if err != nil {
		t.Fatalf("DeletePlan: %v", err)
	}

	// Verify it's gone
	_, err = store.GetPlan(plan.ID)
	if err == nil {
		t.Error("Expected error after deletion, got nil")
	}
}

func TestDeletePlan_NotFound(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Deleting non-existent plan should not error
	if err := store.DeletePlan("plan-nonexistent"); err != nil {
		t.Errorf("DeletePlan of non-existent: %v", err)
	}
}

func TestDeleteExpiredPlans(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	now := time.Now()

	// Create an expired plan
	expiredPlan := &Plan{
		ID:             "plan-expired",
		AppDescription: "Expired app",
		Status:         PlanStatusCreated,
		CreatedAt:      now.Add(-48 * time.Hour),
		ExpiresAt:      now.Add(-24 * time.Hour), // Expired 24 hours ago
	}
	err = store.CreatePlan(expiredPlan)
	if err != nil {
		t.Fatalf("CreatePlan (expired): %v", err)
	}

	// Create a valid plan
	validPlan := &Plan{
		ID:             "plan-valid",
		AppDescription: "Valid app",
		Status:         PlanStatusCreated,
		CreatedAt:      now,
		ExpiresAt:      now.Add(24 * time.Hour), // Expires in 24 hours
	}
	err = store.CreatePlan(validPlan)
	if err != nil {
		t.Fatalf("CreatePlan (valid): %v", err)
	}

	// Delete expired plans
	deleted, err := store.DeleteExpiredPlans()
	if err != nil {
		t.Fatalf("DeleteExpiredPlans: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify expired plan is gone
	_, err = store.GetPlan(expiredPlan.ID)
	if err == nil {
		t.Error("Expected expired plan to be deleted")
	}

	// Verify valid plan still exists
	_, err = store.GetPlan(validPlan.ID)
	if err != nil {
		t.Errorf("Valid plan should still exist: %v", err)
	}
}

func TestDeleteExpiredPlans_NoExpired(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create only valid plans
	plan := &Plan{
		ID:             "plan-valid",
		AppDescription: "Valid app",
		Status:         PlanStatusCreated,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}
	err = store.CreatePlan(plan)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	deleted, err := store.DeleteExpiredPlans()
	if err != nil {
		t.Fatalf("DeleteExpiredPlans: %v", err)
	}

	if deleted != 0 {
		t.Errorf("Expected 0 deleted, got %d", deleted)
	}
}

func TestDefaultCleanupConfig(t *testing.T) {
	cfg := DefaultCleanupConfig()

	if cfg.Interval != 1*time.Hour {
		t.Errorf("Interval = %v, want 1h", cfg.Interval)
	}
	if cfg.OnCleanup != nil {
		t.Error("OnCleanup should be nil by default")
	}
}

func TestCleanupService_StartStop(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	config := CleanupConfig{
		Interval: 100 * time.Millisecond,
	}
	service := NewCleanupService(store, config)

	if service.IsRunning() {
		t.Error("Service should not be running initially")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !service.IsRunning() {
		t.Error("Service should be running after Start")
	}

	// Allow a cleanup cycle
	time.Sleep(150 * time.Millisecond)

	service.Stop()

	if service.IsRunning() {
		t.Error("Service should not be running after Stop")
	}
}

func TestCleanupService_Stats(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	config := CleanupConfig{
		Interval: 100 * time.Millisecond,
	}
	service := NewCleanupService(store, config)

	// Initial stats
	stats := service.Stats()
	if stats.Running {
		t.Error("Should not be running")
	}
	if stats.TotalDeleted != 0 {
		t.Error("TotalDeleted should be 0")
	}
	if stats.Interval != 100*time.Millisecond {
		t.Errorf("Interval = %v, want 100ms", stats.Interval)
	}
}

func TestCleanupService_CleanupNow(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create an expired plan
	plan := &Plan{
		ID:             "plan-expired-now",
		AppDescription: "Expired",
		Status:         PlanStatusCreated,
		CreatedAt:      time.Now().Add(-48 * time.Hour),
		ExpiresAt:      time.Now().Add(-24 * time.Hour),
	}
	err = store.CreatePlan(plan)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	service := NewCleanupService(store, DefaultCleanupConfig())

	deleted, err := service.CleanupNow()
	if err != nil {
		t.Fatalf("CleanupNow: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Check stats updated
	stats := service.Stats()
	if stats.TotalDeleted != 1 {
		t.Errorf("TotalDeleted = %d, want 1", stats.TotalDeleted)
	}
	if stats.LastRun.IsZero() {
		t.Error("LastRun should be set")
	}
}

func TestCleanupService_OnCleanupCallback(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	var mu sync.Mutex
	callbackCalled := false
	callbackDeleted := 0

	config := CleanupConfig{
		Interval: 100 * time.Millisecond,
		OnCleanup: func(deleted int) {
			mu.Lock()
			callbackCalled = true
			callbackDeleted = deleted
			mu.Unlock()
		},
	}
	service := NewCleanupService(store, config)

	// Create an expired plan
	plan := &Plan{
		ID:             "plan-callback-test",
		AppDescription: "Expired",
		Status:         PlanStatusCreated,
		CreatedAt:      time.Now().Add(-48 * time.Hour),
		ExpiresAt:      time.Now().Add(-24 * time.Hour),
	}
	err = store.CreatePlan(plan)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	_, err = service.CleanupNow()
	if err != nil {
		t.Fatalf("CleanupNow: %v", err)
	}

	mu.Lock()
	if !callbackCalled {
		t.Error("Callback should have been called")
	}
	if callbackDeleted != 1 {
		t.Errorf("Callback received deleted=%d, want 1", callbackDeleted)
	}
	mu.Unlock()
}

func TestCleanupService_DoubleStart(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	service := NewCleanupService(store, DefaultCleanupConfig())

	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("First Start: %v", err)
	}
	defer service.Stop()

	// Second start should be no-op
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Second Start: %v", err)
	}
}

func TestCleanupService_DoubleStop(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	service := NewCleanupService(store, DefaultCleanupConfig())

	// Stop without start should be no-op
	service.Stop()

	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	service.Stop()
	service.Stop() // Double stop should be safe
}

// TestCleanupService_ConcurrentStop verifies that concurrent Stop() calls
// do not cause a panic from closing an already-closed channel.
// This tests the fix for P3.21 race condition.
func TestCleanupService_ConcurrentStop(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	config := CleanupConfig{
		Interval: 100 * time.Millisecond,
	}
	service := NewCleanupService(store, config)

	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let the service run briefly
	time.Sleep(50 * time.Millisecond)

	// Call Stop() concurrently from multiple goroutines.
	// Without the sync.Once fix, this would panic with
	// "close of closed channel".
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.Stop()
		}()
	}

	// Wait for all Stop() calls to complete
	wg.Wait()

	// Verify service is stopped
	if service.IsRunning() {
		t.Error("Service should not be running after concurrent stops")
	}
}
