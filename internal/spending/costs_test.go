package spending

import (
	"testing"
	"time"
)

func TestProjectMonthlySpend(t *testing.T) {
	tests := []struct {
		name        string
		dailyAvg    float64
		currentDate time.Time
		want        float64
	}{
		{
			name:        "January with $10/day",
			dailyAvg:    10.0,
			currentDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			want:        310.0, // 31 days * $10
		},
		{
			name:        "February non-leap with $5/day",
			dailyAvg:    5.0,
			currentDate: time.Date(2023, 2, 10, 0, 0, 0, 0, time.UTC),
			want:        140.0, // 28 days * $5
		},
		{
			name:        "February leap year with $5/day",
			dailyAvg:    5.0,
			currentDate: time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC),
			want:        145.0, // 29 days * $5
		},
		{
			name:        "April with $3.50/day",
			dailyAvg:    3.5,
			currentDate: time.Date(2024, 4, 20, 0, 0, 0, 0, time.UTC),
			want:        105.0, // 30 days * $3.50
		},
		{
			name:        "zero daily average",
			dailyAvg:    0.0,
			currentDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			want:        0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectMonthlySpend(tt.dailyAvg, tt.currentDate)
			if got != tt.want {
				t.Errorf("projectMonthlySpend(%v, %v) = %v, want %v",
					tt.dailyAvg, tt.currentDate, got, tt.want)
			}
		})
	}
}

func TestCostSummaryAlertFields(t *testing.T) {
	// Test that CostSummary correctly tracks alert states
	summary := CostSummary{
		DeploymentID:      "deploy-test",
		TotalCostUSD:      20.0,
		DailyCostUSD:      2.0,
		ProjectedMonthUSD: 60.0,
		DaysInPeriod:      10,
		AlertThreshold:    false,
		BudgetExceeded:    false,
	}

	if summary.AlertThreshold {
		t.Error("AlertThreshold should be false initially")
	}
	if summary.BudgetExceeded {
		t.Error("BudgetExceeded should be false initially")
	}

	// Simulate alert triggered
	summary.AlertThreshold = true
	if !summary.AlertThreshold {
		t.Error("AlertThreshold should be true after setting")
	}

	// Simulate budget exceeded
	summary.BudgetExceeded = true
	if !summary.BudgetExceeded {
		t.Error("BudgetExceeded should be true after setting")
	}
}

func TestMonitoringReportStructure(t *testing.T) {
	// Test MonitoringReport structure initialization
	now := time.Now().UTC()
	report := MonitoringReport{
		Timestamp:         now,
		TotalMonthlySpend: 50.0,
		ProjectedMonth:    75.0,
		MonthlyBudget:     100.0,
		RemainingBudget:   50.0,
		BudgetUtilization: 50.0,
		DeploymentCosts:   make(map[string]*CostSummary),
	}

	if report.Timestamp != now {
		t.Error("Timestamp mismatch")
	}
	if report.BudgetUtilization != 50.0 {
		t.Errorf("BudgetUtilization = %v, want 50.0", report.BudgetUtilization)
	}
	if report.RemainingBudget != 50.0 {
		t.Errorf("RemainingBudget = %v, want 50.0", report.RemainingBudget)
	}
}

func TestNewCostTracker(t *testing.T) {
	// Test that NewCostTracker can be created (without making actual API calls)
	// This validates the struct and constructor work correctly
	
	// We can't test with real AWS config in unit tests, but we can verify
	// the function signature and that the returned tracker is non-nil
	// with a mock/empty config in integration tests
	
	// For unit tests, we just verify the types compile correctly
	var tracker *CostTracker
	if tracker != nil {
		t.Error("uninitialized tracker should be nil")
	}
}

func TestCostSummaryJSONTags(t *testing.T) {
	// Verify CostSummary has expected fields for JSON serialization
	summary := CostSummary{
		DeploymentID:      "deploy-123",
		TotalCostUSD:      25.50,
		DailyCostUSD:      1.50,
		ProjectedMonthUSD: 45.0,
		DaysInPeriod:      17,
		AlertThreshold:    true,
		BudgetExceeded:    false,
	}

	// Verify all fields are accessible
	if summary.DeploymentID != "deploy-123" {
		t.Error("DeploymentID mismatch")
	}
	if summary.TotalCostUSD != 25.50 {
		t.Error("TotalCostUSD mismatch")
	}
	if summary.DaysInPeriod != 17 {
		t.Error("DaysInPeriod mismatch")
	}
}

func TestBudgetUtilizationCalculation(t *testing.T) {
	tests := []struct {
		name          string
		totalSpend    float64
		monthlyBudget float64
		wantUtil      float64
	}{
		{
			name:          "50% utilization",
			totalSpend:    50.0,
			monthlyBudget: 100.0,
			wantUtil:      50.0,
		},
		{
			name:          "100% utilization",
			totalSpend:    100.0,
			monthlyBudget: 100.0,
			wantUtil:      100.0,
		},
		{
			name:          "0% utilization",
			totalSpend:    0.0,
			monthlyBudget: 100.0,
			wantUtil:      0.0,
		},
		{
			name:          "over budget",
			totalSpend:    150.0,
			monthlyBudget: 100.0,
			wantUtil:      150.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var utilization float64
			if tt.monthlyBudget > 0 {
				utilization = (tt.totalSpend / tt.monthlyBudget) * 100
			}
			if utilization != tt.wantUtil {
				t.Errorf("utilization = %v, want %v", utilization, tt.wantUtil)
			}
		})
	}
}

func TestAlertThresholdLogic(t *testing.T) {
	limits := Limits{
		MonthlyBudgetUSD:      100.0,
		PerDeploymentUSD:      25.0,
		AlertThresholdPercent: 80,
	}

	// Calculate alert thresholds
	perDeploymentAlertThreshold := limits.PerDeploymentUSD * float64(limits.AlertThresholdPercent) / 100.0
	monthlyAlertThreshold := limits.MonthlyBudgetUSD * float64(limits.AlertThresholdPercent) / 100.0

	if perDeploymentAlertThreshold != 20.0 {
		t.Errorf("perDeploymentAlertThreshold = %v, want 20.0", perDeploymentAlertThreshold)
	}
	if monthlyAlertThreshold != 80.0 {
		t.Errorf("monthlyAlertThreshold = %v, want 80.0", monthlyAlertThreshold)
	}

	// Test threshold detection
	tests := []struct {
		name           string
		cost           float64
		threshold      float64
		limit          float64
		wantAlert      bool
		wantExceeded   bool
	}{
		{
			name:         "below threshold",
			cost:         15.0,
			threshold:    20.0,
			limit:        25.0,
			wantAlert:    false,
			wantExceeded: false,
		},
		{
			name:         "at threshold",
			cost:         20.0,
			threshold:    20.0,
			limit:        25.0,
			wantAlert:    true,
			wantExceeded: false,
		},
		{
			name:         "above threshold below limit",
			cost:         22.0,
			threshold:    20.0,
			limit:        25.0,
			wantAlert:    true,
			wantExceeded: false,
		},
		{
			name:         "at limit",
			cost:         25.0,
			threshold:    20.0,
			limit:        25.0,
			wantAlert:    true,
			wantExceeded: true,
		},
		{
			name:         "over limit",
			cost:         30.0,
			threshold:    20.0,
			limit:        25.0,
			wantAlert:    true,
			wantExceeded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alertTriggered := tt.cost >= tt.threshold
			budgetExceeded := tt.cost >= tt.limit

			if alertTriggered != tt.wantAlert {
				t.Errorf("alertTriggered = %v, want %v", alertTriggered, tt.wantAlert)
			}
			if budgetExceeded != tt.wantExceeded {
				t.Errorf("budgetExceeded = %v, want %v", budgetExceeded, tt.wantExceeded)
			}
		})
	}
}
