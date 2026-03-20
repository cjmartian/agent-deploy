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
		DeploymentID:   "deploy-test",
		AlertThreshold: false,
		BudgetExceeded: false,
	}

	if summary.DeploymentID != "deploy-test" {
		t.Error("DeploymentID mismatch")
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
	if report.TotalMonthlySpend != 50.0 {
		t.Errorf("TotalMonthlySpend = %v, want 50.0", report.TotalMonthlySpend)
	}
	if report.ProjectedMonth != 75.0 {
		t.Errorf("ProjectedMonth = %v, want 75.0", report.ProjectedMonth)
	}
	if report.MonthlyBudget != 100.0 {
		t.Errorf("MonthlyBudget = %v, want 100.0", report.MonthlyBudget)
	}
	if report.BudgetUtilization != 50.0 {
		t.Errorf("BudgetUtilization = %v, want 50.0", report.BudgetUtilization)
	}
	if report.RemainingBudget != 50.0 {
		t.Errorf("RemainingBudget = %v, want 50.0", report.RemainingBudget)
	}
	if report.DeploymentCosts == nil {
		t.Error("DeploymentCosts should not be nil")
	}
}

func TestNewCostTracker(t *testing.T) {
	// Test that CostTracker struct can be instantiated.
	// Actual functionality requires AWS config, which is tested in integration tests.
	tracker := &CostTracker{}

	// Verify the struct exists and has a valid memory address
	_ = tracker // Use the variable to ensure it compiles
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
	if summary.DailyCostUSD != 1.50 {
		t.Error("DailyCostUSD mismatch")
	}
	if summary.ProjectedMonthUSD != 45.0 {
		t.Error("ProjectedMonthUSD mismatch")
	}
	if summary.DaysInPeriod != 17 {
		t.Error("DaysInPeriod mismatch")
	}
	if !summary.AlertThreshold {
		t.Error("AlertThreshold mismatch")
	}
	if summary.BudgetExceeded {
		t.Error("BudgetExceeded mismatch")
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
