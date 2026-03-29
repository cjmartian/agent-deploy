# Implementation Plan

**Project Goal:** Natural language deployment of applications via MCP server → Cloud provider. Allow users to end-to-end create applications and make them publicly available while ensuring spend does not cross user-defined boundaries.

**Last Updated:** 2026-03-30  
**Last Audit:** 2025-07-21 (Comprehensive codebase audit — spec gap analysis, test gap analysis, quality issues verified)

---

## 🚨 Remaining Work Summary

### HIGH PRIORITY — Unimplemented Specs (P1)
| ID | Issue | Spec | Impact |
|----|-------|------|--------|
| **P1.29** | Custom DNS / Route 53 — **entire spec unimplemented** | `custom-dns.md` | No custom domain support, no Route 53, no ACM auto-provisioning |
| **P1.9** | VPC CIDR hardcoded to 10.0.0.0/16 | `networking.md` | Cannot customize VPC for peering scenarios |
| **P1.21** | Per-request spending override missing | `spending-safeguards.md` | Users cannot set deployment-specific budget caps |

### MEDIUM PRIORITY — Test Gaps (P2)
| ID | Issue | Impact |
|----|-------|--------|
| **P2.9** | main.go 0% coverage | Entry point completely untested; flag/signal handling unverified |
| **P2.10** | Concurrent access patterns untested | Store has RWMutex but locking never verified under race conditions |
| **P2.5** | AWS provider error scenarios incomplete | Some error paths not fully covered |

### LOWER PRIORITY — Quality (P3)
| ID | Issue | Impact |
|----|-------|--------|
| **P3.13** | Shallow reconciliation (3/19 resource types) | Orphaned resources (subnets, NAT GW, SGs, etc.) may not be detected; SyncedCount misleading |
| **P3.9** | Silent error suppression in store.go | Data loss/corruption could go undetected (lines 86, 123, 220) |
| **P3.11** | Non-atomic infrastructure updates | os.WriteFile without temp+rename pattern; corrupted files on interruption |
| **P3.10** | Missing error types (ErrCertificateInvalid, ErrInvalidInput) | Inconsistent error handling for TLS/validation scenarios |
| **P3.15** | Missing DNS state constants | Blocks P1.29 implementation |
| **P3.17** | No Route 53 client in awsclient | Blocks P1.29 Custom DNS implementation |
| **P3.16** | Missing input validations (ID format, ImageRef, AppDescription length, etc.) | Invalid inputs accepted without validation |
| **P3.12** | Missing state transitions | No deployment update or infrastructure retry transitions |
| **P3.14** | Main.go startup error handling (partial) | Background services not cleaned up on startup failure |

---

## ✅ Completed (All P0 Critical Issues Resolved)

<details>
<summary>Click to expand completed items</summary>

| Component | Status | Location |
|-----------|--------|----------|
| MCP server (stdio + HTTP) | ✅ | `internal/main.go` |
| Provider interface | ✅ | `internal/providers/provider.go` |
| **AWS 6 tools** | ✅ | `internal/providers/aws.go` — plan, approve, create, deploy, status, teardown |
| **AWS resource (aws:deployments)** | ✅ | `internal/providers/aws.go` |
| **AWS prompt (aws_deploy_plan)** | ✅ | `internal/providers/aws.go` |
| State model (Plan, Infrastructure, Deployment) | ✅ | `internal/state/types.go` |
| State storage with file persistence | ✅ | `internal/state/store.go` |
| Spending safeguards (config, Cost Explorer, monitoring, alerts) | ✅ | `internal/spending/` |
| Auto-teardown when budget exceeded | ✅ | `internal/main.go`, `internal/providers/` |
| Auto-scaling (CPU/memory target tracking) | ✅ | `internal/providers/aws.go` |
| TLS/HTTPS (ACM validation, HTTP redirect) | ✅ | `internal/providers/aws.go` |
| ECR image push (Docker SDK) | ✅ | `internal/providers/aws.go` |
| Plan approval workflow | ✅ | `internal/providers/aws.go`, `internal/state/store.go` |
| Rollback on failure | ✅ | `internal/providers/aws.go` |
| Private subnets with NAT Gateway | ✅ | `internal/providers/aws.go` |
| 24-hour plan cleanup | ✅ | `internal/state/cleanup.go` |
| CI workflow (lint, test, build) | ✅ | `.github/workflows/ci.yml` |
| Structured logging (slog) | ✅ | `internal/logging/logging.go` |
| Input validation (CPU/memory, port, region, etc.) | ✅ | `internal/providers/aws.go` (validations embedded in provider) |
| IAM task execution role | ✅ | `internal/providers/aws.go` |
| Test coverage 51% (target 50%) | ✅ | All packages |
| **P1.30 Distribution / cmd structure** | ✅ | `cmd/agent-deploy/main.go`, `.goreleaser.yml`, `.github/workflows/release.yml` |

**P1.30 Distribution Notes:**
- Entry point moved: `internal/main.go` → `cmd/agent-deploy/main.go`
- Added `.goreleaser.yml` and `.github/workflows/release.yml`
- Updated Makefile, CI workflow, README.md
- Fixed 2 test isolation bugs in `config_test.go` and `aws_test.go`

**P1.28 Container-Level Health Check** | ✅ | `internal/providers/aws.go`
- Added container-level health check to ECS task definition
- Uses curl to check health endpoint (CMD-SHELL healthcheck)
- Added health_check_grace_period parameter (default: 60s)
- Container health check interval: 30s, timeout: 5s, retries: 3
- Health check runs inside ECS container, independent of ALB health checks
- If container fails health check, ECS stops and replaces the task automatically
- Health check uses same path as ALB health check for consistency

**P1.22 Auto-Scaling Cost Range** | ✅ | `internal/providers/aws.go`
- Added min_count and max_count parameters to planInfraInput
- Added CostRange field to planInfraOutput (MinimumCostMo, MaximumCostMo, Note)
- When max_count > min_count, shows cost range like "$47.23–$188.92"
- Spending limit check uses max cost when auto-scaling enabled

</details>

---

## Current State Summary

| Component | Status | Notes |
|-----------|--------|-------|
| **AWS 6 tools** | ✅ Complete | plan, approve, create, deploy, status, teardown |
| **AWS resource + prompt** | ✅ Complete | aws:deployments, aws_deploy_plan |
| **Spending safeguards** | ✅ Working | Config, Cost Explorer, monitoring, alerts, auto-teardown |
| **State storage** | ✅ Complete | Plan, Infrastructure, Deployment with file persistence |
| **Reconciliation** | ⚠️ Partial | Only 3/19 resource types reconciled (VPC, ECS Cluster, ALB) |
| **Cost estimation** | ⚠️ Partial | Fargate via Pricing API; ALB/NAT/CW use hardcoded fallback |
| **Custom DNS / Route 53** | ❌ Not started | Entire spec `ralph/specs/custom-dns.md` unimplemented |
| **Distribution / cmd structure** | ✅ Complete | Entry point at `cmd/agent-deploy/main.go`, GoReleaser configured |
| **Test coverage** | ✅ 51% | Target 50% met; main.go at 0% |

---

## P1 — Spec Compliance Gaps (High Priority)

### P1.29 Custom DNS / Route 53 — ENTIRE SPEC UNIMPLEMENTED ❌

**Spec:** `ralph/specs/custom-dns.md`  
**Impact:** Users cannot map custom domain names to their deployments; no DNS automation

**Required Work:**
- [ ] Add optional `domain_name` parameter to `aws_plan_infra` tool
- [ ] Implement `findHostedZone()` — Route 53 hosted zone lookup with walk-up algorithm for subdomains
- [ ] Auto-provision ACM certificates with DNS validation when `domain_name` is provided
- [ ] Create Route 53 alias A record pointing custom domain to ALB
- [ ] Update `aws_status` output to show custom domain as primary URL
- [ ] Implement teardown: delete Route 53 record, ACM cert, DNS validation CNAME
- [ ] Add state constants: `ResourceDomainName`, `ResourceHostedZoneID`, `ResourceCertAutoCreated`, `ResourceDNSRecordName` to `internal/state/types.go`
- [ ] Add `github.com/aws/aws-sdk-go-v2/service/route53` SDK dependency
- [ ] Include Route 53 costs in plan estimation ($0.50/mo hosted zone + query costs)
- **Location:** `internal/providers/aws.go`, `internal/state/types.go`, `go.mod`

### P1.9 VPC CIDR Not Configurable ❌

**Spec:** `ralph/specs/networking.md`  
**Impact:** Cannot customize VPC for VPC peering scenarios

**Required Work:**
- [ ] Add `vpc_cidr` parameter to `planInfraInput` struct
- [ ] Remove hardcoded "10.0.0.0/16" at line 1076
- [ ] Implement `CalculateSubnetLayout()` function for dynamic subnet CIDR calculation
- [ ] Update subnet CIDRs calculation (lines 1145, 1180) to derive from `vpc_cidr`
- **Location:** `internal/providers/aws.go`

### P1.21 Per-Request Spending Override ❌

**Spec:** `ralph/specs/spending-safeguards.md`  
**Impact:** Users cannot set deployment-specific budget caps; global config only

**Required Work:**
- [ ] Add optional `monthly_budget_usd` and `per_deployment_usd` parameters to tool inputs
- [ ] Override global limits when per-request limits are provided
- [ ] Validate per-request limits do not exceed global limits
- **Location:** `internal/providers/aws.go` tool inputs

### P1.22 Auto-Scaling Cost Range ✅ COMPLETE

**Status:** Implemented

**Implementation:**
- [x] Added min_count and max_count parameters to planInfraInput
- [x] Added CostRange field to planInfraOutput (MinimumCostMo, MaximumCostMo, Note)
- [x] When max_count > min_count, shows cost range like "$47.23–$188.92"
- [x] Spending limit check uses max cost when auto-scaling enabled
- **Location:** `internal/providers/aws.go`

### P1.28 Container-Level Health Check ✅ COMPLETE

**Status:** Implemented

**Implementation:**
- [x] Added container health check configuration to ECS task definition
- [x] Added `health_check_grace_period` parameter (default: 60s)
- [x] Container health check interval: 30s, timeout: 5s, retries: 3
- [x] Uses CMD-SHELL healthcheck with curl to check health endpoint
- [x] Health check runs inside ECS container, independent of ALB health checks
- [x] If container fails health check, ECS stops and replaces the task automatically
- [x] Health check uses same path as ALB health check for consistency
- **Location:** `internal/providers/aws.go` task definition

---

## P2 — Test Coverage Gaps (Medium Priority)

> **Status:** 51% overall coverage achieved — exceeds the 50% target.
> CI enforces 25% floor; target is 50% per `ralph/specs/testing.md`.

### P2.9 main.go 0% Coverage ❌ 🔴

**Status:** CONFIRMED 0% COVERAGE  
**Impact:** Entry point completely untested; flag parsing bugs undetected

**Evidence:**
- `main_test.go` has 13 tests but **NONE test main()**
- Tests create isolated MCP servers via helper functions, bypassing main() entirely
- No flag parsing tests (-http, -log-level, -log-format, etc.)
- No signal handling tests (SIGINT, SIGTERM)

**Required Work:**
- [ ] Test `main()` function startup
- [ ] Test flag parsing behavior
- [ ] Test signal handling (SIGINT, SIGTERM)
- [ ] Test background service integration (cleanup service, cost monitor)
- [ ] Test exit codes on errors
- **Location:** `internal/main.go`, `internal/main_test.go`

### P2.10 Concurrent Access Patterns Untested ❌

**Status:** NOT TESTED  
**Impact:** Concurrent access bugs could go undetected

**Evidence:**
- store.go has RWMutex but ZERO concurrent tests
- No goroutine usage in store_test.go
- No `t.Parallel()` usage
- No race condition stress tests
- DeletePlan, DeleteInfra, DeleteExpiredPlans, ListInfra untested for concurrency

**Required Work:**
- [ ] Add concurrent read/write tests for Store
- [ ] Add race condition stress tests
- [ ] Verify RWMutex locking behavior under contention
- [ ] Test DeletePlan, DeleteInfra, DeleteExpiredPlans, ListInfra concurrently
- **Location:** `internal/state/store_test.go`

### P2.5 AWS Provider Error Scenarios Incomplete ⚠️

**Status:** Partial coverage (42.9%)  
**Impact:** Some error paths not fully covered

**Evidence:**
- Network errors untested
- AWS API errors untested
- Deployment failures untested
- Rollback scenarios only test empty infrastructure
- Context/deadline handling untested

**Required Work:**
- [ ] Test network error scenarios with AWS mocking
- [ ] Test AWS API error handling
- [ ] Test deployment failure scenarios
- [ ] Test rollback with non-empty infrastructure
- [ ] Test context/deadline handling
- **Location:** `internal/providers/aws_test.go`

---

## P3 — Quality Improvements (Lower Priority)

### P3.13 Shallow Reconciliation ❌

**Status:** PARTIAL — only 3 of 19 resource types reconciled  
**Impact:** Orphaned resources (subnets, NAT GW, SGs, etc.) may not be detected; SyncedCount is misleading (counts local entries, not AWS verification)

**Currently Reconciled (3):**
- VPC
- ECS Cluster
- ALB

**Missing (16):**
- Subnets (public and private)
- Route tables (public and private)
- Security groups (ALB and Task)
- NAT Gateway
- Elastic IP
- Internet Gateway
- IAM roles (execution role)
- CloudWatch Log Groups
- ECR repositories
- ECS services
- ECS task definitions
- Target groups
- Listeners

**Required Work:**
- [ ] Add reconciliation for all 19 resource types tracked in state
- [ ] Fix SyncedCount to verify against AWS instead of counting local entries
- **Location:** `internal/state/reconcile.go`

### P3.9 Silent Error Suppression in Store ❌

**Status:** NOT ADDRESSED  
**Impact:** Data loss could go undetected; debugging difficult

**Locations:**
- `store.go:86` — writeJSON errors on expiration state silently ignored with `_`
- `store.go:123` — file write errors silently ignored with `_`
- `store.go:220` — DeleteExpiredPlans delete errors silently skipped (comment says "Log but continue" but NO LOGGING)

**Required Work:**
- [ ] Add error logging for JSON marshal failures at line 86
- [ ] Add error logging for file write failures at line 123
- [ ] Add actual error logging for delete failures at line 220 (not just comment)
- **Location:** `internal/state/store.go:86,123,220`

### P3.11 Non-Atomic Infrastructure Updates ❌

**Status:** NOT ADDRESSED  
**Impact:** Potential data corruption if write is interrupted

**Evidence:**
- writeJSON uses os.WriteFile directly (not atomic)
- No temp file + rename pattern implemented
- Interrupted writes leave corrupted files

**Required Work:**
- [ ] Use temp file + rename pattern for atomic writes
- [ ] Ensure infrastructure updates are atomic
- **Location:** `internal/state/store.go`

### P3.10 Missing Error Types ⚠️

**Status:** PARTIAL  
**Impact:** Inconsistent error handling for specific scenarios

**Evidence:**
- ErrCertificateInvalid needed for TLS/ACM errors (not defined)
- ErrInvalidInput needed for validation errors (not defined)
- All 9 existing error types ARE used (ErrPlanNotApproved, ErrProvisioningFailed, ErrInvalidState)

**Required Work:**
- [ ] Add `ErrCertificateInvalid` for TLS/ACM errors
- [ ] Add `ErrInvalidInput` for validation errors
- **Location:** `internal/errors/errors.go`

### P3.15 Missing DNS State Constants ❌

**Status:** NOT IMPLEMENTED — blocks P1.29  
**Spec:** `ralph/specs/custom-dns.md`

**Required Work:**
- [ ] Add `ResourceDomainName` constant
- [ ] Add `ResourceHostedZoneID` constant
- [ ] Add `ResourceCertAutoCreated` constant
- [ ] Add `ResourceDNSRecordName` constant
- **Location:** `internal/state/types.go`

### P3.12 Missing State Transitions ❌

**Status:** NOT IMPLEMENTED  
**Impact:** Limited state management flexibility

**Required Work:**
- [ ] Add deployment update transition in state model
- [ ] Add infrastructure retry transition in state model
- **Location:** `internal/state/store.go`

### P3.14 Main.go Startup Error Handling ⚠️

**Status:** PARTIAL  
**Impact:** Partial startup could leave system in bad state

**Evidence:**
- `-enable-reconcile` flag IS properly wired up ✅
- Background services not cleaned up on startup failure
- Orphaned resources logged as warnings but not auto-teardown optioned

**Required Work:**
- [x] Wire up `-enable-reconcile` flag properly ✅ (already implemented)
- [ ] Clean up background services on startup failure
- [ ] Add optional `--auto-teardown-orphans` flag to remove orphaned resources
- **Location:** `internal/main.go`

### P3.16 Missing Input Validations ❌

**Status:** NOT IMPLEMENTED  
**Impact:** Invalid inputs accepted without validation; potential for malformed state or unexpected behavior

**Required Work:**
- [ ] Validate InfraID/PlanID/DeploymentID format (ULID format expected)
- [ ] Validate ImageRef format (should be valid Docker image reference, not just non-empty)
- [ ] Validate AppDescription max length (prevent excessively long descriptions)
- [ ] Validate ExpectedUsers range (should be positive, reasonable upper bound)
- [ ] Validate LatencyMS range (should be positive, reasonable values)
- [ ] Validate CertificateARN region matches deployment region
- **Location:** `internal/providers/aws.go`

### P3.17 No Route 53 Client in awsclient ❌

**Status:** NOT IMPLEMENTED  
**Impact:** Blocks P1.29 Custom DNS implementation
**Spec:** `ralph/specs/custom-dns.md`

**Required Work:**
- [ ] Add Route 53 client interface to `internal/awsclient/`
- [ ] Add `github.com/aws/aws-sdk-go-v2/service/route53` dependency to `go.mod`
- **Location:** `internal/awsclient/`, `go.mod`

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
- **Depends on:** P1.6 (environment variables) ✅

---

## Quick Reference

### Build & Run

```bash
make build           # Build the binary (cmd/agent-deploy/)
./agent-deploy       # Run (stdio mode)
./agent-deploy -http :8080  # Run (HTTP mode)

# Logging options
./agent-deploy -log-level debug    # Set log level (debug/info/warn/error, default: info)
./agent-deploy -log-format json    # Set log format (text/json, default: text)
./agent-deploy -http :8080 -log-level debug -log-format json  # Combined

# Background services
./agent-deploy -enable-cost-monitor    # Enable runtime cost monitoring (requires AWS credentials)
./agent-deploy -enable-auto-teardown   # Enable auto-teardown when budget exceeded
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

### Test Coverage Summary (current)

| Package | Coverage | Notes |
|---------|----------|-------|
| `internal/awsclient/` | **91.7%** | Comprehensive tests added |
| `internal/errors/` | **100%** | Comprehensive tests added |
| `internal/spending/config.go` | **100%** | Comprehensive tests added |
| `internal/providers/provider.go` | **80%** | All(), AllWithStore(), GetAWSProvider() tested |
| `internal/providers/aws.go` | **42.9%** | planInfra, deploy, teardown, status, approval workflows, provisionVPC, provisionECSCluster, provisionALB tested |
| `internal/main.go` | **0%** | Test file doesn't test main() |
| `internal/spending/` | **67.4%** | CostTracker, CostMonitor, PricingEstimator tests added |
| `internal/state/` | **82.0%** | Reconciler tests added, comprehensive coverage |
| **Overall** | **51.0%** | ✅ **TARGET MET** (target: 50%) |

### Key Files

| File | Purpose |
|------|---------|
| `cmd/agent-deploy/main.go` | MCP server entry point |
| `internal/providers/provider.go` | Provider interface + registration |
| `internal/providers/aws.go` | AWS provider (6 tools, 1 resource, 1 prompt) + all input validations |
| `internal/state/store.go` | File-backed state storage |
| `internal/state/types.go` | Plan, Infrastructure, Deployment structs + 18 ResourceType constants |
| `internal/state/reconcile.go` | State reconciliation with AWS resource tags (only 3/19 resources) |
| `internal/id/id.go` | ULID-based ID generation |
| `internal/awsclient/` | AWS SDK configuration (8 service interfaces: EC2, ECS, ELBV2, IAM, ECR, CloudWatch Logs, AutoScaling, ACM; NO Route 53) |
| `internal/spending/config.go` | Spending limits configuration |
| `internal/spending/check.go` | Pre-provisioning budget check |
| `internal/spending/costs.go` | AWS Cost Explorer integration |
| `internal/spending/monitor.go` | Runtime cost monitoring with alerts |
| `internal/spending/pricing.go` | AWS Pricing API for Fargate; hardcoded fallback for ALB/NAT/CloudWatch |
| `internal/state/cleanup.go` | Expired plan cleanup service |
| `internal/errors/errors.go` | Domain error types (9 defined; missing ErrCertificateInvalid, ErrInvalidInput) |
| `internal/logging/logging.go` | Structured logging with slog |
| `internal/main_test.go` | MCP server integration tests |
| `ralph/specs/aws-provider.md` | Tool/resource/prompt specifications |
| `ralph/specs/deployment-state.md` | State model and storage spec |
| `ralph/specs/spending-safeguards.md` | Budget enforcement spec |
| `ralph/specs/custom-dns.md` | Route 53 / custom domain spec |
| `ralph/specs/distribution.md` | Distribution / GoReleaser spec |
| `ralph/specs/ci.md` | CI/CD requirements spec |

### Hardcoded Values Summary

| Value | Location | Status |
|-------|----------|--------|
| VPC CIDR: `10.0.0.0/16` | `aws.go:1076` | ❌ HARDCODED — No `vpc_cidr` parameter (P1.9) |
| Subnet CIDRs | `aws.go:1145,1180` | ❌ HARDCODED — CIDRs calculated from fixed VPC CIDR (P1.9) |
| Fargate pricing | `pricing.go` | ✅ **IMPLEMENTED** — parsePricingResponse() extracts prices from AWS Pricing API |
| ALB pricing | `pricing.go:372-377` | ⚠️ HARDCODED FALLBACK — uses $20/mo when Pricing API unavailable |
| NAT Gateway pricing | `pricing.go:372-377` | ⚠️ HARDCODED FALLBACK — included in $15 base cost |
| CloudWatch Logs pricing | `pricing.go:372-377` | ⚠️ HARDCODED FALLBACK — no Pricing API call |
| ~~ECS Task CPU: `"256"`~~ | ~~`aws.go`~~ | ✅ Now configurable via `cpu` parameter (P1.17) |
| ~~ECS Task Memory: `"512"`~~ | ~~`aws.go`~~ | ✅ Now configurable via `memory` parameter (P1.17) |
| ~~ECS Desired Count: `1`~~ | ~~`aws.go`~~ | ✅ Now configurable (P1.5) |
| ~~Container Port: `80`~~ | ~~`aws.go`~~ | ✅ Now configurable (P1.3) |
| ~~Health Check Path: `"/"`~~ | ~~`aws.go`~~ | ✅ Now configurable (P1.4) |
| ~~Log Retention: `7` days~~ | ~~`aws.go`~~ | ✅ Now configurable via `log_retention_days` (P1.16) |
| ~~Default Image: `nginx:latest`~~ | ~~`aws.go`~~ | ✅ Removed — `image_ref` now required (P1.15) |
| ~~Current spend: `$25/deployment`~~ | ~~`aws.go`~~ | ✅ Uses Cost Explorer (P1.2) |

### Missing Validations Summary

| Validation | Location | Status |
|------------|----------|--------|
| Fargate CPU/Memory compatibility | `aws.go` | ✅ VALIDATED (P1.19) |
| Log retention (CloudWatch allowed values) | `aws.go` | ✅ VALIDATED (P1.20) |
| Container port (1-65535) | `aws.go` | ✅ VALIDATED (P1.23) |
| Environment variable names | `aws.go` | ✅ VALIDATED (P1.24) |
| Health check path (must start with /) | `aws.go` | ✅ VALIDATED (P1.25) |
| AWS region | `aws.go` | ✅ VALIDATED (P1.26) |
| Desired count upper limit | `aws.go` | ✅ VALIDATED (P1.27) |
| Auto-scaling params (minCount, maxCount, targetCPU, targetMem) | `aws.go` | ✅ VALIDATED |
| ACM certificate validation | `aws.go` | ✅ VALIDATED (via API) |
| InfraID/PlanID/DeploymentID format | `aws.go` | ❌ NOT VALIDATED (accepts any string) |
| ImageRef format (beyond empty check) | `aws.go` | ❌ NOT VALIDATED |
| AppDescription max length | `aws.go` | ❌ NOT VALIDATED |
| ExpectedUsers/LatencyMS range | `aws.go` | ❌ NOT VALIDATED |
| CertificateARN region match | `aws.go` | ❌ NOT VALIDATED |

### Remaining Work by Priority

| Priority | Count | Items |
|----------|-------|-------|
| **P0 Critical** | 0 | *(All P0 issues resolved)* |
| **P1 Spec Gaps** | 3 | P1.29 (Custom DNS), P1.9 (VPC CIDR), P1.21 (per-request spending) |
| **P2 Test Gaps** | 3 | P2.9 (main.go 0%), P2.10 (concurrent access), P2.5 (AWS error scenarios) |
| **P3 Quality** | 9 | P3.13 (shallow reconciliation), P3.15 (DNS state constants), P3.9 (silent errors), P3.10 (missing error types), P3.11 (non-atomic updates), P3.12 (state transitions), P3.14 (startup handling), P3.16 (missing validations), P3.17 (no Route 53 client) |
| **P5 Stretch** | 3 | CloudFormation, multi-cloud, secrets |
| **Total remaining** | **21** | |

---

## Spec Reference Summary (ralph/specs/)

| Spec | Requirement | Status |
|------|-------------|--------|
| **aws-provider.md** | 6 tools | ✅ Implemented (plan, approve, create, deploy, status, teardown) |
| **aws-provider.md** | 1 resource (aws:deployments) | ✅ Implemented |
| **aws-provider.md** | 1 prompt (aws_deploy_plan) | ✅ Implemented |
| **aws-provider.md** | AWS Pricing API for cost estimation | ✅ **IMPLEMENTED** — parsePricingResponse() extracts Fargate prices from AWS Pricing API |
| **aws-provider.md** | Wait for healthy deployment in aws_deploy | ✅ IMPLEMENTED — polls ECS + ALB health checks |
| **aws-provider.md** | TLS/HTTPS with ACM certificate support | ✅ IMPLEMENTED — TLS 1.2+ policy, HTTP-to-HTTPS redirect |
| **tls-https.md** | ACM certificate validation | ✅ IMPLEMENTED |
| **tls-https.md** | HTTP to HTTPS redirect | ✅ IMPLEMENTED |
| **tls-https.md** | TLS 1.2+ security policy | ✅ IMPLEMENTED |
| **aws-provider.md** | Plan approval before provisioning | ✅ IMPLEMENTED — explicit approval workflow |
| **plan-approval.md** | Explicit plan approval workflow | ✅ IMPLEMENTED |
| **plan-approval.md** | Plan rejection support | ✅ IMPLEMENTED |
| **plan-approval.md** | Plan expiration (24h) | ✅ IMPLEMENTED |
| **aws-provider.md** | Rollback on provisioning failure | ✅ IMPLEMENTED — rollbackInfra() cleans up partial resources |
| **ecr-image-push.md** | Push local images to ECR | ✅ **IMPLEMENTED** — P0.6 completed |
| **cost-estimation.md** | Fargate pricing via AWS Pricing API | ✅ IMPLEMENTED |
| **cost-estimation.md** | ALB/NAT/CloudWatch pricing via API | ⚠️ PARTIAL — uses hardcoded fallback values |
| **deploy-configuration.md** | Fargate CPU/memory validation | ✅ IMPLEMENTED — P1.19 |
| **deploy-configuration.md** | Log retention validation | ✅ IMPLEMENTED — P1.20 |
| **deploy-configuration.md** | Container port validation (1-65535) | ✅ IMPLEMENTED — P1.23 |
| **deploy-configuration.md** | Environment variables validation | ✅ IMPLEMENTED — P1.24 |
| **deploy-configuration.md** | Health check path validation (must start with /) | ✅ IMPLEMENTED — P1.25 |
| **custom-dns.md** | Route 53 hosted zone lookup | ❌ **NOT IMPLEMENTED** — P1.29 |
| **custom-dns.md** | ACM certificate auto-provisioning | ❌ **NOT IMPLEMENTED** — P1.29 |
| **custom-dns.md** | DNS alias A record creation | ❌ **NOT IMPLEMENTED** — P1.29 |
| **custom-dns.md** | DNS resource tracking in state | ❌ **NOT IMPLEMENTED** — P3.15 |
| **distribution.md** | Move main.go to cmd/agent-deploy/ | ✅ IMPLEMENTED |
| **distribution.md** | GoReleaser + release workflow | ✅ IMPLEMENTED |
| **distribution.md** | `go install` support | ✅ IMPLEMENTED |
| **deployment-state.md** | Plan, Infrastructure, Deployment types | ✅ Implemented |
| **deployment-state.md** | File-backed JSON at ~/.agent-deploy/state/ | ✅ Implemented |
| **deployment-state.md** | 24-hour plan expiration, hourly cleanup | ✅ Implemented |
| **deployment-state.md** | AWS resource tag reconciliation | ⚠️ **PARTIAL** — only 3 of 19 resource types reconciled (P3.13) |
| **spending-safeguards.md** | monthly_budget_usd, per_deployment_usd, alert_threshold_percent | ✅ Implemented |
| **spending-safeguards.md** | Pre-provisioning budget check | ⚠️ PARTIAL — Cost Explorer works, but ALB/NAT/CW pricing uses hardcoded fallback |
| **spending-safeguards.md** | Runtime cost monitoring with Cost Explorer | ✅ Implemented |
| **spending-safeguards.md** | Auto-teardown when budget exceeded | ✅ IMPLEMENTED |
| **spending-safeguards.md** | Per-request spending limit overrides | ❌ NOT IMPLEMENTED — P1.21 |
| **spending-safeguards.md** | Resource tagging | ✅ Implemented |
| **auto-scaling.md** | Auto-scaling with target tracking | ✅ IMPLEMENTED — CPU/memory policies, cooldowns, cleanup |
| **auto-scaling.md** | Cost range in planInfra output (min/max) | ✅ IMPLEMENTED |
| **networking.md** | VPC CIDR configurable | ❌ NOT IMPLEMENTED — hardcoded to 10.0.0.0/16 (P1.9) |
| **networking.md** | Private subnets with NAT Gateway | ✅ IMPLEMENTED |
| **ci.md** | CI workflow with lint, test, build jobs | ✅ IMPLEMENTED |
| **testing.md** | 50% code coverage | ✅ **TARGET MET** — 51% overall |
| **testing.md** | main.go test coverage | ❌ **0% COVERAGE** — P2.9 |
| **testing.md** | Concurrent access testing | ❌ NOT TESTED — P2.10 |
| **error-handling.md** | Domain error types | ⚠️ PARTIAL — missing ErrCertificateInvalid, ErrInvalidInput (P3.10) |
| **operational.md** | No silent error suppression | ❌ NOT ADDRESSED — store.go:86,123,220 (P3.9) |
| **operational.md** | Pagination for list operations | ✅ IMPLEMENTED |
