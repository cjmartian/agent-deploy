# TLS/HTTPS Specification

## Overview

The ALB currently only serves HTTP traffic on port 80. Production deployments require HTTPS with TLS termination at the load balancer. This spec defines optional HTTPS support using AWS Certificate Manager (ACM).

## Current State

The ALB is provisioned with a single HTTP listener:

```go
elbClient.CreateListener(ctx, &elbv2.CreateListenerInput{
    LoadBalancerArn: aws.String(albARN),
    Protocol:        elbv2types.ProtocolEnumHttp,
    Port:            aws.Int32(80),
    DefaultActions:  []elbv2types.Action{{
        Type:           elbv2types.ActionTypeEnumForward,
        TargetGroupArn: aws.String(tgARN),
    }},
})
```

No HTTPS listener exists. The security group allows port 443 inbound but nothing listens on it.

## Requirements

### 1. Optional Certificate ARN Parameter

Add an optional `certificate_arn` parameter to `aws_create_infra`. When provided, an HTTPS listener is created alongside the HTTP listener.

```go
type createInfraInput struct {
    PlanID         string `json:"plan_id"         jsonschema:"plan ID from aws_plan_infra,required"`
    CertificateARN string `json:"certificate_arn" jsonschema:"ACM certificate ARN for HTTPS (optional)"`
}
```

### 2. HTTPS Listener

When a certificate ARN is provided, create an HTTPS listener on port 443:

```go
// HTTPS listener (port 443)
_, err = elbClient.CreateListener(ctx, &elbv2.CreateListenerInput{
    LoadBalancerArn: aws.String(albARN),
    Protocol:        elbv2types.ProtocolEnumHttps,
    Port:            aws.Int32(443),
    SslPolicy:       aws.String("ELBSecurityPolicy-TLS13-1-2-2021-06"),
    Certificates: []elbv2types.Certificate{{
        CertificateArn: aws.String(certificateARN),
    }},
    DefaultActions: []elbv2types.Action{{
        Type:           elbv2types.ActionTypeEnumForward,
        TargetGroupArn: aws.String(tgARN),
    }},
})
```

### 3. HTTP-to-HTTPS Redirect

When HTTPS is enabled, reconfigure the HTTP listener to redirect to HTTPS instead of forwarding:

```go
// HTTP listener becomes a redirect when HTTPS is enabled
_, err = elbClient.CreateListener(ctx, &elbv2.CreateListenerInput{
    LoadBalancerArn: aws.String(albARN),
    Protocol:        elbv2types.ProtocolEnumHttp,
    Port:            aws.Int32(80),
    DefaultActions: []elbv2types.Action{{
        Type: elbv2types.ActionTypeEnumRedirect,
        RedirectConfig: &elbv2types.RedirectActionConfig{
            Protocol:   aws.String("HTTPS"),
            Port:       aws.String("443"),
            StatusCode: elbv2types.RedirectActionStatusCodeEnumHttp301,
        },
    }},
})
```

### 4. URL Scheme

Update `getALBURLs` to return the correct scheme based on whether HTTPS is configured:

```go
// Without TLS: http://alb-dns-name
// With TLS:    https://alb-dns-name
```

Store whether TLS is enabled in the infrastructure record:

```go
const ResourceTLSEnabled = "tls_enabled" // "true" or "false"
```

### 5. Certificate Validation

Before creating the HTTPS listener, validate the certificate:

1. Confirm the ARN format is valid (`arn:aws:acm:region:account:certificate/id`)
2. Call `acm.DescribeCertificate` to verify the certificate exists and is `ISSUED`
3. Return a clear error if the certificate is pending validation or expired

```go
// Required IAM permission: acm:DescribeCertificate
func (p *AWSProvider) validateCertificate(ctx context.Context, cfg aws.Config, certARN string) error
```

### 6. TLS Policy

Use a modern TLS security policy that enforces TLS 1.2+:

| Policy | TLS Versions | Use Case |
|--------|-------------|----------|
| `ELBSecurityPolicy-TLS13-1-2-2021-06` | TLS 1.2, 1.3 | Default (recommended) |

Do not support TLS 1.0 or 1.1.

## Behavior Matrix

| Certificate ARN | HTTP (80) | HTTPS (443) | URLs |
|----------------|-----------|-------------|------|
| Not provided | Forward to target group | No listener | `http://...` |
| Provided | Redirect to HTTPS | Forward to target group | `https://...` |

## Dependencies

Add the ACM SDK:

```
github.com/aws/aws-sdk-go-v2/service/acm
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| No certificate ARN provided | HTTP-only mode (current behavior) |
| Invalid certificate ARN format | Reject with validation error |
| Certificate not found | Reject with clear error |
| Certificate not yet issued (pending validation) | Reject with instructions to complete DNS validation |
| Certificate expired | Reject with error |

## File Locations

| File | Changes |
|------|---------|
| `internal/providers/aws.go` | Add `certificate_arn` input; create HTTPS listener; HTTP redirect; certificate validation |
| `internal/state/types.go` | Add `ResourceTLSEnabled` constant |
| `go.mod` | Add `service/acm` dependency |
| `internal/providers/aws_test.go` | Test HTTPS listener creation, HTTP redirect, certificate validation |
