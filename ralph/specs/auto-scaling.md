# Auto Scaling Specification

## Overview

ECS services currently run a fixed number of task replicas with no automatic scaling. This spec defines how to configure Application Auto Scaling for ECS Fargate services to scale based on CPU and memory utilization.

## Current State

The ECS cluster is provisioned with `FARGATE` and `FARGATE_SPOT` capacity providers, but no scaling policies are configured. The `DesiredCount` parameter sets a static replica count.

`planInfra` adds "Auto Scaling" to the services list when `expectedUsers > 1000`, but no scaling resources are created.

## Requirements

### 1. Auto Scaling Parameters

Add optional scaling parameters to the `aws_deploy` tool:

```go
type deployInput struct {
    // ... existing fields ...
    MinCount        int `json:"min_count"         jsonschema:"minimum task count for auto scaling (default: same as desired_count)"`
    MaxCount        int `json:"max_count"         jsonschema:"maximum task count for auto scaling (default: same as desired_count)"`
    TargetCPU       int `json:"target_cpu_percent" jsonschema:"target CPU utilization percentage for scaling (default: 70)"`
    TargetMemory    int `json:"target_memory_percent" jsonschema:"target memory utilization percentage for scaling (default: 70)"`
}
```

**Behavior:**
- If `min_count` and `max_count` are both unset or equal to `desired_count`, no scaling is configured (static mode)
- If `max_count > desired_count`, configure auto scaling
- `min_count` defaults to `desired_count`
- `max_count` must be >= `min_count`

### 2. Register Scalable Target

Register the ECS service as a scalable target with Application Auto Scaling:

```go
import "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"

func (p *AWSProvider) configureAutoScaling(ctx context.Context, cfg aws.Config, clusterName, serviceName string, minCount, maxCount int) error {
    asClient := applicationautoscaling.NewFromConfig(cfg)

    resourceID := fmt.Sprintf("service/%s/%s", clusterName, serviceName)

    _, err := asClient.RegisterScalableTarget(ctx, &applicationautoscaling.RegisterScalableTargetInput{
        ServiceNamespace:  types.ServiceNamespaceEcs,
        ResourceId:        aws.String(resourceID),
        ScalableDimension: types.ScalableDimensionECSServiceDesiredCount,
        MinCapacity:       aws.Int32(int32(minCount)),
        MaxCapacity:       aws.Int32(int32(maxCount)),
    })
    return err
}
```

### 3. Target Tracking Scaling Policies

Create target tracking policies for CPU and/or memory:

#### CPU-Based Scaling

```go
_, err := asClient.PutScalingPolicy(ctx, &applicationautoscaling.PutScalingPolicyInput{
    PolicyName:        aws.String("agent-deploy-cpu-" + deployID[:12]),
    ServiceNamespace:  types.ServiceNamespaceEcs,
    ResourceId:        aws.String(resourceID),
    ScalableDimension: types.ScalableDimensionECSServiceDesiredCount,
    PolicyType:        types.PolicyTypeTargetTrackingScaling,
    TargetTrackingScalingPolicyConfiguration: &types.TargetTrackingScalingPolicyConfiguration{
        PredefinedMetricSpecification: &types.PredefinedMetricSpecification{
            PredefinedMetricType: types.MetricTypeECSServiceAverageCPUUtilization,
        },
        TargetValue:      aws.Float64(float64(targetCPU)),
        ScaleInCooldown:  aws.Int32(300), // 5 minutes
        ScaleOutCooldown: aws.Int32(60),  // 1 minute
    },
})
```

#### Memory-Based Scaling

```go
_, err := asClient.PutScalingPolicy(ctx, &applicationautoscaling.PutScalingPolicyInput{
    PolicyName:        aws.String("agent-deploy-memory-" + deployID[:12]),
    ServiceNamespace:  types.ServiceNamespaceEcs,
    ResourceId:        aws.String(resourceID),
    ScalableDimension: types.ScalableDimensionECSServiceDesiredCount,
    PolicyType:        types.PolicyTypeTargetTrackingScaling,
    TargetTrackingScalingPolicyConfiguration: &types.TargetTrackingScalingPolicyConfiguration{
        PredefinedMetricSpecification: &types.PredefinedMetricSpecification{
            PredefinedMetricType: types.MetricTypeECSServiceAverageMemoryUtilization,
        },
        TargetValue:      aws.Float64(float64(targetMemory)),
        ScaleInCooldown:  aws.Int32(300),
        ScaleOutCooldown: aws.Int32(60),
    },
})
```

### 4. Cooldown Periods

| Direction | Cooldown | Rationale |
|-----------|----------|-----------|
| Scale out | 60 seconds | React quickly to load spikes |
| Scale in | 300 seconds | Avoid thrashing on brief load dips |

### 5. Cost Impact

Auto scaling affects cost estimates significantly:

- **Minimum cost:** `min_count × per-task cost` (guaranteed baseline)
- **Maximum cost:** `max_count × per-task cost` (worst case)
- Cost estimates should report both min and max ranges when auto scaling is configured

Update `planInfra` output:

```json
{
  "estimated_cost_monthly": "$47.23–$188.92",
  "cost_range": {
    "minimum": 47.23,
    "maximum": 188.92,
    "note": "Range reflects auto scaling from 1 to 4 tasks"
  }
}
```

### 6. Teardown

Deregister the scalable target and delete scaling policies before deleting the ECS service:

```go
func (p *AWSProvider) deleteAutoScaling(ctx context.Context, cfg aws.Config, clusterName, serviceName string) error {
    asClient := applicationautoscaling.NewFromConfig(cfg)
    resourceID := fmt.Sprintf("service/%s/%s", clusterName, serviceName)

    // Delete scaling policies first
    asClient.DeleteScalingPolicy(ctx, ...)

    // Deregister scalable target
    asClient.DeregisterScalableTarget(ctx, &applicationautoscaling.DeregisterScalableTargetInput{
        ServiceNamespace:  types.ServiceNamespaceEcs,
        ResourceId:        aws.String(resourceID),
        ScalableDimension: types.ScalableDimensionECSServiceDesiredCount,
    })
    return err
}
```

### 7. Status Reporting

`aws_status` should include scaling information when auto scaling is configured:

```json
{
  "deployment_id": "deploy-xxx",
  "status": "running",
  "scaling": {
    "min_count": 1,
    "max_count": 4,
    "current_count": 2,
    "target_cpu_percent": 70,
    "target_memory_percent": 70
  }
}
```

## Dependencies

Add the Application Auto Scaling SDK:

```
github.com/aws/aws-sdk-go-v2/service/applicationautoscaling
```

## Validation Rules

| Rule | Error |
|------|-------|
| `max_count < min_count` | `max_count must be >= min_count` |
| `min_count < 1` | `min_count must be at least 1` |
| `max_count > 10` | Warning (not error): high max may cause cost spikes |
| `target_cpu < 10 or > 90` | `target CPU must be between 10 and 90 percent` |
| `target_memory < 10 or > 90` | `target memory must be between 10 and 90 percent` |

## File Locations

| File | Changes |
|------|---------|
| `internal/providers/aws.go` | Add scaling params, `configureAutoScaling`, `deleteAutoScaling`; update status |
| `go.mod` | Add `service/applicationautoscaling` dependency |
| `internal/providers/aws_test.go` | Test scaling configuration, validation, teardown |
