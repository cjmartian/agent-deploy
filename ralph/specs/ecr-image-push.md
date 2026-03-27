# ECR Image Push Specification

## Problem

When deploying with `aws_deploy`, the provider creates an ECR repository and registers an ECS task definition, but **never pushes the container image to ECR**. The task definition receives the raw `image_ref` value (e.g. `tic-tac-toe:latest`), which only exists in the local Docker daemon. ECS Fargate cannot pull from the local daemon, so the task fails to start and the ALB returns **503**.

### Current Deploy Flow (Broken)

1. `ensureECRRepository` — creates an ECR repo named `agent-deploy-<deployID[:12]>`
2. `createTaskDefinition` — registers a task definition with `Image` set to the raw `image_ref` (e.g. `tic-tac-toe:latest`)
3. `createOrUpdateService` — starts the ECS service

Fargate tries to pull `tic-tac-toe:latest` from Docker Hub, which doesn't exist there. The task never becomes healthy. The ALB target group has no healthy targets, so all requests return 503.

## Requirements

### 1. Push Local Images to ECR

When `image_ref` refers to a local image (not already an ECR URI or a known public registry), the provider must:

1. Authenticate Docker with ECR using `ecr:GetAuthorizationToken`.
2. Tag the local image with the full ECR URI: `<account_id>.dkr.ecr.<region>.amazonaws.com/<repo_name>:<tag>`.
3. Push the image to ECR.
4. Use the full ECR URI in the task definition instead of the raw `image_ref`.

### 2. Detect Image Source

Determine whether `image_ref` is already a fully-qualified registry reference or a local-only image.

| Pattern | Classification | Action |
|---------|---------------|--------|
| `<account>.dkr.ecr.<region>.amazonaws.com/...` | ECR URI | Use as-is |
| `docker.io/...`, `ghcr.io/...`, `public.ecr.aws/...` | Public registry | Use as-is |
| `<name>:<tag>` or `<name>` (no registry prefix) | Local image | Push to ECR |

```go
func isLocalImage(imageRef string) bool
```

### 3. Updated Deploy Flow

1. `ensureECRRepository` — create ECR repo (existing, unchanged)
2. **`pushImageToECR`** — if local image, authenticate, tag, and push to ECR *(new step)*
3. `createTaskDefinition` — use the ECR URI as the image reference *(modified)*
4. `createOrUpdateService` — start/update ECS service (existing, unchanged)

### 4. Implementation: `pushImageToECR`

```go
func (p *AWSProvider) pushImageToECR(
    ctx context.Context,
    cfg aws.Config,
    infra *state.Infrastructure,
    imageRef string,
    deployID string,
) (ecrImageURI string, err error)
```

**Steps (using the Docker Engine SDK):**

1. Call `ecr:GetAuthorizationToken` (AWS SDK) to get a Docker login token.
2. Decode the base64-encoded token to get `username:password`.
3. Determine the ECR repo name (`agent-deploy-<deployID[:12]>`) and construct the full URI.
4. Extract the tag from `imageRef` (default to `latest`).
5. Create a Docker client via `docker/client.NewClientWithOpts(client.FromEnv)`.
6. Call `client.ImageTag(ctx, imageRef, ecrURI+":"+tag)` to tag the local image.
7. Build a base64-encoded `registry.AuthConfig{Username, Password, ServerAddress}` for the push.
8. Call `client.ImagePush(ctx, ecrURI+":"+tag, image.PushOptions{RegistryAuth: encodedAuth})`.
9. Read the push response stream to completion, checking for errors in the JSON messages.
10. Return the full ECR image URI.

### 5. Error Handling

| Failure | Behavior |
|---------|----------|
| `GetAuthorizationToken` fails | Return error, mark deployment failed |
| Docker client creation fails | Return error indicating Docker daemon is not available |
| `ImageTag` fails | Return error: `"image '<ref>' not found locally; build it first or provide a full registry URI"` |
| `ImagePush` fails | Return error with push details, mark deployment failed |
| Push stream contains error JSON | Parse and return the error message from the stream |

### 6. Logging

Log at each step:

- `slog.Info("pushing image to ECR", "imageRef", imageRef, "ecrURI", ecrURI)`
- `slog.Info("image pushed to ECR", "ecrURI", ecrURI)`
- `slog.Error(...)` on failure with full context

## Changes Required

| File | Change |
|------|--------|
| `internal/providers/aws.go` | Add `pushImageToECR` and `isLocalImage` functions |
| `internal/providers/aws.go` | Update `deploy()` to call `pushImageToECR` between `ensureECRRepository` and `createTaskDefinition` |
| `internal/providers/aws.go` | Pass the returned ECR URI to `createTaskDefinition` instead of raw `image_ref` |
| `internal/providers/aws_test.go` | Add tests for `isLocalImage`, mock tests for `pushImageToECR` |
| `go.mod` | Add `github.com/docker/docker` and `github.com/docker/go-connections` dependencies |

## Dependencies

The implementation uses the Docker Engine SDK (`github.com/docker/docker/client`) for image tagging and pushing. This avoids a runtime dependency on the `docker` CLI being on `PATH` and provides structured error handling via the Go API.

Key packages:
- `github.com/docker/docker/client` — Docker client creation
- `github.com/docker/docker/api/types/image` — `PushOptions`
- `github.com/docker/docker/api/types/registry` — `AuthConfig`

## Open Questions

- **Image build:** Should the provider also support building from a `Dockerfile` path, or is that out of scope? Currently the user must build locally before calling `aws_deploy`.
