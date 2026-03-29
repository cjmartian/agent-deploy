# Implementation Plan

**Project Goal:** Natural language deployment of applications via MCP server → Cloud provider. Allow users to end-to-end create applications and make them publicly available while ensuring spend does not cross user-defined boundaries.

**Last Updated:** 2026-03-29  
**Last Audit:** 2026-03-29 (Comprehensive codebase audit — spec gap analysis)

---

## 🚨 Priority Summary (Post-Audit)

### CRITICAL — Must Fix First
| ID | Issue | Impact |
|----|-------|--------|
| *(None — all P0 issues resolved)* | | |

### HIGH PRIORITY — Spec Compliance
| ID | Issue | Impact |
|----|-------|--------|
| **P1.9** | VPC CIDR hardcoded to 10.0.0.0/16 | Cannot customize VPC for peering scenarios |
| **P1.21** | Per-request spending override MISSING | Users cannot set deployment-specific budget caps |
| **P1.22** | Auto-scaling cost range MISSING | Cannot predict worst-case costs when auto-scaling configured |
| **P1.28** | Container-level health check MISSING | Unhealthy containers may not be replaced by ECS |
| **P1.29** | Custom DNS / Route 53 MISSING (entire spec) | No custom domain support, no ACM auto-provisioning |
| **P1.30** | Distribution / cmd structure MISSING (entire spec) | Cannot `go install`, no GoReleaser, not distributable |

### MEDIUM PRIORITY — Test Gaps
| ID | Issue | Impact |
|----|-------|--------|
| **P2.5** | AWS provider error scenarios incomplete | Error paths not fully covered |
| **P2.9** | main.go 0% coverage | Entry point completely untested; flag/signal handling unverified |
| **P2.10** | Concurrent access UNTESTED | Store has RWMutex but locking never verified |

### LOWER PRIORITY — Quality
| ID | Issue | Impact |
|----|-------|--------|
| **P3.9** | Silent error suppression (store.go:86,123,220) | Data loss/corruption could go undetected |
| **P3.10** | Missing error types (ErrCertificateInvalid, ErrInvalidInput) | Inconsistent error handling |
| **P3.11** | Non-atomic infrastructure updates | Potential data corruption under concurrent access |
| **P3.12** | Missing state transitions | Limited state management flexibility |
| **P3.13** | Shallow reconciliation (3/19 resource types) | Orphaned resources may not be detected |
| **P3.14** | Main.go startup error handling | Partial startup could leave system in bad state |
| **P3.15** | Missing state/types.go DNS resource constants | Required for custom-dns.md implementation (P1.29) |

---

## ✅ Completed

| Component | Status | Location | Audit Notes (2026-03-29) |
|-----------|--------|----------|-------------|
| MCP server (stdio + HTTP) | ✅ Working | `internal/main.go` | Verified |
| Provider interface | ✅ Defined | `internal/providers/provider.go` | Verified |
| AWS 6 tools | ✅ Implemented | `internal/providers/aws.go` | aws_plan_infra, aws_approve_plan, aws_create_infra, aws_deploy, aws_status, aws_teardown |
| AWS `aws:deployments` resource | ✅ Implemented | `internal/providers/aws.go` | Verified |
| AWS `aws_deploy_plan` prompt | ✅ Implemented | `internal/providers/aws.go` | Verified |
| Tool input/output types | ✅ Defined | `internal/providers/aws.go` | Verified |
| Specifications | ✅ Written | `ralph/specs/` | 16 specs present (4 original + 12 added) |
| Makefile syntax | ✅ Fixed | `Makefile` | tabs, build path, test flags |
| AWS SDK dependency | ✅ Added | `go.mod` | ec2, ecs, ecr, elbv2, cloudwatchlogs, costexplorer |
| ULID dependency | ✅ Added | `go.mod` | github.com/oklog/ulid/v2 |
| Docker SDK dependency | ✅ Added | `go.mod` | github.com/docker/docker v27.5.1+incompatible |
| ID generation | ✅ Done | `internal/id/id.go` | New(), NewPlan(), NewInfra(), NewDeploy() |
| **State model types** | ✅ **100% compliant** | `internal/state/types.go` | Plan, Infrastructure, Deployment + constants |
| **State storage** | ✅ **100% compliant** | `internal/state/store.go` | Full Store implementation, exceeds spec |
| **State persistence** | ✅ **100% compliant** | `internal/state/store.go` | All state types, transitions implemented |
| AWS client config | ✅ Done | `internal/awsclient/client.go` | LoadConfig(), ResourceTags() |
| Wire Store into AWSProvider | ✅ Done | `internal/providers/aws.go` | store field, NewAWSProvider constructor |
| Auto-teardown wiring | ✅ Done | `internal/main.go`, `internal/providers/` | TeardownCallback now calls actual AWS teardown |
| **Spending limit configuration** | ✅ **100% compliant** | `internal/spending/config.go` | Limits, LoadLimits(), env var support |
| **Cost tracking with Cost Explorer** | ✅ **100% compliant** | `internal/spending/costs.go` | Full implementation |
| **Runtime cost monitoring** | ✅ **100% compliant** | `internal/spending/monitor.go` | Background checking, alerts |
| **Alert thresholds and notifications** | ✅ **100% compliant** | `internal/spending/monitor.go` | Verified |
| **Resource tagging** | ✅ **100% compliant** | `internal/awsclient/client.go` | ResourceTags() |
| Pre-provisioning budget check | ✅ **100% compliant** | `internal/spending/check.go` | CheckBudget(), CheckResult — uses AWS Pricing API (P1.1) and Cost Explorer (P1.2) |
| createInfra | ✅ Done | `internal/providers/aws.go` | VPC, subnets, IGW, route tables, SGs, ECS, ALB, CloudWatch |
| deploy | ✅ Done | `internal/providers/aws.go` | ECR repo, task def, ECS service, ALB URLs |
| status | ✅ Done | `internal/providers/aws.go` | ECS service status, ALB URLs |
| teardown | ✅ Done | `internal/providers/aws.go` | Reverse order deletion, ECR cleanup |
| Error handling patterns | ✅ Done | `internal/errors/errors.go` | Domain errors (all error types now in use) |
| ID generation tests | ✅ Done | `internal/id/id_test.go` | Verified |
| State storage tests | ✅ Done | `internal/state/store_test.go` | 45.5% coverage |
| Spending check tests | ✅ Done | `internal/spending/check_test.go` | Verified |
| Cost Explorer tests | ✅ Done | `internal/spending/costs_test.go` | Comprehensive |
| Runtime cost monitoring tests | ✅ Done | `internal/spending/monitor_test.go` | Comprehensive |
| MCP server integration test | ✅ Done | `internal/main_test.go` | 11 tests (does not test main()) |
| Graceful shutdown | ✅ Done | `internal/main.go` | In-flight requests complete, defers run |
| Expired plan cleanup (24-hour expiration, hourly cleanup) | ✅ Done | `internal/state/cleanup.go` | Full cleanup service |
| Expired plan cleanup tests | ✅ Done | `internal/state/cleanup_test.go` | Comprehensive |
| State reconciliation | ✅ Done | `internal/state/reconcile.go` | Full pagination support (P3.1, P3.2 completed) |
| Reconciliation tests | ✅ Done | `internal/state/reconcile_test.go` | Mock-based only |
| Structured logging infrastructure | ✅ Done | `internal/logging/logging.go` | Full slog infrastructure |
| Structured logging tests | ✅ Done | `internal/logging/logging_test.go` | Comprehensive |
| AllWithStore provider init | ✅ Done | `internal/providers/provider.go` | Shared store instances |
| Background services integration | ✅ Done | `internal/main.go` | CleanupService, CostMonitor, signal handling |
| Integration tests | ✅ Done | `internal/providers/aws_integration_test.go` | Full workflow tests |
| CI/CD workflows | ✅ Done | `.github/workflows/ci.yml`, `.golangci.yml` | lint, test, build on push/PR |
| CI coverage threshold | ✅ Done | `.github/workflows/ci.yml` | Enforces 25% floor (target 50% per ralph/specs/testing.md); P2 tests in progress |
| IAM task execution role | ✅ Done | `internal/providers/aws.go`, `internal/state/types.go` | ECS tasks can now pull from ECR and write to CloudWatch |
| Configurable container port | ✅ Done | `internal/providers/aws.go` | ContainerPort parameter (default: 80) |
| Configurable health check path | ✅ Done | `internal/providers/aws.go` | HealthCheckPath parameter (default: /) |
| Configurable desired count | ✅ Done | `internal/providers/aws.go` | DesiredCount parameter (default: 1) |
| Environment variables support | ✅ Done | `internal/providers/aws.go` | Environment map parameter |
| Structured logging migration | ✅ Done | `internal/providers/`, `internal/spending/costs.go` | All log.Printf migrated to slog |
| Wait for healthy deployment | ✅ Done | `internal/providers/aws.go` | waitForHealthyDeployment polls ECS/ALB |
| **P0.1 Plan Approval** | ✅ Done | `internal/providers/aws.go`, `internal/state/store.go` | `aws_approve_plan` tool, explicit approval workflow |
| Plan approval tests | ✅ Done | `internal/state/store_test.go`, `internal/providers/aws_test.go` | Comprehensive approval workflow tests |
| **P1.12 Auto Scaling** | ✅ Done | `internal/providers/aws.go`, `internal/providers/aws_test.go` | MinCount, MaxCount, CPU/memory target tracking policies |
| **P0.6 ECR Image Push** | ✅ Done | `internal/providers/aws.go`, `internal/awsclient/interfaces.go`, `internal/awsclient/mocks/other.go` | `isLocalImage()` detection, `pushImageToECR()` with Docker SDK, ECR auth token handling, comprehensive tests |
| **P2.12 Spending Config Tests** | ✅ Done | `internal/spending/check_test.go`, `internal/spending/config_test.go` | Fixed tests to use isolated HOME directory with t.TempDir(); tests no longer pick up real config files |

---

## Current State Summary

| Component | Status | Notes |
|-----------|--------|-------|
| **State storage** | ✅ **100% compliant** | Exceeds spec with additional operations |
| **Spending safeguards** | ✅ **Working** | Config, Cost Explorer, monitoring, alerts, tagging |
| **Cleanup service** | ✅ **Working** | `internal/state/cleanup.go` — 24-hour plan expiration with hourly cleanup |
| **Cost monitoring** | ✅ **Working** | `internal/spending/monitor.go` |
| **State reconciliation** | ⚠️ **PARTIAL** | Pagination implemented for VPC/ECS/ALB, batch tag fetching works, but only reconciles 3 of 19 resource types (P3.13) |
| AWS 6 tools | ✅ **Complete** | All features work including ECR image push (P0.6 completed) |
| AWS `aws:deployments` resource | ✅ **Implemented** | `internal/providers/aws.go` |
| AWS `aws_deploy_plan` prompt | ✅ **Implemented** | `internal/providers/aws.go` |
| Cost estimation (planInfra) | ✅ **Working** | AWS Pricing API parsing implemented — `parsePricingResponse()` extracts prices from API response (P1.1 completed) |
| Current spend calculation | ✅ **IMPLEMENTED** | CostTracker.GetTotalMonthlySpend() from Cost Explorer with fallback |
| Auto-teardown | ✅ **Working** | TeardownCallback wired to AWS provider's teardown method |
| CI/CD workflows | ✅ **Working** | `.github/workflows/ci.yml`, `.golangci.yml` |
| golangci-lint config | ✅ **Working** | `.golangci.yml` with version 2 format |
| IAM role provisioning | ✅ **Done** | `provisionExecutionRole()`, `deleteExecutionRole()`, `ResourceExecutionRole` constant |
| go.mod dependencies | ✅ **Complete** | All AWS SDK dependencies including `github.com/aws/aws-sdk-go-v2/service/pricing`, Docker SDK (`github.com/docker/docker v27.5.1+incompatible`) |
| Auto Scaling | ✅ **IMPLEMENTED** | MinCount, MaxCount, CPU/memory target tracking, cooldowns, cleanup |
| TLS/HTTPS | ✅ **IMPLEMENTED** | ACM certificate validation, HTTPS listener, HTTP-to-HTTPS redirect, TLS 1.2+ policy |
| Private subnets | ✅ **IMPLEMENTED** | Public/private subnet architecture with NAT Gateway |
| Plan approval | ✅ **IMPLEMENTED** | `aws_approve_plan` tool with explicit approval workflow |
| Wait for healthy deployment | ✅ **Done** | waitForHealthyDeployment polls ECS + ALB health checks |
| Test coverage | ✅ **TARGET MET** | Overall 51.0% (target 50%); `spending/` at 67.4%; `state/` at 82.0%; `awsclient/` at 91.7% |
| Structured logging | ✅ **Done** | All log.Printf migrated to slog (30 in aws.go, 1 in provider.go, ~4 in costs.go) |
| AWS SDK Mocking Infrastructure | ✅ **Complete** | Mock interfaces (EC2API, ECSAPI, ELBV2API, IAMAPI, ECRAPI, CloudWatchLogsAPI, AutoScalingAPI, ACMAPI), AWSClients struct, compile-time verification |
| Makefile | ✅ **Complete** | all, test-race, coverage, coverage-html, run, install, help targets added |
| **Input validation** | ✅ **Complete** | ValidateFargateResources, ValidateLogRetention, ValidateContainerPort, ValidateEnvironmentVariables, ValidateHealthCheckPath, ValidateAWSRegion, ValidateDesiredCount implemented in `internal/providers/validation.go` |
| **Rollback on failure** | ✅ **Done** | rollbackInfra() cleans up partially created resources on provisioning failure |
| **Error types** | ⚠️ **INCOMPLETE** | Missing ErrCertificateInvalid, ErrInvalidInput (P3.10) |
| **Custom DNS / Route 53** | ❌ **NOT STARTED** | Entire spec `ralph/specs/custom-dns.md` unimplemented (P1.29) |
| **Distribution / cmd structure** | ❌ **NOT STARTED** | Entire spec `ralph/specs/distribution.md` unimplemented (P1.30) |

---

## P0 — Critical Issues (Security/Cost Risks, Broken Functionality)

### P0.6 ECR Image Push ✅ COMPLETED

- [x] ✅ Added `github.com/docker/docker` SDK dependency (v27.5.1+incompatible)
- [x] ✅ Implemented `isLocalImage(imageRef string) bool` function to detect local vs registry images
- [x] ✅ Implemented `pushImageToECR()` function that:
  - Gets ECR authorization token via AWS SDK
  - Decodes token to username:password
  - Tags local image with full ECR URI
  - Pushes image using Docker client
  - Returns full ECR URI for task definition
- [x] ✅ Updated deploy flow to call `pushImageToECR` between `ensureECRRepository` and `createTaskDefinition`
- [x] ✅ Added comprehensive tests for `isLocalImage()` in aws_test.go
- [x] ✅ Added `GetAuthorizationToken` to ECRAPI interface
- [x] ✅ Added mock implementation for GetAuthorizationToken
- **Status:** COMPLETED
- **Spec:** ralph/specs/ecr-image-push.md
- **Location:** `internal/providers/aws.go`, `internal/awsclient/interfaces.go`, `internal/awsclient/mocks/other.go`
- **Completed:** 2026-03-28

### P0.1 CI/CD Workflows ✅ COMPLETED

- [x] Created `.github/workflows/ci.yml` with lint, test, build jobs on push/PR
- [x] Added golangci-lint configuration (`.golangci.yml`) with version 2 format
- [x] All linter issues in the codebase have been fixed (was 63 issues, now 0)
- **Location:** `.github/workflows/ci.yml`, `.golangci.yml`
- **Completed:** 2026-03-20

### P0.2 Auto-Teardown Not Wired ✅ COMPLETED

- [x] Wired `TeardownCallback` to actually call AWS provider's teardown method
- [x] Added `Teardown(ctx, deploymentID)` public method to AWSProvider
- [x] Added `TeardownProvider` interface in providers package
- [x] Added `GetAWSProvider(store)` helper function
- [x] Added tests for the new functionality (TestPublicTeardown, TestTeardownProvider_Interface, TestGetAWSProvider)
- **Location:** `internal/main.go`, `internal/providers/aws.go`, `internal/providers/provider.go`
- **Completed:** 2026-03-20

### P0.3 IAM Task Execution Role Missing ✅ COMPLETED

- [x] ✅ Added `github.com/aws/aws-sdk-go-v2/service/iam` to `go.mod`
- [x] ✅ Created `provisionExecutionRole()` to create IAM role with ECS task assume-role policy
- [x] ✅ Attaches `AmazonECSTaskExecutionRolePolicy` for ECR pull and CloudWatch logs permissions
- [x] ✅ Pass `ExecutionRoleArn` to `RegisterTaskDefinition` (was nil, now properly set)
- [x] ✅ Added `deleteExecutionRole()` for cleanup during teardown
- [x] ✅ Added `ResourceExecutionRole` constant to state package
- **Location:** `internal/providers/aws.go`, `internal/state/types.go`, `go.mod`
- **Completed:** 2026-03-20

### P0.4 Graceful Shutdown Issues ✅ COMPLETED

- [x] ✅ HTTP mode now uses `Shutdown()` instead of `Close()` for graceful shutdown with 30s timeout
- [x] ✅ Removed `os.Exit(0)` from stdio shutdown path - now returns naturally to allow defers to run
- [x] ✅ Replaced `os.Exit(1)` with `return` to allow cleanup on errors
- **Location:** `internal/main.go`
- **Completed:** 2026-03-20

### P0.5 Plan Approval Bypassed ✅ COMPLETED

- [x] ✅ Added `PlanStatusRejected` constant to `internal/state/types.go`
- [x] ✅ Added `aws_approve_plan` tool to `internal/providers/aws.go` with proper input/output types
- [x] ✅ Removed auto-approval from `createInfra` — now requires explicitly approved plans
- [x] ✅ Wired `ErrPlanNotApproved` error from `internal/errors/errors.go`
- [x] ✅ Added `RejectPlan()` method to `internal/state/store.go` with full state validation
- [x] ✅ Updated `ApprovePlan()` to be idempotent and handle rejected plans properly
- [x] ✅ Updated `planInfra` summary message to instruct users to call `aws_approve_plan`
- [x] ✅ Added comprehensive tests for the approval workflow
- **Location:** `internal/providers/aws.go`, `internal/state/store.go`, `internal/state/types.go`, `internal/errors/errors.go`
- **Completed:** 2026-03-24

---

## P1 — Spec Compliance Gaps

### P1.1 Pricing API Parsing ✅ COMPLETED

- [x] Added `github.com/aws/aws-sdk-go-v2/service/pricing` to `go.mod`
- [x] Created `internal/spending/pricing.go` with PricingEstimator
- [x] Architecture for AWS Pricing API with regional pricing lookup
- [x] Implemented 24-hour cache for pricing data
- [x] **Implemented `parsePricingResponse()` to parse AWS Pricing API JSON responses**
- [x] **Navigates nested structure: `terms.OnDemand.<skuTermCode>.priceDimensions.<rateCode>.pricePerUnit.USD`**
- [x] **Handles all error cases: invalid JSON, missing terms, missing USD price, invalid price format**
- [x] **`queryFargatePrice()` now returns actual prices from the API (no longer stubbed)**
- [x] **Added comprehensive tests in `pricing_test.go` (9 test cases for parsePricingResponse)**
- [ ] ALB, NAT Gateway, CloudWatch Logs use hardcoded values directly (no API calls)
- **Status:** ✅ COMPLETED — Fargate pricing now uses real AWS Pricing API data
- **Location:** `internal/spending/pricing.go`, `internal/spending/pricing_test.go`

### P1.2 Current Spend Calculation Hardcoded ✅ COMPLETED

- [x] Updated createInfra to use `CostTracker.GetTotalMonthlySpend()` from Cost Explorer
- [x] Removed hardcoded `currentSpend += 25.0` at `aws.go:220`
- [x] Falls back to local state estimates when Cost Explorer unavailable
- [x] Uses plan `EstimatedCostMo` for each running deployment as fallback
- [x] Improved logging for spend calculation source (Cost Explorer vs fallback)
- **Location:** `internal/providers/aws.go`
- **Completed:** Real spend data from Cost Explorer with graceful fallback to state-based estimates

### P1.3 Container Port Hardcoded to 80 (5 locations) ✅ COMPLETED

- [x] Add `container_port` input parameter to deploy tool
- [x] Update task definition to use configurable port (`aws.go:814`)
- [x] Update ALB target group health checks (`aws.go:865`)
- [x] 5 hardcoded port 80 references across `aws.go`
- **Location:** `internal/providers/aws.go`
- **Completed:** ContainerPort parameter added with default value of 80

### P1.4 Health Check Path Not Configurable ✅ COMPLETED

- [x] Add `health_check_path` input parameter
- [x] Update target group health check configuration (currently "/" at `aws.go:701`)
- **Location:** `internal/providers/aws.go`
- **Completed:** HealthCheckPath parameter added with default value of /

### P1.5 Single Replica Deployments Only ✅ COMPLETED

- [x] Add `desired_count` parameter to deploy tool
- [x] Update ECS service `DesiredCount` (currently always 1 at `aws.go:853`)
- **Location:** `internal/providers/aws.go`
- **Completed:** DesiredCount parameter added with default value of 1

### P1.6 No Environment Variables Support ✅ COMPLETED

- [x] Add `environment` map input to deploy tool
- [x] Pass environment to container definition
- **Location:** `internal/providers/aws.go`
- **Completed:** Environment map parameter added for container environment variables

### P1.7 HTTPS/TLS Support ✅ COMPLETED

- [x] ✅ Added `certificate_arn` parameter to `createInfraInput` struct
- [x] ✅ Added `validateCertificate()` function to verify ACM certificate exists and is ISSUED
- [x] ✅ Updated `provisionALB()` to create HTTPS listener on port 443 with TLS 1.2+ policy
- [x] ✅ Implemented HTTP-to-HTTPS redirect (301) when certificate is provided
- [x] ✅ Updated `getALBURLs()` to return https:// URLs when TLS is enabled
- [x] ✅ Added `ResourceTLSEnabled` and `ResourceCertificateARN` constants to state types
- [x] ✅ Added comprehensive tests for certificate ARN validation and TLS configuration
- [x] ✅ Added `github.com/aws/aws-sdk-go-v2/service/acm` dependency
- **Location:** `internal/providers/aws.go`, `internal/state/types.go`, `go.mod`
- **Completed:** 2026-03-25

### P1.8 AWS Provider Not Using Structured Logging (32 instances) ✅ COMPLETED

- [x] Migrate 30 instances of `log.Printf()` in `aws.go` to `slog`-based structured logging
- [x] Migrate 1 instance in `provider.go`
- [x] Migrate ~4 instances in `costs.go`
- [x] Use existing `internal/logging/logging.go` infrastructure
- **Location:** `internal/providers/aws.go` (30), `internal/providers/provider.go` (1), `internal/spending/costs.go` (~4)

### P1.9 VPC CIDR NOT CONFIGURABLE ⚠️ STILL OPEN

- [ ] No `vpc_cidr` parameter in planInfraInput struct
- [ ] VPC CIDR hardcoded to "10.0.0.0/16" at line 893
- [ ] Subnet CIDRs hardcoded in calculations (lines 962, 997)
- [ ] No CalculateSubnetLayout() function
- **Status:** STILL HARDCODED
- **Impact:** Cannot customize VPC for VPC peering scenarios
- **Spec:** ralph/specs/networking.md
- **Location:** `internal/providers/aws.go`

### P1.10 Subnet CIDRs ✅ COMPLETED

- [x] ✅ 4 subnets created across 2 AZs (per spec ralph/specs/networking.md)
- [x] ✅ Public subnets: 10.0.1.0/24, 10.0.2.0/24 (for ALB, NAT Gateway)
- [x] ✅ Private subnets: 10.0.10.0/24, 10.0.11.0/24 (for ECS tasks)
- [x] ✅ CIDRs derived automatically from VPC CIDR
- **Location:** `internal/providers/aws.go`
- **Completed:** 2026-03-25

### P1.11 Private Subnets ✅ COMPLETED

- [x] ✅ Public/private subnet architecture implemented per spec ralph/specs/networking.md
- [x] ✅ NAT Gateway created in first public subnet for private subnet egress
- [x] ✅ Elastic IP allocated for NAT Gateway
- [x] ✅ Private route table with NAT Gateway route
- [x] ✅ ECS tasks now run in private subnets with AssignPublicIp: DISABLED
- [x] ✅ Separate security groups: ALB SG (public HTTP/HTTPS) and Task SG (internal from ALB only)
- [x] ✅ ALB remains in public subnets, forwards to tasks in private subnets
- [x] ✅ Teardown handles all new resources (NAT GW, EIP, private subnets, private RT)
- [x] ✅ Added ResourceNATGateway, ResourceElasticIP, ResourceRouteTablePrivate, ResourceSecurityGroupALB, ResourceSecurityGroupTask constants
- [x] ✅ Backward compatibility: falls back to legacy resources if new ones not present
- [x] ✅ Added comprehensive tests for networking configuration
- **Location:** `internal/providers/aws.go`, `internal/state/types.go`, `internal/providers/aws_test.go`
- **Completed:** 2026-03-25

### P1.12 Auto Scaling ✅ COMPLETED

- [x] ✅ Added `applicationautoscaling` SDK dependency to go.mod
- [x] ✅ Updated `deployInput` struct with MinCount, MaxCount, TargetCPUPercent, TargetMemPercent
- [x] ✅ Added `scalingInfo` struct and updated `statusOutput` to include scaling information
- [x] ✅ Implemented `validateAutoScalingParams()` for input validation per spec rules
- [x] ✅ Implemented `configureAutoScaling()` to register scalable target and create CPU/memory target tracking policies
- [x] ✅ Implemented `deleteAutoScaling()` to clean up scaling policies and deregister target before ECS service deletion
- [x] ✅ Implemented `getScalingInfo()` to retrieve current scaling config for status reporting
- [x] ✅ Added helper functions `extractClusterName()` and `extractServiceName()`
- [x] ✅ Updated `deploy()` to configure auto-scaling when `max_count > desired_count`
- [x] ✅ Updated `teardown()` to delete auto-scaling before ECS service
- [x] ✅ Comprehensive tests for all new functionality
- **Location:** `internal/providers/aws.go`, `internal/providers/aws_test.go`, `go.mod`
- **Completed:** 2026-03-24

### P1.13 Plan Approval Bypassed ✅ COMPLETED

- [x] ✅ Plans now require explicit approval via `aws_approve_plan` tool
- [x] ✅ Added `RejectPlan()` method for explicit rejection
- [x] ✅ `createInfra` now checks for approved status, returns `ErrPlanNotApproved` otherwise
- **Location:** `internal/providers/aws.go`, `internal/state/store.go`
- **Completed:** 2026-03-24

### P1.14 No Wait for Healthy Deployment ✅ COMPLETED

- [x] Currently returns immediately after creating service
- [x] Implement wait for ECS service to reach RUNNING state
- [x] Check ALB health check passes
- [x] Spec `ralph/specs/aws-provider.md` requires waiting for healthy deployment
- **Location:** `internal/providers/aws.go`

### P1.15 Default Docker Image ✅ COMPLETED

- [x] Default image is `nginx:latest` at `aws.go:787`
- [x] Should require explicit image specification or document default clearly
- [x] Now requires explicit `image_ref` parameter - no nginx:latest default
- **Location:** `internal/providers/aws.go:787`
- **Resolution:** Removed nginx:latest default. The `image_ref` parameter is now required and must be explicitly specified by the user.

### P1.16 Log Retention Hardcoded ✅ COMPLETED

- [x] CloudWatch log retention hardcoded to 7 days at `aws.go:749`
- [x] Make configurable per deployment
- [x] Now configurable via `log_retention_days` parameter
- **Location:** `internal/providers/aws.go:749`

### P1.17 ECS Task Resources Hardcoded ✅ COMPLETED

- [x] CPU hardcoded to "256" at `aws.go:806`
- [x] Memory hardcoded to "512" at `aws.go:807`
- [x] Make configurable via deploy tool parameters
- [x] Now configurable via `cpu` and `memory` parameters
- **Location:** `internal/providers/aws.go:806-807`

### P1.18 Error Wrapping Breaks errors.Is() ✅ COMPLETED

- [x] Inconsistent error wrapping at `aws.go:226`
- [x] Should use `fmt.Errorf("...: %w", err)` for proper wrapping
- **Location:** `internal/providers/aws.go:226`

### P1.19 Fargate CPU/Memory Validation ✅ COMPLETED

- [x] Implemented `ValidateFargateResources(cpu, memory string) error`
- [x] Validates against valid Fargate CPU/memory combinations per AWS docs
- [x] Clear error messages listing valid combinations
- **Spec:** ralph/specs/deploy-configuration.md
- **Location:** `internal/providers/validation.go`

### P1.20 Log Retention Validation ✅ COMPLETED

- [x] Implemented `ValidateLogRetention(days int) error`
- [x] Validates against CloudWatch-accepted retention values
- [x] Wired into createInfra
- **Spec:** ralph/specs/deploy-configuration.md
- **Location:** `internal/providers/validation.go`

### P1.21 Per-Request Spending Override MISSING ❌

- [ ] Spec requires passing spending limits in tool arguments
- [ ] No mechanism to override limits on a per-request basis
- [ ] Users cannot set deployment-specific budget caps via tool inputs
- **Status:** NOT IMPLEMENTED
- **Spec:** ralph/specs/spending-safeguards.md
- **Impact:** Users cannot set deployment-specific limits; global config only
- **Location:** `internal/providers/aws.go` tool inputs

### P1.22 Auto-Scaling Cost Range MISSING ❌

- [ ] planInfra output should include cost range (min × per-task cost, max × per-task cost)
- [ ] Current output only has single `EstimatedCostMo` field
- [ ] Users cannot predict worst-case costs when auto-scaling is configured
- **Status:** NOT IMPLEMENTED
- **Spec:** ralph/specs/auto-scaling.md section 5
- **Impact:** Users cannot predict worst-case costs during planning
- **Location:** `internal/providers/aws.go`

### P1.23 Container Port Validation ✅ COMPLETED

- [x] Implemented `ValidateContainerPort(port int) error`
- [x] Validates port range 1-65535
- [x] Wired into deploy
- **Spec:** ralph/specs/deploy-configuration.md
- **Location:** `internal/providers/validation.go`

### P1.24 Environment Variables Validation ✅ COMPLETED

- [x] Implemented `ValidateEnvironmentVariables(env map[string]string) error`
- [x] Validates name format (alphanumeric + underscore)
- [x] Blocks reserved AWS_, ECS_, FARGATE_ prefixes
- [x] Wired into deploy
- **Location:** `internal/providers/validation.go`

### P1.25 Health Check Path Validation ✅ COMPLETED

- [x] Implemented `ValidateHealthCheckPath(path string) error`
- [x] Validates path starts with /
- [x] Wired into deploy
- **Location:** `internal/providers/validation.go`

### P1.26 AWS Region Validation ✅ COMPLETED

- [x] Implemented `ValidateAWSRegion(region string) error`
- [x] Validates against list of valid AWS regions
- [x] Wired into planInfra
- **Location:** `internal/providers/validation.go`

### P1.27 Desired Count Upper Limit ✅ COMPLETED

- [x] Implemented `ValidateDesiredCount(count int) error`
- [x] Enforces max of 100 to prevent runaway costs
- [x] Wired into deploy
- **Location:** `internal/providers/validation.go`

### P1.28 Container Health Check MISSING ❌

- [ ] No container-level health check in task definition
- [ ] Only ALB health check exists; container can be unhealthy but pass ALB check
- [ ] No health check grace period parameter
- **Status:** NOT IMPLEMENTED
- **Impact:** Unhealthy containers may not be replaced by ECS
- **Location:** `internal/providers/aws.go` task definition

### P1.29 Custom DNS / Route 53 — COMPLETELY MISSING ❌ 🆕

- [ ] Add optional `domain_name` parameter to `aws_plan_infra` tool
- [ ] Implement `findHostedZone()` to lookup existing Route 53 hosted zones (walk-up algorithm for subdomains)
- [ ] Auto-provision ACM certificates with DNS validation when `domain_name` is provided
- [ ] Create Route 53 alias A record pointing custom domain to ALB
- [ ] Update `aws_status` output to show custom domain as primary URL
- [ ] Implement teardown: delete Route 53 record, ACM cert, DNS validation CNAME
- [ ] Add state constants: `ResourceDomainName`, `ResourceHostedZoneID`, `ResourceCertAutoCreated`, `ResourceDNSRecordName` to `internal/state/types.go`
- [ ] Add `github.com/aws/aws-sdk-go-v2/service/route53` SDK dependency
- [ ] Include Route 53 costs in plan estimation ($0.50/mo hosted zone + query costs)
- **Status:** NOT IMPLEMENTED — entire spec `ralph/specs/custom-dns.md` is unimplemented
- **Spec:** ralph/specs/custom-dns.md
- **Impact:** Users cannot map custom domain names to their deployments; no DNS automation
- **Location:** `internal/providers/aws.go`, `internal/state/types.go`, `go.mod`

### P1.30 Distribution / cmd Structure — COMPLETELY MISSING ❌ 🆕

- [ ] Move entry point from `internal/main.go` → `cmd/agent-deploy/main.go` (enables `go install`)
- [ ] Move `internal/main_test.go` → `cmd/agent-deploy/main_test.go`
- [ ] Create `.goreleaser.yml` (version: 2, tar.gz archives, checksum file, cross-platform builds)
- [ ] Create `.github/workflows/release.yml` (trigger on tags matching `v*`)
- [ ] Update `Makefile` build/install/run targets to use `./cmd/agent-deploy`
- [ ] Update `README.md` with installation section (go install, binary download, devcontainer)
- [ ] Add MCP client configuration guide for using agent-deploy from external repos
- [ ] Generate cross-platform prebuilt binaries: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
- **Status:** NOT IMPLEMENTED — entire spec `ralph/specs/distribution.md` is unimplemented
- **Spec:** ralph/specs/distribution.md
- **Impact:** Not distributable; cannot `go install`; no release automation
- **Location:** `internal/main.go`, `Makefile`, `README.md`

---

## P2 — Test Coverage Gaps

> **🎉 MAJOR MILESTONE: 51% overall test coverage achieved — exceeds the 50% target!**
>
> **Note:** CI now tracks coverage percentage and will fail if it drops below 25% (see `.github/workflows/ci.yml`). Target is 50% per `ralph/specs/testing.md` — **TARGET MET**.

### P2.1 AWS Client Package Has No Tests (0% coverage) ✅ COMPLETED

- [x] Create `internal/awsclient/client_test.go`
- [x] Test `LoadConfig` basic behavior (full AWS integration tested elsewhere)
- [x] Test `ResourceTags` generation with all/partial/no fields
- **Coverage:** 0% → 91.7%
- **Location:** `internal/awsclient/`

### P2.2 Errors Package Has No Tests (0% coverage) ✅ COMPLETED

- [x] Create `internal/errors/errors_test.go`
- [x] Test error type identification and wrapping
- [x] Test all error types: ErrPlanNotApproved, ErrProvisioningFailed, ErrInvalidState, ErrNotSupported
- [x] Test error message formatting and Error() method
- [x] Test errors.Is() compatibility
- **Location:** `internal/errors/`

### P2.3 Spending Config Has No Tests (0% coverage) ✅ COMPLETED

- [x] Create tests for `internal/spending/config.go`
- [x] Test `LoadLimits()` with various env var configurations
- [x] Test default values when env vars not set
- [x] Test custom values from env vars
- [x] Test zero values handling
- [x] Test negative values handling
- **Location:** `internal/spending/config.go`

### P2.4 Provider.go Has No Tests (0% coverage) ✅ COMPLETED

- [x] Create tests for `internal/providers/provider.go`
- [x] Test `All()` and `AllWithStore()` registration
- [x] Test Provider and TeardownProvider interface implementations
- [x] Test graceful degradation with nil store
- **Coverage:** 0% → 80%+
- **Location:** `internal/providers/provider.go`

### P2.5 AWS Provider Tool Tests Missing (Coverage improved 18.2% → 42.9%) ⚠️

**Completed:**
- [x] Added 12 new unit tests for validation and error handling
- [x] Tests for deploy, teardown, status, createInfra error paths
- [x] Tests for plan approval/rejection workflows
- [x] Coverage improved from 18.2% to 27.5%
- [x] Coverage improved from 27.5% to 42.9%
- [x] Add unit tests with mocked AWS SDK
- [x] Added NewAWSProviderWithClients and getClients functions for dependency injection
- [x] provisionVPC: 0% → 77%, provisionECSCluster: 0% → 90.9%, provisionALB: 0% → 61.4%
- [x] All AWS client calls now use getClients() dependency injection pattern
- [x] 15+ provisioning/deletion functions updated for testability

**Remaining:**
- [ ] Test error scenarios with full AWS mocking

- **Impact:** Extended test coverage for core AWS provider tools
- **Location:** `internal/providers/aws_test.go`
- **Depends on:** P2.6 (AWS SDK mocking setup) ✅ COMPLETED
- **Progress:** 42.9% coverage achieved with new unit tests

### P2.6 AWS SDK Mocking Infrastructure ✅ COMPLETED

- [x] Create mock interfaces for EC2, ECS, ECR, ELB, CloudWatch clients
- [x] Set up test fixtures for common AWS responses
- [x] Enable unit testing without LocalStack
- **Location:** `internal/awsclient/interfaces.go`, `internal/awsclient/mocks/`
- **Completed:** 2026-03-25

### P2.7 Reconciliation Unit Tests with Mocks ✅ COMPLETED

- [x] Refactored Reconciler to use interfaces (ReconcileEC2API, ReconcileECSAPI, ReconcileELBV2API)
- [x] Added NewReconcilerWithClients() for dependency injection in tests
- [x] Added comprehensive mock-based unit tests
- **Coverage:** reconcile.go now tested via mocks; state package coverage 44.4% → 82.0%
- **Location:** `internal/state/reconcile.go`, `internal/state/reconcile_test.go`
- **Completed:** 2026-03-25

### P2.8 State Store Silent Failure Handling ✅ COMPLETED

- [x] Add logging for malformed state files in List operations
- [x] Silent skips at `store.go:111,248,335` now log warnings
- **Location:** `internal/state/store.go:111,248,335`
- **Completed:** 2026-03-25

### P2.9 Main.go Test Coverage (0%) ❌ 🔴 CONFIRMED

- [ ] Test `main()` function startup
- [ ] Test flag parsing (-http, -log-level, -log-format, etc.)
- [ ] Test signal handling (SIGINT, SIGTERM)
- [ ] Test background service integration (cleanup service, cost monitor)
- [ ] Test exit codes on errors (currently returns instead of os.Exit(1))
- **Status:** CONFIRMED 0% COVERAGE
- **Evidence:**
  - main_test.go has 11 tests but **NONE test main()**
  - Tests create isolated MCP servers, bypassing main() entirely
  - No flag parsing tests
  - No signal handling tests
  - Reconciliation failure doesn't fail startup (should it?)
- **Impact:** Entry point completely untested; flag parsing bugs undetected
- **Location:** `internal/main.go`, `internal/main_test.go`

### P2.10 Concurrent Access Patterns UNTESTED ❌

- [ ] No goroutine usage in store_test.go
- [ ] No t.Parallel() usage
- [ ] No race condition stress tests
- [ ] Store has RWMutex but locking never verified
- **Status:** NOT TESTED
- **Impact:** Concurrent access bugs could go undetected
- **Location:** `internal/state/store_test.go`
- **Note:** State package coverage is 82.0% but concurrent access patterns remain untested

### P2.11 Spending Package Coverage (67.4%) ✅ COMPLETE

- [x] Added CostExplorerAPI interface for dependency injection
- [x] Added NewCostTrackerWithClient() constructor
- [x] Added NewCostMonitorWithTracker() constructor
- [x] Comprehensive tests for CostTracker methods
- [x] CostMonitor lifecycle tests (Start/Stop)
- [x] PricingEstimator tests
- **Coverage:** 23.0% → 67.4%
- **Location:** `internal/spending/costs.go`, `internal/spending/costs_test.go`
- **Completed:** 2026-03-25

### P2.12 Spending Config Tests ✅ RESOLVED

- [x] ✅ Fixed tests in `check_test.go` and `config_test.go` to use isolated HOME directory
- [x] ✅ Tests were picking up a config file from ~/.agent-deploy/config.json which had $70 instead of default $25
- [x] ✅ Tests now properly isolate from the real config file using t.TempDir()
- **Status:** RESOLVED — tests now pass with proper isolation
- **Location:** `internal/spending/check_test.go`, `internal/spending/config_test.go`
- **Completed:** 2026-03-28

---

## P3 — Quality Improvements

### P3.1 AWS Reconciliation Lacks Pagination ✅ COMPLETED

- [x] Add pagination support for `DescribeVpcs` using `NewDescribeVpcsPaginator`
- [x] Add pagination support for `ListClusters` using `NewListClustersPaginator` with batch processing of 100 clusters
- [x] Add pagination support for `DescribeLoadBalancers` using `NewDescribeLoadBalancersPaginator`
- [x] Handle large deployments (>100 resources per API call)
- **Location:** `internal/state/reconcile.go`
- **Completed:** 2026-03-25

### P3.2 Inefficient ALB Tag Fetching ✅ COMPLETED

- [x] Currently makes individual tag API calls per ALB
- [x] Batch tag fetching for multiple resources using `batchFetchALBTags()`
- **Location:** `internal/state/reconcile.go`
- **Completed:** 2026-03-25

### P3.3 Version String Duplicated ✅ COMPLETED

- [x] Consolidate version "v0.1.0" (appears at `main.go:41` and `main.go:165`)
- [x] Use single constant for version
- **Location:** `internal/main.go:41,165`
- **Completed:** 2026-03-25

### P3.4 Cost Monitor Region Hardcoded ✅ COMPLETED

- [x] Cost monitor intentionally uses `us-east-1` (at `main.go:113`)
- **Location:** `internal/main.go:113`
- **Completed:** 2026-03-25
- **Details:** This is intentional; AWS Cost Explorer API is only available in us-east-1.

### P3.5 rollbackInfra Implementation ✅ COMPLETE

- [x] `ErrInvalidState` - Used in store.go (ApprovePlan/RejectPlan state validation)
- [x] `ErrProvisioningFailed` - Used in createInfra with rollbackInfra() for partial failure cleanup
- [x] `ErrPlanNotApproved` - Used in createInfra for plan state validation (P0.5)
- [x] Reverse order deletion implemented
- [x] Continues on delete failures
- [x] Marks infra as destroyed
- [x] ErrProvisioningFailed properly used
- **Location:** `internal/errors/errors.go`, `internal/state/store.go`, `internal/providers/aws.go`
- **Completed:** 2026-03-25

### P3.6 planInfra Cost Estimate Disclaimer ✅ COMPLETED

- The disclaimer is already implemented in internal/spending/pricing.go
- "Estimate based on AWS pricing. Actual costs may vary based on usage." is set on every cost estimate
- **Completed:** 2026-03-25

### P3.7 Makefile Missing Targets ✅ COMPLETED

- [x] Add `coverage` target (with `-coverprofile`)
- [x] Add `coverage-html` target
- [x] Add `test-race` target
- [x] Add `install` target
- [x] Add `run` target
- [x] Add `all` target
- [x] Add `help` target
- **Location:** `Makefile`
- **Completed:** 2026-03-25

### P3.8 Logging Config AddTime Field Unused ✅ COMPLETED

- [x] `AddTime` field in `internal/logging/Config` is defined but never used
- [x] Removed unused AddTime field from logging.Config
- **Location:** `internal/logging/logging.go`
- **Completed:** 2026-03-25

### P3.9 Silent Error Suppression in Store ❌ 🔴 CONFIRMED

- [ ] store.go:86 — writeJSON errors (JSON marshal) silently ignored
- [ ] store.go:123 — file write errors silently ignored
- [ ] store.go:220 — DeleteExpiredPlans delete errors silently ignored
- [ ] Errors should be logged or returned to caller
- **Status:** NOT ADDRESSED — specific line numbers confirmed
- **Impact:** Data loss could go undetected; debugging difficult; state file corruption silent
- **Location:** `internal/state/store.go:86,123,220`
- **Required Work:** Add error logging or return errors for:
  1. JSON marshal failures at line 86
  2. File write failures at line 123
  3. Delete failures in DeleteExpiredPlans at line 220

### P3.10 Missing Error Types ❌

- [ ] Missing `ErrCertificateInvalid` for TLS/ACM errors
- [ ] Missing `ErrInvalidInput` for validation errors
- [ ] Current validation errors use generic fmt.Errorf
- **Status:** NOT IMPLEMENTED
- **Impact:** Inconsistent error handling; harder to test error paths
- **Location:** `internal/errors/errors.go`

### P3.11 Non-Atomic Infrastructure Updates ❌

- [ ] Infrastructure updates in store are not atomic
- [ ] Concurrent updates could cause race conditions
- [ ] Should use file locking or atomic write patterns
- **Status:** NOT ADDRESSED
- **Impact:** Potential data corruption under concurrent access
- **Location:** `internal/state/store.go`

### P3.12 Missing State Transitions ❌

- [ ] No deployment update transition in state model
- [ ] No infrastructure retry transition in state model
- [ ] Some state transitions may be missing from spec
- **Status:** NOT IMPLEMENTED
- **Impact:** Limited state management flexibility
- **Location:** `internal/state/store.go`

### P3.13 Shallow Reconciliation ❌

- [ ] Reconciliation only checks 3 resource types: VPC, ECS cluster, ALB
- [ ] Missing checks for 16+ resource types:
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
- [ ] 19 resource types tracked in state but only 3 reconciled
- **Status:** PARTIAL IMPLEMENTATION — VPC/ECS/ALB reconciled with pagination, others not checked
- **Impact:** Orphaned resources (subnets, NAT gateways, security groups, etc.) may not be detected
- **Location:** `internal/state/reconcile.go`
- **Note:** Pagination and batch tag fetching ARE implemented for the 3 types that exist

### P3.14 Main.go Startup Error Handling ❌

- [ ] Background services not cleaned up on startup failure
- [ ] No verification that services actually started
- [ ] Should verify cost monitor, cleanup service running
- **Status:** NOT IMPLEMENTED
- **Impact:** Partial startup could leave system in bad state
- **Location:** `internal/main.go`

### P3.15 Missing State Constants for Custom DNS 🆕 ❌

- [ ] `ResourceDomainName` — needed for custom domain tracking
- [ ] `ResourceHostedZoneID` — needed for Route 53 zone tracking
- [ ] `ResourceCertAutoCreated` — needed to distinguish auto-provisioned vs user-provided certs
- [ ] `ResourceDNSRecordName` — needed for DNS record cleanup in teardown
- **Status:** NOT IMPLEMENTED — constants only exist in spec, not in code
- **Impact:** Blocks P1.29 (Custom DNS) implementation
- **Spec:** ralph/specs/custom-dns.md
- **Location:** `internal/state/types.go`

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
make build           # Build the binary
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
| `internal/main.go` | MCP server entry point |
| `internal/providers/provider.go` | Provider interface + registration |
| `internal/providers/aws.go` | AWS provider (6 tools, 1 resource, 1 prompt) |
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
| `ralph/specs/custom-dns.md` | Route 53 / custom domain spec |
| `ralph/specs/distribution.md` | Distribution / GoReleaser spec |
| `ralph/specs/ci.md` | CI/CD requirements spec |

### Hardcoded Values Summary

| Value | Location | Status |
|-------|----------|--------|
| VPC CIDR: `10.0.0.0/16` | `aws.go:893` | ❌ HARDCODED — No `vpc_cidr` parameter (P1.9) |
| Subnet CIDRs | `aws.go:962,997` | ❌ HARDCODED — CIDRs calculated from fixed VPC CIDR (P1.9) |
| Fargate pricing | `pricing.go` | ✅ **IMPLEMENTED** — parsePricingResponse() extracts prices from AWS Pricing API (P1.1 completed) |
| ALB pricing | `pricing.go` | ❌ HARDCODED — No Pricing API call for ALB |
| NAT Gateway pricing | `pricing.go` | ❌ HARDCODED — No Pricing API call for NAT Gateway |
| CloudWatch Logs pricing | `pricing.go` | ❌ HARDCODED — No Pricing API call for CloudWatch |
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
| Fargate CPU/Memory compatibility | `validation.go` | ✅ VALIDATED (P1.19) |
| Log retention (CloudWatch allowed values) | `validation.go` | ✅ VALIDATED (P1.20) |
| Container port (1-65535) | `validation.go` | ✅ VALIDATED (P1.23) |
| Environment variable names | `validation.go` | ✅ VALIDATED (P1.24) |
| Health check path (must start with /) | `validation.go` | ✅ VALIDATED (P1.25) |
| AWS region | `validation.go` | ✅ VALIDATED (P1.26) |
| Desired count upper limit | `validation.go` | ✅ VALIDATED (P1.27) |

### Remaining Work by Priority

| Priority | Count | Items |
|----------|-------|-------|
| **P0 Critical** | 0 | *(All P0 issues resolved)* |
| **P1 Spec Gaps** | 6 | P1.9 (VPC CIDR hardcoded), P1.21 (per-request spending override), P1.22 (auto-scaling cost range), P1.28 (container health check), **P1.29 (Custom DNS / Route 53)** 🆕, **P1.30 (Distribution / cmd structure)** 🆕 |
| **P2 Test Gaps** | 3 | P2.5 (AWS error scenarios), P2.9 (main.go 0% confirmed), P2.10 (concurrent access untested) |
| **P3 Quality** | 7 | P3.9 (silent error suppression), P3.10 (missing error types), P3.11 (non-atomic updates), P3.12 (missing state transitions), P3.13 (shallow reconciliation 3/19), P3.14 (startup error handling), **P3.15 (missing DNS state constants)** 🆕 |
| **P5 Stretch** | 3 | CloudFormation, multi-cloud, secrets |
| **Total remaining** | **19** | |

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
| **aws-provider.md** | Plan approval before provisioning | ✅ IMPLEMENTED — explicit approval workflow |
| **aws-provider.md** | Rollback on provisioning failure | ✅ IMPLEMENTED — rollbackInfra() cleans up partial resources |
| **ecr-image-push.md** | Push local images to ECR | ✅ **IMPLEMENTED** — P0.6 completed |
| **deploy-configuration.md** | Fargate CPU/memory validation | ✅ IMPLEMENTED — P1.19 |
| **deploy-configuration.md** | Log retention validation | ✅ IMPLEMENTED — P1.20 |
| **deploy-configuration.md** | Container port validation (1-65535) | ✅ IMPLEMENTED — P1.23 |
| **deploy-configuration.md** | Environment variables validation | ✅ IMPLEMENTED — P1.24 |
| **deploy-configuration.md** | Health check path validation (must start with /) | ✅ IMPLEMENTED — P1.25 |
| **custom-dns.md** | Route 53 hosted zone lookup | ❌ **NOT IMPLEMENTED** — P1.29 |
| **custom-dns.md** | ACM certificate auto-provisioning | ❌ **NOT IMPLEMENTED** — P1.29 |
| **custom-dns.md** | DNS alias A record creation | ❌ **NOT IMPLEMENTED** — P1.29 |
| **custom-dns.md** | DNS resource tracking in state | ❌ **NOT IMPLEMENTED** — P3.15 |
| **distribution.md** | Move main.go to cmd/agent-deploy/ | ❌ **NOT IMPLEMENTED** — P1.30 |
| **distribution.md** | GoReleaser + release workflow | ❌ **NOT IMPLEMENTED** — P1.30 |
| **distribution.md** | `go install` support | ❌ **NOT IMPLEMENTED** — P1.30 |
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
| **auto-scaling.md** | Cost range in planInfra output (min/max) | ❌ NOT IMPLEMENTED — P1.22 |
| **networking.md** | VPC CIDR configurable | ❌ NOT IMPLEMENTED — hardcoded to 10.0.0.0/16 (P1.9) |
| **networking.md** | Private subnets with NAT Gateway | ✅ IMPLEMENTED |
| **ci.md** | CI workflow with lint, test, build jobs | ✅ IMPLEMENTED |
| **testing.md** | 50% code coverage | ✅ **TARGET MET** — 51% overall |
| **testing.md** | main.go test coverage | ❌ **0% COVERAGE** — P2.9 |
| **testing.md** | Concurrent access testing | ❌ NOT TESTED — P2.10 |
| **error-handling.md** | Domain error types | ⚠️ PARTIAL — missing ErrCertificateInvalid, ErrInvalidInput (P3.10) |
| **operational.md** | No silent error suppression | ❌ NOT ADDRESSED — store.go:86,123,220 (P3.9) |
