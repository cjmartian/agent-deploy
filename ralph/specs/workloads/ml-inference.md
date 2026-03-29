# ML Inference Workload Specification

## Overview

ML inference workloads serve a trained model behind an HTTP endpoint. A client sends input data (text, images, structured data), the model produces predictions, and the response is returned. The infrastructure must handle variable request volumes, potentially with GPU acceleration, while keeping cold starts low.

## Examples

User descriptions that should classify as `ml-inference`:

- "Deploy my TensorFlow model as an API"
- "Serve this PyTorch model for image classification"
- "I have an ML model that needs a prediction endpoint"
- "Deploy a model that classifies text sentiment"
- "Serve this LLM for inference"

## Infrastructure Shape (AWS)

Two tiers depending on model complexity:

### Tier 1: Lightweight Models (CPU-only)

| Component | Service | Purpose |
|-----------|---------|---------|
| Compute | Lambda (container) or Fargate | Runs inference |
| API | API Gateway or ALB | HTTP endpoint |
| Model Storage | S3 | Model artifact storage |
| Logging | CloudWatch Logs | Request/response logging |

Best for: scikit-learn, small TensorFlow/PyTorch models, ONNX, < 10 GB model.

### Tier 2: Heavy Models (GPU, large models)

| Component | Service | Purpose |
|-----------|---------|---------|
| Compute | SageMaker Inference Endpoint | GPU-backed model serving |
| API | SageMaker endpoint (built-in) | HTTP endpoint |
| Model Storage | S3 | Model artifact storage |
| Logging | CloudWatch Logs | Request/response logging |

Best for: Large neural networks, LLMs, models requiring GPU, > 10 GB model.

### Cost Profile

| Config | Monthly Cost |
|--------|-------------|
| Lambda CPU (light traffic) | ~$1-10 |
| Fargate CPU (always-on) | ~$10-30 |
| SageMaker `ml.t3.medium` (CPU) | ~$50 |
| SageMaker `ml.g4dn.xlarge` (GPU) | ~$530 |
| SageMaker Serverless (pay-per-request) | ~$1-50 (varies) |

### Tier Selection

```go
func selectMLComputeTier(description string, modelSizeMB int) MLComputeTier {
    if containsAny(description, "GPU", "LLM", "large language model", "transformer", "diffusion") {
        return MLTierSageMaker
    }
    if modelSizeMB > 500 {
        return MLTierSageMaker
    }
    if containsAny(description, "real-time", "low latency", "always on") {
        return MLTierFargate
    }
    return MLTierLambda // cheapest default
}
```

## Requirements

### 1. Model Artifact Handling

Models must be uploaded to S3 for both Lambda/Fargate (baked into container) and SageMaker (loaded from S3).

```go
func (p *AWSProvider) uploadModelArtifact(
    ctx context.Context,
    cfg aws.Config,
    modelPath string,
    infraID string,
) (s3URI string, err error)
```

For container-based inference (Lambda/Fargate), the model is typically baked into the Docker image. The Dockerfile should `COPY` the model into the image.

For SageMaker, the model must be a tarball in S3:
1. Package model files into `model.tar.gz`
2. Upload to `s3://agent-deploy-{infraID[:12]}-models/model.tar.gz`

### 2. SageMaker Endpoint Provisioning

```go
import "github.com/aws/aws-sdk-go-v2/service/sagemaker"

func (p *AWSProvider) createSageMakerEndpoint(
    ctx context.Context,
    cfg aws.Config,
    infraID string,
    modelS3URI string,
    imageURI string,
    instanceType string,
) (endpointName string, err error)
```

**Steps:**

1. Create a SageMaker Model (references the container image and S3 model artifact).
2. Create an Endpoint Configuration (instance type, count).
3. Create an Endpoint (triggers instance provisioning).
4. Wait for endpoint to reach `InService` status.

```go
modelName := fmt.Sprintf("agent-deploy-%s", infraID[:12])

// 1. Create Model
_, err = smClient.CreateModel(ctx, &sagemaker.CreateModelInput{
    ModelName:        aws.String(modelName),
    ExecutionRoleArn: aws.String(roleARN),
    PrimaryContainer: &smtypes.ContainerDefinition{
        Image:        aws.String(imageURI),
        ModelDataUrl: aws.String(modelS3URI),
    },
})

// 2. Create Endpoint Config
endpointConfigName := modelName + "-config"
_, err = smClient.CreateEndpointConfig(ctx, &sagemaker.CreateEndpointConfigInput{
    EndpointConfigName: aws.String(endpointConfigName),
    ProductionVariants: []smtypes.ProductionVariant{{
        VariantName:          aws.String("primary"),
        ModelName:            aws.String(modelName),
        InstanceType:         smtypes.ProductionVariantInstanceType(instanceType),
        InitialInstanceCount: aws.Int32(1),
    }},
})

// 3. Create Endpoint
endpointName = modelName + "-endpoint"
_, err = smClient.CreateEndpoint(ctx, &sagemaker.CreateEndpointInput{
    EndpointName:       aws.String(endpointName),
    EndpointConfigName: aws.String(endpointConfigName),
})
```

### 3. SageMaker Serverless Option

For intermittent traffic, use SageMaker Serverless Inference (no idle cost):

```go
ProductionVariants: []smtypes.ProductionVariant{{
    VariantName:   aws.String("primary"),
    ModelName:     aws.String(modelName),
    ServerlessConfig: &smtypes.ProductionVariantServerlessConfig{
        MaxConcurrency: aws.Int32(5),
        MemorySizeInMB: aws.Int32(2048),
    },
}},
```

**Trade-off:** Cold starts of 30-60 seconds. Use real-time endpoints for latency-sensitive workloads.

### 4. Lambda/Fargate Inference (Lightweight)

For small CPU-only models, deploy as a regular web service using the existing container deploy path but with inference-specific optimizations:

- Container health check at `/ping` (SageMaker convention)
- Inference at `/invocations` (SageMaker convention) or custom path
- Memory sized to model: at least 2× model file size

### 5. Status Output

```json
{
  "deployment_id": "deploy-01HX...",
  "status": "running",
  "workload_type": "ml-inference",
  "endpoint": {
    "url": "https://runtime.sagemaker.us-east-1.amazonaws.com/endpoints/agent-deploy-abc123-endpoint/invocations",
    "status": "InService",
    "instance_type": "ml.g4dn.xlarge",
    "instance_count": 1
  },
  "model": {
    "s3_uri": "s3://agent-deploy-abc123-models/model.tar.gz",
    "size_mb": 350
  },
  "metrics": {
    "invocations_last_hour": 142,
    "avg_latency_ms": 45,
    "error_rate": 0.002
  }
}
```

### 6. Teardown

1. Delete the SageMaker Endpoint.
2. Delete the Endpoint Configuration.
3. Delete the SageMaker Model.
4. Delete the S3 model bucket (confirm with user if large).
5. Delete IAM roles.
6. Delete CloudWatch log groups.

**Warning:** SageMaker GPU instances are expensive. The agent should proactively suggest teardown when done: "Your SageMaker endpoint costs ~$530/mo. Want to tear it down when you're done testing?"

### 7. State Resources

```go
const (
    ResourceSageMakerModel          = "sagemaker_model"           // Model name
    ResourceSageMakerEndpointConfig = "sagemaker_endpoint_config" // Endpoint config name
    ResourceSageMakerEndpoint       = "sagemaker_endpoint"        // Endpoint name
    ResourceModelBucket             = "model_bucket"              // S3 bucket for model artifacts
    ResourceModelS3URI              = "model_s3_uri"              // S3 URI of model tarball
)
```

## Changes Required

| File | Change |
|------|--------|
| `internal/state/types.go` | Add ML inference resource constants |
| `internal/providers/aws.go` | Add `createSageMakerEndpoint()`, `uploadModelArtifact()`, `selectMLComputeTier()` |
| `internal/providers/aws.go` | Update planner to detect ML workloads and estimate SageMaker costs |
| `internal/providers/aws.go` | Update deploy/status/teardown for SageMaker resources |
| `internal/awsclient/interfaces.go` | Add SageMaker client interface |
| `go.mod` | Add `github.com/aws/aws-sdk-go-v2/service/sagemaker`, `github.com/aws/aws-sdk-go-v2/service/sagemakerruntime` |
