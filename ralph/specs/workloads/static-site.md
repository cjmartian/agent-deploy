# Static Site Workload Specification

## Overview

A static site is HTML, CSS, JavaScript, and assets served directly from object storage via a CDN. There is no application server — the "compute" is the CDN edge. This is the cheapest and fastest way to serve frontend-only apps, documentation sites, blogs, and marketing pages.

## Examples

User descriptions that should classify as `static-site`:

- "Deploy this React app, it's just static files"
- "Host my documentation site"
- "This is a Vite/Next.js/Hugo/Jekyll static export"
- "Serve these HTML files"
- "Deploy my SPA — no backend needed"

**Important:** If the description mentions an API, backend, server, or database, this is NOT a static site — classify as `web-service` instead.

## Infrastructure Shape (AWS)

| Component | Service | Purpose |
|-----------|---------|---------|
| Storage | S3 | Hosts static files |
| CDN | CloudFront | Global edge caching, HTTPS |
| DNS | Route 53 (optional) | Custom domain |
| Certificate | ACM (us-east-1) | TLS for custom domain |

**No compute, no VPC, no ALB, no NAT Gateway, no containers.**

### Cost Profile

| Traffic | Monthly Cost |
|---------|-------------|
| < 1 TB transfer, < 10M requests | ~$1-3 |
| Personal site / documentation | ~$0.50-1.00 |
| Moderate traffic blog | ~$2-5 |

S3 storage is ~$0.023/GB/mo. CloudFront free tier includes 1 TB transfer and 10M requests/mo. For a personal portfolio, this is essentially free.

## Requirements

### 1. Build Detection

The planner must detect that the app is a static site and determine the build output directory.

```go
type StaticSiteConfig struct {
    BuildCommand string // "npm run build", "hugo", "jekyll build", etc.
    OutputDir    string // "dist", "build", "public", "_site", "out"
    IndexFile    string // "index.html" (default)
    ErrorFile    string // "404.html" or "index.html" (for SPAs)
    IsSPA        bool   // Single-page app (all routes serve index.html)
}
```

**Auto-detection from project files:**

| File Found | Framework | Build Command | Output Dir |
|-----------|-----------|---------------|------------|
| `package.json` with `vite` | Vite | `npm run build` | `dist` |
| `package.json` with `react-scripts` | CRA | `npm run build` | `build` |
| `package.json` with `next` | Next.js | `npx next build && npx next export` | `out` |
| `hugo.toml` / `config.toml` | Hugo | `hugo` | `public` |
| `_config.yml` | Jekyll | `jekyll build` | `_site` |
| `index.html` (no build tool) | Plain HTML | None | `.` (current dir) |

### 2. S3 Bucket Provisioning

```go
import "github.com/aws/aws-sdk-go-v2/service/s3"

func (p *AWSProvider) provisionStaticSiteBucket(
    ctx context.Context,
    cfg aws.Config,
    infraID string,
) (bucketName string, err error) {
    s3Client := s3.NewFromConfig(cfg)

    bucketName = fmt.Sprintf("agent-deploy-%s", infraID[:12])

    _, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
        Bucket: aws.String(bucketName),
    })
    if err != nil {
        return "", fmt.Errorf("create S3 bucket: %w", err)
    }

    // Block all public access — CloudFront uses OAC, not public bucket access
    _, err = s3Client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
        Bucket: aws.String(bucketName),
        PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
            BlockPublicAcls:       aws.Bool(true),
            BlockPublicPolicy:     aws.Bool(true),
            IgnorePublicAcls:      aws.Bool(true),
            RestrictPublicBuckets: aws.Bool(true),
        },
    })

    return bucketName, err
}
```

### 3. CloudFront Distribution

```go
import "github.com/aws/aws-sdk-go-v2/service/cloudfront"

func (p *AWSProvider) createCloudFrontDistribution(
    ctx context.Context,
    cfg aws.Config,
    bucketName string,
    bucketRegion string,
    isSPA bool,
    domainName string,       // optional
    certificateARN string,  // optional, must be in us-east-1
) (distributionID string, distributionDomain string, err error)
```

**Configuration:**

| Setting | Value |
|---------|-------|
| Origin | S3 bucket via Origin Access Control (OAC) |
| Viewer Protocol Policy | Redirect HTTP to HTTPS |
| Default Root Object | `index.html` |
| Price Class | `PriceClass_100` (US/EU only — cheapest) |
| Cache Policy | `CachingOptimized` for assets, `CachingDisabled` for HTML |
| Error Pages (SPA) | 403/404 → `/index.html` with 200 status |
| Compress | Enabled (gzip + brotli) |

**SPA routing:** For single-page apps, configure custom error responses so that all paths serve `index.html`:

```go
if isSPA {
    customErrorResponses = []cftypes.CustomErrorResponse{
        {
            ErrorCode:          aws.Int32(403),
            ResponseCode:       aws.Int32(200),
            ResponsePagePath:   aws.String("/index.html"),
            ErrorCachingMinTTL: aws.Int64(0),
        },
        {
            ErrorCode:          aws.Int32(404),
            ResponseCode:       aws.Int32(200),
            ResponsePagePath:   aws.String("/index.html"),
            ErrorCachingMinTTL: aws.Int64(0),
        },
    }
}
```

**Origin Access Control (OAC):** Use OAC instead of the deprecated Origin Access Identity (OAI):

```go
oacOut, err := cfClient.CreateOriginAccessControl(ctx, &cloudfront.CreateOriginAccessControlInput{
    OriginAccessControlConfig: &cftypes.OriginAccessControlConfig{
        Name:                          aws.String(fmt.Sprintf("agent-deploy-%s", infraID[:12])),
        OriginAccessControlOriginType: cftypes.OriginAccessControlOriginTypesS3,
        SigningBehavior:               cftypes.OriginAccessControlSigningBehaviorsAlways,
        SigningProtocol:               cftypes.OriginAccessControlSigningProtocolsSigv4,
    },
})
```

Then add an S3 bucket policy allowing CloudFront to read via the OAC.

### 4. File Upload (Deploy)

Deploying a static site means uploading files to S3, not pushing a container image.

```go
func (p *AWSProvider) deployStaticSite(
    ctx context.Context,
    cfg aws.Config,
    bucketName string,
    sourceDir string,
    distributionID string,
) error
```

**Steps:**

1. Build the project (run the build command if detected).
2. Walk the output directory.
3. Upload each file to S3 with the correct `Content-Type` based on extension.
4. Delete any S3 objects not in the current build (sync).
5. Create a CloudFront invalidation for `/*` to clear the cache.

**Content-Type mapping:**

| Extension | Content-Type |
|-----------|-------------|
| `.html` | `text/html; charset=utf-8` |
| `.css` | `text/css` |
| `.js` | `application/javascript` |
| `.json` | `application/json` |
| `.svg` | `image/svg+xml` |
| `.png` | `image/png` |
| `.jpg`, `.jpeg` | `image/jpeg` |
| `.woff2` | `font/woff2` |
| `.woff` | `font/woff` |

Set `Cache-Control` headers:
- HTML files: `no-cache` (always revalidate)
- Hashed assets (e.g., `index-abc123.js`): `max-age=31536000, immutable`
- Other assets: `max-age=86400`

### 5. Custom DNS

When `domain_name` is provided:

1. Provision an ACM certificate in **us-east-1** (CloudFront requirement, regardless of bucket region).
2. DNS-validate via Route 53.
3. Add the domain as an `Alias` in the CloudFront distribution.
4. Create a Route 53 alias record (A + AAAA) pointing to the CloudFront distribution.

CloudFront distributions have their own hosted zone ID for Route 53 alias records: `Z2FDTNDATAQYW2`.

### 6. Status Output

```json
{
  "deployment_id": "deploy-01HX...",
  "status": "running",
  "workload_type": "static-site",
  "urls": [
    "https://cjmartian.com",
    "https://d1234abcdef.cloudfront.net"
  ],
  "cdn": {
    "distribution_id": "E1234567890",
    "status": "Deployed",
    "price_class": "PriceClass_100",
    "cache_invalidation_status": "Completed"
  },
  "storage": {
    "bucket": "agent-deploy-abc123",
    "object_count": 42,
    "total_size_mb": 2.3
  }
}
```

### 7. Teardown

1. Delete the Route 53 records (A and AAAA alias records).
2. Delete the ACM certificate (if auto-provisioned).
3. Disable and delete the CloudFront distribution (must be disabled first, then wait for status `Deployed`, then delete).
4. Delete the Origin Access Control.
5. Empty and delete the S3 bucket.

**CloudFront teardown is slow** — disabling a distribution can take 10-15 minutes. The teardown should handle this with polling.

### 8. State Resources

```go
const (
    ResourceS3Bucket           = "s3_bucket"            // Bucket name
    ResourceCloudFrontDist     = "cloudfront_dist"       // Distribution ID
    ResourceCloudFrontDomain   = "cloudfront_domain"     // Distribution domain name
    ResourceCloudFrontOAC      = "cloudfront_oac"        // Origin Access Control ID
    ResourceStaticSiteBuildCmd = "static_build_cmd"      // Build command used
    ResourceStaticSiteOutDir   = "static_output_dir"     // Build output directory
)
```

## Changes Required

| File | Change |
|------|--------|
| `internal/state/types.go` | Add static site resource constants |
| `internal/providers/aws.go` | Add `provisionStaticSiteBucket()`, `createCloudFrontDistribution()`, `deployStaticSite()` |
| `internal/providers/aws.go` | Update planner to detect static sites and estimate S3+CloudFront costs |
| `internal/providers/aws.go` | Update deploy to handle file upload instead of container push |
| `internal/providers/aws.go` | Update status and teardown for CloudFront/S3 resources |
| `internal/awsclient/interfaces.go` | Add S3 and CloudFront client interfaces |
| `go.mod` | Add `github.com/aws/aws-sdk-go-v2/service/s3`, `github.com/aws/aws-sdk-go-v2/service/cloudfront` |
