// Package spending provides pre-provisioning budget checks.
package spending

import (
	"fmt"
)

// CheckResult contains the result of a budget check.
type CheckResult struct {
	Allowed         bool    `json:"allowed"`
	EstimatedCost   float64 `json:"estimated_cost_usd"`
	RemainingBudget float64 `json:"remaining_budget_usd"`
	Reason          string  `json:"reason,omitempty"`
}

// CheckBudget validates that the estimated cost is within spending limits.
// It checks both per-deployment and remaining monthly budget.
func CheckBudget(estimatedCostMo float64, limits Limits, currentMonthlySpend float64) CheckResult {
	result := CheckResult{
		EstimatedCost:   estimatedCostMo,
		RemainingBudget: limits.MonthlyBudgetUSD - currentMonthlySpend,
	}

	// Check per-deployment limit.
	if estimatedCostMo > limits.PerDeploymentUSD {
		result.Allowed = false
		result.Reason = fmt.Sprintf(
			"Estimated cost $%.2f/mo exceeds per-deployment limit of $%.2f",
			estimatedCostMo, limits.PerDeploymentUSD,
		)
		return result
	}

	// Check remaining monthly budget.
	if estimatedCostMo > result.RemainingBudget {
		result.Allowed = false
		result.Reason = fmt.Sprintf(
			"Estimated cost $%.2f/mo exceeds remaining monthly budget of $%.2f",
			estimatedCostMo, result.RemainingBudget,
		)
		return result
	}

	result.Allowed = true
	return result
}

// Error formats a CheckResult as an error response.
type Error struct {
	Code    string      `json:"error"`
	Message string      `json:"message"`
	Details CheckResult `json:"details"`
}

// NewBudgetError creates a formatted budget exceeded error.
func NewBudgetError(check CheckResult, perDeployLimit float64) *Error {
	return &Error{
		Code:    "SPENDING_LIMIT_EXCEEDED",
		Message: check.Reason,
		Details: check,
	}
}
