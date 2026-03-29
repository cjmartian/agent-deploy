# Background Worker Workload Specification

## Overview

A background worker is a long-running process that consumes messages from a queue, processes them, and optionally produces output. Workers have no public HTTP endpoint — they pull work from a queue and run indefinitely.

This is one of the most common non-web workload patterns: email senders, image processors, event handlers, data enrichment pipelines, webhook relayers.

## Examples

User descriptions that should classify as `background-worker`:

- "I have a Go service that processes messages from SQS"
- "Deploy a worker that consumes events and writes to a database"
- "This app listens on a queue and sends emails"
- "Background job processor for my web app"

## Infrastructure Shape (AWS)

| Component | Service | Purpose |
|-----------|---------|---------|
| Compute | ECS Fargate (or Lambda for short tasks) | Runs the worker container |
| Queue | SQS (Standard or FIFO) | Message ingest |
| Dead letter queue | SQS | Failed message capture |
| Logging | CloudWatch Logs | Worker output |
| Networking | VPC private subnet (optional) | Only if worker needs VPC resources |

**No ALB, no public endpoint, no NAT Gateway** (unless the worker needs to call external APIs, in which case NAT is required).

### Cost Profile

| Config | Monthly Cost |
|--------|-------------|
| Fargate Nano (0.25 vCPU, 512 MB) + SQS | ~$10 |
| Lambda (event-driven, low volume) + SQS | ~$1-3 |
| Fargate + NAT (needs external API access) | ~$43 |

SQS itself is essentially free at low volume (1M requests/mo free tier, $0.40/million after).

## Requirements

### 1. Queue Provisioning

When deploying a background worker, the system provisions an SQS queue and a dead letter queue.

```go
func (p *AWSProvider) provisionWorkerQueue(
    ctx context.Context,
    cfg aws.Config,
    infraID string,
    fifo bool,
) (queueURL string, queueARN string, dlqURL string, dlqARN string, err error)
```

**Steps:**

1. Create a dead letter queue: `agent-deploy-{infraID[:12]}-dlq`
2. Create the main queue: `agent-deploy-{infraID[:12]}`
3. Set redrive policy on the main queue pointing to the DLQ (maxReceiveCount: 3)
4. Return queue URL and ARN for both

```go
import "github.com/aws/aws-sdk-go-v2/service/sqs"

func (p *AWSProvider) provisionWorkerQueue(
    ctx context.Context,
    cfg aws.Config,
    infraID string,
    fifo bool,
) (string, string, string, string, error) {
    sqsClient := sqs.NewFromConfig(cfg)

    queueName := fmt.Sprintf("agent-deploy-%s", infraID[:12])
    dlqName := fmt.Sprintf("agent-deploy-%s-dlq", infraID[:12])

    if fifo {
        queueName += ".fifo"
        dlqName += ".fifo"
    }

    // Create DLQ first
    dlqAttrs := map[string]string{
        "MessageRetentionPeriod": "1209600", // 14 days
    }
    if fifo {
        dlqAttrs["FifoQueue"] = "true"
    }

    dlqOut, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
        QueueName:  aws.String(dlqName),
        Attributes: dlqAttrs,
    })
    if err != nil {
        return "", "", "", "", fmt.Errorf("create DLQ: %w", err)
    }

    // Get DLQ ARN
    dlqAttrOut, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
        QueueUrl:       dlqOut.QueueUrl,
        AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
    })
    if err != nil {
        return "", "", "", "", fmt.Errorf("get DLQ ARN: %w", err)
    }
    dlqARN := dlqAttrOut.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]

    // Create main queue with redrive policy
    redrivePolicy := fmt.Sprintf(`{"maxReceiveCount":"3","deadLetterTargetArn":"%s"}`, dlqARN)
    queueAttrs := map[string]string{
        "RedrivePolicy":         redrivePolicy,
        "VisibilityTimeout":     "300", // 5 minutes default
        "MessageRetentionPeriod": "345600", // 4 days
    }
    if fifo {
        queueAttrs["FifoQueue"] = "true"
        queueAttrs["ContentBasedDeduplication"] = "true"
    }

    queueOut, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
        QueueName:  aws.String(queueName),
        Attributes: queueAttrs,
    })
    if err != nil {
        return "", "", "", "", fmt.Errorf("create queue: %w", err)
    }

    queueAttrOut, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
        QueueUrl:       queueOut.QueueUrl,
        AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
    })
    if err != nil {
        return "", "", "", "", fmt.Errorf("get queue ARN: %w", err)
    }
    mainARN := queueAttrOut.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]

    return aws.ToString(queueOut.QueueUrl), mainARN, aws.ToString(dlqOut.QueueUrl), dlqARN, nil
}
```

### 2. Compute Options

#### Option A: ECS Fargate (long-running worker)

For workers that run continuously (polling a queue in a loop):

- Deploy as an ECS service with `desiredCount: 1` (or more for parallelism)
- **No ALB or target group** — the service has no public endpoint
- Pass the queue URL as an environment variable
- The container polls SQS, processes messages, deletes them

```go
func (p *AWSProvider) createWorkerTaskDefinition(
    ctx context.Context,
    cfg aws.Config,
    imageURI string,
    queueURL string,
    infraID string,
) (taskDefARN string, err error)
```

The task definition includes:
- `SQS_QUEUE_URL` environment variable
- `AWS_REGION` environment variable
- IAM role with `sqs:ReceiveMessage`, `sqs:DeleteMessage`, `sqs:GetQueueAttributes` permissions on the queue ARN

#### Option B: Lambda (event-driven, short tasks)

For workers where each message takes < 15 minutes:

- Create a Lambda function from the container image
- Configure an SQS event source mapping
- Lambda scales automatically with queue depth
- No ECS, no Fargate, no VPC needed

```go
func (p *AWSProvider) createWorkerLambda(
    ctx context.Context,
    cfg aws.Config,
    imageURI string,
    queueARN string,
    infraID string,
) (functionARN string, err error)
```

**Selection between Fargate and Lambda:**

| Signal | Fargate | Lambda |
|--------|---------|--------|
| "long-running", "persistent", "always on" | Yes | No |
| "event-driven", "quick processing" | No | Yes |
| Processing time > 15 min per message | Yes | No |
| Default (ambiguous) | Yes | No |

### 3. IAM Role for Queue Access

The worker's execution role needs SQS permissions:

```go
func (p *AWSProvider) createWorkerRole(
    ctx context.Context,
    cfg aws.Config,
    queueARN string,
    dlqARN string,
    infraID string,
) (roleARN string, err error)
```

Policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueAttributes",
        "sqs:ChangeMessageVisibility"
      ],
      "Resource": ["<queue-arn>", "<dlq-arn>"]
    },
    {
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "<log-group-arn>"
    }
  ]
}
```

### 4. Networking Decision

Workers don't need public inbound access. The networking depends on what the worker calls:

| Worker Needs | Networking |
|-------------|------------|
| Only SQS (AWS service) | No VPC needed — use awsvpc with public subnet or VPC endpoints |
| External APIs | NAT Gateway or public subnet |
| VPC resources (RDS, ElastiCache) | Private subnet in existing VPC |

Default: **no VPC** (cheapest). The planner should ask if the worker needs to connect to anything beyond SQS.

### 5. Status Output

Workers don't have URLs. Status reports queue metrics instead:

```json
{
  "deployment_id": "deploy-01HX...",
  "status": "running",
  "workload_type": "background-worker",
  "queue": {
    "url": "https://sqs.us-east-1.amazonaws.com/123456789/agent-deploy-abc123",
    "messages_available": 42,
    "messages_in_flight": 3,
    "dlq_messages": 0
  },
  "compute": {
    "type": "fargate",
    "running_tasks": 1,
    "cpu": "256",
    "memory": "512"
  }
}
```

### 6. Teardown

1. Stop the ECS service (set desiredCount to 0) or delete the Lambda function.
2. Wait for running tasks to drain (respect in-flight messages).
3. Delete the SQS queue and DLQ.
4. Delete the IAM role and policies.
5. Delete the CloudWatch log group.
6. Clean up VPC resources if they were created.

**Important:** Warn the user if the queue has unprocessed messages before deleting. The agent should confirm: "The queue has 42 unprocessed messages. Delete anyway?"

### 7. State Resources

```go
const (
    ResourceSQSQueue    = "sqs_queue"     // Queue URL
    ResourceSQSQueueARN = "sqs_queue_arn" // Queue ARN
    ResourceSQSDLQ      = "sqs_dlq"       // Dead letter queue URL
    ResourceSQSDLQARN   = "sqs_dlq_arn"   // DLQ ARN
    ResourceWorkerRole  = "worker_role"   // IAM role ARN for queue access
    ResourceLambdaFunc  = "lambda_func"   // Lambda function ARN (if using Lambda)
)
```

## Changes Required

| File | Change |
|------|--------|
| `internal/state/types.go` | Add worker resource constants |
| `internal/providers/aws.go` | Add `provisionWorkerQueue()`, `createWorkerTaskDefinition()`, `createWorkerRole()` |
| `internal/providers/aws.go` | Update `createInfra()` to branch on workload type |
| `internal/providers/aws.go` | Update `deploy()` to handle worker deployments (no ALB) |
| `internal/providers/aws.go` | Update `status()` to show queue metrics |
| `internal/providers/aws.go` | Update `teardown()` to clean up SQS and worker resources |
| `internal/awsclient/interfaces.go` | Add SQS client interface |
| `internal/awsclient/mocks/sqs.go` | Mock SQS client |
| `go.mod` | Add `github.com/aws/aws-sdk-go-v2/service/sqs` |

## Dependencies

```
github.com/aws/aws-sdk-go-v2/service/sqs
github.com/aws/aws-sdk-go-v2/service/lambda (if Lambda compute option)
```
