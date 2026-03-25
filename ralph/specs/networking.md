# Networking Specification

## Overview

The system provisions a VPC with networking infrastructure for each deployment. Currently, VPC CIDR, subnet CIDRs, and subnet topology are hardcoded with no private subnet tier. This spec defines configurable networking with public/private subnet architecture.

## Current State

### Hardcoded Values

| Value | Location | Current |
|-------|----------|---------|
| VPC CIDR | `aws.go:provisionVPC` | `10.0.0.0/16` |
| Subnet CIDRs | `aws.go:provisionVPC` | `10.0.1.0/24`, `10.0.2.0/24` |
| Subnet type | `aws.go:provisionVPC` | Public only |

### Missing Infrastructure

- No private subnets for ECS tasks (all tasks get public IPs)
- No NAT Gateway for private subnet egress
- No way to avoid CIDR conflicts with existing VPCs/peering

## Requirements

### 1. Configurable VPC CIDR

Add a `vpc_cidr` parameter to the plan or make it configurable globally.

```go
type planInfraInput struct {
    // ... existing fields ...
    VpcCIDR string `json:"vpc_cidr,omitempty" jsonschema:"VPC CIDR block (default: 10.0.0.0/16)"`
}
```

**Validation:**
- Must be a valid IPv4 CIDR block
- Prefix length must be between /16 and /24
- Default: `10.0.0.0/16`

### 2. Public/Private Subnet Architecture

Create both public and private subnets across 2 availability zones.

#### Subnet Layout (for default 10.0.0.0/16)

| Subnet | CIDR | AZ | Purpose |
|--------|------|----|---------|
| Public A | `10.0.1.0/24` | AZ-1 | ALB, NAT Gateway |
| Public B | `10.0.2.0/24` | AZ-2 | ALB |
| Private A | `10.0.10.0/24` | AZ-1 | ECS tasks |
| Private B | `10.0.11.0/24` | AZ-2 | ECS tasks |

#### Dynamic CIDR Calculation

When a custom VPC CIDR is provided, derive subnet CIDRs automatically:

```go
type SubnetLayout struct {
    PublicCIDRs  []string // 2 public subnet CIDRs
    PrivateCIDRs []string // 2 private subnet CIDRs
}

// CalculateSubnetLayout derives 4 subnet CIDRs from a VPC CIDR.
// Splits the VPC into /24 blocks:
//   - Public:  .1.0/24, .2.0/24
//   - Private: .10.0/24, .11.0/24
func CalculateSubnetLayout(vpcCIDR string) (*SubnetLayout, error)
```

### 3. NAT Gateway

When private subnets are used, provision a NAT Gateway so ECS tasks can reach the internet (pull container images, call external APIs).

#### Provisioning Steps

1. Allocate an Elastic IP for the NAT Gateway
2. Create NAT Gateway in the first public subnet
3. Create a private route table with a route to the NAT Gateway
4. Associate private subnets with the private route table

#### Resource Tracking

Add new resource constants:

```go
const (
    ResourceNATGateway       = "nat_gateway"
    ResourceElasticIP        = "elastic_ip"
    ResourceRouteTablePrivate = "route_table_private"
    ResourceSubnetPrivate    = "subnet_private" // already defined
)
```

#### Cost Impact

NAT Gateway adds ~$32/month (hourly rate) plus data processing charges. This must be reflected in cost estimates (see `cost-estimation.md`).

### 4. ECS Task Placement

With private subnets available, ECS tasks should run in private subnets:

```go
// Current (public):
AssignPublicIp: ecstypes.AssignPublicIpEnabled

// Updated (private):
AssignPublicIp: ecstypes.AssignPublicIpDisabled
Subnets:        privateSubnetIDs  // instead of public
```

The ALB remains in public subnets, forwarding traffic to tasks in private subnets.

### 5. Security Group Updates

With public/private subnet separation, tighten security groups:

| Security Group | Inbound Rules | Applied To |
|----------------|---------------|------------|
| ALB SG | `0.0.0.0/0:80`, `0.0.0.0/0:443` | ALB |
| ECS Task SG | ALB SG → container port | ECS tasks |

Currently a single security group allows inbound 80/443 from anywhere and is used for both ALB and ECS tasks.

```go
// Create separate security groups:
// 1. ALB security group - allows public HTTP/HTTPS inbound
// 2. Task security group - allows inbound only from ALB SG on container port
```

### 6. Teardown Updates

Teardown must clean up new resources in reverse dependency order:

1. Delete NAT Gateway (and wait for deletion)
2. Release Elastic IP
3. Delete private route table
4. Delete private subnets
5. ... (existing teardown continues)

NAT Gateway deletion is asynchronous — the teardown must wait for the NAT Gateway to reach `deleted` state before releasing the Elastic IP.

### 7. Reconciliation Updates

The reconciler must discover orphaned NAT Gateways and Elastic IPs:

- Query `DescribeNatGateways` with `agent-deploy:*` tag filter
- Query `DescribeAddresses` with `agent-deploy:*` tag filter

## Configuration Summary

| Parameter | Source | Default |
|-----------|--------|---------|
| `vpc_cidr` | Plan input | `10.0.0.0/16` |
| Subnet CIDRs | Derived from VPC CIDR | Auto-calculated |
| Private subnets | Always created | Yes |
| NAT Gateway | Created with private subnets | Yes |

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Invalid CIDR format | Reject plan with validation error |
| CIDR too small for 4 subnets | Reject plan with explanation |
| CIDR conflicts with existing VPC | AWS API will return error; surface to user |
| NAT Gateway creation fails | Mark infra as failed; cleanup already-created resources |
| NAT Gateway deletion timeout | Log warning, continue teardown, flag for reconciliation |
| AZ has fewer than 2 zones | Fail with clear error (existing behavior) |

## File Locations

| File | Changes |
|------|---------|
| `internal/providers/aws.go` | Update `provisionVPC` for private subnets, NAT GW, split SGs; update teardown |
| `internal/state/types.go` | Add `ResourceNATGateway`, `ResourceElasticIP`, `ResourceRouteTablePrivate` |
| `internal/state/reconcile.go` | Add NAT Gateway and EIP orphan detection |
| `internal/spending/pricing.go` | Include NAT Gateway in cost estimates |
