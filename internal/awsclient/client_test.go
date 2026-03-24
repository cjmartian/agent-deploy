// Package awsclient tests verify AWS SDK configuration and resource tagging.
package awsclient

import (
	"context"
	"testing"
)

// TestResourceTags_AllFields tests ResourceTags with all IDs provided.
func TestResourceTags_AllFields(t *testing.T) {
	tags := ResourceTags("plan-123", "infra-456", "deploy-789")

	expected := map[string]string{
		"agent-deploy:created-by":    "agent-deploy",
		"agent-deploy:plan-id":       "plan-123",
		"agent-deploy:infra-id":      "infra-456",
		"agent-deploy:deployment-id": "deploy-789",
	}

	if len(tags) != len(expected) {
		t.Errorf("ResourceTags returned %d tags, want %d", len(tags), len(expected))
	}

	for k, v := range expected {
		if tags[k] != v {
			t.Errorf("tags[%q] = %q, want %q", k, tags[k], v)
		}
	}
}

// TestResourceTags_PartialFields tests ResourceTags with only some IDs provided.
func TestResourceTags_PartialFields(t *testing.T) {
	tags := ResourceTags("plan-123", "", "")

	// Should have created-by and plan-id only.
	if len(tags) != 2 {
		t.Errorf("ResourceTags returned %d tags, want 2", len(tags))
	}

	if tags["agent-deploy:created-by"] != "agent-deploy" {
		t.Errorf("created-by = %q, want %q", tags["agent-deploy:created-by"], "agent-deploy")
	}
	if tags["agent-deploy:plan-id"] != "plan-123" {
		t.Errorf("plan-id = %q, want %q", tags["agent-deploy:plan-id"], "plan-123")
	}
	if _, ok := tags["agent-deploy:infra-id"]; ok {
		t.Error("infra-id should not be present when empty")
	}
	if _, ok := tags["agent-deploy:deployment-id"]; ok {
		t.Error("deployment-id should not be present when empty")
	}
}

// TestResourceTags_NoFields tests ResourceTags with no IDs (only created-by tag).
func TestResourceTags_NoFields(t *testing.T) {
	tags := ResourceTags("", "", "")

	// Should only have created-by.
	if len(tags) != 1 {
		t.Errorf("ResourceTags returned %d tags, want 1", len(tags))
	}

	if tags["agent-deploy:created-by"] != "agent-deploy" {
		t.Errorf("created-by = %q, want %q", tags["agent-deploy:created-by"], "agent-deploy")
	}
}

// TestResourceTags_PlanOnly tests ResourceTags with only plan ID.
func TestResourceTags_PlanOnly(t *testing.T) {
	tags := ResourceTags("plan-abc", "", "")

	if len(tags) != 2 {
		t.Errorf("ResourceTags returned %d tags, want 2", len(tags))
	}
	if tags["agent-deploy:plan-id"] != "plan-abc" {
		t.Errorf("plan-id = %q, want %q", tags["agent-deploy:plan-id"], "plan-abc")
	}
}

// TestResourceTags_InfraOnly tests ResourceTags with only infra ID.
func TestResourceTags_InfraOnly(t *testing.T) {
	tags := ResourceTags("", "infra-xyz", "")

	if len(tags) != 2 {
		t.Errorf("ResourceTags returned %d tags, want 2", len(tags))
	}
	if tags["agent-deploy:infra-id"] != "infra-xyz" {
		t.Errorf("infra-id = %q, want %q", tags["agent-deploy:infra-id"], "infra-xyz")
	}
}

// TestResourceTags_DeploymentOnly tests ResourceTags with only deployment ID.
func TestResourceTags_DeploymentOnly(t *testing.T) {
	tags := ResourceTags("", "", "deploy-123")

	if len(tags) != 2 {
		t.Errorf("ResourceTags returned %d tags, want 2", len(tags))
	}
	if tags["agent-deploy:deployment-id"] != "deploy-123" {
		t.Errorf("deployment-id = %q, want %q", tags["agent-deploy:deployment-id"], "deploy-123")
	}
}

// TestLoadConfig_InvalidRegion tests LoadConfig behavior.
// Note: This test verifies the function signature and basic behavior.
// Full integration testing requires AWS credentials.
func TestLoadConfig_ValidRegion(t *testing.T) {
	// This test verifies the function can be called without panicking.
	// It may fail if no AWS credentials are configured, which is expected
	// in a test environment without AWS access.
	ctx := context.Background()
	cfg, err := LoadConfig(ctx, "us-east-1")

	// We don't assert on error here because the test environment
	// may or may not have AWS credentials configured.
	// The important thing is that the function doesn't panic.
	_ = cfg
	_ = err
}

// TestLoadConfig_EmptyRegion tests LoadConfig with empty region.
// AWS SDK will use default region from environment or credentials file.
func TestLoadConfig_EmptyRegion(t *testing.T) {
	ctx := context.Background()
	cfg, err := LoadConfig(ctx, "")

	// The SDK should still work with empty region (uses default).
	// We just verify it doesn't panic.
	_ = cfg
	_ = err
}
