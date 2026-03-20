# AWS Provider Specification

## Overview

The AWS provider enables natural-language deployment of applications to Amazon Web Services. It exposes MCP tools for planning, provisioning, deploying, monitoring, and tearing down infrastructure.

## Tools

### `aws_plan_infra`

Analyzes application requirements and proposes an infrastructure plan with cost estimate.

**Input:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `app_description` | string | yes | Description of the application to deploy |
| `expected_users` | int | yes | Estimated number of concurrent users |
| `latency_ms` | int | yes | Target p99 latency in milliseconds |
| `region` | string | yes | Preferred AWS region (e.g., us-east-1) |

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `plan_id` | string | Unique identifier for this plan |
| `services` | []string | AWS services included in the plan |
| `estimated_cost_monthly` | string | Estimated monthly cost |
| `summary` | string | Human-readable plan summary |

**Behavior:**
1. Analyze application requirements
2. Select appropriate AWS services (ECS Fargate, Lambda, EC2, etc.)
3. Query AWS Pricing API for cost estimation
4. Generate unique plan_id for tracking
5. Return structured plan for user approval

---

### `aws_create_infra`

Provisions AWS infrastructure according to an approved plan.

**Input:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `plan_id` | string | yes | Plan ID from `aws_plan_infra` |

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `infra_id` | string | Unique infrastructure identifier |
| `status` | string | Provisioning status |

**Behavior:**
1. Validate plan_id exists and is approved
2. Check spending limits before proceeding
3. Create VPC with public/private subnets
4. Set up security groups
5. Create ECS cluster (or other compute)
6. Configure ALB with target groups
7. Set up CloudWatch logging
8. Return infra_id for deployment

---

### `aws_deploy`

Deploys an application onto provisioned infrastructure.

**Input:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `infra_id` | string | yes | Infrastructure ID from `aws_create_infra` |
| `image_ref` | string | yes | Container image or artifact reference |

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `deployment_id` | string | Unique deployment identifier |
| `status` | string | Deployment status |

**Behavior:**
1. Validate infra_id exists
2. Create/update ECR repository if needed
3. Push or reference container image
4. Create ECS task definition
5. Update ECS service
6. Wait for healthy deployment
7. Return deployment_id

---

### `aws_status`

Gets current status and public URLs of a deployment.

**Input:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `deployment_id` | string | yes | Deployment ID from `aws_deploy` |

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `deployment_id` | string | Deployment identifier |
| `status` | string | Current status (pending, running, failed, etc.) |
| `urls` | []string | Public URLs where app is accessible |

**Behavior:**
1. Query ECS service status
2. Check ALB target group health
3. Get ALB DNS name(s)
4. Return consolidated status

---

### `aws_teardown`

Tears down all AWS resources for a deployment.

**Input:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `deployment_id` | string | yes | Deployment ID to tear down |

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `deployment_id` | string | Deployment identifier |
| `status` | string | Teardown status |

**Behavior:**
1. Stop ECS service
2. Delete ECS cluster
3. Remove ALB and target groups
4. Delete security groups
5. Delete VPC and subnets
6. Clean up CloudWatch logs
7. Delete ECR repository (optional)

---

## Resources

### `aws:deployments`

Returns list of all current deployments.

**URI:** `aws:deployments`
**MIME Type:** `application/json`

**Response:**
```json
{
  "deployments": [
    {
      "deployment_id": "deploy-xxx",
      "infra_id": "infra-xxx",
      "status": "running",
      "created_at": "2024-01-01T00:00:00Z",
      "urls": ["https://..."]
    }
  ]
}
```

---

## Prompts

### `aws_deploy_plan`

Guided workflow for planning and deploying an application.

**Arguments:**
| Name | Required | Description |
|------|----------|-------------|
| `app_description` | yes | Brief description of the application |

**Workflow:**
1. Ask clarifying questions about traffic, latency, region
2. Generate infrastructure plan with `aws_plan_infra`
3. Present plan and cost estimate for approval
4. On approval, provision with `aws_create_infra`
5. Deploy with `aws_deploy`
6. Return public URLs

---

## Authentication

The provider requires AWS credentials via:
- Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
- AWS credentials file (`~/.aws/credentials`)
- IAM role (when running on AWS)

Required IAM permissions:
- EC2: VPC, subnet, security group management
- ECS: Cluster, service, task definition management
- ELB: ALB management
- ECR: Repository management
- CloudWatch: Log group management
- Pricing: GetProducts (for cost estimation)
