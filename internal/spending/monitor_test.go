package spending

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

func TestDefaultMonitorConfig(t *testing.T) {
	cfg := DefaultMonitorConfig()

	if cfg.CheckInterval != 1*time.Hour {
		t.Errorf("CheckInterval = %v, want 1h", cfg.CheckInterval)
	}
	if cfg.EnableAutoTeardown {
		t.Error("EnableAutoTeardown should be false by default")
	}
	if cfg.TeardownCallback != nil {
		t.Error("TeardownCallback should be nil by default")
	}
	if cfg.AlertCallback != nil {
		t.Error("AlertCallback should be nil by default")
	}
}

func TestGetAlertLevel(t *testing.T) {
	tests := []struct {
		name             string
		currentSpend     float64
		limit            float64
		thresholdPercent int
		want             AlertLevel
	}{
		{
			name:             "no spend",
			currentSpend:     0,
			limit:            100,
			thresholdPercent: 80,
			want:             AlertLevelNone,
		},
		{
			name:             "below warning",
			currentSpend:     50,
			limit:            100,
			thresholdPercent: 80,
			want:             AlertLevelNone,
		},
		{
			name:             "at warning level (90% of threshold)",
			currentSpend:     72, // 72% is 90% of 80%
			limit:            100,
			thresholdPercent: 80,
			want:             AlertLevelWarning,
		},
		{
			name:             "at critical level (at threshold)",
			currentSpend:     80,
			limit:            100,
			thresholdPercent: 80,
			want:             AlertLevelCritical,
		},
		{
			name:             "above threshold below limit",
			currentSpend:     90,
			limit:            100,
			thresholdPercent: 80,
			want:             AlertLevelCritical,
		},
		{
			name:             "at limit",
			currentSpend:     100,
			limit:            100,
			thresholdPercent: 80,
			want:             AlertLevelExceeded,
		},
		{
			name:             "over limit",
			currentSpend:     150,
			limit:            100,
			thresholdPercent: 80,
			want:             AlertLevelExceeded,
		},
		{
			name:             "zero limit",
			currentSpend:     50,
			limit:            0,
			thresholdPercent: 80,
			want:             AlertLevelNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAlertLevel(tt.currentSpend, tt.limit, tt.thresholdPercent)
			if got != tt.want {
				t.Errorf("GetAlertLevel(%v, %v, %v) = %v, want %v",
					tt.currentSpend, tt.limit, tt.thresholdPercent, got, tt.want)
			}
		})
	}
}

func TestNewSpendingAlert(t *testing.T) {
	tests := []struct {
		name         string
		deploymentID string
		spend        float64
		limit        float64
		threshold    int
		autoTeardown bool
		wantLevel    AlertLevel
		wantAuto     bool
	}{
		{
			name:         "deployment over budget with auto-teardown",
			deploymentID: "deploy-123",
			spend:        30,
			limit:        25,
			threshold:    80,
			autoTeardown: true,
			wantLevel:    AlertLevelExceeded,
			wantAuto:     true,
		},
		{
			name:         "deployment over budget without auto-teardown",
			deploymentID: "deploy-456",
			spend:        30,
			limit:        25,
			threshold:    80,
			autoTeardown: false,
			wantLevel:    AlertLevelExceeded,
			wantAuto:     false,
		},
		{
			name:         "total budget at critical",
			deploymentID: "TOTAL",
			spend:        80,
			limit:        100,
			threshold:    80,
			autoTeardown: true,
			wantLevel:    AlertLevelCritical,
			wantAuto:     false, // Auto-teardown only triggers on exceeded
		},
		{
			name:         "within limits",
			deploymentID: "deploy-789",
			spend:        10,
			limit:        25,
			threshold:    80,
			autoTeardown: true,
			wantLevel:    AlertLevelNone,
			wantAuto:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := NewSpendingAlert(tt.deploymentID, tt.spend, tt.limit, tt.threshold, tt.autoTeardown)

			if alert.Level != tt.wantLevel {
				t.Errorf("Level = %v, want %v", alert.Level, tt.wantLevel)
			}
			if alert.AutoTeardownSet != tt.wantAuto {
				t.Errorf("AutoTeardownSet = %v, want %v", alert.AutoTeardownSet, tt.wantAuto)
			}
			if alert.DeploymentID != tt.deploymentID {
				t.Errorf("DeploymentID = %v, want %v", alert.DeploymentID, tt.deploymentID)
			}
			if alert.CurrentSpend != tt.spend {
				t.Errorf("CurrentSpend = %v, want %v", alert.CurrentSpend, tt.spend)
			}
			if alert.Limit != tt.limit {
				t.Errorf("Limit = %v, want %v", alert.Limit, tt.limit)
			}
			if alert.Message == "" {
				t.Error("Message should not be empty")
			}
			if alert.Timestamp.IsZero() {
				t.Error("Timestamp should be set")
			}
		})
	}
}

func TestMonitorStats(t *testing.T) {
	lastRun := time.Now()
	stats := MonitorStats{
		Running:       true,
		LastRun:       lastRun,
		AlertsSent:    5,
		TeardownsDone: 2,
		CheckInterval: 30 * time.Minute,
	}

	if !stats.Running {
		t.Error("Running should be true")
	}
	if stats.LastRun != lastRun {
		t.Errorf("LastRun = %v, want %v", stats.LastRun, lastRun)
	}
	if stats.AlertsSent != 5 {
		t.Errorf("AlertsSent = %v, want 5", stats.AlertsSent)
	}
	if stats.TeardownsDone != 2 {
		t.Errorf("TeardownsDone = %v, want 2", stats.TeardownsDone)
	}
	if stats.CheckInterval != 30*time.Minute {
		t.Errorf("CheckInterval = %v, want 30m", stats.CheckInterval)
	}
}

func TestAlertTarget(t *testing.T) {
	tests := []struct {
		name  string
		alert CostSummary
		want  string
	}{
		{
			name:  "total alert",
			alert: CostSummary{DeploymentID: "TOTAL"},
			want:  "Total monthly spend",
		},
		{
			name:  "deployment alert",
			alert: CostSummary{DeploymentID: "deploy-abc"},
			want:  "Deployment deploy-abc",
		},
		{
			name:  "empty deployment",
			alert: CostSummary{DeploymentID: ""},
			want:  "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alertTarget(tt.alert)
			if got != tt.want {
				t.Errorf("alertTarget() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlertLevelConstants(t *testing.T) {
	// Verify alert level constants are distinct
	levels := []AlertLevel{
		AlertLevelNone,
		AlertLevelWarning,
		AlertLevelCritical,
		AlertLevelExceeded,
	}

	seen := make(map[AlertLevel]bool)
	for _, level := range levels {
		if seen[level] {
			t.Errorf("Duplicate alert level: %v", level)
		}
		seen[level] = true
	}

	// Verify expected values
	if AlertLevelNone != "none" {
		t.Errorf("AlertLevelNone = %v, want 'none'", AlertLevelNone)
	}
	if AlertLevelWarning != "warning" {
		t.Errorf("AlertLevelWarning = %v, want 'warning'", AlertLevelWarning)
	}
	if AlertLevelCritical != "critical" {
		t.Errorf("AlertLevelCritical = %v, want 'critical'", AlertLevelCritical)
	}
	if AlertLevelExceeded != "exceeded" {
		t.Errorf("AlertLevelExceeded = %v, want 'exceeded'", AlertLevelExceeded)
	}
}

func TestCallbackInvocation(t *testing.T) {
	var mu sync.Mutex
	alertsCalled := 0
	teardownsCalled := 0
	var lastAlertDeployment string

	config := MonitorConfig{
		AlertCallback: func(ctx context.Context, alert CostSummary) {
			mu.Lock()
			alertsCalled++
			lastAlertDeployment = alert.DeploymentID
			mu.Unlock()
		},
		TeardownCallback: func(ctx context.Context, deploymentID string) error {
			mu.Lock()
			teardownsCalled++
			mu.Unlock()
			return nil
		},
	}

	// Verify callbacks are properly set
	if config.AlertCallback == nil {
		t.Fatal("AlertCallback should not be nil")
	}
	if config.TeardownCallback == nil {
		t.Fatal("TeardownCallback should not be nil")
	}

	// Test alert callback directly
	testAlert := CostSummary{
		DeploymentID:   "test-deploy",
		TotalCostUSD:   30.0,
		AlertThreshold: true,
		BudgetExceeded: true,
	}
	config.AlertCallback(context.Background(), testAlert)

	mu.Lock()
	if alertsCalled != 1 {
		t.Errorf("alertsCalled = %v, want 1", alertsCalled)
	}
	if lastAlertDeployment != "test-deploy" {
		t.Errorf("lastAlertDeployment = %v, want 'test-deploy'", lastAlertDeployment)
	}
	mu.Unlock()

	// Test teardown callback directly
	if err := config.TeardownCallback(context.Background(), "teardown-deploy"); err != nil {
		t.Errorf("TeardownCallback returned error: %v", err)
	}

	mu.Lock()
	if teardownsCalled != 1 {
		t.Errorf("teardownsCalled = %v, want 1", teardownsCalled)
	}
	mu.Unlock()
}

func TestPercentUsedCalculation(t *testing.T) {
	tests := []struct {
		name    string
		spend   float64
		limit   float64
		wantPct float64
	}{
		{
			name:    "50%",
			spend:   50,
			limit:   100,
			wantPct: 50.0,
		},
		{
			name:    "100%",
			spend:   100,
			limit:   100,
			wantPct: 100.0,
		},
		{
			name:    "150%",
			spend:   150,
			limit:   100,
			wantPct: 150.0,
		},
		{
			name:    "zero limit",
			spend:   50,
			limit:   0,
			wantPct: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := NewSpendingAlert("test", tt.spend, tt.limit, 80, false)
			if alert.PercentUsed != tt.wantPct {
				t.Errorf("PercentUsed = %v, want %v", alert.PercentUsed, tt.wantPct)
			}
		})
	}
}

// ============================================================================
// CostMonitor lifecycle tests
// ============================================================================

func TestNewCostMonitorWithTracker(t *testing.T) {
	// Create a mock tracker
	mockClient := &mockCostExplorerClient{}
	tracker := NewCostTrackerWithClient(mockClient)

	limits := Limits{
		MonthlyBudgetUSD:      100.0,
		PerDeploymentUSD:      25.0,
		AlertThresholdPercent: 80,
	}

	config := MonitorConfig{
		CheckInterval: 5 * time.Minute,
	}

	monitor := NewCostMonitorWithTracker(tracker, limits, config)

	if monitor == nil {
		t.Fatal("Expected non-nil CostMonitor")
	}
	if monitor.tracker != tracker {
		t.Error("Expected tracker to be the injected tracker")
	}
	if monitor.config.CheckInterval != 5*time.Minute {
		t.Errorf("Expected CheckInterval 5m, got %v", monitor.config.CheckInterval)
	}
}

func TestNewCostMonitorWithTracker_DefaultInterval(t *testing.T) {
	mockClient := &mockCostExplorerClient{}
	tracker := NewCostTrackerWithClient(mockClient)

	limits := DefaultLimits()
	config := MonitorConfig{} // Zero interval

	monitor := NewCostMonitorWithTracker(tracker, limits, config)

	// Should default to 1 hour
	if monitor.config.CheckInterval != 1*time.Hour {
		t.Errorf("Expected default CheckInterval 1h, got %v", monitor.config.CheckInterval)
	}
}

func TestCostMonitor_IsRunning(t *testing.T) {
	mockClient := &mockCostExplorerClient{}
	tracker := NewCostTrackerWithClient(mockClient)
	limits := DefaultLimits()
	config := MonitorConfig{CheckInterval: 100 * time.Millisecond}

	monitor := NewCostMonitorWithTracker(tracker, limits, config)

	// Initially not running
	if monitor.IsRunning() {
		t.Error("Monitor should not be running initially")
	}
}

func TestCostMonitor_LastRunTime(t *testing.T) {
	mockClient := &mockCostExplorerClient{}
	tracker := NewCostTrackerWithClient(mockClient)
	limits := DefaultLimits()
	config := MonitorConfig{CheckInterval: 100 * time.Millisecond}

	monitor := NewCostMonitorWithTracker(tracker, limits, config)

	// Initially zero
	if !monitor.LastRunTime().IsZero() {
		t.Error("LastRunTime should be zero initially")
	}
}

func TestCostMonitor_LastError(t *testing.T) {
	mockClient := &mockCostExplorerClient{}
	tracker := NewCostTrackerWithClient(mockClient)
	limits := DefaultLimits()
	config := MonitorConfig{CheckInterval: 100 * time.Millisecond}

	monitor := NewCostMonitorWithTracker(tracker, limits, config)

	// Initially nil
	if monitor.LastError() != nil {
		t.Error("LastError should be nil initially")
	}
}

func TestCostMonitor_Stats(t *testing.T) {
	mockClient := &mockCostExplorerClient{}
	tracker := NewCostTrackerWithClient(mockClient)
	limits := DefaultLimits()
	config := MonitorConfig{CheckInterval: 100 * time.Millisecond}

	monitor := NewCostMonitorWithTracker(tracker, limits, config)

	stats := monitor.Stats()

	if stats.AlertsSent != 0 {
		t.Errorf("Expected AlertsSent 0, got %d", stats.AlertsSent)
	}
	if stats.TeardownsDone != 0 {
		t.Errorf("Expected TeardownsDone 0, got %d", stats.TeardownsDone)
	}
	if stats.Running {
		t.Error("Expected IsRunning false")
	}
}

func TestCostMonitor_StartStop(t *testing.T) {
	// Mock returns empty results quickly
	mockClient := &mockCostExplorerClient{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []cetypes.ResultByTime{},
			}, nil
		},
	}
	tracker := NewCostTrackerWithClient(mockClient)
	limits := DefaultLimits()
	config := MonitorConfig{CheckInterval: 50 * time.Millisecond}

	monitor := NewCostMonitorWithTracker(tracker, limits, config)

	ctx := context.Background()

	// Start in a goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = monitor.Start(ctx)
	}()

	// Give it time to start
	time.Sleep(20 * time.Millisecond)

	if !monitor.IsRunning() {
		t.Error("Monitor should be running after Start")
	}

	// Stop
	monitor.Stop()

	// Wait for it to fully stop
	wg.Wait()

	if monitor.IsRunning() {
		t.Error("Monitor should not be running after Stop")
	}
}

func TestCostMonitor_StartTwice(t *testing.T) {
	mockClient := &mockCostExplorerClient{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return &costexplorer.GetCostAndUsageOutput{}, nil
		},
	}
	tracker := NewCostTrackerWithClient(mockClient)
	limits := DefaultLimits()
	config := MonitorConfig{CheckInterval: 100 * time.Millisecond}

	monitor := NewCostMonitorWithTracker(tracker, limits, config)
	ctx := context.Background()

	// Start first time
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = monitor.Start(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	// Try to start again - should error
	err := monitor.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already-running monitor")
	}

	monitor.Stop()
	wg.Wait()
}

func TestCostMonitor_CheckNow(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	mockClient := &mockCostExplorerClient{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []cetypes.ResultByTime{
					{
						Total: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("50.00")},
						},
					},
				},
			}, nil
		},
	}
	tracker := NewCostTrackerWithClient(mockClient)
	limits := DefaultLimits()
	config := MonitorConfig{CheckInterval: 1 * time.Hour} // Long interval

	monitor := NewCostMonitorWithTracker(tracker, limits, config)
	ctx := context.Background()

	// Start monitor
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = monitor.Start(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	// CheckNow should trigger an immediate check
	_, _ = monitor.CheckNow(ctx)

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	monitor.Stop()
	wg.Wait()

	// Should have made at least one check
	mu.Lock()
	count := callCount
	mu.Unlock()
	if count == 0 {
		t.Error("Expected at least one Cost Explorer call from CheckNow")
	}

	// LastRunTime should be set
	if monitor.LastRunTime().IsZero() {
		t.Error("LastRunTime should be set after check")
	}
}

func TestCostMonitor_AlertCallback(t *testing.T) {
	// Mock returns costs that trigger an alert
	mockClient := &mockCostExplorerClient{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			// First call is for total spend, second is for by-deployment
			if len(params.GroupBy) > 0 {
				return &costexplorer.GetCostAndUsageOutput{
					ResultsByTime: []cetypes.ResultByTime{
						{
							Groups: []cetypes.Group{
								{
									Keys:    []string{"deploy-alert"},
									Metrics: map[string]cetypes.MetricValue{"UnblendedCost": {Amount: aws.String("22.00")}},
								},
							},
						},
					},
				}, nil
			}
			return &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []cetypes.ResultByTime{
					{
						Total: map[string]cetypes.MetricValue{"UnblendedCost": {Amount: aws.String("22.00")}},
					},
				},
			}, nil
		},
	}
	tracker := NewCostTrackerWithClient(mockClient)

	alertReceived := false
	var alertMu sync.Mutex

	limits := Limits{
		MonthlyBudgetUSD:      100.0,
		PerDeploymentUSD:      25.0,
		AlertThresholdPercent: 80, // 80% of 25 = 20, so 22 should trigger
	}
	config := MonitorConfig{
		CheckInterval: 50 * time.Millisecond,
		AlertCallback: func(ctx context.Context, alert CostSummary) {
			alertMu.Lock()
			alertReceived = true
			alertMu.Unlock()
		},
	}

	monitor := NewCostMonitorWithTracker(tracker, limits, config)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = monitor.Start(ctx)
	}()

	// Wait for at least one check
	time.Sleep(150 * time.Millisecond)

	monitor.Stop()
	wg.Wait()

	alertMu.Lock()
	received := alertReceived
	alertMu.Unlock()

	if !received {
		t.Error("Expected alert callback to be called")
	}
}
