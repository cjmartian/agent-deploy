# Live Testing Plan

End-to-end live testing of agent-deploy: build a tool via natural language, deploy it to AWS, verify it works, and tear it down.

---

## Phase 0: AWS Account Prep

### 1. Create IAM Credentials

Create a dedicated IAM user (or use temporary credentials via SSO). Attach a policy with these permissions:

- `ec2:*` — VPC, subnets, security groups, NAT Gateway, Elastic IP, Internet Gateway, route tables
- `ecs:*` — clusters, services, task definitions
- `ecr:*` — repositories, image push/pull
- `elasticloadbalancing:*` — ALB, target groups, listeners
- `iam:CreateRole`, `iam:DeleteRole`, `iam:AttachRolePolicy`, `iam:DetachRolePolicy`, `iam:GetRole`, `iam:PassRole` — ECS task execution role
- `logs:*` — CloudWatch log groups
- `application-autoscaling:*` — if testing auto-scaling
- `pricing:GetProducts` — cost estimation
- `ce:GetCostAndUsage` — optional, for cost monitoring

> **Tip:** Scope the policy with a condition like `aws:RequestTag/managed-by = agent-deploy` where possible to limit blast radius.

### 2. Export Credentials

```sh
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=us-east-1
```

### 3. Set a Conservative Budget

```sh
export AGENT_DEPLOY_MONTHLY_BUDGET=20
export AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET=20
```

---

## Phase 1: Build the Binary

```sh
cd /workspaces/agent-deploy
make build
```

Confirm `./agent-deploy` exists and runs:

```sh
./agent-deploy --help
```

---

## Phase 2: Wire Up to Your AI Agent

### Option A — VS Code Copilot (recommended)

Add to your VS Code `settings.json`:

```jsonc
{
  "mcp": {
    "servers": {
      "agent-deploy": {
        "command": "/workspaces/agent-deploy/agent-deploy",
        "env": {
          "AWS_ACCESS_KEY_ID": "AKIA...",
          "AWS_SECRET_ACCESS_KEY": "...",
          "AWS_REGION": "us-east-1",
          "AGENT_DEPLOY_MONTHLY_BUDGET": "20",
          "AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET": "20"
        }
      }
    }
  }
}
```

### Option B — Claude Desktop

Add to `claude_desktop_config.json`:

```jsonc
{
  "mcpServers": {
    "agent-deploy": {
      "command": "/workspaces/agent-deploy/agent-deploy",
      "env": {
        "AWS_ACCESS_KEY_ID": "AKIA...",
        "AWS_SECRET_ACCESS_KEY": "...",
        "AWS_REGION": "us-east-1",
        "AGENT_DEPLOY_MONTHLY_BUDGET": "20",
        "AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET": "20"
      }
    }
  }
}
```

### Verify Connection

Restart/reload the client and confirm the agent lists all 6 tools:

- `aws_plan_infra`
- `aws_approve_plan`
- `aws_create_infra`
- `aws_deploy`
- `aws_status`
- `aws_teardown`

---

## Phase 3: The Happy Path

### Step 1 — Ask the agent to build you a tool

Say to the agent:

> "Build me a simple Go HTTP service that returns the current UTC time as JSON on GET /time. Put it in a Dockerfile."

The agent should produce a `main.go` + `Dockerfile`. Verify it builds locally:

```sh
docker build -t my-time-service .
docker run -p 8080:8080 my-time-service
curl http://localhost:8080/time
```

### Step 2 — Push the image to a registry

You need the image somewhere ECS can pull from. Two options:

- **Public Docker Hub:**
  ```sh
  docker tag my-time-service yourdockerhubuser/time-service:latest
  docker push yourdockerhubuser/time-service:latest
  ```
- **Let agent-deploy handle it:** The `aws_deploy` tool creates an ECR repo — but you'll need to push the image to that ECR repo after infrastructure is created. The agent can guide you through this.

### Step 3 — Ask the agent to plan the deployment

Say to the agent:

> "Deploy my time service container to AWS in us-east-1. I expect about 5 concurrent users and want sub-200ms latency."

The agent should call `aws_plan_infra` and return:

- [ ] A `plan_id`
- [ ] List of services (VPC, ECS, ALB, etc.)
- [ ] Estimated monthly cost (~$50–80 with NAT Gateway + ALB + Fargate)

### Step 4 — Approve the plan

Say to the agent:

> "That cost looks fine, approve it."

- [ ] Agent calls `aws_approve_plan` with `confirmed: true`
- [ ] You get back a confirmation message

### Step 5 — Create infrastructure

Say to the agent:

> "Go ahead and create the infrastructure."

- [ ] Agent calls `aws_create_infra` with the plan ID
- [ ] This takes 2–5 minutes (NAT Gateway creation is slow)
- [ ] You get back an `infra_id`

**⚠️ Checkpoint — verify in AWS Console:**

- [ ] A VPC tagged `managed-by: agent-deploy` exists
- [ ] 4 subnets (2 public, 2 private)
- [ ] An ECS cluster
- [ ] An ALB
- [ ] A NAT Gateway

### Step 6 — Deploy the container

Say to the agent:

> "Deploy the image `yourdockerhubuser/time-service:latest` on port 8080 with health check path `/time`."

- [ ] Agent calls `aws_deploy`
- [ ] Creates ECR repo, registers task definition, starts ECS service, waits for healthy
- [ ] You get back a `deployment_id`

**Note:** If using a Docker Hub public image, it should work directly. If using a private image, you may need to push to the ECR repo that gets created.

### Step 7 — Check status

Say to the agent:

> "What's the status of my deployment?"

- [ ] Agent calls `aws_status`
- [ ] Returns deployment status and public ALB URL(s)

### Step 8 — Hit the live endpoint

```sh
curl http://<alb-url>/time
```

- [ ] You get your JSON time response

---

## Phase 4: Teardown (critical — do NOT skip)

Say to the agent:

> "Tear down my deployment and all associated infrastructure."

- [ ] Agent calls `aws_teardown` with the deployment ID
- [ ] Deletes resources in reverse order: auto-scaling → ECS service → task definition → ECR repo → ALB → ECS cluster → NAT Gateway → subnets → security groups → route tables → Internet Gateway → VPC → IAM role

**⚠️ Verify in AWS Console:**

- [ ] No VPCs tagged `managed-by: agent-deploy` remain
- [ ] No running ECS clusters or services
- [ ] No ALBs
- [ ] No NAT Gateways (cost ~$0.045/hr even when idle)
- [ ] No Elastic IPs (cost $0.005/hr when unattached)

---

## Phase 5: Edge Case Tests

| # | Test | What to say | Expected behavior |
|---|------|------------|-------------------|
| 1 | **Reject a plan** | "Actually, reject that plan." | Agent calls `aws_approve_plan` with `confirmed: false`; plan marked rejected |
| 2 | **Budget exceeded** | Set `AGENT_DEPLOY_PER_DEPLOYMENT_BUDGET=1` and try to deploy | Should block with a budget error before creating resources |
| 3 | **Bad image** | Deploy `nonexistent/image:v999` | Should fail at ECS task startup; verify teardown still works after |
| 4 | **Status after teardown** | "What's the status?" after teardown | Should report deployment not found or torn down |
| 5 | **Double teardown** | Try tearing down the same deployment again | Should handle gracefully, not error on missing resources |

---

## Cost Estimate for This Test

| Resource | Hourly Cost | Created At | Deleted At |
|----------|------------|------------|------------|
| NAT Gateway | ~$0.045/hr | Step 5 | Teardown |
| ALB | ~$0.023/hr | Step 5 | Teardown |
| Fargate task (256 CPU / 512 MB) | ~$0.012/hr | Step 6 | Teardown |
| Elastic IP | ~$0.005/hr | Step 5 | Teardown |

- **Full cycle completed in 1 hour:** ~$0.09
- **Left running overnight (8 hours):** ~$0.70
- **Left running for 24 hours:** ~$2.04

---

## Troubleshooting

| Problem | Likely cause | Fix |
|---------|-------------|-----|
| `aws_plan_infra` returns pricing errors | Missing `pricing:GetProducts` IAM permission, or Pricing API not available in region | Falls back to hardcoded estimates; add the permission for accurate pricing |
| `aws_create_infra` times out | NAT Gateway can take 3–5 min | Wait and retry; check VPC console for gateway status |
| ECS tasks keep restarting | Image can't start, wrong port, health check failing | Check CloudWatch log group `/ecs/agentdeploy-<infra_id>` for container logs |
| ALB returns 502/503 | Tasks not healthy yet, or wrong container port | Verify `container_port` matches what the app listens on; check target group health |
| Teardown fails partway | Resource dependency ordering | Re-run teardown; check AWS Console for remaining resources and delete manually |
| Cost monitor not working | Missing `ce:GetCostAndUsage` permission or not running with `--enable-cost-monitor` | Add the IAM permission; restart with the flag |
