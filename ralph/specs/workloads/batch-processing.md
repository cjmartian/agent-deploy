# Batch Processing Workload Specification

## Overview

Batch processing workloads ingest a large dataset, process it, and terminate. Unlike background workers (which run indefinitely polling a queue), batch jobs are finite — they have a clear start and end.

Examples: CSV imports, image resizing of an entire S3 bucket, report generation, database migrations, ML training data preprocessing.

## Examples

User descriptions that should classify as `batch-processing`:

- "Process 10,000 CSV files from S3"
- "Resize all images in this bucket"
- "Run a one-time data migration"
- "Generate monthly reports from our database"
- "Transform and load data into our warehouse"

## Infrastructure Shape (AWS)

| Component | Service | Purpose |
|-----------|---------|---------|
| Compute | AWS Batch (Fargate) | Runs job containers, manages job queue |
| Storage | S3 | Input/output data |
| Logging | CloudWatch Logs | Job output |
| Orchestration | AWS Batch Job Queue + Compute Env | Manages job scheduling and compute |

### Cost Profile

| Config | Cost |
|--------|------|
| Small job (0.25 vCPU, 30 min) | ~$0.01 |
| Medium job (1 vCPU, 2 hours) | ~$0.10 |
| Large job (4 vCPU, 8 hours) | ~$1.50 |
| Parallel (10 × 1 vCPU, 1 hour each) | ~$0.50 |

Batch processing is pay-per-use — zero cost when no jobs are running. AWS Batch itself is free; you only pay for the underlying Fargate compute.

## Requirements

### 1. AWS Batch Environment Provisioning

Create a Fargate-based compute environment and job queue.

```go
import "github.com/aws/aws-sdk-go-v2/service/batch"

func (p *AWSProvider) provisionBatchEnvironment(
    ctx context.Context,
    cfg aws.Config,
    infraID string,
) (computeEnvARN string, jobQueueARN string, err error)
```

**Steps:**

1. Create a Fargate compute environment:
   ```go
   _, err := batchClient.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
       ComputeEnvironmentName: aws.String(fmt.Sprintf("agent-deploy-%s", infraID[:12])),
       Type:                   batchtypes.CETypeManaged,
       State:                  batchtypes.CEStateEnabled,
       ComputeResources: &batchtypes.ComputeResource{
           Type:      batchtypes.CRTypeFargate,
           MaxvCpus:  aws.Int32(16),
           Subnets:   subnets,             // Public subnets (no NAT needed for FARGATE)
           SecurityGroupIds: []string{sgID},
       },
   })
   ```

2. Create a job queue linked to the compute environment:
   ```go
   _, err := batchClient.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
       JobQueueName: aws.String(fmt.Sprintf("agent-deploy-%s", infraID[:12])),
       State:        batchtypes.JQStateEnabled,
       Priority:     aws.Int32(1),
       ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{{
           ComputeEnvironment: aws.String(computeEnvARN),
           Order:              aws.Int32(1),
       }},
   })
   ```

3. Wait for both to reach `VALID` state.

### 2. Job Definition

Register a Batch job definition from the container image.

```go
func (p *AWSProvider) registerBatchJobDefinition(
    ctx context.Context,
    cfg aws.Config,
    imageURI string,
    infraID string,
    cpu string,
    memory string,
) (jobDefARN string, err error) {
    _, err = batchClient.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
        JobDefinitionName: aws.String(fmt.Sprintf("agent-deploy-%s", infraID[:12])),
        Type:              batchtypes.JobDefinitionTypeContainer,
        PlatformCapabilities: []batchtypes.PlatformCapability{
            batchtypes.PlatformCapabilityFargate,
        },
        ContainerProperties: &batchtypes.ContainerProperties{
            Image: aws.String(imageURI),
            ResourceRequirements: []batchtypes.ResourceRequirement{
                {Type: batchtypes.ResourceTypeVcpu, Value: aws.String(cpu)},
                {Type: batchtypes.ResourceTypeMemory, Value: aws.String(memory)},
            },
            ExecutionRoleArn: aws.String(executionRoleARN),
            LogConfiguration: &batchtypes.LogConfiguration{
                LogDriver: batchtypes.LogDriverAwslogs,
            },
        },
    })
    return
}
```

### 3. Job Submission

Deploy triggers job submission rather than creating a persistent service.

```go
func (p *AWSProvider) submitBatchJob(
    ctx context.Context,
    cfg aws.Config,
    jobQueueARN string,
    jobDefARN string,
    deployID string,
    envVars map[string]string,
) (jobID string, err error) {
    var envOverrides []batchtypes.KeyValuePair
    for k, v := range envVars {
        envOverrides = append(envOverrides, batchtypes.KeyValuePair{
            Name:  aws.String(k),
            Value: aws.String(v),
        })
    }

    out, err := batchClient.SubmitJob(ctx, &batch.SubmitJobInput{
        JobName:       aws.String(fmt.Sprintf("agent-deploy-%s", deployID[:12])),
        JobQueue:      aws.String(jobQueueARN),
        JobDefinition: aws.String(jobDefARN),
        ContainerOverrides: &batchtypes.ContainerOverrides{
            Environment: envOverrides,
        },
    })
    if err != nil {
        return "", err
    }
    return aws.ToString(out.JobId), nil
}
```

### 4. Array Jobs (Parallel Processing)

For large datasets, support array jobs that run N copies in parallel:

```go
func (p *AWSProvider) submitBatchArrayJob(
    ctx context.Context,
    batchClient *batch.Client,
    jobQueueARN string,
    jobDefARN string,
    arraySize int,
) (jobID string, err error) {
    out, err := batchClient.SubmitJob(ctx, &batch.SubmitJobInput{
        JobName:       aws.String("agent-deploy-array"),
        JobQueue:      aws.String(jobQueueARN),
        JobDefinition: aws.String(jobDefARN),
        ArrayProperties: &batchtypes.ArrayProperties{
            Size: aws.Int32(int32(arraySize)),
        },
    })
    return aws.ToString(out.JobId), err
}
```

Each array child gets `AWS_BATCH_JOB_ARRAY_INDEX` (0 to N-1) to know which slice of data to process.

### 5. S3 Data Bucket (Optional)

If the job processes files, optionally provision an S3 bucket for input/output:

```go
func (p *AWSProvider) provisionDataBucket(
    ctx context.Context,
    cfg aws.Config,
    infraID string,
) (bucketName string, err error)
```

The job's IAM role gets `s3:GetObject`, `s3:PutObject`, `s3:ListBucket` on the bucket.

### 6. Status Output

```json
{
  "deployment_id": "deploy-01HX...",
  "status": "running",
  "workload_type": "batch-processing",
  "job": {
    "id": "abc123-def456",
    "status": "RUNNING",
    "started_at": "2026-03-29T14:00:00Z",
    "progress": "processing item 4,523 of 10,000",
    "array": {
      "size": 10,
      "succeeded": 6,
      "running": 3,
      "failed": 1
    }
  },
  "compute": {
    "type": "batch-fargate",
    "cpu": "1024",
    "memory": "2048"
  }
}
```

### 7. Teardown

1. Cancel any running jobs (`TerminateJob`).
2. Deregister the job definition.
3. Delete the job queue (set state to DISABLED first, wait, then delete).
4. Delete the compute environment (set state to DISABLED first, wait, then delete).
5. Delete the S3 data bucket (only if empty, or confirm with user).
6. Delete IAM roles and CloudWatch log groups.

### 8. State Resources

```go
const (
    ResourceBatchComputeEnv = "batch_compute_env"  // Compute environment ARN
    ResourceBatchJobQueue   = "batch_job_queue"     // Job queue ARN
    ResourceBatchJobDef     = "batch_job_def"       // Job definition ARN
    ResourceBatchJobID      = "batch_job_id"        // Latest submitted job ID
    ResourceDataBucket      = "data_bucket"         // S3 bucket name
)
```

## Changes Required

| File | Change |
|------|--------|
| `internal/state/types.go` | Add batch resource constants |
| `internal/providers/aws.go` | Add `provisionBatchEnvironment()`, `registerBatchJobDefinition()`, `submitBatchJob()` |
| `internal/providers/aws.go` | Update infra/deploy/status/teardown to handle batch workloads |
| `internal/awsclient/interfaces.go` | Add Batch and S3 client interfaces |
| `go.mod` | Add `github.com/aws/aws-sdk-go-v2/service/batch` |
