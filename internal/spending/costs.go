// Package spending provides AWS Cost Explorer integration for tracking
// actual spend by deployment.
package spending

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

// CostTracker queries AWS Cost Explorer to track actual spending
// per deployment, filtered by agent-deploy resource tags.
type CostTracker struct {
	client *costexplorer.Client
}

// NewCostTracker creates a CostTracker with the given AWS config.
// Cost Explorer API is only available in us-east-1, regardless of
// where resources are deployed.
func NewCostTracker(cfg aws.Config) *CostTracker {
	// Cost Explorer API is only available in us-east-1
	ceConfig := cfg.Copy()
	ceConfig.Region = "us-east-1"
	client := costexplorer.NewFromConfig(ceConfig)
	return &CostTracker{client: client}
}

// CostSummary contains cost information for a deployment or time period.
type CostSummary struct {
	DeploymentID       string    `json:"deployment_id,omitempty"`
	TotalCostUSD       float64   `json:"total_cost_usd"`
	DailyCostUSD       float64   `json:"daily_cost_usd"`
	ProjectedMonthUSD  float64   `json:"projected_month_usd"`
	StartDate          time.Time `json:"start_date"`
	EndDate            time.Time `json:"end_date"`
	DaysInPeriod       int       `json:"days_in_period"`
	AlertThreshold     bool      `json:"alert_threshold_reached"`
	BudgetExceeded     bool      `json:"budget_exceeded"`
}

// GetDeploymentCosts queries Cost Explorer for costs associated with
// a specific deployment, filtered by the agent-deploy:deployment-id tag.
func (ct *CostTracker) GetDeploymentCosts(ctx context.Context, deploymentID string, startDate, endDate time.Time) (*CostSummary, error) {
	// Format dates as YYYY-MM-DD for Cost Explorer API
	start := startDate.Format("2006-01-02")
	end := endDate.Format("2006-01-02")

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &types.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		Filter: &types.Expression{
			Tags: &types.TagValues{
				Key:    aws.String("agent-deploy:deployment-id"),
				Values: []string{deploymentID},
			},
		},
	}

	output, err := ct.client.GetCostAndUsage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("query cost explorer: %w", err)
	}

	summary := &CostSummary{
		DeploymentID: deploymentID,
		StartDate:    startDate,
		EndDate:      endDate,
	}

	// Sum up costs from all returned time periods
	var totalCost float64
	for _, result := range output.ResultsByTime {
		if cost, ok := result.Total["UnblendedCost"]; ok && cost.Amount != nil {
			if amount, err := strconv.ParseFloat(*cost.Amount, 64); err == nil {
				totalCost += amount
			} else {
				slog.Warn("failed to parse cost amount",
					slog.String("component", "spending"),
					slog.String("amount", *cost.Amount),
					slog.Any("error", err))
			}
		}
	}

	summary.TotalCostUSD = totalCost
	summary.DaysInPeriod = int(endDate.Sub(startDate).Hours() / 24)
	if summary.DaysInPeriod > 0 {
		summary.DailyCostUSD = totalCost / float64(summary.DaysInPeriod)
	}

	// Project end-of-month spend based on daily average
	summary.ProjectedMonthUSD = projectMonthlySpend(summary.DailyCostUSD, endDate)

	return summary, nil
}

// GetTotalMonthlySpend queries Cost Explorer for total spend across all
// agent-deploy tagged resources for the current month.
func (ct *CostTracker) GetTotalMonthlySpend(ctx context.Context) (*CostSummary, error) {
	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	// Cost Explorer end date is exclusive, so use tomorrow
	endDate := now.AddDate(0, 0, 1)

	start := startOfMonth.Format("2006-01-02")
	end := endDate.Format("2006-01-02")

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &types.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: types.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		Filter: &types.Expression{
			Tags: &types.TagValues{
				Key:    aws.String("agent-deploy:created-by"),
				Values: []string{"agent-deploy"},
			},
		},
	}

	output, err := ct.client.GetCostAndUsage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("query cost explorer: %w", err)
	}

	summary := &CostSummary{
		StartDate: startOfMonth,
		EndDate:   now,
	}

	for _, result := range output.ResultsByTime {
		if cost, ok := result.Total["UnblendedCost"]; ok && cost.Amount != nil {
			if amount, err := strconv.ParseFloat(*cost.Amount, 64); err == nil {
				summary.TotalCostUSD += amount
			} else {
				slog.Warn("failed to parse cost amount",
					slog.String("component", "spending"),
					slog.String("amount", *cost.Amount),
					slog.Any("error", err))
			}
		}
	}

	daysElapsed := int(now.Sub(startOfMonth).Hours()/24) + 1
	summary.DaysInPeriod = daysElapsed
	if daysElapsed > 0 {
		summary.DailyCostUSD = summary.TotalCostUSD / float64(daysElapsed)
	}
	summary.ProjectedMonthUSD = projectMonthlySpend(summary.DailyCostUSD, now)

	return summary, nil
}

// GetCostsByDeployment queries Cost Explorer and groups costs by deployment ID.
// Returns a map of deployment_id -> CostSummary for the specified time period.
func (ct *CostTracker) GetCostsByDeployment(ctx context.Context, startDate, endDate time.Time) (map[string]*CostSummary, error) {
	start := startDate.Format("2006-01-02")
	end := endDate.Format("2006-01-02")

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &types.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		Filter: &types.Expression{
			Tags: &types.TagValues{
				Key:    aws.String("agent-deploy:created-by"),
				Values: []string{"agent-deploy"},
			},
		},
		GroupBy: []types.GroupDefinition{
			{
				Type: types.GroupDefinitionTypeTag,
				Key:  aws.String("agent-deploy:deployment-id"),
			},
		},
	}

	output, err := ct.client.GetCostAndUsage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("query cost explorer: %w", err)
	}

	daysInPeriod := int(endDate.Sub(startDate).Hours() / 24)
	results := make(map[string]*CostSummary)

	for _, result := range output.ResultsByTime {
		for _, group := range result.Groups {
			// Extract deployment ID from group keys
			deploymentID := ""
			for _, key := range group.Keys {
				// Key format is "agent-deploy:deployment-id$value"
				if len(key) > 0 {
					deploymentID = key
					// Remove tag key prefix if present
					if len(key) > 27 && key[:27] == "agent-deploy:deployment-id$" {
						deploymentID = key[27:]
					}
				}
			}

			if deploymentID == "" {
				continue
			}

			if _, exists := results[deploymentID]; !exists {
				results[deploymentID] = &CostSummary{
					DeploymentID: deploymentID,
					StartDate:    startDate,
					EndDate:      endDate,
					DaysInPeriod: daysInPeriod,
				}
			}

			if cost, ok := group.Metrics["UnblendedCost"]; ok && cost.Amount != nil {
				if amount, err := strconv.ParseFloat(*cost.Amount, 64); err == nil {
					results[deploymentID].TotalCostUSD += amount
				} else {
					slog.Warn("failed to parse cost amount",
						slog.String("component", "spending"),
						slog.String("amount", *cost.Amount),
						slog.Any("error", err))
				}
			}
		}
	}

	// Calculate daily average and projections for each deployment
	for _, summary := range results {
		if summary.DaysInPeriod > 0 {
			summary.DailyCostUSD = summary.TotalCostUSD / float64(summary.DaysInPeriod)
		}
		summary.ProjectedMonthUSD = projectMonthlySpend(summary.DailyCostUSD, endDate)
	}

	return results, nil
}

// CheckAlerts evaluates spending against limits and returns deployments
// that have reached alert thresholds or exceeded budgets.
func (ct *CostTracker) CheckAlerts(ctx context.Context, limits Limits) ([]CostSummary, error) {
	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endDate := now.AddDate(0, 0, 1)

	costsByDeployment, err := ct.GetCostsByDeployment(ctx, startOfMonth, endDate)
	if err != nil {
		return nil, err
	}

	totalMonthly, err := ct.GetTotalMonthlySpend(ctx)
	if err != nil {
		return nil, err
	}

	var alerts []CostSummary

	// Check per-deployment alerts
	alertThreshold := limits.PerDeploymentUSD * float64(limits.AlertThresholdPercent) / 100.0
	for _, summary := range costsByDeployment {
		if summary.TotalCostUSD >= alertThreshold {
			summary.AlertThreshold = true
		}
		if summary.TotalCostUSD >= limits.PerDeploymentUSD {
			summary.BudgetExceeded = true
		}
		if summary.AlertThreshold || summary.BudgetExceeded {
			alerts = append(alerts, *summary)
		}
	}

	// Check total monthly budget
	monthlyAlertThreshold := limits.MonthlyBudgetUSD * float64(limits.AlertThresholdPercent) / 100.0
	if totalMonthly.TotalCostUSD >= monthlyAlertThreshold {
		totalMonthly.AlertThreshold = true
		totalMonthly.DeploymentID = "TOTAL"
	}
	if totalMonthly.TotalCostUSD >= limits.MonthlyBudgetUSD {
		totalMonthly.BudgetExceeded = true
		totalMonthly.DeploymentID = "TOTAL"
	}
	if totalMonthly.AlertThreshold || totalMonthly.BudgetExceeded {
		alerts = append(alerts, *totalMonthly)
	}

	return alerts, nil
}

// GetDeploymentsOverBudget returns deployment IDs that have exceeded their
// per-deployment budget limit. These are candidates for auto-teardown.
func (ct *CostTracker) GetDeploymentsOverBudget(ctx context.Context, limits Limits) ([]string, error) {
	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endDate := now.AddDate(0, 0, 1)

	costsByDeployment, err := ct.GetCostsByDeployment(ctx, startOfMonth, endDate)
	if err != nil {
		return nil, err
	}

	var overBudget []string
	for deploymentID, summary := range costsByDeployment {
		if summary.TotalCostUSD >= limits.PerDeploymentUSD {
			overBudget = append(overBudget, deploymentID)
			slog.Warn("deployment over budget",
				slog.String("component", "spending"),
				slog.String("deployment_id", deploymentID),
				slog.Float64("total_cost_usd", summary.TotalCostUSD),
				slog.Float64("limit_usd", limits.PerDeploymentUSD))
		}
	}

	return overBudget, nil
}

// projectMonthlySpend calculates projected end-of-month spend based on
// daily average and the date within the month.
func projectMonthlySpend(dailyAvg float64, currentDate time.Time) float64 {
	// Get the number of days in the current month
	year, month, _ := currentDate.Date()
	firstOfNextMonth := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := firstOfNextMonth.AddDate(0, 0, -1).Day()

	return dailyAvg * float64(daysInMonth)
}

// MonitoringReport contains a full spending monitoring report.
type MonitoringReport struct {
	Timestamp          time.Time              `json:"timestamp"`
	TotalMonthlySpend  float64                `json:"total_monthly_spend_usd"`
	ProjectedMonth     float64                `json:"projected_month_usd"`
	MonthlyBudget      float64                `json:"monthly_budget_usd"`
	RemainingBudget    float64                `json:"remaining_budget_usd"`
	BudgetUtilization  float64                `json:"budget_utilization_percent"`
	DeploymentCosts    map[string]*CostSummary `json:"deployment_costs"`
	AlertsTriggered    []CostSummary          `json:"alerts_triggered,omitempty"`
	DeploymentsAtRisk  []string               `json:"deployments_at_risk,omitempty"`
}

// GenerateMonitoringReport creates a comprehensive spending report.
func (ct *CostTracker) GenerateMonitoringReport(ctx context.Context, limits Limits) (*MonitoringReport, error) {
	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endDate := now.AddDate(0, 0, 1)

	report := &MonitoringReport{
		Timestamp:     now,
		MonthlyBudget: limits.MonthlyBudgetUSD,
	}

	// Get total monthly spend
	totalMonthly, err := ct.GetTotalMonthlySpend(ctx)
	if err != nil {
		return nil, fmt.Errorf("get total monthly spend: %w", err)
	}
	report.TotalMonthlySpend = totalMonthly.TotalCostUSD
	report.ProjectedMonth = totalMonthly.ProjectedMonthUSD
	report.RemainingBudget = limits.MonthlyBudgetUSD - totalMonthly.TotalCostUSD
	if limits.MonthlyBudgetUSD > 0 {
		report.BudgetUtilization = (totalMonthly.TotalCostUSD / limits.MonthlyBudgetUSD) * 100
	}

	// Get per-deployment costs
	costsByDeployment, err := ct.GetCostsByDeployment(ctx, startOfMonth, endDate)
	if err != nil {
		return nil, fmt.Errorf("get costs by deployment: %w", err)
	}
	report.DeploymentCosts = costsByDeployment

	// Check for alerts
	alerts, err := ct.CheckAlerts(ctx, limits)
	if err != nil {
		return nil, fmt.Errorf("check alerts: %w", err)
	}
	report.AlertsTriggered = alerts

	// Identify at-risk deployments (approaching limit)
	riskThreshold := limits.PerDeploymentUSD * 0.9 // 90% of limit
	for deploymentID, summary := range costsByDeployment {
		if summary.TotalCostUSD >= riskThreshold && summary.TotalCostUSD < limits.PerDeploymentUSD {
			report.DeploymentsAtRisk = append(report.DeploymentsAtRisk, deploymentID)
		}
	}

	return report, nil
}
