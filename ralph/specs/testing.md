# Testing Specification

## Overview

Multiple packages have 0% test coverage, and the AWS provider has only 8.3% coverage (only `planInfra` is tested). This spec defines the testing strategy, coverage targets, and mocking infrastructure needed to achieve adequate test coverage without requiring live AWS credentials.

## Current Coverage

| Package | Coverage | Gap |
|---------|----------|-----|
| `internal/awsclient/` | 0% | No tests exist |
| `internal/errors/` | 0% | No tests exist |
| `internal/spending/config.go` | 0% | No tests exist |
| `internal/providers/provider.go` | 0% | No tests exist |
| `internal/providers/aws.go` | 8.3% | Only `planInfra` tested |
| `internal/main.go` | 0% | Test file doesn't test `main()` |
| `internal/spending/` | 21.5% | Partial |
| `internal/state/` | 45.5% | Partial |

## Coverage Targets

| Package | Target | Priority |
|---------|--------|----------|
| `internal/awsclient/` | 80% | P2 |
| `internal/errors/` | 100% | P2 |
| `internal/spending/config.go` | 90% | P2 |
| `internal/providers/provider.go` | 80% | P2 |
| `internal/providers/aws.go` | 60% | P2 |
| `internal/spending/` | 70% | P2 |
| `internal/state/` | 70% | P2 |

## Requirements

### 1. AWS SDK Mocking Infrastructure

Create mock interfaces for AWS service clients to enable unit testing without AWS credentials.

#### Interface Extraction

Define interfaces matching the AWS SDK methods used by the provider:

```go
// internal/awsclient/interfaces.go

// EC2API is the subset of EC2 API used by agent-deploy.
type EC2API interface {
    CreateVpc(ctx context.Context, params *ec2.CreateVpcInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error)
    ModifyVpcAttribute(ctx context.Context, params *ec2.ModifyVpcAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyVpcAttributeOutput, error)
    CreateInternetGateway(ctx context.Context, params *ec2.CreateInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateInternetGatewayOutput, error)
    AttachInternetGateway(ctx context.Context, params *ec2.AttachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.AttachInternetGatewayOutput, error)
    DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
    CreateSubnet(ctx context.Context, params *ec2.CreateSubnetInput, optFns ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error)
    ModifySubnetAttribute(ctx context.Context, params *ec2.ModifySubnetAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifySubnetAttributeOutput, error)
    CreateRouteTable(ctx context.Context, params *ec2.CreateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteTableOutput, error)
    CreateRoute(ctx context.Context, params *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error)
    AssociateRouteTable(ctx context.Context, params *ec2.AssociateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.AssociateRouteTableOutput, error)
    CreateSecurityGroup(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
    AuthorizeSecurityGroupIngress(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
    // Teardown methods
    DeleteVpc(ctx context.Context, params *ec2.DeleteVpcInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error)
    DeleteSubnet(ctx context.Context, params *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error)
    DeleteInternetGateway(ctx context.Context, params *ec2.DeleteInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteInternetGatewayOutput, error)
    DetachInternetGateway(ctx context.Context, params *ec2.DetachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DetachInternetGatewayOutput, error)
    DeleteRouteTable(ctx context.Context, params *ec2.DeleteRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteTableOutput, error)
    DeleteSecurityGroup(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
}

// ECSAPI is the subset of ECS API used by agent-deploy.
type ECSAPI interface {
    CreateCluster(ctx context.Context, params *ecs.CreateClusterInput, optFns ...func(*ecs.Options)) (*ecs.CreateClusterOutput, error)
    DeleteCluster(ctx context.Context, params *ecs.DeleteClusterInput, optFns ...func(*ecs.Options)) (*ecs.DeleteClusterOutput, error)
    RegisterTaskDefinition(ctx context.Context, params *ecs.RegisterTaskDefinitionInput, optFns ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error)
    CreateService(ctx context.Context, params *ecs.CreateServiceInput, optFns ...func(*ecs.Options)) (*ecs.CreateServiceOutput, error)
    UpdateService(ctx context.Context, params *ecs.UpdateServiceInput, optFns ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
    DeleteService(ctx context.Context, params *ecs.DeleteServiceInput, optFns ...func(*ecs.Options)) (*ecs.DeleteServiceOutput, error)
    DescribeServices(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
}

// ELBAPI is the subset of ELBv2 API used by agent-deploy.
type ELBAPI interface {
    CreateLoadBalancer(ctx context.Context, params *elbv2.CreateLoadBalancerInput, optFns ...func(*elbv2.Options)) (*elbv2.CreateLoadBalancerOutput, error)
    CreateTargetGroup(ctx context.Context, params *elbv2.CreateTargetGroupInput, optFns ...func(*elbv2.Options)) (*elbv2.CreateTargetGroupOutput, error)
    CreateListener(ctx context.Context, params *elbv2.CreateListenerInput, optFns ...func(*elbv2.Options)) (*elbv2.CreateListenerOutput, error)
    DescribeLoadBalancers(ctx context.Context, params *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
    DeleteLoadBalancer(ctx context.Context, params *elbv2.DeleteLoadBalancerInput, optFns ...func(*elbv2.Options)) (*elbv2.DeleteLoadBalancerOutput, error)
    DeleteTargetGroup(ctx context.Context, params *elbv2.DeleteTargetGroupInput, optFns ...func(*elbv2.Options)) (*elbv2.DeleteTargetGroupOutput, error)
}
```

#### Mock Implementations

Create mock structs that implement these interfaces, with configurable return values and error injection:

```go
// internal/awsclient/mocks/ec2.go

type MockEC2 struct {
    CreateVpcFunc                func(ctx context.Context, params *ec2.CreateVpcInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error)
    CreateSubnetFunc             func(ctx context.Context, params *ec2.CreateSubnetInput, optFns ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error)
    // ... etc
}

func (m *MockEC2) CreateVpc(ctx context.Context, params *ec2.CreateVpcInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error) {
    if m.CreateVpcFunc != nil {
        return m.CreateVpcFunc(ctx, params, optFns...)
    }
    // Return default success response
    return &ec2.CreateVpcOutput{
        Vpc: &ec2types.Vpc{VpcId: aws.String("vpc-mock-123")},
    }, nil
}
```

#### Provider Refactor

Update `AWSProvider` to accept interfaces instead of creating clients inline:

```go
type AWSProvider struct {
    store     *state.Store
    ec2Client EC2API      // nil = create from aws.Config at call time
    ecsClient ECSAPI
    elbClient ELBAPI
}

// WithMockClients sets mock clients for testing.
func (p *AWSProvider) WithMockClients(ec2 EC2API, ecs ECSAPI, elb ELBAPI) {
    p.ec2Client = ec2
    p.ecsClient = ecs
    p.elbClient = elb
}
```

### 2. Package-Level Test Plans

#### `internal/awsclient/client_test.go`

| Test | Description |
|------|-------------|
| `TestLoadConfig_DefaultRegion` | Verify config loads with specified region |
| `TestResourceTags_AllFields` | All three IDs provided |
| `TestResourceTags_PartialFields` | Only some IDs provided |
| `TestResourceTags_NoFields` | No IDs — only `created-by` tag |

#### `internal/errors/errors_test.go`

| Test | Description |
|------|-------------|
| `TestErrorTypes_AreDistinct` | Each error has unique identity |
| `TestErrorWrapping_Is` | `errors.Is()` works through wrapping |
| `TestErrorWrapping_As` | `errors.As()` works for type checking |
| `TestErrorMessages` | Error messages are human-readable |

#### `internal/spending/config_test.go`

| Test | Description |
|------|-------------|
| `TestDefaultLimits` | Verify default values |
| `TestLoadLimits_EnvVars` | Environment variables override defaults |
| `TestLoadLimits_ConfigFile` | Config file values loaded correctly |
| `TestLoadLimits_EnvOverridesConfig` | Env vars take precedence over config file |
| `TestLoadLimits_InvalidEnvVar` | Invalid env var values fall back to defaults |
| `TestLoadLimits_NoConfig` | Missing config file doesn't error |

#### `internal/providers/provider_test.go`

| Test | Description |
|------|-------------|
| `TestAll_ReturnsProviders` | `All()` returns at least the AWS provider |
| `TestAllWithStore_PassesStore` | Store is passed to providers |
| `TestGetAWSProvider_ReturnsProvider` | Helper returns the correct type |
| `TestGetAWSProvider_NilStore` | Returns nil when store is nil |
| `TestTeardownProvider_Interface` | AWSProvider implements TeardownProvider |

#### `internal/providers/aws_test.go` (expanded)

| Test | Description |
|------|-------------|
| `TestCreateInfra_Success` | Full provisioning with mocked AWS |
| `TestCreateInfra_VPCFails` | VPC creation failure triggers rollback |
| `TestCreateInfra_ECSFails` | ECS failure after VPC success triggers rollback |
| `TestCreateInfra_ALBFails` | ALB failure triggers rollback |
| `TestCreateInfra_BudgetExceeded` | Budget check blocks provisioning |
| `TestDeploy_Success` | Full deployment with mocked AWS |
| `TestDeploy_InfraNotReady` | Rejects deployment on non-ready infra |
| `TestDeploy_MissingImage` | Rejects empty image_ref |
| `TestStatus_RunningDeployment` | Returns correct status for running service |
| `TestStatus_AWSUnavailable` | Returns cached status when AWS unreachable |
| `TestTeardown_Success` | Full teardown with mocked AWS |
| `TestTeardown_PartialFailure` | Continues teardown even if some deletes fail |

### 3. Integration Tests

Integration tests require LocalStack or real AWS credentials and use the `integration` build tag.

```go
//go:build integration

func TestFullWorkflow_Integration(t *testing.T) {
    // Plan → Approve → Create Infra → Deploy → Status → Teardown
}
```

#### LocalStack Configuration

```bash
# docker-compose.yml for LocalStack
services:
  localstack:
    image: localstack/localstack:3.0
    ports:
      - "4566:4566"
    environment:
      - SERVICES=ec2,ecs,ecr,elbv2,cloudwatch,iam,sts
```

### 4. Test Utilities

#### State Store Test Helper

```go
// internal/state/testutil.go
func NewTestStore(t *testing.T) *Store {
    t.Helper()
    dir := t.TempDir()
    store, err := NewStore(dir)
    require.NoError(t, err)
    return store
}
```

### 5. CI Coverage Enforcement

Add coverage reporting to the CI workflow:

```yaml
- name: Test with coverage
  run: go test -coverprofile=coverage.out -covermode=atomic ./...

- name: Check coverage threshold
  run: |
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    if (( $(echo "$COVERAGE < 50" | bc -l) )); then
      echo "Coverage $COVERAGE% is below 50% threshold"
      exit 1
    fi
```

### 6. Race Detector

All tests must pass with the race detector enabled:

```bash
go test -race ./...
```

Add a CI job for race detection and a Makefile target:

```makefile
test-race:
	go test -race ./...
```

## State Store Silent Failures

The state store silently skips malformed JSON files in `List*` operations (`store.go:111,248,335`). Add logging for these cases:

```go
if err := json.Unmarshal(data, &plan); err != nil {
    slog.Warn("skipping malformed state file",
        slog.String("file", path),
        logging.Err(err))
    continue
}
```

## File Locations

| File | Status |
|------|--------|
| `internal/awsclient/interfaces.go` | New: AWS SDK interfaces |
| `internal/awsclient/mocks/ec2.go` | New: EC2 mock |
| `internal/awsclient/mocks/ecs.go` | New: ECS mock |
| `internal/awsclient/mocks/elb.go` | New: ELBv2 mock |
| `internal/awsclient/client_test.go` | New: awsclient tests |
| `internal/errors/errors_test.go` | New: error type tests |
| `internal/spending/config_test.go` | New: config tests |
| `internal/providers/provider_test.go` | New: provider registration tests |
| `internal/providers/aws_test.go` | Expand: add mock-based tool tests |
| `internal/state/testutil.go` | New: test helpers |
