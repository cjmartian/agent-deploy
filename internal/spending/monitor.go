// Package spending provides runtime cost monitoring with alert and auto-teardown
// capabilities for deployments that exceed spending limits.
package spending

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/cjmartian/agent-deploy/internal/logging"
)

// MonitorConfig configures the runtime cost monitor behavior.
type MonitorConfig struct {
	// CheckInterval is how often to check costs (default: 1 hour)
	CheckInterval time.Duration
	// EnableAutoTeardown allows automatic teardown of over-budget deployments
	EnableAutoTeardown bool
	// TeardownCallback is called when a deployment should be torn down
	TeardownCallback func(ctx context.Context, deploymentID string) error
	// AlertCallback is called when a deployment reaches alert threshold
	AlertCallback func(ctx context.Context, alert CostSummary)
}

// DefaultMonitorConfig returns sensible defaults for monitoring.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		CheckInterval:      1 * time.Hour,
		EnableAutoTeardown: false,
		TeardownCallback:   nil,
		AlertCallback:      nil,
	}
}

// CostMonitor periodically checks spending against limits and triggers
// alerts or auto-teardown when thresholds are exceeded.
type CostMonitor struct {
	tracker *CostTracker
	limits  Limits
	config  MonitorConfig

	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	lastRun  time.Time
	lastErr  error

	// Metrics tracking
	alertsSent    int
	teardownsDone int
}

// NewCostMonitor creates a new cost monitor with the given configuration.
func NewCostMonitor(cfg aws.Config, limits Limits, config MonitorConfig) *CostMonitor {
	if config.CheckInterval == 0 {
		config.CheckInterval = 1 * time.Hour
	}
	return &CostMonitor{
		tracker: NewCostTracker(cfg),
		limits:  limits,
		config:  config,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Start begins the background monitoring loop.
// It runs until Stop() is called.
func (m *CostMonitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("monitor already running")
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})
	m.mu.Unlock()

	go m.runLoop(ctx)
	log := logging.WithComponent(logging.ComponentCostMonitor)
	log.Info("cost monitor started",
		slog.Duration("interval", m.config.CheckInterval),
		slog.Bool("auto_teardown", m.config.EnableAutoTeardown))
	return nil
}

// Stop gracefully stops the monitoring loop.
func (m *CostMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	close(m.stopCh)
	<-m.doneCh

	m.mu.Lock()
	m.running = false
	m.mu.Unlock()

	log := logging.WithComponent(logging.ComponentCostMonitor)
	log.Info("cost monitor stopped",
		slog.Int("alerts_sent", m.alertsSent),
		slog.Int("teardowns_done", m.teardownsDone))
}

// IsRunning returns whether the monitor is actively checking costs.
func (m *CostMonitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// LastRunTime returns when the last cost check occurred.
func (m *CostMonitor) LastRunTime() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastRun
}

// LastError returns the error from the most recent check, if any.
func (m *CostMonitor) LastError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

// Stats returns monitoring statistics.
func (m *CostMonitor) Stats() MonitorStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MonitorStats{
		Running:       m.running,
		LastRun:       m.lastRun,
		LastError:     m.lastErr,
		AlertsSent:    m.alertsSent,
		TeardownsDone: m.teardownsDone,
		CheckInterval: m.config.CheckInterval,
	}
}

// MonitorStats contains runtime statistics for the cost monitor.
type MonitorStats struct {
	Running       bool          `json:"running"`
	LastRun       time.Time     `json:"last_run"`
	LastError     error         `json:"last_error,omitempty"`
	AlertsSent    int           `json:"alerts_sent"`
	TeardownsDone int           `json:"teardowns_done"`
	CheckInterval time.Duration `json:"check_interval"`
}

// CheckNow performs an immediate cost check outside the regular interval.
func (m *CostMonitor) CheckNow(ctx context.Context) (*MonitoringReport, error) {
	return m.performCheck(ctx)
}

func (m *CostMonitor) runLoop(ctx context.Context) {
	defer close(m.doneCh)

	log := logging.WithComponent(logging.ComponentCostMonitor)

	// Perform initial check immediately
	if _, err := m.performCheck(ctx); err != nil {
		log.Error("initial cost check failed", logging.Err(err))
	}

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Debug("context canceled, stopping")
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			if _, err := m.performCheck(ctx); err != nil {
				log.Error("cost check failed", logging.Err(err))
			}
		}
	}
}

func (m *CostMonitor) performCheck(ctx context.Context) (*MonitoringReport, error) {
	log := logging.WithComponent(logging.ComponentCostMonitor)
	log.Debug("performing cost check")

	report, err := m.tracker.GenerateMonitoringReport(ctx, m.limits)
	if err != nil {
		m.mu.Lock()
		m.lastErr = err
		m.lastRun = time.Now()
		m.mu.Unlock()
		return nil, fmt.Errorf("generate report: %w", err)
	}

	m.mu.Lock()
	m.lastRun = time.Now()
	m.lastErr = nil
	m.mu.Unlock()

	// Process alerts
	for _, alert := range report.AlertsTriggered {
		m.processAlert(ctx, alert)
	}

	// Log summary
	log.Info("cost check complete",
		logging.Cost(report.TotalMonthlySpend),
		slog.Float64("budget_utilization_pct", report.BudgetUtilization),
		slog.Float64("monthly_budget_usd", report.MonthlyBudget),
		slog.Int("alerts", len(report.AlertsTriggered)))

	return report, nil
}

func (m *CostMonitor) processAlert(ctx context.Context, alert CostSummary) {
	log := logging.WithComponent(logging.ComponentCostMonitor)

	// Skip the aggregate TOTAL alert for auto-teardown (can't teardown everything)
	isDeploymentAlert := alert.DeploymentID != "" && alert.DeploymentID != "TOTAL"

	if alert.AlertThreshold && !alert.BudgetExceeded {
		// Threshold reached but not exceeded - send alert only
		percentUsed := (alert.TotalCostUSD / m.limits.PerDeploymentUSD) * 100
		log.Warn("spending alert threshold reached",
			slog.String("target", alertTarget(alert)),
			slog.Float64("percent_used", percentUsed),
			logging.Cost(alert.TotalCostUSD))

		m.mu.Lock()
		m.alertsSent++
		m.mu.Unlock()

		if m.config.AlertCallback != nil {
			m.config.AlertCallback(ctx, alert)
		}
	}

	if alert.BudgetExceeded {
		log.Error("budget exceeded",
			slog.String("target", alertTarget(alert)),
			logging.Cost(alert.TotalCostUSD),
			slog.Float64("limit_usd", m.limits.PerDeploymentUSD))

		m.mu.Lock()
		m.alertsSent++
		m.mu.Unlock()

		if m.config.AlertCallback != nil {
			m.config.AlertCallback(ctx, alert)
		}

		// Auto-teardown if enabled and this is a specific deployment
		if m.config.EnableAutoTeardown && isDeploymentAlert && m.config.TeardownCallback != nil {
			log.Warn("auto-teardown triggered",
				logging.DeploymentID(alert.DeploymentID))
			if err := m.config.TeardownCallback(ctx, alert.DeploymentID); err != nil {
				log.Error("auto-teardown failed",
					logging.DeploymentID(alert.DeploymentID),
					logging.Err(err))
			} else {
				m.mu.Lock()
				m.teardownsDone++
				m.mu.Unlock()
				log.Info("auto-teardown completed",
					logging.DeploymentID(alert.DeploymentID))
			}
		}
	}
}

func alertTarget(alert CostSummary) string {
	if alert.DeploymentID == "TOTAL" {
		return "Total monthly spend"
	}
	if alert.DeploymentID != "" {
		return fmt.Sprintf("Deployment %s", alert.DeploymentID)
	}
	return "Unknown"
}

// AlertLevel represents the severity of a spending alert.
type AlertLevel string

const (
	AlertLevelNone     AlertLevel = "none"
	AlertLevelWarning  AlertLevel = "warning"  // Approaching threshold
	AlertLevelCritical AlertLevel = "critical" // At or exceeding threshold
	AlertLevelExceeded AlertLevel = "exceeded" // Over budget
)

// GetAlertLevel determines the alert level based on current spend vs limits.
func GetAlertLevel(currentSpend float64, limit float64, thresholdPercent int) AlertLevel {
	if limit <= 0 {
		return AlertLevelNone
	}

	percentUsed := (currentSpend / limit) * 100

	if currentSpend >= limit {
		return AlertLevelExceeded
	}
	if percentUsed >= float64(thresholdPercent) {
		return AlertLevelCritical
	}
	if percentUsed >= float64(thresholdPercent)*0.9 { // 90% of threshold
		return AlertLevelWarning
	}
	return AlertLevelNone
}

// SpendingAlert represents a spending alert with context.
type SpendingAlert struct {
	Level           AlertLevel `json:"level"`
	DeploymentID    string     `json:"deployment_id,omitempty"`
	CurrentSpend    float64    `json:"current_spend_usd"`
	Limit           float64    `json:"limit_usd"`
	PercentUsed     float64    `json:"percent_used"`
	ProjectedMonth  float64    `json:"projected_month_usd"`
	Message         string     `json:"message"`
	Timestamp       time.Time  `json:"timestamp"`
	AutoTeardownSet bool       `json:"auto_teardown_set"`
}

// NewSpendingAlert creates a spending alert with appropriate message.
func NewSpendingAlert(deploymentID string, spend, limit float64, thresholdPercent int, autoTeardown bool) SpendingAlert {
	level := GetAlertLevel(spend, limit, thresholdPercent)
	percentUsed := 0.0
	if limit > 0 {
		percentUsed = (spend / limit) * 100
	}

	var message string
	switch level {
	case AlertLevelExceeded:
		message = fmt.Sprintf("Budget exceeded: $%.2f spent (limit: $%.2f)", spend, limit)
		if autoTeardown {
			message += " - auto-teardown will be triggered"
		}
	case AlertLevelCritical:
		message = fmt.Sprintf("Approaching budget limit: $%.2f spent (%.1f%% of $%.2f)", spend, percentUsed, limit)
	case AlertLevelWarning:
		message = fmt.Sprintf("Spending alert: $%.2f spent (%.1f%% of $%.2f)", spend, percentUsed, limit)
	default:
		message = fmt.Sprintf("Spending within limits: $%.2f of $%.2f (%.1f%%)", spend, limit, percentUsed)
	}

	target := "total monthly"
	if deploymentID != "" && deploymentID != "TOTAL" {
		target = fmt.Sprintf("deployment %s", deploymentID)
	}

	return SpendingAlert{
		Level:           level,
		DeploymentID:    deploymentID,
		CurrentSpend:    spend,
		Limit:           limit,
		PercentUsed:     percentUsed,
		Message:         fmt.Sprintf("[%s] %s", target, message),
		Timestamp:       time.Now(),
		AutoTeardownSet: autoTeardown && level == AlertLevelExceeded,
	}
}
