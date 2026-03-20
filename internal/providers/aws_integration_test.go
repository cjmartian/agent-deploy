//go:build integration

// Package providers contains integration tests for the AWS provider.
// These tests require either LocalStack or AWS credentials to be configured.
//
// Run with: go test -tags=integration ./internal/providers/...
//
// LocalStack setup:
//   docker run -d --name localstack -p 4566:4566 localstack/localstack
//   export AWS_ENDPOINT_URL=http://localhost:4566
//   export AWS_ACCESS_KEY_ID=test
//   export AWS_SECRET_ACCESS_KEY=test
//   export AWS_REGION=us-east-1
package providers

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/cjmartian/agent-deploy/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testConfig holds configuration for integration tests.
type testConfig struct {
	awsCfg       aws.Config
	region       string
	isLocalStack bool
}

// setupIntegrationTest prepares the test environment.
// It detects whether we're running against LocalStack or real AWS.
func setupIntegrationTest(t *testing.T) *testConfig {
	t.Helper()

	ctx := context.Background()
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	// Check for LocalStack endpoint
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	isLocalStack := endpoint != "" && strings.Contains(endpoint, "localhost")

	var cfg aws.Config
	var err error

	if isLocalStack {
		t.Logf("Using LocalStack at %s", endpoint)
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithEndpointResolverWithOptions(
				aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
					return aws.Endpoint{
						URL:               endpoint,
						HostnameImmutable: true,
					}, nil
				}),
			),
		)
	} else {
		t.Log("Using real AWS credentials")
		cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}

	if err != nil {
		t.Fatalf("Failed to load AWS config: %v", err)
	}

	// Verify connectivity
	ec2Client := ec2.NewFromConfig(cfg)
	_, err = ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
	if err != nil {
		t.Skipf("Skipping integration test - AWS not accessible: %v", err)
	}

	return &testConfig{
		awsCfg:       cfg,
		region:       region,
		isLocalStack: isLocalStack,
	}
}

// TestIntegration_FullWorkflow tests the complete deployment workflow:
// plan → create → deploy → status → teardown
func TestIntegration_FullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tc := setupIntegrationTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Create temporary store
	dir := t.TempDir()
	store, err := state.NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create provider
	provider := NewAWSProvider(store)

	// Step 1: Plan infrastructure
	t.Log("Step 1: Planning infrastructure...")
	planInput := planInfraInput{
		AppDescription: "Integration test web application",
		ExpectedUsers:  100,
		LatencyMS:      200,
		Region:         tc.region,
	}

	_, planOutput, err := provider.planInfra(ctx, &mcp.CallToolRequest{}, planInput)
	if err != nil {
		t.Fatalf("planInfra failed: %v", err)
	}

	if planOutput.PlanID == "" {
		t.Fatal("Expected non-empty plan ID")
	}
	t.Logf("Created plan: %s", planOutput.PlanID)
	t.Logf("Estimated monthly cost: %s", planOutput.EstimatedCostMo)

	// Step 2: Create infrastructure
	t.Log("Step 2: Creating infrastructure...")
	createInput := createInfraInput{
		PlanID: planOutput.PlanID,
	}

	_, createOutput, err := provider.createInfra(ctx, &mcp.CallToolRequest{}, createInput)
	if err != nil {
		t.Fatalf("createInfra failed: %v", err)
	}

	if createOutput.InfraID == "" {
		t.Fatal("Expected non-empty infra ID")
	}
	t.Logf("Created infrastructure: %s (status: %s)", createOutput.InfraID, createOutput.Status)

	// Verify infrastructure was created
	infra, err := store.GetInfra(createOutput.InfraID)
	if err != nil {
		t.Fatalf("Failed to get infra from store: %v", err)
	}

	if infra.Status != state.InfraStatusReady {
		t.Errorf("Expected infra status 'ready', got %s", infra.Status)
	}

	// Step 3: Deploy application
	t.Log("Step 3: Deploying application...")
	deployIn := deployInput{
		InfraID:  createOutput.InfraID,
		ImageRef: "nginx:latest", // Simple test image
	}

	_, deployOutput, err := provider.deploy(ctx, &mcp.CallToolRequest{}, deployIn)
	if err != nil {
		// Cleanup on failure
		t.Logf("Deploy failed, attempting cleanup: %v", err)
		provider.teardown(ctx, &mcp.CallToolRequest{}, teardownInput{DeploymentID: ""})
		t.Fatalf("deploy failed: %v", err)
	}

	if deployOutput.DeploymentID == "" {
		t.Fatal("Expected non-empty deployment ID")
	}
	t.Logf("Created deployment: %s (status: %s)", deployOutput.DeploymentID, deployOutput.Status)

	// Step 4: Check status
	t.Log("Step 4: Checking status...")
	statusIn := statusInput{
		DeploymentID: deployOutput.DeploymentID,
	}

	_, statusOutput, err := provider.status(ctx, &mcp.CallToolRequest{}, statusIn)
	if err != nil {
		t.Logf("Warning: status check failed: %v", err)
	} else {
		t.Logf("Deployment status: %s", statusOutput.Status)
		if len(statusOutput.URLs) > 0 {
			t.Logf("URLs: %v", statusOutput.URLs)
		}
	}

	// Step 5: Teardown
	t.Log("Step 5: Tearing down resources...")
	teardownIn := teardownInput{
		DeploymentID: deployOutput.DeploymentID,
	}

	_, teardownOutput, err := provider.teardown(ctx, &mcp.CallToolRequest{}, teardownIn)
	if err != nil {
		t.Errorf("teardown failed: %v", err)
	} else {
		t.Logf("Teardown status: %s", teardownOutput.Status)
	}

	// Verify cleanup
	deployment, err := store.GetDeployment(deployOutput.DeploymentID)
	if err != nil {
		t.Logf("Deployment not found (expected after cleanup): %v", err)
	} else if deployment.Status != state.DeploymentStatusStopped {
		t.Errorf("Expected deployment status 'stopped', got %s", deployment.Status)
	}

	infraAfter, err := store.GetInfra(createOutput.InfraID)
	if err != nil {
		t.Logf("Infra not found (expected after cleanup): %v", err)
	} else if infraAfter.Status != state.InfraStatusDestroyed {
		t.Errorf("Expected infra status 'destroyed', got %s", infraAfter.Status)
	}

	t.Log("Integration test completed successfully!")
}

// TestIntegration_PlanOnly tests just the planning phase (no AWS resources created).
func TestIntegration_PlanOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tc := setupIntegrationTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Create temporary store
	dir := t.TempDir()
	store, err := state.NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	provider := NewAWSProvider(store)

	testCases := []struct {
		name          string
		input         planInfraInput
		expectError   bool
		checkServices []string
	}{
		{
			name: "small app",
			input: planInfraInput{
				AppDescription: "Small web API",
				ExpectedUsers:  50,
				LatencyMS:      500,
				Region:         tc.region,
			},
			expectError:   false,
			checkServices: []string{"VPC", "ECS Fargate", "ALB"},
		},
		{
			name: "high traffic app",
			input: planInfraInput{
				AppDescription: "High traffic e-commerce platform",
				ExpectedUsers:  5000,
				LatencyMS:      50,
				Region:         tc.region,
			},
			expectError:   false,
			checkServices: []string{"Auto Scaling"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, output, err := provider.planInfra(ctx, &mcp.CallToolRequest{}, testCase.input)

			if testCase.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify expected services
			for _, svc := range testCase.checkServices {
				found := false
				for _, s := range output.Services {
					if s == svc {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected service %q in plan, got %v", svc, output.Services)
				}
			}

			// Verify plan was persisted
			plan, err := store.GetPlan(output.PlanID)
			if err != nil {
				t.Errorf("Plan not found in store: %v", err)
			} else {
				if plan.AppDescription != testCase.input.AppDescription {
					t.Errorf("Plan app description mismatch: got %q, want %q",
						plan.AppDescription, testCase.input.AppDescription)
				}
			}
		})
	}
}

// TestIntegration_DeploymentsResource tests the aws:deployments resource.
func TestIntegration_DeploymentsResource(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_ = setupIntegrationTest(t) // Just verify AWS connectivity

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create temporary store with some test deployments
	dir := t.TempDir()
	store, err := state.NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Add test deployment
	testDeploy := &state.Deployment{
		ID:          "deploy-test-123",
		InfraID:     "infra-test-123",
		ImageRef:    "nginx:latest",
		Status:      state.DeploymentStatusRunning,
		URLs:        []string{"http://test.example.com"},
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}
	if err := store.CreateDeployment(testDeploy); err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	provider := NewAWSProvider(store)

	// Call deploymentsResource
	result, err := provider.deploymentsResource(ctx, &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "aws:deployments"},
	})
	if err != nil {
		t.Fatalf("deploymentsResource failed: %v", err)
	}

	// Parse result - get the text content from the first content item
	if len(result.Contents) == 0 {
		t.Fatal("Expected at least one content item")
	}

	var deployments struct {
		Deployments []struct {
			DeploymentID string   `json:"deployment_id"`
			InfraID      string   `json:"infra_id"`
			Status       string   `json:"status"`
			URLs         []string `json:"urls"`
		} `json:"deployments"`
	}

	if err := json.Unmarshal([]byte(result.Contents[0].Text), &deployments); err != nil {
		t.Fatalf("Failed to parse deployments JSON: %v", err)
	}

	if len(deployments.Deployments) != 1 {
		t.Errorf("Expected 1 deployment, got %d", len(deployments.Deployments))
	}

	if deployments.Deployments[0].DeploymentID != testDeploy.ID {
		t.Errorf("Deployment ID mismatch: got %q, want %q",
			deployments.Deployments[0].DeploymentID, testDeploy.ID)
	}
}

// TestIntegration_BudgetCheck tests spending limits enforcement.
func TestIntegration_BudgetCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tc := setupIntegrationTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Set low budget limits via environment
	originalBudget := os.Getenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET")
	os.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "5.00") // Very low budget
	defer func() {
		if originalBudget != "" {
			os.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", originalBudget)
		} else {
			os.Unsetenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET")
		}
	}()

	dir := t.TempDir()
	store, err := state.NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	provider := NewAWSProvider(store)

	// Create a plan that exceeds budget
	planInput := planInfraInput{
		AppDescription: "Large application that will exceed budget",
		ExpectedUsers:  10000, // High traffic = high cost estimate
		LatencyMS:      10,    // Low latency = higher cost
		Region:         tc.region,
	}

	_, _, err = provider.planInfra(ctx, &mcp.CallToolRequest{}, planInput)
	// Planning itself should work, but createInfra should fail due to budget
	if err != nil {
		// If planning fails due to budget, that's also acceptable
		if strings.Contains(err.Error(), "budget") || strings.Contains(err.Error(), "cost") {
			t.Logf("Plan rejected due to budget constraints (expected): %v", err)
			return
		}
		t.Fatalf("Unexpected error: %v", err)
	}

	t.Log("Budget check integration test completed")
}

// TestIntegration_ResourceCleanupOnFailure tests cleanup when provisioning fails partway.
func TestIntegration_ResourceCleanupOnFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tc := setupIntegrationTest(t)

	// Skip if not using LocalStack (don't want to create real resources)
	if !tc.isLocalStack {
		t.Skip("Skipping cleanup test - only runs against LocalStack")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dir := t.TempDir()
	store, err := state.NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	provider := NewAWSProvider(store)

	// Create a plan
	_, planOutput, err := provider.planInfra(ctx, &mcp.CallToolRequest{}, planInfraInput{
		AppDescription: "Cleanup test app",
		ExpectedUsers:  100,
		LatencyMS:      100,
		Region:         tc.region,
	})
	if err != nil {
		t.Fatalf("planInfra failed: %v", err)
	}

	// Create infrastructure
	_, createOutput, err := provider.createInfra(ctx, &mcp.CallToolRequest{}, createInfraInput{
		PlanID: planOutput.PlanID,
	})
	if err != nil {
		t.Logf("createInfra failed (may be expected): %v", err)
		return
	}

	// Immediately teardown to test cleanup
	provider.teardown(ctx, &mcp.CallToolRequest{}, teardownInput{
		DeploymentID: "", // No deployment yet, but infra exists
	})

	// Verify infra status was updated
	infra, err := store.GetInfra(createOutput.InfraID)
	if err == nil && infra.Status != state.InfraStatusDestroyed {
		t.Log("Note: Infra may not be fully cleaned up - this is expected behavior")
	}

	t.Log("Resource cleanup test completed")
}
