# Implementation Plan

**Project Goal:** Natural language deployment of applications via MCP server → Cloud provider. Allow users to end-to-end create applications and make them publicly available while ensuring spend does not cross user-defined boundaries.

**Last Updated:** 2026-03-30  
**Last Audit:** 2026-03-30 (Deep audit — comprehensive codebase analysis)

## Priority Definitions

- **P0**: Critical production blockers (data loss, security, crashes)
- **P1**: High priority — spec compliance gaps AND author-prioritized features
- **P2**: Medium — test coverage gaps
- **P3**: Lower — quality improvements
- **P4**: New features (default for unimplemented specs WITHOUT explicit priority)
- **P5**: Stretch goals

**Current Status:**
- ✅ Coverage: 52.9% (meets 50% target)
- ✅ All P0 critical issues resolved (P0.1, P0.2, P0.3)
- ✅ P1.29 Custom DNS — 100% complete (P1.35 status URL gap fixed)
- ✅ P1.36 Spending confirmation — 100% complete (warns when using defaults)
- ✅ P3.32 Reconcile error handling — fixed silent error suppression
- ✅ P1.34 Lightsail provider — 100% complete (auto-selects backend, cost comparison)
- 🔴 P1.37 Static Site workload — workload-roadmap.md priority P1, 0% implemented
- 🔴 P1.38 Background Worker workload — workload-roadmap.md priority P1, 0% implemented

---

## 🚨 Remaining Work Summary

### CRITICAL — Production Blockers (P0)
| ID | Issue | Status | Impact |
|----|-------|--------|--------|
| ✅ ~~P0.3~~ | ~~Provider nil store checks~~ | ✅ FIXED | All 11 `p.store.*` accesses now guarded with `checkStore()` method. Silent error suppression on lines 967-968 fixed with proper logging. |

### HIGH PRIORITY — Spec Compliance Gaps (P1)
| ID | Issue | Status | Impact |
|----|-------|--------|--------|
| ✅ ~~P1.34~~ | ~~Lightsail provider not implemented~~ | ✅ COMPLETE | Full Lightsail provider implemented with auto-backend selection, cost comparison, and $7-25/mo deployments. |
| 🚨 **P1.37** | **Static Site workload not implemented** | ❌ | **Priority P1** per `ralph/specs/workload-roadmap.md`. S3+CloudFront = $1-5/mo vs $65+/mo. No CloudFront, no S3 bucket provisioning code exists. |
| 🚨 **P1.38** | **Background Worker workload not implemented** | ❌ | **Priority P1** per `ralph/specs/workload-roadmap.md`. SQS+Lambda/Fargate without ALB. No SQS, no worker patterns implemented. |
| ✅ ~~P1.35~~ | ~~Custom DNS status URL gap~~ | ✅ FIXED | Custom domain now included in `aws_status` URL list and `custom_domain` field. |
| ✅ ~~P1.36~~ | ~~Spending confirmation gap~~ | ✅ FIXED | Plan output now includes `requires_confirmation` when using default limits. |

### MEDIUM PRIORITY — Test Gaps (P2)
| ID | Issue | Status | Impact |
|----|-------|--------|--------|
| **P2.5** | AWS provider error scenarios incomplete | ⚠️ | 48.8% coverage; Route53/ALB/IAM error paths untested |
| **P2.11** | `processAlert()` untested | ❌ | `internal/spending/monitor.go` — alert processing path has zero test coverage |
| **P2.12** | `checkInfraResources()` untested | ❌ | `internal/state/reconcile.go` — infrastructure resource checking has zero test coverage |
| **P2.13** | Sleep-based timing in tests | ⚠️ | Flaky CI risk — 8 instances of `time.Sleep` in tests instead of channels/conditions |
| **P2.14** | Error injection missing in reconcile mocks | ❌ | Reconcile tests only cover happy paths; no failure simulation |
| **P2.15** | main.go signal/HTTP gaps | ❌ | SIGINT/SIGTERM untested, HTTP server untested, graceful shutdown untested |

### LOWER PRIORITY — Quality (P3)
| ID | Issue | Status | Impact |
|----|-------|--------|--------|
| **P3.13** | Shallow reconciliation (3/25 resource types) | ⚠️ | Only VPC, ECS cluster, ALB reconciled; orphaned resources (subnets, NAT GW, SGs, etc.) may not be detected |
| **P3.19** | Hardcoded ALB/NAT/CloudWatch pricing | ⚠️ | Cost estimation inaccurate; Pricing API not wired into EstimateCosts() |
| **P3.20** | NAT Gateway single AZ | ⚠️ | Single point of failure for private subnet traffic; no HA |
| **P3.31** | `deleteDNSResources()` error not checked | ❌ | Called without error checking in `teardown()` (~line 1510); should return and propagate error |
| ✅ ~~P3.32~~ | ~~Reconcile error handling issues~~ | ✅ FIXED | Errors now logged/returned instead of silently ignored |

### CI/CD GAPS (P3)
| ID | Issue | Status | Impact |
|----|-------|--------|--------|
| **P3.27** | Missing security scanning in CI | ❌ | No gosec linter, no govulncheck |
| **P3.28** | Missing dependency audit in CI | ❌ | No `go mod verify` check |
| **P3.29** | No SBOM generation | ❌ | No software bill of materials for releases |
| **P3.30** | No goreleaser --validate check | ❌ | .goreleaser.yml syntax not validated pre-release |

### NEW FEATURES (P4)
| ID | Issue | Status | Spec Priority | Impact |
|----|-------|--------|---------------|--------|
| **P4.4** | Scheduled job workload | ❌ | P2 | EventBridge+Lambda — 0% implemented |
| **P4.5** | Batch processing workload | ❌ | P2 | AWS Batch — 0% implemented |
| **P4.6** | ML inference workload | ❌ | P3 | SageMaker/GPU Fargate — 0% implemented |
| **P4.7** | Data pipeline workload | ❌ | P3 | Step Functions — 0% implemented |

### STRETCH GOALS (P5)
| ID | Issue | Status | Impact |
|----|-------|--------|--------|
| **P5.1** | CloudFormation-based provisioning | ❌ | Simplifies create/teardown; atomic operations |
| **P5.2** | Additional cloud providers (GCP, Azure) | ❌ | Multi-cloud support |
| **P5.3** | Secrets Management | ❌ | AWS Secrets Manager / SSM integration |

---

## ✅ Completed (All Verified)

<details>
<summary>Click to expand completed items</summary>

### Core Features
| Component | Status | Location |
|-----------|--------|----------|
| MCP server (stdio + HTTP) | ✅ | `cmd/agent-deploy/main.go` |
| Provider interface | ✅ | `internal/providers/provider.go` |
| **AWS 6 tools** | ✅ | `internal/providers/aws.go` — plan, approve, create, deploy, status, teardown |
| **AWS resource (aws:deployments)** | ✅ | `internal/providers/aws.go` |
| **AWS prompt (aws_deploy_plan)** | ✅ | `internal/providers/aws.go` |
| State model (Plan, Infrastructure, Deployment) | ✅ | `internal/state/types.go` |
| State storage with file persistence | ✅ | `internal/state/store.go` |
| Spending safeguards (config, Cost Explorer, monitoring, alerts) | ✅ | `internal/spending/` |
| Auto-teardown when budget exceeded | ✅ | `cmd/agent-deploy/main.go`, `internal/providers/` |
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
| Test coverage 52.9% (target 50%) | ✅ | Meets target |
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

**P0.1 Non-Atomic File Writes** | ✅ | `internal/state/store.go`
- Implemented atomic writes using temp file + rename pattern
- `writeJSON` now: creates temp file in same directory, writes data, syncs to disk, sets permissions, renames atomically
- Added tests: `TestAtomicWrites` and `TestConcurrentWrites`

**P0.2 Silent Error Suppression in Store** | ✅ | `internal/state/store.go`
- Line 86 (ApprovePlan): Changed from `_ = s.writeJSON(...)` to logging error with `slog.Error`
- Line 123 (RejectPlan): Changed from `_ = s.writeJSON(...)` to logging error with `slog.Error`
- Lines 218-220 (DeleteExpiredPlans): Added `slog.Warn` logging for delete failures

**P3.26 Race Condition in monitor_test.go** | ✅ | `internal/spending/monitor_test.go`
- Added mutex synchronization for callCount variable in `TestCostMonitor_CheckNow`
- Was causing intermittent race detector failures

**P3.10 Error Types** | ✅ | `internal/errors/errors.go`
- All 9 required error types defined and properly wired (ErrPlanNotApproved, ErrProvisioningFailed, ErrInvalidState, etc.)
- Code audit confirmed ErrCertificateInvalid and ErrInvalidInput are NOT required by spec

</details>

---

## Current State Summary

| Component | Status | Notes |
|-----------|--------|-------|
| **AWS 6 tools** | ✅ Complete | plan, approve, create, deploy, status, teardown |
| **AWS resource + prompt** | ✅ Complete | aws:deployments, aws_deploy_plan |
| **Spending safeguards** | ⚠️ Gap | Config, Cost Explorer, monitoring, alerts, auto-teardown working; missing confirmation when no limits set (P1.36) |
| **State storage** | ✅ Complete | Plan, Infrastructure, Deployment with file persistence |
| **Provider safety** | ✅ Complete | All `p.store.*` accesses guarded with `checkStore()` method (P0.3) |
| **Reconciliation** | ⚠️ Partial | Only 3/25 resource types reconciled (P3.13); error handling issues (P3.32) |
| **Cost estimation** | ⚠️ Partial | Fargate OK; ALB/NAT/CW hardcoded (P3.19) |
| **Networking** | ⚠️ Partial | NAT Gateway single AZ only (P3.20) |
| **Custom DNS / Route 53** | ✅ Complete | Core working; status URL includes custom domain (P1.35); DNS teardown error unchecked (P3.31) |
| **Distribution** | ✅ Complete | `cmd/agent-deploy/main.go`, GoReleaser configured |
| **Test coverage** | ✅ 52.9% | Meets 50% target; gaps in processAlert, checkInfraResources, main.go signal/HTTP (P2.11-P2.15) |
| **CI/CD** | ⚠️ Partial | Missing security scanning, SBOM (P3.27-P3.30) |
| **Error handling** | ✅ Complete | All 9 error types defined, no silent suppression |
| **Logging** | ✅ Complete | Structured slog with component tags |
| **Lightsail provider** | ✅ Complete | Auto-selects backend, Lightsail vs ECS cost comparison (P1.34) |
| **Static Site workload** | 🔴 Missing | 0% implemented — priority P1 per workload-roadmap.md (P1.37) |
| **Background Worker workload** | 🔴 Missing | 0% implemented — priority P1 per workload-roadmap.md (P1.38) |

---

## P1 — Spec Compliance Gaps

### P1.29 Custom DNS / Route 53 ⚠️ 95% COMPLETE

**Spec:** `ralph/specs/custom-dns.md`  
**Status:** Core implementation complete; **status URL gap remaining** (see P1.35)

**Implementation:**
- [x] Added `domain_name` parameter to `aws_plan_infra` tool
- [x] Added `DomainName` field to `state.Plan` struct
- [x] Added `ValidateDomainName()` and `extractParentDomain()` validation functions
- [x] Implemented `findHostedZone()` — Route 53 hosted zone lookup with walk-up algorithm for subdomains
- [x] Auto-provision ACM certificates with DNS validation when `domain_name` is provided (`provisionCertificate()`)
- [x] Create Route 53 alias A record pointing custom domain to ALB (`createDNSRecord()`)
- [x] Implement teardown: delete Route 53 record, ACM cert, DNS validation CNAME (`deleteDNSResources()`)
- [x] Added state constants: `ResourceDomainName`, `ResourceHostedZoneID`, `ResourceCertAutoCreated`, `ResourceDNSRecordName`
- [x] Added `github.com/aws/aws-sdk-go-v2/service/route53` SDK dependency
- [x] Added Route53API interface and Route53Mock
- [x] Added ACMAPI methods: `RequestCertificate`, `DeleteCertificate`
- [x] Integrated DNS provisioning into `createInfra`
- [x] Added DNS cleanup to `teardown`
- [x] Added tests: `TestValidateDomainName`, `TestExtractParentDomain`, `TestPlanInfra_DomainName`
- **Location:** `internal/providers/aws.go`, `internal/state/types.go`, `go.mod`

**P1.35 Status URL Gap: ✅ FIXED**
- [x] Added `custom_domain` field to `statusOutput` so `aws_status` URL list includes the custom domain
- [x] Updated `getALBURLs()` to return custom domain URL first when configured
- [x] Added tests: `TestStatusOutput_CustomDomain`, `TestStatusOutput_NoCustomDomain`
- **Location:** `internal/providers/aws.go`

### P1.9 VPC CIDR Configuration ✅ COMPLETE

**Status:** Implemented

**Implementation:**
- [x] Added VpcCIDR parameter to planInfraInput (default: "10.0.0.0/16")
- [x] Added VpcCIDR field to state.Plan struct
- [x] Added ValidateVpcCIDR() function (validates /16 to /24, IPv4 only)
- [x] Added CalculateSubnetLayout() function to derive subnet CIDRs from VPC CIDR
- [x] Updated provisionVPC to accept vpcCIDR parameter and use calculated subnet layout
- [x] Added tests: TestValidateVpcCIDR, TestCalculateSubnetLayout, TestPlanInfra_VpcCIDR
- **Location:** `internal/providers/aws.go`, `internal/state/types.go`

### P1.21 Per-Request Spending Override ✅ COMPLETE

**Status:** Implemented

**Implementation:**
- [x] Added per_deployment_budget_usd parameter to planInfraInput and createInfraInput
- [x] Override is validated to not exceed global per-deployment limit
- [x] When override is provided, uses it instead of global config
- [x] Logs when using per-request override
- **Location:** `internal/providers/aws.go`

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

### P1.31 Missing Input Validations ✅ COMPLETE

**Status:** FIXED  
**Impact:** Invalid inputs now properly validated; prevents malformed state and security issues

**Implementation:**
- [x] Added `ValidateID()`: validates ULID format with backwards-compatible legacy ID support
- [x] Added `ValidateImageRef()`: validates Docker image reference format
- [x] Added `ValidateAppDescription()`: enforces max length of 1024 chars
- [x] Added `ValidateExpectedUsers()`: enforces range 1 to 100 million
- [x] Added `ValidateLatencyMS()`: enforces range 1 to 60000 ms
- [x] Added `ValidateCertificateARNRegion()`: validates cert ARN region matches deployment region
- [x] Integrated validations into: `planInfra()`, `approvePlan()`, `createInfra()`, `deploy()`, `status()`, `teardown()`
- [x] Added comprehensive tests for all new validation functions
- **Location:** `internal/providers/aws.go`

---

## P0 — Critical Production Blockers (Must Fix)

### P0.3 Provider Nil Store Checks ✅ COMPLETE

**Status:** FIXED  
**Impact:** **CRITICAL** — Prevented nil pointer panic when store initialization fails

**Resolution:**
- Added `checkStore()` helper method that returns `ErrInvalidState` if store is nil
- Added nil store guard to all 7 affected methods: `planInfra`, `approvePlan`, `createInfra`, `deploy`, `status`, `teardown`, `deploymentsResource`
- Fixed silent error suppression on lines 967-968: now logs errors with `slog.Warn` instead of ignoring with `_, _`
- Added comprehensive test `TestNilStoreGuard` covering all 7 methods
- **Location:** `internal/providers/aws.go`, `internal/providers/aws_test.go`

### P0.1 Non-Atomic File Writes ✅ COMPLETE

**Status:** FIXED  
**Impact:** **CRITICAL** — Data corruption if write is interrupted; state files corrupted on crash

**Resolution:**
- Implemented atomic writes using temp file + rename pattern in `store.go`
- `writeJSON` now: creates temp file in same directory, writes data, syncs to disk, sets permissions, renames atomically
- Added tests: `TestAtomicWrites` and `TestConcurrentWrites`
- **Location:** `internal/state/store.go`

### P0.2 Silent Error Suppression in Store ✅ COMPLETE

**Status:** FIXED  
**Impact:** **CRITICAL** — Data loss goes undetected; debugging impossible

**Resolution:**
- Line 86 (ApprovePlan): Changed from `_ = s.writeJSON(...)` to logging error with `slog.Error`
- Line 123 (RejectPlan): Changed from `_ = s.writeJSON(...)` to logging error with `slog.Error`
- Lines 218-220 (DeleteExpiredPlans): Added `slog.Warn` logging for delete failures
- **Location:** `internal/state/store.go`

---

### ~~P1.32 Route53 Client Not Initialized~~ ✅ NOT A BUG

**Status:** RESOLVED — False positive  
**Resolution:** The Route53 client uses a **lazy initialization pattern** which is working correctly.

**Analysis:**
- DNS functions (`findHostedZone`, `createDNSRecord`, `deleteDNSResources`) create Route53 clients on-the-fly if not injected for testing
- This is the **intentional design** — creates client when needed, avoids unnecessary initialization
- Test mocking still works: tests inject Route53Mock into the provider when needed
- No nil pointer panic — the on-the-fly client creation ensures a valid client is always available

**No work required** — this was a false positive in the original audit.

### P1.33 DNS Deletion Placeholder ✅ COMPLETE

**Status:** FIXED  
**Impact:** DNS deletion now uses actual infrastructure state; works in all AWS regions

**Implementation:**
- [x] Added state constants: `ResourceALBDNSName` and `ResourceALBHostedZoneID`
- [x] Modified `createDNSRecord()` to store ALB DNS name and hosted zone ID during creation
- [x] Modified `deleteDNSResources()` to use stored values instead of hardcoded placeholder
- [x] Now works in all AWS regions (not just us-east-1)
- [x] Added graceful fallback with warning for older deployments missing ALB DNS data
- [x] Added tests: `TestResourceDNSConstants`, `TestInfraResources_ALBDNSData`
- **Location:** `internal/providers/aws.go`, `internal/state/types.go`

### P1.35 Custom DNS Status URL Gap ❌

**Status:** NOT ADDRESSED  
**Spec:** `ralph/specs/custom-dns.md`  
**Impact:** Spec requires custom domain in `aws_status` URL list; users cannot see their custom domain in status output

### P1.35 Custom DNS Status URL Gap ✅ COMPLETE

**Status:** FIXED  
**Spec:** `ralph/specs/custom-dns.md`  
**Impact:** Spec requires custom domain in `aws_status` URL list; now properly included

**Resolution:**
- Added `custom_domain` field to `statusOutput` struct with `omitempty` tag
- Updated `getALBURLs()` to include custom domain URL first (as primary URL) when configured
- Custom domain URL now appears before ALB DNS in URLs list
- Status method populates `CustomDomain` field from stored state (`ResourceDomainName`)
- Added tests: `TestStatusOutput_CustomDomain`, `TestStatusOutput_NoCustomDomain`
- **Location:** `internal/providers/aws.go`, `internal/providers/aws_test.go`

### P1.36 Spending Confirmation Gap ✅ COMPLETE

**Status:** FIXED  
**Spec:** `ralph/specs/spending-safeguards.md`  
**Impact:** Spec requires "allow with confirmation" when no spending limits configured; now properly warns user

**Resolution:**
- Added `LimitsWithSource` struct with `ExplicitlyConfigured` flag
- Added `LoadLimitsWithSource()` function to detect if limits came from config file or env vars
- Added `RequiresConfirmation` and `ConfirmationReason` fields to `planInfraOutput`
- When no explicit config exists, sets `requires_confirmation: true` with explanation
- Per-request spending override counts as explicit configuration (no confirmation needed)
- Plan summary now includes warning when using default limits
- Added tests: `TestLoadLimitsWithSource_NoConfig`, `TestLoadLimitsWithSource_WithConfigFile`, `TestLoadLimitsWithSource_WithEnvVars`
- **Location:** `internal/spending/config.go`, `internal/providers/aws.go`, `internal/spending/config_test.go`

### P1.37 Static Site Workload ❌

**Status:** NOT IMPLEMENTED — Priority P1 per `ralph/specs/workload-roadmap.md`  
**Spec:** `ralph/specs/workloads/static-site.md`  
**Impact:** Would enable ultra-cheap static deployments ($1-5/mo vs $65+/mo with ECS)

**Evidence:**
- No CloudFront references in any .go files
- No S3 bucket provisioning code exists
- `workload-roadmap.md` explicitly lists "Static site" as Priority P1

**Benefits:**
- S3 + CloudFront = extremely low cost
- Global CDN distribution
- Perfect for docs, landing pages, SPAs
- No container overhead

**Required Work:**
- [ ] Add `workload_type` parameter to `aws_plan_infra` tool
- [ ] Add workload type constants to `state/types.go`
- [ ] Implement S3 bucket creation for static assets
- [ ] Implement CloudFront distribution with S3 origin
- [ ] Support custom domains via Route 53 (leverage existing DNS code)
- [ ] Add S3/CloudFront SDK dependencies to `go.mod`
- [ ] Add cost estimation for S3+CloudFront
- [ ] Add tests for static site provisioning
- **Location:** `internal/providers/aws.go` or `internal/providers/aws_static.go`

### P1.38 Background Worker Workload ❌

**Status:** NOT IMPLEMENTED — Priority P1 per `ralph/specs/workload-roadmap.md`  
**Spec:** `ralph/specs/workloads/background-worker.md`  
**Impact:** Would enable cost-effective background processing without ALB overhead

**Evidence:**
- No SQS references in any .go files
- No worker patterns implemented
- `workload-roadmap.md` explicitly lists "Background worker" as Priority P1

**Benefits:**
- No ALB required = reduced cost
- SQS integration for job queues
- ECS Fargate or Lambda backends
- Good for async processing, queue workers

**Required Work:**
- [ ] Add `background_worker` workload type constant
- [ ] Modify `createInfra` to skip ALB provisioning for workers
- [ ] Implement SQS queue creation and configuration
- [ ] Configure ECS service without load balancer
- [ ] Add SQS SDK dependency to `go.mod`
- [ ] Add cost estimation for SQS+Fargate (no ALB)
- [ ] Add tests for background worker provisioning
- **Location:** `internal/providers/aws.go`

### P1.34 Lightsail Provider ✅ COMPLETE

**Status:** FULLY IMPLEMENTED  
**Spec:** `ralph/specs/lightsail-provider.md`  
**Impact:** Enables $7-25/mo deployments vs $65+/mo with ECS Fargate

**Implementation Summary:**
- Added `Backend` field to `state.Plan` struct in `internal/state/types.go`
- Added Lightsail resource constants: `ResourceLightsailService`, `ResourceLightsailEndpoint`, `ResourceLightsailPower`, `ResourceLightsailNodes`
- Added `LightsailAPI` interface to `internal/awsclient/interfaces.go`
- Added `LightsailMock` in `internal/awsclient/mocks/lightsail.go`
- Added Lightsail SDK dependency `github.com/aws/aws-sdk-go-v2/service/lightsail`

**Key Functions in `internal/providers/aws.go`:**
- `selectBackend()`: Auto-selects Lightsail vs ECS Fargate based on workload signals (expected users, latency, auto-scaling needs)
- `selectLightsailPower()`: Chooses power level (nano, micro, small, medium, large, xlarge)
- `calculateLightsailNodes()`: Determines node count based on expected users
- `createLightsailService()`: Provisions Lightsail container service
- `deployToLightsail()`: Deploys container to Lightsail
- `teardownLightsail()`: Deletes Lightsail service
- `getLightsailStatus()`: Gets status of Lightsail deployment
- `lightsailPowerPricing`: Fixed pricing map for cost estimation

**Modified Tool Flows:**
- `planInfra()`: Selects backend and shows Lightsail vs ECS cost comparison
- `createInfra()`: Branches between Lightsail and ECS Fargate paths
- `deploy()`: Handles Lightsail deployments
- `status()`: Retrieves Lightsail status
- `teardown()`: Handles Lightsail teardown

**Completed Work:**
- [x] Auto-backend selection based on workload signals
- [x] Lightsail container service provisioning
- [x] Container deployment to Lightsail
- [x] Lightsail SDK dependency added to `go.mod`
- [x] Cost estimation for Lightsail tiers with fixed pricing map
- [x] Comprehensive tests for backend selection
- **Location:** `internal/providers/aws.go`

---

## P2 — Test Coverage Gaps (Medium Priority)

> **Status:** 52.9% overall coverage — target met.
> CI enforces 25% floor; target is 50% per `ralph/specs/testing.md`.

### P2.9 main.go Test Coverage ⚠️ 🟡

**Status:** SIGNIFICANT PROGRESS — 52.9% overall coverage achieved  
**Impact:** Entry point components now tested; main() function itself remains architecturally challenging to test

**New Tests Added:**
- **TestVersion** — verifies Version constant
- **TestFlagDefaults** — verifies all flag default values
- **TestLoggingInitialization** — tests various logging configurations
- **TestStateStoreInitialization** — verifies store creation
- **TestCleanupServiceIntegration** — tests cleanup service with store
- **TestProvidersWithStore** — verifies provider creation with store
- **TestAWSProviderRetrieval** — tests GetAWSProvider function
- **TestMCPServerCreation** — verifies MCP server creation
- **TestEnvironmentVariableConfiguration** — tests env var handling

**Remaining Challenges:**
- The `main()` function itself is hard to test directly (runs forever with signal handling)
- Components used by main() are now covered through the tests above

**Completed Work:**
- [x] Test flag parsing behavior
- [x] Test logging initialization
- [x] Test state store initialization
- [x] Test cleanup service integration
- [x] Test provider creation with store
- [x] Test MCP server creation
- [x] Test environment variable configuration
- **Location:** `cmd/agent-deploy/main.go`, `cmd/agent-deploy/main_test.go`

**Not Feasible:**
- [ ] Direct `main()` function testing (runs forever with signal handling — architecturally challenging)

### P2.10 Concurrent Access Patterns Untested ✅ COMPLETE

**Status:** ✅ COMPLETE  
**Impact:** ~~Concurrent access bugs could go undetected~~ RWMutex locking verified correct

**Evidence:**
- ~~store.go has RWMutex but ZERO concurrent tests~~ ✅ Comprehensive concurrent tests added
- ~~No goroutine usage in store_test.go~~ ✅ Multiple goroutine-based tests
- ~~No `t.Parallel()` usage~~ ✅ Tests run with -race flag
- ~~No race condition stress tests~~ ✅ Race condition stress tests pass
- ~~DeletePlan, DeleteInfra, DeleteExpiredPlans, ListInfra untested for concurrency~~ ✅ All tested

**Completed Work:**
- [x] Add concurrent read/write tests for Store
- [x] Add race condition stress tests
- [x] Verify RWMutex locking behavior under contention
- [x] Test DeletePlan, DeleteInfra, DeleteExpiredPlans, ListInfra concurrently
- **Location:** `internal/state/store_test.go`

**Tests Added:**
1. `TestConcurrentPlanOperations` - Tests concurrent plan create/read/delete with 20 goroutines × 5 plans each
2. `TestConcurrentMixedReadWrite` - Tests 50 concurrent readers + 10 concurrent writers on deployments
3. `TestConcurrentListOperations` - Tests 20 concurrent list operations with 50 iterations each
4. `TestConcurrentDeleteOperations` - Tests 5 concurrent goroutines trying to delete same items

All tests pass with `-race` flag, verifying the RWMutex locking is correct.

### P2.5 AWS Provider Error Scenarios Incomplete ⚠️

**Status:** Partial coverage (48.8%)  
**Impact:** Error paths not fully covered; missing E2E flow tests

**Progress (2025-01):**
Added VPC cleanup, route table, and rollback error scenario tests:
- `TestDeleteVPCResources_Success` - Tests full VPC resource cleanup with proper deletion order
- `TestDeleteVPCResources_EmptyInfra` - Tests cleanup of empty infrastructure
- `TestDeleteVPCResources_VPCDeleteError` - Tests VPC delete error propagation
- `TestDeleteVPCResources_PartialFailureContinues` - Tests that non-VPC errors don't block cleanup
- `TestDeleteRouteTable_WithAssociations` - Tests route table disassociation before deletion
- `TestDeleteRouteTable_DescribeError` - Tests describe error handling
- `TestDeleteRouteTable_DisassociateError` - Tests disassociate error handling
- `TestRollbackInfra_WithResources` - Tests rollback with resources to clean up
- `TestRollbackInfra_ContinuesOnErrors` - Tests rollback continues despite errors

Coverage improved from 44.6% → 48.8% on providers package.

**Evidence (remaining):**
- Route 53 error scenarios untested
- ALB provisioning error paths untested
- IAM/ECR/CloudWatch error handling untested
- No E2E flow tests (provision → deploy → teardown)
- Context/deadline handling untested

**Required Work:**
- [ ] Test Route 53 error scenarios (hosted zone not found, DNS validation timeout, etc.)
- [ ] Test ALB provisioning error paths (target group creation, listener setup, health checks)
- [ ] Test IAM/ECR/CloudWatch error handling
- [ ] Add E2E flow tests (full provision → deploy → teardown cycles)
- [x] Test rollback with non-empty infrastructure
- [ ] Test context/deadline handling
- **Location:** `internal/providers/aws_test.go`

### P2.11 processAlert() Untested ❌

**Status:** NOT ADDRESSED  
**Impact:** Alert processing logic in cost monitor has zero test coverage; bugs in alerting would go undetected

**Required Work:**
- [ ] Add tests for `processAlert()` covering: threshold exceeded, threshold not exceeded, edge cases
- [ ] Test alert callback invocation and error handling
- **Location:** `internal/spending/monitor.go`, `internal/spending/monitor_test.go`

### P2.12 checkInfraResources() Untested ❌

**Status:** NOT ADDRESSED  
**Impact:** Infrastructure resource checking in reconciler untested; reconciliation bugs could cause false positives/negatives

**Required Work:**
- [ ] Add tests for `checkInfraResources()` with various resource states
- [ ] Test with missing resources, stale resources, and healthy resources
- **Location:** `internal/state/reconcile.go`, `internal/state/reconcile_test.go`

### P2.13 Sleep-Based Timing in Tests ⚠️

**Status:** NOT ADDRESSED  
**Impact:** Tests using `time.Sleep` are inherently flaky in CI; timing-sensitive assertions may fail under load

**Evidence:**
- 8 instances of `time.Sleep` in test files (flaky CI risk)

**Required Work:**
- [ ] Audit test files for `time.Sleep` usage (8 known instances)
- [ ] Replace with channel-based synchronization, `sync.WaitGroup`, or condition variables where possible
- [ ] Use `testing.Short()` to skip slow tests in fast CI runs
- **Location:** Various `*_test.go` files

### P2.14 Error Injection Missing in Reconcile Mocks ❌

**Status:** NOT ADDRESSED  
**Impact:** Reconcile tests only cover happy paths; error resilience unverified

**Required Work:**
- [ ] Add mock configurations that return errors for AWS Describe* calls
- [ ] Test reconciler behavior when AWS API calls fail (partial results, timeouts, access denied)
- [ ] Verify reconciler logs warnings and continues rather than panicking
- **Location:** `internal/state/reconcile_test.go`

### P2.15 main.go Signal/HTTP Test Gaps ❌

**Status:** NOT ADDRESSED  
**Impact:** Core server functionality untested; signal handling, HTTP server, graceful shutdown, and cost monitoring flow have zero test coverage

**Evidence:**
- Signal handling completely untested — no test sends SIGINT/SIGTERM to verify graceful shutdown
- HTTP server completely untested — no test starts HTTP mode and sends requests
- Graceful shutdown sequence untested — no verification of proper service cleanup ordering
- Cost monitoring startup/shutdown flow untested

**Required Work:**
- [ ] Add signal handling tests (SIGINT, SIGTERM graceful shutdown)
- [ ] Add HTTP server mode tests (startup, request handling, shutdown)
- [ ] Add graceful shutdown sequence tests (verify cleanup order)
- [ ] Add cost monitoring integration tests
- **Location:** `cmd/agent-deploy/main_test.go`

---

## P3 — Quality Improvements (Lower Priority)

### P3.13 Shallow Reconciliation ❌

**Status:** PARTIAL — only 3 of 25 resource types reconciled  
**Impact:** Orphaned resources (subnets, NAT GW, SGs, etc.) may not be detected; SyncedCount is misleading (counts local entries, not AWS verification)

**Currently Reconciled (3):**
- VPC
- ECS Cluster
- ALB

**Missing (22):**
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
- ACM certificates
- Route53 records
- Auto-scaling policies

**Required Work:**
- [ ] Add reconciliation for all 25 resource types tracked in state
- [ ] Fix SyncedCount to verify against AWS instead of counting local entries
- **Location:** `internal/state/reconcile.go`

### ~~P3.9 Silent Error Suppression in Store~~ → **MOVED TO P0.2**

### ~~P3.11 Non-Atomic Infrastructure Updates~~ → **MOVED TO P0.1**

### ~~P3.10 Missing Error Types~~ ✅ RESOLVED

**Status:** COMPLETE — NOT REQUIRED  
**Resolution:** Code audit confirmed all 9 required error types are defined and properly wired. ErrCertificateInvalid and ErrInvalidInput are NOT actually required by spec.

### P3.15 Missing DNS State Constants ✅ COMPLETE

**Status:** Implemented as part of P1.29

**Implementation:**
- [x] Added `ResourceDomainName` constant
- [x] Added `ResourceHostedZoneID` constant
- [x] Added `ResourceCertAutoCreated` constant
- [x] Added `ResourceDNSRecordName` constant
- **Location:** `internal/state/types.go`

### ~~P3.12 Missing State Transitions~~ ✅

**Status:** COMPLETE  
**Impact:** ~~Limited state management flexibility; any→any transitions allowed~~

**Implementation:**
- Added `validateInfraTransition()` with proper state machine:
  - provisioning → ready (success) or failed (error)
  - failed → provisioning (retry) or destroyed (teardown)
  - ready → destroyed (teardown)
  - destroyed → terminal (no transitions)
- Added `validateDeploymentTransition()` with proper state machine:
  - deploying → running (success), failed (error), or stopped (teardown)
  - running → deploying (update), failed (error), or stopped (teardown)
  - failed → deploying (retry) or stopped (teardown)
  - stopped → terminal (no transitions)
- Returns `ErrInvalidState` for invalid transitions
- Idempotent: same-state transitions always succeed
- Added 8 comprehensive test functions covering all transitions
- **Location:** `internal/state/store.go`

### P3.14 Main.go Startup Error Handling ✅

**Status:** COMPLETE  
**Impact:** Partial startup could leave system in bad state

**Evidence:**
- `-enable-reconcile` flag IS properly wired up ✅
- Background services not cleaned up on startup failure
- Orphaned resources logged as warnings but not auto-teardown optioned

**Work Completed:**
- [x] Wire up `-enable-reconcile` flag properly ✅ (already implemented)
- [x] Clean up background services on startup failure
- [ ] Add optional `--auto-teardown-orphans` flag to remove orphaned resources (separate feature)
- **Location:** `cmd/agent-deploy/main.go`

**Implementation Details:**
- Added `defer shutdown()` to ensure cleanup runs on any exit path
- Added `sync.Once` to shutdown handler to make it idempotent
- Shutdown can now be called from: defer, signal handler, or HTTP server shutdown
- Background services (CleanupService, CostMonitor) are properly stopped on any exit

### ~~P3.16 Missing Input Validations~~ → **MOVED TO P1.31**

### P3.17 No Route 53 Client in awsclient ✅ COMPLETE

**Status:** Implemented as part of P1.29

**Implementation:**
- [x] Added Route53API interface to `internal/providers/aws.go`
- [x] Added Route53Mock for testing
- [x] Added `github.com/aws/aws-sdk-go-v2/service/route53` dependency to `go.mod`
- **Location:** `internal/providers/aws.go`, `go.mod`

### P3.18 Silent Error Suppression in config.go ✅

**Status:** FIXED  
**Impact:** Config file load errors silently ignored; users unaware of malformed config

**Evidence:**
- `config.go:36` — Config file load errors silently ignored with `_ = err`
- Comment says "Config file is optional; log but don't fail" but NO LOGGING

**Work Completed:**
- [x] Added slog import and logging package import to config.go
- [x] Changed `_ = err` to proper warning log when config file exists but fails to parse
- [x] Only logs if error is NOT "file not found" (missing file is expected behavior)
- [x] Uses logging.ComponentSpending for consistent component tagging
- **Location:** `internal/spending/config.go`

### P3.19 Hardcoded ALB/NAT/CloudWatch Pricing ❌

**Status:** NOT ADDRESSED  
**Impact:** Cost estimation inaccurate in some regions; pricing could become stale

**Evidence (from pricing.go):**
- ALB: Uses hardcoded $0.0225/hr fallback when Pricing API unavailable
- NAT Gateway: Hardcoded $0.045/hr + $0.045/GB
- CloudWatch Logs: Hardcoded $0.50/GB ingestion, $0.03/GB storage
- LCU: Hardcoded $0.008/hr

**Required Work:**
- [ ] Add Pricing API calls for ALB, NAT Gateway, CloudWatch Logs
- [ ] Fall back to hardcoded values only when API fails
- [ ] Log warning when using hardcoded fallback
- **Location:** `internal/spending/pricing.go`

### P3.20 NAT Gateway Single AZ ⚠️ NEW

**Status:** NOT ADDRESSED  
**Impact:** Single point of failure for private subnet traffic; reduced availability

**Evidence:**
- NAT Gateway only created in first public subnet (`internal/providers/aws.go:1624`)
- Private subnets in other AZs route through single NAT Gateway
- If NAT Gateway or its AZ becomes unavailable, all private subnet egress traffic fails
- Standard best practice is one NAT Gateway per AZ

**Required Work:**
- [ ] Create NAT Gateway in each availability zone's public subnet
- [ ] Create separate route tables for each AZ's private subnets
- [ ] Route each private subnet to its AZ's NAT Gateway
- [ ] Update teardown to delete multiple NAT Gateways and EIPs
- [ ] Document cost impact (additional NAT Gateway hourly charges)
- **Location:** `internal/providers/aws.go:1624`

### P3.31 deleteDNSResources() Error Not Checked ❌

**Status:** NOT ADDRESSED  
**Impact:** DNS cleanup errors silently swallowed during teardown; orphaned DNS records and certificates may persist

**Evidence:**
- `deleteDNSResources()` is called in `teardown()` (~line 1510) without checking the returned error
- If DNS deletion fails, teardown reports success but leaves orphaned Route 53 records and ACM certificates
- These orphaned resources could cause conflicts on re-deployment or accumulate costs

**Required Work:**
- [ ] Check error return from `deleteDNSResources()` in `teardown()`
- [ ] Log warning and continue teardown (don't block on DNS errors, but don't ignore them)
- [ ] Add test: teardown with DNS deletion error still completes but logs warning
- **Location:** `internal/providers/aws.go` (teardown function)

### P3.32 Reconcile Error Handling Issues ❌

**Status:** NOT ADDRESSED  
**Impact:** Silent failures in reconciliation could cause missed orphaned resources or incorrect sync counts

**Evidence:**
- Errors silently ignored at lines 143-155, 272, 293, 301, 309 in `internal/state/reconcile.go`
- `checkInfraResources()`: Errors from AWS Describe* calls are logged but silently ignored; function continues with stale/incomplete data
- `findOrphanedResources()`: Error aggregation incomplete; some error paths return early, others continue
- `countSyncedResources()`: Silent failures; errors don't propagate to caller

**Required Work:**
- [ ] Properly propagate or aggregate errors in `checkInfraResources()`
- [ ] Complete error aggregation in `findOrphanedResources()`
- [ ] Return meaningful error information from `countSyncedResources()`
- [ ] Add tests for error handling paths in reconciliation
- **Location:** `internal/state/reconcile.go` — lines 143-155, 272, 293, 301, 309

### P3.27-P3.30 CI/CD Gaps ❌

**Status:** NOT IMPLEMENTED  
**Impact:** Missing security hardening and release validation

**Gaps Identified:**
| ID | Gap | Required Work |
|----|-----|---------------|
| **P3.27** | Missing security scanning | Add `gosec` linter to CI workflow |
| **P3.28** | Missing dependency audit | Add `go mod verify` to CI |
| **P3.29** | No SBOM generation | Add Software Bill of Materials for releases |
| **P3.30** | No goreleaser validation | Add `goreleaser --validate` pre-release check |

**Location:** `.github/workflows/ci.yml`, `.github/workflows/release.yml`

---

### ✅ Completed P3 Items (moved to collapsed section)

<details>
<summary>Click to expand completed P3 items</summary>

### ~~P3.21 Cleanup Service Race Condition~~ ✅ COMPLETE

**Status:** COMPLETE  
**Impact:** Could panic on concurrent Stop() calls; minor race condition

**Evidence:**
- `internal/state/cleanup.go:78-96` — Lock released between checking `running` flag and closing `stopCh`
- Concurrent Stop() calls could both pass the `running` check before either closes channel
- Second close on already-closed channel would panic

**Fix Applied:**
- [x] Added `sync.Once` field (`stopOnce`) to CleanupService struct to ensure Stop() is safe to call concurrently
- [x] Modified Stop() to use `stopOnce.Do()` when closing the stopCh channel, preventing panic from concurrent close
- [x] Modified Start() to reset `stopOnce` for new run cycles
- [x] Added TestCleanupService_ConcurrentStop test to verify the fix
- **Location:** `internal/state/cleanup.go`

### ~~P3.22 Deployment Status Update Failures Silently Ignored~~ ✅ VERIFIED

**Status:** VERIFIED (behavior correct)  
**Impact:** ~~Deployment status could become stale if status update fails; debugging harder~~

**Analysis:**
- The current behavior is correct: when a primary operation fails, we try to update status to "failed"
- If the status update itself fails, we log it with `slog.Error` and return the primary error
- This is the right pattern: we don't want to mask the primary error with a secondary status update failure
- The logging is already proper with structured fields (deployment ID, error details)
- No changes needed — the implementation follows best practices for error handling

**Location:** `internal/providers/aws.go:1163-1212`

### ~~P3.23 Certificate ARN Storage Failures Silently Ignored~~ ✅ FIXED

**Status:** COMPLETE  
**Impact:** ~~Certificate state could be lost; teardown may not find certificate to delete~~

**Fix Applied:**
1. If certificate ARN storage fails, we now attempt to delete the certificate we just created (rollback)
2. After rollback attempt, the operation returns an error to the caller
3. This prevents orphaned certificates that would accumulate AWS costs
4. The auto-created flag failure is still logged but not fatal (non-critical metadata)

**Location:** `internal/providers/aws.go:3633-3641`

### ~~P3.24 No Exponential Backoff in Certificate Validation~~ ✅ FIXED

**Status:** COMPLETE  
**Impact:** ~~Could overload API during high latency periods; inefficient polling~~

**Fix Applied:**
1. Added `backoffWithJitter()` helper function that implements exponential backoff with ±25% jitter
2. Updated certificate validation options polling (Step 2) to use exponential backoff: 1s, 2s, 4s, ... up to 15s max
3. Updated certificate issuance polling (Step 4) to use exponential backoff: 5s, 10s, 20s, ... up to 30s max
4. Added `TestBackoffWithJitter` test verifying exponential growth, max delay cap, jitter variance, and minimum delay
5. Added `math/rand` import for jitter calculation

**Benefits:**
- Reduces API load during certificate validation
- Prevents thundering herd when multiple certificates being validated
- More resilient during high latency periods

**Location:** `internal/providers/aws.go`

### ~~P3.25 isLocalImage() Validation Incomplete~~ ✅ VERIFIED

**Status:** ✅ VERIFIED (implementation adequate)  
**Impact:** Implementation is comprehensive with 27 test cases passing

**Analysis:**
- Handles ECR URIs, 8 major public registries, custom registries via '.' or ':' detection
- Handles localhost:port, IP:port patterns
- Implementation is adequate for real-world use cases
- **Location:** `internal/providers/aws.go` (isLocalImage function)

**No work required** — implementation covers all practical scenarios.

</details>

---

## P4 — New Features (Unimplemented Specs)

> **Note:** Static Site and Background Worker workloads have been promoted to P1.37 and P1.38 respectively, as `ralph/specs/workload-roadmap.md` explicitly lists them as Priority P1. Lightsail provider (P1.34) has been implemented.

### P4.4 Scheduled Job Workload ❌

**Status:** NOT IMPLEMENTED — Spec exists (spec priority: P2)  
**Spec:** `ralph/specs/workloads/scheduled-job.md`

**Required Work:**
- [ ] Add scheduled_job workload type
- [ ] Integrate EventBridge Scheduler
- [ ] Support cron expressions
- **Location:** `internal/providers/aws.go`

### P4.5 Batch Processing Workload ❌

**Status:** NOT IMPLEMENTED — Spec exists (spec priority: P2)  
**Spec:** `ralph/specs/workloads/batch-processing.md`

**Required Work:**
- [ ] Add batch_processing workload type
- [ ] Integrate AWS Batch
- [ ] Support job queues and compute environments
- **Location:** `internal/providers/aws_batch.go` (new file)

### P4.6 ML Inference Workload ❌

**Status:** NOT IMPLEMENTED — Spec exists (spec priority: P3)  
**Spec:** `ralph/specs/workloads/ml-inference.md`

**Required Work:**
- [ ] Add ml_inference workload type
- [ ] Support GPU instance types
- [ ] Integrate SageMaker or GPU-enabled Fargate
- **Location:** `internal/providers/aws.go`

### P4.7 Data Pipeline Workload ❌

**Status:** NOT IMPLEMENTED — Spec exists (spec priority: P3)  
**Spec:** `ralph/specs/workloads/data-pipeline.md`

**Required Work:**
- [ ] Add data_pipeline workload type
- [ ] Integrate Step Functions
- [ ] Support complex workflow orchestration
- **Location:** `internal/providers/aws_stepfunctions.go` (new file)

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

### P5.4 CI Enhancements (Minor Gaps)

**Status:** NOT CRITICAL — CI is complete and working  
**Impact:** Minor hardening improvements

**Current state:**
- ✅ CI workflow with lint, test, build jobs working
- ✅ Coverage thresholds enforced (25% floor, 50% target)
- ✅ Release workflow configured correctly

**Minor gaps (nice-to-have):**
- [ ] Add Go version validation in CI (ensure go.mod version matches CI matrix)
- [ ] Add goreleaser check in CI (validate .goreleaser.yml syntax)
- [ ] Add security scanning (e.g., govulncheck, trivy)
- **Location:** `.github/workflows/ci.yml`

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
| `cmd/agent-deploy/` | **0.0%** | Entry point completely untested |
| `internal/awsclient/` | **91.7%** | Comprehensive tests added |
| `internal/id/` | **100.0%** | Fully tested |
| `internal/logging/` | **86.0%** | Good coverage |
| `internal/providers/` | **48.8%** | planInfra, deploy, teardown, status, approval workflows, provisionVPC, provisionECSCluster, provisionALB tested |
| `internal/spending/` | **67.9%** | CostTracker, CostMonitor, PricingEstimator tests added |
| `internal/state/` | **82.9%** | Reconciler tests added, comprehensive coverage |
| **Overall** | **52.9%** | ✅ **MEETS TARGET** (target: 50%) |

### Key Files

| File | Purpose |
|------|---------|
| `cmd/agent-deploy/main.go` | MCP server entry point |
| `internal/providers/provider.go` | Provider interface + registration |
| `internal/providers/aws.go` | AWS provider (6 tools, 1 resource, 1 prompt) + all input validations |
| `internal/state/store.go` | File-backed state storage |
| `internal/state/types.go` | Plan, Infrastructure, Deployment structs + 18 ResourceType constants |
| `internal/state/reconcile.go` | State reconciliation with AWS (3/25 resources; error handling issues P3.32) |
| `internal/id/id.go` | ULID-based ID generation |
| `internal/awsclient/` | AWS SDK configuration (9 service interfaces: EC2, ECS, ELBV2, IAM, ECR, CloudWatch Logs, AutoScaling, ACM, Route 53) |
| `internal/spending/config.go` | Spending limits configuration |
| `internal/spending/check.go` | Pre-provisioning budget check |
| `internal/spending/costs.go` | AWS Cost Explorer integration |
| `internal/spending/monitor.go` | Runtime cost monitoring with alerts |
| `internal/spending/pricing.go` | AWS Pricing API for Fargate; hardcoded fallback for ALB/NAT/CloudWatch |
| `internal/state/cleanup.go` | Expired plan cleanup service |
| `internal/errors/errors.go` | Domain error types (all 9 required types defined and wired) |
| `internal/logging/logging.go` | Structured logging with slog |
| `internal/main_test.go` | MCP server integration tests |
| `ralph/specs/aws-provider.md` | Tool/resource/prompt specifications |
| `ralph/specs/deployment-state.md` | State model and storage spec |
| `ralph/specs/spending-safeguards.md` | Budget enforcement spec |
| `ralph/specs/custom-dns.md` | Route 53 / custom domain spec |
| `ralph/specs/distribution.md` | Distribution / GoReleaser spec |
| `ralph/specs/ci.md` | CI/CD requirements spec |
| `ralph/specs/lightsail-provider.md` | Lightsail provider spec (IMPLEMENTED — P1.34) |
| `ralph/specs/workload-roadmap.md` | **Workload types roadmap (2 P1 + 4 P2-P3 workloads pending)** |
| `ralph/specs/workloads/static-site.md` | **Static site spec (NOT IMPLEMENTED — P1.37, priority: P1)** |
| `ralph/specs/workloads/background-worker.md` | **Background worker spec (NOT IMPLEMENTED — P1.38, priority: P1)** |
| `ralph/specs/workloads/` | **6 workload specs total (static-site, background-worker, scheduled-job, batch, ml-inference, data-pipeline)** |

### Hardcoded Values Summary

| Value | Location | Status |
|-------|----------|--------|
| VPC CIDR: `10.0.0.0/16` | `aws.go` | ✅ IMPLEMENTED — Configurable via vpc_cidr parameter (P1.9) |
| Subnet CIDRs | `aws.go` | ✅ IMPLEMENTED — Dynamically calculated via CalculateSubnetLayout() (P1.9) |
| Fargate pricing | `pricing.go` | ✅ **IMPLEMENTED** — parsePricingResponse() extracts prices from AWS Pricing API |
| ALB pricing | `pricing.go:372-377` | ⚠️ HARDCODED FALLBACK — P3.19 |
| NAT Gateway pricing | `pricing.go:372-377` | ⚠️ HARDCODED FALLBACK — P3.19 |
| CloudWatch Logs pricing | `pricing.go:372-377` | ⚠️ HARDCODED FALLBACK — P3.19 |
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
| InfraID/PlanID/DeploymentID format | `aws.go` | ✅ VALIDATED (P1.31) |
| ImageRef format (beyond empty check) | `aws.go` | ✅ VALIDATED (P1.31) |
| AppDescription max length | `aws.go` | ✅ VALIDATED (P1.31) |
| ExpectedUsers/LatencyMS range | `aws.go` | ✅ VALIDATED (P1.31) |
| CertificateARN region match | `aws.go` | ✅ VALIDATED (P1.31) |

### Remaining Work by Priority

| Priority | Count | Items |
|----------|-------|-------|
| **P0 Critical** | 1 | **P0.3 (provider nil store checks — 11 unguarded accesses)** |
| **P1 Spec Gaps** | 3 | **P1.37 (Static Site)**, **P1.38 (Background Worker)**, ~~P1.34 (Lightsail — COMPLETE)~~, ~~P1.35 (DNS status URL — COMPLETE)~~, ~~P1.36 (spending confirmation — COMPLETE)~~ |
| **P2 Test Gaps** | 6 | P2.5 (AWS error scenarios), P2.11 (processAlert), P2.12 (checkInfraResources), P2.13 (sleep timing), P2.14 (reconcile mocks), P2.15 (signal/HTTP) |
| **P3 Quality** | 9 | P3.13 (reconciliation 3/25 types), P3.19 (pricing), P3.20 (NAT HA), P3.31 (DNS error handling), P3.32 (reconcile error handling), P3.27-P3.30 (CI gaps) |
| **P4 New Features** | 4 | P4.4-P4.7 (scheduled job, batch, ML inference, data pipeline) |
| **P5 Stretch** | 3 | CloudFormation, multi-cloud, secrets |
| **Total remaining** | **28** | |

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
| **custom-dns.md** | Route 53 hosted zone lookup | ✅ IMPLEMENTED — P1.29 |
| **custom-dns.md** | ACM certificate auto-provisioning | ✅ IMPLEMENTED — P1.29 |
| **custom-dns.md** | DNS alias A record creation | ✅ IMPLEMENTED — P1.29 |
| **custom-dns.md** | Custom domain in status URL list | ⚠️ **MISSING** — P1.35 (statusOutput lacks custom_domain field) |
| **custom-dns.md** | DNS resource tracking in state | ✅ IMPLEMENTED — P3.15 |
| **distribution.md** | Move main.go to cmd/agent-deploy/ | ✅ IMPLEMENTED |
| **distribution.md** | GoReleaser + release workflow | ✅ IMPLEMENTED |
| **distribution.md** | `go install` support | ✅ IMPLEMENTED |
| **deployment-state.md** | Plan, Infrastructure, Deployment types | ✅ Implemented |
| **deployment-state.md** | File-backed JSON at ~/.agent-deploy/state/ | ✅ Implemented |
| **deployment-state.md** | 24-hour plan expiration, hourly cleanup | ✅ Implemented |
| **deployment-state.md** | AWS resource tag reconciliation | ⚠️ **PARTIAL** — only 3 of 19 resource types reconciled (P3.13) |
| **spending-safeguards.md** | monthly_budget_usd, per_deployment_usd, alert_threshold_percent | ✅ Implemented |
| **spending-safeguards.md** | Pre-provisioning budget check | ⚠️ PARTIAL — Cost Explorer works, but ALB/NAT/CW pricing uses hardcoded fallback |
| **spending-safeguards.md** | Confirmation when no limits configured | ⚠️ **MISSING** — P1.36 (silently applies defaults without user confirmation) |
| **spending-safeguards.md** | Runtime cost monitoring with Cost Explorer | ✅ Implemented |
| **spending-safeguards.md** | Auto-teardown when budget exceeded | ✅ IMPLEMENTED |
| **spending-safeguards.md** | Per-request spending limit overrides | ✅ IMPLEMENTED |
| **spending-safeguards.md** | Resource tagging | ✅ Implemented |
| **auto-scaling.md** | Auto-scaling with target tracking | ✅ IMPLEMENTED — CPU/memory policies, cooldowns, cleanup |
| **auto-scaling.md** | Cost range in planInfra output (min/max) | ✅ IMPLEMENTED |
| **networking.md** | VPC CIDR configurable | ✅ IMPLEMENTED — vpc_cidr parameter with validation (P1.9) |
| **networking.md** | Private subnets with NAT Gateway | ✅ IMPLEMENTED |
| **ci.md** | CI workflow with lint, test, build jobs | ✅ IMPLEMENTED |
| **testing.md** | 50% code coverage | ✅ **TARGET MET** — 52.9% overall |
| **testing.md** | main.go test coverage | ⚠️ **PARTIAL** — components tested, main() itself hard to test (P2.9); signal/HTTP untested (P2.15) |
| **testing.md** | Concurrent access testing | ✅ **COMPLETE** — P2.10 fixed with comprehensive concurrent tests |
| **error-handling.md** | Domain error types | ✅ **COMPLETE** — all 9 required error types defined and wired |
| **operational.md** | No silent error suppression | ✅ FIXED — P0.2 complete, store.go now logs errors |
| **operational.md** | Pagination for list operations | ✅ IMPLEMENTED |
| **lightsail-provider.md** | Full Lightsail provider (priority: P1) | ✅ **IMPLEMENTED** — P1.34 (auto-backend selection, cost comparison) |
| **workload-roadmap.md** | 6 workload types | ❌ **PARTIAL** — only web service exists; 2 P1 workloads + 4 P2-P3 workloads pending |
| **workloads/static-site.md** | S3+CloudFront static sites (priority: P1) | ❌ **NOT IMPLEMENTED** — P1.37 |
| **workloads/background-worker.md** | ECS without ALB (priority: P1) | ❌ **NOT IMPLEMENTED** — P1.38 |
| **workloads/scheduled-job.md** | EventBridge scheduled tasks (priority: P2) | ❌ NOT IMPLEMENTED — P4.4 |
| **workloads/batch-processing.md** | AWS Batch integration (priority: P2) | ❌ NOT IMPLEMENTED — P4.5 |
| **workloads/ml-inference.md** | GPU-enabled inference (priority: P3) | ❌ NOT IMPLEMENTED — P4.6 |
| **workloads/data-pipeline.md** | Step Functions workflows (priority: P3) | ❌ NOT IMPLEMENTED — P4.7 |
