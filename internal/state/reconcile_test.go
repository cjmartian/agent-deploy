package state

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// TestReconcileResult verifies the ReconcileResult struct.
func TestReconcileResult(t *testing.T) {
	timestamp := time.Now()
	result := &ReconcileResult{
		OrphanedResources: []OrphanedResource{
			{
				ResourceType: "vpc",
				ResourceID:   "vpc-123",
				Region:       "us-east-1",
				InfraID:      "infra-test",
			},
		},
		StaleLocalEntries: []StaleEntry{
			{
				EntryType:        "deployment",
				EntryID:          "deploy-123",
				MissingResources: []string{"ecs_service"},
			},
		},
		SyncedCount: 5,
		Timestamp:   timestamp,
	}

	if len(result.OrphanedResources) != 1 {
		t.Errorf("Expected 1 orphaned resource, got %d", len(result.OrphanedResources))
	}

	if result.OrphanedResources[0].ResourceType != "vpc" {
		t.Errorf("Expected resource type 'vpc', got %s", result.OrphanedResources[0].ResourceType)
	}

	if len(result.StaleLocalEntries) != 1 {
		t.Errorf("Expected 1 stale entry, got %d", len(result.StaleLocalEntries))
	}

	if result.StaleLocalEntries[0].EntryType != "deployment" {
		t.Errorf("Expected entry type 'deployment', got %s", result.StaleLocalEntries[0].EntryType)
	}

	if result.SyncedCount != 5 {
		t.Errorf("Expected synced count 5, got %d", result.SyncedCount)
	}

	if result.Timestamp != timestamp {
		t.Errorf("Expected timestamp %v, got %v", timestamp, result.Timestamp)
	}
}

// TestOrphanedResource verifies the OrphanedResource struct.
func TestOrphanedResource(t *testing.T) {
	orphan := OrphanedResource{
		ResourceType: "ecs_cluster",
		ResourceID:   "arn:aws:ecs:us-east-1:123456789:cluster/test",
		Region:       "us-east-1",
		DeploymentID: "deploy-abc",
		InfraID:      "infra-xyz",
		PlanID:       "plan-123",
	}

	if orphan.ResourceType != "ecs_cluster" {
		t.Errorf("Expected resource type 'ecs_cluster', got %s", orphan.ResourceType)
	}

	if orphan.ResourceID != "arn:aws:ecs:us-east-1:123456789:cluster/test" {
		t.Errorf("Expected resource ID 'arn:aws:ecs:us-east-1:123456789:cluster/test', got %s", orphan.ResourceID)
	}

	if orphan.Region != "us-east-1" {
		t.Errorf("Expected region 'us-east-1', got %s", orphan.Region)
	}

	if orphan.DeploymentID != "deploy-abc" {
		t.Errorf("Expected deployment ID 'deploy-abc', got %s", orphan.DeploymentID)
	}

	if orphan.InfraID != "infra-xyz" {
		t.Errorf("Expected infra ID 'infra-xyz', got %s", orphan.InfraID)
	}

	if orphan.PlanID != "plan-123" {
		t.Errorf("Expected plan ID 'plan-123', got %s", orphan.PlanID)
	}
}

// TestStaleEntry verifies the StaleEntry struct.
func TestStaleEntry(t *testing.T) {
	stale := StaleEntry{
		EntryType:        "infra",
		EntryID:          "infra-123",
		MissingResources: []string{"vpc", "alb"},
	}

	if stale.EntryType != "infra" {
		t.Errorf("Expected entry type 'infra', got %s", stale.EntryType)
	}

	if stale.EntryID != "infra-123" {
		t.Errorf("Expected entry ID 'infra-123', got %s", stale.EntryID)
	}

	if len(stale.MissingResources) != 2 {
		t.Errorf("Expected 2 missing resources, got %d", len(stale.MissingResources))
	}
}

// TestReconcilerWithMockStore tests reconciliation logic with mock store.
func TestReconcilerWithMockStore(t *testing.T) {
	// Create a temporary store
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create some test data
	infra := &Infrastructure{
		ID:     "infra-test-1",
		PlanID: "plan-test-1",
		Region: "us-east-1",
		Resources: map[string]string{
			ResourceVPC:        "vpc-12345",
			ResourceECSCluster: "arn:aws:ecs:us-east-1:123:cluster/test",
		},
		Status:    InfraStatusReady,
		CreatedAt: time.Now(),
	}

	err = store.CreateInfra(infra)
	if err != nil {
		t.Fatalf("Failed to create infra: %v", err)
	}

	deploy := &Deployment{
		ID:          "deploy-test-1",
		InfraID:     "infra-test-1",
		ImageRef:    "nginx:latest",
		Status:      DeploymentStatusRunning,
		URLs:        []string{"http://example.com"},
		ServiceARN:  "arn:aws:ecs:us-east-1:123:service/test",
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	err = store.CreateDeployment(deploy)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Verify data was created
	infras, err := store.ListInfra()
	if err != nil {
		t.Fatalf("Failed to list infra: %v", err)
	}

	if len(infras) != 1 {
		t.Errorf("Expected 1 infra, got %d", len(infras))
	}

	deployments, err := store.ListDeployments()
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	if len(deployments) != 1 {
		t.Errorf("Expected 1 deployment, got %d", len(deployments))
	}
}

// TestCleanupStaleEntries tests marking stale entries as stopped/destroyed.
func TestCleanupStaleEntries(t *testing.T) {
	// Create a temporary store
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create test deployment that will be marked stale
	deploy := &Deployment{
		ID:          "deploy-stale",
		InfraID:     "infra-missing", // Infra doesn't exist
		ImageRef:    "nginx:latest",
		Status:      DeploymentStatusRunning,
		URLs:        []string{"http://example.com"},
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	err = store.CreateDeployment(deploy)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Create test infra that will be marked stale
	infra := &Infrastructure{
		ID:     "infra-stale",
		PlanID: "plan-test",
		Region: "us-east-1",
		Resources: map[string]string{
			ResourceVPC: "vpc-nonexistent",
		},
		Status:    InfraStatusReady,
		CreatedAt: time.Now(),
	}

	err = store.CreateInfra(infra)
	if err != nil {
		t.Fatalf("Failed to create infra: %v", err)
	}

	// Test cleanup of stale entries (without actual reconciler - just test store operations)
	staleEntries := []StaleEntry{
		{
			EntryType:        "deployment",
			EntryID:          "deploy-stale",
			MissingResources: []string{"infra:infra-missing"},
		},
		{
			EntryType:        "infra",
			EntryID:          "infra-stale",
			MissingResources: []string{"vpc"},
		},
	}

	// Manually update status (simulating what CleanupStaleEntries does)
	for _, entry := range staleEntries {
		switch entry.EntryType {
		case "deployment":
			err = store.UpdateDeploymentStatus(entry.EntryID, DeploymentStatusStopped, nil)
			if err != nil {
				t.Errorf("Failed to update deployment status: %v", err)
			}
		case "infra":
			err = store.SetInfraStatus(entry.EntryID, InfraStatusDestroyed)
			if err != nil {
				t.Errorf("Failed to update infra status: %v", err)
			}
		}
	}

	// Verify deployment was marked as stopped
	updatedDeploy, err := store.GetDeployment("deploy-stale")
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}
	if updatedDeploy.Status != DeploymentStatusStopped {
		t.Errorf("Expected deployment status 'stopped', got %s", updatedDeploy.Status)
	}

	// Verify infra was marked as destroyed
	updatedInfra, err := store.GetInfra("infra-stale")
	if err != nil {
		t.Fatalf("Failed to get infra: %v", err)
	}
	if updatedInfra.Status != InfraStatusDestroyed {
		t.Errorf("Expected infra status 'destroyed', got %s", updatedInfra.Status)
	}
}

// TestCountSyncedResources tests the synced resource counting logic.
func TestCountSyncedResourcesLogic(t *testing.T) {
	// Create a temporary store
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create infra with multiple resources
	infra1 := &Infrastructure{
		ID:     "infra-1",
		PlanID: "plan-1",
		Region: "us-east-1",
		Resources: map[string]string{
			ResourceVPC:        "vpc-1",
			ResourceECSCluster: "cluster-1",
			ResourceALB:        "alb-1",
		},
		Status:    InfraStatusReady,
		CreatedAt: time.Now(),
	}

	infra2 := &Infrastructure{
		ID:     "infra-2",
		PlanID: "plan-2",
		Region: "us-east-1",
		Resources: map[string]string{
			ResourceVPC:        "vpc-2",
			ResourceECSCluster: "cluster-2",
		},
		Status:    InfraStatusReady,
		CreatedAt: time.Now(),
	}

	// Create destroyed infra (should not be counted)
	infraDestroyed := &Infrastructure{
		ID:     "infra-destroyed",
		PlanID: "plan-3",
		Region: "us-east-1",
		Resources: map[string]string{
			ResourceVPC: "vpc-3",
		},
		Status:    InfraStatusDestroyed,
		CreatedAt: time.Now(),
	}

	err = store.CreateInfra(infra1)
	if err != nil {
		t.Fatalf("Failed to create infra1: %v", err)
	}
	err = store.CreateInfra(infra2)
	if err != nil {
		t.Fatalf("Failed to create infra2: %v", err)
	}
	err = store.CreateInfra(infraDestroyed)
	if err != nil {
		t.Fatalf("Failed to create destroyed infra: %v", err)
	}

	// Count resources manually (simulating countSyncedResources)
	infras, err := store.ListInfra()
	if err != nil {
		t.Fatalf("Failed to list infra: %v", err)
	}

	count := 0
	for _, infra := range infras {
		if infra.Status == InfraStatusDestroyed {
			continue
		}
		for _, resourceID := range infra.Resources {
			if resourceID != "" {
				count++
			}
		}
	}

	// Expected: 3 (infra1) + 2 (infra2) = 5, infraDestroyed should be skipped
	if count != 5 {
		t.Errorf("Expected 5 synced resources, got %d", count)
	}
}

// TestReconcileResultWithErrors verifies error handling in results.
func TestReconcileResultWithErrors(t *testing.T) {
	timestamp := time.Now()
	result := &ReconcileResult{
		Timestamp: timestamp,
		Errors: []string{
			"failed to list VPCs: access denied",
			"failed to describe ECS clusters: timeout",
		},
	}

	if result.Timestamp != timestamp {
		t.Errorf("Expected timestamp %v, got %v", timestamp, result.Timestamp)
	}

	if len(result.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(result.Errors))
	}

	if result.Errors[0] != "failed to list VPCs: access denied" {
		t.Errorf("Unexpected error message: %s", result.Errors[0])
	}
}

// TestFindStaleEntriesLogic tests the logic for finding stale entries.
func TestFindStaleEntriesLogic(t *testing.T) {
	// Create a temporary store
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create deployment with missing infra
	deploy := &Deployment{
		ID:          "deploy-orphan",
		InfraID:     "infra-nonexistent",
		ImageRef:    "nginx:latest",
		Status:      DeploymentStatusRunning,
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	err = store.CreateDeployment(deploy)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Try to get the referenced infra (should fail)
	_, err = store.GetInfra(deploy.InfraID)
	if err == nil {
		t.Error("Expected error when getting non-existent infra")
	}

	// This deployment should be considered stale because its infra doesn't exist
	deployments, err := store.ListDeployments()
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	staleCount := 0
	for _, d := range deployments {
		if d.Status == DeploymentStatusStopped {
			continue
		}
		_, err := store.GetInfra(d.InfraID)
		if err != nil {
			staleCount++
		}
	}

	if staleCount != 1 {
		t.Errorf("Expected 1 stale deployment, got %d", staleCount)
	}
}

// TestSkipDestroyedInfra verifies destroyed infra is skipped during reconciliation.
func TestSkipDestroyedInfra(t *testing.T) {
	// Create a temporary store
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create destroyed infrastructure
	infra := &Infrastructure{
		ID:     "infra-destroyed",
		PlanID: "plan-test",
		Region: "us-east-1",
		Resources: map[string]string{
			ResourceVPC: "vpc-deleted",
		},
		Status:    InfraStatusDestroyed,
		CreatedAt: time.Now(),
	}

	err = store.CreateInfra(infra)
	if err != nil {
		t.Fatalf("Failed to create infra: %v", err)
	}

	// Verify the infra is skipped in stale detection
	infras, err := store.ListInfra()
	if err != nil {
		t.Fatalf("Failed to list infra: %v", err)
	}

	activeCount := 0
	for _, i := range infras {
		if i.Status != InfraStatusDestroyed {
			activeCount++
		}
	}

	if activeCount != 0 {
		t.Errorf("Expected 0 active infra (all destroyed), got %d", activeCount)
	}
}

// TestSkipStoppedDeployments verifies stopped deployments are skipped.
func TestSkipStoppedDeployments(t *testing.T) {
	ctx := context.Background()
	_ = ctx // Would be used with actual reconciler

	// Create a temporary store
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create stopped deployment
	deploy := &Deployment{
		ID:          "deploy-stopped",
		InfraID:     "infra-nonexistent",
		ImageRef:    "nginx:latest",
		Status:      DeploymentStatusStopped,
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}

	err = store.CreateDeployment(deploy)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Verify the deployment is skipped in stale detection
	deployments, err := store.ListDeployments()
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	activeCount := 0
	for _, d := range deployments {
		if d.Status != DeploymentStatusStopped {
			activeCount++
		}
	}

	if activeCount != 0 {
		t.Errorf("Expected 0 active deployments (all stopped), got %d", activeCount)
	}
}

// ============================================================================
// Mock implementations for reconciler tests
// ============================================================================

// mockEC2Client implements ReconcileEC2API for testing.
type mockEC2Client struct {
	DescribeVpcsFunc func(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
}

func (m *mockEC2Client) DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	if m.DescribeVpcsFunc != nil {
		return m.DescribeVpcsFunc(ctx, params, optFns...)
	}
	return &ec2.DescribeVpcsOutput{}, nil
}

// mockECSClient implements ReconcileECSAPI for testing.
type mockECSClient struct {
	ListClustersFunc     func(ctx context.Context, params *ecs.ListClustersInput, optFns ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeClustersFunc func(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)
	DescribeServicesFunc func(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
}

func (m *mockECSClient) ListClusters(ctx context.Context, params *ecs.ListClustersInput, optFns ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
	if m.ListClustersFunc != nil {
		return m.ListClustersFunc(ctx, params, optFns...)
	}
	return &ecs.ListClustersOutput{}, nil
}

func (m *mockECSClient) DescribeClusters(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
	if m.DescribeClustersFunc != nil {
		return m.DescribeClustersFunc(ctx, params, optFns...)
	}
	return &ecs.DescribeClustersOutput{}, nil
}

func (m *mockECSClient) DescribeServices(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	if m.DescribeServicesFunc != nil {
		return m.DescribeServicesFunc(ctx, params, optFns...)
	}
	return &ecs.DescribeServicesOutput{}, nil
}

// mockELBV2Client implements ReconcileELBV2API for testing.
type mockELBV2Client struct {
	DescribeLoadBalancersFunc func(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error)
	DescribeTagsFunc          func(ctx context.Context, params *elasticloadbalancingv2.DescribeTagsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTagsOutput, error)
}

func (m *mockELBV2Client) DescribeLoadBalancers(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	if m.DescribeLoadBalancersFunc != nil {
		return m.DescribeLoadBalancersFunc(ctx, params, optFns...)
	}
	return &elasticloadbalancingv2.DescribeLoadBalancersOutput{}, nil
}

func (m *mockELBV2Client) DescribeTags(ctx context.Context, params *elasticloadbalancingv2.DescribeTagsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTagsOutput, error) {
	if m.DescribeTagsFunc != nil {
		return m.DescribeTagsFunc(ctx, params, optFns...)
	}
	return &elasticloadbalancingv2.DescribeTagsOutput{}, nil
}

// ============================================================================
// Reconciler tests with mocked AWS clients
// ============================================================================

// TestReconciler_NoResources tests reconciliation when AWS has no resources.
func TestReconciler_NoResources(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ec2Mock := &mockEC2Client{}
	ecsMock := &mockECSClient{}
	elbMock := &mockELBV2Client{}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	result, err := r.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if len(result.OrphanedResources) != 0 {
		t.Errorf("Expected 0 orphaned resources, got %d", len(result.OrphanedResources))
	}

	if len(result.StaleLocalEntries) != 0 {
		t.Errorf("Expected 0 stale entries, got %d", len(result.StaleLocalEntries))
	}

	if result.SyncedCount != 0 {
		t.Errorf("Expected 0 synced count, got %d", result.SyncedCount)
	}
}

// TestReconciler_OrphanedVPC tests detection of orphaned VPCs.
func TestReconciler_OrphanedVPC(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Mock EC2 to return a tagged VPC that's not in local state
	ec2Mock := &mockEC2Client{
		DescribeVpcsFunc: func(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
			return &ec2.DescribeVpcsOutput{
				Vpcs: []ec2types.Vpc{
					{
						VpcId: aws.String("vpc-orphan-123"),
						Tags: []ec2types.Tag{
							{Key: aws.String("agent-deploy:created-by"), Value: aws.String("agent-deploy")},
							{Key: aws.String("agent-deploy:infra-id"), Value: aws.String("infra-missing")},
							{Key: aws.String("agent-deploy:deployment-id"), Value: aws.String("deploy-missing")},
						},
					},
				},
			}, nil
		},
	}
	ecsMock := &mockECSClient{}
	elbMock := &mockELBV2Client{}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	result, err := r.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if len(result.OrphanedResources) != 1 {
		t.Errorf("Expected 1 orphaned resource, got %d", len(result.OrphanedResources))
	}

	if len(result.OrphanedResources) > 0 {
		orphan := result.OrphanedResources[0]
		if orphan.ResourceType != "vpc" {
			t.Errorf("Expected resource type 'vpc', got %s", orphan.ResourceType)
		}
		if orphan.ResourceID != "vpc-orphan-123" {
			t.Errorf("Expected resource ID 'vpc-orphan-123', got %s", orphan.ResourceID)
		}
		if orphan.InfraID != "infra-missing" {
			t.Errorf("Expected infra ID 'infra-missing', got %s", orphan.InfraID)
		}
	}
}

// TestReconciler_OrphanedECSCluster tests detection of orphaned ECS clusters.
func TestReconciler_OrphanedECSCluster(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ec2Mock := &mockEC2Client{}
	ecsMock := &mockECSClient{
		ListClustersFunc: func(ctx context.Context, params *ecs.ListClustersInput, optFns ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
			return &ecs.ListClustersOutput{
				ClusterArns: []string{"arn:aws:ecs:us-east-1:123:cluster/orphan-cluster"},
			}, nil
		},
		DescribeClustersFunc: func(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
			return &ecs.DescribeClustersOutput{
				Clusters: []ecstypes.Cluster{
					{
						ClusterArn: aws.String("arn:aws:ecs:us-east-1:123:cluster/orphan-cluster"),
						Status:     aws.String("ACTIVE"),
						Tags: []ecstypes.Tag{
							{Key: aws.String("agent-deploy:created-by"), Value: aws.String("agent-deploy")},
							{Key: aws.String("agent-deploy:infra-id"), Value: aws.String("infra-missing")},
						},
					},
				},
			}, nil
		},
	}
	elbMock := &mockELBV2Client{}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	result, err := r.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if len(result.OrphanedResources) != 1 {
		t.Errorf("Expected 1 orphaned resource, got %d", len(result.OrphanedResources))
	}

	if len(result.OrphanedResources) > 0 {
		orphan := result.OrphanedResources[0]
		if orphan.ResourceType != "ecs_cluster" {
			t.Errorf("Expected resource type 'ecs_cluster', got %s", orphan.ResourceType)
		}
	}
}

// TestReconciler_OrphanedALB tests detection of orphaned ALBs.
func TestReconciler_OrphanedALB(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ec2Mock := &mockEC2Client{}
	ecsMock := &mockECSClient{}
	elbMock := &mockELBV2Client{
		DescribeLoadBalancersFunc: func(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
			return &elasticloadbalancingv2.DescribeLoadBalancersOutput{
				LoadBalancers: []elbv2types.LoadBalancer{
					{
						LoadBalancerArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/orphan/12345"),
					},
				},
			}, nil
		},
		DescribeTagsFunc: func(ctx context.Context, params *elasticloadbalancingv2.DescribeTagsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTagsOutput, error) {
			return &elasticloadbalancingv2.DescribeTagsOutput{
				TagDescriptions: []elbv2types.TagDescription{
					{
						ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/orphan/12345"),
						Tags: []elbv2types.Tag{
							{Key: aws.String("agent-deploy:created-by"), Value: aws.String("agent-deploy")},
							{Key: aws.String("agent-deploy:infra-id"), Value: aws.String("infra-missing")},
						},
					},
				},
			}, nil
		},
	}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	result, err := r.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if len(result.OrphanedResources) != 1 {
		t.Errorf("Expected 1 orphaned resource, got %d", len(result.OrphanedResources))
	}

	if len(result.OrphanedResources) > 0 {
		orphan := result.OrphanedResources[0]
		if orphan.ResourceType != "alb" {
			t.Errorf("Expected resource type 'alb', got %s", orphan.ResourceType)
		}
	}
}

// TestReconciler_SyncedResources tests resources properly tracked in both AWS and local state.
func TestReconciler_SyncedResources(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create local infra that matches AWS
	infra := &Infrastructure{
		ID:     "infra-synced",
		PlanID: "plan-1",
		Region: "us-east-1",
		Resources: map[string]string{
			ResourceVPC:        "vpc-synced-123",
			ResourceECSCluster: "arn:aws:ecs:us-east-1:123:cluster/synced",
			ResourceALB:        "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/synced/12345",
		},
		Status:    InfraStatusReady,
		CreatedAt: time.Now(),
	}
	if err := store.CreateInfra(infra); err != nil {
		t.Fatalf("Failed to create infra: %v", err)
	}

	// Mock AWS to return the same VPC (tagged with matching infra ID)
	ec2Mock := &mockEC2Client{
		DescribeVpcsFunc: func(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
			// If querying specific VPCs, return them
			if len(params.VpcIds) > 0 {
				var vpcs []ec2types.Vpc
				for _, id := range params.VpcIds {
					vpcs = append(vpcs, ec2types.Vpc{VpcId: aws.String(id)})
				}
				return &ec2.DescribeVpcsOutput{Vpcs: vpcs}, nil
			}
			// Tagged VPCs query - return synced VPC
			return &ec2.DescribeVpcsOutput{
				Vpcs: []ec2types.Vpc{
					{
						VpcId: aws.String("vpc-synced-123"),
						Tags: []ec2types.Tag{
							{Key: aws.String("agent-deploy:created-by"), Value: aws.String("agent-deploy")},
							{Key: aws.String("agent-deploy:infra-id"), Value: aws.String("infra-synced")},
						},
					},
				},
			}, nil
		},
	}
	ecsMock := &mockECSClient{
		ListClustersFunc: func(ctx context.Context, params *ecs.ListClustersInput, optFns ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
			return &ecs.ListClustersOutput{
				ClusterArns: []string{"arn:aws:ecs:us-east-1:123:cluster/synced"},
			}, nil
		},
		DescribeClustersFunc: func(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
			return &ecs.DescribeClustersOutput{
				Clusters: []ecstypes.Cluster{
					{
						ClusterArn: aws.String("arn:aws:ecs:us-east-1:123:cluster/synced"),
						Status:     aws.String("ACTIVE"),
						Tags: []ecstypes.Tag{
							{Key: aws.String("agent-deploy:created-by"), Value: aws.String("agent-deploy")},
							{Key: aws.String("agent-deploy:infra-id"), Value: aws.String("infra-synced")},
						},
					},
				},
			}, nil
		},
	}
	elbMock := &mockELBV2Client{
		DescribeLoadBalancersFunc: func(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
			// If querying specific ALBs, return them
			if len(params.LoadBalancerArns) > 0 {
				var lbs []elbv2types.LoadBalancer
				for _, arn := range params.LoadBalancerArns {
					lbs = append(lbs, elbv2types.LoadBalancer{LoadBalancerArn: aws.String(arn)})
				}
				return &elasticloadbalancingv2.DescribeLoadBalancersOutput{LoadBalancers: lbs}, nil
			}
			return &elasticloadbalancingv2.DescribeLoadBalancersOutput{
				LoadBalancers: []elbv2types.LoadBalancer{
					{LoadBalancerArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/synced/12345")},
				},
			}, nil
		},
		DescribeTagsFunc: func(ctx context.Context, params *elasticloadbalancingv2.DescribeTagsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTagsOutput, error) {
			return &elasticloadbalancingv2.DescribeTagsOutput{
				TagDescriptions: []elbv2types.TagDescription{
					{
						ResourceArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/synced/12345"),
						Tags: []elbv2types.Tag{
							{Key: aws.String("agent-deploy:created-by"), Value: aws.String("agent-deploy")},
							{Key: aws.String("agent-deploy:infra-id"), Value: aws.String("infra-synced")},
						},
					},
				},
			}, nil
		},
	}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	result, err := r.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// No orphans - resources are tracked
	if len(result.OrphanedResources) != 0 {
		t.Errorf("Expected 0 orphaned resources, got %d", len(result.OrphanedResources))
	}

	// Synced count should be 3 (VPC, ECS cluster, ALB)
	if result.SyncedCount != 3 {
		t.Errorf("Expected 3 synced resources, got %d", result.SyncedCount)
	}
}

// TestReconciler_StaleInfra tests detection of stale infrastructure entries.
func TestReconciler_StaleInfra(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create local infra pointing to non-existent VPC
	infra := &Infrastructure{
		ID:     "infra-stale",
		PlanID: "plan-1",
		Region: "us-east-1",
		Resources: map[string]string{
			ResourceVPC: "vpc-deleted-123",
		},
		Status:    InfraStatusReady,
		CreatedAt: time.Now(),
	}
	if err := store.CreateInfra(infra); err != nil {
		t.Fatalf("Failed to create infra: %v", err)
	}

	// Mock EC2 to return "not found" for the VPC
	ec2Mock := &mockEC2Client{
		DescribeVpcsFunc: func(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
			if len(params.VpcIds) > 0 {
				return nil, fmt.Errorf("InvalidVpcID.NotFound: VPC not found")
			}
			return &ec2.DescribeVpcsOutput{}, nil
		},
	}
	ecsMock := &mockECSClient{}
	elbMock := &mockELBV2Client{}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	result, err := r.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if len(result.StaleLocalEntries) != 1 {
		t.Errorf("Expected 1 stale entry, got %d", len(result.StaleLocalEntries))
	}

	if len(result.StaleLocalEntries) > 0 {
		stale := result.StaleLocalEntries[0]
		if stale.EntryType != "infra" {
			t.Errorf("Expected entry type 'infra', got %s", stale.EntryType)
		}
		if stale.EntryID != "infra-stale" {
			t.Errorf("Expected entry ID 'infra-stale', got %s", stale.EntryID)
		}
		if len(stale.MissingResources) == 0 || stale.MissingResources[0] != "vpc" {
			t.Errorf("Expected missing resource 'vpc', got %v", stale.MissingResources)
		}
	}
}

// TestReconciler_StaleDeployment tests detection of stale deployment entries.
func TestReconciler_StaleDeployment(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create deployment pointing to non-existent infra
	deploy := &Deployment{
		ID:          "deploy-stale",
		InfraID:     "infra-nonexistent",
		ImageRef:    "nginx:latest",
		Status:      DeploymentStatusRunning,
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}
	if err := store.CreateDeployment(deploy); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	ec2Mock := &mockEC2Client{}
	ecsMock := &mockECSClient{}
	elbMock := &mockELBV2Client{}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	result, err := r.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if len(result.StaleLocalEntries) != 1 {
		t.Errorf("Expected 1 stale entry, got %d", len(result.StaleLocalEntries))
	}

	if len(result.StaleLocalEntries) > 0 {
		stale := result.StaleLocalEntries[0]
		if stale.EntryType != "deployment" {
			t.Errorf("Expected entry type 'deployment', got %s", stale.EntryType)
		}
		if stale.EntryID != "deploy-stale" {
			t.Errorf("Expected entry ID 'deploy-stale', got %s", stale.EntryID)
		}
	}
}

// TestReconciler_CleanupStaleEntries tests the CleanupStaleEntries function.
func TestReconciler_CleanupStaleEntries(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create stale infra and deployment
	infra := &Infrastructure{
		ID:        "infra-to-cleanup",
		PlanID:    "plan-1",
		Region:    "us-east-1",
		Status:    InfraStatusReady,
		CreatedAt: time.Now(),
	}
	if err := store.CreateInfra(infra); err != nil {
		t.Fatalf("Failed to create infra: %v", err)
	}

	deploy := &Deployment{
		ID:          "deploy-to-cleanup",
		InfraID:     "infra-to-cleanup",
		Status:      DeploymentStatusRunning,
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}
	if err := store.CreateDeployment(deploy); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	ec2Mock := &mockEC2Client{}
	ecsMock := &mockECSClient{}
	elbMock := &mockELBV2Client{}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	staleEntries := []StaleEntry{
		{EntryType: "infra", EntryID: "infra-to-cleanup"},
		{EntryType: "deployment", EntryID: "deploy-to-cleanup"},
	}

	cleaned, err := r.CleanupStaleEntries(ctx, staleEntries)
	if err != nil {
		t.Fatalf("CleanupStaleEntries failed: %v", err)
	}

	if cleaned != 2 {
		t.Errorf("Expected 2 cleaned entries, got %d", cleaned)
	}

	// Verify infra is marked as destroyed
	updatedInfra, err := store.GetInfra("infra-to-cleanup")
	if err != nil {
		t.Fatalf("Failed to get infra: %v", err)
	}
	if updatedInfra.Status != InfraStatusDestroyed {
		t.Errorf("Expected infra status 'destroyed', got %s", updatedInfra.Status)
	}

	// Verify deployment is marked as stopped
	updatedDeploy, err := store.GetDeployment("deploy-to-cleanup")
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}
	if updatedDeploy.Status != DeploymentStatusStopped {
		t.Errorf("Expected deployment status 'stopped', got %s", updatedDeploy.Status)
	}
}

// TestReconciler_VpcExists tests the vpcExists helper function.
func TestReconciler_VpcExists(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	tests := []struct {
		name        string
		vpcID       string
		mockResp    func(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
		wantExists  bool
		wantErr     bool
	}{
		{
			name:  "VPC exists",
			vpcID: "vpc-existing",
			mockResp: func(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
				return &ec2.DescribeVpcsOutput{
					Vpcs: []ec2types.Vpc{{VpcId: aws.String("vpc-existing")}},
				}, nil
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:  "VPC not found",
			vpcID: "vpc-notfound",
			mockResp: func(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
				return nil, fmt.Errorf("InvalidVpcID.NotFound: The vpc ID 'vpc-notfound' does not exist")
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name:  "AWS error",
			vpcID: "vpc-error",
			mockResp: func(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
				return nil, fmt.Errorf("AccessDenied: You don't have permission")
			},
			wantExists: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ec2Mock := &mockEC2Client{DescribeVpcsFunc: tt.mockResp}
			r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, &mockECSClient{}, &mockELBV2Client{})

			ctx := context.Background()
			exists, err := r.vpcExists(ctx, tt.vpcID)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if exists != tt.wantExists {
				t.Errorf("Expected exists=%v, got %v", tt.wantExists, exists)
			}
		})
	}
}

// TestReconciler_EcsClusterExists tests the ecsClusterExists helper function.
func TestReconciler_EcsClusterExists(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	tests := []struct {
		name        string
		clusterARN  string
		mockResp    func(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)
		wantExists  bool
		wantErr     bool
	}{
		{
			name:       "Cluster exists and active",
			clusterARN: "arn:aws:ecs:us-east-1:123:cluster/active",
			mockResp: func(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
				return &ecs.DescribeClustersOutput{
					Clusters: []ecstypes.Cluster{
						{ClusterArn: aws.String("arn:aws:ecs:us-east-1:123:cluster/active"), Status: aws.String("ACTIVE")},
					},
				}, nil
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:       "Cluster inactive",
			clusterARN: "arn:aws:ecs:us-east-1:123:cluster/inactive",
			mockResp: func(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
				return &ecs.DescribeClustersOutput{
					Clusters: []ecstypes.Cluster{
						{ClusterArn: aws.String("arn:aws:ecs:us-east-1:123:cluster/inactive"), Status: aws.String("INACTIVE")},
					},
				}, nil
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name:       "Cluster not found",
			clusterARN: "arn:aws:ecs:us-east-1:123:cluster/notfound",
			mockResp: func(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
				return &ecs.DescribeClustersOutput{Clusters: []ecstypes.Cluster{}}, nil
			},
			wantExists: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecsMock := &mockECSClient{DescribeClustersFunc: tt.mockResp}
			r := NewReconcilerWithClients(store, "us-east-1", &mockEC2Client{}, ecsMock, &mockELBV2Client{})

			ctx := context.Background()
			exists, err := r.ecsClusterExists(ctx, tt.clusterARN)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if exists != tt.wantExists {
				t.Errorf("Expected exists=%v, got %v", tt.wantExists, exists)
			}
		})
	}
}

// TestReconciler_AlbExists tests the albExists helper function.
func TestReconciler_AlbExists(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	tests := []struct {
		name       string
		albARN     string
		mockResp   func(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error)
		wantExists bool
		wantErr    bool
	}{
		{
			name:   "ALB exists",
			albARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/exists/12345",
			mockResp: func(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
				return &elasticloadbalancingv2.DescribeLoadBalancersOutput{
					LoadBalancers: []elbv2types.LoadBalancer{{LoadBalancerArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/exists/12345")}},
				}, nil
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:   "ALB not found",
			albARN: "arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/notfound/12345",
			mockResp: func(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
				return nil, fmt.Errorf("LoadBalancerNotFound: Load balancer not found")
			},
			wantExists: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elbMock := &mockELBV2Client{DescribeLoadBalancersFunc: tt.mockResp}
			r := NewReconcilerWithClients(store, "us-east-1", &mockEC2Client{}, &mockECSClient{}, elbMock)

			ctx := context.Background()
			exists, err := r.albExists(ctx, tt.albARN)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if exists != tt.wantExists {
				t.Errorf("Expected exists=%v, got %v", tt.wantExists, exists)
			}
		})
	}
}

// TestReconciler_EcsServiceExists tests the ecsServiceExists helper function.
func TestReconciler_EcsServiceExists(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	tests := []struct {
		name       string
		clusterARN string
		serviceARN string
		mockResp   func(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
		wantExists bool
		wantErr    bool
	}{
		{
			name:       "Service exists and active",
			clusterARN: "arn:aws:ecs:us-east-1:123:cluster/test",
			serviceARN: "arn:aws:ecs:us-east-1:123:service/test/active-service",
			mockResp: func(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
				return &ecs.DescribeServicesOutput{
					Services: []ecstypes.Service{
						{ServiceArn: aws.String("arn:aws:ecs:us-east-1:123:service/test/active-service"), Status: aws.String("ACTIVE")},
					},
				}, nil
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:       "Service inactive",
			clusterARN: "arn:aws:ecs:us-east-1:123:cluster/test",
			serviceARN: "arn:aws:ecs:us-east-1:123:service/test/inactive-service",
			mockResp: func(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
				return &ecs.DescribeServicesOutput{
					Services: []ecstypes.Service{
						{ServiceArn: aws.String("arn:aws:ecs:us-east-1:123:service/test/inactive-service"), Status: aws.String("INACTIVE")},
					},
				}, nil
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name:       "Empty cluster ARN",
			clusterARN: "",
			serviceARN: "arn:aws:ecs:us-east-1:123:service/test/service",
			mockResp:   nil,
			wantExists: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecsMock := &mockECSClient{DescribeServicesFunc: tt.mockResp}
			r := NewReconcilerWithClients(store, "us-east-1", &mockEC2Client{}, ecsMock, &mockELBV2Client{})

			ctx := context.Background()
			exists, err := r.ecsServiceExists(ctx, tt.clusterARN, tt.serviceARN)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if exists != tt.wantExists {
				t.Errorf("Expected exists=%v, got %v", tt.wantExists, exists)
			}
		})
	}
}

// TestReconciler_Pagination tests that pagination works correctly.
func TestReconciler_Pagination(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Mock ECS to return paginated cluster results
	callCount := 0
	ecsMock := &mockECSClient{
		ListClustersFunc: func(ctx context.Context, params *ecs.ListClustersInput, optFns ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
			callCount++
			if callCount == 1 {
				// First page
				token := "page2"
				return &ecs.ListClustersOutput{
					ClusterArns: []string{
						"arn:aws:ecs:us-east-1:123:cluster/cluster-1",
						"arn:aws:ecs:us-east-1:123:cluster/cluster-2",
					},
					NextToken: &token,
				}, nil
			}
			// Second page (last)
			return &ecs.ListClustersOutput{
				ClusterArns: []string{
					"arn:aws:ecs:us-east-1:123:cluster/cluster-3",
				},
			}, nil
		},
		DescribeClustersFunc: func(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
			var clusters []ecstypes.Cluster
			for _, arn := range params.Clusters {
				clusters = append(clusters, ecstypes.Cluster{
					ClusterArn: aws.String(arn),
					Status:     aws.String("ACTIVE"),
					// No agent-deploy tag - not our clusters
				})
			}
			return &ecs.DescribeClustersOutput{Clusters: clusters}, nil
		},
	}

	ec2Mock := &mockEC2Client{}
	elbMock := &mockELBV2Client{}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	_, err = r.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify pagination was used (should have called ListClusters twice)
	if callCount != 2 {
		t.Errorf("Expected 2 ListClusters calls for pagination, got %d", callCount)
	}
}

// TestReconciler_BatchTagFetching tests that ALB tags are fetched in batches.
func TestReconciler_BatchTagFetching(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create 25 ALBs to test batching (batch size is 20)
	var albARNs []string
	for i := 0; i < 25; i++ {
		albARNs = append(albARNs, fmt.Sprintf("arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/alb-%d/12345", i))
	}

	describeTagsCalls := 0
	elbMock := &mockELBV2Client{
		DescribeLoadBalancersFunc: func(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
			var lbs []elbv2types.LoadBalancer
			for _, arn := range albARNs {
				lbs = append(lbs, elbv2types.LoadBalancer{LoadBalancerArn: aws.String(arn)})
			}
			return &elasticloadbalancingv2.DescribeLoadBalancersOutput{LoadBalancers: lbs}, nil
		},
		DescribeTagsFunc: func(ctx context.Context, params *elasticloadbalancingv2.DescribeTagsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTagsOutput, error) {
			describeTagsCalls++
			var tagDescs []elbv2types.TagDescription
			for _, arn := range params.ResourceArns {
				tagDescs = append(tagDescs, elbv2types.TagDescription{
					ResourceArn: aws.String(arn),
					Tags:        []elbv2types.Tag{}, // No agent-deploy tags
				})
			}
			return &elasticloadbalancingv2.DescribeTagsOutput{TagDescriptions: tagDescs}, nil
		},
	}

	ec2Mock := &mockEC2Client{}
	ecsMock := &mockECSClient{}

	r := NewReconcilerWithClients(store, "us-east-1", ec2Mock, ecsMock, elbMock)

	ctx := context.Background()
	_, err = r.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// 25 ALBs should require 2 batches (20 + 5)
	if describeTagsCalls != 2 {
		t.Errorf("Expected 2 DescribeTags calls for batching, got %d", describeTagsCalls)
	}
}

// TestGetTagValue tests the getTagValue helper function.
func TestGetTagValue(t *testing.T) {
	tests := []struct {
		name   string
		tags   []ec2types.Tag
		key    string
		want   string
	}{
		{
			name: "Key exists",
			tags: []ec2types.Tag{
				{Key: aws.String("agent-deploy:infra-id"), Value: aws.String("infra-123")},
			},
			key:  "agent-deploy:infra-id",
			want: "infra-123",
		},
		{
			name: "Key not found",
			tags: []ec2types.Tag{
				{Key: aws.String("other-key"), Value: aws.String("value")},
			},
			key:  "agent-deploy:infra-id",
			want: "",
		},
		{
			name: "Empty tags",
			tags: []ec2types.Tag{},
			key:  "agent-deploy:infra-id",
			want: "",
		},
		{
			name: "Multiple tags",
			tags: []ec2types.Tag{
				{Key: aws.String("agent-deploy:created-by"), Value: aws.String("agent-deploy")},
				{Key: aws.String("agent-deploy:infra-id"), Value: aws.String("infra-456")},
				{Key: aws.String("agent-deploy:plan-id"), Value: aws.String("plan-789")},
			},
			key:  "agent-deploy:infra-id",
			want: "infra-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTagValue(tt.tags, tt.key)
			if got != tt.want {
				t.Errorf("getTagValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
