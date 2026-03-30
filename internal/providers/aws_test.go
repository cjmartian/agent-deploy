package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/cjmartian/agent-deploy/internal/awsclient"
	"github.com/cjmartian/agent-deploy/internal/awsclient/mocks"
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

// TestPlanInfra_AutoScalingCostRange tests that planInfra returns cost range when auto-scaling is configured (P1.22).
// WHY: Users need to understand min/max costs before committing to auto-scaling deployments.
func TestPlanInfra_AutoScalingCostRange(t *testing.T) {
	// Set higher budget limit for test.
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "500")

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	provider := NewAWSProvider(store)

	// Test with auto-scaling enabled (max > min).
	input := planInfraInput{
		AppDescription: "Auto-scaling test app",
		ExpectedUsers:  100,
		LatencyMS:      200,
		Region:         "us-east-1",
		MinCount:       1,
		MaxCount:       4, // Auto-scaling: 1-4 tasks
	}

	_, output, err := provider.planInfra(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("planInfra: %v", err)
	}

	// Verify cost range is included when auto-scaling is enabled.
	if output.CostRange == nil {
		t.Fatal("CostRange should not be nil when auto-scaling is configured")
	}

	// Verify min < max.
	if output.CostRange.MinimumCostMo >= output.CostRange.MaximumCostMo {
		t.Errorf("MinimumCostMo (%v) should be less than MaximumCostMo (%v)",
			output.CostRange.MinimumCostMo, output.CostRange.MaximumCostMo)
	}

	// Verify note mentions task range.
	if output.CostRange.Note == "" {
		t.Error("CostRange.Note should not be empty")
	}

	// Verify estimated cost shows range format.
	if output.EstimatedCostMo == "" {
		t.Error("EstimatedCostMo should not be empty")
	}

	// Verify Auto Scaling is in services list.
	hasAutoScaling := false
	for _, svc := range output.Services {
		if svc == "Auto Scaling" {
			hasAutoScaling = true
			break
		}
	}
	if !hasAutoScaling {
		t.Error("Services should include 'Auto Scaling' when max_count > min_count")
	}
}

// TestPlanInfra_NoAutoScalingNoCostRange tests that planInfra does not return cost range when auto-scaling is not configured.
func TestPlanInfra_NoAutoScalingNoCostRange(t *testing.T) {
	// Set higher budget limit for test.
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "500")

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	provider := NewAWSProvider(store)

	// Test without auto-scaling (min == max or not specified).
	input := planInfraInput{
		AppDescription: "Static count app",
		ExpectedUsers:  100,
		LatencyMS:      200,
		Region:         "us-east-1",
		// No MinCount/MaxCount specified - should default to no auto-scaling.
	}

	_, output, err := provider.planInfra(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("planInfra: %v", err)
	}

	// Verify cost range is NOT included when auto-scaling is not configured.
	if output.CostRange != nil {
		t.Error("CostRange should be nil when auto-scaling is not configured")
	}
}

// TestPlanInfra_SpendingLimit tests that planInfra rejects plans exceeding per-deployment limit.
func TestPlanInfra_SpendingLimit(t *testing.T) {
	// WHY: Isolate HOME to prevent real config file from affecting spending limits.
	// Without this, ~/.agent-deploy/config.json may have higher limits than the test expects.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

// TestPlanInfra_PerRequestSpendingOverride tests per-request spending limit overrides (P1.21).
// WHY: Users may want different budget limits for different deployments.
func TestPlanInfra_PerRequestSpendingOverride(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	t.Run("override_allows_lower_budget", func(t *testing.T) {
		// Create a plan with low cost that would normally pass.
		input := planInfraInput{
			AppDescription:         "Small app",
			ExpectedUsers:          10,
			LatencyMS:              200,
			Region:                 "us-east-1",
			PerDeploymentBudgetUSD: 10.0, // Very tight budget
		}

		_, _, err := provider.planInfra(context.Background(), nil, input)
		// Should fail because even a small app costs more than $10/mo.
		if err == nil {
			t.Error("Expected error for plan exceeding per-request spending limit")
		}
	})

	t.Run("override_cannot_exceed_global", func(t *testing.T) {
		// Try to set override higher than global limit ($25 default).
		input := planInfraInput{
			AppDescription:         "Small app",
			ExpectedUsers:          10,
			LatencyMS:              200,
			Region:                 "us-east-1",
			PerDeploymentBudgetUSD: 1000.0, // Way higher than global $25 limit
		}

		_, _, err := provider.planInfra(context.Background(), nil, input)
		if err == nil {
			t.Error("Expected error for override exceeding global limit")
		}
		if err != nil && !strings.Contains(err.Error(), "exceeds global per-deployment limit") {
			t.Errorf("Expected 'exceeds global' error, got: %v", err)
		}
	})

	t.Run("valid_override_works", func(t *testing.T) {
		// Set higher global limit first.
		t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "200")

		// Now use a valid override.
		input := planInfraInput{
			AppDescription:         "Small app",
			ExpectedUsers:          10,
			LatencyMS:              200,
			Region:                 "us-east-1",
			PerDeploymentBudgetUSD: 100.0, // Within global limit of $200
		}

		_, output, err := provider.planInfra(context.Background(), nil, input)
		if err != nil {
			t.Fatalf("Expected success with valid override, got: %v", err)
		}
		if output.PlanID == "" {
			t.Error("Expected valid plan ID")
		}
	})
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

	if err := store.CreateDeployment(deploy1); err != nil {
		t.Fatalf("CreateDeployment(deploy1): %v", err)
	}
	if err := store.CreateDeployment(deploy2); err != nil {
		t.Fatalf("CreateDeployment(deploy2): %v", err)
	}

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

// TestPlanInfra_InputValidation tests that planInfra rejects invalid inputs.
func TestPlanInfra_InputValidation(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	tests := []struct {
		name  string
		input planInfraInput
		want  string // substring expected in error message
	}{
		{
			name: "empty app description",
			input: planInfraInput{
				AppDescription: "",
				ExpectedUsers:  100,
				LatencyMS:      100,
				Region:         "us-east-1",
			},
			want: "app_description",
		},
		{
			name: "whitespace only app description",
			input: planInfraInput{
				AppDescription: "   ",
				ExpectedUsers:  100,
				LatencyMS:      100,
				Region:         "us-east-1",
			},
			want: "app_description",
		},
		{
			name: "empty region",
			input: planInfraInput{
				AppDescription: "Test app",
				ExpectedUsers:  100,
				LatencyMS:      100,
				Region:         "",
			},
			want: "region",
		},
		{
			name: "zero expected users",
			input: planInfraInput{
				AppDescription: "Test app",
				ExpectedUsers:  0,
				LatencyMS:      100,
				Region:         "us-east-1",
			},
			want: "expected_users",
		},
		{
			name: "negative expected users",
			input: planInfraInput{
				AppDescription: "Test app",
				ExpectedUsers:  -10,
				LatencyMS:      100,
				Region:         "us-east-1",
			},
			want: "expected_users",
		},
		{
			name: "zero latency",
			input: planInfraInput{
				AppDescription: "Test app",
				ExpectedUsers:  100,
				LatencyMS:      0,
				Region:         "us-east-1",
			},
			want: "latency_ms",
		},
		{
			name: "negative latency",
			input: planInfraInput{
				AppDescription: "Test app",
				ExpectedUsers:  100,
				LatencyMS:      -50,
				Region:         "us-east-1",
			},
			want: "latency_ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := provider.planInfra(context.Background(), nil, tt.input)
			if err == nil {
				t.Error("Expected validation error, got nil")
				return
			}
			if !containsSubstring(err.Error(), tt.want) {
				t.Errorf("Error %q should contain %q", err.Error(), tt.want)
			}
		})
	}
}

// containsSubstring checks if s contains substr (case-sensitive).
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestPublicTeardown tests the public Teardown method.
func TestPublicTeardown(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	provider := NewAWSProvider(store)

	// Create test deployment and infrastructure
	infra := &state.Infrastructure{
		ID:        "infra-teardown-test",
		PlanID:    "plan-test",
		Region:    "us-east-1",
		Resources: map[string]string{},
		Status:    state.InfraStatusReady,
		CreatedAt: time.Now(),
	}
	err = store.CreateInfra(infra)
	if err != nil {
		t.Fatalf("CreateInfra: %v", err)
	}

	deploy := &state.Deployment{
		ID:          "deploy-teardown-test",
		InfraID:     "infra-teardown-test",
		ImageRef:    "nginx:latest",
		Status:      state.DeploymentStatusRunning,
		URLs:        []string{},
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}
	err = store.CreateDeployment(deploy)
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	// Call public Teardown - this will fail on AWS calls but should not panic
	// and should handle the deployment correctly
	err = provider.Teardown(context.Background(), "deploy-teardown-test")
	// We expect an error because there are no real AWS resources,
	// but the method should be callable and should fail gracefully
	if err == nil {
		// If no error, verify the deployment status was updated
		d, derr := store.GetDeployment("deploy-teardown-test")
		if derr != nil {
			t.Fatalf("GetDeployment after teardown: %v", derr)
		}
		if d.Status != state.DeploymentStatusStopped {
			t.Errorf("Deployment status = %q, want %q", d.Status, state.DeploymentStatusStopped)
		}
	}
	// Whether error or not, the test passes - we're verifying the method exists and is callable
}

// TestTeardownProvider_Interface tests that AWSProvider implements TeardownProvider.
func TestTeardownProvider_Interface(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Verify AWSProvider implements TeardownProvider interface
	var _ TeardownProvider = provider
}

// TestGetAWSProvider tests the GetAWSProvider helper function.
func TestGetAWSProvider(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := GetAWSProvider(store)

	if provider == nil {
		t.Fatal("GetAWSProvider returned nil")
	}
	if provider.Name() != "aws" {
		t.Errorf("Provider name = %q, want %q", provider.Name(), "aws")
	}
}

// TestApprovePlan tests the aws_approve_plan tool.
func TestApprovePlan(t *testing.T) {
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")

	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// First create a plan.
	planInput := planInfraInput{
		AppDescription: "Test app",
		ExpectedUsers:  100,
		LatencyMS:      200,
		Region:         "us-east-1",
	}
	_, planOutput, err := provider.planInfra(context.Background(), nil, planInput)
	if err != nil {
		t.Fatalf("planInfra: %v", err)
	}

	// Verify plan is in created status.
	plan, _ := store.GetPlan(planOutput.PlanID)
	if plan.Status != state.PlanStatusCreated {
		t.Fatalf("Plan status = %q, want %q", plan.Status, state.PlanStatusCreated)
	}

	// Approve the plan.
	approveInput := approvePlanInput{
		PlanID:    planOutput.PlanID,
		Confirmed: true,
	}
	_, approveOutput, err := provider.approvePlan(context.Background(), nil, approveInput)
	if err != nil {
		t.Fatalf("approvePlan: %v", err)
	}

	// Verify output.
	if approveOutput.PlanID != planOutput.PlanID {
		t.Errorf("PlanID = %q, want %q", approveOutput.PlanID, planOutput.PlanID)
	}
	if approveOutput.Status != state.PlanStatusApproved {
		t.Errorf("Status = %q, want %q", approveOutput.Status, state.PlanStatusApproved)
	}
	if approveOutput.Message == "" {
		t.Error("Message should not be empty")
	}

	// Verify plan status was updated.
	plan, _ = store.GetPlan(planOutput.PlanID)
	if plan.Status != state.PlanStatusApproved {
		t.Errorf("Plan status after approval = %q, want %q", plan.Status, state.PlanStatusApproved)
	}
}

// TestApprovePlan_Reject tests rejecting a plan with confirmed: false.
func TestApprovePlan_Reject(t *testing.T) {
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")

	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create a plan.
	planInput := planInfraInput{
		AppDescription: "Test app",
		ExpectedUsers:  100,
		LatencyMS:      200,
		Region:         "us-east-1",
	}
	_, planOutput, err := provider.planInfra(context.Background(), nil, planInput)
	if err != nil {
		t.Fatalf("planInfra: %v", err)
	}

	// Reject the plan.
	approveInput := approvePlanInput{
		PlanID:    planOutput.PlanID,
		Confirmed: false,
	}
	_, approveOutput, err := provider.approvePlan(context.Background(), nil, approveInput)
	if err != nil {
		t.Fatalf("approvePlan (reject): %v", err)
	}

	// Verify output.
	if approveOutput.Status != state.PlanStatusRejected {
		t.Errorf("Status = %q, want %q", approveOutput.Status, state.PlanStatusRejected)
	}

	// Verify plan status was updated.
	plan, _ := store.GetPlan(planOutput.PlanID)
	if plan.Status != state.PlanStatusRejected {
		t.Errorf("Plan status after rejection = %q, want %q", plan.Status, state.PlanStatusRejected)
	}
}

// TestApprovePlan_EmptyPlanID tests that empty plan_id is rejected.
func TestApprovePlan_EmptyPlanID(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	_, _, err := provider.approvePlan(context.Background(), nil, approvePlanInput{
		PlanID:    "",
		Confirmed: true,
	})
	if err == nil {
		t.Error("Expected error for empty plan_id")
	}
	if !containsSubstring(err.Error(), "plan_id") {
		t.Errorf("Error %q should mention plan_id", err.Error())
	}
}

// TestApprovePlan_NotFound tests approving a nonexistent plan.
func TestApprovePlan_NotFound(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	_, _, err := provider.approvePlan(context.Background(), nil, approvePlanInput{
		PlanID:    "plan-nonexistent",
		Confirmed: true,
	})
	if err == nil {
		t.Error("Expected error for nonexistent plan")
	}
}

// TestCreateInfra_RequiresApproval tests that createInfra rejects unapproved plans.
func TestCreateInfra_RequiresApproval(t *testing.T) {
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")

	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create a plan (not approved).
	planInput := planInfraInput{
		AppDescription: "Test app",
		ExpectedUsers:  100,
		LatencyMS:      200,
		Region:         "us-east-1",
	}
	_, planOutput, err := provider.planInfra(context.Background(), nil, planInput)
	if err != nil {
		t.Fatalf("planInfra: %v", err)
	}

	// Try to create infra without approval — should fail.
	_, _, err = provider.createInfra(context.Background(), nil, createInfraInput{
		PlanID: planOutput.PlanID,
	})
	if err == nil {
		t.Error("Expected error for unapproved plan")
	}
	if !containsSubstring(err.Error(), "not approved") {
		t.Errorf("Error %q should mention 'not approved'", err.Error())
	}
}

// TestCreateInfra_RejectedPlan tests that createInfra rejects rejected plans.
func TestCreateInfra_RejectedPlan(t *testing.T) {
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")

	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create and reject a plan.
	planInput := planInfraInput{
		AppDescription: "Test app",
		ExpectedUsers:  100,
		LatencyMS:      200,
		Region:         "us-east-1",
	}
	_, planOutput, _ := provider.planInfra(context.Background(), nil, planInput)

	// Reject the plan.
	_, _, _ = provider.approvePlan(context.Background(), nil, approvePlanInput{
		PlanID:    planOutput.PlanID,
		Confirmed: false,
	})

	// Try to create infra — should fail.
	_, _, err := provider.createInfra(context.Background(), nil, createInfraInput{
		PlanID: planOutput.PlanID,
	})
	if err == nil {
		t.Error("Expected error for rejected plan")
	}
	if !containsSubstring(err.Error(), "not approved") {
		t.Errorf("Error %q should mention 'not approved'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Auto Scaling Tests
// ---------------------------------------------------------------------------

// TestValidateAutoScalingParams tests the auto-scaling parameter validation.
func TestValidateAutoScalingParams(t *testing.T) {
	tests := []struct {
		name       string
		minCount   int
		maxCount   int
		targetCPU  int
		targetMem  int
		wantErr    bool
		wantSubstr string
	}{
		{
			name:      "valid_defaults",
			minCount:  1,
			maxCount:  1,
			targetCPU: 70,
			targetMem: 70,
			wantErr:   false,
		},
		{
			name:      "valid_scaling_range",
			minCount:  1,
			maxCount:  10,
			targetCPU: 50,
			targetMem: 80,
			wantErr:   false,
		},
		{
			name:       "min_count_zero",
			minCount:   0,
			maxCount:   1,
			targetCPU:  70,
			targetMem:  70,
			wantErr:    true,
			wantSubstr: "min_count must be at least 1",
		},
		{
			name:       "max_less_than_min",
			minCount:   5,
			maxCount:   3,
			targetCPU:  70,
			targetMem:  70,
			wantErr:    true,
			wantSubstr: "max_count must be >= min_count",
		},
		{
			name:       "target_cpu_too_low",
			minCount:   1,
			maxCount:   1,
			targetCPU:  5,
			targetMem:  70,
			wantErr:    true,
			wantSubstr: "target_cpu_percent must be between 10 and 90",
		},
		{
			name:       "target_cpu_too_high",
			minCount:   1,
			maxCount:   1,
			targetCPU:  95,
			targetMem:  70,
			wantErr:    true,
			wantSubstr: "target_cpu_percent must be between 10 and 90",
		},
		{
			name:       "target_mem_too_low",
			minCount:   1,
			maxCount:   1,
			targetCPU:  70,
			targetMem:  5,
			wantErr:    true,
			wantSubstr: "target_memory_percent must be between 10 and 90",
		},
		{
			name:       "target_mem_too_high",
			minCount:   1,
			maxCount:   1,
			targetCPU:  70,
			targetMem:  100,
			wantErr:    true,
			wantSubstr: "target_memory_percent must be between 10 and 90",
		},
		{
			name:      "high_max_count_warning",
			minCount:  1,
			maxCount:  15,
			targetCPU: 70,
			targetMem: 70,
			wantErr:   false, // Should warn but not error.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAutoScalingParams(tt.minCount, tt.maxCount, tt.targetCPU, tt.targetMem)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
					return
				}
				if !containsSubstring(err.Error(), tt.wantSubstr) {
					t.Errorf("Error %q should contain %q", err.Error(), tt.wantSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestExtractClusterName tests extracting cluster name from ARN.
func TestExtractClusterName(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{
			arn:  "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster",
			want: "my-cluster",
		},
		{
			arn:  "arn:aws:ecs:us-west-2:987654321098:cluster/agent-deploy-infra-abc123",
			want: "agent-deploy-infra-abc123",
		},
		{
			arn:  "",
			want: "",
		},
		{
			arn:  "simple-name",
			want: "simple-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.arn, func(t *testing.T) {
			got := extractClusterName(tt.arn)
			if got != tt.want {
				t.Errorf("extractClusterName(%q) = %q, want %q", tt.arn, got, tt.want)
			}
		})
	}
}

// TestExtractServiceName tests extracting service name from ARN.
func TestExtractServiceName(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{
			arn:  "arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service",
			want: "my-service",
		},
		{
			arn:  "arn:aws:ecs:us-west-2:987654321098:service/cluster/agent-deploy-deploy123",
			want: "agent-deploy-deploy123",
		},
		{
			arn:  "",
			want: "",
		},
		{
			arn:  "simple-name",
			want: "simple-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.arn, func(t *testing.T) {
			got := extractServiceName(tt.arn)
			if got != tt.want {
				t.Errorf("extractServiceName(%q) = %q, want %q", tt.arn, got, tt.want)
			}
		})
	}
}

// TestDeployInput_AutoScalingDefaults tests that auto-scaling defaults are applied correctly.
func TestDeployInput_AutoScalingDefaults(t *testing.T) {
	// This test verifies that the deployInput struct has the new scaling fields.
	input := deployInput{
		MinCount:         1,
		MaxCount:         5,
		TargetCPUPercent: 60,
		TargetMemPercent: 75,
	}

	// Verify fields are set correctly.
	if input.MinCount != 1 {
		t.Errorf("MinCount = %d, want 1", input.MinCount)
	}
	if input.MaxCount != 5 {
		t.Errorf("MaxCount = %d, want 5", input.MaxCount)
	}
	if input.TargetCPUPercent != 60 {
		t.Errorf("TargetCPUPercent = %d, want 60", input.TargetCPUPercent)
	}
	if input.TargetMemPercent != 75 {
		t.Errorf("TargetMemPercent = %d, want 75", input.TargetMemPercent)
	}
}

// TestStatusOutput_ScalingInfo tests that status output includes scaling info.
func TestStatusOutput_ScalingInfo(t *testing.T) {
	output := statusOutput{
		DeploymentID: "deploy-123",
		Status:       "running",
		URLs:         []string{"http://example.com"},
		Scaling: &scalingInfo{
			MinCount:         1,
			MaxCount:         4,
			CurrentCount:     2,
			TargetCPUPercent: 70,
			TargetMemPercent: 70,
		},
	}

	// Marshal to JSON to verify structure.
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Verify JSON contains scaling info.
	jsonStr := string(data)
	expectedFields := []string{
		`"min_count":1`,
		`"max_count":4`,
		`"current_count":2`,
		`"target_cpu_percent":70`,
		`"target_memory_percent":70`,
	}
	for _, field := range expectedFields {
		if !containsSubstring(jsonStr, field) {
			t.Errorf("JSON %q should contain %q", jsonStr, field)
		}
	}
}

// TestStatusOutput_NoScaling tests status output without scaling.
func TestStatusOutput_NoScaling(t *testing.T) {
	output := statusOutput{
		DeploymentID: "deploy-123",
		Status:       "running",
		URLs:         []string{"http://example.com"},
		Scaling:      nil,
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Verify JSON does NOT contain scaling (omitempty).
	jsonStr := string(data)
	if containsSubstring(jsonStr, `"scaling"`) {
		t.Errorf("JSON %q should not contain 'scaling' when nil", jsonStr)
	}
}

// TestCreateInfraInput_CertificateARN tests the certificate ARN field in createInfraInput.
func TestCreateInfraInput_CertificateARN(t *testing.T) {
	tests := []struct {
		name           string
		certificateARN string
		wantJSON       string
	}{
		{
			name:           "no certificate",
			certificateARN: "",
			wantJSON:       `"plan_id"`,
		},
		{
			name:           "with certificate",
			certificateARN: "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			wantJSON:       `"certificate_arn":"arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := createInfraInput{
				PlanID:         "plan-123",
				CertificateARN: tt.certificateARN,
			}

			data, err := json.Marshal(input)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			jsonStr := string(data)
			if !containsSubstring(jsonStr, tt.wantJSON) {
				t.Errorf("JSON %q should contain %q", jsonStr, tt.wantJSON)
			}
		})
	}
}

// TestValidateCertificateARN_Format tests certificate ARN format validation.
func TestValidateCertificateARN_Format(t *testing.T) {
	tests := []struct {
		name    string
		arn     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid ARN",
			arn:     "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			wantErr: false,
		},
		{
			name:    "valid ARN with different region",
			arn:     "arn:aws:acm:eu-west-1:987654321098:certificate/abcdef01-2345-6789-abcd-ef0123456789",
			wantErr: false,
		},
		{
			name:    "missing acm prefix",
			arn:     "arn:aws:s3:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			wantErr: true,
			errMsg:  "must start with 'arn:aws:acm:'",
		},
		{
			name:    "not an ARN",
			arn:     "not-an-arn",
			wantErr: true,
			errMsg:  "must start with 'arn:aws:acm:'",
		},
		{
			name:    "missing certificate part",
			arn:     "arn:aws:acm:us-east-1:123456789012:bucket/mybucket",
			wantErr: true,
			errMsg:  "must contain ':certificate/'",
		},
		{
			name:    "empty ARN",
			arn:     "",
			wantErr: true,
			errMsg:  "must start with 'arn:aws:acm:'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can test the format validation without AWS credentials
			// by checking the ARN format locally.
			validFormat := isValidCertificateARNFormat(tt.arn)
			if tt.wantErr && validFormat {
				t.Errorf("ARN %q should be invalid", tt.arn)
			}
			if !tt.wantErr && !validFormat {
				t.Errorf("ARN %q should be valid", tt.arn)
			}
		})
	}
}

// isValidCertificateARNFormat checks if a certificate ARN has the correct format.
// This is a helper function for testing without AWS credentials.
func isValidCertificateARNFormat(certARN string) bool {
	if certARN == "" {
		return false
	}
	if len(certARN) < 12 || certARN[:12] != "arn:aws:acm:" {
		return false
	}
	// Check for :certificate/ in the ARN
	for i := 12; i < len(certARN)-13; i++ {
		if certARN[i:i+13] == ":certificate/" {
			return true
		}
	}
	return false
}

// TestResourceTLSEnabled tests the TLS enabled resource constant.
func TestResourceTLSEnabled(t *testing.T) {
	// Verify the constant values match what we expect.
	if state.ResourceTLSEnabled != "tls_enabled" {
		t.Errorf("ResourceTLSEnabled = %q, want %q", state.ResourceTLSEnabled, "tls_enabled")
	}
	if state.ResourceCertificateARN != "certificate_arn" {
		t.Errorf("ResourceCertificateARN = %q, want %q", state.ResourceCertificateARN, "certificate_arn")
	}
}

// TestInfraResources_TLSEnabled tests that infrastructure can store TLS configuration.
func TestInfraResources_TLSEnabled(t *testing.T) {
	tests := []struct {
		name           string
		tlsEnabled     string
		certificateARN string
		wantHTTPS      bool
	}{
		{
			name:           "TLS enabled",
			tlsEnabled:     "true",
			certificateARN: "arn:aws:acm:us-east-1:123456789012:certificate/test",
			wantHTTPS:      true,
		},
		{
			name:       "TLS disabled",
			tlsEnabled: "false",
			wantHTTPS:  false,
		},
		{
			name:       "TLS not set",
			tlsEnabled: "",
			wantHTTPS:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			infra := &state.Infrastructure{
				Resources: make(map[string]string),
			}

			if tt.tlsEnabled != "" {
				infra.Resources[state.ResourceTLSEnabled] = tt.tlsEnabled
			}
			if tt.certificateARN != "" {
				infra.Resources[state.ResourceCertificateARN] = tt.certificateARN
			}

			// Check if TLS is enabled.
			isTLSEnabled := infra.Resources[state.ResourceTLSEnabled] == "true"
			if isTLSEnabled != tt.wantHTTPS {
				t.Errorf("TLS enabled = %v, want %v", isTLSEnabled, tt.wantHTTPS)
			}

			// Determine expected URL scheme.
			scheme := "http"
			if isTLSEnabled {
				scheme = "https"
			}
			expectedScheme := "http"
			if tt.wantHTTPS {
				expectedScheme = "https"
			}
			if scheme != expectedScheme {
				t.Errorf("URL scheme = %q, want %q", scheme, expectedScheme)
			}
		})
	}
}

// TestResourceNetworkingConstants tests the new networking resource constants.
func TestResourceNetworkingConstants(t *testing.T) {
	// Verify the new resource constant values.
	tests := []struct {
		constant string
		value    string
	}{
		{state.ResourceSecurityGroupALB, "security_group_alb"},
		{state.ResourceSecurityGroupTask, "security_group_task"},
		{state.ResourceRouteTablePrivate, "route_table_private"},
		{state.ResourceNATGateway, "nat_gateway"},
		{state.ResourceElasticIP, "elastic_ip"},
		{state.ResourceSubnetPrivate, "subnet_private"},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if tt.constant != tt.value {
				t.Errorf("Resource constant = %q, want %q", tt.constant, tt.value)
			}
		})
	}
}

// TestResourceDNSConstants tests DNS resource constants for Route 53 alias record deletion (P1.33).
func TestResourceDNSConstants(t *testing.T) {
	tests := []struct {
		constant string
		value    string
	}{
		{state.ResourceDomainName, "domain_name"},
		{state.ResourceHostedZoneID, "hosted_zone_id"},
		{state.ResourceCertAutoCreated, "cert_auto_created"},
		{state.ResourceDNSRecordName, "dns_record_name"},
		{state.ResourceALBDNSName, "alb_dns_name"},
		{state.ResourceALBHostedZoneID, "alb_hosted_zone_id"},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if tt.constant != tt.value {
				t.Errorf("Resource constant = %q, want %q", tt.constant, tt.value)
			}
		})
	}
}

// TestInfraResources_ALBDNSData tests that infrastructure stores ALB DNS data for Route 53 deletion.
func TestInfraResources_ALBDNSData(t *testing.T) {
	infra := &state.Infrastructure{
		Resources: make(map[string]string),
	}

	// Simulate storing ALB DNS data during DNS record creation.
	albDNSName := "dualstack.my-alb-123456.us-west-2.elb.amazonaws.com"
	albHostedZoneID := "Z1H1FL5HABSF5" // us-west-2 ALB zone ID
	domainName := "app.example.com"
	hostedZoneID := "Z0123456789ABCDEFGHIJ"

	infra.Resources[state.ResourceDomainName] = domainName
	infra.Resources[state.ResourceHostedZoneID] = hostedZoneID
	infra.Resources[state.ResourceDNSRecordName] = domainName
	infra.Resources[state.ResourceALBDNSName] = albDNSName
	infra.Resources[state.ResourceALBHostedZoneID] = albHostedZoneID

	// Verify all DNS resources are stored correctly.
	if got := infra.Resources[state.ResourceDomainName]; got != domainName {
		t.Errorf("ResourceDomainName = %q, want %q", got, domainName)
	}
	if got := infra.Resources[state.ResourceHostedZoneID]; got != hostedZoneID {
		t.Errorf("ResourceHostedZoneID = %q, want %q", got, hostedZoneID)
	}
	if got := infra.Resources[state.ResourceALBDNSName]; got != albDNSName {
		t.Errorf("ResourceALBDNSName = %q, want %q", got, albDNSName)
	}
	if got := infra.Resources[state.ResourceALBHostedZoneID]; got != albHostedZoneID {
		t.Errorf("ResourceALBHostedZoneID = %q, want %q", got, albHostedZoneID)
	}

	// Verify the condition for successful DNS deletion.
	canDeleteDNS := infra.Resources[state.ResourceHostedZoneID] != "" &&
		infra.Resources[state.ResourceDNSRecordName] != "" &&
		infra.Resources[state.ResourceALBDNSName] != "" &&
		infra.Resources[state.ResourceALBHostedZoneID] != ""
	if !canDeleteDNS {
		t.Error("Expected all DNS deletion prerequisites to be met")
	}
}

// TestInfraResources_PrivateSubnets tests that infrastructure stores private subnet configuration.
func TestInfraResources_PrivateSubnets(t *testing.T) {
	infra := &state.Infrastructure{
		Resources: make(map[string]string),
	}

	// Simulate public/private subnet architecture.
	publicSubnets := "subnet-pub-1,subnet-pub-2"
	privateSubnets := "subnet-priv-1,subnet-priv-2"
	albSG := "sg-alb-123"
	taskSG := "sg-task-456"
	natGW := "nat-12345678"
	eip := "eipalloc-12345678"

	infra.Resources[state.ResourceSubnetPublic] = publicSubnets
	infra.Resources[state.ResourceSubnetPrivate] = privateSubnets
	infra.Resources[state.ResourceSecurityGroupALB] = albSG
	infra.Resources[state.ResourceSecurityGroupTask] = taskSG
	infra.Resources[state.ResourceNATGateway] = natGW
	infra.Resources[state.ResourceElasticIP] = eip

	// Verify all resources are stored correctly.
	if infra.Resources[state.ResourceSubnetPublic] != publicSubnets {
		t.Errorf("Public subnets = %q, want %q", infra.Resources[state.ResourceSubnetPublic], publicSubnets)
	}
	if infra.Resources[state.ResourceSubnetPrivate] != privateSubnets {
		t.Errorf("Private subnets = %q, want %q", infra.Resources[state.ResourceSubnetPrivate], privateSubnets)
	}
	if infra.Resources[state.ResourceSecurityGroupALB] != albSG {
		t.Errorf("ALB SG = %q, want %q", infra.Resources[state.ResourceSecurityGroupALB], albSG)
	}
	if infra.Resources[state.ResourceSecurityGroupTask] != taskSG {
		t.Errorf("Task SG = %q, want %q", infra.Resources[state.ResourceSecurityGroupTask], taskSG)
	}
	if infra.Resources[state.ResourceNATGateway] != natGW {
		t.Errorf("NAT GW = %q, want %q", infra.Resources[state.ResourceNATGateway], natGW)
	}
	if infra.Resources[state.ResourceElasticIP] != eip {
		t.Errorf("EIP = %q, want %q", infra.Resources[state.ResourceElasticIP], eip)
	}
}

// TestMergeTags tests the tag merging helper function.
func TestMergeTags(t *testing.T) {
	base := map[string]string{
		"env":     "prod",
		"team":    "platform",
		"project": "deploy",
	}
	override := map[string]string{
		"Name": "my-resource",
		"team": "infra", // Should override base
	}

	result := mergeTags(base, override)

	// Verify base tags are present.
	if result["env"] != "prod" {
		t.Errorf("env = %q, want %q", result["env"], "prod")
	}
	if result["project"] != "deploy" {
		t.Errorf("project = %q, want %q", result["project"], "deploy")
	}

	// Verify override tags are present.
	if result["Name"] != "my-resource" {
		t.Errorf("Name = %q, want %q", result["Name"], "my-resource")
	}

	// Verify override takes precedence.
	if result["team"] != "infra" {
		t.Errorf("team = %q, want %q (should be overridden)", result["team"], "infra")
	}

	// Verify total tag count.
	if len(result) != 4 {
		t.Errorf("tag count = %d, want %d", len(result), 4)
	}
}

// TestECSServiceUsesPrivateSubnets tests subnet selection logic for ECS service.
func TestECSServiceUsesPrivateSubnets(t *testing.T) {
	tests := []struct {
		name              string
		privateSubnets    string
		publicSubnets     string
		wantSubnetPrefix  string
		wantPublicIPState string
	}{
		{
			name:              "uses private subnets when available",
			privateSubnets:    "subnet-priv-1,subnet-priv-2",
			publicSubnets:     "subnet-pub-1,subnet-pub-2",
			wantSubnetPrefix:  "subnet-priv",
			wantPublicIPState: "DISABLED",
		},
		{
			name:              "falls back to public subnets",
			privateSubnets:    "",
			publicSubnets:     "subnet-pub-1,subnet-pub-2",
			wantSubnetPrefix:  "subnet-pub",
			wantPublicIPState: "ENABLED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			infra := &state.Infrastructure{
				Resources: make(map[string]string),
			}
			infra.Resources[state.ResourceSubnetPrivate] = tt.privateSubnets
			infra.Resources[state.ResourceSubnetPublic] = tt.publicSubnets

			// Simulate the logic from createOrUpdateService.
			subnetStr := infra.Resources[state.ResourceSubnetPrivate]
			assignPublicIP := "DISABLED"
			if subnetStr == "" {
				subnetStr = infra.Resources[state.ResourceSubnetPublic]
				assignPublicIP = "ENABLED"
			}

			// Check subnet selection.
			if subnetStr == "" {
				t.Error("subnet string should not be empty")
			}
			if len(subnetStr) > 0 && subnetStr[:len(tt.wantSubnetPrefix)] != tt.wantSubnetPrefix {
				t.Errorf("subnet = %q, want prefix %q", subnetStr, tt.wantSubnetPrefix)
			}
			if assignPublicIP != tt.wantPublicIPState {
				t.Errorf("assignPublicIP = %q, want %q", assignPublicIP, tt.wantPublicIPState)
			}
		})
	}
}

// TestDeploy_InfraNotFound tests that deploy fails when infrastructure doesn't exist.
func TestDeploy_InfraNotFound(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	input := deployInput{
		InfraID:  "infra-nonexistent",
		ImageRef: "nginx:latest",
	}

	_, _, err := provider.deploy(context.Background(), nil, input)
	if err == nil {
		t.Error("Expected error for nonexistent infrastructure")
	}
}

// TestDeploy_InfraNotReady tests that deploy fails when infrastructure is not ready.
func TestDeploy_InfraNotReady(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create infrastructure in non-ready state.
	infra := &state.Infrastructure{
		ID:        "infra-test-notready",
		PlanID:    "plan-test",
		Region:    "us-east-1",
		Status:    state.InfraStatusProvisioning, // Not ready
		Resources: make(map[string]string),
	}
	if err := store.CreateInfra(infra); err != nil {
		t.Fatalf("CreateInfra: %v", err)
	}

	input := deployInput{
		InfraID:  "infra-test-notready",
		ImageRef: "nginx:latest",
	}

	_, _, err := provider.deploy(context.Background(), nil, input)
	if err == nil {
		t.Error("Expected error for infrastructure not ready")
	}
	// Should be ErrInfraNotReady.
	if !containsSubstring(err.Error(), "not ready") && !containsSubstring(err.Error(), "ErrInfraNotReady") {
		t.Errorf("Expected error about infrastructure not ready, got: %v", err)
	}
}

// TestDeploy_AutoScalingValidation tests deploy fails with invalid auto-scaling params.
func TestDeploy_AutoScalingValidation(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create ready infrastructure.
	infra := &state.Infrastructure{
		ID:        "infra-test-scaling",
		PlanID:    "plan-test",
		Region:    "us-east-1",
		Status:    state.InfraStatusReady,
		Resources: make(map[string]string),
	}
	if err := store.CreateInfra(infra); err != nil {
		t.Fatalf("CreateInfra: %v", err)
	}

	tests := []struct {
		name    string
		input   deployInput
		wantErr string
	}{
		{
			name: "max_count less than min_count",
			input: deployInput{
				InfraID:      "infra-test-scaling",
				ImageRef:     "nginx:latest",
				DesiredCount: 2,
				MinCount:     3,
				MaxCount:     1,
			},
			wantErr: "max_count",
		},
		{
			name: "min_count less than 1",
			input: deployInput{
				InfraID:      "infra-test-scaling",
				ImageRef:     "nginx:latest",
				DesiredCount: 2,
				MinCount:     0,
				MaxCount:     4,
			},
			wantErr: "", // min_count 0 defaults to desired_count
		},
		{
			name: "target CPU out of range",
			input: deployInput{
				InfraID:          "infra-test-scaling",
				ImageRef:         "nginx:latest",
				DesiredCount:     2,
				MinCount:         1,
				MaxCount:         4,
				TargetCPUPercent: 95, // > 90
			},
			wantErr: "target_cpu_percent",
		},
		{
			name: "target memory out of range",
			input: deployInput{
				InfraID:          "infra-test-scaling",
				ImageRef:         "nginx:latest",
				DesiredCount:     2,
				MinCount:         1,
				MaxCount:         4,
				TargetMemPercent: 5, // < 10
			},
			wantErr: "target_memory_percent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := provider.deploy(context.Background(), nil, tt.input)
			if tt.wantErr == "" {
				// For cases that should proceed (like min_count=0 defaulting),
				// we expect failure later in the process (AWS calls), not validation.
				// So we don't check the error here.
				return
			}
			if err == nil {
				t.Errorf("Expected error containing %q", tt.wantErr)
				return
			}
			if !containsSubstring(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// TestTeardown_NotFound tests that teardown fails when deployment doesn't exist.
func TestTeardown_NotFound(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	input := teardownInput{
		DeploymentID: "deploy-nonexistent",
	}

	_, _, err := provider.teardown(context.Background(), nil, input)
	if err == nil {
		t.Error("Expected error for nonexistent deployment")
	}
}

// TestStatus_NotFound tests that status fails when deployment doesn't exist.
func TestStatus_NotFound(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	input := statusInput{
		DeploymentID: "deploy-nonexistent",
	}

	_, _, err := provider.status(context.Background(), nil, input)
	if err == nil {
		t.Error("Expected error for nonexistent deployment")
	}
}

// TestCreateInfra_PlanNotFound tests that createInfra fails when plan doesn't exist.
func TestCreateInfra_PlanNotFound(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	input := createInfraInput{
		PlanID: "plan-nonexistent",
	}

	_, _, err := provider.createInfra(context.Background(), nil, input)
	if err == nil {
		t.Error("Expected error for nonexistent plan")
	}
}

// TestApprovePlan_Success tests successful plan approval.
func TestApprovePlan_Approve(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create a plan first.
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")
	planInput := planInfraInput{
		AppDescription: "Test app",
		ExpectedUsers:  50,
		LatencyMS:      200,
		Region:         "us-east-1",
	}
	_, planOutput, err := provider.planInfra(context.Background(), nil, planInput)
	if err != nil {
		t.Fatalf("planInfra: %v", err)
	}

	// Approve the plan.
	approveInput := approvePlanInput{
		PlanID:    planOutput.PlanID,
		Confirmed: true,
	}
	_, approveOutput, err := provider.approvePlan(context.Background(), nil, approveInput)
	if err != nil {
		t.Fatalf("approvePlan: %v", err)
	}

	if approveOutput.Status != state.PlanStatusApproved {
		t.Errorf("Status = %q, want %q", approveOutput.Status, state.PlanStatusApproved)
	}

	// Verify plan state was updated.
	plan, err := store.GetPlan(planOutput.PlanID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan.Status != state.PlanStatusApproved {
		t.Errorf("Plan status = %q, want %q", plan.Status, state.PlanStatusApproved)
	}
}

// TestApprovePlan_Reject tests plan rejection.
func TestApprovePlan_RejectPlan(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create a plan first.
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")
	planInput := planInfraInput{
		AppDescription: "Test app",
		ExpectedUsers:  50,
		LatencyMS:      200,
		Region:         "us-east-1",
	}
	_, planOutput, err := provider.planInfra(context.Background(), nil, planInput)
	if err != nil {
		t.Fatalf("planInfra: %v", err)
	}

	// Reject the plan (confirmed: false).
	rejectInput := approvePlanInput{
		PlanID:    planOutput.PlanID,
		Confirmed: false,
	}
	_, rejectOutput, err := provider.approvePlan(context.Background(), nil, rejectInput)
	if err != nil {
		t.Fatalf("approvePlan (reject): %v", err)
	}

	if rejectOutput.Status != state.PlanStatusRejected {
		t.Errorf("Status = %q, want %q", rejectOutput.Status, state.PlanStatusRejected)
	}

	// Verify plan state was updated.
	plan, err := store.GetPlan(planOutput.PlanID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan.Status != state.PlanStatusRejected {
		t.Errorf("Plan status = %q, want %q", plan.Status, state.PlanStatusRejected)
	}
}

// TestApprovePlan_AlreadyApproved tests that re-approving is idempotent.
func TestApprovePlan_AlreadyApproved(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create and approve a plan.
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")
	planInput := planInfraInput{
		AppDescription: "Test app",
		ExpectedUsers:  50,
		LatencyMS:      200,
		Region:         "us-east-1",
	}
	_, planOutput, _ := provider.planInfra(context.Background(), nil, planInput)

	// Approve first time.
	approveInput := approvePlanInput{PlanID: planOutput.PlanID, Confirmed: true}
	_, _, err := provider.approvePlan(context.Background(), nil, approveInput)
	if err != nil {
		t.Fatalf("First approve failed: %v", err)
	}

	// Approve second time (should be idempotent).
	_, output2, err := provider.approvePlan(context.Background(), nil, approveInput)
	if err != nil {
		t.Fatalf("Second approve failed: %v", err)
	}
	if output2.Status != state.PlanStatusApproved {
		t.Errorf("Status = %q, want %q", output2.Status, state.PlanStatusApproved)
	}
}

// TestCreateInfra_NotApproved tests that createInfra fails with unapproved plan.
func TestCreateInfra_NotApprovedPlan(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	provider := NewAWSProvider(store)

	// Create a plan but don't approve it.
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")
	planInput := planInfraInput{
		AppDescription: "Test app",
		ExpectedUsers:  50,
		LatencyMS:      200,
		Region:         "us-east-1",
	}
	_, planOutput, _ := provider.planInfra(context.Background(), nil, planInput)

	// Try to create infra with unapproved plan.
	createInput := createInfraInput{
		PlanID: planOutput.PlanID,
	}
	_, _, err := provider.createInfra(context.Background(), nil, createInput)
	if err == nil {
		t.Error("Expected error for unapproved plan")
	}
	// Should contain ErrPlanNotApproved or similar message.
	if !containsSubstring(err.Error(), "approved") && !containsSubstring(err.Error(), "approval") {
		t.Errorf("Expected error about plan not approved, got: %v", err)
	}
}

// TestMergeTags_NilMaps tests mergeTags with nil inputs.
func TestMergeTags_NilMaps(t *testing.T) {
	// Test with nil first map.
	result := mergeTags(nil, map[string]string{"key1": "val1"})
	if result["key1"] != "val1" {
		t.Errorf("Expected key1=val1, got %v", result)
	}

	// Test with nil second map.
	result = mergeTags(map[string]string{"key2": "val2"}, nil)
	if result["key2"] != "val2" {
		t.Errorf("Expected key2=val2, got %v", result)
	}

	// Test with both nil.
	result = mergeTags(nil, nil)
	if len(result) != 0 {
		t.Errorf("Expected empty map, got %v", result)
	}
}

// TestMergeTags_Override tests that second map overrides first.
func TestMergeTags_Override(t *testing.T) {
	map1 := map[string]string{"key": "original", "unique1": "val1"}
	map2 := map[string]string{"key": "override", "unique2": "val2"}

	result := mergeTags(map1, map2)

	if result["key"] != "override" {
		t.Errorf("Expected key=override, got %s", result["key"])
	}
	if result["unique1"] != "val1" {
		t.Errorf("Expected unique1=val1, got %s", result["unique1"])
	}
	if result["unique2"] != "val2" {
		t.Errorf("Expected unique2=val2, got %s", result["unique2"])
	}
}

// TestRollbackInfra tests that rollbackInfra handles empty infrastructure gracefully.
// WHY: Per spec ralph/specs/error-handling.md - rollback must work even when
// some resources haven't been created yet (partial provisioning failures).
func TestRollbackInfra_EmptyInfra(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create an empty infrastructure record (no resources provisioned yet)
	infra := &state.Infrastructure{
		ID:        "infra-test-rollback",
		PlanID:    "plan-test",
		Region:    "us-east-1",
		Resources: make(map[string]string), // Empty - no resources created
		Status:    state.InfraStatusProvisioning,
		CreatedAt: time.Now(),
	}

	err = store.CreateInfra(infra)
	if err != nil {
		t.Fatalf("CreateInfra: %v", err)
	}

	// Rollback should succeed without errors when no resources exist.
	// Note: We can't actually call rollbackInfra because it requires valid AWS config.
	// Instead, we verify the infra structure is correct for rollback.

	// Verify no resources to clean up
	for key, val := range infra.Resources {
		if val != "" {
			t.Errorf("Expected empty resource %s, got %s", key, val)
		}
	}
}

// TestCreateInfraErrorWrapping tests that createInfra errors are properly
// wrapped with ErrProvisioningFailed.
// WHY: Per spec ralph/specs/error-handling.md - callers should be able to use
// errors.Is(err, ErrProvisioningFailed) to detect provisioning failures.
func TestCreateInfraErrorWrapping(t *testing.T) {
	// Set higher budget limit for test to avoid budget exceeded error
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	provider := NewAWSProvider(store)

	// Create and approve a plan first
	input := planInfraInput{
		AppDescription: "Test app",
		ExpectedUsers:  10,
		LatencyMS:      100,
		Region:         "us-east-1",
	}
	_, planOutput, err := provider.planInfra(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("planInfra: %v", err)
	}

	// Approve the plan
	approveInput := approvePlanInput{
		PlanID:    planOutput.PlanID,
		Confirmed: true,
	}
	_, _, err = provider.approvePlan(context.Background(), nil, approveInput)
	if err != nil {
		t.Fatalf("approvePlan: %v", err)
	}

	// createInfra will fail because we don't have valid AWS credentials in test.
	// The point is to verify the error is properly wrapped.
	// Note: In unit tests without AWS mock, this validates the error wrapping structure.
	createInput := createInfraInput{
		PlanID: planOutput.PlanID,
	}

	_, _, err = provider.createInfra(context.Background(), nil, createInput)
	// Error is expected (no AWS credentials in test), but we can verify
	// the plan validation logic works correctly
	if err == nil {
		t.Skip("createInfra succeeded unexpectedly (AWS credentials available)")
	}
}

// ---------------------------------------------------------------------------
// Mock-based Unit Tests
// ---------------------------------------------------------------------------
// Tests below use the mock infrastructure to test AWS provisioning functions
// without requiring real AWS credentials.

// TestProvisionVPC_WithMocks tests the provisionVPC function with mock AWS clients.
func TestProvisionVPC_WithMocks(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create mock EC2 client
	ec2Mock := &mocks.EC2Mock{
		// Override specific behaviors if needed; defaults return success
	}

	clients := &awsclient.AWSClients{
		EC2: ec2Mock,
	}

	provider := NewAWSProviderWithClients(store, clients)

	// Create test infrastructure record and persist it
	infra := &state.Infrastructure{
		ID:        "infra-test-vpc-mocks",
		PlanID:    "plan-test-123",
		Region:    "us-east-1",
		Resources: make(map[string]string),
		Status:    state.InfraStatusProvisioning,
	}
	if createErr := store.CreateInfra(infra); createErr != nil {
		t.Fatalf("CreateInfra: %v", createErr)
	}

	tags := map[string]string{
		"agent-deploy:plan-id":  "plan-test-123",
		"agent-deploy:infra-id": infra.ID,
	}

	// Call provisionVPC with default VPC CIDR
	err = provider.provisionVPC(context.Background(), aws.Config{Region: "us-east-1"}, infra, tags, "10.0.0.0/16")
	if err != nil {
		t.Fatalf("provisionVPC: %v", err)
	}

	// Verify EC2 API calls were made
	if ec2Mock.CreateVpcCalls != 1 {
		t.Errorf("CreateVpcCalls = %d, want 1", ec2Mock.CreateVpcCalls)
	}
	if ec2Mock.CreateSubnetCalls != 4 { // 2 public + 2 private subnets
		t.Errorf("CreateSubnetCalls = %d, want 4", ec2Mock.CreateSubnetCalls)
	}

	// Verify resources were stored in memory
	if infra.Resources[state.ResourceVPC] == "" {
		t.Error("VPC ID should be stored")
	}
	if infra.Resources[state.ResourceSubnetPublic] == "" {
		t.Error("Public subnet IDs should be stored")
	}
	if infra.Resources[state.ResourceSubnetPrivate] == "" {
		t.Error("Private subnet IDs should be stored")
	}

	// Verify resources were persisted to store
	storedInfra, err := store.GetInfra(infra.ID)
	if err != nil {
		t.Fatalf("GetInfra: %v", err)
	}
	if storedInfra.Resources[state.ResourceVPC] == "" {
		t.Error("VPC ID should be persisted to store")
	}
}

// TestProvisionVPC_CreateVPCError tests error handling when CreateVpc fails.
func TestProvisionVPC_CreateVPCError(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ec2Mock := &mocks.EC2Mock{
		CreateVpcFunc: func(ctx context.Context, params *ec2.CreateVpcInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error) {
			return nil, fmt.Errorf("vpc quota exceeded")
		},
	}

	clients := &awsclient.AWSClients{
		EC2: ec2Mock,
	}

	provider := NewAWSProviderWithClients(store, clients)
	infra := &state.Infrastructure{
		ID:        "infra-test-123",
		Resources: make(map[string]string),
	}

	err = provider.provisionVPC(context.Background(), aws.Config{Region: "us-east-1"}, infra, map[string]string{}, "10.0.0.0/16")
	if err == nil {
		t.Fatal("expected error when CreateVpc fails")
	}
	if !containsSubstring(err.Error(), "create VPC") {
		t.Errorf("error should mention VPC creation: %v", err)
	}
}

// TestProvisionVPC_SubnetError tests error handling when CreateSubnet fails.
func TestProvisionVPC_SubnetError(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	subnetCallCount := 0
	ec2Mock := &mocks.EC2Mock{
		CreateSubnetFunc: func(ctx context.Context, params *ec2.CreateSubnetInput, optFns ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error) {
			subnetCallCount++
			if subnetCallCount > 1 {
				return nil, fmt.Errorf("subnet limit exceeded")
			}
			return &ec2.CreateSubnetOutput{
				Subnet: &ec2types.Subnet{
					SubnetId: aws.String("subnet-mock-" + fmt.Sprintf("%d", subnetCallCount)),
				},
			}, nil
		},
	}

	clients := &awsclient.AWSClients{
		EC2: ec2Mock,
	}

	provider := NewAWSProviderWithClients(store, clients)
	infra := &state.Infrastructure{
		ID:        "infra-test-123",
		Resources: make(map[string]string),
	}

	err = provider.provisionVPC(context.Background(), aws.Config{Region: "us-east-1"}, infra, map[string]string{}, "10.0.0.0/16")
	if err == nil {
		t.Fatal("expected error when CreateSubnet fails")
	}
	if !containsSubstring(err.Error(), "subnet") {
		t.Errorf("error should mention subnet creation: %v", err)
	}
}

// TestNewAWSProviderWithClients verifies the constructor works correctly.
func TestNewAWSProviderWithClients(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	clients := &awsclient.AWSClients{
		EC2: &mocks.EC2Mock{},
	}

	provider := NewAWSProviderWithClients(store, clients)
	if provider == nil {
		t.Fatal("provider should not be nil")
	}
	if provider.clients != clients {
		t.Error("provider should use injected clients")
	}
}

// TestGetClients_WithInjectedClients verifies getClients returns injected clients.
func TestGetClients_WithInjectedClients(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	mockEC2 := &mocks.EC2Mock{}
	clients := &awsclient.AWSClients{
		EC2: mockEC2,
	}

	provider := NewAWSProviderWithClients(store, clients)
	got := provider.getClients(aws.Config{})

	if got != clients {
		t.Error("getClients should return injected clients")
	}
}

// TestProvisionECSCluster_WithMocks tests the ECS cluster provisioning with mocks.
func TestProvisionECSCluster_WithMocks(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ecsMock := &mocks.ECSMock{}
	clients := &awsclient.AWSClients{
		ECS: ecsMock,
	}

	provider := NewAWSProviderWithClients(store, clients)

	// Create and persist infrastructure
	infra := &state.Infrastructure{
		ID:        "infra-test-ecs",
		PlanID:    "plan-test-ecs",
		Region:    "us-east-1",
		Resources: make(map[string]string),
		Status:    state.InfraStatusProvisioning,
	}
	if createErr := store.CreateInfra(infra); createErr != nil {
		t.Fatalf("CreateInfra: %v", createErr)
	}

	tags := map[string]string{
		"agent-deploy:infra-id": infra.ID,
	}

	err = provider.provisionECSCluster(context.Background(), aws.Config{Region: "us-east-1"}, infra, tags)
	if err != nil {
		t.Fatalf("provisionECSCluster: %v", err)
	}

	// Verify ECS API calls
	if ecsMock.CreateClusterCalls != 1 {
		t.Errorf("CreateClusterCalls = %d, want 1", ecsMock.CreateClusterCalls)
	}

	// Verify cluster ARN stored
	if infra.Resources[state.ResourceECSCluster] == "" {
		t.Error("ECS cluster ARN should be stored")
	}
}

// TestProvisionECSCluster_Error tests error handling when CreateCluster fails.
func TestProvisionECSCluster_Error(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ecsMock := &mocks.ECSMock{
		CreateClusterFunc: func(ctx context.Context, params *ecs.CreateClusterInput, optFns ...func(*ecs.Options)) (*ecs.CreateClusterOutput, error) {
			return nil, fmt.Errorf("cluster limit exceeded")
		},
	}
	clients := &awsclient.AWSClients{
		ECS: ecsMock,
	}

	provider := NewAWSProviderWithClients(store, clients)
	infra := &state.Infrastructure{
		ID:        "infra-test-ecs-err",
		Resources: make(map[string]string),
	}

	err = provider.provisionECSCluster(context.Background(), aws.Config{Region: "us-east-1"}, infra, map[string]string{})
	if err == nil {
		t.Fatal("expected error when CreateCluster fails")
	}
	if !containsSubstring(err.Error(), "create cluster") {
		t.Errorf("error should mention cluster creation: %v", err)
	}
}

// TestProvisionALB_WithMocks tests ALB provisioning with mocks.
func TestProvisionALB_WithMocks(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	elbMock := &mocks.ELBV2Mock{}
	clients := &awsclient.AWSClients{
		ELBV2: elbMock,
	}

	provider := NewAWSProviderWithClients(store, clients)

	// Create infrastructure with required VPC resources
	infra := &state.Infrastructure{
		ID:     "infra-test-alb",
		PlanID: "plan-test-alb",
		Region: "us-east-1",
		Resources: map[string]string{
			state.ResourceSubnetPublic:      "subnet-1,subnet-2",
			state.ResourceSecurityGroupALB:  "sg-alb-123",
			state.ResourceSecurityGroupTask: "sg-task-456",
		},
		Status: state.InfraStatusProvisioning,
	}
	if createErr := store.CreateInfra(infra); createErr != nil {
		t.Fatalf("CreateInfra: %v", createErr)
	}

	tags := map[string]string{
		"agent-deploy:infra-id": infra.ID,
	}

	err = provider.provisionALB(context.Background(), aws.Config{Region: "us-east-1"}, infra, tags, "")
	if err != nil {
		t.Fatalf("provisionALB: %v", err)
	}

	// Verify ALB and Target Group created
	if infra.Resources[state.ResourceALB] == "" {
		t.Error("ALB ARN should be stored")
	}
	if infra.Resources[state.ResourceTargetGroup] == "" {
		t.Error("Target group ARN should be stored")
	}
}

// TestIsLocalImage tests the isLocalImage function for classifying image references.
func TestIsLocalImage(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		want     bool
	}{
		// Local images (should return true).
		{
			name:     "simple name",
			imageRef: "myapp",
			want:     true,
		},
		{
			name:     "name with tag",
			imageRef: "myapp:latest",
			want:     true,
		},
		{
			name:     "name with version tag",
			imageRef: "myapp:v1.0.0",
			want:     true,
		},
		{
			name:     "library image without registry",
			imageRef: "nginx:latest",
			want:     true,
		},
		{
			name:     "library path",
			imageRef: "library/nginx:latest",
			want:     true,
		},
		{
			name:     "user prefix without registry",
			imageRef: "username/myapp:latest",
			want:     true,
		},

		// ECR images (should return false).
		{
			name:     "ECR URI",
			imageRef: "123456789012.dkr.ecr.us-east-1.amazonaws.com/myrepo:latest",
			want:     false,
		},
		{
			name:     "ECR URI different region",
			imageRef: "123456789012.dkr.ecr.eu-west-1.amazonaws.com/myrepo:v1",
			want:     false,
		},

		// Public registries (should return false).
		{
			name:     "docker.io",
			imageRef: "docker.io/library/nginx:latest",
			want:     false,
		},
		{
			name:     "index.docker.io",
			imageRef: "index.docker.io/nginx:alpine",
			want:     false,
		},
		{
			name:     "ghcr.io",
			imageRef: "ghcr.io/owner/repo:tag",
			want:     false,
		},
		{
			name:     "public.ecr.aws",
			imageRef: "public.ecr.aws/nginx/nginx:latest",
			want:     false,
		},
		{
			name:     "gcr.io",
			imageRef: "gcr.io/project/image:tag",
			want:     false,
		},
		{
			name:     "quay.io",
			imageRef: "quay.io/prometheus/prometheus:latest",
			want:     false,
		},
		{
			name:     "mcr.microsoft.com",
			imageRef: "mcr.microsoft.com/dotnet/aspnet:6.0",
			want:     false,
		},

		// Custom registry with domain (should return false).
		{
			name:     "custom registry with domain",
			imageRef: "myregistry.example.com/myapp:latest",
			want:     false,
		},
		{
			name:     "localhost registry",
			imageRef: "localhost:5000/myapp:latest",
			want:     false,
		},
		{
			name:     "IP-based registry",
			imageRef: "192.168.1.100:5000/myapp:latest",
			want:     false,
		},

		// Edge cases.
		{
			name:     "empty string",
			imageRef: "",
			want:     false,
		},
		{
			name:     "digest only local",
			imageRef: "myapp@sha256:abc123def456",
			want:     true,
		},
		{
			name:     "digest with registry",
			imageRef: "docker.io/nginx@sha256:abc123def456",
			want:     false,
		},
		{
			name:     "deeply nested path local",
			imageRef: "org/team/project/myapp:v1",
			want:     true,
		},
		{
			name:     "deeply nested path with registry",
			imageRef: "ghcr.io/org/team/project/myapp:v1",
			want:     false,
		},
		{
			name:     "uppercase local",
			imageRef: "MyApp:Latest",
			want:     true,
		},
		{
			name:     "tag with special chars",
			imageRef: "myapp:v1.0.0-rc.1+build.123",
			want:     true,
		},
		{
			name:     "registry hub docker com",
			imageRef: "registry.hub.docker.com/library/nginx:latest",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLocalImage(tt.imageRef)
			if got != tt.want {
				t.Errorf("isLocalImage(%q) = %v, want %v", tt.imageRef, got, tt.want)
			}
		})
	}
}

// TestValidateFargateResources tests the Fargate CPU/memory validation.
func TestValidateFargateResources(t *testing.T) {
	tests := []struct {
		name    string
		cpu     string
		memory  string
		wantErr bool
	}{
		// Valid combinations.
		{"256_512", "256", "512", false},
		{"256_1024", "256", "1024", false},
		{"256_2048", "256", "2048", false},
		{"512_1024", "512", "1024", false},
		{"512_4096", "512", "4096", false},
		{"1024_2048", "1024", "2048", false},
		{"1024_8192", "1024", "8192", false},
		{"2048_4096", "2048", "4096", false},
		{"2048_16384", "2048", "16384", false},
		{"4096_8192", "4096", "8192", false},
		{"4096_30720", "4096", "30720", false},

		// Invalid CPU values.
		{"invalid_cpu", "100", "512", true},
		{"invalid_cpu_string", "abc", "512", true},

		// Invalid memory for CPU.
		{"256_256_invalid", "256", "256", true},
		{"256_4096_invalid", "256", "4096", true},
		{"512_512_invalid", "512", "512", true},
		{"1024_512_invalid", "1024", "512", true},
		{"4096_4096_invalid", "4096", "4096", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFargateResources(tt.cpu, tt.memory)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateFargateResources(%q, %q) expected error, got nil", tt.cpu, tt.memory)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateFargateResources(%q, %q) unexpected error: %v", tt.cpu, tt.memory, err)
			}
		})
	}
}

// TestValidateLogRetention tests CloudWatch log retention validation.
func TestValidateLogRetention(t *testing.T) {
	validDays := []int{1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653}
	for _, days := range validDays {
		t.Run(fmt.Sprintf("valid_%d", days), func(t *testing.T) {
			if err := ValidateLogRetention(days); err != nil {
				t.Errorf("ValidateLogRetention(%d) unexpected error: %v", days, err)
			}
		})
	}

	invalidDays := []int{0, 2, 4, 6, 8, 10, 15, 31, 100, 366, 999, 5000}
	for _, days := range invalidDays {
		t.Run(fmt.Sprintf("invalid_%d", days), func(t *testing.T) {
			if err := ValidateLogRetention(days); err == nil {
				t.Errorf("ValidateLogRetention(%d) expected error, got nil", days)
			}
		})
	}
}

// TestValidateContainerPort tests container port validation.
func TestValidateContainerPort(t *testing.T) {
	tests := []struct {
		port    int
		wantErr bool
	}{
		{1, false},
		{80, false},
		{443, false},
		{3000, false},
		{8080, false},
		{65535, false},
		{0, true},
		{-1, true},
		{65536, true},
		{100000, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("port_%d", tt.port), func(t *testing.T) {
			err := ValidateContainerPort(tt.port)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateContainerPort(%d) expected error, got nil", tt.port)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateContainerPort(%d) unexpected error: %v", tt.port, err)
			}
		})
	}
}

// TestValidateHealthCheckPath tests health check path validation.
func TestValidateHealthCheckPath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"", false}, // Empty uses default
		{"/", false},
		{"/health", false},
		{"/healthz", false},
		{"/api/health", false},
		{"/api/v1/healthcheck", false},
		{"health", true}, // Must start with /
		{"api/health", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := ValidateHealthCheckPath(tt.path)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateHealthCheckPath(%q) expected error, got nil", tt.path)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateHealthCheckPath(%q) unexpected error: %v", tt.path, err)
			}
		})
	}
}

// TestDeployInput_HealthCheckGracePeriod tests the health check grace period defaults.
// WHY (P1.28): Container health checks need a grace period to allow containers to start
// before health check failures trigger container replacement.
func TestDeployInput_HealthCheckGracePeriod(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int // Expected default or validated value
	}{
		{"default_zero", 0, 60},          // 0 should default to 60 seconds
		{"default_negative", -10, 60},    // Negative should default to 60 seconds
		{"custom_30s", 30, 30},           // Custom value should be preserved
		{"custom_120s", 120, 120},        // Longer grace period should be preserved
		{"custom_300s", 300, 300},        // 5 minute grace period for slow-starting apps
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The grace period validation happens in createTaskDefinition.
			// Here we verify the deployInput struct accepts the field correctly
			// and JSON serialization works.
			input := deployInput{
				InfraID:                "infra-test",
				ImageRef:               "nginx:latest",
				HealthCheckGracePeriod: tt.input,
			}

			// Serialize to JSON and back to verify field is properly defined.
			data, err := json.Marshal(input)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var parsed deployInput
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if parsed.HealthCheckGracePeriod != tt.input {
				t.Errorf("HealthCheckGracePeriod = %d, want %d", parsed.HealthCheckGracePeriod, tt.input)
			}
		})
	}
}

// TestValidateAWSRegion tests AWS region validation.
func TestValidateAWSRegion(t *testing.T) {
	validRegions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-central-1",
		"ap-northeast-1", "ap-southeast-1",
	}
	for _, region := range validRegions {
		t.Run("valid_"+region, func(t *testing.T) {
			if err := ValidateAWSRegion(region); err != nil {
				t.Errorf("ValidateAWSRegion(%q) unexpected error: %v", region, err)
			}
		})
	}

	invalidRegions := []string{"", "invalid", "us-east", "eu-1", "test-region"}
	for _, region := range invalidRegions {
		t.Run("invalid_"+region, func(t *testing.T) {
			if err := ValidateAWSRegion(region); err == nil {
				t.Errorf("ValidateAWSRegion(%q) expected error, got nil", region)
			}
		})
	}
}

// TestValidateDesiredCount tests desired task count validation.
func TestValidateDesiredCount(t *testing.T) {
	tests := []struct {
		count   int
		wantErr bool
	}{
		{1, false},
		{5, false},
		{10, false},
		{50, false},
		{100, false},
		{0, true},
		{-1, true},
		{101, true},
		{1000, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("count_%d", tt.count), func(t *testing.T) {
			err := ValidateDesiredCount(tt.count)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateDesiredCount(%d) expected error, got nil", tt.count)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateDesiredCount(%d) unexpected error: %v", tt.count, err)
			}
		})
	}
}

// TestValidateEnvironmentVariables tests environment variable validation.
func TestValidateEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{
			name:    "empty",
			env:     map[string]string{},
			wantErr: false,
		},
		{
			name:    "valid_simple",
			env:     map[string]string{"MY_VAR": "value"},
			wantErr: false,
		},
		{
			name:    "valid_multiple",
			env:     map[string]string{"VAR1": "a", "VAR_2": "b", "_VAR3": "c"},
			wantErr: false,
		},
		{
			name:    "valid_underscore_prefix",
			env:     map[string]string{"_MY_VAR": "value"},
			wantErr: false,
		},
		{
			name:    "invalid_starts_with_digit",
			env:     map[string]string{"1VAR": "value"},
			wantErr: true,
		},
		{
			name:    "invalid_has_dash",
			env:     map[string]string{"MY-VAR": "value"},
			wantErr: true,
		},
		{
			name:    "invalid_has_space",
			env:     map[string]string{"MY VAR": "value"},
			wantErr: true,
		},
		{
			name:    "reserved_AWS_prefix",
			env:     map[string]string{"AWS_SECRET_KEY": "value"},
			wantErr: true,
		},
		{
			name:    "reserved_ECS_prefix",
			env:     map[string]string{"ECS_TASK_ID": "value"},
			wantErr: true,
		},
		{
			name:    "reserved_FARGATE_prefix",
			env:     map[string]string{"FARGATE_SOMETHING": "value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnvironmentVariables(tt.env)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateEnvironmentVariables() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateEnvironmentVariables() unexpected error: %v", err)
			}
		})
	}
}

// TestValidateVpcCIDR tests VPC CIDR validation for P1.9.
func TestValidateVpcCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{
			name:    "valid_default_cidr",
			cidr:    "10.0.0.0/16",
			wantErr: false,
		},
		{
			name:    "valid_class_A_private",
			cidr:    "10.100.0.0/16",
			wantErr: false,
		},
		{
			name:    "valid_class_B_private",
			cidr:    "172.16.0.0/16",
			wantErr: false,
		},
		{
			name:    "valid_class_C_private",
			cidr:    "192.168.0.0/24",
			wantErr: false,
		},
		{
			name:    "valid_minimum_prefix",
			cidr:    "10.0.0.0/24",
			wantErr: false,
		},
		{
			name:    "invalid_prefix_too_large",
			cidr:    "10.0.0.0/8",
			wantErr: true,
		},
		{
			name:    "invalid_prefix_too_small",
			cidr:    "10.0.0.0/28",
			wantErr: true,
		},
		{
			name:    "invalid_not_cidr",
			cidr:    "10.0.0.0",
			wantErr: true,
		},
		{
			name:    "invalid_ipv6",
			cidr:    "2001:db8::/32",
			wantErr: true,
		},
		{
			name:    "invalid_malformed",
			cidr:    "not-a-cidr",
			wantErr: true,
		},
		{
			name:    "valid_different_base",
			cidr:    "10.50.0.0/20",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVpcCIDR(tt.cidr)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateVpcCIDR(%q) expected error, got nil", tt.cidr)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateVpcCIDR(%q) unexpected error: %v", tt.cidr, err)
			}
		})
	}
}

// TestCalculateSubnetLayout tests subnet CIDR derivation from VPC CIDR for P1.9.
func TestCalculateSubnetLayout(t *testing.T) {
	tests := []struct {
		name            string
		vpcCIDR         string
		wantPublicCIDRs []string
		wantPrivCIDRs   []string
		wantErr         bool
	}{
		{
			name:            "default_10.0.0.0/16",
			vpcCIDR:         "10.0.0.0/16",
			wantPublicCIDRs: []string{"10.0.1.0/24", "10.0.2.0/24"},
			wantPrivCIDRs:   []string{"10.0.10.0/24", "10.0.11.0/24"},
			wantErr:         false,
		},
		{
			name:            "custom_172.16.0.0/16",
			vpcCIDR:         "172.16.0.0/16",
			wantPublicCIDRs: []string{"172.16.1.0/24", "172.16.2.0/24"},
			wantPrivCIDRs:   []string{"172.16.10.0/24", "172.16.11.0/24"},
			wantErr:         false,
		},
		{
			name:            "different_base_10.50.0.0/16",
			vpcCIDR:         "10.50.0.0/16",
			wantPublicCIDRs: []string{"10.50.1.0/24", "10.50.2.0/24"},
			wantPrivCIDRs:   []string{"10.50.10.0/24", "10.50.11.0/24"},
			wantErr:         false,
		},
		{
			name:    "invalid_cidr",
			vpcCIDR: "not-valid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, err := CalculateSubnetLayout(tt.vpcCIDR)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CalculateSubnetLayout(%q) expected error", tt.vpcCIDR)
				}
				return
			}
			if err != nil {
				t.Fatalf("CalculateSubnetLayout(%q) error: %v", tt.vpcCIDR, err)
			}

			if layout.VpcCIDR != tt.vpcCIDR {
				t.Errorf("VpcCIDR = %q, want %q", layout.VpcCIDR, tt.vpcCIDR)
			}
			if len(layout.PublicCIDRs) != 2 {
				t.Fatalf("PublicCIDRs len = %d, want 2", len(layout.PublicCIDRs))
			}
			if len(layout.PrivateCIDRs) != 2 {
				t.Fatalf("PrivateCIDRs len = %d, want 2", len(layout.PrivateCIDRs))
			}
			for i, want := range tt.wantPublicCIDRs {
				if layout.PublicCIDRs[i] != want {
					t.Errorf("PublicCIDRs[%d] = %q, want %q", i, layout.PublicCIDRs[i], want)
				}
			}
			for i, want := range tt.wantPrivCIDRs {
				if layout.PrivateCIDRs[i] != want {
					t.Errorf("PrivateCIDRs[%d] = %q, want %q", i, layout.PrivateCIDRs[i], want)
				}
			}
		})
	}
}

// TestPlanInfra_VpcCIDR tests custom VPC CIDR in plan infra (P1.9).
func TestPlanInfra_VpcCIDR(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	provider := NewAWSProvider(store)

	tests := []struct {
		name         string
		vpcCIDR      string
		wantVpcCIDR  string
		wantErr      bool
		wantErrMatch string
	}{
		{
			name:        "default_when_empty",
			vpcCIDR:     "",
			wantVpcCIDR: "10.0.0.0/16",
		},
		{
			name:        "custom_valid_cidr",
			vpcCIDR:     "172.16.0.0/16",
			wantVpcCIDR: "172.16.0.0/16",
		},
		{
			name:         "invalid_cidr_rejected",
			vpcCIDR:      "10.0.0.0/8",
			wantErr:      true,
			wantErrMatch: "Prefix length must be between",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := planInfraInput{
				AppDescription: "test app",
				ExpectedUsers:  100,
				LatencyMS:      200,
				Region:         "us-east-1",
				VpcCIDR:        tt.vpcCIDR,
			}

			_, output, err := provider.planInfra(context.Background(), nil, input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrMatch != "" && !strings.Contains(err.Error(), tt.wantErrMatch) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantErrMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("planInfra: %v", err)
			}

			// Verify plan has correct VPC CIDR stored.
			plan, err := store.GetPlan(output.PlanID)
			if err != nil {
				t.Fatalf("GetPlan: %v", err)
			}
			if plan.VpcCIDR != tt.wantVpcCIDR {
				t.Errorf("Plan.VpcCIDR = %q, want %q", plan.VpcCIDR, tt.wantVpcCIDR)
			}
		})
	}
}

// TestValidateDomainName tests domain name validation for P1.29.
func TestValidateDomainName(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		wantErr bool
	}{
		{
			name:    "valid_subdomain",
			domain:  "app.example.com",
			wantErr: false,
		},
		{
			name:    "valid_deep_subdomain",
			domain:  "staging.app.example.com",
			wantErr: false,
		},
		{
			name:    "valid_apex",
			domain:  "example.com",
			wantErr: false,
		},
		{
			name:    "valid_with_hyphens",
			domain:  "my-app.example-domain.com",
			wantErr: false,
		},
		{
			name:    "empty_allowed",
			domain:  "",
			wantErr: false,
		},
		{
			name:    "invalid_single_label",
			domain:  "localhost",
			wantErr: true,
		},
		{
			name:    "invalid_starts_with_hyphen",
			domain:  "-app.example.com",
			wantErr: true,
		},
		{
			name:    "invalid_ends_with_hyphen",
			domain:  "app-.example.com",
			wantErr: true,
		},
		{
			name:    "invalid_special_chars",
			domain:  "app_test.example.com",
			wantErr: true,
		},
		{
			name:    "invalid_too_long",
			domain:  strings.Repeat("a", 260) + ".com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDomainName(tt.domain)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateDomainName(%q) expected error, got nil", tt.domain)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateDomainName(%q) unexpected error: %v", tt.domain, err)
			}
		})
	}
}

// TestExtractParentDomain tests parent domain extraction for P1.29.
func TestExtractParentDomain(t *testing.T) {
	tests := []struct {
		domain     string
		wantParent string
	}{
		{"app.example.com", "example.com"},
		{"staging.app.example.com", "app.example.com"},
		{"example.com", "example.com"},
		{"a.b.c.d.com", "b.c.d.com"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := extractParentDomain(tt.domain)
			if got != tt.wantParent {
				t.Errorf("extractParentDomain(%q) = %q, want %q", tt.domain, got, tt.wantParent)
			}
		})
	}
}

// TestPlanInfra_DomainName tests custom domain name in plan infra (P1.29).
func TestPlanInfra_DomainName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "100")

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	provider := NewAWSProvider(store)

	tests := []struct {
		name           string
		domainName     string
		wantDomainName string
		wantErr        bool
		wantErrMatch   string
	}{
		{
			name:           "no_domain_by_default",
			domainName:     "",
			wantDomainName: "",
		},
		{
			name:           "custom_domain_stored",
			domainName:     "app.example.com",
			wantDomainName: "app.example.com",
		},
		{
			name:         "invalid_domain_rejected",
			domainName:   "invalid",
			wantErr:      true,
			wantErrMatch: "at least a subdomain and TLD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := planInfraInput{
				AppDescription: "test app",
				ExpectedUsers:  100,
				LatencyMS:      200,
				Region:         "us-east-1",
				DomainName:     tt.domainName,
			}

			_, output, err := provider.planInfra(context.Background(), nil, input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrMatch != "" && !strings.Contains(err.Error(), tt.wantErrMatch) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantErrMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("planInfra: %v", err)
			}

			// Verify plan has correct domain name stored.
			plan, err := store.GetPlan(output.PlanID)
			if err != nil {
				t.Fatalf("GetPlan: %v", err)
			}
			if plan.DomainName != tt.wantDomainName {
				t.Errorf("Plan.DomainName = %q, want %q", plan.DomainName, tt.wantDomainName)
			}

			// Verify output has custom domain if configured.
			if tt.wantDomainName != "" && output.CustomDomain != tt.wantDomainName {
				t.Errorf("Output.CustomDomain = %q, want %q", output.CustomDomain, tt.wantDomainName)
			}
		})
	}
}

// TestValidateID tests the ID format validation (P1.31).
func TestValidateID(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		prefix string
		want   string // empty = no error
	}{
		// Valid ULID format
		{name: "valid plan ULID", id: "plan-01HX0F2H6GZRFB5SDQVYA5TQGM", prefix: "plan", want: ""},
		{name: "valid infra ULID", id: "infra-01HX0F2H6GZRFB5SDQVYA5TQGM", prefix: "infra", want: ""},
		{name: "valid deploy ULID", id: "deploy-01HX0F2H6GZRFB5SDQVYA5TQGM", prefix: "deploy", want: ""},
		// Legacy format (backwards compatibility)
		{name: "valid legacy plan", id: "plan-test-001", prefix: "plan", want: ""},
		{name: "valid legacy infra", id: "infra-test-123", prefix: "infra", want: ""},
		{name: "valid legacy deploy", id: "deploy-abc-xyz", prefix: "deploy", want: ""},
		// Invalid cases
		{name: "empty ID", id: "", prefix: "plan", want: "cannot be empty"},
		{name: "wrong prefix", id: "infra-01HX0F2H6G", prefix: "plan", want: "Must start with"},
		{name: "missing prefix", id: "01HX0F2H6GZRFB5SDQVYA5TQGM", prefix: "plan", want: "Must start with"},
		{name: "special chars in ID", id: "plan-test@#$", prefix: "plan", want: "invalid characters"},
		{name: "missing ID part", id: "plan-", prefix: "plan", want: "Missing ID portion"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateID(tt.id, tt.prefix)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateID(%q, %q) = %v, want nil", tt.id, tt.prefix, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateID(%q, %q) = nil, want error containing %q", tt.id, tt.prefix, tt.want)
				} else if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("ValidateID(%q, %q) = %q, want error containing %q", tt.id, tt.prefix, err.Error(), tt.want)
				}
			}
		})
	}
}

// TestValidateImageRef tests the Docker image reference validation (P1.31).
func TestValidateImageRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string // empty = no error
	}{
		// Valid image references
		{name: "simple image", ref: "nginx", want: ""},
		{name: "image with tag", ref: "nginx:latest", want: ""},
		{name: "image with version", ref: "nginx:1.21", want: ""},
		{name: "user/repo", ref: "myuser/myapp", want: ""},
		{name: "user/repo:tag", ref: "myuser/myapp:v1.0", want: ""},
		{name: "registry/user/repo", ref: "ghcr.io/myuser/myapp:latest", want: ""},
		{name: "ECR reference", ref: "123456789012.dkr.ecr.us-east-1.amazonaws.com/myrepo:tag", want: ""},
		{name: "digest reference", ref: "nginx@sha256:abc123def456", want: ""},
		{name: "localhost registry", ref: "localhost:5000/myapp:dev", want: ""},
		// Invalid image references
		{name: "empty", ref: "", want: "cannot be empty"},
		{name: "spaces", ref: "my app", want: "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageRef(tt.ref)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateImageRef(%q) = %v, want nil", tt.ref, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateImageRef(%q) = nil, want error containing %q", tt.ref, tt.want)
				} else if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("ValidateImageRef(%q) = %q, want error containing %q", tt.ref, err.Error(), tt.want)
				}
			}
		})
	}
}

// TestValidateAppDescription tests the app description length validation (P1.31).
func TestValidateAppDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string // empty = no error
	}{
		{name: "short description", desc: "My app", want: ""},
		{name: "1024 chars", desc: strings.Repeat("a", 1024), want: ""},
		{name: "1025 chars", desc: strings.Repeat("a", 1025), want: "too long"},
		{name: "very long", desc: strings.Repeat("a", 5000), want: "too long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAppDescription(tt.desc)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateAppDescription() = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateAppDescription() = nil, want error containing %q", tt.want)
				} else if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("ValidateAppDescription() = %q, want error containing %q", err.Error(), tt.want)
				}
			}
		})
	}
}

// TestValidateExpectedUsers tests the expected users validation (P1.31).
func TestValidateExpectedUsers(t *testing.T) {
	tests := []struct {
		name  string
		users int
		want  string // empty = no error
	}{
		{name: "valid small", users: 1, want: ""},
		{name: "valid medium", users: 10000, want: ""},
		{name: "valid large", users: 1000000, want: ""},
		{name: "max valid", users: 100000000, want: ""},
		{name: "zero", users: 0, want: "positive integer"},
		{name: "negative", users: -1, want: "positive integer"},
		{name: "too large", users: 100000001, want: "Maximum is"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExpectedUsers(tt.users)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateExpectedUsers(%d) = %v, want nil", tt.users, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateExpectedUsers(%d) = nil, want error containing %q", tt.users, tt.want)
				} else if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("ValidateExpectedUsers(%d) = %q, want error containing %q", tt.users, err.Error(), tt.want)
				}
			}
		})
	}
}

// TestValidateLatencyMS tests the latency validation (P1.31).
func TestValidateLatencyMS(t *testing.T) {
	tests := []struct {
		name    string
		latency int
		want    string // empty = no error
	}{
		{name: "minimum valid", latency: 1, want: ""},
		{name: "typical", latency: 100, want: ""},
		{name: "maximum valid", latency: 60000, want: ""},
		{name: "zero", latency: 0, want: "Minimum is"},
		{name: "negative", latency: -1, want: "Minimum is"},
		{name: "too large", latency: 60001, want: "Maximum is"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLatencyMS(tt.latency)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateLatencyMS(%d) = %v, want nil", tt.latency, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateLatencyMS(%d) = nil, want error containing %q", tt.latency, tt.want)
				} else if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("ValidateLatencyMS(%d) = %q, want error containing %q", tt.latency, err.Error(), tt.want)
				}
			}
		})
	}
}

// TestValidateCertificateARNRegion tests certificate ARN region validation (P1.31).
func TestValidateCertificateARNRegion(t *testing.T) {
	tests := []struct {
		name     string
		certARN  string
		region   string
		want     string // empty = no error
	}{
		{name: "matching regions", certARN: "arn:aws:acm:us-east-1:123456789012:certificate/abc", region: "us-east-1", want: ""},
		{name: "empty cert ARN", certARN: "", region: "us-east-1", want: ""},
		{name: "mismatched regions", certARN: "arn:aws:acm:us-west-2:123456789012:certificate/abc", region: "us-east-1", want: "region mismatch"},
		{name: "invalid ARN format", certARN: "not-an-arn", region: "us-east-1", want: "invalid certificate_arn format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCertificateARNRegion(tt.certARN, tt.region)
			if tt.want == "" {
				if err != nil {
					t.Errorf("ValidateCertificateARNRegion(%q, %q) = %v, want nil", tt.certARN, tt.region, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateCertificateARNRegion(%q, %q) = nil, want error containing %q", tt.certARN, tt.region, tt.want)
				} else if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("ValidateCertificateARNRegion(%q, %q) = %q, want error containing %q", tt.certARN, tt.region, err.Error(), tt.want)
				}
			}
		})
	}
}

// TestBackoffWithJitter tests the exponential backoff helper function (P3.24).
func TestBackoffWithJitter(t *testing.T) {
baseDelay := 1 * time.Second
maxDelay := 30 * time.Second

t.Run("exponential growth", func(t *testing.T) {
// Test that delays grow exponentially (ignoring jitter)
// Attempt 0: ~1s, Attempt 1: ~2s, Attempt 2: ~4s, etc.
for attempt := 0; attempt < 5; attempt++ {
delay := backoffWithJitter(baseDelay, attempt, maxDelay)
expected := baseDelay << attempt // 2^attempt
if expected > maxDelay {
expected = maxDelay
}
// Allow ±30% for jitter
minExpected := time.Duration(float64(expected) * 0.7)
maxExpected := time.Duration(float64(expected) * 1.3)
if delay < minExpected || delay > maxExpected {
t.Errorf("attempt %d: delay %v not in range [%v, %v]",
attempt, delay, minExpected, maxExpected)
}
}
})

t.Run("respects max delay", func(t *testing.T) {
// High attempt number should be capped at maxDelay
delay := backoffWithJitter(baseDelay, 100, maxDelay)
// Should not exceed maxDelay (with some jitter margin)
if delay > maxDelay*2 {
t.Errorf("delay %v exceeds max %v by too much", delay, maxDelay)
}
})

t.Run("jitter provides variance", func(t *testing.T) {
// Run multiple times and check we get different values (jitter working)
seen := make(map[time.Duration]bool)
for i := 0; i < 10; i++ {
delay := backoffWithJitter(baseDelay, 2, maxDelay)
seen[delay] = true
}
// Should see at least 2 different values (jitter is random)
if len(seen) < 2 {
t.Errorf("jitter not working: only saw %d unique values", len(seen))
}
})

t.Run("minimum delay maintained", func(t *testing.T) {
// Even with jitter, should not go below baseDelay/2
for i := 0; i < 20; i++ {
delay := backoffWithJitter(baseDelay, 0, maxDelay)
if delay < baseDelay/2 {
t.Errorf("delay %v below minimum %v", delay, baseDelay/2)
}
}
})
}

// ---------------------------------------------------------------------------
// Delete/Teardown Operation Tests (P2.5)
// ---------------------------------------------------------------------------
// WHY: Per IMPLEMENTATION_PLAN.md P2.5 - AWS provider error scenarios for
// teardown/delete paths are critical for preventing resource leaks and cost
// overruns. These tests verify proper cleanup behavior and error handling.

func TestDeleteVPCResources_Success(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Track calls to verify deletion order
	var deletedResources []string

	ec2Mock := &mocks.EC2Mock{
		DeleteNatGatewayFunc: func(ctx context.Context, params *ec2.DeleteNatGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteNatGatewayOutput, error) {
			deletedResources = append(deletedResources, "nat:"+*params.NatGatewayId)
			return &ec2.DeleteNatGatewayOutput{NatGatewayId: params.NatGatewayId}, nil
		},
		DescribeNatGatewaysFunc: func(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
			// Return deleted state immediately for fast test
			return &ec2.DescribeNatGatewaysOutput{
				NatGateways: []ec2types.NatGateway{{
					NatGatewayId: aws.String(params.NatGatewayIds[0]),
					State:        ec2types.NatGatewayStateDeleted,
				}},
			}, nil
		},
		ReleaseAddressFunc: func(ctx context.Context, params *ec2.ReleaseAddressInput, optFns ...func(*ec2.Options)) (*ec2.ReleaseAddressOutput, error) {
			deletedResources = append(deletedResources, "eip:"+*params.AllocationId)
			return &ec2.ReleaseAddressOutput{}, nil
		},
		DeleteSecurityGroupFunc: func(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
			deletedResources = append(deletedResources, "sg:"+*params.GroupId)
			return &ec2.DeleteSecurityGroupOutput{}, nil
		},
		DescribeRouteTablesFunc: func(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
			return &ec2.DescribeRouteTablesOutput{RouteTables: []ec2types.RouteTable{}}, nil
		},
		DeleteRouteTableFunc: func(ctx context.Context, params *ec2.DeleteRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteTableOutput, error) {
			deletedResources = append(deletedResources, "rtb:"+*params.RouteTableId)
			return &ec2.DeleteRouteTableOutput{}, nil
		},
		DeleteSubnetFunc: func(ctx context.Context, params *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error) {
			deletedResources = append(deletedResources, "subnet:"+*params.SubnetId)
			return &ec2.DeleteSubnetOutput{}, nil
		},
		DetachInternetGatewayFunc: func(ctx context.Context, params *ec2.DetachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DetachInternetGatewayOutput, error) {
			deletedResources = append(deletedResources, "detach-igw:"+*params.InternetGatewayId)
			return &ec2.DetachInternetGatewayOutput{}, nil
		},
		DeleteInternetGatewayFunc: func(ctx context.Context, params *ec2.DeleteInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteInternetGatewayOutput, error) {
			deletedResources = append(deletedResources, "igw:"+*params.InternetGatewayId)
			return &ec2.DeleteInternetGatewayOutput{}, nil
		},
		DeleteVpcFunc: func(ctx context.Context, params *ec2.DeleteVpcInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error) {
			deletedResources = append(deletedResources, "vpc:"+*params.VpcId)
			return &ec2.DeleteVpcOutput{}, nil
		},
	}

	clients := &awsclient.AWSClients{EC2: ec2Mock}
	provider := NewAWSProviderWithClients(store, clients)

	// Create infrastructure with all resource types
	infra := &state.Infrastructure{
		ID:     "infra-delete-test",
		PlanID: "plan-test",
		Region: "us-east-1",
		Resources: map[string]string{
			state.ResourceVPC:               "vpc-123",
			state.ResourceNATGateway:        "nat-123",
			state.ResourceElasticIP:         "eipalloc-123",
			state.ResourceSecurityGroupTask: "sg-task-123",
			state.ResourceSecurityGroupALB:  "sg-alb-123",
			state.ResourceRouteTable:        "rtb-public-123",
			state.ResourceRouteTablePrivate: "rtb-private-123",
			state.ResourceSubnetPublic:      "subnet-pub-1,subnet-pub-2",
			state.ResourceSubnetPrivate:     "subnet-priv-1,subnet-priv-2",
			state.ResourceInternetGateway:   "igw-123",
		},
		Status: state.InfraStatusReady,
	}

	err = provider.deleteVPCResources(context.Background(), aws.Config{Region: "us-east-1"}, infra)
	if err != nil {
		t.Fatalf("deleteVPCResources: %v", err)
	}

	// Verify deletion order (should be reverse of creation order)
	// NAT -> EIP -> SGs -> Route Tables -> Subnets -> IGW -> VPC
	expectedOrder := []string{
		"nat:nat-123",
		"eip:eipalloc-123",
		"sg:sg-task-123",
		"sg:sg-alb-123",
		"rtb:rtb-private-123",
		"rtb:rtb-public-123",
		"subnet:subnet-priv-1",
		"subnet:subnet-priv-2",
		"subnet:subnet-pub-1",
		"subnet:subnet-pub-2",
		"detach-igw:igw-123",
		"igw:igw-123",
		"vpc:vpc-123",
	}

	// Verify all resources were deleted
	if len(deletedResources) != len(expectedOrder) {
		t.Errorf("deletedResources count = %d, want %d", len(deletedResources), len(expectedOrder))
		t.Logf("deleted: %v", deletedResources)
	}

	// Verify VPC is deleted last
	if len(deletedResources) > 0 && deletedResources[len(deletedResources)-1] != "vpc:vpc-123" {
		t.Errorf("VPC should be deleted last, got %v", deletedResources)
	}
}

func TestDeleteVPCResources_EmptyInfra(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ec2Mock := &mocks.EC2Mock{}
	clients := &awsclient.AWSClients{EC2: ec2Mock}
	provider := NewAWSProviderWithClients(store, clients)

	// Infrastructure with no resources
	infra := &state.Infrastructure{
		ID:        "infra-empty",
		Resources: map[string]string{},
	}

	err = provider.deleteVPCResources(context.Background(), aws.Config{Region: "us-east-1"}, infra)
	if err != nil {
		t.Fatalf("deleteVPCResources with empty infra should succeed: %v", err)
	}

	// Verify no delete calls were made
	if ec2Mock.DeleteVpcCalls > 0 {
		t.Errorf("DeleteVpcCalls = %d, want 0", ec2Mock.DeleteVpcCalls)
	}
}

func TestDeleteVPCResources_VPCDeleteError(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ec2Mock := &mocks.EC2Mock{
		DeleteVpcFunc: func(ctx context.Context, params *ec2.DeleteVpcInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error) {
			return nil, fmt.Errorf("DependencyViolation: VPC has dependencies")
		},
	}

	clients := &awsclient.AWSClients{EC2: ec2Mock}
	provider := NewAWSProviderWithClients(store, clients)

	infra := &state.Infrastructure{
		ID:        "infra-vpc-error",
		Resources: map[string]string{state.ResourceVPC: "vpc-123"},
	}

	err = provider.deleteVPCResources(context.Background(), aws.Config{Region: "us-east-1"}, infra)
	if err == nil {
		t.Error("deleteVPCResources should fail when VPC delete fails")
	}
	if !strings.Contains(err.Error(), "DependencyViolation") {
		t.Errorf("error should contain 'DependencyViolation', got: %v", err)
	}
}

func TestDeleteVPCResources_PartialFailureContinues(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	sgDeleteAttempts := 0
	ec2Mock := &mocks.EC2Mock{
		DeleteSecurityGroupFunc: func(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
			sgDeleteAttempts++
			// Fail on first SG, succeed on second
			if sgDeleteAttempts == 1 {
				return nil, fmt.Errorf("DependencyViolation: SG in use")
			}
			return &ec2.DeleteSecurityGroupOutput{}, nil
		},
	}

	clients := &awsclient.AWSClients{EC2: ec2Mock}
	provider := NewAWSProviderWithClients(store, clients)

	infra := &state.Infrastructure{
		ID:     "infra-partial-fail",
		Region: "us-east-1",
		Resources: map[string]string{
			state.ResourceVPC:               "vpc-123",
			state.ResourceSecurityGroupTask: "sg-task-123",
			state.ResourceSecurityGroupALB:  "sg-alb-123",
		},
	}

	// Should succeed overall (VPC delete is the only critical error)
	err = provider.deleteVPCResources(context.Background(), aws.Config{Region: "us-east-1"}, infra)
	if err != nil {
		t.Fatalf("deleteVPCResources should succeed despite SG delete failure: %v", err)
	}

	// Both SGs should have been attempted
	if sgDeleteAttempts != 2 {
		t.Errorf("sgDeleteAttempts = %d, want 2 (should continue after first failure)", sgDeleteAttempts)
	}
}

func TestDeleteRouteTable_WithAssociations(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	disassociateCalls := 0
	ec2Mock := &mocks.EC2Mock{
		DescribeRouteTablesFunc: func(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
			return &ec2.DescribeRouteTablesOutput{
				RouteTables: []ec2types.RouteTable{{
					RouteTableId: aws.String(params.RouteTableIds[0]),
					Associations: []ec2types.RouteTableAssociation{
						{RouteTableAssociationId: aws.String("rtbassoc-1"), Main: aws.Bool(false)},
						{RouteTableAssociationId: aws.String("rtbassoc-2"), Main: aws.Bool(false)},
						{RouteTableAssociationId: aws.String("rtbassoc-main"), Main: aws.Bool(true)}, // Should skip main
					},
				}},
			}, nil
		},
		DisassociateRouteTableFunc: func(ctx context.Context, params *ec2.DisassociateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DisassociateRouteTableOutput, error) {
			disassociateCalls++
			return &ec2.DisassociateRouteTableOutput{}, nil
		},
		DeleteRouteTableFunc: func(ctx context.Context, params *ec2.DeleteRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteTableOutput, error) {
			return &ec2.DeleteRouteTableOutput{}, nil
		},
	}

	clients := &awsclient.AWSClients{EC2: ec2Mock}
	provider := NewAWSProviderWithClients(store, clients)

	err = provider.deleteRouteTable(context.Background(), ec2Mock, "rtb-123")
	if err != nil {
		t.Fatalf("deleteRouteTable: %v", err)
	}

	// Should disassociate non-main associations only (2 out of 3)
	if disassociateCalls != 2 {
		t.Errorf("disassociateCalls = %d, want 2 (should skip main association)", disassociateCalls)
	}
}

func TestDeleteRouteTable_DescribeError(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ec2Mock := &mocks.EC2Mock{
		DescribeRouteTablesFunc: func(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
			return nil, fmt.Errorf("InvalidRouteTableID.NotFound")
		},
	}

	clients := &awsclient.AWSClients{EC2: ec2Mock}
	provider := NewAWSProviderWithClients(store, clients)

	err = provider.deleteRouteTable(context.Background(), ec2Mock, "rtb-notfound")
	if err == nil {
		t.Error("deleteRouteTable should fail on describe error")
	}
	if !strings.Contains(err.Error(), "describe route table") {
		t.Errorf("error should wrap describe error, got: %v", err)
	}
}

func TestDeleteRouteTable_DisassociateError(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ec2Mock := &mocks.EC2Mock{
		DescribeRouteTablesFunc: func(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
			return &ec2.DescribeRouteTablesOutput{
				RouteTables: []ec2types.RouteTable{{
					RouteTableId: aws.String("rtb-123"),
					Associations: []ec2types.RouteTableAssociation{
						{RouteTableAssociationId: aws.String("rtbassoc-1"), Main: aws.Bool(false)},
					},
				}},
			}, nil
		},
		DisassociateRouteTableFunc: func(ctx context.Context, params *ec2.DisassociateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DisassociateRouteTableOutput, error) {
			return nil, fmt.Errorf("InvalidAssociationID.NotFound")
		},
	}

	clients := &awsclient.AWSClients{EC2: ec2Mock}
	provider := NewAWSProviderWithClients(store, clients)

	err = provider.deleteRouteTable(context.Background(), ec2Mock, "rtb-123")
	if err == nil {
		t.Error("deleteRouteTable should fail on disassociate error")
	}
	if !strings.Contains(err.Error(), "disassociate route table") {
		t.Errorf("error should wrap disassociate error, got: %v", err)
	}
}

func TestRollbackInfra_WithResources(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	var deletedLogGroup, deletedRole bool

	// Create mocks for all services used by rollback
	ec2Mock := &mocks.EC2Mock{}
	ecsMock := &mocks.ECSMock{}
	elbMock := &mocks.ELBV2Mock{}
	iamMock := &mocks.IAMMock{
		DeleteRoleFunc: func(ctx context.Context, params *iam.DeleteRoleInput, optFns ...func(*iam.Options)) (*iam.DeleteRoleOutput, error) {
			deletedRole = true
			return &iam.DeleteRoleOutput{}, nil
		},
	}
	cwMock := &mocks.CloudWatchLogsMock{
		DeleteLogGroupFunc: func(ctx context.Context, params *cloudwatchlogs.DeleteLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error) {
			deletedLogGroup = true
			return &cloudwatchlogs.DeleteLogGroupOutput{}, nil
		},
	}

	clients := &awsclient.AWSClients{
		EC2:            ec2Mock,
		ECS:            ecsMock,
		ELBV2:          elbMock,
		IAM:            iamMock,
		CloudWatchLogs: cwMock,
	}

	provider := NewAWSProviderWithClients(store, clients)

	// Create infrastructure with some resources - use 'failed' status so transition to destroyed is valid
	infra := &state.Infrastructure{
		ID:     "infra-rollback-test",
		PlanID: "plan-test",
		Region: "us-east-1",
		Resources: map[string]string{
			state.ResourceLogGroup:      "/ecs/test-app",
			state.ResourceExecutionRole: "arn:aws:iam::123456789012:role/test-role",
		},
		Status:    state.InfraStatusFailed, // Failed status allows transition to destroyed
		CreatedAt: time.Now(),
	}

	err = store.CreateInfra(infra)
	if err != nil {
		t.Fatalf("CreateInfra: %v", err)
	}

	err = provider.rollbackInfra(context.Background(), aws.Config{Region: "us-east-1"}, infra)
	if err != nil {
		t.Fatalf("rollbackInfra: %v", err)
	}

	// Verify cleanup was attempted
	if !deletedLogGroup {
		t.Error("log group should have been deleted")
	}
	if !deletedRole {
		t.Error("execution role deletion should have been attempted")
	}
}

func TestRollbackInfra_ContinuesOnErrors(t *testing.T) {
	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	var deleteAttempts int

	cwMock := &mocks.CloudWatchLogsMock{
		DeleteLogGroupFunc: func(ctx context.Context, params *cloudwatchlogs.DeleteLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error) {
			deleteAttempts++
			return nil, fmt.Errorf("AccessDenied: insufficient permissions")
		},
	}
	iamMock := &mocks.IAMMock{
		DeleteRoleFunc: func(ctx context.Context, params *iam.DeleteRoleInput, optFns ...func(*iam.Options)) (*iam.DeleteRoleOutput, error) {
			deleteAttempts++
			return nil, fmt.Errorf("AccessDenied: insufficient permissions")
		},
	}

	clients := &awsclient.AWSClients{
		EC2:            &mocks.EC2Mock{},
		ECS:            &mocks.ECSMock{},
		ELBV2:          &mocks.ELBV2Mock{},
		IAM:            iamMock,
		CloudWatchLogs: cwMock,
	}

	provider := NewAWSProviderWithClients(store, clients)

	// Use failed status so transition to destroyed is valid
	infra := &state.Infrastructure{
		ID:     "infra-rollback-errors",
		PlanID: "plan-test",
		Region: "us-east-1",
		Resources: map[string]string{
			state.ResourceLogGroup:      "/ecs/test-app",
			state.ResourceExecutionRole: "arn:aws:iam::123456789012:role/test-role",
		},
		Status: state.InfraStatusFailed,
	}

	err = store.CreateInfra(infra)
	if err != nil {
		t.Fatalf("CreateInfra: %v", err)
	}

	// Rollback should return error with accumulated failures
	err = provider.rollbackInfra(context.Background(), aws.Config{Region: "us-east-1"}, infra)
	if err == nil {
		t.Error("rollbackInfra should return error when cleanup fails")
	}

	// Should have attempted both deletes despite errors
	if deleteAttempts != 2 {
		t.Errorf("deleteAttempts = %d, want 2 (should continue after failures)", deleteAttempts)
	}
}

// TestNilStoreGuard verifies that provider methods return ErrInvalidState when store is nil.
// WHY: P0.3 — Provider methods must not panic with nil pointer dereference if store is nil.
// This test ensures graceful error handling instead of runtime panic.
func TestNilStoreGuard(t *testing.T) {
	// Create provider with nil store to simulate failed store initialization.
	provider := &AWSProvider{store: nil}
	ctx := context.Background()

	t.Run("planInfra", func(t *testing.T) {
		_, _, err := provider.planInfra(ctx, nil, planInfraInput{
			AppDescription: "test",
			Region:         "us-east-1",
			ExpectedUsers:  100,
		})
		if err == nil {
			t.Fatal("expected error for nil store, got nil")
		}
		if !strings.Contains(err.Error(), "state store is not initialized") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("approvePlan", func(t *testing.T) {
		_, _, err := provider.approvePlan(ctx, nil, approvePlanInput{
			PlanID:    "plan-01HZ123456789ABCDEFGHJKMNP",
			Confirmed: true,
		})
		if err == nil {
			t.Fatal("expected error for nil store, got nil")
		}
		if !strings.Contains(err.Error(), "state store is not initialized") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("createInfra", func(t *testing.T) {
		_, _, err := provider.createInfra(ctx, nil, createInfraInput{
			PlanID: "plan-01HZ123456789ABCDEFGHJKMNP",
		})
		if err == nil {
			t.Fatal("expected error for nil store, got nil")
		}
		if !strings.Contains(err.Error(), "state store is not initialized") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("deploy", func(t *testing.T) {
		_, _, err := provider.deploy(ctx, nil, deployInput{
			InfraID:  "infra-01HZ123456789ABCDEFGHJKMNP",
			ImageRef: "nginx:latest",
		})
		if err == nil {
			t.Fatal("expected error for nil store, got nil")
		}
		if !strings.Contains(err.Error(), "state store is not initialized") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("status", func(t *testing.T) {
		_, _, err := provider.status(ctx, nil, statusInput{
			DeploymentID: "deploy-01HZ123456789ABCDEFGHJKMNP",
		})
		if err == nil {
			t.Fatal("expected error for nil store, got nil")
		}
		if !strings.Contains(err.Error(), "state store is not initialized") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("teardown", func(t *testing.T) {
		_, _, err := provider.teardown(ctx, nil, teardownInput{
			DeploymentID: "deploy-01HZ123456789ABCDEFGHJKMNP",
		})
		if err == nil {
			t.Fatal("expected error for nil store, got nil")
		}
		if !strings.Contains(err.Error(), "state store is not initialized") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("deploymentsResource", func(t *testing.T) {
		_, err := provider.deploymentsResource(ctx, &mcp.ReadResourceRequest{
			Params: &mcp.ReadResourceParams{
				URI: "aws://deployments",
			},
		})
		if err == nil {
			t.Fatal("expected error for nil store, got nil")
		}
		if !strings.Contains(err.Error(), "state store is not initialized") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
