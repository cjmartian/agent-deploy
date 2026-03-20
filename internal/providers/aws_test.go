package providers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cjmartian/agent-deploy/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestPlanInfra tests the plan infrastructure tool.
func TestPlanInfra(t *testing.T) {
	// Set higher budget limit for test.
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	provider := NewAWSProvider(store)

	input := planInfraInput{
		AppDescription: "Test web app",
		ExpectedUsers:  100,
		LatencyMS:      200,
		Region:         "us-east-1",
	}

	_, output, err := provider.planInfra(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("planInfra: %v", err)
	}

	// Verify output.
	if output.PlanID == "" {
		t.Error("PlanID should not be empty")
	}
	if len(output.Services) == 0 {
		t.Error("Services should not be empty")
	}
	if output.EstimatedCostMo == "" {
		t.Error("EstimatedCostMo should not be empty")
	}
	if output.Summary == "" {
		t.Error("Summary should not be empty")
	}

	// Verify plan was persisted.
	plan, err := store.GetPlan(output.PlanID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan.AppDescription != input.AppDescription {
		t.Errorf("AppDescription = %q, want %q", plan.AppDescription, input.AppDescription)
	}
	if plan.Status != state.PlanStatusCreated {
		t.Errorf("Status = %q, want %q", plan.Status, state.PlanStatusCreated)
	}
}

// TestPlanInfra_SpendingLimit tests that planInfra rejects plans exceeding per-deployment limit.
func TestPlanInfra_SpendingLimit(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Request a plan for many users (high cost).
	input := planInfraInput{
		AppDescription: "High traffic app",
		ExpectedUsers:  10000, // This should result in estimated cost > $25/mo default limit.
		LatencyMS:      50,
		Region:         "us-east-1",
	}

	_, _, err := provider.planInfra(context.Background(), nil, input)
	if err == nil {
		t.Error("Expected error for plan exceeding spending limit")
	}
}

// TestDeploymentsResource tests the aws:deployments resource.
func TestDeploymentsResource(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create some test deployments.
	deploy1 := &state.Deployment{
		ID:          "deploy-test-001",
		InfraID:     "infra-test-001",
		ImageRef:    "nginx:latest",
		Status:      state.DeploymentStatusRunning,
		URLs:        []string{"http://example.com"},
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}
	deploy2 := &state.Deployment{
		ID:          "deploy-test-002",
		InfraID:     "infra-test-002",
		ImageRef:    "nginx:alpine",
		Status:      state.DeploymentStatusStopped,
		URLs:        []string{},
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	store.CreateDeployment(deploy1)
	store.CreateDeployment(deploy2)

	// Call resource handler.
	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "aws:deployments",
		},
	}

	result, err := provider.deploymentsResource(context.Background(), req)
	if err != nil {
		t.Fatalf("deploymentsResource: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("Expected 1 content, got %d", len(result.Contents))
	}

	// Parse response.
	var resp deploymentsResponse
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(resp.Deployments) != 2 {
		t.Errorf("Expected 2 deployments, got %d", len(resp.Deployments))
	}
}

// TestStatusOutput tests status output structure.
func TestStatusOutput_JSON(t *testing.T) {
	output := statusOutput{
		DeploymentID: "deploy-123",
		Status:       "running",
		URLs:         []string{"http://example.com"},
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var parsed statusOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if parsed.DeploymentID != output.DeploymentID {
		t.Errorf("DeploymentID = %q, want %q", parsed.DeploymentID, output.DeploymentID)
	}
}
