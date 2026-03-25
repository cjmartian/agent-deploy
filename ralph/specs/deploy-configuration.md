# Deploy Configuration Specification

## Overview

Several deployment parameters are hardcoded in the AWS provider. This spec defines how to make ECS task resources, log retention, and the default container image configurable via tool inputs.

## Current State

| Parameter | Hardcoded Value | Location |
|-----------|----------------|----------|
| ECS Task CPU | `"256"` | `aws.go:createTaskDefinition` |
| ECS Task Memory | `"512"` | `aws.go:createTaskDefinition` |
| CloudWatch Log Retention | `7` days | `aws.go:provisionLogGroup` |
| Default Container Image | `nginx:latest` | `aws.go:createTaskDefinition` |

## Requirements

### 1. Configurable CPU and Memory

Add `cpu` and `memory` parameters to the `aws_deploy` tool.

**Input Parameters:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `cpu` | string | no | `"256"` | ECS Fargate CPU units |
| `memory` | string | no | `"512"` | ECS Fargate memory in MB |

**Valid Fargate Combinations:**

| CPU (units) | Memory (MB) |
|-------------|-------------|
| 256 | 512, 1024, 2048 |
| 512 | 1024, 2048, 3072, 4096 |
| 1024 | 2048, 3072, 4096, 5120, 6144, 7168, 8192 |
| 2048 | 4096–16384 (in 1024 increments) |
| 4096 | 8192–30720 (in 1024 increments) |

**Validation:**

Reject invalid CPU/memory combinations before calling the ECS API:

```go
type ResourceConfig struct {
    CPU    string `json:"cpu"`
    Memory string `json:"memory"`
}

// ValidateFargateResources checks that the CPU/memory combination is valid.
func ValidateFargateResources(cpu, memory string) error
```

Return a clear error listing valid combinations when an invalid pair is provided.

### 2. Configurable Log Retention

Add `log_retention_days` parameter to `aws_create_infra` or as a global configuration.

**Input Parameter:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `log_retention_days` | int | no | `7` | CloudWatch log retention period |

**Valid Values:**

CloudWatch only accepts specific retention values: `1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653`.

**Validation:**

```go
var validRetentionDays = map[int32]bool{
    1: true, 3: true, 5: true, 7: true, 14: true, 30: true,
    60: true, 90: true, 120: true, 150: true, 180: true, 365: true,
    // ... extended values
}

func ValidateLogRetention(days int) error
```

### 3. Required Container Image

Remove the `nginx:latest` default. The `image_ref` field on `aws_deploy` should be truly required — fail with a clear error if empty.

```go
// Current behavior:
image := imageRef
if image == "" {
    image = "nginx:latest"  // silent default
}

// Required behavior:
if strings.TrimSpace(imageRef) == "" {
    return nil, deployOutput{}, fmt.Errorf("image_ref is required: specify a container image (e.g., 'myapp:latest', '123456789.dkr.ecr.us-east-1.amazonaws.com/myapp:v1')")
}
```

### 4. Updated Deploy Input

```go
type deployInput struct {
    InfraID         string            `json:"infra_id"         jsonschema:"infrastructure ID to deploy onto,required"`
    ImageRef        string            `json:"image_ref"        jsonschema:"container image reference,required"`
    ContainerPort   int               `json:"container_port"   jsonschema:"container port (default: 80)"`
    HealthCheckPath string            `json:"health_check_path" jsonschema:"ALB health check path (default: /)"`
    DesiredCount    int               `json:"desired_count"    jsonschema:"number of task replicas (default: 1)"`
    CPU             string            `json:"cpu"              jsonschema:"Fargate CPU units: 256, 512, 1024, 2048, 4096 (default: 256)"`
    Memory          string            `json:"memory"           jsonschema:"Fargate memory in MB (default: 512)"`
    Environment     map[string]string `json:"environment"      jsonschema:"environment variables for the container"`
}
```

### 5. Updated CreateInfra Input

```go
type createInfraInput struct {
    PlanID           string `json:"plan_id"            jsonschema:"plan ID from aws_plan_infra,required"`
    LogRetentionDays int    `json:"log_retention_days" jsonschema:"CloudWatch log retention in days (default: 7)"`
}
```

### 6. Cost Impact

CPU and memory configuration directly affects cost estimates. The cost estimation (see `cost-estimation.md`) must use the configured values rather than assuming 256 CPU / 512 MB.

Store chosen CPU/memory in the `Plan` or `Infrastructure` record so cost estimates can reference them.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Invalid CPU/memory combination | Reject with error listing valid combinations |
| Invalid log retention value | Reject with error listing valid values |
| Empty `image_ref` | Reject with error explaining requirement |
| CPU/memory exceeds budget | Surface through spending safeguards (existing flow) |

## File Locations

| File | Changes |
|------|---------|
| `internal/providers/aws.go` | Add CPU, memory, log_retention_days params; remove nginx default; add validation |
| `internal/providers/aws_test.go` | Test valid/invalid CPU+memory combos, image_ref validation |
