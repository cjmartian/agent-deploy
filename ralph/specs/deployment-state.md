# Deployment State Specification

## Overview

The system must track deployment state to:
- Resume operations after restart
- Return deployment lists via `aws:deployments` resource
- Enable teardown of specific deployments
- Track costs per deployment

## State Model

### Plan

```go
type Plan struct {
    ID              string    `json:"id"`
    AppDescription  string    `json:"app_description"`
    ExpectedUsers   int       `json:"expected_users"`
    LatencyMS       int       `json:"latency_ms"`
    Region          string    `json:"region"`
    Services        []string  `json:"services"`
    EstimatedCostMo float64   `json:"estimated_cost_monthly"`
    Status          string    `json:"status"` // created, approved, expired
    CreatedAt       time.Time `json:"created_at"`
    ExpiresAt       time.Time `json:"expires_at"`
}
```

### Infrastructure

```go
type Infrastructure struct {
    ID        string            `json:"id"`
    PlanID    string            `json:"plan_id"`
    Region    string            `json:"region"`
    Resources map[string]string `json:"resources"` // type -> ARN
    Status    string            `json:"status"`    // provisioning, ready, failed, destroyed
    CreatedAt time.Time         `json:"created_at"`
}
```

Resources tracked:
- `vpc`: VPC ID
- `subnet_public`: Subnet IDs
- `subnet_private`: Subnet IDs
- `security_group`: Security Group IDs
- `ecs_cluster`: Cluster ARN
- `alb`: ALB ARN
- `target_group`: Target Group ARN

### Deployment

```go
type Deployment struct {
    ID           string    `json:"id"`
    InfraID      string    `json:"infra_id"`
    ImageRef     string    `json:"image_ref"`
    Status       string    `json:"status"` // deploying, running, failed, stopped
    URLs         []string  `json:"urls"`
    TaskDefARN   string    `json:"task_def_arn"`
    ServiceARN   string    `json:"service_arn"`
    CreatedAt    time.Time `json:"created_at"`
    LastUpdated  time.Time `json:"last_updated"`
}
```

## Storage Options

### Option A: Local File Storage

Store state in `~/.agent-deploy/state/`:
```
~/.agent-deploy/
├── state/
│   ├── plans/
│   │   └── plan-xxx.json
│   ├── infra/
│   │   └── infra-xxx.json
│   └── deployments/
│       └── deploy-xxx.json
└── config.json
```

**Pros:** Simple, no dependencies
**Cons:** Single-machine, not shared

### Option B: AWS-Native Storage

Store state in AWS:
- DynamoDB table for structured state
- S3 bucket for larger artifacts
- Use resource tags as source of truth

**Pros:** Distributed, survives local machine loss
**Cons:** Requires AWS resources, chicken-and-egg for bootstrapping

### Recommended Approach

Use **hybrid approach**:
1. Local file storage as primary (simple, always available)
2. AWS resource tags as secondary source of truth
3. Reconciliation on startup (sync local state with AWS tags)

## State Operations

### Create Plan
```go
func (s *Store) CreatePlan(plan *Plan) error
func (s *Store) GetPlan(id string) (*Plan, error)
func (s *Store) ApprovePlan(id string) error
```

### Create Infrastructure
```go
func (s *Store) CreateInfra(infra *Infrastructure) error
func (s *Store) GetInfra(id string) (*Infrastructure, error)
func (s *Store) UpdateInfraResource(id, resourceType, arn string) error
func (s *Store) SetInfraStatus(id, status string) error
```

### Create Deployment
```go
func (s *Store) CreateDeployment(deploy *Deployment) error
func (s *Store) GetDeployment(id string) (*Deployment, error)
func (s *Store) UpdateDeploymentStatus(id, status string, urls []string) error
func (s *Store) ListDeployments() ([]*Deployment, error)
```

## ID Generation

Use ULIDs for all identifiers:
- Lexicographically sortable
- Timestamp-encoded
- Collision-resistant

Format: `{type}-{ulid}`
- Plans: `plan-01HX...`
- Infrastructure: `infra-01HX...`
- Deployments: `deploy-01HX...`

## Cleanup

### Expired Plans

Plans expire after 24 hours if not approved:
- Background goroutine checks hourly
- Deletes expired plan files

### Orphaned Resources

On startup, reconcile local state with AWS:
1. List all resources tagged with `agent-deploy:*`
2. Compare against local state
3. Alert on orphaned resources (in AWS but not local)
4. Clean up stale local entries (in local but not AWS)
