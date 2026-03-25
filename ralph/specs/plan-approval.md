# Plan Approval Specification

## Overview

Before provisioning AWS infrastructure, users must have the opportunity to review the proposed plan and cost estimate. The current implementation auto-approves plans in `createInfra`, bypassing user confirmation entirely. This spec defines the approval workflow.

## Current State (Broken)

`createInfra` silently approves any plan in `created` status:

```go
if plan.Status == state.PlanStatusCreated {
    if err = p.store.ApprovePlan(in.PlanID); err != nil {
        return nil, createInfraOutput{}, err
    }
}
```

This means calling `aws_create_infra` immediately after `aws_plan_infra` provisions resources without any user review of the cost estimate or service selection.

## Requirements

### 1. Explicit Approval Step

`createInfra` must reject plans that have not been explicitly approved. Remove the auto-approval block.

```go
// createInfra must require approved status:
if plan.Status != state.PlanStatusApproved {
    return nil, createInfraOutput{}, fmt.Errorf(
        "%w: plan %s is in '%s' status, must be 'approved'. "+
        "Review the plan and call aws_approve_plan to approve it",
        apperrors.ErrPlanNotApproved, plan.ID, plan.Status,
    )
}
```

### 2. New Tool: `aws_approve_plan`

Add a new MCP tool that explicitly approves a plan after user review.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `plan_id` | string | yes | Plan ID from `aws_plan_infra` |
| `confirmed` | bool | yes | Must be `true` to approve |

**Output:**

| Field | Type | Description |
|-------|------|-------------|
| `plan_id` | string | Plan identifier |
| `status` | string | New status (`approved`) |
| `message` | string | Confirmation message with cost summary |

**Behavior:**

1. Validate plan exists and is in `created` status
2. Require `confirmed: true` (reject if `false` or missing)
3. Transition plan to `approved` status
4. Return confirmation with cost estimate reminder

```go
type approveInput struct {
    PlanID    string `json:"plan_id"    jsonschema:"plan ID to approve"`
    Confirmed bool   `json:"confirmed"  jsonschema:"must be true to confirm approval"`
}

type approveOutput struct {
    PlanID  string `json:"plan_id"`
    Status  string `json:"status"`
    Message string `json:"message"`
}
```

### 3. Plan Summary for Review

When `planInfra` returns a plan, the summary must clearly indicate that approval is required:

```
Proposed plan for "my-app": ECS Fargate in us-east-1, targeting 100 users at ≤200ms p99.
Estimated cost: $47.23/mo. Plan ID: plan-01HX... (expires in 24h).

⚠️ Review the cost estimate above. Call aws_approve_plan with plan_id to approve,
then aws_create_infra to provision infrastructure.
```

### 4. Rejection Support

Allow users to explicitly reject a plan:

```go
// New status constant:
PlanStatusRejected = "rejected"
```

A rejected plan cannot be approved or used for provisioning. The user must create a new plan.

### 5. MCP Prompt Update

Update the `aws_deploy_plan` prompt workflow to include the approval step:

1. Ask clarifying questions about traffic, latency, region
2. Generate infrastructure plan with `aws_plan_infra`
3. **Present plan and cost estimate for user review**
4. **Wait for user to call `aws_approve_plan`**
5. On approval, provision with `aws_create_infra`
6. Deploy with `aws_deploy`
7. Return public URLs

## State Transitions

```
created → approved  (via aws_approve_plan with confirmed: true)
created → rejected  (via aws_approve_plan with confirmed: false)
created → expired   (24 hours elapsed, via cleanup service)
approved → [used by createInfra]
rejected → [terminal state, cannot transition]
expired  → [terminal state, cannot transition]
```

## Error Responses

| Scenario | Error |
|----------|-------|
| Plan not found | `ErrPlanNotFound` |
| Plan already approved | Idempotent — return success |
| Plan expired | `ErrPlanExpired` |
| Plan rejected | `ErrInvalidState: plan was rejected` |
| `confirmed` is false | `ErrPlanNotApproved: confirmation required` |
| `createInfra` with unapproved plan | `ErrPlanNotApproved: plan must be approved first` |

## File Locations

| File | Changes |
|------|---------|
| `internal/providers/aws.go` | Add `aws_approve_plan` tool; remove auto-approval from `createInfra` |
| `internal/state/types.go` | Add `PlanStatusRejected` constant |
| `internal/errors/errors.go` | Wire existing `ErrPlanNotApproved` (currently unused) |
| `internal/providers/aws_test.go` | Add tests for approval workflow |
