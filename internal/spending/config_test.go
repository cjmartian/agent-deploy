package spending

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultLimits verifies the default spending limits.
func TestDefaultLimits(t *testing.T) {
	limits := DefaultLimits()

	if limits.MonthlyBudgetUSD != 100.0 {
		t.Errorf("MonthlyBudgetUSD = %v, want 100.0", limits.MonthlyBudgetUSD)
	}
	if limits.PerDeploymentUSD != 25.0 {
		t.Errorf("PerDeploymentUSD = %v, want 25.0", limits.PerDeploymentUSD)
	}
	if limits.AlertThresholdPercent != 80 {
		t.Errorf("AlertThresholdPercent = %v, want 80", limits.AlertThresholdPercent)
	}
}

// TestLoadLimits_EnvVars tests that environment variables override defaults.
func TestLoadLimits_EnvVars(t *testing.T) {
	// Set environment variables for this test.
	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "200.50")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "50.25")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "90")

	limits, err := LoadLimits()
	if err != nil {
		t.Fatalf("LoadLimits: %v", err)
	}

	if limits.MonthlyBudgetUSD != 200.50 {
		t.Errorf("MonthlyBudgetUSD = %v, want 200.50", limits.MonthlyBudgetUSD)
	}
	if limits.PerDeploymentUSD != 50.25 {
		t.Errorf("PerDeploymentUSD = %v, want 50.25", limits.PerDeploymentUSD)
	}
	if limits.AlertThresholdPercent != 90 {
		t.Errorf("AlertThresholdPercent = %v, want 90", limits.AlertThresholdPercent)
	}
}

// TestLoadLimits_InvalidEnvVarFallsBackToDefault tests that invalid env vars are ignored.
func TestLoadLimits_InvalidEnvVarFallsBackToDefault(t *testing.T) {
	// Use temp HOME to avoid picking up real config file.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "not-a-number")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "invalid")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "abc")

	limits, err := LoadLimits()
	if err != nil {
		t.Fatalf("LoadLimits: %v", err)
	}

	defaults := DefaultLimits()
	if limits.MonthlyBudgetUSD != defaults.MonthlyBudgetUSD {
		t.Errorf("MonthlyBudgetUSD = %v, want %v (default)", limits.MonthlyBudgetUSD, defaults.MonthlyBudgetUSD)
	}
	if limits.PerDeploymentUSD != defaults.PerDeploymentUSD {
		t.Errorf("PerDeploymentUSD = %v, want %v (default)", limits.PerDeploymentUSD, defaults.PerDeploymentUSD)
	}
	if limits.AlertThresholdPercent != defaults.AlertThresholdPercent {
		t.Errorf("AlertThresholdPercent = %v, want %v (default)", limits.AlertThresholdPercent, defaults.AlertThresholdPercent)
	}
}

// TestLoadLimits_NoEnvVars tests that defaults are used when no env vars are set.
func TestLoadLimits_NoEnvVars(t *testing.T) {
	// Use temp HOME to avoid picking up real config file.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Ensure no env vars are set by using t.Setenv with empty string.
	// This is preferred over os.Unsetenv which is harder to track.
	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "")

	limits, err := LoadLimits()
	if err != nil {
		t.Fatalf("LoadLimits: %v", err)
	}

	defaults := DefaultLimits()
	if limits != defaults {
		t.Errorf("LoadLimits() = %+v, want %+v (defaults)", limits, defaults)
	}
}

// TestLoadLimits_PartialEnvVars tests that only specified env vars override defaults.
func TestLoadLimits_PartialEnvVars(t *testing.T) {
	// WHY: Isolate HOME to prevent real config file from affecting test.
	// Without this, ~/.agent-deploy/config.json may override defaults.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "75.00")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "")

	limits, err := LoadLimits()
	if err != nil {
		t.Fatalf("LoadLimits: %v", err)
	}

	if limits.MonthlyBudgetUSD != 100.0 {
		t.Errorf("MonthlyBudgetUSD = %v, want 100.0 (default)", limits.MonthlyBudgetUSD)
	}
	if limits.PerDeploymentUSD != 75.00 {
		t.Errorf("PerDeploymentUSD = %v, want 75.00", limits.PerDeploymentUSD)
	}
	if limits.AlertThresholdPercent != 80 {
		t.Errorf("AlertThresholdPercent = %v, want 80 (default)", limits.AlertThresholdPercent)
	}
}

// TestLoadLimits_ConfigFile tests loading limits from a config file.
func TestLoadLimits_ConfigFile(t *testing.T) {
	// Create a temporary home directory with config file.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDir := filepath.Join(tmpHome, ".agent-deploy")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	config := Config{
		SpendingLimits: Limits{
			MonthlyBudgetUSD:      500.0,
			PerDeploymentUSD:      100.0,
			AlertThresholdPercent: 75,
		},
	}
	configData, _ := json.Marshal(config)
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Clear any env vars that might override.
	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "")

	limits, err := LoadLimits()
	if err != nil {
		t.Fatalf("LoadLimits: %v", err)
	}

	if limits.MonthlyBudgetUSD != 500.0 {
		t.Errorf("MonthlyBudgetUSD = %v, want 500.0", limits.MonthlyBudgetUSD)
	}
	if limits.PerDeploymentUSD != 100.0 {
		t.Errorf("PerDeploymentUSD = %v, want 100.0", limits.PerDeploymentUSD)
	}
	if limits.AlertThresholdPercent != 75 {
		t.Errorf("AlertThresholdPercent = %v, want 75", limits.AlertThresholdPercent)
	}
}

// TestLoadLimits_EnvOverridesConfigFile tests that env vars take precedence over config file.
func TestLoadLimits_EnvOverridesConfigFile(t *testing.T) {
	// Create a temporary home directory with config file.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDir := filepath.Join(tmpHome, ".agent-deploy")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	config := Config{
		SpendingLimits: Limits{
			MonthlyBudgetUSD:      500.0,
			PerDeploymentUSD:      100.0,
			AlertThresholdPercent: 75,
		},
	}
	configData, _ := json.Marshal(config)
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Set env var to override config file.
	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "999.0")

	limits, err := LoadLimits()
	if err != nil {
		t.Fatalf("LoadLimits: %v", err)
	}

	if limits.MonthlyBudgetUSD != 999.0 {
		t.Errorf("MonthlyBudgetUSD = %v, want 999.0 (from env, not config)", limits.MonthlyBudgetUSD)
	}
	// These should come from config file.
	if limits.PerDeploymentUSD != 100.0 {
		t.Errorf("PerDeploymentUSD = %v, want 100.0 (from config)", limits.PerDeploymentUSD)
	}
	if limits.AlertThresholdPercent != 75 {
		t.Errorf("AlertThresholdPercent = %v, want 75 (from config)", limits.AlertThresholdPercent)
	}
}

// TestLoadLimits_MissingConfigFileNoError tests that missing config file doesn't error.
func TestLoadLimits_MissingConfigFileNoError(t *testing.T) {
	// Create a temporary home directory without config file.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "")

	limits, err := LoadLimits()
	if err != nil {
		t.Fatalf("LoadLimits: %v", err)
	}

	// Should get defaults since no config file.
	defaults := DefaultLimits()
	if limits != defaults {
		t.Errorf("LoadLimits() = %+v, want %+v (defaults)", limits, defaults)
	}
}

// TestLoadLimits_InvalidConfigFile tests that invalid config file is ignored.
func TestLoadLimits_InvalidConfigFile(t *testing.T) {
	// Create a temporary home directory with invalid config file.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDir := filepath.Join(tmpHome, ".agent-deploy")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write invalid JSON.
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "")

	limits, err := LoadLimits()
	if err != nil {
		t.Fatalf("LoadLimits: %v", err)
	}

	// Should get defaults since config file is invalid.
	defaults := DefaultLimits()
	if limits != defaults {
		t.Errorf("LoadLimits() = %+v, want %+v (defaults)", limits, defaults)
	}
}

// TestLimits_JSONSerialization tests that Limits can be serialized/deserialized.
func TestLimits_JSONSerialization(t *testing.T) {
	original := Limits{
		MonthlyBudgetUSD:      150.0,
		PerDeploymentUSD:      30.0,
		AlertThresholdPercent: 85,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Limits
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded != original {
		t.Errorf("Decoded = %+v, want %+v", decoded, original)
	}
}

// TestLoadLimitsWithSource_NoConfig tests that ExplicitlyConfigured is false when no config exists.
// WHY: P1.36 - Spec requires confirmation when using defaults, not explicit user config.
func TestLoadLimitsWithSource_NoConfig(t *testing.T) {
	// Create a temporary home directory without config file.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Clear env vars.
	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "")

	result, err := LoadLimitsWithSource()
	if err != nil {
		t.Fatalf("LoadLimitsWithSource: %v", err)
	}

	if result.ExplicitlyConfigured {
		t.Error("ExplicitlyConfigured should be false when no config exists")
	}

	// Should still get default limits.
	defaults := DefaultLimits()
	if result.Limits != defaults {
		t.Errorf("Limits = %+v, want %+v (defaults)", result.Limits, defaults)
	}
}

// TestLoadLimitsWithSource_WithConfigFile tests that ExplicitlyConfigured is true when config file exists.
func TestLoadLimitsWithSource_WithConfigFile(t *testing.T) {
	// Create a temporary home directory with config file.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDir := filepath.Join(tmpHome, ".agent-deploy")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	config := Config{
		SpendingLimits: Limits{
			MonthlyBudgetUSD:      500.0,
			PerDeploymentUSD:      100.0,
			AlertThresholdPercent: 75,
		},
	}
	configData, _ := json.Marshal(config)
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Clear env vars.
	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "")

	result, err := LoadLimitsWithSource()
	if err != nil {
		t.Fatalf("LoadLimitsWithSource: %v", err)
	}

	if !result.ExplicitlyConfigured {
		t.Error("ExplicitlyConfigured should be true when config file exists")
	}
}

// TestLoadLimitsWithSource_WithEnvVars tests that ExplicitlyConfigured is true when env vars are set.
func TestLoadLimitsWithSource_WithEnvVars(t *testing.T) {
	// Create a temporary home directory without config file.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Set env var.
	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "200")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "")

	result, err := LoadLimitsWithSource()
	if err != nil {
		t.Fatalf("LoadLimitsWithSource: %v", err)
	}

	if !result.ExplicitlyConfigured {
		t.Error("ExplicitlyConfigured should be true when env vars are set")
	}

	if result.Limits.MonthlyBudgetUSD != 200.0 {
		t.Errorf("MonthlyBudgetUSD = %v, want 200.0", result.Limits.MonthlyBudgetUSD)
	}
}
