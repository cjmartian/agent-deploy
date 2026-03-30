// Package spending provides spending limits configuration and budget checks.
package spending

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cjmartian/agent-deploy/internal/logging"
)

// Limits defines user spending thresholds.
type Limits struct {
	MonthlyBudgetUSD      float64 `json:"monthly_budget_usd"`
	PerDeploymentUSD      float64 `json:"per_deployment_usd"`
	AlertThresholdPercent int     `json:"alert_threshold_percent"`
}

// LimitsWithSource includes spending limits and whether they were explicitly configured.
// WHY: P1.36 - Spec requires confirmation when using defaults, not explicit user config.
type LimitsWithSource struct {
	Limits
	ExplicitlyConfigured bool // True if limits came from config file or env vars
}

// DefaultLimits returns reasonable defaults when no configuration is set.
func DefaultLimits() Limits {
	return Limits{
		MonthlyBudgetUSD:      100.0,
		PerDeploymentUSD:      25.0,
		AlertThresholdPercent: 80,
	}
}

// LoadLimits loads spending limits from environment variables and config file.
// Environment variables take precedence over config file values.
func LoadLimits() (Limits, error) {
	result, err := LoadLimitsWithSource()
	return result.Limits, err
}

// LoadLimitsWithSource loads spending limits and indicates whether they were explicitly configured.
// WHY: P1.36 - Spec requires confirmation when no limits are explicitly configured.
// Returns ExplicitlyConfigured=true if limits came from config file or environment variables.
func LoadLimitsWithSource() (LimitsWithSource, error) {
	limits := DefaultLimits()
	explicitlyConfigured := false

	// Try to load from config file.
	if err := loadFromConfigFile(&limits); err != nil {
		// Config file is optional; log but don't fail.
		// Only log if the error is NOT "file not found" (file missing is expected).
		if !os.IsNotExist(err) {
			log := logging.WithComponent(logging.ComponentSpending)
			log.Warn("could not load config file, using defaults",
				slog.String("reason", err.Error()))
		}
	} else {
		// Config file loaded successfully.
		explicitlyConfigured = true
	}

	// Override with environment variables.
	if v := os.Getenv("AGENT_DEPLOY_MONTHLY_BUDGET"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			limits.MonthlyBudgetUSD = f
			explicitlyConfigured = true
		}
	}
	if v := os.Getenv("AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			limits.PerDeploymentUSD = f
			explicitlyConfigured = true
		}
	}
	if v := os.Getenv("AGENT_DEPLOY_ALERT_THRESHOLD"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			limits.AlertThresholdPercent = i
			explicitlyConfigured = true
		}
	}

	return LimitsWithSource{
		Limits:               limits,
		ExplicitlyConfigured: explicitlyConfigured,
	}, nil
}

// Config represents the full configuration file structure.
type Config struct {
	SpendingLimits Limits `json:"spending_limits"`
}

func loadFromConfigFile(limits *Limits) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(home, ".agent-deploy", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Only override non-zero values from config.
	if cfg.SpendingLimits.MonthlyBudgetUSD > 0 {
		limits.MonthlyBudgetUSD = cfg.SpendingLimits.MonthlyBudgetUSD
	}
	if cfg.SpendingLimits.PerDeploymentUSD > 0 {
		limits.PerDeploymentUSD = cfg.SpendingLimits.PerDeploymentUSD
	}
	if cfg.SpendingLimits.AlertThresholdPercent > 0 {
		limits.AlertThresholdPercent = cfg.SpendingLimits.AlertThresholdPercent
	}

	return nil
}
