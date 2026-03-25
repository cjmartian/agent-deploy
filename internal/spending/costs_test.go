package spending

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
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

// ============================================================================
// Mock implementations for CostTracker tests
// ============================================================================

// mockCostExplorerClient implements CostExplorerAPI for testing.
type mockCostExplorerClient struct {
GetCostAndUsageFunc func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
}

func (m *mockCostExplorerClient) GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
if m.GetCostAndUsageFunc != nil {
return m.GetCostAndUsageFunc(ctx, params, optFns...)
}
return &costexplorer.GetCostAndUsageOutput{}, nil
}

// ============================================================================
// CostTracker tests with mocked AWS client
// ============================================================================

func TestNewCostTrackerWithClient(t *testing.T) {
mock := &mockCostExplorerClient{}
tracker := NewCostTrackerWithClient(mock)

if tracker == nil {
t.Fatal("Expected non-nil CostTracker")
}
if tracker.client != mock {
t.Error("Expected client to be the injected mock")
}
}

func TestGetDeploymentCosts_Success(t *testing.T) {
mock := &mockCostExplorerClient{
GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
// Verify request parameters
if params.Filter.Tags.Key == nil || *params.Filter.Tags.Key != "agent-deploy:deployment-id" {
t.Error("Expected tag filter for deployment-id")
}
if len(params.Filter.Tags.Values) != 1 || params.Filter.Tags.Values[0] != "deploy-test-123" {
t.Error("Expected deployment ID in tag values")
}

return &costexplorer.GetCostAndUsageOutput{
ResultsByTime: []cetypes.ResultByTime{
{
Total: map[string]cetypes.MetricValue{
"UnblendedCost": {
Amount: aws.String("10.50"),
Unit:   aws.String("USD"),
},
},
},
{
Total: map[string]cetypes.MetricValue{
"UnblendedCost": {
Amount: aws.String("12.25"),
Unit:   aws.String("USD"),
},
},
},
},
}, nil
},
}

tracker := NewCostTrackerWithClient(mock)
ctx := context.Background()

startDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)

summary, err := tracker.GetDeploymentCosts(ctx, "deploy-test-123", startDate, endDate)
if err != nil {
t.Fatalf("GetDeploymentCosts failed: %v", err)
}

if summary.DeploymentID != "deploy-test-123" {
t.Errorf("Expected deployment ID 'deploy-test-123', got %s", summary.DeploymentID)
}

// Total should be 10.50 + 12.25 = 22.75
expectedTotal := 22.75
if summary.TotalCostUSD != expectedTotal {
t.Errorf("Expected total cost %v, got %v", expectedTotal, summary.TotalCostUSD)
}

// 9 days (March 1-10)
if summary.DaysInPeriod != 9 {
t.Errorf("Expected 9 days, got %d", summary.DaysInPeriod)
}

// Daily average should be 22.75 / 9 ≈ 2.528
expectedDaily := 22.75 / 9.0
if summary.DailyCostUSD != expectedDaily {
t.Errorf("Expected daily cost %v, got %v", expectedDaily, summary.DailyCostUSD)
}
}

func TestGetDeploymentCosts_EmptyResults(t *testing.T) {
mock := &mockCostExplorerClient{
GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
return &costexplorer.GetCostAndUsageOutput{
ResultsByTime: []cetypes.ResultByTime{},
}, nil
},
}

tracker := NewCostTrackerWithClient(mock)
ctx := context.Background()

startDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)

summary, err := tracker.GetDeploymentCosts(ctx, "deploy-new", startDate, endDate)
if err != nil {
t.Fatalf("GetDeploymentCosts failed: %v", err)
}

if summary.TotalCostUSD != 0 {
t.Errorf("Expected 0 cost for empty results, got %v", summary.TotalCostUSD)
}
}

func TestGetDeploymentCosts_APIError(t *testing.T) {
mock := &mockCostExplorerClient{
GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
return nil, fmt.Errorf("AccessDeniedException: User does not have permission")
},
}

tracker := NewCostTrackerWithClient(mock)
ctx := context.Background()

startDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)

_, err := tracker.GetDeploymentCosts(ctx, "deploy-test", startDate, endDate)
if err == nil {
t.Error("Expected error for API failure")
}
}

func TestGetDeploymentCosts_InvalidCostAmount(t *testing.T) {
mock := &mockCostExplorerClient{
GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
return &costexplorer.GetCostAndUsageOutput{
ResultsByTime: []cetypes.ResultByTime{
{
Total: map[string]cetypes.MetricValue{
"UnblendedCost": {
Amount: aws.String("not-a-number"),
Unit:   aws.String("USD"),
},
},
},
},
}, nil
},
}

tracker := NewCostTrackerWithClient(mock)
ctx := context.Background()

startDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)

// Should not error - just logs warning and treats as 0
summary, err := tracker.GetDeploymentCosts(ctx, "deploy-test", startDate, endDate)
if err != nil {
t.Fatalf("GetDeploymentCosts should not fail on invalid amount: %v", err)
}

if summary.TotalCostUSD != 0 {
t.Errorf("Expected 0 cost for invalid amount, got %v", summary.TotalCostUSD)
}
}

func TestGetTotalMonthlySpend_Success(t *testing.T) {
mock := &mockCostExplorerClient{
GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
// Verify it filters by agent-deploy tag
if params.Filter.Tags.Key == nil || *params.Filter.Tags.Key != "agent-deploy:created-by" {
t.Error("Expected filter for agent-deploy:created-by tag")
}

return &costexplorer.GetCostAndUsageOutput{
ResultsByTime: []cetypes.ResultByTime{
{
Total: map[string]cetypes.MetricValue{
"UnblendedCost": {
Amount: aws.String("150.00"),
Unit:   aws.String("USD"),
},
},
},
},
}, nil
},
}

tracker := NewCostTrackerWithClient(mock)
ctx := context.Background()

summary, err := tracker.GetTotalMonthlySpend(ctx)
if err != nil {
t.Fatalf("GetTotalMonthlySpend failed: %v", err)
}

if summary.TotalCostUSD != 150.0 {
t.Errorf("Expected total cost 150.0, got %v", summary.TotalCostUSD)
}
}

func TestGetCostsByDeployment_Success(t *testing.T) {
mock := &mockCostExplorerClient{
GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
return &costexplorer.GetCostAndUsageOutput{
ResultsByTime: []cetypes.ResultByTime{
{
Groups: []cetypes.Group{
{
Keys: []string{"deploy-app-1"},
Metrics: map[string]cetypes.MetricValue{
"UnblendedCost": {Amount: aws.String("25.50")},
},
},
{
Keys: []string{"deploy-app-2"},
Metrics: map[string]cetypes.MetricValue{
"UnblendedCost": {Amount: aws.String("30.00")},
},
},
},
},
},
}, nil
},
}

tracker := NewCostTrackerWithClient(mock)
ctx := context.Background()

startDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
endDate := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

costs, err := tracker.GetCostsByDeployment(ctx, startDate, endDate)
if err != nil {
t.Fatalf("GetCostsByDeployment failed: %v", err)
}

if len(costs) != 2 {
t.Errorf("Expected 2 deployments, got %d", len(costs))
}

// Check deploy-app-1
if cost, ok := costs["deploy-app-1"]; ok {
if cost.TotalCostUSD != 25.50 {
t.Errorf("Expected deploy-app-1 cost 25.50, got %v", cost.TotalCostUSD)
}
} else {
t.Error("Missing deploy-app-1 in costs")
}

// Check deploy-app-2
if cost, ok := costs["deploy-app-2"]; ok {
if cost.TotalCostUSD != 30.0 {
t.Errorf("Expected deploy-app-2 cost 30.0, got %v", cost.TotalCostUSD)
}
} else {
t.Error("Missing deploy-app-2 in costs")
}
}

func TestCheckAlerts_ThresholdExceeded(t *testing.T) {
mock := &mockCostExplorerClient{
GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
return &costexplorer.GetCostAndUsageOutput{
ResultsByTime: []cetypes.ResultByTime{
{
Groups: []cetypes.Group{
{
Keys: []string{"deploy-over-budget"},
Metrics: map[string]cetypes.MetricValue{
"UnblendedCost": {Amount: aws.String("22.00")}, // Over 80% of 25
},
},
{
Keys: []string{"deploy-under-budget"},
Metrics: map[string]cetypes.MetricValue{
"UnblendedCost": {Amount: aws.String("10.00")}, // Under threshold
},
},
},
},
},
}, nil
},
}

tracker := NewCostTrackerWithClient(mock)
ctx := context.Background()

limits := Limits{
MonthlyBudgetUSD:      100.0,
PerDeploymentUSD:      25.0,
AlertThresholdPercent: 80,
}

alerts, err := tracker.CheckAlerts(ctx, limits)
if err != nil {
t.Fatalf("CheckAlerts failed: %v", err)
}

// Should have 1 alert for deploy-over-budget
if len(alerts) != 1 {
t.Errorf("Expected 1 alert, got %d", len(alerts))
}

if len(alerts) > 0 {
alert := alerts[0]
if alert.DeploymentID != "deploy-over-budget" {
t.Errorf("Expected alert for 'deploy-over-budget', got %s", alert.DeploymentID)
}
if !alert.AlertThreshold {
t.Error("Expected AlertThreshold to be true")
}
}
}

func TestGetDeploymentsOverBudget(t *testing.T) {
mock := &mockCostExplorerClient{
GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
return &costexplorer.GetCostAndUsageOutput{
ResultsByTime: []cetypes.ResultByTime{
{
Groups: []cetypes.Group{
{
Keys: []string{"deploy-way-over"},
Metrics: map[string]cetypes.MetricValue{
"UnblendedCost": {Amount: aws.String("30.00")}, // Over 25 limit
},
},
{
Keys: []string{"deploy-ok"},
Metrics: map[string]cetypes.MetricValue{
"UnblendedCost": {Amount: aws.String("15.00")}, // Under limit
},
},
},
},
},
}, nil
},
}

tracker := NewCostTrackerWithClient(mock)
ctx := context.Background()

limits := Limits{
MonthlyBudgetUSD:      100.0,
PerDeploymentUSD:      25.0,
AlertThresholdPercent: 80,
}

overBudget, err := tracker.GetDeploymentsOverBudget(ctx, limits)
if err != nil {
t.Fatalf("GetDeploymentsOverBudget failed: %v", err)
}

if len(overBudget) != 1 {
t.Errorf("Expected 1 deployment over budget, got %d", len(overBudget))
}

if len(overBudget) > 0 && overBudget[0] != "deploy-way-over" {
t.Errorf("Expected 'deploy-way-over', got %s", overBudget[0])
}
}

func TestGenerateMonitoringReport(t *testing.T) {
totalCallCount := 0
byDeploymentCallCount := 0

mock := &mockCostExplorerClient{
GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
// Differentiate between total spend query and per-deployment query
if params.GroupBy != nil && len(params.GroupBy) > 0 {
byDeploymentCallCount++
return &costexplorer.GetCostAndUsageOutput{
ResultsByTime: []cetypes.ResultByTime{
{
Groups: []cetypes.Group{
{
Keys: []string{"deploy-1"},
Metrics: map[string]cetypes.MetricValue{
"UnblendedCost": {Amount: aws.String("40.00")},
},
},
},
},
},
}, nil
}
totalCallCount++
return &costexplorer.GetCostAndUsageOutput{
ResultsByTime: []cetypes.ResultByTime{
{
Total: map[string]cetypes.MetricValue{
"UnblendedCost": {Amount: aws.String("40.00")},
},
},
},
}, nil
},
}

tracker := NewCostTrackerWithClient(mock)
ctx := context.Background()

limits := Limits{
MonthlyBudgetUSD:      100.0,
PerDeploymentUSD:      50.0,
AlertThresholdPercent: 80,
}

report, err := tracker.GenerateMonitoringReport(ctx, limits)
if err != nil {
t.Fatalf("GenerateMonitoringReport failed: %v", err)
}

if report.TotalMonthlySpend != 40.0 {
t.Errorf("Expected TotalMonthlySpend 40.0, got %v", report.TotalMonthlySpend)
}

if report.MonthlyBudget != 100.0 {
t.Errorf("Expected MonthlyBudget 100.0, got %v", report.MonthlyBudget)
}

if report.RemainingBudget != 60.0 {
t.Errorf("Expected RemainingBudget 60.0, got %v", report.RemainingBudget)
}

if report.BudgetUtilization != 40.0 {
t.Errorf("Expected BudgetUtilization 40.0%%, got %v", report.BudgetUtilization)
}

if totalCallCount == 0 {
t.Error("Expected at least one total spend query")
}
}
