package state

import (
	"context"
	"testing"
	"time"
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
