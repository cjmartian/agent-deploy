# Workload & Provider Roadmap

## Vision

Users describe what they built in natural language. The system classifies the workload, selects the best infrastructure shape, picks the cheapest provider that meets requirements, and deploys — all without the user needing to know what ECS, Lambda, or S3 are.

Today the system handles one workload type (web service) on one provider (AWS ECS Fargate). This roadmap defines the path to supporting arbitrary workload types across multiple cloud providers.

## Architecture

### Workload Classification

The planner's first job is to classify what the user is deploying. Classification drives every downstream decision: compute type, networking, storage, triggers, and cost model.

```go
type WorkloadType string

const (
    WorkloadWebService      WorkloadType = "web-service"       // HTTP/HTTPS service with public endpoint
    WorkloadBackgroundWorker WorkloadType = "background-worker" // Long-running process consuming from a queue
    WorkloadScheduledJob     WorkloadType = "scheduled-job"     // Runs on a cron schedule
    WorkloadBatchProcessing  WorkloadType = "batch-processing"  // Processes large datasets, then terminates
    WorkloadStaticSite       WorkloadType = "static-site"       // HTML/CSS/JS served from CDN
    WorkloadMLInference      WorkloadType = "ml-inference"      // Model serving endpoint
    WorkloadDataPipeline     WorkloadType = "data-pipeline"     // Multi-step data transformation
)
```

Classification comes from analyzing the app description with simple keyword/pattern matching:

| Signal in Description | Classified As |
|----------------------|---------------|
| "website", "API", "server", "frontend", "backend", "port" | `web-service` |
| "worker", "consumer", "queue", "process messages", "background" | `background-worker` |
| "every hour", "daily", "cron", "scheduled", "nightly" | `scheduled-job` |
| "process files", "batch", "ETL", "transform data", "import" | `batch-processing` |
| "static site", "HTML", "React app", "no backend", "SPA" | `static-site` |
| "model", "inference", "predictions", "ML", "classify" | `ml-inference` |
| "pipeline", "step function", "workflow", "orchestrate" | `data-pipeline` |

When ambiguous, the agent should ask: "Is this a web service that handles HTTP requests, or a background worker that processes messages?"

### Infrastructure Shapes

Each workload type maps to an infrastructure shape — a set of cloud resources that work together.

```go
type InfraShape struct {
    WorkloadType   WorkloadType
    Compute        ComputeType    // fargate, lambda, batch, lightsail, ec2
    Networking     NetworkingType // public-alb, none, cdn
    Storage        StorageType    // none, s3, efs
    Triggers       []TriggerType  // http, sqs, schedule, s3-event, manual
    HasPublicURL   bool
}
```

| Workload | Compute | Networking | Triggers | Public URL |
|----------|---------|------------|----------|------------|
| Web service | Fargate / Lightsail | ALB or built-in LB | HTTP | Yes |
| Background worker | Fargate / Lambda | None | SQS / SNS | No |
| Scheduled job | Lambda / Fargate | None | EventBridge cron | No |
| Batch processing | AWS Batch / Fargate | None | Manual / S3 event | No |
| Static site | None (S3 origin) | CloudFront CDN | HTTP | Yes |
| ML inference | SageMaker / Lambda | API Gateway / ALB | HTTP | Yes |
| Data pipeline | Step Functions + Lambda | None | Manual / schedule | No |

### Provider Abstraction

The current `Provider` interface registers MCP tools. The multi-provider architecture extends this so the planner can evaluate offerings across clouds.

```go
// ProviderCapability declares what a provider can do.
type ProviderCapability struct {
    Provider        string         // "aws", "gcp", "azure"
    WorkloadTypes   []WorkloadType // What it can deploy
    Regions         []string       // Where
    ComputeOptions  []ComputeType  // How
}

// ProviderRegistry holds all available providers and selects the best one.
type ProviderRegistry struct {
    providers map[string]Provider
}

func (r *ProviderRegistry) SelectProvider(workload WorkloadType, constraints Constraints) (Provider, error)
```

**Provider selection criteria:**

1. Can it handle this workload type?
2. Does it support the requested region?
3. What's the cheapest option meeting latency/availability requirements?
4. User preference (if they said "use GCP", respect that).

## Workload Specs

Each workload type has a dedicated spec file defining the AWS implementation:

| Workload | Spec File | Priority |
|----------|-----------|----------|
| Web service | *(existing — `aws-provider.md`, `lightsail-provider.md`)* | P0 (done) |
| Static site | `workloads/static-site.md` | P1 |
| Background worker | `workloads/background-worker.md` | P1 |
| Scheduled job | `workloads/scheduled-job.md` | P2 |
| Batch processing | `workloads/batch-processing.md` | P2 |
| ML inference | `workloads/ml-inference.md` | P3 |
| Data pipeline | `workloads/data-pipeline.md` | P3 |

**Priority rationale:**
- **P0**: Web services are working today.
- **P1**: Static sites and workers cover the vast majority of remaining use cases. Static sites are also the cheapest path for frontend-only apps.
- **P2**: Scheduled jobs and batch are common but less frequent asks.
- **P3**: ML and pipelines are specialized — build when demand appears.

## Provider Roadmap

| Provider | Priority | Rationale |
|----------|----------|-----------|
| AWS | P0 (current) | Most complete ecosystem, already implemented |
| GCP | P1 | Cloud Run is a strong Lightsail/Fargate alternative |
| Azure | P2 | Azure Container Apps is comparable |
| Fly.io | P2 | Excellent DX, dead-simple container deploys |
| Cloudflare | P3 | Workers for edge compute, Pages for static |

Each new provider implements the `Provider` interface and registers its own MCP tools (e.g., `gcp_plan_infra`, `gcp_deploy`). The planner can then compare across providers for the same workload.

## State Model Evolution

The state model needs to generalize beyond ECS-specific resource tracking:

```go
type Plan struct {
    // ... existing fields ...
    Backend      string       `json:"backend,omitempty"`       // "ecs-fargate", "lightsail", "lambda", "s3-cloudfront", etc.
    WorkloadType WorkloadType `json:"workload_type,omitempty"` // Classified workload type
    Provider     string       `json:"provider,omitempty"`      // "aws", "gcp", "azure" (default: "aws")
}
```

Resource keys in `Infrastructure.Resources` are already flexible (a `map[string]string`), so each workload type can store its own resource identifiers without schema changes.

## User Experience

The conversation should feel natural regardless of workload type:

**Web service (today):**
> "Deploy my portfolio" → plans Lightsail, deploys, returns URL

**Static site:**
> "Deploy this React app, it's just static files" → plans S3 + CloudFront, uploads build, returns CDN URL

**Background worker:**
> "I have a Go service that processes messages from SQS" → plans Fargate + SQS queue, deploys, returns queue URL

**Scheduled job:**
> "Run this Python script every night at 2am" → plans Lambda + EventBridge rule, deploys, confirms schedule

**Batch:**
> "I need to process 10,000 CSV files from S3" → plans AWS Batch, deploys, returns job ID

The agent always shows the plan, cost estimate, and waits for approval before provisioning — same flow, different infrastructure.
