# Operational Improvements Specification

## Overview

Several operational issues affect reliability, developer experience, and correctness at scale. This spec covers AWS API pagination, version management, region consistency, Makefile targets, and dead code cleanup.

## 1. AWS Reconciliation Pagination

### Problem

The reconciler makes single API calls without pagination. AWS APIs return a maximum number of results per call (typically 100). Deployments beyond the first page are invisible to reconciliation.

### Affected API Calls

| API Call | Location | Max Results |
|----------|----------|-------------|
| `DescribeVpcs` | `reconcile.go:findTaggedVPCs` | 1000 (but filtered) |
| `ListClusters` | `reconcile.go:findTaggedECSClusters` | 100 |
| `DescribeLoadBalancers` | `reconcile.go:findTaggedALBs` | 400 |

### Solution

Use AWS SDK paginators for all list operations:

```go
func (r *Reconciler) findTaggedVPCs(ctx context.Context) ([]ec2types.Vpc, error) {
    var allVPCs []ec2types.Vpc

    paginator := ec2.NewDescribeVpcsPaginator(r.ec2Client, &ec2.DescribeVpcsInput{
        Filters: []ec2types.Filter{{
            Name:   aws.String("tag-key"),
            Values: []string{"agent-deploy:created-by"},
        }},
    })

    for paginator.HasMorePages() {
        page, err := paginator.NextPage(ctx)
        if err != nil {
            return nil, fmt.Errorf("describe VPCs: %w", err)
        }
        allVPCs = append(allVPCs, page.Vpcs...)
    }

    return allVPCs, nil
}
```

Apply the same pattern for ECS clusters and ALBs.

### Batch ALB Tag Fetching

Currently, tags are fetched per-ALB individually. Use `DescribeTags` with multiple resource ARNs (up to 20 per call):

```go
func (r *Reconciler) batchFetchALBTags(ctx context.Context, arns []string) (map[string]map[string]string, error) {
    tags := make(map[string]map[string]string)

    // Process in batches of 20 (API limit)
    for i := 0; i < len(arns); i += 20 {
        end := min(i+20, len(arns))
        batch := arns[i:end]

        resp, err := r.albClient.DescribeTags(ctx, &elbv2.DescribeTagsInput{
            ResourceArns: batch,
        })
        if err != nil {
            return nil, err
        }

        for _, desc := range resp.TagDescriptions {
            tagMap := make(map[string]string)
            for _, tag := range desc.Tags {
                tagMap[*tag.Key] = *tag.Value
            }
            tags[*desc.ResourceArn] = tagMap
        }
    }

    return tags, nil
}
```

## 2. Version Management

### Problem

The version string `"v0.1.0"` is duplicated at two locations in `main.go` (lines 41 and 165). Updating one without the other causes version drift.

### Solution

Define a single version constant:

```go
// internal/version.go (or top of main.go)
const Version = "v0.1.0"
```

Reference `Version` in both the server info and the startup log message. Consider using build-time injection via ldflags for release builds:

```makefile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o agent-deploy ./internal
```

```go
var Version = "dev" // overridden by ldflags
```

## 3. Cost Monitor Region

### Problem

The cost monitor always uses `us-east-1` (hardcoded at `main.go:113`), while the reconciliation region is configurable via `-reconcile-region`. Cost Explorer API is only available in `us-east-1`, so the hardcoding is correct for Cost Explorer — but it should be documented and validated, not silently hardcoded.

### Solution

1. Document in the CLI help that Cost Explorer requires `us-east-1` (this is an AWS limitation)
2. The `CostTracker` already handles this correctly (overrides to `us-east-1` internally)
3. Remove the redundant region specification in `main.go` and let `CostTracker` handle it:

```go
// Instead of:
awsCfg, err := awsclient.LoadConfig(ctx, "us-east-1")

// Use any valid config — CostTracker overrides to us-east-1 internally:
awsCfg, err := awsclient.LoadConfig(ctx, *reconcileRegion)
```

## 4. Makefile Targets

### Current State

The Makefile only has `build` and `test` targets.

### Required Targets

```makefile
.PHONY: all build test test-race lint coverage coverage-html run install clean help

# Default target
all: lint test build

# Build the binary
build:
	go build -o agent-deploy ./internal

# Run all tests
test:
	go test ./...

# Run tests with race detector
test-race:
	go test -race ./...

# Run linter
lint:
	golangci-lint run ./...

# Generate coverage report
coverage:
	go test -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

# Generate HTML coverage report
coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run the server (stdio mode)
run: build
	./agent-deploy

# Install binary to GOPATH/bin
install:
	go install ./internal

# Clean build artifacts
clean:
	rm -f agent-deploy coverage.out coverage.html

# Show help
help:
	@echo "Available targets:"
	@echo "  all           - lint, test, and build"
	@echo "  build         - build the binary"
	@echo "  test          - run tests"
	@echo "  test-race     - run tests with race detector"
	@echo "  lint          - run golangci-lint"
	@echo "  coverage      - generate coverage report"
	@echo "  coverage-html - generate HTML coverage report"
	@echo "  run           - build and run (stdio mode)"
	@echo "  install       - install to GOPATH/bin"
	@echo "  clean         - remove build artifacts"
	@echo "  help          - show this help"
```

## 5. Dead Code Cleanup

### Unused Logging Config Field

`AddTime` field in `internal/logging/Config` is defined but never used. Either:

1. **Remove it** — if no implementation is planned
2. **Implement it** — add timestamp to log records when `AddTime` is true (slog already includes timestamps by default, so this field is likely unnecessary)

Recommendation: Remove the field.

### Unused Error Types

After implementing plan approval (`plan-approval.md`) and error handling (`error-handling.md`), all three currently-unused error types (`ErrPlanNotApproved`, `ErrProvisioningFailed`, `ErrInvalidState`) will be wired. No removal needed.

## File Locations

| File | Changes |
|------|---------|
| `internal/state/reconcile.go` | Add pagination to all AWS API calls; batch ALB tag fetching |
| `internal/main.go` | Extract version constant; fix cost monitor region |
| `Makefile` | Add all, test-race, lint, coverage, coverage-html, run, install, clean, help targets |
| `internal/logging/logging.go` | Remove unused `AddTime` field |
| `internal/state/reconcile_test.go` | Update tests for paginated responses |
