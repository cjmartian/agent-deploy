package state

import (
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
	if err := store.CreatePlan(plan); err != nil {
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
	if err := store.ApprovePlan(plan.ID); err != nil {
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
	if err != apperrors.ErrPlanNotFound {
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
	store.CreatePlan(plan)

	err := store.ApprovePlan(plan.ID)
	if err != apperrors.ErrPlanExpired {
		t.Errorf("ApprovePlan error = %v, want %v", err, apperrors.ErrPlanExpired)
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
	if err != apperrors.ErrDeploymentNotFound {
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

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
