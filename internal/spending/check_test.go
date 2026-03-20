package spending

import (
	"os"
	"testing"
)

func TestCheckBudget_Allowed(t *testing.T) {
	limits := Limits{
		MonthlyBudgetUSD:      100.0,
		PerDeploymentUSD:      25.0,
		AlertThresholdPercent: 80,
	}

	result := CheckBudget(20.0, limits, 50.0)
	if !result.Allowed {
		t.Errorf("CheckBudget should allow $20/mo (per-deploy: $25, remaining: $50)")
	}
	if result.RemainingBudget != 50.0 {
		t.Errorf("RemainingBudget = %.2f, want 50.00", result.RemainingBudget)
	}
}

func TestCheckBudget_ExceedsPerDeployment(t *testing.T) {
	limits := Limits{
		MonthlyBudgetUSD: 100.0,
		PerDeploymentUSD: 25.0,
	}

	result := CheckBudget(30.0, limits, 0.0)
	if result.Allowed {
		t.Error("CheckBudget should block $30/mo when per-deployment limit is $25")
	}
	if result.Reason == "" {
		t.Error("CheckBudget should provide a reason when blocked")
	}
}

func TestCheckBudget_ExceedsRemainingMonthly(t *testing.T) {
	limits := Limits{
		MonthlyBudgetUSD: 100.0,
		PerDeploymentUSD: 50.0,
	}

	result := CheckBudget(30.0, limits, 80.0) // Only $20 remaining.
	if result.Allowed {
		t.Error("CheckBudget should block $30/mo when only $20 remaining")
	}
	if result.RemainingBudget != 20.0 {
		t.Errorf("RemainingBudget = %.2f, want 20.00", result.RemainingBudget)
	}
}

func TestLoadLimits_Defaults(t *testing.T) {
	// Clear any env vars that might interfere.
	_ = os.Unsetenv("AGENT_DEPLOY_MONTHLY_BUDGET")
	_ = os.Unsetenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET")
	_ = os.Unsetenv("AGENT_DEPLOY_ALERT_THRESHOLD")

	limits, err := LoadLimits()
	if err != nil {
		t.Fatalf("LoadLimits: %v", err)
	}

	defaults := DefaultLimits()
	if limits.MonthlyBudgetUSD != defaults.MonthlyBudgetUSD {
		t.Errorf("MonthlyBudgetUSD = %.2f, want %.2f", limits.MonthlyBudgetUSD, defaults.MonthlyBudgetUSD)
	}
	if limits.PerDeploymentUSD != defaults.PerDeploymentUSD {
		t.Errorf("PerDeploymentUSD = %.2f, want %.2f", limits.PerDeploymentUSD, defaults.PerDeploymentUSD)
	}
}

func TestLoadLimits_EnvOverrides(t *testing.T) {
	t.Setenv("AGENT_DEPLOY_MONTHLY_BUDGET", "200.0")
	t.Setenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET", "50.0")
	t.Setenv("AGENT_DEPLOY_ALERT_THRESHOLD", "90")

	limits, _ := LoadLimits()

	if limits.MonthlyBudgetUSD != 200.0 {
		t.Errorf("MonthlyBudgetUSD = %.2f, want 200.00", limits.MonthlyBudgetUSD)
	}
	if limits.PerDeploymentUSD != 50.0 {
		t.Errorf("PerDeploymentUSD = %.2f, want 50.00", limits.PerDeploymentUSD)
	}
	if limits.AlertThresholdPercent != 90 {
		t.Errorf("AlertThresholdPercent = %d, want 90", limits.AlertThresholdPercent)
	}
}

func TestNewBudgetError(t *testing.T) {
	check := CheckResult{
		Allowed:         false,
		EstimatedCost:   45.0,
		RemainingBudget: 55.0,
		Reason:          "Exceeded per-deployment limit",
	}

	err := NewBudgetError(check, 25.0)
	if err.Code != "SPENDING_LIMIT_EXCEEDED" {
		t.Errorf("Error code = %q, want %q", err.Code, "SPENDING_LIMIT_EXCEEDED")
	}
	if err.Message != check.Reason {
		t.Errorf("Error message = %q, want %q", err.Message, check.Reason)
	}
}
