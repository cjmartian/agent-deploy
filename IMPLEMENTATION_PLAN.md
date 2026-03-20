# Implementation Plan

**Project Goal:** Natural language deployment of applications via MCP server → Cloud provider. Allow users to end-to-end create applications and make them publicly available while ensuring spend does not cross user-defined boundaries.

**Last Updated:** 2026-03-20 (verified against codebase)

---

## ✅ Completed

| Component | Status | Location |
|-----------|--------|----------|
| MCP server (stdio + HTTP) | ✅ Working | `internal/main.go:1-46` |
| Provider interface | ✅ Defined | `internal/providers/provider.go:1-19` |
| AWS `aws_deploy_plan` prompt | ✅ Complete | `internal/providers/aws.go:181-213` |
| Tool input/output types | ✅ Defined | `internal/providers/aws.go:49-99` |
| Tool registrations (stubbed handlers) | ✅ Wired | `internal/providers/aws.go:20-45` |
| Resource registration (stubbed handler) | ✅ Wired | `internal/providers/aws.go:153-159` |
| Specifications | ✅ Written | `ralph/specs/aws-provider.md (202 lines), deployment-state.md (156 lines), spending-safeguards.md (123 lines)` |

---

## Current State Summary

| Component | Status | Location |
|-----------|--------|----------|
| AWS 5 tools (plan/create/deploy/status/teardown) | ⚠️ **Stubbed** | `internal/providers/aws.go:103-147` |
| AWS `aws:deployments` resource | ⚠️ **Stubbed** | `internal/providers/aws.go:161-175` |
| AWS SDK dependency | ❌ **Missing** | `go.mod` — no `aws-sdk-go-v2` |
| ULID dependency | ❌ **Missing** | `go.mod` — no `oklog/ulid` |
| State storage package | ❌ **Missing** | No `internal/state/` directory |
| Spending safeguards package | ❌ **Missing** | No `internal/spending/` directory |
| ID generation package | ❌ **Missing** | No `internal/id/` directory |
| AWS client config package | ❌ **Missing** | No `internal/awsclient/` directory |
| Tests | ❌ **Missing** | No `*_test.go` files exist |
| Makefile | ⚠️ **Broken** | Mixed tabs/spaces, wrong paths, invalid flags |

---

## P0 — Blockers (Must Fix First)

These must be resolved before any real functionality can be implemented.

### P0.1 Fix Makefile syntax ⚡ QUICK WIN

- [ ] **File:** `Makefile`
- **Issues (all lines must use tabs, not spaces):**
  - Line 4: `go build -o agent-deploy .` — should be `./internal` (main.go is in internal/)
  - Line 7: `gofmt -w -s .` — uses spaces instead of tab
  - Line 11: `golangci-lint run ./..` — uses spaces instead of tab, missing trailing dot (should be `./...`)
  - Line 14: `go clean` — uses spaces instead of tab
  - Line 17: `go mod download` — uses spaces instead of tab
  - Line 21: `go test -b ./..` — uses spaces instead of tab, `-b` is invalid flag (should be `-v`), and `./..` should be `./...`
- **Fix:** Convert all recipe lines to tabs, fix build path, fix lint and test commands
- **Depends on:** nothing
- **Effort:** ~5 minutes

### P0.2 Add AWS SDK dependency

- [ ] Add `github.com/aws/aws-sdk-go-v2` and required service modules to `go.mod`
- **File:** `go.mod` (append after line 5)
- **Modules needed:**
  ```
  github.com/aws/aws-sdk-go-v2
  github.com/aws/aws-sdk-go-v2/config
  github.com/aws/aws-sdk-go-v2/credentials
  github.com/aws/aws-sdk-go-v2/service/ec2
  github.com/aws/aws-sdk-go-v2/service/ecs
  github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2
  github.com/aws/aws-sdk-go-v2/service/ecr
  github.com/aws/aws-sdk-go-v2/service/cloudwatch
  github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs
  github.com/aws/aws-sdk-go-v2/service/pricing
  github.com/aws/aws-sdk-go-v2/service/costexplorer
  ```
- **Command:** `go get github.com/aws/aws-sdk-go-v2/...` for each module
- **Depends on:** nothing

### P0.3 Add ULID dependency

- [ ] Add `github.com/oklog/ulid/v2` for ID generation per `ralph/specs/deployment-state.md:130-140`
- **File:** `go.mod` (append after line 5)
- **Command:** `go get github.com/oklog/ulid/v2`
- **Depends on:** nothing

---

## P1 — Core Infrastructure (Foundation)

Implementation order matters: state storage and ID generation are required by every tool handler.

### P1.1 ID generation utility

- [ ] Create `internal/id/id.go` with functions to generate prefixed ULIDs
- **Spec:** `ralph/specs/deployment-state.md:131-142`
- **Format:** `plan-{ulid}`, `infra-{ulid}`, `deploy-{ulid}`
- **Functions to implement:**
  ```go
  func New(prefix string) string  // returns e.g. "plan-01HX..."
  ```
- **Depends on:** P0.3

### P1.2 State model types

- [ ] Create `internal/state/types.go` defining `Plan`, `Infrastructure`, `Deployment` structs
- **Spec:** `ralph/specs/deployment-state.md:14-67`
- **Structs (copy exactly from spec):**
  - `Plan` — lines 14-27
  - `Infrastructure` — lines 31-42
  - `Deployment` — lines 54-67
- **Depends on:** nothing (can be done in parallel with P1.1)

### P1.3 State storage (file-backed Store)

- [ ] Create `internal/state/store.go` implementing the `Store` with local file storage
- **Spec:** `ralph/specs/deployment-state.md:70-99` (Option A — local file storage at `~/.agent-deploy/state/`)
- **Directory structure:**
  ```
  ~/.agent-deploy/
  ├── state/
  │   ├── plans/plan-xxx.json
  │   ├── infra/infra-xxx.json
  │   └── deployments/deploy-xxx.json
  └── config.json
  ```
- **Operations to implement (per spec lines 105-129):**
  - `CreatePlan(plan *Plan) error`
  - `GetPlan(id string) (*Plan, error)`
  - `ApprovePlan(id string) error`
  - `CreateInfra(infra *Infrastructure) error`
  - `GetInfra(id string) (*Infrastructure, error)`
  - `UpdateInfraResource(id, resourceType, arn string) error`
  - `SetInfraStatus(id, status string) error`
  - `CreateDeployment(deploy *Deployment) error`
  - `GetDeployment(id string) (*Deployment, error)`
  - `UpdateDeploymentStatus(id, status string, urls []string) error`
  - `ListDeployments() ([]*Deployment, error)`
- **Depends on:** P1.1, P1.2

### P1.4 AWS client configuration

- [ ] Create `internal/awsclient/client.go` with shared AWS config loader
- **Spec:** `ralph/specs/aws-provider.md:185-202` (Authentication section)
- **Functions to implement:**
  ```go
  func LoadConfig(ctx context.Context, region string) (aws.Config, error)
  ```
- **Support:** env vars (`AWS_ACCESS_KEY_ID`, etc.), `~/.aws/credentials`, IAM roles
- **Depends on:** P0.2

### P1.5 Wire Store into AWSProvider

- [ ] Add a `store *state.Store` field to `AWSProvider` struct
- **File:** `internal/providers/aws.go` line 12
- [ ] Modify `All()` in `internal/providers/provider.go` to initialize store
- **File:** `internal/providers/provider.go` lines 14-18
- [ ] Update tool handlers to receive store (either as methods or via closure)
- **Depends on:** P1.3, P1.4

---

## P2 — Tool Implementation (AWS Integration)

These implement the actual AWS functionality. Each replaces a stub.

### P2.1 Implement `planInfra`

- [ ] Replace stub at `internal/providers/aws.go:103-114`
- **Spec:** `ralph/specs/aws-provider.md:29-35` (Behavior section)
- **Tasks:**
  1. Accept `planInfraInput` and analyze requirements
  2. Call AWS Pricing API (`pricing.GetProducts`) for cost estimation of selected services
  3. Generate plan ID via `id.New("plan")`
  4. Persist plan via `store.CreatePlan()`
  5. Return `planInfraOutput` with real cost estimate and service list
- **Depends on:** P0.2, P1.3, P1.4, P1.5

### P2.2 Spending limits configuration

- [ ] Create `internal/spending/config.go`
- **Spec:** `ralph/specs/spending-safeguards.md:13-28`
- **Tasks:**
  - Define `SpendingLimits` struct: `MonthlyBudgetUSD`, `PerDeploymentUSD`, `AlertThresholdPercent`
  - Load from `~/.agent-deploy/config.json`
  - Support env var overrides: `AGENT_DEPLOY_MONTHLY_BUDGET`, `AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET`
  - Define `SpendingCheckResult` struct per spec lines 52-58
- **Depends on:** nothing

### P2.3 Pre-provisioning budget check

- [ ] Create `internal/spending/check.go`
- **Spec:** `ralph/specs/spending-safeguards.md:41-57`
- **Tasks:**
  - Implement `CheckBudget(estimatedCost float64, limits SpendingLimits) SpendingCheckResult`
  - Compare estimated cost against `per_deployment_usd`
  - Compare against remaining `monthly_budget_usd` (sum existing deployment costs)
  - Block provisioning if limits exceeded, return clear error per spec lines 72-83
- **Depends on:** P2.2, P1.3

### P2.4 Implement `createInfra`

- [ ] Replace stub at `internal/providers/aws.go:116-122`
- **Spec:** `ralph/specs/aws-provider.md:53-62` (Behavior section)
- **Tasks:**
  1. Validate plan exists and is approved via `store.GetPlan()`
  2. **Check spending limits (P2.3) before proceeding**
  3. Provision AWS resources: VPC → subnets → security groups → ECS cluster → ALB → CloudWatch
  4. Tag all resources per `ralph/specs/spending-safeguards.md:106-114`
  5. Track each resource ARN via `store.UpdateInfraResource()`
  6. Generate infra ID via `id.New("infra")`
  7. Persist infrastructure record via `store.CreateInfra()`
- **Resource tagging (required for cost tracking):**
  ```json
  {
    "agent-deploy:deployment-id": "deploy-xxx",
    "agent-deploy:infra-id": "infra-xxx",
    "agent-deploy:plan-id": "plan-xxx",
    "agent-deploy:created-by": "agent-deploy"
  }
  ```
- **Depends on:** P2.1, P2.3

### P2.5 Implement `deploy`

- [ ] Replace stub at `internal/providers/aws.go:124-130`
- **Spec:** `ralph/specs/aws-provider.md:82-88` (Behavior section)
- **Tasks:**
  1. Validate infra exists and is ready via `store.GetInfra()`
  2. Create/update ECR repository
  3. Push or reference container image from `image_ref`
  4. Create ECS task definition
  5. Create or update ECS service
  6. Wait for healthy targets on ALB
  7. Generate deployment ID via `id.New("deploy")`
  8. Persist deployment via `store.CreateDeployment()`
- **Depends on:** P2.4

### P2.6 Implement `status`

- [ ] Replace stub at `internal/providers/aws.go:132-139`
- **Spec:** `ralph/specs/aws-provider.md:108-113` (Behavior section)
- **Tasks:**
  1. Look up deployment via `store.GetDeployment()`
  2. Look up infrastructure via `store.GetInfra()` to get resource ARNs
  3. Query ECS `DescribeServices` for running count and status
  4. Query ALB `DescribeTargetHealth` for health check results
  5. Retrieve ALB DNS name from `DescribeLoadBalancers`
  6. Return consolidated `statusOutput` with real URLs
- **Depends on:** P2.5

### P2.7 Implement `teardown`

- [ ] Replace stub at `internal/providers/aws.go:141-147`
- **Spec:** `ralph/specs/aws-provider.md:131-139` (Behavior section)
- **Tasks:**
  1. Look up deployment and infrastructure via store
  2. Delete in reverse order: ECS service → ECS cluster → ALB/target group → security groups → subnets → VPC
  3. Optionally delete ECR repository
  4. Clean up CloudWatch log groups
  5. Update state: `store.UpdateDeploymentStatus(id, "stopped", nil)`
  6. Update infra: `store.SetInfraStatus(id, "destroyed")`
- **Depends on:** P2.6 (for testing; no code dependency)

### P2.8 Implement `deploymentsResource`

- [ ] Replace stub at `internal/providers/aws.go:161-175`
- **Spec:** `ralph/specs/aws-provider.md:144-164` (Resource response format)
- **Tasks:**
  1. Call `store.ListDeployments()`
  2. For each deployment, enrich with infra status and URLs
  3. Serialize to JSON matching the spec format (`deployment_id`, `infra_id`, `status`, `created_at`, `urls`)
  4. Return as `ResourceContents` with `application/json` MIME type
- **Depends on:** P1.3 (can be done early, before full AWS integration)

---

## P3 — Spending Safeguards (Advanced)

Per README: "Ensure spend does not cross some boundary set by the user."

### P3.1 AWS Cost Explorer integration

- [ ] Create `internal/spending/costs.go`
- **Spec:** `ralph/specs/spending-safeguards.md:96-104`
- **Tasks:**
  - Query Cost Explorer API filtered by `agent-deploy:*` resource tags
  - Track daily/monthly accruals per deployment
  - Project end-of-month spend
- **Depends on:** P0.2, P2.2

### P3.2 Runtime cost monitoring (stretch)

- [ ] Alert when actual spend reaches `alert_threshold_percent` of budget
- [ ] Optional auto-teardown when hard limit exceeded
- **Spec:** `ralph/specs/spending-safeguards.md:60-68, 115-123`
- **Depends on:** P3.1, P2.7

---

## P4 — Testing & Quality

### P4.1 Unit tests for ID generation

- [ ] Create `internal/id/id_test.go`
- **Tasks:**
  - Test prefix formatting (`plan-`, `infra-`, `deploy-`)
  - Test uniqueness across calls
  - Test lexicographic ordering (ULIDs should sort by time)
- **Depends on:** P1.1

### P4.2 Unit tests for state storage

- [ ] Create `internal/state/store_test.go`
- **Tasks:**
  - Test CRUD operations for Plans, Infrastructure, Deployments
  - Test file creation, reading, listing
  - Use `t.TempDir()` for isolation
- **Depends on:** P1.3

### P4.3 Unit tests for spending checks

- [ ] Create `internal/spending/check_test.go`
- **Tasks:**
  - Test budget allowed/blocked scenarios
  - Test missing config defaults
  - Test env var overrides
- **Depends on:** P2.2, P2.3

### P4.4 Unit tests for AWS provider tool handlers

- [ ] Create `internal/providers/aws_test.go`
- **Tasks:**
  - Mock AWS SDK clients (use interfaces + test doubles)
  - Test each handler: planInfra, createInfra, deploy, status, teardown
  - Test deploymentsResource serialization
  - Test error paths (invalid plan_id, missing infra, etc.)
- **Depends on:** P2.1–P2.8

### P4.5 Integration tests

- [ ] Create `internal/providers/aws_integration_test.go` (build-tagged)
- **Tasks:**
  - Test against LocalStack or AWS sandbox
  - Full workflow: plan → create → deploy → status → teardown
  - Verify resource cleanup
- **Build tag:** `//go:build integration`
- **Depends on:** P4.4

### P4.6 MCP server integration test

- [ ] Create `internal/main_test.go`
- **Tasks:**
  - Test server startup and tool registration via MCP client
  - Start server in-process
  - Call tools via MCP client SDK
  - Verify tool listing, resource reading, prompt retrieval
- **Depends on:** P1.5

---

## P5 — Stretch Goals

### P5.1 State reconciliation on startup

- [ ] Sync local state with AWS resource tags on server start
- **Spec:** `ralph/specs/deployment-state.md:152-156`
- **Tasks:**
  - List all resources tagged with `agent-deploy:*`
  - Compare against local state files
  - Alert on orphaned resources
  - Clean up stale local entries
- **Depends on:** P1.3, P0.2

### P5.2 Expired plan cleanup

- [ ] Background goroutine to delete plans older than 24 hours
- **Spec:** `ralph/specs/deployment-state.md:146-150`
- **Depends on:** P1.3

### P5.3 CloudFormation-based provisioning

- [ ] Use CloudFormation stacks instead of individual API calls for atomic create/teardown
- **Impact:** Simplifies `createInfra` (P2.4) and `teardown` (P2.7) significantly
- **Depends on:** P0.2

### P5.4 Additional cloud providers

- [ ] **GCP Provider** — new file `internal/providers/gcp.go`, register in `All()`
- [ ] **Azure Provider** — new file `internal/providers/azure.go`, register in `All()`
- **Depends on:** P1.3 (shared state model)

---

## Technical Debt

### TD.1 Error handling patterns

- [ ] Establish consistent error wrapping with `fmt.Errorf("operation: %w", err)`
- [ ] Define domain error types:
  ```go
  var (
      ErrPlanNotFound      = errors.New("plan not found")
      ErrPlanNotApproved   = errors.New("plan not approved")
      ErrInfraNotFound     = errors.New("infrastructure not found")
      ErrDeploymentNotFound = errors.New("deployment not found")
      ErrBudgetExceeded    = errors.New("spending budget exceeded")
      ErrProvisioningFailed = errors.New("provisioning failed")
  )
  ```
- **File:** new `internal/errors/errors.go`

### TD.2 Structured logging

- [ ] Replace bare `log.Fatal` calls in `internal/main.go` with structured logger
- [ ] Use `log/slog` (stdlib since Go 1.21)
- **Depends on:** nothing

---

## Dependency Graph

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           P0 — BLOCKERS                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  P0.1 (Makefile)     P0.2 (AWS SDK)        P0.3 (ULID)                │
│       │                   │                    │                        │
│       │                   │                    ▼                        │
│       │                   │               P1.1 (ID gen)                 │
│       │                   │                    │                        │
│       ▼                   │                    │        P1.2 (types)    │
│  Build/Test works         │                    │             │          │
│                           │                    │             │          │
│                           ▼                    ▼             ▼          │
│                    P1.4 (AWS client) ──────► P1.3 (Store) ◄─────┘      │
│                                                    │                    │
│                                                    ▼                    │
│                                            P1.5 (Wire into provider)   │
│                                                    │                    │
└────────────────────────────────────────────────────┼────────────────────┘
                                                     │
┌────────────────────────────────────────────────────┼────────────────────┐
│                        P2 — TOOL IMPLEMENTATION    │                    │
├────────────────────────────────────────────────────┼────────────────────┤
│                                                    │                    │
│  P2.2 (spending config)                           │                    │
│       │                                            ▼                    │
│       ▼                                    P2.1 (planInfra)            │
│  P2.3 (budget check) ◄──────────────────────────────┘                   │
│       │                                            │                    │
│       │                                            ▼                    │
│       └─────────────────────────────────► P2.4 (createInfra)           │
│                                                    │                    │
│                                                    ▼                    │
│                                            P2.5 (deploy)               │
│                                                    │                    │
│                                                    ▼                    │
│                                            P2.6 (status)               │
│                                                    │                    │
│                                                    ▼                    │
│                                            P2.7 (teardown)             │
│                                                                         │
│  P2.8 (deploymentsResource) ◄─── P1.3 (can be done early)             │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Critical Path (Minimum Viable Product)

```
P0.1 ──┐
P0.2 ──┼──► P1.1 + P1.2 ──► P1.3 ──► P1.4 + P1.5 ──► P2.1 ──► P2.4 ──► P2.5 ──► P2.6 ──► P2.7
P0.3 ──┘                                   │
                                           └──► P2.8 (parallel)
```

### Parallel Work Tracks

| Track | Items | Can Start After |
|-------|-------|-----------------|
| **A: Foundation** | P0.1, P0.2, P0.3, P1.1, P1.2 | Immediately |
| **B: Spending** | P2.2, P2.3 | P1.3 complete |
| **C: AWS Tools** | P2.1, P2.4, P2.5, P2.6, P2.7 | P1.5 + P2.3 complete |
| **D: Testing** | P4.1, P4.2 | Respective P1 items |

---

## Quick Reference

### Build & Run

```bash
# Build (after P0.1 Makefile fix)
make build
# OR manually:
go build -o agent-deploy ./internal

# Run (stdio mode)
./agent-deploy

# Run (HTTP mode)
./agent-deploy -http :8080

# Dependencies (after P0.2, P0.3)
go mod tidy
```

### Test Commands

```bash
# Unit tests (once tests exist)
go test ./...

# Verbose
go test -v ./...

# With race detector
go test -race ./...

# Integration tests (build-tagged)
go test -tags=integration ./...
```

### Key Existing Files

| File | Purpose | Lines |
|------|---------|-------|
| `internal/main.go` | MCP server entry point | 46 |
| `internal/providers/provider.go` | Provider interface + registration | 19 |
| `internal/providers/aws.go` | AWS provider (5 stubbed tools, 1 resource, 1 prompt) | 213 |
| `ralph/specs/aws-provider.md` | Tool/resource/prompt specifications | 202 |
| `ralph/specs/deployment-state.md` | State model and storage spec | 156 |
| `ralph/specs/spending-safeguards.md` | Budget enforcement spec | 123 |

### New Files to Create (in order)

| Priority | File | Purpose |
|----------|------|---------|
| P1.1 | `internal/id/id.go` | ULID-based ID generation |
| P1.2 | `internal/state/types.go` | Plan, Infrastructure, Deployment structs |
| P1.3 | `internal/state/store.go` | File-backed state storage |
| P1.4 | `internal/awsclient/client.go` | Shared AWS SDK configuration |
| P2.2 | `internal/spending/config.go` | Spending limits configuration |
| P2.3 | `internal/spending/check.go` | Pre-provisioning budget check |
| P3.1 | `internal/spending/costs.go` | AWS Cost Explorer integration |
| TD.1 | `internal/errors/errors.go` | Domain error types |

### Stub Locations (to replace)

| Tool | Location | Lines |
|------|----------|-------|
| `planInfra` | `internal/providers/aws.go` | 103-114 |
| `createInfra` | `internal/providers/aws.go` | 116-122 |
| `deploy` | `internal/providers/aws.go` | 124-130 |
| `status` | `internal/providers/aws.go` | 132-139 |
| `teardown` | `internal/providers/aws.go` | 141-147 |
| `deploymentsResource` | `internal/providers/aws.go` | 161-175 |
