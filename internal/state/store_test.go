package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	apperrors "github.com/cjmartian/agent-deploy/internal/errors"
)

func TestPlanCRUD(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	plan := &Plan{
		ID:              "plan-test-001",
		AppDescription:  "Test app",
		ExpectedUsers:   100,
		LatencyMS:       50,
		Region:          "us-east-1",
		Services:        []string{"ECS", "ALB"},
		EstimatedCostMo: 25.50,
		Status:          PlanStatusCreated,
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}

	// Create.
	err = store.CreatePlan(plan)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Read.
	got, err := store.GetPlan(plan.ID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if got.AppDescription != plan.AppDescription {
		t.Errorf("AppDescription = %q, want %q", got.AppDescription, plan.AppDescription)
	}

	// Approve.
	err = store.ApprovePlan(plan.ID)
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	got, _ = store.GetPlan(plan.ID)
	if got.Status != PlanStatusApproved {
		t.Errorf("Status = %q, want %q", got.Status, PlanStatusApproved)
	}

	// List.
	plans, err := store.ListPlans()
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	if len(plans) != 1 {
		t.Errorf("ListPlans returned %d plans, want 1", len(plans))
	}
}

func TestPlanNotFound(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	_, err := store.GetPlan("nonexistent")
	if !errors.Is(err, apperrors.ErrPlanNotFound) {
		t.Errorf("GetPlan error = %v, want %v", err, apperrors.ErrPlanNotFound)
	}
}

func TestExpiredPlanCannotBeApproved(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	plan := &Plan{
		ID:        "plan-expired",
		Status:    PlanStatusCreated,
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour), // Already expired.
	}
	if err := store.CreatePlan(plan); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	err := store.ApprovePlan(plan.ID)
	if !errors.Is(err, apperrors.ErrPlanExpired) {
		t.Errorf("ApprovePlan error = %v, want %v", err, apperrors.ErrPlanExpired)
	}
}

func TestRejectPlan(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	plan := &Plan{
		ID:        "plan-to-reject",
		Status:    PlanStatusCreated,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := store.CreatePlan(plan); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Reject the plan.
	if err := store.RejectPlan(plan.ID); err != nil {
		t.Fatalf("RejectPlan: %v", err)
	}

	// Verify status changed.
	got, _ := store.GetPlan(plan.ID)
	if got.Status != PlanStatusRejected {
		t.Errorf("Status = %q, want %q", got.Status, PlanStatusRejected)
	}
}

func TestRejectedPlanCannotBeApproved(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	plan := &Plan{
		ID:        "plan-rejected",
		Status:    PlanStatusCreated,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := store.CreatePlan(plan); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// First reject.
	if err := store.RejectPlan(plan.ID); err != nil {
		t.Fatalf("RejectPlan: %v", err)
	}

	// Then try to approve — should fail with ErrInvalidState.
	err := store.ApprovePlan(plan.ID)
	if !errors.Is(err, apperrors.ErrInvalidState) {
		t.Errorf("ApprovePlan after reject error = %v, want %v", err, apperrors.ErrInvalidState)
	}
}

func TestApprovedPlanCannotBeRejected(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	plan := &Plan{
		ID:        "plan-approved",
		Status:    PlanStatusCreated,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := store.CreatePlan(plan); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// First approve.
	if err := store.ApprovePlan(plan.ID); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}

	// Then try to reject — should fail with ErrInvalidState.
	err := store.RejectPlan(plan.ID)
	if !errors.Is(err, apperrors.ErrInvalidState) {
		t.Errorf("RejectPlan after approve error = %v, want %v", err, apperrors.ErrInvalidState)
	}
}

func TestApproveIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	plan := &Plan{
		ID:        "plan-idempotent-approve",
		Status:    PlanStatusCreated,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := store.CreatePlan(plan); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Approve twice — should succeed both times (idempotent).
	if err := store.ApprovePlan(plan.ID); err != nil {
		t.Fatalf("First ApprovePlan: %v", err)
	}
	if err := store.ApprovePlan(plan.ID); err != nil {
		t.Errorf("Second ApprovePlan (idempotent) error = %v, want nil", err)
	}
}

func TestRejectIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	plan := &Plan{
		ID:        "plan-idempotent-reject",
		Status:    PlanStatusCreated,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := store.CreatePlan(plan); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Reject twice — should succeed both times (idempotent).
	if err := store.RejectPlan(plan.ID); err != nil {
		t.Fatalf("First RejectPlan: %v", err)
	}
	if err := store.RejectPlan(plan.ID); err != nil {
		t.Errorf("Second RejectPlan (idempotent) error = %v, want nil", err)
	}
}

func TestExpiredPlanCannotBeRejected(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	plan := &Plan{
		ID:        "plan-expired-reject",
		Status:    PlanStatusCreated,
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour), // Already expired.
	}
	if err := store.CreatePlan(plan); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	err := store.RejectPlan(plan.ID)
	if !errors.Is(err, apperrors.ErrPlanExpired) {
		t.Errorf("RejectPlan error = %v, want %v", err, apperrors.ErrPlanExpired)
	}
}

func TestInfraCRUD(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	infra := &Infrastructure{
		ID:        "infra-test-001",
		PlanID:    "plan-test-001",
		Region:    "us-east-1",
		Resources: map[string]string{},
		Status:    InfraStatusProvisioning,
		CreatedAt: time.Now(),
	}

	// Create.
	if err := store.CreateInfra(infra); err != nil {
		t.Fatalf("CreateInfra: %v", err)
	}

	// Update resource.
	if err := store.UpdateInfraResource(infra.ID, ResourceVPC, "vpc-123"); err != nil {
		t.Fatalf("UpdateInfraResource: %v", err)
	}

	// Verify.
	got, _ := store.GetInfra(infra.ID)
	if got.Resources[ResourceVPC] != "vpc-123" {
		t.Errorf("Resources[vpc] = %q, want %q", got.Resources[ResourceVPC], "vpc-123")
	}

	// Set status.
	if err := store.SetInfraStatus(infra.ID, InfraStatusReady); err != nil {
		t.Fatalf("SetInfraStatus: %v", err)
	}
	got, _ = store.GetInfra(infra.ID)
	if got.Status != InfraStatusReady {
		t.Errorf("Status = %q, want %q", got.Status, InfraStatusReady)
	}
}

func TestDeploymentCRUD(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	deploy := &Deployment{
		ID:          "deploy-test-001",
		InfraID:     "infra-test-001",
		ImageRef:    "nginx:latest",
		Status:      DeploymentStatusDeploying,
		URLs:        []string{},
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	// Create.
	if err := store.CreateDeployment(deploy); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	// Update status.
	urls := []string{"http://example.com"}
	if err := store.UpdateDeploymentStatus(deploy.ID, DeploymentStatusRunning, urls); err != nil {
		t.Fatalf("UpdateDeploymentStatus: %v", err)
	}

	got, _ := store.GetDeployment(deploy.ID)
	if got.Status != DeploymentStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, DeploymentStatusRunning)
	}
	if len(got.URLs) != 1 || got.URLs[0] != "http://example.com" {
		t.Errorf("URLs = %v, want [http://example.com]", got.URLs)
	}

	// List.
	deployments, _ := store.ListDeployments()
	if len(deployments) != 1 {
		t.Errorf("ListDeployments returned %d, want 1", len(deployments))
	}

	// Delete.
	if err := store.DeleteDeployment(deploy.ID); err != nil {
		t.Fatalf("DeleteDeployment: %v", err)
	}
	_, err := store.GetDeployment(deploy.ID)
	if !errors.Is(err, apperrors.ErrDeploymentNotFound) {
		t.Errorf("GetDeployment after delete = %v, want %v", err, apperrors.ErrDeploymentNotFound)
	}
}

func TestStoreDirectoryCreation(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Verify subdirectories were created.
	for _, subdir := range []string{"plans", "infra", "deployments"} {
		path := filepath.Join(dir, subdir)
		if !dirExists(path) {
			t.Errorf("directory %s not created", subdir)
		}
	}
	_ = store
}

// TestAtomicWrites verifies that writeJSON uses atomic file operations.
// This test ensures that:
// 1. The final file exists after write
// 2. No temp files are left behind on success
// 3. The file content is correct
func TestAtomicWrites(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create multiple plans to exercise atomic writes
	for i := 0; i < 10; i++ {
		plan := &Plan{
			ID:              fmt.Sprintf("plan-atomic-%03d", i),
			AppDescription:  fmt.Sprintf("Atomic test app %d", i),
			ExpectedUsers:   100 + i,
			LatencyMS:       50,
			Region:          "us-east-1",
			Services:        []string{"ECS", "ALB"},
			EstimatedCostMo: 25.50,
			Status:          PlanStatusCreated,
			CreatedAt:       time.Now(),
			ExpiresAt:       time.Now().Add(24 * time.Hour),
		}

		if err := store.CreatePlan(plan); err != nil {
			t.Fatalf("CreatePlan: %v", err)
		}

		// Verify file exists and content is correct
		got, err := store.GetPlan(plan.ID)
		if err != nil {
			t.Fatalf("GetPlan: %v", err)
		}
		if got.AppDescription != plan.AppDescription {
			t.Errorf("AppDescription = %q, want %q", got.AppDescription, plan.AppDescription)
		}
	}

	// Verify no temp files were left behind
	entries, err := os.ReadDir(filepath.Join(dir, "plans"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			t.Errorf("unexpected file found: %s (should only have .json files)", entry.Name())
		}
	}
}

// TestConcurrentWrites verifies that concurrent writes don't corrupt state.
func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create initial plan
	plan := &Plan{
		ID:              "plan-concurrent",
		AppDescription:  "Concurrent test",
		ExpectedUsers:   100,
		LatencyMS:       50,
		Region:          "us-east-1",
		Services:        []string{"ECS"},
		EstimatedCostMo: 10.0,
		Status:          PlanStatusCreated,
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}
	if err := store.CreatePlan(plan); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Create infrastructure with concurrent updates
	infra := &Infrastructure{
		ID:        "infra-concurrent",
		PlanID:    plan.ID,
		Region:    "us-east-1",
		Status:    "provisioning",
		Resources: make(map[string]string),
		CreatedAt: time.Now(),
	}
	if err := store.CreateInfra(infra); err != nil {
		t.Fatalf("CreateInfra: %v", err)
	}

	// Run concurrent resource updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			resourceType := fmt.Sprintf("resource-%d", n)
			resourceARN := fmt.Sprintf("arn:aws:test:%d", n)
			err := store.UpdateInfraResource(infra.ID, resourceType, resourceARN)
			if err != nil {
				t.Errorf("UpdateInfraResource failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify final state is consistent
	got, err := store.GetInfra(infra.ID)
	if err != nil {
		t.Fatalf("GetInfra: %v", err)
	}

	// Should have all 10 resources (RWMutex ensures no lost updates)
	if len(got.Resources) != 10 {
		t.Errorf("Expected 10 resources, got %d", len(got.Resources))
	}
}

// TestConcurrentPlanOperations verifies concurrent plan create/read/delete operations.
// This tests the P2.10 requirement for verifying RWMutex under race conditions.
func TestConcurrentPlanOperations(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	const numGoroutines = 20
	const plansPerGoroutine = 5

	// Use sync.WaitGroup for proper synchronization
	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*plansPerGoroutine*3) // Enough buffer for all operations

	// Create, read, and delete plans concurrently
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for p := 0; p < plansPerGoroutine; p++ {
				planID := fmt.Sprintf("plan-g%d-p%d", gid, p)
				plan := &Plan{
					ID:              planID,
					AppDescription:  fmt.Sprintf("Test plan %s", planID),
					ExpectedUsers:   100 + gid,
					LatencyMS:       50,
					Region:          "us-east-1",
					Services:        []string{"ECS"},
					EstimatedCostMo: float64(10 + gid),
					Status:          PlanStatusCreated,
					CreatedAt:       time.Now(),
					ExpiresAt:       time.Now().Add(24 * time.Hour),
				}

				// Create
				if err := store.CreatePlan(plan); err != nil {
					errCh <- fmt.Errorf("CreatePlan %s: %w", planID, err)
					continue
				}

				// Read multiple times
				for r := 0; r < 3; r++ {
					got, err := store.GetPlan(planID)
					if err != nil {
						errCh <- fmt.Errorf("GetPlan %s (read %d): %w", planID, r, err)
						continue
					}
					if got.ID != planID {
						errCh <- fmt.Errorf("GetPlan %s: got ID %s", planID, got.ID)
					}
				}

				// Delete
				if err := store.DeletePlan(planID); err != nil {
					errCh <- fmt.Errorf("DeletePlan %s: %w", planID, err)
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	// Collect any errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		t.Errorf("Concurrent operations had %d errors:", len(errs))
		for i, err := range errs {
			if i >= 5 {
				t.Errorf("  ... and %d more", len(errs)-5)
				break
			}
			t.Errorf("  - %v", err)
		}
	}
}

// TestConcurrentMixedReadWrite verifies concurrent readers and writers.
// Multiple readers should be able to read simultaneously,
// while writers should have exclusive access.
func TestConcurrentMixedReadWrite(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create initial deployment
	deploy := &Deployment{
		ID:        "deploy-mixed",
		InfraID:   "infra-test",
		ImageRef:  "nginx:latest",
		Status:    DeploymentStatusRunning,
		CreatedAt: time.Now(),
	}
	if err := store.CreateDeployment(deploy); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	const (
		numReaders = 50
		numWriters = 10
		iterations = 100
	)

	var wg sync.WaitGroup
	errCh := make(chan error, numReaders*iterations+numWriters*iterations)

	// Start readers
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, err := store.GetDeployment("deploy-mixed")
				if err != nil && !errors.Is(err, apperrors.ErrDeploymentNotFound) {
					errCh <- fmt.Errorf("reader: %w", err)
				}
			}
		}()
	}

	// Start writers (update status)
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				status := DeploymentStatusRunning
				if i%2 == 0 {
					status = DeploymentStatusDeploying
				}
				_ = store.UpdateDeploymentStatus("deploy-mixed", status, nil)
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		t.Errorf("Mixed read/write had %d errors", len(errs))
		for _, err := range errs[:min(5, len(errs))] {
			t.Errorf("  - %v", err)
		}
	}

	// Verify final state is valid
	got, err := store.GetDeployment("deploy-mixed")
	if err != nil {
		t.Fatalf("Final GetDeployment: %v", err)
	}
	if got.Status != DeploymentStatusRunning && got.Status != DeploymentStatusDeploying {
		t.Errorf("Final status = %q, want running or deploying", got.Status)
	}
}

// TestConcurrentListOperations verifies listing operations under concurrent load.
func TestConcurrentListOperations(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create some initial infrastructure entries
	for i := 0; i < 10; i++ {
		infra := &Infrastructure{
			ID:        fmt.Sprintf("infra-list-%d", i),
			PlanID:    fmt.Sprintf("plan-%d", i),
			Region:    "us-east-1",
			Status:    InfraStatusReady,
			Resources: map[string]string{"vpc": fmt.Sprintf("vpc-%d", i)},
			CreatedAt: time.Now(),
		}
		if err := store.CreateInfra(infra); err != nil {
			t.Fatalf("CreateInfra: %v", err)
		}
	}

	const numListers = 20
	const iterations = 50

	var wg sync.WaitGroup
	errCh := make(chan error, numListers*iterations)

	// Run concurrent list operations
	for l := 0; l < numListers; l++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				infras, err := store.ListInfra()
				if err != nil {
					errCh <- fmt.Errorf("ListInfra: %w", err)
					continue
				}
				// Should see at least 10 items (or fewer if deletes happen)
				if len(infras) < 1 {
					errCh <- fmt.Errorf("ListInfra returned %d items, want >= 1", len(infras))
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		t.Errorf("Concurrent listing had %d errors", len(errs))
	}
}

// TestConcurrentDeleteOperations verifies concurrent delete operations don't panic.
func TestConcurrentDeleteOperations(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create deployments
	const numDeployments = 20
	for i := 0; i < numDeployments; i++ {
		deploy := &Deployment{
			ID:        fmt.Sprintf("deploy-del-%d", i),
			InfraID:   "infra-test",
			ImageRef:  "nginx:latest",
			Status:    DeploymentStatusRunning,
			CreatedAt: time.Now(),
		}
		if err := store.CreateDeployment(deploy); err != nil {
			t.Fatalf("CreateDeployment: %v", err)
		}
	}

	var wg sync.WaitGroup

	// Multiple goroutines try to delete the same items
	// This tests that concurrent deletes don't panic or corrupt state
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numDeployments; i++ {
				// Ignore errors - multiple goroutines may try to delete same item
				_ = store.DeleteDeployment(fmt.Sprintf("deploy-del-%d", i))
			}
		}()
	}

	wg.Wait()

	// Verify all deployments are deleted
	deploys, err := store.ListDeployments()
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	for _, d := range deploys {
		if d.ID[:11] == "deploy-del-" {
			t.Errorf("Deployment %s should have been deleted", d.ID)
		}
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// --- Infrastructure State Transition Tests ---

func TestInfraStateTransitions_ValidTransitions(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		wantErr  bool
		errCheck func(error) bool
	}{
		// From provisioning
		{name: "provisioning->ready", from: InfraStatusProvisioning, to: InfraStatusReady, wantErr: false},
		{name: "provisioning->failed", from: InfraStatusProvisioning, to: InfraStatusFailed, wantErr: false},
		{name: "provisioning->destroyed (invalid)", from: InfraStatusProvisioning, to: InfraStatusDestroyed, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "provisioning->provisioning (idempotent)", from: InfraStatusProvisioning, to: InfraStatusProvisioning, wantErr: false},

		// From ready
		{name: "ready->destroyed", from: InfraStatusReady, to: InfraStatusDestroyed, wantErr: false},
		{name: "ready->provisioning (invalid)", from: InfraStatusReady, to: InfraStatusProvisioning, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "ready->failed (invalid)", from: InfraStatusReady, to: InfraStatusFailed, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "ready->ready (idempotent)", from: InfraStatusReady, to: InfraStatusReady, wantErr: false},

		// From failed
		{name: "failed->provisioning (retry)", from: InfraStatusFailed, to: InfraStatusProvisioning, wantErr: false},
		{name: "failed->destroyed (teardown)", from: InfraStatusFailed, to: InfraStatusDestroyed, wantErr: false},
		{name: "failed->ready (invalid)", from: InfraStatusFailed, to: InfraStatusReady, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "failed->failed (idempotent)", from: InfraStatusFailed, to: InfraStatusFailed, wantErr: false},

		// From destroyed (terminal)
		{name: "destroyed->provisioning (invalid)", from: InfraStatusDestroyed, to: InfraStatusProvisioning, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "destroyed->ready (invalid)", from: InfraStatusDestroyed, to: InfraStatusReady, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "destroyed->failed (invalid)", from: InfraStatusDestroyed, to: InfraStatusFailed, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "destroyed->destroyed (idempotent)", from: InfraStatusDestroyed, to: InfraStatusDestroyed, wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			store, _ := NewStore(dir)

			infra := &Infrastructure{
				ID:        "infra-" + tc.name,
				PlanID:    "plan-test",
				Region:    "us-east-1",
				Resources: map[string]string{},
				Status:    tc.from,
				CreatedAt: time.Now(),
			}

			if err := store.CreateInfra(infra); err != nil {
				t.Fatalf("CreateInfra: %v", err)
			}

			err := store.SetInfraStatus(infra.ID, tc.to)

			if tc.wantErr {
				if err == nil {
					t.Errorf("SetInfraStatus(%s, %s) = nil, want error", tc.from, tc.to)
				} else if tc.errCheck != nil && !tc.errCheck(err) {
					t.Errorf("SetInfraStatus(%s, %s) = %v, want ErrInvalidState", tc.from, tc.to, err)
				}
			} else {
				if err != nil {
					t.Errorf("SetInfraStatus(%s, %s) = %v, want nil", tc.from, tc.to, err)
				} else {
					// Verify status was actually updated
					got, _ := store.GetInfra(infra.ID)
					if got.Status != tc.to {
						t.Errorf("Status = %q, want %q", got.Status, tc.to)
					}
				}
			}
		})
	}
}

func TestInfraStateTransitions_UnknownStatus(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	infra := &Infrastructure{
		ID:        "infra-unknown",
		PlanID:    "plan-test",
		Region:    "us-east-1",
		Resources: map[string]string{},
		Status:    "unknown-status", // Invalid starting status
		CreatedAt: time.Now(),
	}

	if err := store.CreateInfra(infra); err != nil {
		t.Fatalf("CreateInfra: %v", err)
	}

	err := store.SetInfraStatus(infra.ID, InfraStatusReady)
	if err == nil {
		t.Error("SetInfraStatus with unknown from status = nil, want error")
	}
	if !errors.Is(err, apperrors.ErrInvalidState) {
		t.Errorf("SetInfraStatus error = %v, want ErrInvalidState", err)
	}
}

func TestInfraStateTransitions_NotFound(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	err := store.SetInfraStatus("nonexistent", InfraStatusReady)
	if !errors.Is(err, apperrors.ErrInfraNotFound) {
		t.Errorf("SetInfraStatus nonexistent = %v, want ErrInfraNotFound", err)
	}
}

// --- Deployment State Transition Tests ---

func TestDeploymentStateTransitions_ValidTransitions(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		wantErr  bool
		errCheck func(error) bool
	}{
		// From deploying
		{name: "deploying->running", from: DeploymentStatusDeploying, to: DeploymentStatusRunning, wantErr: false},
		{name: "deploying->failed", from: DeploymentStatusDeploying, to: DeploymentStatusFailed, wantErr: false},
		{name: "deploying->stopped", from: DeploymentStatusDeploying, to: DeploymentStatusStopped, wantErr: false},
		{name: "deploying->deploying (idempotent)", from: DeploymentStatusDeploying, to: DeploymentStatusDeploying, wantErr: false},

		// From running
		{name: "running->deploying (update)", from: DeploymentStatusRunning, to: DeploymentStatusDeploying, wantErr: false},
		{name: "running->failed", from: DeploymentStatusRunning, to: DeploymentStatusFailed, wantErr: false},
		{name: "running->stopped", from: DeploymentStatusRunning, to: DeploymentStatusStopped, wantErr: false},
		{name: "running->running (idempotent)", from: DeploymentStatusRunning, to: DeploymentStatusRunning, wantErr: false},

		// From failed
		{name: "failed->deploying (retry)", from: DeploymentStatusFailed, to: DeploymentStatusDeploying, wantErr: false},
		{name: "failed->stopped", from: DeploymentStatusFailed, to: DeploymentStatusStopped, wantErr: false},
		{name: "failed->running (invalid)", from: DeploymentStatusFailed, to: DeploymentStatusRunning, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "failed->failed (idempotent)", from: DeploymentStatusFailed, to: DeploymentStatusFailed, wantErr: false},

		// From stopped (terminal)
		{name: "stopped->deploying (invalid)", from: DeploymentStatusStopped, to: DeploymentStatusDeploying, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "stopped->running (invalid)", from: DeploymentStatusStopped, to: DeploymentStatusRunning, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "stopped->failed (invalid)", from: DeploymentStatusStopped, to: DeploymentStatusFailed, wantErr: true, errCheck: func(err error) bool { return errors.Is(err, apperrors.ErrInvalidState) }},
		{name: "stopped->stopped (idempotent)", from: DeploymentStatusStopped, to: DeploymentStatusStopped, wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			store, _ := NewStore(dir)

			deploy := &Deployment{
				ID:          "deploy-" + tc.name,
				InfraID:     "infra-test",
				ImageRef:    "nginx:latest",
				Status:      tc.from,
				URLs:        []string{},
				CreatedAt:   time.Now(),
				LastUpdated: time.Now(),
			}

			if err := store.CreateDeployment(deploy); err != nil {
				t.Fatalf("CreateDeployment: %v", err)
			}

			err := store.UpdateDeploymentStatus(deploy.ID, tc.to, nil)

			if tc.wantErr {
				if err == nil {
					t.Errorf("UpdateDeploymentStatus(%s, %s) = nil, want error", tc.from, tc.to)
				} else if tc.errCheck != nil && !tc.errCheck(err) {
					t.Errorf("UpdateDeploymentStatus(%s, %s) = %v, want ErrInvalidState", tc.from, tc.to, err)
				}
			} else {
				if err != nil {
					t.Errorf("UpdateDeploymentStatus(%s, %s) = %v, want nil", tc.from, tc.to, err)
				} else {
					// Verify status was actually updated
					got, _ := store.GetDeployment(deploy.ID)
					if got.Status != tc.to {
						t.Errorf("Status = %q, want %q", got.Status, tc.to)
					}
				}
			}
		})
	}
}

func TestDeploymentStateTransitions_UnknownStatus(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	deploy := &Deployment{
		ID:          "deploy-unknown",
		InfraID:     "infra-test",
		ImageRef:    "nginx:latest",
		Status:      "unknown-status", // Invalid starting status
		URLs:        []string{},
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	if err := store.CreateDeployment(deploy); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	err := store.UpdateDeploymentStatus(deploy.ID, DeploymentStatusRunning, nil)
	if err == nil {
		t.Error("UpdateDeploymentStatus with unknown from status = nil, want error")
	}
	if !errors.Is(err, apperrors.ErrInvalidState) {
		t.Errorf("UpdateDeploymentStatus error = %v, want ErrInvalidState", err)
	}
}

func TestDeploymentStateTransitions_NotFound(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	err := store.UpdateDeploymentStatus("nonexistent", DeploymentStatusRunning, nil)
	if !errors.Is(err, apperrors.ErrDeploymentNotFound) {
		t.Errorf("UpdateDeploymentStatus nonexistent = %v, want ErrDeploymentNotFound", err)
	}
}

func TestDeploymentStateTransitions_PreservesURLs(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	deploy := &Deployment{
		ID:          "deploy-urls",
		InfraID:     "infra-test",
		ImageRef:    "nginx:latest",
		Status:      DeploymentStatusDeploying,
		URLs:        []string{"http://original.com"},
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	if err := store.CreateDeployment(deploy); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	// Update status with nil URLs - should preserve original
	if err := store.UpdateDeploymentStatus(deploy.ID, DeploymentStatusRunning, nil); err != nil {
		t.Fatalf("UpdateDeploymentStatus: %v", err)
	}

	got, _ := store.GetDeployment(deploy.ID)
	if len(got.URLs) != 1 || got.URLs[0] != "http://original.com" {
		t.Errorf("URLs = %v, want [http://original.com]", got.URLs)
	}

	// Update status with new URLs - should replace
	newURLs := []string{"http://new.com", "https://secure.new.com"}
	if err := store.UpdateDeploymentStatus(deploy.ID, DeploymentStatusRunning, newURLs); err != nil {
		t.Fatalf("UpdateDeploymentStatus with new URLs: %v", err)
	}

	got, _ = store.GetDeployment(deploy.ID)
	if len(got.URLs) != 2 || got.URLs[0] != "http://new.com" {
		t.Errorf("URLs = %v, want %v", got.URLs, newURLs)
	}
}

func TestDeploymentStateTransitions_UpdatesLastUpdated(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	now := time.Now()
	deploy := &Deployment{
		ID:          "deploy-time",
		InfraID:     "infra-test",
		ImageRef:    "nginx:latest",
		Status:      DeploymentStatusDeploying,
		URLs:        []string{},
		CreatedAt:   now,
		LastUpdated: now,
	}

	if err := store.CreateDeployment(deploy); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // Ensure time difference

	if err := store.UpdateDeploymentStatus(deploy.ID, DeploymentStatusRunning, nil); err != nil {
		t.Fatalf("UpdateDeploymentStatus: %v", err)
	}

	got, _ := store.GetDeployment(deploy.ID)
	if !got.LastUpdated.After(now) {
		t.Errorf("LastUpdated = %v, want after %v", got.LastUpdated, now)
	}
}
