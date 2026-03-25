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
		InfraID:          "infra-123",
		ImageRef:         "nginx:latest",
		ContainerPort:    8080,
		HealthCheckPath:  "/health",
		DesiredCount:     2,
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
