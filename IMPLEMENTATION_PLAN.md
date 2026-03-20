# Implementation Plan

**Project Goal:** Natural language deployment of applications via MCP server → Cloud provider. Allow users to end-to-end create applications and make them publicly available while ensuring spend does not cross user-defined boundaries.

**Last Updated:** 2026-03-20

---

## ✅ Completed

| Component | Status | Location |
|-----------|--------|----------|
| MCP server (stdio + HTTP) | ✅ Working | `internal/main.go` |
| Provider interface | ✅ Defined | `internal/providers/provider.go` |
| AWS `aws_deploy_plan` prompt | ✅ Complete | `internal/providers/aws.go` |
| Tool input/output types | ✅ Defined | `internal/providers/aws.go` |
| Specifications | ✅ Written | `ralph/specs/` |
| Makefile syntax (P0.1) | ✅ Fixed | `Makefile` — tabs, build path, test flags |
| AWS SDK dependency (P0.2) | ✅ Added | `go.mod` — ec2, ecs, ecr, elbv2, cloudwatchlogs, pricing, costexplorer |
| ULID dependency (P0.3) | ✅ Added | `go.mod` — github.com/oklog/ulid/v2 |
| ID generation (P1.1) | ✅ Done | `internal/id/id.go` — New(), NewPlan(), NewInfra(), NewDeploy() |
| State model types (P1.2) | ✅ Done | `internal/state/types.go` — Plan, Infrastructure, Deployment + constants |
| State storage (P1.3) | ✅ Done | `internal/state/store.go` — full Store implementation |
| AWS client config (P1.4) | ✅ Done | `internal/awsclient/client.go` — LoadConfig(), ResourceTags() |
| Wire Store into AWSProvider (P1.5) | ✅ Done | `internal/providers/aws.go` — store field, NewAWSProvider constructor |
| planInfra (P2.1) | ✅ Done | `internal/providers/aws.go` — analyzes requirements, estimates costs, persists plan |
| Spending limits config (P2.2) | ✅ Done | `internal/spending/config.go` — Limits, LoadLimits(), env var support |
| Pre-provisioning budget check (P2.3) | ✅ Done | `internal/spending/check.go` — CheckBudget(), CheckResult |
| createInfra (P2.4) | ✅ Done | `internal/providers/aws.go` — VPC, subnets, IGW, route tables, SGs, ECS, ALB |
| deploy (P2.5) | ✅ Done | `internal/providers/aws.go` — ECR repo, task def, ECS service, ALB URLs |
| status (P2.6) | ✅ Done | `internal/providers/aws.go` — queries ECS service status, ALB URLs |
| teardown (P2.7) | ✅ Done | `internal/providers/aws.go` — deletes all resources in reverse order |
| deploymentsResource (P2.8) | ✅ Done | `internal/providers/aws.go` — JSON list of deployments from store |
| Error handling patterns (TD.1) | ✅ Done | `internal/errors/errors.go` — domain errors |
| ID generation tests (P4.1) | ✅ Done | `internal/id/id_test.go` |
| State storage tests (P4.2) | ✅ Done | `internal/state/store_test.go` |
| Spending check tests (P4.3) | ✅ Done | `internal/spending/check_test.go` |
| AWS provider tests (P4.4) | ✅ Partial | `internal/providers/aws_test.go` — planInfra, deploymentsResource, statusOutput |
| Cost Explorer integration (P3.1) | ✅ Done | `internal/spending/costs.go` — CostTracker, GetDeploymentCosts, GetTotalMonthlySpend, GetCostsByDeployment, CheckAlerts, GetDeploymentsOverBudget, GenerateMonitoringReport |
| Cost Explorer tests (P3.1) | ✅ Done | `internal/spending/costs_test.go` — comprehensive unit tests |
| Runtime cost monitoring (P3.2) | ✅ Done | `internal/spending/monitor.go` — CostMonitor, MonitorConfig, Start/Stop lifecycle, CheckNow, background cost checking, alert processing, auto-teardown |
| Runtime cost monitoring tests (P3.2) | ✅ Done | `internal/spending/monitor_test.go` — comprehensive unit tests |
| MCP server integration test (P4.6) | ✅ Done | `internal/main_test.go` — 11 tests: server creation, provider registration, tool/resource/prompt listing, server init, resource read, prompt retrieval, capabilities, ping |
| Expired plan cleanup (P5.2) | ✅ Done | `internal/state/cleanup.go` — DeletePlan, DeleteExpiredPlans, CleanupService (background goroutine), CleanupConfig, CleanupNow, CleanupStats, OnCleanup callback |
| Expired plan cleanup tests (P5.2) | ✅ Done | `internal/state/cleanup_test.go` — TestDeletePlan, TestDeletePlan_NotFound, TestDeleteExpiredPlans, TestDeleteExpiredPlans_NoExpired, TestCleanupService_StartStop, TestCleanupService_Stats, TestCleanupService_CleanupNow, TestCleanupService_OnCleanupCallback, TestCleanupService_DoubleStart/Stop |
| Structured logging (TD.2) | ✅ Done | `internal/logging/logging.go` — Initialize(), WithLevel/Format/Output/Source options, text/JSON formats, ParseLevel/ParseFormat, WithComponent, Debug/Info/Warn/Error, attribute helpers (DeploymentID, InfraID, PlanID, Region, Cost, Count, Err) |
| Structured logging tests (TD.2) | ✅ Done | `internal/logging/logging_test.go` — comprehensive unit tests |
| Structured logging migration | ✅ Done | `internal/state/cleanup.go`, `internal/spending/monitor.go` — migrated from log.Printf to slog |
| AllWithStore provider init | ✅ Done | `internal/providers/provider.go` — AllWithStore() for shared store instances |
| Background services integration | ✅ Done | `internal/main.go` — CleanupService and CostMonitor integration, graceful shutdown, signal handling |

---

## Current State Summary

| Component | Status | Location |
|-----------|--------|----------|
| AWS 5 tools (plan/create/deploy/status/teardown) | ✅ **Implemented** | `internal/providers/aws.go` |
| AWS `aws:deployments` resource | ✅ **Implemented** | `internal/providers/aws.go` |
| AWS SDK dependency | ✅ **Present** | `go.mod` |
| ULID dependency | ✅ **Present** | `go.mod` |
| State storage package | ✅ **Exists** | `internal/state/` |
| Spending safeguards package | ✅ **Exists** | `internal/spending/` |
| Cost Explorer integration | ✅ **Present** | `internal/spending/costs.go` |
| ID generation package | ✅ **Exists** | `internal/id/` |
| AWS client config package | ✅ **Exists** | `internal/awsclient/` |
| Domain errors package | ✅ **Exists** | `internal/errors/` |
| Structured logging package | ✅ **Exists** | `internal/logging/` |
| Unit tests | ✅ **Exist** | `internal/*/` |
| Cleanup service | ✅ **Integrated** | `internal/state/cleanup.go` |
| Cost monitoring | ✅ **Integrated** | `internal/spending/monitor.go` |
| Background services (cleanup + cost monitor) | ✅ **Integrated** | `internal/main.go` |
| Makefile | ✅ **Working** | `Makefile` |

---

## P4 — Testing & Quality (Remaining)

### P4.5 Integration tests

- [ ] Create `internal/providers/aws_integration_test.go` (build-tagged)
- **Tasks:**
  - Test against LocalStack or AWS sandbox
  - Full workflow: plan → create → deploy → status → teardown
  - Verify resource cleanup
- **Build tag:** `//go:build integration`

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

### P5.3 CloudFormation-based provisioning

- [ ] Use CloudFormation stacks instead of individual API calls for atomic create/teardown
- **Impact:** Simplifies `createInfra` (P2.4) and `teardown` (P2.7) significantly
- **Depends on:** P0.2

### P5.4 Additional cloud providers

- [ ] **GCP Provider** — new file `internal/providers/gcp.go`, register in `All()`
- [ ] **Azure Provider** — new file `internal/providers/azure.go`, register in `All()`
- **Depends on:** P1.3 (shared state model)

---

## Quick Reference

### Build & Run

```bash
make build           # Build the binary
./agent-deploy       # Run (stdio mode)
./agent-deploy -http :8080  # Run (HTTP mode)

# Logging options
./agent-deploy -log-level debug    # Set log level (debug/info/warn/error, default: info)
./agent-deploy -log-format json    # Set log format (text/json, default: text)
./agent-deploy -http :8080 -log-level debug -log-format json  # Combined

# Background services
./agent-deploy -enable-cost-monitor    # Enable runtime cost monitoring (requires AWS credentials)
./agent-deploy -enable-auto-teardown   # Enable automatic teardown of over-budget deployments
```

### Test Commands

```bash
go test ./...                    # Unit tests
go test -v ./...                 # Verbose
go test -race ./...              # With race detector
go test -tags=integration ./...  # Integration tests
```

### Key Files

| File | Purpose |
|------|---------|
| `internal/main.go` | MCP server entry point |
| `internal/providers/provider.go` | Provider interface + registration |
| `internal/providers/aws.go` | AWS provider (5 tools, 1 resource, 1 prompt) |
| `internal/state/store.go` | File-backed state storage |
| `internal/state/types.go` | Plan, Infrastructure, Deployment structs |
| `internal/id/id.go` | ULID-based ID generation |
| `internal/awsclient/client.go` | Shared AWS SDK configuration |
| `internal/spending/config.go` | Spending limits configuration |
| `internal/spending/check.go` | Pre-provisioning budget check |
| `internal/spending/costs.go` | AWS Cost Explorer integration |
| `internal/spending/monitor.go` | Runtime cost monitoring with alerts and auto-teardown |
| `internal/state/cleanup.go` | Expired plan cleanup service |
| `internal/errors/errors.go` | Domain error types |
| `internal/logging/logging.go` | Structured logging with slog |
| `internal/main_test.go` | MCP server integration tests (InMemoryTransport) |
| `ralph/specs/aws-provider.md` | Tool/resource/prompt specifications |
| `ralph/specs/deployment-state.md` | State model and storage spec |
| `ralph/specs/spending-safeguards.md` | Budget enforcement spec |

### Remaining Work

| Priority | Item | Purpose |
|----------|------|---------|
| P4.5 | Integration tests | LocalStack/AWS sandbox testing |
| P5.1, P5.3-5.4 | Stretch goals | Reconciliation, CloudFormation, multi-cloud |
