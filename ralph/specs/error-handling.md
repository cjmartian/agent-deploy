# Error Handling Specification

## Overview

The system has inconsistent error handling: some errors break `errors.Is()` chains, three domain error types are defined but never used, and partial infrastructure failures leave orphaned AWS resources. This spec defines error handling standards and partial failure rollback.

## Current Issues

### 1. Error Wrapping Breaks errors.Is()

At `aws.go:226`, errors are wrapped inconsistently:

```go
// Broken: fmt.Errorf with %v instead of %w
return nil, createInfraOutput{}, fmt.Errorf("budget exceeded: %v", check.Reason)
```

This prevents callers from using `errors.Is(err, apperrors.ErrBudgetExceeded)`. All error wrapping must use `%w`:

```go
return nil, createInfraOutput{}, fmt.Errorf("%w: %s", apperrors.ErrBudgetExceeded, check.Reason)
```

### 2. Unused Domain Error Types

Three error types in `internal/errors/errors.go` are defined but never used:

| Error | Intended Use | Status |
|-------|-------------|--------|
| `ErrPlanNotApproved` | Plan approval (P1.13) | Unused — wire when implementing plan approval |
| `ErrProvisioningFailed` | Partial failure rollback | Unused — wire in createInfra |
| `ErrInvalidState` | Invalid state transitions | Unused — wire in store operations |

### 3. No Partial Failure Rollback

When `createInfra` fails mid-provisioning, already-created resources are orphaned. For example, if ALB creation succeeds but the execution role fails:

- VPC, subnets, IGW, route tables, security groups, ECS cluster, ALB, and target group exist in AWS
- Infrastructure status is set to `failed`
- No cleanup of the already-created resources occurs
- User must manually teardown or wait for reconciliation

## Requirements

### 1. Consistent Error Wrapping

All error returns that wrap domain errors must use `%w` for proper `errors.Is()` chains:

```go
// Pattern: wrap domain errors with %w
return fmt.Errorf("%w: %s", apperrors.ErrBudgetExceeded, details)
return fmt.Errorf("%w: plan %s expired", apperrors.ErrPlanExpired, planID)

// Pattern: wrap AWS SDK errors with %w and context
return fmt.Errorf("create VPC: %w", err)

// Pattern: wrap multiple layers
return fmt.Errorf("%w: provision VPC: %w", apperrors.ErrProvisioningFailed, err)
```

### 2. Wire Unused Error Types

#### ErrPlanNotApproved

Use in `createInfra` when a plan hasn't been approved (see `plan-approval.md`):

```go
if plan.Status != state.PlanStatusApproved {
    return nil, createInfraOutput{}, fmt.Errorf("%w: plan %s is in '%s' status",
        apperrors.ErrPlanNotApproved, plan.ID, plan.Status)
}
```

#### ErrProvisioningFailed

Wrap provisioning errors in `createInfra`:

```go
if err := p.provisionVPC(ctx, cfg, infra, tags); err != nil {
    rollbackErr := p.rollbackInfra(ctx, cfg, infra)
    return nil, createInfraOutput{}, fmt.Errorf("%w: %s (rollback: %v)",
        apperrors.ErrProvisioningFailed, err, rollbackErr)
}
```

#### ErrInvalidState

Use in state store for invalid transitions:

```go
func (s *Store) ApprovePlan(id string) error {
    plan, err := s.GetPlan(id)
    if err != nil {
        return err
    }
    if plan.Status != PlanStatusCreated {
        return fmt.Errorf("%w: cannot approve plan in '%s' status",
            apperrors.ErrInvalidState, plan.Status)
    }
    // ...
}
```

### 3. Partial Failure Rollback

When `createInfra` fails after creating some resources, roll back the already-created resources.

#### Rollback Function

```go
// rollbackInfra tears down any resources recorded in the infrastructure record.
// This is called when provisioning fails partway through.
// Errors are logged but do not prevent continued rollback attempts.
func (p *AWSProvider) rollbackInfra(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
    var rollbackErrors []string

    // Delete in reverse order of creation.
    // Only attempt deletion for resources that exist in the infra record.

    if infra.Resources[state.ResourceLogGroup] != "" {
        if err := p.deleteLogGroup(ctx, cfg, infra); err != nil {
            rollbackErrors = append(rollbackErrors, "log group: "+err.Error())
        }
    }

    if infra.Resources[state.ResourceExecutionRole] != "" {
        if err := p.deleteExecutionRole(ctx, cfg, infra); err != nil {
            rollbackErrors = append(rollbackErrors, "execution role: "+err.Error())
        }
    }

    if infra.Resources[state.ResourceALB] != "" {
        if err := p.deleteALB(ctx, cfg, infra); err != nil {
            rollbackErrors = append(rollbackErrors, "ALB: "+err.Error())
        }
    }

    if infra.Resources[state.ResourceECSCluster] != "" {
        if err := p.deleteECSCluster(ctx, cfg, infra); err != nil {
            rollbackErrors = append(rollbackErrors, "ECS cluster: "+err.Error())
        }
    }

    if infra.Resources[state.ResourceVPC] != "" {
        if err := p.deleteVPCResources(ctx, cfg, infra); err != nil {
            rollbackErrors = append(rollbackErrors, "VPC: "+err.Error())
        }
    }

    // Mark infra as destroyed after rollback.
    _ = p.store.SetInfraStatus(infra.ID, state.InfraStatusDestroyed)

    if len(rollbackErrors) > 0 {
        return fmt.Errorf("partial rollback: %s", strings.Join(rollbackErrors, "; "))
    }
    return nil
}
```

#### createInfra Integration

```go
func (p *AWSProvider) createInfra(ctx context.Context, ...) (...) {
    // ... setup ...

    if err := p.provisionVPC(ctx, cfg, infra, tags); err != nil {
        if rollbackErr := p.rollbackInfra(ctx, cfg, infra); rollbackErr != nil {
            slog.Error("rollback failed", logging.Err(rollbackErr), logging.InfraID(infra.ID))
        }
        _ = p.store.SetInfraStatus(infraID, state.InfraStatusFailed)
        return nil, createInfraOutput{}, fmt.Errorf("%w: provision VPC: %w",
            apperrors.ErrProvisioningFailed, err)
    }

    if err := p.provisionECSCluster(ctx, cfg, infra, tags); err != nil {
        if rollbackErr := p.rollbackInfra(ctx, cfg, infra); rollbackErr != nil {
            slog.Error("rollback failed", logging.Err(rollbackErr), logging.InfraID(infra.ID))
        }
        _ = p.store.SetInfraStatus(infraID, state.InfraStatusFailed)
        return nil, createInfraOutput{}, fmt.Errorf("%w: provision ECS: %w",
            apperrors.ErrProvisioningFailed, err)
    }

    // ... continue for each step ...
}
```

### 4. Error Response Format

Tool errors returned to MCP clients should be structured consistently:

```json
{
  "error": "PROVISIONING_FAILED",
  "message": "Failed to create ALB: AccessDenied: insufficient permissions",
  "details": {
    "step": "provision_alb",
    "infra_id": "infra-xxx",
    "rollback_status": "completed",
    "resources_cleaned": ["vpc", "ecs_cluster", "subnets"]
  }
}
```

## Error Categories

| Category | Error Type | HTTP-like Code | Auto-Rollback |
|----------|-----------|----------------|---------------|
| Validation | `ErrPlanNotApproved`, input errors | 400 | No |
| Not Found | `ErrPlanNotFound`, `ErrInfraNotFound`, `ErrDeploymentNotFound` | 404 | No |
| State | `ErrInvalidState`, `ErrPlanExpired` | 409 | No |
| Budget | `ErrBudgetExceeded` | 403 | No |
| Provisioning | `ErrProvisioningFailed` | 500 | Yes |

## File Locations

| File | Changes |
|------|---------|
| `internal/providers/aws.go` | Add `rollbackInfra`; wire rollback in `createInfra`; fix `%v` → `%w` |
| `internal/errors/errors.go` | No changes needed (types already defined) |
| `internal/state/store.go` | Use `ErrInvalidState` for invalid transitions |
| `internal/providers/aws_test.go` | Test rollback on partial failure; test error wrapping |
