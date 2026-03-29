# Scheduled Job Workload Specification

## Overview

A scheduled job is a task that runs on a cron schedule — nightly data exports, hourly health checks, weekly reports, periodic cleanup. It starts, does its work, and terminates. There is no long-running process.

## Examples

User descriptions that should classify as `scheduled-job`:

- "Run this Python script every night at 2am"
- "Execute a data export every hour"
- "I need a cron job that cleans up old records weekly"
- "Run this backup script on a schedule"

## Infrastructure Shape (AWS)

| Component | Service | Purpose |
|-----------|---------|---------|
| Trigger | EventBridge Scheduler | Cron/rate schedule |
| Compute | Lambda (< 15 min) or ECS Fargate (> 15 min) | Runs the job |
| Logging | CloudWatch Logs | Job output and errors |
| Alerting | SNS (optional) | Notify on failure |

**No ALB, no SQS, no public endpoint, no VPC** (unless the job needs VPC resources).

### Cost Profile

| Config | Monthly Cost |
|--------|-------------|
| Lambda (runs 5 min/day) | ~$0.01 |
| Lambda (runs 1 hr/day) | ~$1-2 |
| Fargate (runs 30 min/day) | ~$0.50 |
| Fargate (runs 2 hr/day) | ~$2-3 |

Scheduled jobs are extremely cheap because you only pay for compute time, not idle time.

## Requirements

### 1. Schedule Parsing

The planner must extract a schedule from the user's description and convert it to an EventBridge cron or rate expression.

```go
type ScheduleConfig struct {
    Expression string // EventBridge cron or rate expression
    Timezone   string // IANA timezone (default: UTC)
    Enabled    bool   // Start enabled or paused
}

func parseSchedule(description string) (*ScheduleConfig, error)
```

**Natural language mapping:**

| User Says | EventBridge Expression |
|-----------|----------------------|
| "every hour" | `rate(1 hour)` |
| "every 5 minutes" | `rate(5 minutes)` |
| "every day at 2am" | `cron(0 2 * * ? *)` |
| "every night at midnight" | `cron(0 0 * * ? *)` |
| "every Monday at 9am" | `cron(0 9 ? * MON *)` |
| "weekdays at 6pm" | `cron(0 18 ? * MON-FRI *)` |
| "first of every month" | `cron(0 0 1 * ? *)` |

When the schedule is ambiguous, the agent should confirm: "I'll set this to run at 2:00 AM UTC daily. Want a different timezone or schedule?"

### 2. EventBridge Scheduler

Create an EventBridge schedule that triggers the compute target.

```go
import "github.com/aws/aws-sdk-go-v2/service/scheduler"

func (p *AWSProvider) createSchedule(
    ctx context.Context,
    cfg aws.Config,
    infraID string,
    schedule ScheduleConfig,
    targetARN string,
    roleARN string,
) error {
    schedulerClient := scheduler.NewFromConfig(cfg)

    scheduleName := fmt.Sprintf("agent-deploy-%s", infraID[:12])

    _, err := schedulerClient.CreateSchedule(ctx, &scheduler.CreateScheduleInput{
        Name:               aws.String(scheduleName),
        ScheduleExpression: aws.String(schedule.Expression),
        ScheduleExpressionTimezone: aws.String(schedule.Timezone),
        State:              schedulertypes.ScheduleStateEnabled,
        FlexibleTimeWindow: &schedulertypes.FlexibleTimeWindow{
            Mode: schedulertypes.FlexibleTimeWindowModeOff,
        },
        Target: &schedulertypes.Target{
            Arn:     aws.String(targetARN),
            RoleArn: aws.String(roleARN),
            // RetryPolicy for failed invocations
            RetryPolicy: &schedulertypes.RetryPolicy{
                MaximumRetryAttempts: aws.Int32(2),
            },
        },
    })
    return err
}
```

### 3. Compute Selection

#### Option A: Lambda (default, tasks < 15 min)

- Package container image as a Lambda function
- EventBridge invokes Lambda directly
- Cheapest option, zero idle cost

#### Option B: ECS Fargate (tasks > 15 min)

- EventBridge triggers an ECS `RunTask` call
- Task runs to completion and stops
- Use `FARGATE_SPOT` for cheaper compute (jobs tolerate interruption since they'll retry)

**Selection logic:**

```go
func selectJobCompute(description string, estimatedDuration time.Duration) ComputeType {
    if estimatedDuration > 15*time.Minute {
        return ComputeFargate
    }
    // Check for signals suggesting long runtime
    if containsAny(description, "large dataset", "hours", "long running", "batch") {
        return ComputeFargate
    }
    return ComputeLambda
}
```

### 4. IAM Role

The scheduler needs a role to invoke the compute target:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "lambda:InvokeFunction",
      "Resource": "<function-arn>"
    }
  ]
}
```

Or for ECS:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "ecs:RunTask",
      "Resource": "<task-definition-arn>"
    },
    {
      "Effect": "Allow",
      "Action": "iam:PassRole",
      "Resource": "<execution-role-arn>"
    }
  ]
}
```

### 5. Status Output

```json
{
  "deployment_id": "deploy-01HX...",
  "status": "running",
  "workload_type": "scheduled-job",
  "schedule": {
    "expression": "cron(0 2 * * ? *)",
    "timezone": "America/New_York",
    "next_run": "2026-03-30T02:00:00-04:00",
    "last_run": "2026-03-29T02:00:00-04:00",
    "last_status": "succeeded"
  },
  "compute": {
    "type": "lambda",
    "avg_duration_ms": 45000,
    "last_30_days": {
      "invocations": 30,
      "failures": 0
    }
  }
}
```

### 6. Teardown

1. Delete the EventBridge schedule.
2. Delete the Lambda function or ECS task definition.
3. Delete the IAM role and policies.
4. Delete the CloudWatch log group.

No queue to drain, no confirmation needed — scheduled jobs are stateless.

### 7. State Resources

```go
const (
    ResourceScheduleName     = "schedule_name"      // EventBridge schedule name
    ResourceScheduleARN      = "schedule_arn"        // EventBridge schedule ARN
    ResourceScheduleExpr     = "schedule_expression" // Cron/rate expression
    ResourceScheduleTimezone = "schedule_timezone"   // IANA timezone
    ResourceScheduleRole     = "schedule_role"       // IAM role for scheduler
)
```

## Changes Required

| File | Change |
|------|--------|
| `internal/state/types.go` | Add schedule resource constants |
| `internal/providers/aws.go` | Add `createSchedule()`, `parseSchedule()`, `selectJobCompute()` |
| `internal/providers/aws.go` | Update `createInfra()` for scheduled-job workload |
| `internal/providers/aws.go` | Update `status()` to show schedule info and execution history |
| `internal/providers/aws.go` | Update `teardown()` to clean up EventBridge and compute resources |
| `internal/awsclient/interfaces.go` | Add EventBridge Scheduler and Lambda client interfaces |
| `go.mod` | Add `github.com/aws/aws-sdk-go-v2/service/scheduler`, `github.com/aws/aws-sdk-go-v2/service/lambda` |
