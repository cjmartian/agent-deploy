---
priority: P1
reason: Current minimum deployment cost ($65/mo) is prohibitive for personal projects. This is the highest priority new feature.
---

# Lightsail Container Provider Specification

## Overview

The system currently provisions all deployments using ECS Fargate with a full VPC, private subnets, NAT Gateway, and ALB. This architecture costs ~$55/mo in fixed infrastructure before any compute runs — overkill for personal projects, demos, and low-traffic apps.

AWS Lightsail Container Services offer a dramatically simpler and cheaper deployment path: a single API call provisions compute, load balancing, TLS, and a public endpoint with no VPC management. This spec adds Lightsail as an alternative compute backend that the planner can select automatically based on workload characteristics.

## Design Philosophy

Users deploy with natural language. They should never need to say "use Lightsail" or "use ECS." The planner analyzes the app description, expected traffic, and budget, then picks the right backend. Lightsail is the default for simple, low-traffic workloads. ECS Fargate remains the choice for production workloads that need auto-scaling, private networking, or fine-grained control.

## Current State

| Aspect | Current Behavior |
|--------|-----------------|
| Compute backends | ECS Fargate only |
| Minimum cost | ~$65/mo (NAT $33 + ALB $22 + Fargate $9 + CW $0.53) |
| VPC required | Yes, always |
| Load balancer | ALB, always provisioned |
| Plan selection | No selection — single hardcoded path |

## Requirements

### 1. Lightsail Container Service Basics

Lightsail Container Services bundle compute, load balancing, TLS termination, and a public HTTPS endpoint into a single managed resource.

**Pricing (fixed, predictable):**

| Power | vCPU | RAM | Price/node/mo |
|-------|------|-----|---------------|
| Nano | 0.25 | 0.5 GB | $7 |
| Micro | 0.5 | 1 GB | $10 |
| Small | 1 | 2 GB | $25 |
| Medium | 2 | 4 GB | $50 |
| Large | 4 | 8 GB | $100 |
| XLarge | 8 | 16 GB | $200 |

Each service can run 1–20 nodes (containers) at the chosen power level.

**Included at no extra cost:**
- TLS/HTTPS on a `*.{region}.cs.amazonlightsail.com` endpoint
- Built-in load balancing across nodes
- 500 GB data transfer/mo (per service)

### 2. Plan Selection Logic

Modify `planInfra` to choose between Lightsail and ECS Fargate based on workload signals.

```go
type computeBackend string

const (
	BackendLightsail  computeBackend = "lightsail"
	BackendECSFargate computeBackend = "ecs-fargate"
)

func selectBackend(plan planAnalysis) computeBackend
```

**Selection criteria:**

| Signal | Lightsail | ECS Fargate |
|--------|-----------|-------------|
| Expected users | ≤ 500 | > 500 |
| Auto-scaling required | No | Yes |
| Custom VPC/networking | No | Yes |
| Budget ≤ $25/mo | Yes (preferred) | Not achievable |
| App description mentions "production", "enterprise", "high availability" | No | Yes |
| Multiple services / service mesh | No | Yes |

When signals conflict, prefer the cheaper option unless the user explicitly describes production requirements.

**Updated plan output:**

```json
{
  "plan_id": "plan-01HX...",
  "backend": "lightsail",
  "services": ["Lightsail Container Service"],
  "estimated_cost_monthly": "$7.00",
  "summary": "Lightsail Nano (0.25 vCPU, 512 MB) × 1 node in us-east-1. Includes HTTPS endpoint and load balancing. Estimated cost: $7.00/mo."
}
```

### 3. State Model Changes

Add the compute backend to the `Plan` and `Infrastructure` types.

```go
// In state/types.go

type Plan struct {
    // ... existing fields ...
    Backend string `json:"backend,omitempty"` // "lightsail" or "ecs-fargate" (default: "ecs-fargate")
}

// Lightsail-specific resource keys for Infrastructure.Resources map.
const (
    ResourceLightsailService    = "lightsail_service"     // Service name
    ResourceLightsailEndpoint   = "lightsail_endpoint"    // Public HTTPS endpoint URL
    ResourceLightsailPower      = "lightsail_power"       // Power level (nano, micro, etc.)
    ResourceLightsailNodes      = "lightsail_nodes"       // Number of nodes
)
```

### 4. Lightsail Infrastructure Provisioning

When `createInfra` receives a plan with `backend: "lightsail"`, provision a Lightsail Container Service instead of VPC/ECS/ALB.

```go
func (p *AWSProvider) createLightsailInfra(
    ctx context.Context,
    cfg aws.Config,
    plan *state.Plan,
    infraID string,
) (*state.Infrastructure, error)
```

**Steps:**

1. Select the Lightsail power level based on plan CPU/memory requirements.
2. Call `lightsail:CreateContainerService` with the service name, power, and scale (node count).
3. Wait for the service to reach `READY` state (poll `GetContainerServices`).
4. Store the service name and endpoint in `Infrastructure.Resources`.

```go
import "github.com/aws/aws-sdk-go-v2/service/lightsail"

func (p *AWSProvider) createLightsailInfra(
    ctx context.Context,
    cfg aws.Config,
    plan *state.Plan,
    infraID string,
) (*state.Infrastructure, error) {
    lsClient := lightsail.NewFromConfig(cfg)

    serviceName := fmt.Sprintf("agent-deploy-%s", infraID[:12])
    power, scale := selectLightsailPower(plan.ExpectedUsers, plan.LatencyMS)

    _, err := lsClient.CreateContainerService(ctx, &lightsail.CreateContainerServiceInput{
        ServiceName: aws.String(serviceName),
        Power:       lstypes.ContainerServicePowerName(power),
        Scale:       aws.Int32(int32(scale)),
        Tags: []lstypes.Tag{
            {Key: aws.String("agent-deploy:created-by"), Value: aws.String("agent-deploy")},
            {Key: aws.String("agent-deploy:infra-id"), Value: aws.String(infraID)},
        },
    })
    if err != nil {
        return nil, fmt.Errorf("create lightsail container service: %w", err)
    }

    // Poll until READY
    if err := p.waitForLightsailReady(ctx, lsClient, serviceName); err != nil {
        return nil, err
    }

    return &state.Infrastructure{
        ID:     infraID,
        PlanID: plan.ID,
        Region: plan.Region,
        Status: state.InfraStatusReady,
        Resources: map[string]string{
            ResourceLightsailService: serviceName,
            ResourceLightsailPower:   power,
            ResourceLightsailNodes:   fmt.Sprintf("%d", scale),
        },
    }, nil
}
```

**Power selection logic:**

```go
func selectLightsailPower(expectedUsers, latencyMS int) (power string, scale int) {
    switch {
    case expectedUsers <= 50:
        return "nano", 1    // $7/mo
    case expectedUsers <= 200:
        return "micro", 1   // $10/mo
    case expectedUsers <= 500:
        return "small", 1   // $25/mo
    default:
        return "small", 2   // $50/mo — at this point, ECS is better value
    }
}
```

### 5. Lightsail Deployment

When `deploy` receives an `infra_id` backed by Lightsail, deploy via `CreateContainerServiceDeployment` instead of ECS task definitions.

```go
func (p *AWSProvider) deployToLightsail(
    ctx context.Context,
    cfg aws.Config,
    infra *state.Infrastructure,
    imageRef string,
    deployID string,
) (*state.Deployment, error)
```

**Steps:**

1. Push the container image to the Lightsail container image registry (not ECR).
2. Create a deployment with the container configuration.
3. Wait for the deployment to become active.
4. Return the public HTTPS endpoint as the URL.

#### 5a. Image Push

Lightsail has its own image registry. Images must be pushed via the Lightsail API, not ECR.

```go
func (p *AWSProvider) pushImageToLightsail(
    ctx context.Context,
    lsClient *lightsail.Client,
    serviceName string,
    imageRef string,
) (lightsailImageRef string, err error)
```

Use the AWS CLI under the hood since the SDK doesn't directly support Docker image push to Lightsail:

1. Call `lightsail:RegisterContainerImage` with `--service-name` and `--label`.
2. Alternatively, if the image is in a public registry, Lightsail can pull it directly — skip the push step.

**Image source handling (matches existing pattern):**

| Source | Action |
|--------|--------|
| Local image (`myapp:latest`) | Push via `aws lightsail push-container-image` CLI |
| Public registry (`nginx:latest`, `ghcr.io/...`) | Reference directly in deployment config |
| ECR image | Reference directly (Lightsail can pull from ECR in same account) |

#### 5b. Deployment Creation

```go
func (p *AWSProvider) createLightsailDeployment(
    ctx context.Context,
    lsClient *lightsail.Client,
    serviceName string,
    imageURI string,
    containerPort int,
) error {
    containerName := "app"

    _, err := lsClient.CreateContainerServiceDeployment(ctx,
        &lightsail.CreateContainerServiceDeploymentInput{
            ServiceName: aws.String(serviceName),
            Containers: map[string]lstypes.Container{
                containerName: {
                    Image: aws.String(imageURI),
                    Ports: map[string]lstypes.ContainerServiceProtocol{
                        fmt.Sprintf("%d", containerPort): lstypes.ContainerServiceProtocolHttp,
                    },
                },
            },
            PublicEndpoint: &lstypes.EndpointRequest{
                ContainerName: aws.String(containerName),
                ContainerPort: aws.Int32(int32(containerPort)),
                HealthCheck: &lstypes.ContainerServiceHealthCheckConfig{
                    Path: aws.String("/"),
                },
            },
        })
    return err
}
```

### 6. Custom DNS with Lightsail

Lightsail Container Services support custom domains via `UpdateContainerService` with certificate attachment.

**When `domain_name` is set in the plan:**

1. Provision an ACM certificate **in the Lightsail context** (not standard ACM — Lightsail uses its own certificate API).
2. Call `lightsail:CreateCertificate` with the domain name.
3. Create a DNS validation record in Route 53 (same flow as the ECS path).
4. Wait for the certificate to be validated.
5. Attach the certificate to the container service via `UpdateContainerService`.
6. Create a CNAME record pointing the custom domain to the Lightsail endpoint.

```go
func (p *AWSProvider) configureLightsailDNS(
    ctx context.Context,
    cfg aws.Config,
    serviceName string,
    domainName string,
    lightsailEndpoint string,
) error
```

**Important difference from ECS path:** Lightsail endpoints are not ALBs, so the DNS record should be a **CNAME**, not a Route 53 alias (A record). The CNAME target is the Lightsail endpoint URL (e.g., `my-service.abc123.us-east-1.cs.amazonlightsail.com`).

### 7. Status

`aws_status` must handle both backends transparently.

For Lightsail deployments:

1. Call `lightsail:GetContainerServices` to get service state and endpoint.
2. Call `lightsail:GetContainerServiceDeployments` to get deployment state.
3. Return the same output shape as ECS deployments.

```json
{
  "deployment_id": "deploy-01HX...",
  "status": "running",
  "backend": "lightsail",
  "urls": [
    "https://cjmartian.com",
    "https://agent-deploy-abc123.us-east-1.cs.amazonlightsail.com"
  ],
  "resources": {
    "power": "nano",
    "nodes": 1,
    "monthly_cost": "$7.00"
  }
}
```

### 8. Teardown

`aws_teardown` must handle Lightsail resources.

**Steps:**

1. If custom DNS is configured, delete the CNAME record from Route 53.
2. If a Lightsail certificate was created, delete it via `lightsail:DeleteCertificate`.
3. Call `lightsail:DeleteContainerService` — this removes the service, all deployments, and images.
4. Update state to `destroyed`.

```go
func (p *AWSProvider) teardownLightsail(
    ctx context.Context,
    cfg aws.Config,
    infra *state.Infrastructure,
) error {
    lsClient := lightsail.NewFromConfig(cfg)
    serviceName := infra.Resources[ResourceLightsailService]

    _, err := lsClient.DeleteContainerService(ctx, &lightsail.DeleteContainerServiceInput{
        ServiceName: aws.String(serviceName),
    })
    return err
}
```

Lightsail teardown is much simpler than ECS — one API call removes all compute resources.

### 9. Reconciliation

The reconciler must discover orphaned Lightsail Container Services.

```go
func (r *Reconciler) findOrphanedLightsailServices(ctx context.Context) ([]string, error) {
    lsClient := lightsail.NewFromConfig(r.cfg)

    out, err := lsClient.GetContainerServices(ctx, &lightsail.GetContainerServicesInput{})
    if err != nil {
        return nil, err
    }

    var orphaned []string
    for _, svc := range out.ContainerServices {
        for _, tag := range svc.Tags {
            if aws.ToString(tag.Key) == "agent-deploy:created-by" {
                if !r.store.InfraExists(aws.ToString(svc.ContainerServiceName)) {
                    orphaned = append(orphaned, aws.ToString(svc.ContainerServiceName))
                }
            }
        }
    }
    return orphaned, nil
}
```

### 10. Cost Estimation

Lightsail pricing is fixed and predictable — no need for the Pricing API.

```go
var lightsailPricing = map[string]float64{
    "nano":   7.0,
    "micro":  10.0,
    "small":  25.0,
    "medium": 50.0,
    "large":  100.0,
    "xlarge": 200.0,
}

func estimateLightsailCost(power string, scale int) float64 {
    return lightsailPricing[power] * float64(scale)
}
```

The plan output should clearly show this is cheaper:

```
Proposed plan: Lightsail Container Service (Nano: 0.25 vCPU, 512 MB) × 1 node in us-east-1.
Estimated cost: $7.00/mo (vs. ~$65/mo with ECS Fargate).
Includes: HTTPS endpoint, load balancing, 500 GB data transfer.
```

## Limitations

Document these so the agent can communicate trade-offs to the user:

| Limitation | Impact |
|------------|--------|
| No auto-scaling | Fixed node count; must manually update |
| No VPC integration | Cannot access private resources (RDS in VPC, etc.) |
| Max 20 nodes | Ceiling of ~20 containers per service |
| Limited regions | Not available in all AWS regions |
| No Fargate Spot | No spot pricing option |
| 500 GB transfer cap | Overage at $0.09/GB |
| Single container port | One exposed port per container definition |

## Migration Path

If a user's app outgrows Lightsail, the agent should suggest upgrading to ECS Fargate:

1. The image is already containerized — same Dockerfile works.
2. Plan a new ECS Fargate deployment with `aws_plan_infra`.
3. Deploy to ECS.
4. Update DNS to point to the new ALB.
5. Tear down the Lightsail service.

This should be a natural conversation: "Your app is getting more traffic than Lightsail handles well. Want me to upgrade to ECS Fargate? It'll cost ~$65/mo but gives you auto-scaling."

## Changes Required

| File | Change |
|------|--------|
| `internal/state/types.go` | Add `Backend` field to `Plan`, add Lightsail resource constants |
| `internal/providers/aws.go` | Add `selectBackend()`, `createLightsailInfra()`, `deployToLightsail()`, `teardownLightsail()` |
| `internal/providers/aws.go` | Update `planInfra()` to call `selectBackend()` and adjust cost estimation |
| `internal/providers/aws.go` | Update `createInfra()` to branch on `plan.Backend` |
| `internal/providers/aws.go` | Update `deploy()` to branch on backend type |
| `internal/providers/aws.go` | Update `teardown()` to branch on backend type |
| `internal/providers/aws.go` | Update `status()` to handle Lightsail services |
| `internal/providers/aws_test.go` | Tests for all new functions |
| `internal/awsclient/interfaces.go` | Add Lightsail client interface |
| `internal/awsclient/mocks/lightsail.go` | Mock Lightsail client |
| `internal/state/reconcile.go` | Add `findOrphanedLightsailServices()` |
| `internal/spending/costs.go` | Add Lightsail cost tracking |
| `go.mod` | Add `github.com/aws/aws-sdk-go-v2/service/lightsail` dependency |

## Dependencies

```
github.com/aws/aws-sdk-go-v2/service/lightsail
```
