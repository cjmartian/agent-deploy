# Data Pipeline Workload Specification

## Overview

A data pipeline is a multi-step workflow that extracts, transforms, and loads data. Each step may use different compute (Lambda for lightweight transforms, Fargate for heavy processing, Glue for Spark jobs). The orchestrator manages step ordering, retries, parallelism, and error handling.

## Examples

User descriptions that should classify as `data-pipeline`:

- "I need a pipeline that pulls data from an API, transforms it, and loads it into S3"
- "Orchestrate these three processing steps in sequence"
- "Build an ETL pipeline with error handling and retries"
- "Run step A, then fan out to steps B and C in parallel, then merge in step D"
- "Data workflow that processes events through multiple stages"

## Infrastructure Shape (AWS)

| Component | Service | Purpose |
|-----------|---------|---------|
| Orchestration | Step Functions | Workflow definition, state management, retries |
| Compute (light) | Lambda | Short transforms (< 15 min) |
| Compute (heavy) | ECS Fargate / AWS Batch | Long-running or resource-intensive steps |
| Storage | S3 | Intermediate and final data |
| Logging | CloudWatch Logs | Per-step output |

### Cost Profile

| Config | Monthly Cost |
|--------|-------------|
| Step Functions (1,000 executions/mo) | ~$0.025 |
| Step Functions (100,000 executions/mo) | ~$2.50 |
| Lambda steps (moderate usage) | ~$1-10 |
| Fargate steps (heavy processing) | ~$5-50 |
| S3 intermediate storage | ~$0.023/GB |

Step Functions Express workflows (for high-volume, short pipelines) cost $1/million state transitions.

## Requirements

### 1. Pipeline Definition

The planner must translate the user's description into a Step Functions state machine.

```go
type PipelineStep struct {
    Name        string       // Step identifier
    Type        StepType     // lambda, fargate, choice, parallel, wait
    ImageRef    string       // Container image (for fargate steps)
    Handler     string       // Lambda handler (for lambda steps)
    DependsOn   []string     // Steps that must complete first
    Retry       *RetryConfig // Retry policy
    Catch       *CatchConfig // Error handling
    InputPath   string       // JSONPath to filter input
    OutputPath  string       // JSONPath to filter output
}

type PipelineConfig struct {
    Steps      []PipelineStep
    Trigger    TriggerType   // manual, schedule, s3-event
    Schedule   string        // Cron expression (if triggered on schedule)
    Timeout    time.Duration // Max pipeline execution time
}
```

**Step types:**

| Type | AWS Service | Use When |
|------|-------------|----------|
| `lambda` | Lambda | Step takes < 15 min, lightweight processing |
| `fargate` | ECS RunTask | Step takes > 15 min or needs lots of CPU/memory |
| `choice` | Step Functions Choice | Conditional branching |
| `parallel` | Step Functions Parallel | Fan-out concurrent steps |
| `wait` | Step Functions Wait | Delay between steps |
| `map` | Step Functions Map | Process each item in an array |

### 2. Step Functions State Machine

```go
import "github.com/aws/aws-sdk-go-v2/service/sfn"

func (p *AWSProvider) createStateMachine(
    ctx context.Context,
    cfg aws.Config,
    infraID string,
    pipeline PipelineConfig,
    roleARN string,
) (stateMachineARN string, err error) {
    sfnClient := sfn.NewFromConfig(cfg)

    definition := buildASLDefinition(pipeline) // Amazon States Language JSON

    out, err := sfnClient.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
        Name:       aws.String(fmt.Sprintf("agent-deploy-%s", infraID[:12])),
        Definition: aws.String(definition),
        RoleArn:    aws.String(roleARN),
        Type:       sfntypes.StateMachineTypeStandard,
        LoggingConfiguration: &sfntypes.LoggingConfiguration{
            Level: sfntypes.LogLevelAll,
            Destinations: []sfntypes.LogDestination{{
                CloudWatchLogsLogGroup: &sfntypes.CloudWatchLogsLogGroup{
                    LogGroupArn: aws.String(logGroupARN),
                },
            }},
        },
    })
    return aws.ToString(out.StateMachineArn), err
}
```

### 3. Amazon States Language (ASL) Generation

Convert the `PipelineConfig` into ASL JSON:

```go
func buildASLDefinition(pipeline PipelineConfig) string
```

**Example pipeline:** "Pull data from API → transform → load to S3"

```json
{
  "StartAt": "ExtractData",
  "States": {
    "ExtractData": {
      "Type": "Task",
      "Resource": "arn:aws:lambda:us-east-1:123456789:function:extract",
      "Next": "TransformData",
      "Retry": [
        {
          "ErrorEquals": ["States.TaskFailed"],
          "IntervalSeconds": 30,
          "MaxAttempts": 3,
          "BackoffRate": 2.0
        }
      ]
    },
    "TransformData": {
      "Type": "Task",
      "Resource": "arn:aws:states:::ecs:runTask.sync",
      "Parameters": {
        "LaunchType": "FARGATE",
        "Cluster": "arn:aws:ecs:...",
        "TaskDefinition": "arn:aws:ecs:...",
        "Overrides": {
          "ContainerOverrides": [{
            "Name": "app",
            "Environment": [
              {"Name": "INPUT_KEY", "Value.$": "$.output_key"}
            ]
          }]
        }
      },
      "Next": "LoadData"
    },
    "LoadData": {
      "Type": "Task",
      "Resource": "arn:aws:lambda:us-east-1:123456789:function:load",
      "End": true
    }
  }
}
```

### 4. Per-Step Compute Provisioning

Each step in the pipeline may need its own compute resource:

- **Lambda steps:** Create a Lambda function per step from the container image or code.
- **Fargate steps:** Register an ECS task definition per step. Use a shared ECS cluster.

```go
func (p *AWSProvider) provisionPipelineStep(
    ctx context.Context,
    cfg aws.Config,
    step PipelineStep,
    infraID string,
) (resourceARN string, err error)
```

### 5. Triggers

| Trigger | Implementation |
|---------|---------------|
| Manual | User calls `aws_deploy` to start an execution |
| Schedule | EventBridge rule triggers `StartExecution` |
| S3 event | S3 notification → EventBridge → `StartExecution` |

### 6. Pipeline Execution (Deploy)

"Deploying" a pipeline means creating/updating the state machine. "Running" it means starting an execution.

```go
func (p *AWSProvider) startPipelineExecution(
    ctx context.Context,
    cfg aws.Config,
    stateMachineARN string,
    input string, // JSON input
) (executionARN string, err error) {
    sfnClient := sfn.NewFromConfig(cfg)

    out, err := sfnClient.StartExecution(ctx, &sfn.StartExecutionInput{
        StateMachineArn: aws.String(stateMachineARN),
        Input:           aws.String(input),
    })
    return aws.ToString(out.ExecutionArn), err
}
```

### 7. Status Output

```json
{
  "deployment_id": "deploy-01HX...",
  "status": "running",
  "workload_type": "data-pipeline",
  "state_machine": {
    "arn": "arn:aws:states:us-east-1:123456789:stateMachine:agent-deploy-abc123",
    "status": "ACTIVE"
  },
  "latest_execution": {
    "arn": "arn:aws:states:us-east-1:123456789:execution:agent-deploy-abc123:exec-001",
    "status": "RUNNING",
    "started_at": "2026-03-29T14:00:00Z",
    "current_step": "TransformData",
    "steps": [
      {"name": "ExtractData", "status": "SUCCEEDED", "duration_ms": 4500},
      {"name": "TransformData", "status": "RUNNING", "started_at": "2026-03-29T14:00:05Z"},
      {"name": "LoadData", "status": "NOT_STARTED"}
    ]
  },
  "history": {
    "total_executions": 15,
    "succeeded": 13,
    "failed": 2,
    "avg_duration_ms": 120000
  }
}
```

### 8. Teardown

1. Stop any running executions.
2. Delete the Step Functions state machine.
3. Delete Lambda functions for each step.
4. Delete ECS task definitions for Fargate steps.
5. Delete the ECS cluster (if not shared).
6. Delete S3 intermediate data buckets (confirm with user).
7. Delete EventBridge rules (if scheduled).
8. Delete IAM roles and policies.
9. Delete CloudWatch log groups.

### 9. State Resources

```go
const (
    ResourceStateMachine    = "state_machine"     // Step Functions state machine ARN
    ResourceStateMachineName = "state_machine_name" // State machine name
    ResourcePipelineSteps   = "pipeline_steps"     // JSON array of step resource ARNs
    ResourcePipelineTrigger = "pipeline_trigger"   // Trigger type
)
```

## Changes Required

| File | Change |
|------|--------|
| `internal/state/types.go` | Add pipeline resource constants |
| `internal/providers/aws.go` | Add `createStateMachine()`, `buildASLDefinition()`, `provisionPipelineStep()`, `startPipelineExecution()` |
| `internal/providers/aws.go` | Update planner to detect pipeline workloads |
| `internal/providers/aws.go` | Update deploy (create/start pipelines), status (execution tracking), teardown |
| `internal/awsclient/interfaces.go` | Add Step Functions client interface |
| `go.mod` | Add `github.com/aws/aws-sdk-go-v2/service/sfn` |
