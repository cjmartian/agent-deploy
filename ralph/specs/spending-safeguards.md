# Spending Safeguards Specification

## Overview

Per project requirements, the system must "ensure spend does not cross some boundary set by the user." This spec defines how spending limits are configured, enforced, and monitored.

## Configuration

### User-Defined Limits

Users can set spending limits at multiple levels:

```json
{
  "spending_limits": {
    "monthly_budget_usd": 100.00,
    "per_deployment_usd": 25.00,
    "alert_threshold_percent": 80
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `monthly_budget_usd` | float | Maximum monthly spend across all deployments |
| `per_deployment_usd` | float | Maximum spend per individual deployment |
| `alert_threshold_percent` | int | Alert when reaching this % of budget |

### Configuration Methods

1. **Environment variables:**
   - `AGENT_DEPLOY_MONTHLY_BUDGET`
   - `AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET`

2. **Configuration file:** `~/.agent-deploy/config.json`

3. **Per-request override:** Pass limits in tool arguments

## Enforcement Points

### Pre-Provisioning Check

Before `aws_create_infra` provisions resources:

1. Get estimated monthly cost from plan
2. Compare against `per_deployment_usd` limit
3. Compare against remaining `monthly_budget_usd`
4. **Block provisioning if limits would be exceeded**
5. Return clear error message with cost breakdown

```go
type SpendingCheckResult struct {
    Allowed          bool    `json:"allowed"`
    EstimatedCost    float64 `json:"estimated_cost_usd"`
    RemainingBudget  float64 `json:"remaining_budget_usd"`
    Reason           string  `json:"reason,omitempty"`
}
```

### Runtime Monitoring

While deployments are active:

1. Query AWS Cost Explorer API periodically
2. Track actual vs. estimated spend
3. Alert at threshold percentage
4. **Auto-teardown option** if hard limit exceeded

## Error Responses

When spending limits are exceeded:

```json
{
  "error": "SPENDING_LIMIT_EXCEEDED",
  "message": "Estimated cost $45.00/mo exceeds per-deployment limit of $25.00",
  "details": {
    "estimated_cost_monthly": 45.00,
    "per_deployment_limit": 25.00,
    "monthly_remaining": 55.00
  }
}
```

## AWS Cost Integration

### Cost Estimation (Pre-Deploy)

Use AWS Pricing API to estimate costs:
- EC2/ECS compute hours
- Data transfer
- ALB request pricing
- CloudWatch logs/metrics

### Cost Tracking (Runtime)

Use AWS Cost Explorer API:
- Filter by resource tags (deployment_id)
- Track daily/monthly accruals
- Project end-of-month spend

## Resource Tagging

All provisioned resources must be tagged for cost tracking:

```json
{
  "agent-deploy:deployment-id": "deploy-xxx",
  "agent-deploy:infra-id": "infra-xxx",
  "agent-deploy:plan-id": "plan-xxx",
  "agent-deploy:created-by": "agent-deploy"
}
```

## Safeguard Behaviors

| Scenario | Behavior |
|----------|----------|
| Estimated cost > per-deployment limit | Block provisioning, return error |
| Estimated cost > remaining monthly budget | Block provisioning, return error |
| Actual spend reaches alert threshold | Emit warning, continue running |
| Actual spend exceeds monthly budget | Alert user, optionally teardown |
| User has no limits configured | Warn but allow (with confirmation) |
