package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
