# Implementation Plan

**Project Goal:** Natural language deployment of applications via MCP server → Cloud provider. Allow users to end-to-end create applications and make them publicly available while ensuring spend does not cross user-defined boundaries.

**Last Updated:** 2026-03-20  
**Last Audit:** 2026-03-20

---

## ✅ Completed

| Component | Status | Location | Audit Notes (2026-03-20) |
|-----------|--------|----------|-------------|
| MCP server (stdio + HTTP) | ✅ Working | `internal/main.go` | Verified |
| Provider interface | ✅ Defined | `internal/providers/provider.go` | Verified |
| AWS 5 tools | ✅ Implemented | `internal/providers/aws.go` | aws_plan_infra, aws_create_infra, aws_deploy, aws_status, aws_teardown |
| AWS `aws:deployments` resource | ✅ Implemented | `internal/providers/aws.go` | Verified |
| AWS `aws_deploy_plan` prompt | ✅ Implemented | `internal/providers/aws.go` | Verified |
| Tool input/output types | ✅ Defined | `internal/providers/aws.go` | Verified |
| Specifications | ✅ Written | `ralph/specs/` | All 4 specs present: aws-provider.md, deployment-state.md, spending-safeguards.md, ci.md |
| Makefile syntax | ✅ Fixed | `Makefile` | tabs, build path, test flags |
| AWS SDK dependency | ✅ Added | `go.mod` | ec2, ecs, ecr, elbv2, cloudwatchlogs, costexplorer |
| ULID dependency | ✅ Added | `go.mod` | github.com/oklog/ulid/v2 |
| ID generation | ✅ Done | `internal/id/id.go` | New(), NewPlan(), NewInfra(), NewDeploy() |
| **State model types** | ✅ **100% compliant** | `internal/state/types.go` | Plan, Infrastructure, Deployment + constants |
| **State storage** | ✅ **100% compliant** | `internal/state/store.go` | Full Store implementation, exceeds spec |
| **State persistence** | ✅ **100% compliant** | `internal/state/store.go` | All state types, transitions implemented |
| AWS client config | ✅ Done | `internal/awsclient/client.go` | LoadConfig(), ResourceTags() |
| Wire Store into AWSProvider | ✅ Done | `internal/providers/aws.go` | store field, NewAWSProvider constructor |
| **Spending limit configuration** | ✅ **100% compliant** | `internal/spending/config.go` | Limits, LoadLimits(), env var support |
| **Cost tracking with Cost Explorer** | ✅ **100% compliant** | `internal/spending/costs.go` | Full implementation |
| **Runtime cost monitoring** | ✅ **100% compliant** | `internal/spending/monitor.go` | Background checking, alerts |
| **Alert thresholds and notifications** | ✅ **100% compliant** | `internal/spending/monitor.go` | Verified |
| **Resource tagging** | ✅ **100% compliant** | `internal/awsclient/client.go` | ResourceTags() |
| Pre-provisioning budget check | ⚠️ 85% | `internal/spending/check.go` | CheckBudget(), CheckResult (uses hardcoded costs - see P1.1, P1.2) |
| createInfra | ✅ Done | `internal/providers/aws.go` | VPC, subnets, IGW, route tables, SGs, ECS, ALB, CloudWatch |
| deploy | ✅ Done | `internal/providers/aws.go` | ECR repo, task def, ECS service, ALB URLs |
| status | ✅ Done | `internal/providers/aws.go` | ECS service status, ALB URLs |
| teardown | ✅ Done | `internal/providers/aws.go` | Reverse order deletion, ECR cleanup |
| Error handling patterns | ✅ Done | `internal/errors/errors.go` | Domain errors (3 unused types: ErrPlanNotApproved, ErrProvisioningFailed, ErrInvalidState) |
| ID generation tests | ✅ Done | `internal/id/id_test.go` | Verified |
| State storage tests | ✅ Done | `internal/state/store_test.go` | 45.5% coverage |
| Spending check tests | ✅ Done | `internal/spending/check_test.go` | Verified |
| Cost Explorer tests | ✅ Done | `internal/spending/costs_test.go` | Comprehensive |
| Runtime cost monitoring tests | ✅ Done | `internal/spending/monitor_test.go` | Comprehensive |
| MCP server integration test | ✅ Done | `internal/main_test.go` | 11 tests (does not test main()) |
| Expired plan cleanup (24-hour expiration, hourly cleanup) | ✅ Done | `internal/state/cleanup.go` | Full cleanup service |
| Expired plan cleanup tests | ✅ Done | `internal/state/cleanup_test.go` | Comprehensive |
| State reconciliation | ⚠️ Partial | `internal/state/reconcile.go` | No AWS pagination (see P3.1) |
| Reconciliation tests | ✅ Done | `internal/state/reconcile_test.go` | Mock-based only |
| Structured logging infrastructure | ✅ Done | `internal/logging/logging.go` | Full slog infrastructure (AddTime field in Config defined but never used) |
| Structured logging tests | ✅ Done | `internal/logging/logging_test.go` | Comprehensive |
| AllWithStore provider init | ✅ Done | `internal/providers/provider.go` | Shared store instances |
| Background services integration | ✅ Done | `internal/main.go` | CleanupService, CostMonitor, signal handling |
| Integration tests | ✅ Done | `internal/providers/aws_integration_test.go` | Full workflow tests |
| CI/CD workflows | ✅ Done | `.github/workflows/ci.yml`, `.golangci.yml` | lint, test, build on push/PR |

---

## Current State Summary

| Component | Status | Notes |
|-----------|--------|-------|
| **State storage** | ✅ **100% compliant** | Exceeds spec with additional operations |
| **Spending safeguards** | ✅ **Working** | Config, Cost Explorer, monitoring, alerts, tagging |
| **Cleanup service** | ✅ **Working** | `internal/state/cleanup.go` — 24-hour plan expiration with hourly cleanup |
| **Cost monitoring** | ✅ **Working** | `internal/spending/monitor.go` |
| **State reconciliation** | ⚠️ **No pagination** | Will miss resources beyond first page (see P3.1) |
| AWS 5 tools | ⚠️ **13+ hardcoded values** | See P1 issues below |
| AWS `aws:deployments` resource | ✅ **Implemented** | `internal/providers/aws.go` |
| AWS `aws_deploy_plan` prompt | ✅ **Implemented** | `internal/providers/aws.go` |
| Cost estimation (planInfra) | ❌ **Hardcoded** | `baseCost=15.0, ecsCost=users*0.02, albCost=20.0` (aws.go:153-158) |
| Current spend calculation | ❌ **Hardcoded** | `$25/deployment` constant at aws.go:220; Cost Explorer NOT wired to planInfra |
| Auto-teardown | ❌ **NOT WIRED** | Callback logs only at `main.go:135-147` — FALSE SENSE OF SECURITY |
| CI/CD workflows | ✅ **Working** | `.github/workflows/ci.yml`, `.golangci.yml` |
| golangci-lint config | ✅ **Working** | `.golangci.yml` with version 2 format |
| IAM role provisioning | ❌ **MISSING** | `ExecutionRoleArn` is nil at `aws.go:808`; go.mod missing `iam` package |
| go.mod dependencies | ⚠️ **Incomplete** | Missing `github.com/aws/aws-sdk-go-v2/service/iam` and `github.com/aws/aws-sdk-go-v2/service/pricing` |
| Auto Scaling | ❌ **NOT IMPLEMENTED** | Service name added but not configured |
| Private subnets | ❌ **NOT CREATED** | Spec requires public/private subnet architecture |
| Plan approval | ❌ **BYPASSED** | Auto-approves plans, no user confirmation |
| Wait for healthy deployment | ❌ **MISSING** | Returns immediately, doesn't wait for healthy (spec requires this) |
| Test coverage | ⚠️ **Gaps** | `awsclient/`, `errors/`, `spending/config.go`, `providers/provider.go` have 0%; `aws.go` at 8.3% |
| Structured logging | ⚠️ **Partial** | 32 `log.Printf` instances remaining (30 in aws.go, 1 in provider.go, 1 in costs.go) |
| Makefile | ⚠️ **Incomplete** | Missing coverage, coverage-html, test-race, install, run, all, help targets |

---

## P0 — Critical Issues (Security/Cost Risks, Broken Functionality)

### P0.1 CI/CD Workflows ✅ COMPLETED

- [x] Created `.github/workflows/ci.yml` with lint, test, build jobs on push/PR
- [x] Added golangci-lint configuration (`.golangci.yml`) with version 2 format
- [x] All linter issues in the codebase have been fixed (was 63 issues, now 0)
- **Location:** `.github/workflows/ci.yml`, `.golangci.yml`
- **Completed:** 2026-03-20

### P0.2 Auto-Teardown Not Wired ❌ CRITICAL - FALSE SENSE OF SECURITY

- [ ] Wire `TeardownCallback` to actually call AWS provider's teardown tool
- [ ] Current code at `main.go:135-147` only LOGS but does NOT tear down:
  ```go
  // Note: The actual teardown would be performed via the AWS provider.
  // This requires access to the provider, which we'll add in a future iteration.
  // For now, we log the intent. Users can manually teardown using aws_teardown tool.
  log.Info("deployment marked for teardown - use aws_teardown tool to complete", ...)
  return nil
  ```
- [ ] Pass provider reference to CostMonitor or use callback injection pattern
- [ ] Add integration test for auto-teardown flow
- **Impact:** **CRITICAL** — Users enable auto-teardown expecting protection but deployments continue running indefinitely, accumulating costs and security exposure
- **Location:** `internal/main.go:135-147`
- **Audit (2026-03-20):** Verified callback only logs, no teardown action

### P0.3 IAM Task Execution Role Missing ❌ CRITICAL

- [ ] Add `github.com/aws/aws-sdk-go-v2/service/iam` to `go.mod`
- [ ] Create/provision ECS task execution role with required permissions (ECR pull, CloudWatch logs)
- [ ] Pass `ExecutionRoleArn` to `RegisterTaskDefinition` (currently nil at `aws.go:808`)
- [ ] Add IAM role to teardown cleanup
- **Impact:** ECS tasks may fail to pull images from ECR or write logs to CloudWatch; functionality broken
- **Location:** `internal/providers/aws.go:808`, `go.mod`
- **Audit (2026-03-20):** Verified `ExecutionRoleArn: nil` in code

### P0.4 Graceful Shutdown Issues ❌

- [ ] HTTP mode uses `Close()` instead of `Shutdown()` for graceful shutdown
- [ ] `os.Exit(0)` bypasses defers in shutdown path
- **Impact:** In-flight requests may be dropped; cleanup may not run
- **Location:** `internal/main.go`

---

## P1 — Spec Compliance Gaps

### P1.1 Cost Estimation Uses Hardcoded Values ❌

- [ ] Add `github.com/aws/aws-sdk-go-v2/service/pricing` to `go.mod`
- [ ] Integrate AWS Pricing API for accurate cost estimates
- [ ] Replace hardcoded formula at `aws.go:153-158`:
  - `baseCost := 15.0`
  - `ecsCost := expectedUsers * 0.02`
  - `albCost := 20.0`
- [ ] Cache pricing data to avoid repeated API calls
- **Impact:** Cost estimates are inaccurate; spec `ralph/specs/aws-provider.md` requires AWS Pricing API
- **Location:** `internal/providers/aws.go:153-158`, `go.mod`
- **Audit (2026-03-20):** Verified hardcoded values, missing go.mod dependency

### P1.2 Current Spend Calculation Hardcoded ❌

- [ ] Replace hardcoded `currentSpend += 25.0` at `aws.go:220` with actual Cost Explorer data
- [ ] Cost Explorer IS implemented in `internal/spending/costs.go` but NOT called from `planInfra`
- [ ] Wire up `CostTracker.GetDeploymentCosts()` or `GetTotalMonthlySpend()`
- **Impact:** Budget checks use fake numbers; could allow overspend or wrongly block deployments
- **Location:** `internal/providers/aws.go:220`
- **Depends on:** P0.3 (requires working deployments to validate)
- **Audit (2026-03-20):** Verified Cost Explorer implemented but not wired to planInfra

### P1.3 Container Port Hardcoded to 80 (5 locations) ❌

- [ ] Add `container_port` input parameter to deploy tool
- [ ] Update task definition to use configurable port (`aws.go:814`)
- [ ] Update ALB target group health checks (`aws.go:865`)
- [ ] 5 hardcoded port 80 references across `aws.go`
- **Impact:** Cannot deploy apps on non-80 ports (Node.js uses 3000, Go uses 8080, etc.)
- **Location:** `internal/providers/aws.go:814, 865` + 3 other locations

### P1.4 Health Check Path Not Configurable ❌

- [ ] Add `health_check_path` input parameter
- [ ] Update target group health check configuration (currently "/" at `aws.go:701`)
- **Impact:** Apps with custom health endpoints (`/health`, `/healthz`) fail ALB health checks
- **Location:** `internal/providers/aws.go:701`
- **Audit (2026-03-20):** Verified hardcoded "/" health check path

### P1.5 Single Replica Deployments Only ❌

- [ ] Add `desired_count` parameter to deploy tool
- [ ] Update ECS service `DesiredCount` (currently always 1 at `aws.go:853`)
- **Impact:** No high availability; single point of failure; cannot scale
- **Location:** `internal/providers/aws.go:853`
- **Audit (2026-03-20):** Verified DesiredCount always 1

### P1.6 No Environment Variables Support ❌

- [ ] Add `environment` map input to deploy tool
- [ ] Pass environment to container definition
- **Impact:** Cannot configure apps via environment variables (common pattern)
- **Location:** `internal/providers/aws.go` (container definition)

### P1.7 No HTTPS/TLS Support ❌

- [ ] Add optional certificate ARN parameter
- [ ] Configure ALB HTTPS listener when certificate provided
- [ ] Default to HTTP for simplicity
- **Impact:** Production deployments require HTTPS; currently HTTP only
- **Location:** `internal/providers/aws.go` (ALB listener creation)

### P1.8 AWS Provider Not Using Structured Logging (32 instances) ❌

- [ ] Migrate 30 instances of `log.Printf()` in `aws.go` to `slog`-based structured logging
- [ ] Migrate 1 instance in `provider.go`
- [ ] Migrate 1 instance in `costs.go`
- [ ] Use existing `internal/logging/logging.go` infrastructure
- **Impact:** Inconsistent logging; structured logging infrastructure built but not adopted
- **Location:** `internal/providers/aws.go` (30), `internal/providers/provider.go` (1), `internal/spending/costs.go` (1)
- **Audit (2026-03-20):** Total of 32 `log.Printf` instances identified

### P1.9 VPC CIDR Hardcoded ❌

- [ ] Make VPC CIDR configurable (currently hardcoded to `10.0.0.0/16` at `aws.go:478`)
- **Impact:** May conflict with corporate networks or VPC peering
- **Location:** `internal/providers/aws.go:478`
- **Audit (2026-03-20):** Verified hardcoded CIDR

### P1.10 Subnet CIDRs Hardcoded ❌

- [ ] Make subnet CIDRs configurable (currently `10.0.1.0/24`, `10.0.2.0/24` at `aws.go:536`)
- **Impact:** Cannot customize network topology
- **Location:** `internal/providers/aws.go:536`
- **Audit (2026-03-20):** Verified hardcoded subnet CIDRs

### P1.11 Private Subnets Not Created ❌

- [ ] Spec requires public/private subnet architecture
- [ ] Currently only public subnets created
- [ ] Add NAT Gateway for private subnet egress
- **Impact:** All resources publicly accessible; no private tier for databases/internal services
- **Location:** `internal/providers/aws.go`

### P1.12 Auto Scaling Not Implemented ❌

- [ ] Auto Scaling service name added but not configured
- [ ] Add scaling policies based on CPU/memory thresholds
- **Impact:** Cannot automatically scale based on load
- **Location:** `internal/providers/aws.go`

### P1.13 Plan Approval Bypassed ❌

- [ ] Plans auto-approve without user confirmation
- [ ] Implement confirmation step before provisioning (spec `ralph/specs/aws-provider.md` requires approval)
- **Impact:** Users cannot review cost estimates before resources are created
- **Location:** `internal/providers/aws.go`
- **Audit (2026-03-20):** Verified auto-approval without user confirmation

### P1.14 No Wait for Healthy Deployment ❌

- [ ] Currently returns immediately after creating service
- [ ] Implement wait for ECS service to reach RUNNING state
- [ ] Check ALB health check passes
- [ ] Spec `ralph/specs/aws-provider.md` requires waiting for healthy deployment
- **Impact:** Users think deployment succeeded when it may still be starting/failing
- **Location:** `internal/providers/aws.go`
- **Audit (2026-03-20):** Verified returns immediately without waiting

### P1.15 Default Docker Image ❌

- [ ] Default image is `nginx:latest` at `aws.go:787`
- [ ] Should require explicit image specification or document default clearly
- **Impact:** Accidental deployments with wrong image
- **Location:** `internal/providers/aws.go:787`
- **Audit (2026-03-20):** Verified nginx:latest default

### P1.16 Log Retention Hardcoded ❌

- [ ] CloudWatch log retention hardcoded to 7 days at `aws.go:749`
- [ ] Make configurable per deployment
- **Impact:** Cannot retain logs longer for compliance/debugging
- **Location:** `internal/providers/aws.go:749`
- **Audit (2026-03-20):** Verified 7-day hardcoded retention

### P1.17 ECS Task Resources Hardcoded ❌

- [ ] CPU hardcoded to "256" at `aws.go:806`
- [ ] Memory hardcoded to "512" at `aws.go:807`
- [ ] Make configurable via deploy tool parameters
- **Impact:** Apps may run out of resources; no way to allocate more
- **Location:** `internal/providers/aws.go:806-807`
- **Audit (2026-03-20):** Verified hardcoded CPU=256, Memory=512

### P1.18 Error Wrapping Breaks errors.Is() ❌

- [ ] Inconsistent error wrapping at `aws.go:226`
- [ ] Should use `fmt.Errorf("...: %w", err)` for proper wrapping
- **Impact:** Error type checking with `errors.Is()` fails
- **Location:** `internal/providers/aws.go:226`

---

## P2 — Test Coverage Gaps

### P2.1 AWS Client Package Has No Tests (0% coverage) ❌

- [ ] Create `internal/awsclient/client_test.go`
- [ ] Test `LoadConfig` with mocked AWS SDK
- [ ] Test `ResourceTags` generation
- **Impact:** Core AWS configuration untested
- **Location:** `internal/awsclient/`
- **Audit (2026-03-20):** Verified 0% coverage — NO TESTS EXIST

### P2.2 Errors Package Has No Tests (0% coverage) ❌

- [ ] Create `internal/errors/errors_test.go`
- [ ] Test error type identification and wrapping
- [ ] Note: 3 error types are unused (ErrPlanNotApproved, ErrProvisioningFailed, ErrInvalidState)
- **Impact:** Domain error behavior untested
- **Location:** `internal/errors/`
- **Audit (2026-03-20):** Verified 0% coverage — only definitions exist, no tests

### P2.3 Spending Config Has No Tests (0% coverage) ❌

- [ ] Create tests for `internal/spending/config.go`
- [ ] Test `LoadLimits()` with various env var configurations
- **Impact:** Spending configuration untested
- **Location:** `internal/spending/config.go`
- **Audit (2026-03-20):** Verified 0% coverage

### P2.4 Provider.go Has No Tests (0% coverage) ❌

- [ ] Create tests for `internal/providers/provider.go`
- [ ] Test `All()` and `AllWithStore()` registration
- **Impact:** Provider registration untested
- **Location:** `internal/providers/provider.go`
- **Audit (2026-03-20):** Verified 0% coverage

### P2.5 AWS Provider Tool Tests Missing (8.3% coverage) ❌

- [ ] Add unit tests for `createInfra` with mocked AWS SDK
- [ ] Add unit tests for `deploy` with mocked AWS SDK
- [ ] Add unit tests for `status` with mocked AWS SDK
- [ ] Add unit tests for `teardown` with mocked AWS SDK
- [ ] Test error scenarios (VPC creation fails, ECS fails, etc.)
- **Impact:** Only `planInfra` has unit tests; other 4 tools untested
- **Location:** `internal/providers/aws_test.go`
- **Depends on:** P2.6 (AWS SDK mocking setup)
- **Audit (2026-03-20):** Verified only planInfra tested, 8.3% coverage

### P2.6 AWS SDK Mocking Infrastructure ❌

- [ ] Create mock interfaces for EC2, ECS, ECR, ELB, CloudWatch clients
- [ ] Set up test fixtures for common AWS responses
- [ ] Enable unit testing without LocalStack
- **Impact:** Required for P2.5; enables fast, reliable unit tests
- **Location:** `internal/awsclient/mocks/` (new)

### P2.7 Reconciliation AWS Integration Tests ❌

- [ ] Add integration tests for reconciliation with LocalStack/AWS
- [ ] Test orphaned resource detection
- [ ] Test stale entry cleanup with AWS pagination
- **Impact:** Reconciliation has 0% AWS integration coverage
- **Location:** `internal/state/reconcile_integration_test.go` (new)
- **Audit (2026-03-20):** Only mock-based tests exist

### P2.8 State Store Silent Failure Handling ❌

- [ ] Add logging for malformed state files in List operations
- [ ] Consider failing fast vs continuing on error (make configurable)
- [ ] Add tests for malformed file handling
- [ ] Silent skips at `store.go:111,248,335`
- **Impact:** Malformed state files silently ignored; debugging difficult
- **Location:** `internal/state/store.go:111,248,335`

### P2.9 Main.go Test Coverage (0%) ❌

- [ ] Test `main()` function startup
- [ ] Test flag parsing
- [ ] Test signal handling
- **Impact:** Entry point untested; existing test file doesn't test main()
- **Location:** `internal/main.go`, `internal/main_test.go`
- **Audit (2026-03-20):** Verified test file exists but doesn't test main()

### P2.10 State Package Coverage (45.5%) ⚠️

- [ ] Increase coverage of edge cases
- [ ] Test concurrent access patterns
- **Impact:** State management not fully tested
- **Location:** `internal/state/`
- **Audit (2026-03-20):** Verified 45.5% coverage

### P2.11 Spending Package Coverage (21.5%) ⚠️

- [ ] Increase coverage of spending module
- [ ] Test alert processing edge cases
- **Impact:** Spending safeguards not fully tested
- **Location:** `internal/spending/`
- **Audit (2026-03-20):** Verified 21.5% coverage

---

## P3 — Quality Improvements

### P3.1 AWS Reconciliation Lacks Pagination ❌

- [ ] Add pagination support for `DescribeVpcs`
- [ ] Add pagination support for `ListClusters`
- [ ] Add pagination support for `DescribeLoadBalancers`
- [ ] Handle large deployments (>100 resources per API call)
- **Impact:** Reconciliation may miss resources in large AWS accounts
- **Location:** `internal/state/reconcile.go`
- **Audit (2026-03-20):** Verified no pagination implemented

### P3.2 Inefficient ALB Tag Fetching ❌

- [ ] Currently makes individual tag API calls per ALB
- [ ] Batch tag fetching for multiple resources
- **Impact:** Performance issues with many ALBs
- **Location:** `internal/state/reconcile.go`

### P3.3 Version String Duplicated ❌

- [ ] Consolidate version "v0.1.0" (appears at `main.go:41` and `main.go:165`)
- [ ] Use single constant for version
- **Impact:** Version drift possible if updated in one place only
- **Location:** `internal/main.go:41,165`

### P3.4 Cost Monitor Region Hardcoded ❌

- [ ] Cost monitor always uses `us-east-1` (at `main.go:113`)
- [ ] Reconciliation region is configurable via `-reconcile-region` flag
- [ ] Make cost monitor region consistent
- **Impact:** Cost data may not reflect actual deployment regions
- **Location:** `internal/main.go:113`

### P3.5 Unused Error Types ❌

- [ ] Review 3 unused error types: `ErrPlanNotApproved`, `ErrProvisioningFailed`, `ErrInvalidState`
- [ ] Either use them appropriately in P1.13/P1.14 or remove dead code
- **Impact:** Code clutter; misleading error handling patterns
- **Location:** `internal/errors/errors.go`
- **Audit (2026-03-20):** Verified 3 unused types

### P3.6 planInfra Cost Estimate Disclaimer ❌

- [ ] Update planInfra output to clearly indicate estimate is approximate
- [ ] Add disclaimer when using hardcoded values (until P1.1 done)
- **Impact:** Users may rely on inaccurate estimates
- **Location:** `internal/providers/aws.go:152`

### P3.7 Makefile Missing Targets ❌

- [ ] Add `coverage` target (with `-coverprofile`)
- [ ] Add `coverage-html` target
- [ ] Add `test-race` target
- [ ] Add `install` target
- [ ] Add `run` target
- [ ] Add `all` target
- [ ] Add `help` target
- **Impact:** Developer experience; missing common workflows
- **Location:** `Makefile`

### P3.8 Logging Config AddTime Field Unused ❌

- [ ] `AddTime` field in `internal/logging/Config` is defined but never used
- [ ] Either implement time addition logic or remove the field
- **Impact:** Dead code; confusing API
- **Location:** `internal/logging/logging.go`
- **Audit (2026-03-20):** Discovered unused field

---

## P5 — Stretch Goals

### P5.1 CloudFormation-based provisioning

- [ ] Use CloudFormation stacks instead of individual API calls
- [ ] Enables atomic create/teardown
- **Impact:** Simplifies `createInfra` and `teardown` significantly
- **Location:** `internal/providers/aws.go`

### P5.2 Additional cloud providers

- [ ] **GCP Provider** — new file `internal/providers/gcp.go`, register in `All()`
- [ ] **Azure Provider** — new file `internal/providers/azure.go`, register in `All()`
- **Depends on:** Shared state model

### P5.3 Secrets Management

- [ ] Integrate AWS Secrets Manager or SSM Parameter Store
- [ ] Add `secrets` input to deploy tool
- [ ] Secure secret injection into containers
- **Impact:** No way to pass sensitive configuration securely
- **Location:** `internal/providers/aws.go`
- **Depends on:** P1.6 (environment variables)

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
./agent-deploy -enable-auto-teardown   # ⚠️ BROKEN (P0.2) - only logs, does NOT teardown!
./agent-deploy -enable-reconcile           # Enable state reconciliation on startup
./agent-deploy -reconcile-region us-west-2 # Specify AWS region for reconciliation (default: us-east-1)
```

### Test Commands

```bash
go test ./...                    # Unit tests
go test -v ./...                 # Verbose
go test -race ./...              # With race detector
go test -tags=integration ./...  # Integration tests (requires LocalStack or AWS)
go test -coverprofile=coverage.out ./...  # With coverage
go tool cover -html=coverage.out          # View coverage report
```

### Test Coverage Summary (from audit)

| Package | Coverage | Notes |
|---------|----------|-------|
| `internal/awsclient/` | **0%** | No tests |
| `internal/errors/` | **0%** | No tests (only definitions) |
| `internal/spending/config.go` | **0%** | No tests |
| `internal/providers/provider.go` | **0%** | No tests |
| `internal/providers/aws.go` | **8.3%** | Only planInfra tested |
| `internal/main.go` | **0%** | Test file doesn't test main() |
| `internal/spending/` | **21.5%** | Partial |
| `internal/state/` | **45.5%** | Partial |

### Key Files

| File | Purpose |
|------|---------|
| `internal/main.go` | MCP server entry point |
| `internal/providers/provider.go` | Provider interface + registration |
| `internal/providers/aws.go` | AWS provider (5 tools, 1 resource, 1 prompt) |
| `internal/state/store.go` | File-backed state storage |
| `internal/state/types.go` | Plan, Infrastructure, Deployment structs |
| `internal/state/reconcile.go` | State reconciliation with AWS resource tags |
| `internal/id/id.go` | ULID-based ID generation |
| `internal/awsclient/client.go` | Shared AWS SDK configuration |
| `internal/spending/config.go` | Spending limits configuration |
| `internal/spending/check.go` | Pre-provisioning budget check |
| `internal/spending/costs.go` | AWS Cost Explorer integration |
| `internal/spending/monitor.go` | Runtime cost monitoring with alerts |
| `internal/state/cleanup.go` | Expired plan cleanup service |
| `internal/errors/errors.go` | Domain error types |
| `internal/logging/logging.go` | Structured logging with slog |
| `internal/main_test.go` | MCP server integration tests |
| `ralph/specs/aws-provider.md` | Tool/resource/prompt specifications |
| `ralph/specs/deployment-state.md` | State model and storage spec |
| `ralph/specs/spending-safeguards.md` | Budget enforcement spec |
| `ralph/specs/ci.md` | CI/CD requirements spec |

### Hardcoded Values Summary (13+ issues in aws.go)

| Value | Location | Impact |
|-------|----------|--------|
| VPC CIDR: `10.0.0.0/16` | `aws.go:478` | Network conflicts |
| Subnet CIDRs: `10.0.1.0/24`, `10.0.2.0/24` | `aws.go:536` | Network conflicts |
| ECS Task CPU: `"256"` | `aws.go:806` | Resource limits |
| ECS Task Memory: `"512"` | `aws.go:807` | Resource limits |
| ECS Desired Count: `1` | `aws.go:853` | No HA |
| Container Port: `80` | `aws.go:814, 865` | Port flexibility |
| Health Check Path: `"/"` | `aws.go:701` | Health check compatibility |
| Log Retention: `7` days | `aws.go:749` | Compliance |
| Default Image: `nginx:latest` | `aws.go:787` | Accidental deployments |
| Cost baseCost: `15.0` | `aws.go:153` | Inaccurate estimates |
| Cost ecsCost: `users*0.02` | `aws.go:154` | Inaccurate estimates |
| Cost albCost: `20.0` | `aws.go:155` | Inaccurate estimates |
| Current spend: `$25/deployment` | `aws.go:216` | Budget bypass |

### Remaining Work by Priority

| Priority | Count | Items |
|----------|-------|-------|
| **P0 Critical** | 3 | Auto-teardown (P0.2), IAM role (P0.3), Graceful shutdown (P0.4) |
| **P1 Spec Gaps** | 18 | Cost estimation, logging, ports, env vars, HTTPS, VPC, subnets, approval, health wait, etc. |
| **P2 Test Gaps** | 11 | awsclient (0%), errors (0%), config (0%), provider.go (0%), aws.go (8.3%), mocking, coverage |
| **P3 Quality** | 8 | Pagination, ALB tags, version, region, errors, disclaimer, Makefile, unused AddTime |
| **P5 Stretch** | 3 | CloudFormation, multi-cloud, secrets |
| **Total** | **44** | |

---

## Spec Reference Summary (ralph/specs/)

| Spec | Requirement | Status |
|------|-------------|--------|
| **aws-provider.md** | 5 tools | ✅ Implemented |
| **aws-provider.md** | 1 resource (aws:deployments) | ✅ Implemented |
| **aws-provider.md** | 1 prompt (aws_deploy_plan) | ✅ Implemented |
| **aws-provider.md** | AWS Pricing API for cost estimation | ❌ NOT IMPLEMENTED (hardcoded) |
| **aws-provider.md** | Wait for healthy deployment in aws_deploy | ❌ NOT IMPLEMENTED |
| **aws-provider.md** | Plan approval before provisioning | ❌ NOT IMPLEMENTED (auto-approves) |
| **deployment-state.md** | Plan, Infrastructure, Deployment types | ✅ Implemented |
| **deployment-state.md** | File-backed JSON at ~/.agent-deploy/state/ | ✅ Implemented |
| **deployment-state.md** | 24-hour plan expiration, hourly cleanup | ✅ Implemented |
| **deployment-state.md** | AWS resource tag reconciliation | ⚠️ PARTIAL (no pagination) |
| **spending-safeguards.md** | monthly_budget_usd, per_deployment_usd, alert_threshold_percent | ✅ Implemented |
| **spending-safeguards.md** | Pre-provisioning budget check | ⚠️ PARTIAL (uses hardcoded costs) |
| **spending-safeguards.md** | Runtime cost monitoring with Cost Explorer | ✅ Implemented |
| **spending-safeguards.md** | Auto-teardown when budget exceeded | ❌ NOT WIRED (callback only logs) |
| **spending-safeguards.md** | Resource tagging | ✅ Implemented |
| **ci.md** | CI workflow with lint, test, build jobs | ❌ NOT IMPLEMENTED |
