# Cost Estimation Specification

## Overview

Accurate cost estimation is critical for spending safeguards to function. The system currently uses hardcoded cost values for plan estimates and fake current-spend calculations. This spec defines how to replace both with real AWS data.

## Current State (Broken)

### Hardcoded Plan Estimates

`planInfra` uses hardcoded formulas instead of the AWS Pricing API:

```go
baseCost := 15.0                        // VPC, basic networking
ecsCost  := float64(expectedUsers) * 0.02
albCost  := 20.0
```

These values do not reflect actual AWS pricing, which varies by region, instance type, and usage patterns.

### Hardcoded Current Spend

`createInfra` calculates current monthly spend as `$25 × active deployments` instead of querying the existing Cost Explorer integration:

```go
for _, d := range deployments {
    if d.Status == DeploymentStatusRunning {
        currentSpend += 25.0
    }
}
```

The `CostTracker` in `internal/spending/costs.go` already implements `GetTotalMonthlySpend()` and `GetDeploymentCosts()` via the Cost Explorer API, but these are never called from the provisioning path.

## Requirements

### 1. AWS Pricing API Integration

Add the AWS Pricing API (`github.com/aws/aws-sdk-go-v2/service/pricing`) to estimate costs based on actual regional pricing.

#### Services to Price

| Service | Pricing Dimensions |
|---------|-------------------|
| ECS Fargate | vCPU-hours, GB-hours (based on CPU/memory config) |
| ALB | LCU-hours, fixed hourly rate |
| NAT Gateway | Hourly rate, data processing per GB |
| CloudWatch Logs | Ingestion per GB, storage per GB |
| Data Transfer | Outbound per GB |
| ECR | Storage per GB |

#### Pricing Lookup

```go
type PricingEstimator struct {
    client *pricing.Client
    cache  map[string]cachedPrice // region+service -> price, with TTL
}

type CostEstimate struct {
    Services       []ServiceCost `json:"services"`
    TotalMonthlyUSD float64      `json:"total_monthly_usd"`
    Region          string       `json:"region"`
    Assumptions     []string     `json:"assumptions"` // e.g., "730 hours/month"
}

type ServiceCost struct {
    Service     string  `json:"service"`
    Description string  `json:"description"`
    MonthlyCost float64 `json:"monthly_cost_usd"`
}
```

#### Estimation Logic

For a given plan:

1. Look up ECS Fargate pricing for the plan's region
2. Calculate compute cost: `(cpu_units / 1024) × vCPU_price × 730 hours × desired_count`
3. Calculate memory cost: `(memory_mb / 1024) × GB_price × 730 hours × desired_count`
4. Look up ALB pricing: fixed hourly + estimated LCU-hours based on expected users
5. Add CloudWatch Logs estimate based on expected log volume
6. Add NAT Gateway costs if private subnets are used
7. Sum all service costs

#### Pricing Cache

- Cache pricing data for 24 hours (prices change infrequently)
- Store cache in memory with TTL
- Fall back to hardcoded regional estimates if Pricing API is unavailable
- Log a warning when using fallback values

```go
type cachedPrice struct {
    price     float64
    fetchedAt time.Time
}

const priceCacheTTL = 24 * time.Hour
```

### 2. Wire Cost Explorer for Current Spend

Replace the hardcoded `$25/deployment` calculation in `createInfra` with actual Cost Explorer data.

#### Implementation

```go
// In createInfra, replace hardcoded spend calculation:
tracker := spending.NewCostTracker(cfg)
currentSpend, err := tracker.GetTotalMonthlySpend(ctx)
if err != nil {
    // Fall back to estimate from local state if Cost Explorer unavailable.
    // Log warning but don't block provisioning.
    slog.Warn("could not query Cost Explorer, using local estimate", logging.Err(err))
    currentSpend = estimateFromLocalState(deployments)
}
```

#### Fallback Strategy

If Cost Explorer is unavailable (first 24h of a new account, permissions missing, etc.):

1. Sum `EstimatedCostMo` from all active deployments in local state
2. Log a warning that actual costs may differ
3. Include a disclaimer in the plan output

### 3. Cost Estimate Output

Update `planInfra` output to include a detailed cost breakdown:

```json
{
  "plan_id": "plan-xxx",
  "services": ["VPC", "ECS Fargate", "ALB", "CloudWatch Logs"],
  "estimated_cost_monthly": "$47.23",
  "cost_breakdown": [
    {"service": "ECS Fargate (0.25 vCPU, 512 MB)", "monthly_cost": "$14.26"},
    {"service": "ALB", "monthly_cost": "$22.27"},
    {"service": "CloudWatch Logs", "monthly_cost": "$0.70"},
    {"service": "NAT Gateway", "monthly_cost": "$10.00"}
  ],
  "assumptions": [
    "730 hours/month (24/7 operation)",
    "Pricing for us-east-1 as of 2026-03-24",
    "1 task replica"
  ],
  "disclaimer": "Estimate based on AWS Pricing API. Actual costs may vary based on usage."
}
```

### 4. go.mod Dependency

Add the AWS Pricing API SDK:

```
github.com/aws/aws-sdk-go-v2/service/pricing
```

The Pricing API is only available in `us-east-1` and `ap-south-1`. The client must override the region regardless of where resources are deployed.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Pricing API unavailable | Fall back to hardcoded regional estimates; log warning |
| Cost Explorer unavailable | Fall back to local state estimates; log warning |
| Unknown service type | Omit from estimate; note in assumptions |
| Pricing data stale (>24h) | Refresh cache; use stale data if refresh fails |
| IAM permissions missing for pricing:GetProducts | Fall back to hardcoded estimates; log warning |

## File Locations

| File | Changes |
|------|---------|
| `internal/spending/pricing.go` | New: PricingEstimator with cache |
| `internal/providers/aws.go` | Update planInfra to use PricingEstimator; update createInfra to use CostTracker |
| `go.mod` | Add `service/pricing` dependency |
| `internal/spending/pricing_test.go` | New: tests with mocked Pricing API |
