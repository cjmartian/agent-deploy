# Custom DNS Specification

## Overview

Deployments currently receive an auto-generated ALB DNS name (e.g. `my-alb-1234567890.us-east-1.elb.amazonaws.com`). These names are opaque, hard to remember, and unsuitable for production use. This spec adds optional custom domain support via Route 53, including automated ACM certificate provisioning with DNS validation.

## Current State

| Aspect | Current Behavior |
|--------|-----------------|
| Public URL | ALB-generated DNS name only |
| TLS | Optional via pre-existing `certificate_arn` on `aws_create_infra` |
| Route 53 | Not used |
| DNS management | None |

The `aws_status` tool returns URLs like `http://agent-deploy-abc123-1234567890.us-east-1.elb.amazonaws.com`. Users must manually create DNS records and provision certificates outside the tool.

## Requirements

### 1. Domain Name Parameter on `aws_plan_infra`

Add an optional `domain_name` parameter so the agent can ask the user what they want their URL to be during the planning phase.

```go
type planInfraInput struct {
    AppDescription string `json:"app_description" jsonschema:"description of the application,required"`
    ExpectedUsers  int    `json:"expected_users"  jsonschema:"estimated concurrent users,required"`
    LatencyMs      int    `json:"latency_ms"      jsonschema:"target p99 latency in ms,required"`
    Region         string `json:"region"          jsonschema:"AWS region,required"`
    DomainName     string `json:"domain_name"     jsonschema:"custom domain name (e.g. app.example.com). Requires a Route 53 hosted zone for the parent domain"`
}
```

When present, the plan summary should include the custom domain and note that a Route 53 hosted zone is required:

```
Proposed plan for "portfolio": ECS Fargate in us-east-1.
Custom domain: portfolio.example.com (Route 53 hosted zone: example.com)
Estimated cost: $48.73/mo (includes Route 53 hosted zone $0.50/mo).
```

### 2. Hosted Zone Lookup

During `aws_create_infra`, look up an existing Route 53 hosted zone for the domain's parent zone.

```go
func (p *AWSProvider) findHostedZone(ctx context.Context, cfg aws.Config, domainName string) (hostedZoneID string, err error)
```

**Logic:**

1. Extract the parent domain from `domain_name` (e.g. `app.example.com` → `example.com`).
2. Call `route53:ListHostedZonesByName` with `DNSName` set to the parent domain.
3. Match the hosted zone whose `Name` equals the parent domain (with trailing dot).
4. If no matching hosted zone is found, return an error instructing the user to create one or delegate DNS.

**Edge cases:**

| Input | Parent Zone | Behavior |
|-------|-------------|----------|
| `app.example.com` | `example.com` | Look up `example.com.` hosted zone |
| `staging.app.example.com` | `app.example.com`, then `example.com` | Walk up the domain tree until a hosted zone is found |
| `example.com` | `example.com` | Look up `example.com.` hosted zone (apex record) |

### 3. ACM Certificate Provisioning

When `domain_name` is provided and no `certificate_arn` is given, automatically provision an ACM certificate with DNS validation via Route 53.

```go
func (p *AWSProvider) provisionCertificate(
    ctx context.Context,
    cfg aws.Config,
    domainName string,
    hostedZoneID string,
) (certificateARN string, err error)
```

**Steps:**

1. Call `acm:RequestCertificate` with `DomainName` and `ValidationMethod: DNS`.
2. Poll `acm:DescribeCertificate` until `DomainValidationOptions` contains the CNAME validation record.
3. Create the validation CNAME record in Route 53 via `route53:ChangeResourceRecordSets` (UPSERT).
4. Wait for certificate status to become `ISSUED` (poll with backoff, timeout after 5 minutes).
5. Return the certificate ARN.

If `certificate_arn` is already provided alongside `domain_name`, skip certificate provisioning and use the provided ARN (existing TLS behavior).

### 4. DNS Alias Record

After the ALB is created, create a Route 53 alias record pointing the custom domain to the ALB.

```go
func (p *AWSProvider) createDNSRecord(
    ctx context.Context,
    cfg aws.Config,
    hostedZoneID string,
    domainName string,
    albDNSName string,
    albHostedZoneID string,
) error
```

**Record details:**

| Field | Value |
|-------|-------|
| Type | A (alias) |
| Name | `domain_name` (e.g. `app.example.com`) |
| AliasTarget.DNSName | ALB DNS name |
| AliasTarget.HostedZoneId | ALB's canonical hosted zone ID |
| AliasTarget.EvaluateTargetHealth | `true` |

Use `UPSERT` so the operation is idempotent on re-deploy.

```go
_, err = r53Client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
    HostedZoneId: aws.String(hostedZoneID),
    ChangeBatch: &route53types.ChangeBatch{
        Changes: []route53types.Change{{
            Action: route53types.ChangeActionUpsert,
            ResourceRecordSet: &route53types.ResourceRecordSet{
                Name: aws.String(domainName),
                Type: route53types.RRTypeA,
                AliasTarget: &route53types.AliasTarget{
                    DNSName:              aws.String(albDNSName),
                    HostedZoneId:         aws.String(albHostedZoneID),
                    EvaluateTargetHealth: aws.Bool(true),
                },
            },
        }},
    },
})
```

### 5. Updated `aws_status` Output

When a custom domain is configured, `aws_status` should return the custom URL as the primary URL:

```json
{
  "deployment_id": "deploy-01HX...",
  "status": "running",
  "urls": [
    "https://portfolio.example.com",
    "https://agent-deploy-abc123-1234567890.us-east-1.elb.amazonaws.com"
  ],
  "custom_domain": "portfolio.example.com"
}
```

The custom URL should be listed first. The raw ALB URL remains as a fallback.

### 6. Teardown

`aws_teardown` must clean up DNS resources in the correct order:

1. Delete the Route 53 alias record (A record for the custom domain).
2. Delete the ACM certificate (if it was auto-provisioned, not user-provided).
3. Delete the ACM DNS validation CNAME record.
4. Proceed with existing teardown (ALB, ECS, VPC, etc.).

Do **not** delete the hosted zone itself — it is a shared resource the user manages.

### 7. State Tracking

Store DNS-related resource identifiers in the infrastructure record:

```go
const (
    ResourceDomainName      = "domain_name"       // e.g. "app.example.com"
    ResourceHostedZoneID    = "hosted_zone_id"     // Route 53 hosted zone ID
    ResourceCertificateARN  = "certificate_arn"    // ACM certificate ARN (existing constant)
    ResourceCertAutoCreated = "cert_auto_created"  // "true" if cert was auto-provisioned
    ResourceDNSRecordName   = "dns_record_name"    // the A record created
)
```

### 8. Cost Estimation

When `domain_name` is provided, include Route 53 costs in the plan estimate:

| Resource | Cost |
|----------|------|
| Route 53 hosted zone | $0.50/mo (only if a new zone would be created — currently we require an existing zone) |
| Route 53 queries | ~$0.40/mo per million queries (standard) |
| ACM certificate | Free |

## Behavior Matrix

| `domain_name` | `certificate_arn` | Behavior |
|---------------|-------------------|----------|
| Not provided | Not provided | HTTP-only with ALB DNS name (current default) |
| Not provided | Provided | HTTPS with ALB DNS name (existing TLS spec) |
| Provided | Not provided | Auto-provision ACM cert + DNS validation + alias record |
| Provided | Provided | Use provided cert + create alias record (skip cert provisioning) |

## Error Handling

| Scenario | Behavior |
|----------|----------|
| No hosted zone found for domain | Error: `"no Route 53 hosted zone found for 'example.com'. Create a hosted zone first or use a domain managed by Route 53"` |
| ACM certificate fails to validate within 5 minutes | Error with instructions; leave cert in pending state for manual resolution |
| Domain already has an A record (non-alias) | Error: `"existing A record found for 'app.example.com'. Delete it or use a different subdomain"` |
| Invalid domain name format | Validation error before any AWS calls |
| IAM permissions missing for Route 53 or ACM | Error indicating which permissions are required |

## IAM Permissions

The following additional IAM permissions are required when using custom DNS:

```json
{
  "Effect": "Allow",
  "Action": [
    "route53:ListHostedZonesByName",
    "route53:ChangeResourceRecordSets",
    "route53:GetChange",
    "acm:RequestCertificate",
    "acm:DescribeCertificate",
    "acm:DeleteCertificate",
    "acm:ListCertificates"
  ],
  "Resource": "*"
}
```

## Dependencies

Add the Route 53 SDK:

```
github.com/aws/aws-sdk-go-v2/service/route53
```

ACM SDK is already included (from TLS spec).

## File Locations

| File | Changes |
|------|---------|
| `internal/providers/aws.go` | Add `domain_name` input to `planInfra`; add `findHostedZone`, `provisionCertificate`, `createDNSRecord`; update `createInfra` flow; update `teardown` for DNS cleanup; update `getALBURLs` for custom domain |
| `internal/state/types.go` | Add `ResourceDomainName`, `ResourceHostedZoneID`, `ResourceCertAutoCreated`, `ResourceDNSRecordName` constants |
| `internal/awsclient/interfaces.go` | Add Route 53 client interface |
| `internal/awsclient/client.go` | Add Route 53 client initialization |
| `internal/awsclient/mocks/route53.go` | Mock Route 53 client for tests |
| `go.mod` | Add `service/route53` dependency |
| `internal/providers/aws_test.go` | Tests for hosted zone lookup, cert provisioning, DNS record creation, teardown cleanup |
| `cleanup.sh` | Add Route 53 record and ACM certificate discovery/cleanup |
