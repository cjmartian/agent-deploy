# Agent Deploy

Deploy applications to the cloud using natural language.

Agent Deploy is an [MCP](https://modelcontextprotocol.io) server that lets an AI agent plan, provision, deploy, monitor, and tear down AWS infrastructure on your behalf. You describe what you want; the agent handles the rest.

```
User → AI Agent → agent-deploy (MCP) → AWS
```

## How it works

1. You build an application and ask the agent to deploy it.
2. The agent calls `aws_plan_infra` — it asks clarifying questions (expected users, latency, region), then returns a plan with a cost estimate.
3. You approve the plan.
4. The agent calls `aws_create_infra` to provision VPC, subnets, ECS cluster, ALB, security groups, etc.
5. The agent calls `aws_deploy` to push your container image to ECR and start an ECS service behind the ALB.
6. You get back public URLs and status via `aws_status`.
7. When you're done, `aws_teardown` deletes everything in reverse order.

## MCP tools

| Tool | Description |
|------|-------------|
| `aws_plan_infra` | Analyze requirements and propose an infrastructure plan with cost estimate |
| `aws_create_infra` | Provision AWS infrastructure from an approved plan |
| `aws_deploy` | Deploy a container image onto provisioned infrastructure |
| `aws_status` | Get deployment status and public URLs |
| `aws_teardown` | Tear down all resources for a deployment |

The server also exposes an `aws:deployments` resource (JSON list of all deployments) and an `aws_deploy_plan` prompt.

## Installation

### Option 1: `go install` (requires Go 1.25+)

```sh
go install github.com/cjmartian/agent-deploy/cmd/agent-deploy@latest
```

### Option 2: Download prebuilt binary

```sh
# Linux (amd64)
curl -sL https://github.com/cjmartian/agent-deploy/releases/latest/download/agent-deploy_linux_amd64.tar.gz \
  | tar xz -C /usr/local/bin agent-deploy

# macOS (Apple Silicon)
curl -sL https://github.com/cjmartian/agent-deploy/releases/latest/download/agent-deploy_darwin_arm64.tar.gz \
  | tar xz -C /usr/local/bin agent-deploy
```

### Option 3: Build from source

```sh
git clone https://github.com/cjmartian/agent-deploy.git
cd agent-deploy
make build    # produces ./agent-deploy
```

### Option 4: Devcontainer (for Codespaces)

In your repo's `.devcontainer/devcontainer.json`:

```jsonc
{
  "postCreateCommand": "go install github.com/cjmartian/agent-deploy/cmd/agent-deploy@latest"
}
```

## Getting started

### Prerequisites

- AWS credentials configured (`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`, `~/.aws/credentials`, or any method supported by the AWS SDK)

### Run

**Stdio transport** (for MCP-aware agents like VS Code Copilot or Claude Desktop):

```sh
./agent-deploy
```

**HTTP transport** (for testing with curl or web-based MCP clients):

```sh
./agent-deploy -http :8080
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-http` | _(empty — uses stdio)_ | Address for streamable HTTP transport |
| `-log-level` | `info` | `debug`, `info`, `warn`, `error` |
| `-log-format` | `text` | `text` or `json` |

### Connect an MCP client

Add agent-deploy as an MCP server in your client's configuration. For example, in VS Code `settings.json`:

```jsonc
{
  "mcp": {
    "servers": {
      "agent-deploy": {
        "command": "/path/to/agent-deploy"
      }
    }
  }
}
```

Or for the HTTP transport:

```jsonc
{
  "mcp": {
    "servers": {
      "agent-deploy": {
        "url": "http://localhost:8080"
      }
    }
  }
}
```

## Spending safeguards

Budget limits prevent runaway costs. Defaults are $100/month overall and $25 per deployment.

Configure via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_DEPLOY_MONTHLY_BUDGET` | `100` | Monthly budget in USD |
| `AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET` | `25` | Per-deployment budget in USD |
| `AGENT_DEPLOY_ALERT_THRESHOLD` | `80` | Alert when spend reaches this % of budget |

Or via config file at `~/.agent-deploy/config.json`:

```json
{
  "spending_limits": {
    "monthly_budget_usd": 100,
    "per_deployment_usd": 25,
    "alert_threshold_percent": 80
  }
}
```

A background cost monitor checks AWS Cost Explorer and can alert or auto-teardown deployments that exceed their budget.

## Testing

```sh
make test     # run all unit tests
```

## Project structure

```
cmd/
  agent-deploy/      # MCP server entrypoint (per distribution.md spec)
    main.go          # Entry point - enables `go install`
    main_test.go     # Server integration tests
internal/
  providers/
    provider.go      # Provider interface
    aws.go           # AWS provider (tools, resources, prompts)
  state/             # Deployment state storage and cleanup
  spending/          # Budget checks, cost tracking, monitoring
  awsclient/         # AWS SDK configuration
  errors/            # Domain error types
  id/                # ULID-based ID generation
  logging/           # Structured logging
```

## Using from another repository

To use agent-deploy from a separate Codespace or repo (e.g., one containing only your applications):

1. Install agent-deploy (see Installation above)
2. Add MCP configuration to your repo's `.vscode/mcp.json`:

```jsonc
{
  "servers": {
    "agent-deploy": {
      "command": "agent-deploy",
      "env": {
        "AWS_REGION": "us-east-1"
      }
    }
  }
}
```

Or in `.vscode/settings.json`:

```jsonc
{
  "mcp": {
    "servers": {
      "agent-deploy": {
        "command": "agent-deploy"
      }
    }
  }
}
```

AWS credentials should be provided via Codespace secrets, not committed to the repo.
