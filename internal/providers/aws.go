// Package providers implements cloud provider integrations for the MCP server.
package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	astypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/cjmartian/agent-deploy/internal/awsclient"
	apperrors "github.com/cjmartian/agent-deploy/internal/errors"
	"github.com/cjmartian/agent-deploy/internal/id"
	"github.com/cjmartian/agent-deploy/internal/logging"
	"github.com/cjmartian/agent-deploy/internal/spending"
	"github.com/cjmartian/agent-deploy/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AWSProvider implements Provider for Amazon Web Services.
type AWSProvider struct {
	store   *state.Store
	clients *awsclient.AWSClients // Optional: injected clients for testing
}

// NewAWSProvider creates a new AWS provider with the given state store.
func NewAWSProvider(store *state.Store) *AWSProvider {
	return &AWSProvider{store: store}
}

// NewAWSProviderWithClients creates a new AWS provider with injected AWS clients.
// This is primarily used for unit testing with mock clients.
func NewAWSProviderWithClients(store *state.Store, clients *awsclient.AWSClients) *AWSProvider {
	return &AWSProvider{store: store, clients: clients}
}

// getClients returns the injected clients if available, otherwise creates real clients from config.
func (p *AWSProvider) getClients(cfg aws.Config) *awsclient.AWSClients {
	if p.clients != nil {
		return p.clients
	}
	return &awsclient.AWSClients{
		EC2:            ec2.NewFromConfig(cfg),
		ECS:            ecs.NewFromConfig(cfg),
		ELBV2:          elbv2.NewFromConfig(cfg),
		IAM:            iam.NewFromConfig(cfg),
		ECR:            ecr.NewFromConfig(cfg),
		CloudWatchLogs: cloudwatchlogs.NewFromConfig(cfg),
		AutoScaling:    applicationautoscaling.NewFromConfig(cfg),
		ACM:            acm.NewFromConfig(cfg),
	}
}

func (p *AWSProvider) Name() string { return "aws" }

// Teardown tears down all AWS resources for a deployment.
// This is the public API for programmatic teardown (e.g., auto-teardown from cost monitor).
func (p *AWSProvider) Teardown(ctx context.Context, deploymentID string) error {
	_, _, err := p.teardown(ctx, nil, teardownInput{DeploymentID: deploymentID})
	return err
}

// ---------------------------------------------------------------------------
// Tools
// ---------------------------------------------------------------------------

func (p *AWSProvider) RegisterTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "aws_plan_infra",
		Description: "Analyze application requirements and propose an AWS infrastructure plan with cost estimate",
	}, p.planInfra)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "aws_approve_plan",
		Description: "Approve or reject an infrastructure plan after reviewing its cost estimate",
	}, p.approvePlan)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "aws_create_infra",
		Description: "Provision AWS infrastructure according to an approved plan",
	}, p.createInfra)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "aws_deploy",
		Description: "Deploy an application onto provisioned AWS infrastructure",
	}, p.deploy)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "aws_status",
		Description: "Get the current status and public URLs of a deployment",
	}, p.status)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "aws_teardown",
		Description: "Tear down all AWS resources for a deployment",
	}, p.teardown)
}

// --- tool input / output types ---

type planInfraInput struct {
	AppDescription string `json:"app_description" jsonschema:"description of the application to deploy"`
	ExpectedUsers  int    `json:"expected_users"   jsonschema:"estimated number of concurrent users"`
	LatencyMS      int    `json:"latency_ms"       jsonschema:"target p99 latency in milliseconds"`
	Region         string `json:"region"           jsonschema:"preferred AWS region (e.g. us-east-1)"`
}

type planInfraOutput struct {
	PlanID          string   `json:"plan_id"`
	Services        []string `json:"services"`
	EstimatedCostMo string   `json:"estimated_cost_monthly"`
	Summary         string   `json:"summary"`
}

type createInfraInput struct {
	PlanID           string `json:"plan_id"             jsonschema:"the plan ID returned by aws_plan_infra"`
	LogRetentionDays int    `json:"log_retention_days,omitempty" jsonschema:"CloudWatch log retention in days (default: 7). Valid: 1,3,5,7,14,30,60,90,120,150,180,365,400,545,731,1096,1827,2192,2557,2922,3288,3653"`
	CertificateARN   string `json:"certificate_arn,omitempty" jsonschema:"ACM certificate ARN for HTTPS (optional). When provided, creates HTTPS listener on port 443 and redirects HTTP to HTTPS"`
}

type createInfraOutput struct {
	InfraID string `json:"infra_id"`
	Status  string `json:"status"`
}

type deployInput struct {
	InfraID         string            `json:"infra_id"             jsonschema:"infrastructure ID from aws_create_infra"`
	ImageRef        string            `json:"image_ref"            jsonschema:"container image reference (required, no default)"`
	ContainerPort   int               `json:"container_port,omitempty"   jsonschema:"container port (default: 80)"`
	HealthCheckPath string            `json:"health_check_path,omitempty" jsonschema:"ALB health check path (default: /)"`
	DesiredCount    int               `json:"desired_count,omitempty"    jsonschema:"number of tasks to run (default: 1)"`
	Environment     map[string]string `json:"environment,omitempty"      jsonschema:"environment variables for the container"`
	CPU             string            `json:"cpu,omitempty"              jsonschema:"ECS task CPU units (default: 256). Valid: 256,512,1024,2048,4096"`
	Memory          string            `json:"memory,omitempty"           jsonschema:"ECS task memory in MB (default: 512). Must be compatible with CPU"`
	// Auto Scaling parameters (per spec ralph/specs/auto-scaling.md).
	MinCount          int `json:"min_count,omitempty"           jsonschema:"minimum task count for auto scaling (default: same as desired_count)"`
	MaxCount          int `json:"max_count,omitempty"           jsonschema:"maximum task count for auto scaling (default: same as desired_count, no scaling)"`
	TargetCPUPercent  int `json:"target_cpu_percent,omitempty"  jsonschema:"target CPU utilization percentage for scaling (default: 70)"`
	TargetMemPercent  int `json:"target_memory_percent,omitempty" jsonschema:"target memory utilization percentage for scaling (default: 70)"`
}

type deployOutput struct {
	DeploymentID string `json:"deployment_id"`
	Status       string `json:"status"`
}

type statusInput struct {
	DeploymentID string `json:"deployment_id" jsonschema:"deployment ID from aws_deploy"`
}

// scalingInfo contains auto scaling configuration details.
type scalingInfo struct {
	MinCount          int `json:"min_count"`
	MaxCount          int `json:"max_count"`
	CurrentCount      int `json:"current_count"`
	TargetCPUPercent  int `json:"target_cpu_percent"`
	TargetMemPercent  int `json:"target_memory_percent"`
}

type statusOutput struct {
	DeploymentID string       `json:"deployment_id"`
	Status       string       `json:"status"`
	URLs         []string     `json:"urls"`
	Scaling      *scalingInfo `json:"scaling,omitempty"`
}

type teardownInput struct {
	DeploymentID string `json:"deployment_id" jsonschema:"deployment ID to tear down"`
}

type teardownOutput struct {
	DeploymentID string `json:"deployment_id"`
	Status       string `json:"status"`
}

type approvePlanInput struct {
	PlanID    string `json:"plan_id"   jsonschema:"plan ID to approve or reject"`
	Confirmed bool   `json:"confirmed" jsonschema:"must be true to approve the plan; false rejects it"`
}

type approvePlanOutput struct {
	PlanID  string `json:"plan_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// --- tool handlers ---

// planInfra analyzes requirements and creates an infrastructure plan with cost estimate.
func (p *AWSProvider) planInfra(ctx context.Context, _ *mcp.CallToolRequest, in planInfraInput) (*mcp.CallToolResult, planInfraOutput, error) {
	// Validate input.
	if strings.TrimSpace(in.AppDescription) == "" {
		return nil, planInfraOutput{}, fmt.Errorf("app_description is required and cannot be empty")
	}
	if strings.TrimSpace(in.Region) == "" {
		return nil, planInfraOutput{}, fmt.Errorf("region is required and cannot be empty")
	}
	if in.ExpectedUsers <= 0 {
		return nil, planInfraOutput{}, fmt.Errorf("expected_users must be a positive integer, got %d", in.ExpectedUsers)
	}
	if in.LatencyMS <= 0 {
		return nil, planInfraOutput{}, fmt.Errorf("latency_ms must be a positive integer, got %d", in.LatencyMS)
	}

	// Select services based on requirements.
	services := []string{"VPC", "ECS Fargate", "ALB", "CloudWatch Logs"}
	if in.ExpectedUsers > 1000 {
		services = append(services, "Auto Scaling")
	}

	// Per spec ralph/specs/cost-estimation.md: Use PricingEstimator for accurate cost estimates.
	// Falls back to hardcoded regional estimates if Pricing API is unavailable.
	estimator, err := spending.NewPricingEstimator(ctx)
	var costEstimate *spending.CostEstimate
	if err != nil {
		slog.Warn("could not create pricing estimator, using simplified estimate",
			slog.String("component", "aws_plan_infra"),
			logging.Err(err))
		// Fallback to simplified calculation.
		baseCost := 15.0
		ecsCost := float64(in.ExpectedUsers) * 0.02
		if in.LatencyMS < 100 {
			ecsCost *= 1.5
		}
		albCost := 20.0
		costEstimate = &spending.CostEstimate{
			TotalMonthlyUSD: baseCost + ecsCost + albCost,
			Region:          in.Region,
			UsingFallback:   true,
			Disclaimer:      "Using simplified cost estimate (pricing service unavailable).",
		}
	} else {
		// Estimate costs with proper pricing.
		costEstimate, err = estimator.EstimateCosts(ctx, spending.EstimateParams{
			Region:            in.Region,
			CPUUnits:          256,  // Default for planning
			MemoryMB:          512,  // Default for planning
			DesiredCount:      1,
			ExpectedUsers:     in.ExpectedUsers,
			IncludeNATGateway: true, // Private subnets are now standard
			LogRetentionDays:  7,
		})
		if err != nil {
			slog.Warn("cost estimation failed, using simplified estimate",
				slog.String("component", "aws_plan_infra"),
				logging.Err(err))
			baseCost := 15.0
			ecsCost := float64(in.ExpectedUsers) * 0.02
			albCost := 20.0
			costEstimate = &spending.CostEstimate{
				TotalMonthlyUSD: baseCost + ecsCost + albCost,
				Region:          in.Region,
				UsingFallback:   true,
				Disclaimer:      "Using simplified cost estimate.",
			}
		}
	}

	estimatedCost := costEstimate.TotalMonthlyUSD

	// Check spending limits before creating plan.
	limits, _ := spending.LoadLimits()
	if estimatedCost > limits.PerDeploymentUSD {
		return nil, planInfraOutput{}, fmt.Errorf("estimated cost $%.2f/mo exceeds per-deployment limit of $%.2f", estimatedCost, limits.PerDeploymentUSD)
	}

	// Create and persist plan.
	plan := &state.Plan{
		ID:              id.NewPlan(),
		AppDescription:  in.AppDescription,
		ExpectedUsers:   in.ExpectedUsers,
		LatencyMS:       in.LatencyMS,
		Region:          in.Region,
		Services:        services,
		EstimatedCostMo: estimatedCost,
		Status:          state.PlanStatusCreated,
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}

	if err := p.store.CreatePlan(plan); err != nil {
		return nil, planInfraOutput{}, fmt.Errorf("save plan: %w", err)
	}

	slog.Info("created plan",
		slog.String("component", "aws_plan_infra"),
		logging.PlanID(plan.ID),
		slog.String("app_description", in.AppDescription),
		logging.Cost(estimatedCost),
		slog.Bool("using_fallback_pricing", costEstimate.UsingFallback))

	// Build detailed summary including cost breakdown.
	summaryBuilder := fmt.Sprintf(
		"Proposed plan for %q: ECS Fargate in %s, targeting %d users at ≤%dms p99. Estimated cost: $%.2f/mo. Plan ID: %s (expires in 24h).\n\n",
		in.AppDescription, in.Region, in.ExpectedUsers, in.LatencyMS, estimatedCost, plan.ID,
	)
	if len(costEstimate.Services) > 0 {
		summaryBuilder += "Cost breakdown:\n"
		for _, svc := range costEstimate.Services {
			if svc.MonthlyCost > 0 {
				summaryBuilder += fmt.Sprintf("  - %s (%s): $%.2f/mo\n", svc.Service, svc.Description, svc.MonthlyCost)
			}
		}
		summaryBuilder += "\n"
	}
	if costEstimate.Disclaimer != "" {
		summaryBuilder += "Note: " + costEstimate.Disclaimer + "\n\n"
	}
	summaryBuilder += "⚠️ Review the cost estimate above. Call aws_approve_plan with plan_id and confirmed: true to approve, then aws_create_infra to provision infrastructure."

	return nil, planInfraOutput{
		PlanID:          plan.ID,
		Services:        services,
		EstimatedCostMo: fmt.Sprintf("$%.2f", estimatedCost),
		Summary:         summaryBuilder,
	}, nil
}

// approvePlan allows the user to approve or reject an infrastructure plan after review.
// Per spec ralph/specs/plan-approval.md: explicit approval is required before provisioning.
func (p *AWSProvider) approvePlan(_ context.Context, _ *mcp.CallToolRequest, in approvePlanInput) (*mcp.CallToolResult, approvePlanOutput, error) {
	// Validate input.
	if strings.TrimSpace(in.PlanID) == "" {
		return nil, approvePlanOutput{}, fmt.Errorf("plan_id is required and cannot be empty")
	}

	// Get plan to verify it exists and include cost info in response.
	plan, err := p.store.GetPlan(in.PlanID)
	if err != nil {
		return nil, approvePlanOutput{}, err
	}

	if in.Confirmed {
		// Approve the plan.
		if err := p.store.ApprovePlan(in.PlanID); err != nil {
			return nil, approvePlanOutput{}, err
		}

		slog.Info("approved plan",
			slog.String("component", "aws_approve_plan"),
			logging.PlanID(in.PlanID),
			logging.Cost(plan.EstimatedCostMo))

		return nil, approvePlanOutput{
			PlanID:  in.PlanID,
			Status:  state.PlanStatusApproved,
			Message: fmt.Sprintf("Plan approved. Estimated monthly cost: $%.2f. You can now call aws_create_infra with plan_id %q to provision infrastructure.", plan.EstimatedCostMo, in.PlanID),
		}, nil
	}

	// Reject the plan.
	if err := p.store.RejectPlan(in.PlanID); err != nil {
		return nil, approvePlanOutput{}, err
	}

	slog.Info("rejected plan",
		slog.String("component", "aws_approve_plan"),
		logging.PlanID(in.PlanID))

	return nil, approvePlanOutput{
		PlanID:  in.PlanID,
		Status:  state.PlanStatusRejected,
		Message: "Plan rejected. Create a new plan with aws_plan_infra if needed.",
	}, nil
}

// createInfra provisions AWS infrastructure according to an approved plan.
func (p *AWSProvider) createInfra(ctx context.Context, _ *mcp.CallToolRequest, in createInfraInput) (*mcp.CallToolResult, createInfraOutput, error) {
	// Get and validate plan.
	plan, err := p.store.GetPlan(in.PlanID)
	if err != nil {
		return nil, createInfraOutput{}, err
	}

	// Require explicit plan approval — no auto-approval.
	// Per spec ralph/specs/plan-approval.md: users must review cost estimates before provisioning.
	if plan.Status != state.PlanStatusApproved {
		return nil, createInfraOutput{}, fmt.Errorf(
			"%w: plan %s is in '%s' status, must be 'approved'. "+
				"Review the plan and call aws_approve_plan with confirmed: true to approve it",
			apperrors.ErrPlanNotApproved, plan.ID, plan.Status,
		)
	}

	// Check spending limits.
	// Per spec ralph/specs/cost-estimation.md: Use CostTracker for actual spend, with fallback.
	limits, _ := spending.LoadLimits()

	// Try to get actual spend from Cost Explorer.
	cfg, err := awsclient.LoadConfig(ctx, plan.Region)
	if err != nil {
		return nil, createInfraOutput{}, err
	}

	var currentSpend float64
	costTracker := spending.NewCostTracker(cfg)
	actualSpend, costErr := costTracker.GetTotalMonthlySpend(ctx)
	if costErr != nil {
		// Fall back to estimate from local state if Cost Explorer unavailable.
		// Per spec: Log warning but don't block provisioning.
		slog.Warn("could not query Cost Explorer, using local estimate",
			slog.String("component", "aws_create_infra"),
			logging.Err(costErr))

		// Sum estimated costs from running deployments.
		deployments, _ := p.store.ListDeployments()
		plans, _ := p.store.ListPlans()
		planCosts := make(map[string]float64)
		for _, pl := range plans {
			planCosts[pl.ID] = pl.EstimatedCostMo
		}

		for _, d := range deployments {
			if d.Status == state.DeploymentStatusRunning {
				// Get infrastructure to find plan, then get estimated cost.
				infra, infraErr := p.store.GetInfra(d.InfraID)
				if infraErr == nil && infra != nil {
					if cost, ok := planCosts[infra.PlanID]; ok {
						currentSpend += cost
					} else {
						// Fallback if plan not found.
						currentSpend += 25.0
					}
				} else {
					currentSpend += 25.0
				}
			}
		}
	} else {
		// Use projected monthly cost from Cost Explorer.
		currentSpend = actualSpend.ProjectedMonthUSD
		slog.Info("retrieved actual spend from Cost Explorer",
			slog.String("component", "aws_create_infra"),
			slog.Float64("current_spend_usd", currentSpend),
			slog.Float64("total_cost_usd", actualSpend.TotalCostUSD),
			slog.Float64("projected_month_usd", actualSpend.ProjectedMonthUSD))
	}

	check := spending.CheckBudget(plan.EstimatedCostMo, limits, currentSpend)
	if !check.Allowed {
		return nil, createInfraOutput{}, fmt.Errorf("%w: %s", apperrors.ErrBudgetExceeded, check.Reason)
	}

	// Create infrastructure record.
	infraID := id.NewInfra()
	infra := &state.Infrastructure{
		ID:        infraID,
		PlanID:    plan.ID,
		Region:    plan.Region,
		Resources: make(map[string]string),
		Status:    state.InfraStatusProvisioning,
		CreatedAt: time.Now(),
	}
	if err := p.store.CreateInfra(infra); err != nil {
		return nil, createInfraOutput{}, fmt.Errorf("save infra: %w", err)
	}

	tags := awsclient.ResourceTags(plan.ID, infraID, "")

	// Provision resources in order. On failure, rollback already-created resources.
	// WHY: Per spec ralph/specs/error-handling.md - partial failures must clean up
	// to prevent orphaned AWS resources and unexpected costs.
	if err := p.provisionVPC(ctx, cfg, infra, tags); err != nil {
		rollbackErr := p.rollbackInfra(ctx, cfg, infra)
		if rollbackErr != nil {
			slog.Error("rollback failed after VPC error", logging.Err(rollbackErr), logging.InfraID(infraID))
		}
		if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
			slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
		}
		return nil, createInfraOutput{}, fmt.Errorf("%w: provision VPC: %w", apperrors.ErrProvisioningFailed, err)
	}

	if err := p.provisionECSCluster(ctx, cfg, infra, tags); err != nil {
		rollbackErr := p.rollbackInfra(ctx, cfg, infra)
		if rollbackErr != nil {
			slog.Error("rollback failed after ECS cluster error", logging.Err(rollbackErr), logging.InfraID(infraID))
		}
		if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
			slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
		}
		return nil, createInfraOutput{}, fmt.Errorf("%w: provision ECS cluster: %w", apperrors.ErrProvisioningFailed, err)
	}

	// Validate certificate if provided (before creating ALB with HTTPS listener).
	if in.CertificateARN != "" {
		if err := p.validateCertificate(ctx, cfg, in.CertificateARN); err != nil {
			rollbackErr := p.rollbackInfra(ctx, cfg, infra)
			if rollbackErr != nil {
				slog.Error("rollback failed after certificate validation error", logging.Err(rollbackErr), logging.InfraID(infraID))
			}
			if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
				slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
			}
			return nil, createInfraOutput{}, fmt.Errorf("%w: validate certificate: %w", apperrors.ErrProvisioningFailed, err)
		}
	}

	if err := p.provisionALB(ctx, cfg, infra, tags, in.CertificateARN); err != nil {
		rollbackErr := p.rollbackInfra(ctx, cfg, infra)
		if rollbackErr != nil {
			slog.Error("rollback failed after ALB error", logging.Err(rollbackErr), logging.InfraID(infraID))
		}
		if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
			slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
		}
		return nil, createInfraOutput{}, fmt.Errorf("%w: provision ALB: %w", apperrors.ErrProvisioningFailed, err)
	}

	// Create IAM execution role for ECS tasks (needed before tasks can run).
	if err := p.provisionExecutionRole(ctx, cfg, infra, tags); err != nil {
		rollbackErr := p.rollbackInfra(ctx, cfg, infra)
		if rollbackErr != nil {
			slog.Error("rollback failed after execution role error", logging.Err(rollbackErr), logging.InfraID(infraID))
		}
		if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
			slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
		}
		return nil, createInfraOutput{}, fmt.Errorf("%w: provision execution role: %w", apperrors.ErrProvisioningFailed, err)
	}

	// Create CloudWatch log group for ECS task logs.
	if err := p.provisionLogGroup(ctx, cfg, infra, tags, in.LogRetentionDays); err != nil {
		rollbackErr := p.rollbackInfra(ctx, cfg, infra)
		if rollbackErr != nil {
			slog.Error("rollback failed after log group error", logging.Err(rollbackErr), logging.InfraID(infraID))
		}
		if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
			slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
		}
		return nil, createInfraOutput{}, fmt.Errorf("%w: provision log group: %w", apperrors.ErrProvisioningFailed, err)
	}

	// Mark infrastructure as ready.
	if err := p.store.SetInfraStatus(infraID, state.InfraStatusReady); err != nil {
		return nil, createInfraOutput{}, err
	}

	slog.Info("infrastructure ready",
		slog.String("component", "aws_create_infra"),
		logging.InfraID(infraID),
		logging.Region(plan.Region))

	return nil, createInfraOutput{
		InfraID: infraID,
		Status:  "ready",
	}, nil
}

// deploy deploys an application onto provisioned infrastructure.
func (p *AWSProvider) deploy(ctx context.Context, _ *mcp.CallToolRequest, in deployInput) (*mcp.CallToolResult, deployOutput, error) {
	// Get infrastructure.
	infra, err := p.store.GetInfra(in.InfraID)
	if err != nil {
		return nil, deployOutput{}, err
	}
	if infra.Status != state.InfraStatusReady {
		return nil, deployOutput{}, apperrors.ErrInfraNotReady
	}

	// Apply defaults for optional parameters.
	containerPort := in.ContainerPort
	if containerPort == 0 {
		containerPort = 80
	}
	healthCheckPath := in.HealthCheckPath
	if healthCheckPath == "" {
		healthCheckPath = "/"
	}
	desiredCount := in.DesiredCount
	if desiredCount == 0 {
		desiredCount = 1
	}

	// Auto-scaling defaults (per spec ralph/specs/auto-scaling.md).
	minCount := in.MinCount
	if minCount == 0 {
		minCount = desiredCount
	}
	maxCount := in.MaxCount
	if maxCount == 0 {
		maxCount = desiredCount // No scaling by default.
	}
	targetCPU := in.TargetCPUPercent
	if targetCPU == 0 {
		targetCPU = 70
	}
	targetMem := in.TargetMemPercent
	if targetMem == 0 {
		targetMem = 70
	}

	// Validate auto-scaling parameters.
	if err := validateAutoScalingParams(minCount, maxCount, targetCPU, targetMem); err != nil {
		return nil, deployOutput{}, err
	}

	// Load AWS config.
	cfg, err := awsclient.LoadConfig(ctx, infra.Region)
	if err != nil {
		return nil, deployOutput{}, err
	}

	// Create deployment record.
	deployID := id.NewDeploy()
	deployment := &state.Deployment{
		ID:          deployID,
		InfraID:     infra.ID,
		ImageRef:    in.ImageRef,
		Status:      state.DeploymentStatusDeploying,
		URLs:        []string{},
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
	}
	if err = p.store.CreateDeployment(deployment); err != nil {
		return nil, deployOutput{}, fmt.Errorf("save deployment: %w", err)
	}

	tags := awsclient.ResourceTags("", infra.ID, deployID)

	// Create ECR repository if needed.
	if err = p.ensureECRRepository(ctx, cfg, infra, deployID, tags); err != nil {
		if statusErr := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusFailed, nil); statusErr != nil {
			slog.Error("failed to update deployment status", "deployID", deployID, "error", statusErr)
		}
		return nil, deployOutput{}, fmt.Errorf("ECR setup: %w", err)
	}

	// Create ECS task definition.
	taskDefARN, err := p.createTaskDefinition(ctx, cfg, infra, in.ImageRef, deployID, containerPort, in.Environment, in.CPU, in.Memory)
	if err != nil {
		if statusErr := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusFailed, nil); statusErr != nil {
			slog.Error("failed to update deployment status", "deployID", deployID, "error", statusErr)
		}
		return nil, deployOutput{}, fmt.Errorf("task definition: %w", err)
	}

	// Update target group health check settings for this deployment.
	if err = p.updateTargetGroupHealthCheck(ctx, cfg, infra, healthCheckPath, containerPort); err != nil {
		if statusErr := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusFailed, nil); statusErr != nil {
			slog.Error("failed to update deployment status", "deployID", deployID, "error", statusErr)
		}
		return nil, deployOutput{}, fmt.Errorf("target group health check: %w", err)
	}

	// Create or update ECS service.
	serviceARN, err := p.createOrUpdateService(ctx, cfg, infra, taskDefARN, deployID, containerPort, desiredCount)
	if err != nil {
		if statusErr := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusFailed, nil); statusErr != nil {
			slog.Error("failed to update deployment status", "deployID", deployID, "error", statusErr)
		}
		return nil, deployOutput{}, fmt.Errorf("ECS service: %w", err)
	}

	// Configure auto-scaling if max_count > desired_count (per spec ralph/specs/auto-scaling.md).
	if maxCount > desiredCount {
		clusterName := extractClusterName(infra.Resources[state.ResourceECSCluster])
		serviceName := extractServiceName(serviceARN)
		if err = p.configureAutoScaling(ctx, cfg, clusterName, serviceName, deployID, minCount, maxCount, targetCPU, targetMem); err != nil {
			// Log but don't fail deployment - auto-scaling is optional enhancement.
			slog.Warn("failed to configure auto-scaling",
				slog.String("component", "aws_deploy"),
				logging.DeploymentID(deployID),
				logging.Err(err))
		} else {
			slog.Info("auto-scaling configured",
				slog.String("component", "aws_deploy"),
				logging.DeploymentID(deployID),
				slog.Int("min_count", minCount),
				slog.Int("max_count", maxCount),
				slog.Int("target_cpu_percent", targetCPU),
				slog.Int("target_memory_percent", targetMem))
		}
	}

	// Get ALB DNS name for URL.
	urls, err := p.getALBURLs(ctx, cfg, infra)
	if err != nil {
		slog.Warn("could not get ALB URLs",
			slog.String("component", "aws_deploy"),
			logging.Err(err))
	}

	// Update deployment with URLs and status.
	if err := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusRunning, urls); err != nil {
		return nil, deployOutput{}, err
	}

	// Store task def and service ARNs.
	deployment.TaskDefARN = taskDefARN
	deployment.ServiceARN = serviceARN

	// Wait for deployment to become healthy (default 5 minute timeout).
	if err := p.waitForHealthyDeployment(ctx, cfg, infra, deployment, 5*time.Minute); err != nil {
		slog.Warn("deployment may not be fully healthy", logging.Err(err), logging.DeploymentID(deployID))
		// Don't fail - deployment was created, just might not be healthy yet.
	}

	slog.Info("deployment running",
		slog.String("component", "aws_deploy"),
		logging.DeploymentID(deployID),
		slog.Any("urls", urls))

	return nil, deployOutput{
		DeploymentID: deployID,
		Status:       "running",
	}, nil
}

// status gets the current status of a deployment.
func (p *AWSProvider) status(ctx context.Context, _ *mcp.CallToolRequest, in statusInput) (*mcp.CallToolResult, statusOutput, error) {
	deployment, err := p.store.GetDeployment(in.DeploymentID)
	if err != nil {
		return nil, statusOutput{}, err
	}

	infra, err := p.store.GetInfra(deployment.InfraID)
	if err != nil {
		return nil, statusOutput{}, err
	}

	var scaling *scalingInfo

	// Try to get live status from AWS.
	cfg, err := awsclient.LoadConfig(ctx, infra.Region)
	if err == nil {
		// Get ECS service status.
		clients := p.getClients(cfg)
		ecsClient := clients.ECS
		clusterARN := infra.Resources[state.ResourceECSCluster]
		if clusterARN != "" {
			resp, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
				Cluster:  aws.String(clusterARN),
				Services: []string{deployment.ServiceARN},
			})
			if err == nil && len(resp.Services) > 0 {
				svc := resp.Services[0]
				if svc.RunningCount > 0 {
					deployment.Status = state.DeploymentStatusRunning
				} else if svc.DesiredCount > 0 {
					deployment.Status = state.DeploymentStatusDeploying
				}

				// Get scaling info if configured.
				clusterName := extractClusterName(clusterARN)
				serviceName := extractServiceName(deployment.ServiceARN)
				if info, err := p.getScalingInfo(ctx, cfg, clusterName, serviceName, int(svc.RunningCount)); err == nil && info != nil {
					scaling = info
				}
			}
		}

		// Refresh URLs.
		urls, err := p.getALBURLs(ctx, cfg, infra)
		if err == nil && len(urls) > 0 {
			deployment.URLs = urls
		}
	}

	return nil, statusOutput{
		DeploymentID: deployment.ID,
		Status:       deployment.Status,
		URLs:         deployment.URLs,
		Scaling:      scaling,
	}, nil
}

// teardown tears down all AWS resources for a deployment.
func (p *AWSProvider) teardown(ctx context.Context, _ *mcp.CallToolRequest, in teardownInput) (*mcp.CallToolResult, teardownOutput, error) {
	deployment, err := p.store.GetDeployment(in.DeploymentID)
	if err != nil {
		return nil, teardownOutput{}, err
	}

	infra, err := p.store.GetInfra(deployment.InfraID)
	if err != nil {
		return nil, teardownOutput{}, err
	}

	cfg, err := awsclient.LoadConfig(ctx, infra.Region)
	if err != nil {
		return nil, teardownOutput{}, err
	}

	// Delete auto-scaling configuration BEFORE deleting ECS service.
	// Per spec ralph/specs/auto-scaling.md: must deregister scalable target first.
	clusterName := extractClusterName(infra.Resources[state.ResourceECSCluster])
	serviceName := extractServiceName(deployment.ServiceARN)
	if clusterName != "" && serviceName != "" {
		if err := p.deleteAutoScaling(ctx, cfg, clusterName, serviceName, in.DeploymentID); err != nil {
			slog.Warn("failed to delete auto-scaling",
				slog.String("component", "aws_teardown"),
				logging.Err(err))
		}
	}

	// Delete ECS service.
	if err := p.deleteECSService(ctx, cfg, infra, deployment); err != nil {
		slog.Warn("failed to delete ECS service",
			slog.String("component", "aws_teardown"),
			logging.Err(err))
	}

	// Delete ECS cluster.
	if err := p.deleteECSCluster(ctx, cfg, infra); err != nil {
		slog.Warn("failed to delete ECS cluster",
			slog.String("component", "aws_teardown"),
			logging.Err(err))
	}

	// Delete ALB and target group.
	if err := p.deleteALB(ctx, cfg, infra); err != nil {
		slog.Warn("failed to delete ALB",
			slog.String("component", "aws_teardown"),
			logging.Err(err))
	}

	// Delete ECR repository.
	if err := p.deleteECRRepository(ctx, cfg, infra); err != nil {
		slog.Warn("failed to delete ECR repository",
			slog.String("component", "aws_teardown"),
			logging.Err(err))
	}

	// Delete CloudWatch log group.
	if err := p.deleteLogGroup(ctx, cfg, infra); err != nil {
		slog.Warn("failed to delete log group",
			slog.String("component", "aws_teardown"),
			logging.Err(err))
	}

	// Delete IAM execution role.
	if err := p.deleteExecutionRole(ctx, cfg, infra); err != nil {
		slog.Warn("failed to delete execution role",
			slog.String("component", "aws_teardown"),
			logging.Err(err))
	}

	// Delete VPC resources (security groups, subnets, internet gateway, VPC).
	if err := p.deleteVPCResources(ctx, cfg, infra); err != nil {
		slog.Warn("failed to delete VPC resources",
			slog.String("component", "aws_teardown"),
			logging.Err(err))
	}

	// Update state (best-effort cleanup, log errors but continue).
	if err := p.store.UpdateDeploymentStatus(in.DeploymentID, state.DeploymentStatusStopped, nil); err != nil {
		slog.Error("failed to update deployment status during teardown", "deploymentID", in.DeploymentID, "error", err)
	}
	if err := p.store.SetInfraStatus(infra.ID, state.InfraStatusDestroyed); err != nil {
		slog.Error("failed to set infra status during teardown", "infraID", infra.ID, "error", err)
	}

	slog.Info("deployment torn down",
		slog.String("component", "aws_teardown"),
		logging.DeploymentID(in.DeploymentID))

	return nil, teardownOutput{
		DeploymentID: in.DeploymentID,
		Status:       "destroyed",
	}, nil
}

// ---------------------------------------------------------------------------
// AWS Provisioning Helpers
// ---------------------------------------------------------------------------

func (p *AWSProvider) provisionVPC(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string) error {
	clients := p.getClients(cfg)
	ec2Client := clients.EC2

	// Create VPC.
	// Per spec ralph/specs/networking.md: Default CIDR 10.0.0.0/16
	vpcResp, err := ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String("10.0.0.0/16"),
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeVpc,
			Tags:         mapToEC2Tags(tags),
		}},
	})
	if err != nil {
		return fmt.Errorf("create VPC: %w", err)
	}
	vpcID := *vpcResp.Vpc.VpcId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceVPC, vpcID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceVPC, "error", storeErr)
	}
	infra.Resources[state.ResourceVPC] = vpcID

	// Enable DNS hostnames for ALB.
	_, err = ec2Client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
		VpcId:              aws.String(vpcID),
		EnableDnsHostnames: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
	})
	if err != nil {
		return fmt.Errorf("enable DNS hostnames: %w", err)
	}

	// Create Internet Gateway.
	igwResp, err := ec2Client.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeInternetGateway,
			Tags:         mapToEC2Tags(tags),
		}},
	})
	if err != nil {
		return fmt.Errorf("create IGW: %w", err)
	}
	igwID := *igwResp.InternetGateway.InternetGatewayId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceInternetGateway, igwID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceInternetGateway, "error", storeErr)
	}
	infra.Resources[state.ResourceInternetGateway] = igwID

	// Attach IGW to VPC.
	_, err = ec2Client.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: aws.String(igwID),
		VpcId:             aws.String(vpcID),
	})
	if err != nil {
		return fmt.Errorf("attach IGW: %w", err)
	}

	// Get availability zones.
	azResp, err := ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return fmt.Errorf("describe AZs: %w", err)
	}
	if len(azResp.AvailabilityZones) < 2 {
		return fmt.Errorf("need at least 2 AZs, found %d", len(azResp.AvailabilityZones))
	}

	// Per spec ralph/specs/networking.md: Create 4 subnets (2 public, 2 private) across 2 AZs.
	// Subnet Layout (for default 10.0.0.0/16):
	//   Public A:  10.0.1.0/24 (AZ-1) - ALB, NAT Gateway
	//   Public B:  10.0.2.0/24 (AZ-2) - ALB
	//   Private A: 10.0.10.0/24 (AZ-1) - ECS tasks
	//   Private B: 10.0.11.0/24 (AZ-2) - ECS tasks

	// Create public subnets in 2 AZs (required for ALB).
	var publicSubnetIDs []string
	for i := 0; i < 2; i++ {
		az := *azResp.AvailabilityZones[i].ZoneName
		cidr := fmt.Sprintf("10.0.%d.0/24", i+1)

		subnetResp, err := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            aws.String(vpcID),
			CidrBlock:        aws.String(cidr),
			AvailabilityZone: aws.String(az),
			TagSpecifications: []ec2types.TagSpecification{{
				ResourceType: ec2types.ResourceTypeSubnet,
				Tags:         mapToEC2Tags(mergeTags(tags, map[string]string{"Name": fmt.Sprintf("agent-deploy-public-%d", i+1)})),
			}},
		})
		if err != nil {
			return fmt.Errorf("create public subnet %d: %w", i, err)
		}
		publicSubnetIDs = append(publicSubnetIDs, *subnetResp.Subnet.SubnetId)

		// Enable auto-assign public IP for public subnets.
		_, err = ec2Client.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
			SubnetId:            subnetResp.Subnet.SubnetId,
			MapPublicIpOnLaunch: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("enable public IP for public subnet %d: %w", i, err)
		}
	}
	publicSubnetsStr := publicSubnetIDs[0] + "," + publicSubnetIDs[1]
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceSubnetPublic, publicSubnetsStr); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceSubnetPublic, "error", storeErr)
	}
	infra.Resources[state.ResourceSubnetPublic] = publicSubnetsStr

	// Create private subnets in 2 AZs (for ECS tasks).
	var privateSubnetIDs []string
	for i := 0; i < 2; i++ {
		az := *azResp.AvailabilityZones[i].ZoneName
		cidr := fmt.Sprintf("10.0.%d.0/24", i+10) // 10.0.10.0/24 and 10.0.11.0/24

		subnetResp, err := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            aws.String(vpcID),
			CidrBlock:        aws.String(cidr),
			AvailabilityZone: aws.String(az),
			TagSpecifications: []ec2types.TagSpecification{{
				ResourceType: ec2types.ResourceTypeSubnet,
				Tags:         mapToEC2Tags(mergeTags(tags, map[string]string{"Name": fmt.Sprintf("agent-deploy-private-%d", i+1)})),
			}},
		})
		if err != nil {
			return fmt.Errorf("create private subnet %d: %w", i, err)
		}
		privateSubnetIDs = append(privateSubnetIDs, *subnetResp.Subnet.SubnetId)
		// Private subnets do NOT get auto-assign public IP.
	}
	privateSubnetsStr := privateSubnetIDs[0] + "," + privateSubnetIDs[1]
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceSubnetPrivate, privateSubnetsStr); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceSubnetPrivate, "error", storeErr)
	}
	infra.Resources[state.ResourceSubnetPrivate] = privateSubnetsStr

	// Create public route table with internet route (for public subnets).
	publicRTResp, err := ec2Client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId: aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeRouteTable,
			Tags:         mapToEC2Tags(mergeTags(tags, map[string]string{"Name": "agent-deploy-public"})),
		}},
	})
	if err != nil {
		return fmt.Errorf("create public route table: %w", err)
	}
	publicRTID := *publicRTResp.RouteTable.RouteTableId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceRouteTable, publicRTID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceRouteTable, "error", storeErr)
	}
	infra.Resources[state.ResourceRouteTable] = publicRTID

	// Add route to internet gateway (for public subnets).
	_, err = ec2Client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         aws.String(publicRTID),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(igwID),
	})
	if err != nil {
		return fmt.Errorf("create route to IGW: %w", err)
	}

	// Associate public subnets with public route table.
	for _, subnetID := range publicSubnetIDs {
		_, err = ec2Client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(publicRTID),
			SubnetId:     aws.String(subnetID),
		})
		if err != nil {
			return fmt.Errorf("associate public subnet %s: %w", subnetID, err)
		}
	}

	// Per spec ralph/specs/networking.md: Create NAT Gateway for private subnet egress.
	// Allocate Elastic IP for NAT Gateway.
	eipResp, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
		Domain: ec2types.DomainTypeVpc,
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeElasticIp,
			Tags:         mapToEC2Tags(tags),
		}},
	})
	if err != nil {
		return fmt.Errorf("allocate elastic IP: %w", err)
	}
	eipAllocationID := *eipResp.AllocationId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceElasticIP, eipAllocationID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceElasticIP, "error", storeErr)
	}
	infra.Resources[state.ResourceElasticIP] = eipAllocationID

	// Create NAT Gateway in first public subnet.
	natResp, err := ec2Client.CreateNatGateway(ctx, &ec2.CreateNatGatewayInput{
		SubnetId:     aws.String(publicSubnetIDs[0]),
		AllocationId: aws.String(eipAllocationID),
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeNatgateway,
			Tags:         mapToEC2Tags(tags),
		}},
	})
	if err != nil {
		return fmt.Errorf("create NAT gateway: %w", err)
	}
	natGWID := *natResp.NatGateway.NatGatewayId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceNATGateway, natGWID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceNATGateway, "error", storeErr)
	}
	infra.Resources[state.ResourceNATGateway] = natGWID

	// Wait for NAT Gateway to become available (can take 1-2 minutes).
	slog.Info("waiting for NAT gateway to become available",
		slog.String("component", "provisionVPC"),
		slog.String("nat_gateway_id", natGWID))

	waiter := ec2.NewNatGatewayAvailableWaiter(ec2Client)
	err = waiter.Wait(ctx, &ec2.DescribeNatGatewaysInput{
		NatGatewayIds: []string{natGWID},
	}, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("wait for NAT gateway: %w", err)
	}

	// Create private route table with NAT Gateway route.
	privateRTResp, err := ec2Client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId: aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeRouteTable,
			Tags:         mapToEC2Tags(mergeTags(tags, map[string]string{"Name": "agent-deploy-private"})),
		}},
	})
	if err != nil {
		return fmt.Errorf("create private route table: %w", err)
	}
	privateRTID := *privateRTResp.RouteTable.RouteTableId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceRouteTablePrivate, privateRTID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceRouteTablePrivate, "error", storeErr)
	}
	infra.Resources[state.ResourceRouteTablePrivate] = privateRTID

	// Add route to NAT Gateway (for private subnets).
	_, err = ec2Client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         aws.String(privateRTID),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		NatGatewayId:         aws.String(natGWID),
	})
	if err != nil {
		return fmt.Errorf("create route to NAT gateway: %w", err)
	}

	// Associate private subnets with private route table.
	for _, subnetID := range privateSubnetIDs {
		_, err = ec2Client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(privateRTID),
			SubnetId:     aws.String(subnetID),
		})
		if err != nil {
			return fmt.Errorf("associate private subnet %s: %w", subnetID, err)
		}
	}

	// Per spec ralph/specs/networking.md: Create separate security groups.
	// ALB security group - allows public HTTP/HTTPS inbound.
	albSGResp, err := ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String("agent-deploy-alb-" + infra.ID),
		Description: aws.String("ALB security group - allows HTTP/HTTPS from internet"),
		VpcId:       aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeSecurityGroup,
			Tags:         mapToEC2Tags(mergeTags(tags, map[string]string{"Name": "agent-deploy-alb"})),
		}},
	})
	if err != nil {
		return fmt.Errorf("create ALB security group: %w", err)
	}
	albSGID := *albSGResp.GroupId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceSecurityGroupALB, albSGID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceSecurityGroupALB, "error", storeErr)
	}
	infra.Resources[state.ResourceSecurityGroupALB] = albSGID

	// Allow inbound HTTP (80) and HTTPS (443) to ALB.
	_, err = ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(albSGID),
		IpPermissions: []ec2types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(80),
				ToPort:     aws.Int32(80),
				IpRanges:   []ec2types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
			},
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(443),
				ToPort:     aws.Int32(443),
				IpRanges:   []ec2types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("authorize ALB ingress: %w", err)
	}

	// Task security group - allows inbound only from ALB security group.
	taskSGResp, err := ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String("agent-deploy-task-" + infra.ID),
		Description: aws.String("ECS task security group - allows inbound only from ALB"),
		VpcId:       aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeSecurityGroup,
			Tags:         mapToEC2Tags(mergeTags(tags, map[string]string{"Name": "agent-deploy-task"})),
		}},
	})
	if err != nil {
		return fmt.Errorf("create task security group: %w", err)
	}
	taskSGID := *taskSGResp.GroupId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceSecurityGroupTask, taskSGID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceSecurityGroupTask, "error", storeErr)
	}
	infra.Resources[state.ResourceSecurityGroupTask] = taskSGID

	// Allow inbound from ALB security group on all container ports (1-65535).
	// We use a wide range because container port is configurable.
	_, err = ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(taskSGID),
		IpPermissions: []ec2types.IpPermission{{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(1),
			ToPort:     aws.Int32(65535),
			UserIdGroupPairs: []ec2types.UserIdGroupPair{{
				GroupId: aws.String(albSGID),
			}},
		}},
	})
	if err != nil {
		return fmt.Errorf("authorize task ingress from ALB: %w", err)
	}

	// Store legacy ResourceSecurityGroup as the task SG for backward compatibility.
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceSecurityGroup, taskSGID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceSecurityGroup, "error", storeErr)
	}
	infra.Resources[state.ResourceSecurityGroup] = taskSGID

	slog.Info("VPC with public/private subnets created",
		slog.String("component", "provisionVPC"),
		slog.String("vpc_id", vpcID),
		slog.Any("public_subnets", publicSubnetIDs),
		slog.Any("private_subnets", privateSubnetIDs),
		slog.String("nat_gateway_id", natGWID),
		slog.String("alb_security_group", albSGID),
		slog.String("task_security_group", taskSGID))
	return nil
}

// mergeTags merges two tag maps, with the second map's values overriding the first.
func mergeTags(base, override map[string]string) map[string]string {
	result := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

func (p *AWSProvider) provisionECSCluster(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string) error {
	clients := p.getClients(cfg)
	ecsClient := clients.ECS

	clusterResp, err := ecsClient.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String("agent-deploy-" + infra.ID),
		Tags:        mapToECSTags(tags),
		CapacityProviders: []string{
			"FARGATE", "FARGATE_SPOT",
		},
	})
	if err != nil {
		return fmt.Errorf("create cluster: %w", err)
	}

	clusterARN := *clusterResp.Cluster.ClusterArn
	if err := p.store.UpdateInfraResource(infra.ID, state.ResourceECSCluster, clusterARN); err != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceECSCluster, "error", err)
	}
	infra.Resources[state.ResourceECSCluster] = clusterARN

	slog.Info("ECS cluster created",
		slog.String("component", "provisionECSCluster"),
		slog.String("cluster_arn", clusterARN))
	return nil
}

func (p *AWSProvider) provisionALB(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string, certificateARN string) error {
	clients := p.getClients(cfg)
	elbClient := clients.ELBV2

	// Parse subnet IDs.
	subnetStr := infra.Resources[state.ResourceSubnetPublic]
	var subnetIDs []string
	for _, s := range splitComma(subnetStr) {
		if s != "" {
			subnetIDs = append(subnetIDs, s)
		}
	}

	// Create ALB with ALB security group (allows public HTTP/HTTPS).
	// Per spec ralph/specs/networking.md: ALB uses dedicated ALB security group.
	albSGID := infra.Resources[state.ResourceSecurityGroupALB]
	if albSGID == "" {
		// Fallback to legacy single security group for backward compatibility.
		albSGID = infra.Resources[state.ResourceSecurityGroup]
	}
	albResp, err := elbClient.CreateLoadBalancer(ctx, &elbv2.CreateLoadBalancerInput{
		Name:           aws.String("agent-deploy-" + infra.ID[:8]),
		Subnets:        subnetIDs,
		SecurityGroups: []string{albSGID},
		Scheme:         elbv2types.LoadBalancerSchemeEnumInternetFacing,
		Type:           elbv2types.LoadBalancerTypeEnumApplication,
		Tags:           mapToELBTags(tags),
	})
	if err != nil {
		return fmt.Errorf("create ALB: %w", err)
	}

	albARN := *albResp.LoadBalancers[0].LoadBalancerArn
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceALB, albARN); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceALB, "error", storeErr)
	}
	infra.Resources[state.ResourceALB] = albARN

	// Create target group.
	tgResp, err := elbClient.CreateTargetGroup(ctx, &elbv2.CreateTargetGroupInput{
		Name:       aws.String("agent-deploy-" + infra.ID[:8]),
		Protocol:   elbv2types.ProtocolEnumHttp,
		Port:       aws.Int32(80),
		VpcId:      aws.String(infra.Resources[state.ResourceVPC]),
		TargetType: elbv2types.TargetTypeEnumIp,
		HealthCheckPath: aws.String("/"),
		Tags:       mapToELBTags(tags),
	})
	if err != nil {
		return fmt.Errorf("create target group: %w", err)
	}

	tgARN := *tgResp.TargetGroups[0].TargetGroupArn
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceTargetGroup, tgARN); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceTargetGroup, "error", storeErr)
	}
	infra.Resources[state.ResourceTargetGroup] = tgARN

	// Store TLS configuration in infrastructure resources.
	tlsEnabled := certificateARN != ""
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceTLSEnabled, fmt.Sprintf("%t", tlsEnabled)); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceTLSEnabled, "error", storeErr)
	}
	infra.Resources[state.ResourceTLSEnabled] = fmt.Sprintf("%t", tlsEnabled)

	if certificateARN != "" {
		// Store certificate ARN for reference.
		if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceCertificateARN, certificateARN); storeErr != nil {
			slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceCertificateARN, "error", storeErr)
		}
		infra.Resources[state.ResourceCertificateARN] = certificateARN

		// Create HTTPS listener on port 443 with TLS termination.
		// Per spec ralph/specs/tls-https.md: Use modern TLS policy enforcing TLS 1.2+.
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
			Tags: mapToELBTags(tags),
		})
		if err != nil {
			return fmt.Errorf("create HTTPS listener: %w", err)
		}

		// Create HTTP listener that redirects to HTTPS (per spec).
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
			Tags: mapToELBTags(tags),
		})
		if err != nil {
			return fmt.Errorf("create HTTP redirect listener: %w", err)
		}

		slog.Info("ALB with HTTPS created",
			slog.String("component", "provisionALB"),
			slog.String("alb_arn", albARN),
			slog.String("target_group_arn", tgARN),
			slog.String("certificate_arn", certificateARN))
	} else {
		// No certificate: create HTTP-only listener (forward to target group).
		_, err = elbClient.CreateListener(ctx, &elbv2.CreateListenerInput{
			LoadBalancerArn: aws.String(albARN),
			Protocol:        elbv2types.ProtocolEnumHttp,
			Port:            aws.Int32(80),
			DefaultActions: []elbv2types.Action{{
				Type:           elbv2types.ActionTypeEnumForward,
				TargetGroupArn: aws.String(tgARN),
			}},
			Tags: mapToELBTags(tags),
		})
		if err != nil {
			return fmt.Errorf("create listener: %w", err)
		}

		slog.Info("ALB and target group created",
			slog.String("component", "provisionALB"),
			slog.String("alb_arn", albARN),
			slog.String("target_group_arn", tgARN))
	}

	return nil
}

// provisionLogGroup creates a CloudWatch log group for ECS task logs.
func (p *AWSProvider) provisionLogGroup(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string, logRetentionDays int) error {
	clients := p.getClients(cfg)
	cwlClient := clients.CloudWatchLogs

	logGroupName := "/ecs/agent-deploy-" + infra.ID

	_, err := cwlClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(logGroupName),
		Tags:         tags,
	})
	if err != nil {
		// Check if the log group already exists (ResourceAlreadyExistsException).
		if !strings.Contains(err.Error(), "ResourceAlreadyExistsException") {
			return fmt.Errorf("failed to create log group: %w", err)
		}
		slog.Debug("log group already exists",
			slog.String("component", "provisionLogGroup"))
	}

	// Default to 7 days if not specified. Per spec ralph/specs/deploy-configuration.md.
	if logRetentionDays <= 0 {
		logRetentionDays = 7
	}

	// Set log retention policy.
	_, err = cwlClient.PutRetentionPolicy(ctx, &cloudwatchlogs.PutRetentionPolicyInput{
		LogGroupName:    aws.String(logGroupName),
		RetentionInDays: aws.Int32(int32(logRetentionDays)),
	})
	if err != nil {
		return fmt.Errorf("failed to set retention policy: %w", err)
	}

	if err := p.store.UpdateInfraResource(infra.ID, state.ResourceLogGroup, logGroupName); err != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceLogGroup, "error", err)
	}
	infra.Resources[state.ResourceLogGroup] = logGroupName

	slog.Info("log group created",
		slog.String("component", "provisionLogGroup"),
		slog.String("log_group_name", logGroupName))
	return nil
}

// provisionExecutionRole creates an IAM execution role for ECS Fargate tasks.
// This role allows tasks to pull images from ECR and write logs to CloudWatch.
func (p *AWSProvider) provisionExecutionRole(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string) error {
	clients := p.getClients(cfg)
	iamClient := clients.IAM

	// Create role name, truncated to 64 chars max (IAM role name limit).
	roleName := "agent-deploy-ecs-task-" + infra.ID
	if len(roleName) > 64 {
		roleName = roleName[:64]
	}

	// ECS task assume role policy document.
	assumeRolePolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

	// Create the IAM role.
	createRoleResp, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(assumeRolePolicy),
		Tags:                     mapToIAMTags(tags),
	})
	var roleARN string
	if err != nil {
		// Check if the role already exists (EntityAlreadyExists).
		if !strings.Contains(err.Error(), "EntityAlreadyExists") {
			return fmt.Errorf("failed to create execution role: %w", err)
		}
		slog.Info("execution role already exists, retrieving it", "roleName", roleName)
		// Retrieve the existing role.
		getRoleResp, getErr := iamClient.GetRole(ctx, &iam.GetRoleInput{
			RoleName: aws.String(roleName),
		})
		if getErr != nil {
			return fmt.Errorf("failed to get existing execution role: %w", getErr)
		}
		roleARN = *getRoleResp.Role.Arn
	} else {
		roleARN = *createRoleResp.Role.Arn
	}

	// Attach the AWS managed ECS task execution policy.
	managedPolicyARN := "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
	_, err = iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String(managedPolicyARN),
	})
	if err != nil {
		// Policy might already be attached, which is fine.
		if !strings.Contains(err.Error(), "already attached") && !strings.Contains(err.Error(), "LimitExceeded") {
			return fmt.Errorf("failed to attach execution role policy: %w", err)
		}
		slog.Info("execution role policy already attached or limit reached", "roleName", roleName)
	}

	// Store the role ARN in infrastructure resources.
	if err := p.store.UpdateInfraResource(infra.ID, state.ResourceExecutionRole, roleARN); err != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceExecutionRole, "error", err)
	}
	infra.Resources[state.ResourceExecutionRole] = roleARN

	slog.Info("execution role provisioned", "roleName", roleName, "roleARN", roleARN)
	return nil
}

// mapToIAMTags converts a map of tags to IAM tag format.
func mapToIAMTags(tags map[string]string) []iamtypes.Tag {
	result := make([]iamtypes.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, iamtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return result
}

func (p *AWSProvider) ensureECRRepository(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, deployID string, tags map[string]string) error {
	clients := p.getClients(cfg)
	ecrClient := clients.ECR

	repoName := "agent-deploy-" + deployID[:12]

	_, err := ecrClient.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repoName),
		Tags:           mapToECRTags(tags),
	})
	if err != nil {
		// Check if the repository already exists (RepositoryAlreadyExistsException).
		if !strings.Contains(err.Error(), "RepositoryAlreadyExistsException") {
			return fmt.Errorf("failed to create ECR repository: %w", err)
		}
		slog.Debug("ECR repository already exists",
			slog.String("component", "ensureECRRepository"))
	}

	if err := p.store.UpdateInfraResource(infra.ID, state.ResourceECRRepository, repoName); err != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceECRRepository, "error", err)
	}
	return nil
}

func (p *AWSProvider) createTaskDefinition(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, imageRef, deployID string, containerPort int, environment map[string]string, cpu, memory string) (string, error) {
	clients := p.getClients(cfg)
	ecsClient := clients.ECS

	// Image is required - no default.
	// Per spec ralph/specs/deploy-configuration.md: require explicit image specification.
	if strings.TrimSpace(imageRef) == "" {
		return "", fmt.Errorf("image_ref is required and cannot be empty")
	}
	image := imageRef

	// Get log group name from infrastructure.
	logGroupName := infra.Resources[state.ResourceLogGroup]
	if logGroupName == "" {
		logGroupName = "/ecs/agent-deploy-" + infra.ID
	}

	// Extract region from log group or use a default.
	region := infra.Region
	if region == "" {
		region = "us-east-1"
	}

	// Get execution role ARN from infrastructure.
	executionRoleARN := infra.Resources[state.ResourceExecutionRole]
	if executionRoleARN == "" {
		return "", fmt.Errorf("execution role not found in infrastructure resources")
	}

	// Default CPU/Memory if not specified.
	if cpu == "" {
		cpu = "256"
	}
	if memory == "" {
		memory = "512"
	}

	// Build environment variables for the container.
	envVars := make([]ecstypes.KeyValuePair, 0, len(environment))
	for k, v := range environment {
		envVars = append(envVars, ecstypes.KeyValuePair{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	resp, err := ecsClient.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("agent-deploy-" + deployID[:12]),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String(cpu),
		Memory:                  aws.String(memory),
		ExecutionRoleArn:        aws.String(executionRoleARN),
		ContainerDefinitions: []ecstypes.ContainerDefinition{{
			Name:        aws.String("app"),
			Image:       aws.String(image),
			Essential:   aws.Bool(true),
			Environment: envVars,
			PortMappings: []ecstypes.PortMapping{{
				ContainerPort: aws.Int32(int32(containerPort)),
				Protocol:      ecstypes.TransportProtocolTcp,
			}},
			LogConfiguration: &ecstypes.LogConfiguration{
				LogDriver: ecstypes.LogDriverAwslogs,
				Options: map[string]string{
					"awslogs-group":         logGroupName,
					"awslogs-region":        region,
					"awslogs-stream-prefix": "ecs",
				},
			},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("register task definition: %w", err)
	}

	taskDefARN := *resp.TaskDefinition.TaskDefinitionArn
	slog.Info("task definition created",
		slog.String("component", "createTaskDefinition"),
		slog.String("task_def_arn", taskDefARN),
		slog.String("log_group_name", logGroupName))
	return taskDefARN, nil
}

func (p *AWSProvider) createOrUpdateService(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, taskDefARN, deployID string, containerPort, desiredCount int) (string, error) {
	clients := p.getClients(cfg)
	ecsClient := clients.ECS

	// Per spec ralph/specs/networking.md: ECS tasks run in private subnets.
	// Use private subnets if available, fall back to public for backward compatibility.
	subnetStr := infra.Resources[state.ResourceSubnetPrivate]
	assignPublicIP := ecstypes.AssignPublicIpDisabled
	if subnetStr == "" {
		// Fallback to public subnets (legacy infrastructure).
		subnetStr = infra.Resources[state.ResourceSubnetPublic]
		assignPublicIP = ecstypes.AssignPublicIpEnabled
	}

	var subnetIDs []string
	for _, s := range splitComma(subnetStr) {
		if s != "" {
			subnetIDs = append(subnetIDs, s)
		}
	}

	// Use task security group if available, fall back to legacy security group.
	taskSGID := infra.Resources[state.ResourceSecurityGroupTask]
	if taskSGID == "" {
		taskSGID = infra.Resources[state.ResourceSecurityGroup]
	}

	serviceName := "agent-deploy-" + deployID[:12]

	resp, err := ecsClient.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster:        aws.String(infra.Resources[state.ResourceECSCluster]),
		ServiceName:    aws.String(serviceName),
		TaskDefinition: aws.String(taskDefARN),
		DesiredCount:   aws.Int32(int32(desiredCount)),
		LaunchType:     ecstypes.LaunchTypeFargate,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        subnetIDs,
				SecurityGroups: []string{taskSGID},
				AssignPublicIp: assignPublicIP,
			},
		},
		LoadBalancers: []ecstypes.LoadBalancer{{
			TargetGroupArn: aws.String(infra.Resources[state.ResourceTargetGroup]),
			ContainerName:  aws.String("app"),
			ContainerPort:  aws.Int32(int32(containerPort)),
		}},
	})
	if err != nil {
		return "", fmt.Errorf("create service: %w", err)
	}

	serviceARN := *resp.Service.ServiceArn
	slog.Info("ECS service created",
		slog.String("component", "createOrUpdateService"),
		slog.String("service_arn", serviceARN),
		slog.Any("subnets", subnetIDs),
		slog.String("assign_public_ip", string(assignPublicIP)))
	return serviceARN, nil
}

// waitForHealthyDeployment waits for the ECS service to be running and healthy.
// It polls the service status and ALB target health until healthy or timeout.
func (p *AWSProvider) waitForHealthyDeployment(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, deployment *state.Deployment, timeout time.Duration) error {
	clients := p.getClients(cfg)
	ecsClient := clients.ECS
	elbClient := clients.ELBV2

	clusterARN := infra.Resources[state.ResourceECSCluster]
	targetGroupARN := infra.Resources[state.ResourceTargetGroup]

	if clusterARN == "" || deployment.ServiceARN == "" {
		return fmt.Errorf("missing cluster ARN or service ARN")
	}

	pollInterval := 10 * time.Second
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for healthy deployment after %v", timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check ECS service status.
		ecsResp, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  aws.String(clusterARN),
			Services: []string{deployment.ServiceARN},
		})
		if err != nil {
			slog.Debug("error checking ECS service status",
				slog.String("component", "waitForHealthyDeployment"),
				logging.Err(err))
			time.Sleep(pollInterval)
			continue
		}

		if len(ecsResp.Services) == 0 {
			slog.Debug("no services found",
				slog.String("component", "waitForHealthyDeployment"))
			time.Sleep(pollInterval)
			continue
		}

		svc := ecsResp.Services[0]

		// Check if we have deployments.
		if len(svc.Deployments) == 0 {
			slog.Debug("no deployments found in service",
				slog.String("component", "waitForHealthyDeployment"))
			time.Sleep(pollInterval)
			continue
		}

		primaryDeployment := svc.Deployments[0]
		runningCount := primaryDeployment.RunningCount
		desiredCount := primaryDeployment.DesiredCount
		rolloutState := primaryDeployment.RolloutState

		slog.Debug("ECS deployment status",
			slog.String("component", "waitForHealthyDeployment"),
			slog.Int("running_count", int(runningCount)),
			slog.Int("desired_count", int(desiredCount)),
			slog.String("rollout_state", string(rolloutState)))

		// Check if rollout is completed and running count matches desired.
		ecsHealthy := rolloutState == ecstypes.DeploymentRolloutStateCompleted &&
			runningCount >= desiredCount && desiredCount > 0

		if !ecsHealthy {
			time.Sleep(pollInterval)
			continue
		}

		// ECS is healthy, now check ALB target health.
		if targetGroupARN != "" {
			healthResp, err := elbClient.DescribeTargetHealth(ctx, &elbv2.DescribeTargetHealthInput{
				TargetGroupArn: aws.String(targetGroupARN),
			})
			if err != nil {
				slog.Debug("error checking target health",
					slog.String("component", "waitForHealthyDeployment"),
					logging.Err(err))
				time.Sleep(pollInterval)
				continue
			}

			healthyTargets := 0
			for _, target := range healthResp.TargetHealthDescriptions {
				if target.TargetHealth != nil && target.TargetHealth.State == elbv2types.TargetHealthStateEnumHealthy {
					healthyTargets++
				}
			}

			slog.Debug("ALB target health",
				slog.String("component", "waitForHealthyDeployment"),
				slog.Int("healthy_targets", healthyTargets),
				slog.Int("total_targets", len(healthResp.TargetHealthDescriptions)))

			if healthyTargets == 0 {
				time.Sleep(pollInterval)
				continue
			}
		}

		// Both ECS and ALB are healthy.
		slog.Info("deployment is healthy",
			slog.String("component", "waitForHealthyDeployment"),
			logging.DeploymentID(deployment.ID))
		return nil
	}
}

// updateTargetGroupHealthCheck updates the target group's health check settings.
func (p *AWSProvider) updateTargetGroupHealthCheck(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, healthCheckPath string, containerPort int) error {
	clients := p.getClients(cfg)
	elbClient := clients.ELBV2

	targetGroupARN := infra.Resources[state.ResourceTargetGroup]
	if targetGroupARN == "" {
		return fmt.Errorf("target group not found in infrastructure resources")
	}

	_, err := elbClient.ModifyTargetGroup(ctx, &elbv2.ModifyTargetGroupInput{
		TargetGroupArn:  aws.String(targetGroupARN),
		HealthCheckPath: aws.String(healthCheckPath),
		HealthCheckPort: aws.String(fmt.Sprintf("%d", containerPort)),
	})
	if err != nil {
		return fmt.Errorf("modify target group health check: %w", err)
	}

	slog.Info("target group health check updated",
		slog.String("component", "updateTargetGroupHealthCheck"),
		slog.String("health_check_path", healthCheckPath),
		slog.Int("container_port", containerPort))
	return nil
}

func (p *AWSProvider) getALBURLs(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) ([]string, error) {
	clients := p.getClients(cfg)
	elbClient := clients.ELBV2

	albARN := infra.Resources[state.ResourceALB]
	if albARN == "" {
		return nil, nil
	}

	resp, err := elbClient.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{albARN},
	})
	if err != nil {
		return nil, err
	}

	// Determine URL scheme based on whether TLS is enabled.
	// Per spec ralph/specs/tls-https.md: Return https:// URLs when TLS is enabled.
	scheme := "http"
	if infra.Resources[state.ResourceTLSEnabled] == "true" {
		scheme = "https"
	}

	var urls []string
	for _, lb := range resp.LoadBalancers {
		if lb.DNSName != nil {
			urls = append(urls, scheme+"://"+*lb.DNSName)
		}
	}
	return urls, nil
}

func (p *AWSProvider) deleteECSService(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, deployment *state.Deployment) error {
	clients := p.getClients(cfg)
	ecsClient := clients.ECS

	clusterARN := infra.Resources[state.ResourceECSCluster]
	if clusterARN == "" || deployment.ServiceARN == "" {
		return nil
	}

	// Set desired count to 0 first.
	_, err := ecsClient.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      aws.String(clusterARN),
		Service:      aws.String(deployment.ServiceARN),
		DesiredCount: aws.Int32(0),
	})
	if err != nil {
		slog.Warn("could not scale down service",
			slog.String("component", "deleteECSService"),
			logging.Err(err))
	}

	// Delete service.
	_, err = ecsClient.DeleteService(ctx, &ecs.DeleteServiceInput{
		Cluster: aws.String(clusterARN),
		Service: aws.String(deployment.ServiceARN),
		Force:   aws.Bool(true),
	})
	return err
}

func (p *AWSProvider) deleteECSCluster(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
	clients := p.getClients(cfg)
	ecsClient := clients.ECS

	clusterARN := infra.Resources[state.ResourceECSCluster]
	if clusterARN == "" {
		return nil
	}

	_, err := ecsClient.DeleteCluster(ctx, &ecs.DeleteClusterInput{
		Cluster: aws.String(clusterARN),
	})
	return err
}

func (p *AWSProvider) deleteALB(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
	clients := p.getClients(cfg)
	elbClient := clients.ELBV2

	// Delete ALB.
	albARN := infra.Resources[state.ResourceALB]
	if albARN != "" {
		_, err := elbClient.DeleteLoadBalancer(ctx, &elbv2.DeleteLoadBalancerInput{
			LoadBalancerArn: aws.String(albARN),
		})
		if err != nil {
			return fmt.Errorf("failed to delete ALB: %w", err)
		}
	}

	// Delete target group.
	tgARN := infra.Resources[state.ResourceTargetGroup]
	if tgARN != "" {
		// Wait for ALB deletion to complete (target group can't be deleted while attached).
		time.Sleep(5 * time.Second)
		_, err := elbClient.DeleteTargetGroup(ctx, &elbv2.DeleteTargetGroupInput{
			TargetGroupArn: aws.String(tgARN),
		})
		if err != nil {
			return fmt.Errorf("failed to delete target group: %w", err)
		}
	}

	return nil
}

func (p *AWSProvider) deleteECRRepository(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
	clients := p.getClients(cfg)
	ecrClient := clients.ECR

	repoName := infra.Resources[state.ResourceECRRepository]
	if repoName == "" {
		return nil
	}

	_, err := ecrClient.DeleteRepository(ctx, &ecr.DeleteRepositoryInput{
		RepositoryName: aws.String(repoName),
		Force:          true, // Delete even if images exist.
	})
	if err != nil {
		return fmt.Errorf("failed to delete ECR repository: %w", err)
	}
	slog.Info("ECR repository deleted",
		slog.String("component", "deleteECRRepository"),
		slog.String("repo_name", repoName))
	return nil
}

func (p *AWSProvider) deleteLogGroup(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
	clients := p.getClients(cfg)
	cwlClient := clients.CloudWatchLogs

	logGroupName := infra.Resources[state.ResourceLogGroup]
	if logGroupName == "" {
		return nil
	}

	_, err := cwlClient.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	})
	if err != nil {
		return fmt.Errorf("failed to delete log group: %w", err)
	}
	slog.Info("log group deleted",
		slog.String("component", "deleteLogGroup"),
		slog.String("log_group_name", logGroupName))
	return nil
}

// deleteExecutionRole deletes the IAM execution role for ECS tasks.
func (p *AWSProvider) deleteExecutionRole(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
	clients := p.getClients(cfg)
	iamClient := clients.IAM

	// Get role name from role ARN.
	roleARN := infra.Resources[state.ResourceExecutionRole]
	if roleARN == "" {
		return nil
	}

	// Extract role name from ARN (format: arn:aws:iam::123456789012:role/role-name).
	roleName := roleARN
	if idx := strings.LastIndex(roleARN, "/"); idx >= 0 {
		roleName = roleARN[idx+1:]
	}

	// Detach the managed policy first.
	managedPolicyARN := "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
	_, err := iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String(managedPolicyARN),
	})
	if err != nil {
		// Handle NoSuchEntity error gracefully.
		if !strings.Contains(err.Error(), "NoSuchEntity") {
			slog.Warn("failed to detach execution role policy", "roleName", roleName, "error", err)
		}
	}

	// Delete the role.
	_, err = iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		// Handle NoSuchEntity error gracefully.
		if strings.Contains(err.Error(), "NoSuchEntity") {
			slog.Info("execution role already deleted", "roleName", roleName)
			return nil
		}
		return fmt.Errorf("failed to delete execution role: %w", err)
	}

	slog.Info("execution role deleted", "roleName", roleName)
	return nil
}

func (p *AWSProvider) deleteVPCResources(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
	clients := p.getClients(cfg)
	ec2Client := clients.EC2

	// Per spec ralph/specs/networking.md: Delete in reverse dependency order.
	// 1. Delete NAT Gateway first (and wait for deletion).
	natGWID := infra.Resources[state.ResourceNATGateway]
	if natGWID != "" {
		_, err := ec2Client.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{
			NatGatewayId: aws.String(natGWID),
		})
		if err != nil {
			slog.Warn("failed to delete NAT gateway",
				slog.String("component", "deleteVPCResources"),
				slog.String("nat_gateway_id", natGWID),
				logging.Err(err))
		} else {
			// Wait for NAT Gateway to be deleted before releasing Elastic IP.
			slog.Info("waiting for NAT gateway deletion",
				slog.String("component", "deleteVPCResources"),
				slog.String("nat_gateway_id", natGWID))
			waiter := ec2.NewNatGatewayDeletedWaiter(ec2Client)
			if err := waiter.Wait(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []string{natGWID},
			}, 5*time.Minute); err != nil {
				slog.Warn("timeout waiting for NAT gateway deletion",
					slog.String("component", "deleteVPCResources"),
					slog.String("nat_gateway_id", natGWID),
					logging.Err(err))
			}
		}
	}

	// 2. Release Elastic IP.
	eipAllocationID := infra.Resources[state.ResourceElasticIP]
	if eipAllocationID != "" {
		_, err := ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{
			AllocationId: aws.String(eipAllocationID),
		})
		if err != nil {
			slog.Warn("failed to release Elastic IP",
				slog.String("component", "deleteVPCResources"),
				slog.String("allocation_id", eipAllocationID),
				logging.Err(err))
		}
	}

	// 3. Delete task security group.
	taskSGID := infra.Resources[state.ResourceSecurityGroupTask]
	if taskSGID != "" {
		_, err := ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: aws.String(taskSGID),
		})
		if err != nil {
			slog.Warn("failed to delete task security group",
				slog.String("component", "deleteVPCResources"),
				slog.String("security_group_id", taskSGID),
				logging.Err(err))
		}
	}

	// 4. Delete ALB security group.
	albSGID := infra.Resources[state.ResourceSecurityGroupALB]
	if albSGID != "" {
		_, err := ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: aws.String(albSGID),
		})
		if err != nil {
			slog.Warn("failed to delete ALB security group",
				slog.String("component", "deleteVPCResources"),
				slog.String("security_group_id", albSGID),
				logging.Err(err))
		}
	}

	// 5. Delete legacy security group (for backward compatibility).
	sgID := infra.Resources[state.ResourceSecurityGroup]
	if sgID != "" && sgID != taskSGID {
		_, err := ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: aws.String(sgID),
		})
		if err != nil {
			slog.Warn("failed to delete security group",
				slog.String("component", "deleteVPCResources"),
				slog.String("security_group_id", sgID),
				logging.Err(err))
		}
	}

	// 6. Delete private route table.
	privateRTID := infra.Resources[state.ResourceRouteTablePrivate]
	if privateRTID != "" {
		if err := p.deleteRouteTable(ctx, ec2Client, privateRTID); err != nil {
			slog.Warn("failed to delete private route table",
				slog.String("component", "deleteVPCResources"),
				slog.String("route_table_id", privateRTID),
				logging.Err(err))
		}
	}

	// 7. Delete public route table.
	rtID := infra.Resources[state.ResourceRouteTable]
	if rtID != "" {
		if err := p.deleteRouteTable(ctx, ec2Client, rtID); err != nil {
			slog.Warn("failed to delete public route table",
				slog.String("component", "deleteVPCResources"),
				slog.String("route_table_id", rtID),
				logging.Err(err))
		}
	}

	// 8. Delete private subnets.
	privateSubnetStr := infra.Resources[state.ResourceSubnetPrivate]
	for _, subnetID := range splitComma(privateSubnetStr) {
		if subnetID != "" {
			_, err := ec2Client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{
				SubnetId: aws.String(subnetID),
			})
			if err != nil {
				slog.Warn("failed to delete private subnet",
					slog.String("component", "deleteVPCResources"),
					slog.String("subnet_id", subnetID),
					logging.Err(err))
			}
		}
	}

	// 9. Delete public subnets.
	subnetStr := infra.Resources[state.ResourceSubnetPublic]
	for _, subnetID := range splitComma(subnetStr) {
		if subnetID != "" {
			_, err := ec2Client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{
				SubnetId: aws.String(subnetID),
			})
			if err != nil {
				slog.Warn("failed to delete public subnet",
					slog.String("component", "deleteVPCResources"),
					slog.String("subnet_id", subnetID),
					logging.Err(err))
			}
		}
	}

	// 10. Detach and delete internet gateway.
	igwID := infra.Resources[state.ResourceInternetGateway]
	vpcID := infra.Resources[state.ResourceVPC]
	if igwID != "" && vpcID != "" {
		if _, err := ec2Client.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{
			InternetGatewayId: aws.String(igwID),
			VpcId:             aws.String(vpcID),
		}); err != nil {
			slog.Warn("failed to detach internet gateway",
				slog.String("component", "deleteVPCResources"),
				slog.String("igw_id", igwID),
				logging.Err(err))
		}
		if _, err := ec2Client.DeleteInternetGateway(ctx, &ec2.DeleteInternetGatewayInput{
			InternetGatewayId: aws.String(igwID),
		}); err != nil {
			slog.Warn("failed to delete internet gateway",
				slog.String("component", "deleteVPCResources"),
				slog.String("igw_id", igwID),
				logging.Err(err))
		}
	}

	// 11. Delete VPC.
	if vpcID != "" {
		_, err := ec2Client.DeleteVpc(ctx, &ec2.DeleteVpcInput{
			VpcId: aws.String(vpcID),
		})
		if err != nil {
			return fmt.Errorf("failed to delete VPC: %w", err)
		}
	}

	return nil
}

// deleteRouteTable disassociates and deletes a route table.
func (p *AWSProvider) deleteRouteTable(ctx context.Context, ec2Client awsclient.EC2API, rtID string) error {
	// Get associations.
	rtResp, err := ec2Client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		RouteTableIds: []string{rtID},
	})
	if err != nil {
		return fmt.Errorf("describe route table: %w", err)
	}
	if len(rtResp.RouteTables) > 0 {
		for _, assoc := range rtResp.RouteTables[0].Associations {
			if assoc.RouteTableAssociationId != nil && !*assoc.Main {
				if _, err := ec2Client.DisassociateRouteTable(ctx, &ec2.DisassociateRouteTableInput{
					AssociationId: assoc.RouteTableAssociationId,
				}); err != nil {
					return fmt.Errorf("disassociate route table: %w", err)
				}
			}
		}
	}
	if _, err := ec2Client.DeleteRouteTable(ctx, &ec2.DeleteRouteTableInput{
		RouteTableId: aws.String(rtID),
	}); err != nil {
		return fmt.Errorf("delete route table: %w", err)
	}
	return nil
}

// rollbackInfra tears down any resources recorded in the infrastructure record.
// WHY: Per spec ralph/specs/error-handling.md - when createInfra fails partway
// through, we must clean up already-created resources to prevent orphaned AWS
// resources and unexpected costs.
//
// Errors during rollback are logged but don't prevent continued cleanup attempts.
// The function cleans up resources in reverse order of creation.
func (p *AWSProvider) rollbackInfra(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
	log := logging.WithComponent("rollback")
	log.Info("starting rollback", logging.InfraID(infra.ID))

	var rollbackErrors []string

	// Delete in reverse order of creation to respect dependencies.
	// Order: log group -> execution role -> ALB -> ECS cluster -> VPC resources

	if infra.Resources[state.ResourceLogGroup] != "" {
		if err := p.deleteLogGroup(ctx, cfg, infra); err != nil {
			log.Warn("rollback: failed to delete log group", logging.Err(err), logging.InfraID(infra.ID))
			rollbackErrors = append(rollbackErrors, "log group: "+err.Error())
		} else {
			log.Info("rollback: deleted log group", logging.InfraID(infra.ID))
		}
	}

	if infra.Resources[state.ResourceExecutionRole] != "" {
		if err := p.deleteExecutionRole(ctx, cfg, infra); err != nil {
			log.Warn("rollback: failed to delete execution role", logging.Err(err), logging.InfraID(infra.ID))
			rollbackErrors = append(rollbackErrors, "execution role: "+err.Error())
		} else {
			log.Info("rollback: deleted execution role", logging.InfraID(infra.ID))
		}
	}

	if infra.Resources[state.ResourceALB] != "" {
		if err := p.deleteALB(ctx, cfg, infra); err != nil {
			log.Warn("rollback: failed to delete ALB", logging.Err(err), logging.InfraID(infra.ID))
			rollbackErrors = append(rollbackErrors, "ALB: "+err.Error())
		} else {
			log.Info("rollback: deleted ALB", logging.InfraID(infra.ID))
		}
	}

	if infra.Resources[state.ResourceECSCluster] != "" {
		if err := p.deleteECSCluster(ctx, cfg, infra); err != nil {
			log.Warn("rollback: failed to delete ECS cluster", logging.Err(err), logging.InfraID(infra.ID))
			rollbackErrors = append(rollbackErrors, "ECS cluster: "+err.Error())
		} else {
			log.Info("rollback: deleted ECS cluster", logging.InfraID(infra.ID))
		}
	}

	if infra.Resources[state.ResourceVPC] != "" {
		if err := p.deleteVPCResources(ctx, cfg, infra); err != nil {
			log.Warn("rollback: failed to delete VPC resources", logging.Err(err), logging.InfraID(infra.ID))
			rollbackErrors = append(rollbackErrors, "VPC: "+err.Error())
		} else {
			log.Info("rollback: deleted VPC resources", logging.InfraID(infra.ID))
		}
	}

	// Mark infra as destroyed after rollback attempt.
	if err := p.store.SetInfraStatus(infra.ID, state.InfraStatusDestroyed); err != nil {
		log.Warn("rollback: failed to set infra status to destroyed", logging.Err(err), logging.InfraID(infra.ID))
	}

	if len(rollbackErrors) > 0 {
		log.Warn("rollback completed with errors",
			logging.InfraID(infra.ID),
			logging.Count(len(rollbackErrors)))
		return fmt.Errorf("partial rollback: %s", strings.Join(rollbackErrors, "; "))
	}

	log.Info("rollback completed successfully", logging.InfraID(infra.ID))
	return nil
}

// ---------------------------------------------------------------------------
// Resources
// ---------------------------------------------------------------------------

func (p *AWSProvider) RegisterResources(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		Name:     "deployments",
		URI:      "aws:deployments",
		MIMEType: "application/json",
	}, p.deploymentsResource)
}

type deploymentsResponse struct {
	Deployments []deploymentInfo `json:"deployments"`
}

type deploymentInfo struct {
	DeploymentID string   `json:"deployment_id"`
	InfraID      string   `json:"infra_id"`
	Status       string   `json:"status"`
	CreatedAt    string   `json:"created_at"`
	URLs         []string `json:"urls"`
}

func (p *AWSProvider) deploymentsResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "aws" {
		return nil, fmt.Errorf("unsupported scheme: %q", u.Scheme)
	}

	deployments, err := p.store.ListDeployments()
	if err != nil {
		return nil, err
	}

	resp := deploymentsResponse{Deployments: make([]deploymentInfo, 0, len(deployments))}
	for _, d := range deployments {
		resp.Deployments = append(resp.Deployments, deploymentInfo{
			DeploymentID: d.ID,
			InfraID:      d.InfraID,
			Status:       d.Status,
			CreatedAt:    d.CreatedAt.Format(time.RFC3339),
			URLs:         d.URLs,
		})
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Auto Scaling
// ---------------------------------------------------------------------------

// validateAutoScalingParams validates auto-scaling input parameters per spec ralph/specs/auto-scaling.md.
func validateAutoScalingParams(minCount, maxCount, targetCPU, targetMem int) error {
	if minCount < 1 {
		return fmt.Errorf("min_count must be at least 1, got %d", minCount)
	}
	if maxCount < minCount {
		return fmt.Errorf("max_count must be >= min_count (min=%d, max=%d)", minCount, maxCount)
	}
	if targetCPU < 10 || targetCPU > 90 {
		return fmt.Errorf("target_cpu_percent must be between 10 and 90, got %d", targetCPU)
	}
	if targetMem < 10 || targetMem > 90 {
		return fmt.Errorf("target_memory_percent must be between 10 and 90, got %d", targetMem)
	}
	// Warn for high max_count (but don't fail).
	if maxCount > 10 {
		slog.Warn("high max_count may cause cost spikes",
			slog.String("component", "validateAutoScalingParams"),
			slog.Int("max_count", maxCount))
	}
	return nil
}

// configureAutoScaling registers an ECS service as a scalable target and creates
// target tracking scaling policies for CPU and memory utilization.
// Per spec ralph/specs/auto-scaling.md: cooldowns are 60s scale-out, 300s scale-in.
func (p *AWSProvider) configureAutoScaling(ctx context.Context, cfg aws.Config, clusterName, serviceName, deployID string, minCount, maxCount, targetCPU, targetMem int) error {
	clients := p.getClients(cfg)
	asClient := clients.AutoScaling

	resourceID := fmt.Sprintf("service/%s/%s", clusterName, serviceName)

	// Register scalable target.
	_, err := asClient.RegisterScalableTarget(ctx, &applicationautoscaling.RegisterScalableTargetInput{
		ServiceNamespace:  astypes.ServiceNamespaceEcs,
		ResourceId:        aws.String(resourceID),
		ScalableDimension: astypes.ScalableDimensionECSServiceDesiredCount,
		MinCapacity:       aws.Int32(int32(minCount)),
		MaxCapacity:       aws.Int32(int32(maxCount)),
	})
	if err != nil {
		return fmt.Errorf("register scalable target: %w", err)
	}

	// Create CPU-based scaling policy.
	cpuPolicyName := "agent-deploy-cpu-" + deployID[:12]
	_, err = asClient.PutScalingPolicy(ctx, &applicationautoscaling.PutScalingPolicyInput{
		PolicyName:        aws.String(cpuPolicyName),
		ServiceNamespace:  astypes.ServiceNamespaceEcs,
		ResourceId:        aws.String(resourceID),
		ScalableDimension: astypes.ScalableDimensionECSServiceDesiredCount,
		PolicyType:        astypes.PolicyTypeTargetTrackingScaling,
		TargetTrackingScalingPolicyConfiguration: &astypes.TargetTrackingScalingPolicyConfiguration{
			PredefinedMetricSpecification: &astypes.PredefinedMetricSpecification{
				PredefinedMetricType: astypes.MetricTypeECSServiceAverageCPUUtilization,
			},
			TargetValue:      aws.Float64(float64(targetCPU)),
			ScaleInCooldown:  aws.Int32(300), // 5 minutes - avoid thrashing on brief load dips.
			ScaleOutCooldown: aws.Int32(60),  // 1 minute - react quickly to load spikes.
		},
	})
	if err != nil {
		return fmt.Errorf("create CPU scaling policy: %w", err)
	}

	// Create memory-based scaling policy.
	memPolicyName := "agent-deploy-memory-" + deployID[:12]
	_, err = asClient.PutScalingPolicy(ctx, &applicationautoscaling.PutScalingPolicyInput{
		PolicyName:        aws.String(memPolicyName),
		ServiceNamespace:  astypes.ServiceNamespaceEcs,
		ResourceId:        aws.String(resourceID),
		ScalableDimension: astypes.ScalableDimensionECSServiceDesiredCount,
		PolicyType:        astypes.PolicyTypeTargetTrackingScaling,
		TargetTrackingScalingPolicyConfiguration: &astypes.TargetTrackingScalingPolicyConfiguration{
			PredefinedMetricSpecification: &astypes.PredefinedMetricSpecification{
				PredefinedMetricType: astypes.MetricTypeECSServiceAverageMemoryUtilization,
			},
			TargetValue:      aws.Float64(float64(targetMem)),
			ScaleInCooldown:  aws.Int32(300),
			ScaleOutCooldown: aws.Int32(60),
		},
	})
	if err != nil {
		return fmt.Errorf("create memory scaling policy: %w", err)
	}

	return nil
}

// deleteAutoScaling removes scaling policies and deregisters the scalable target.
// Per spec ralph/specs/auto-scaling.md: must be called BEFORE deleting ECS service.
func (p *AWSProvider) deleteAutoScaling(ctx context.Context, cfg aws.Config, clusterName, serviceName, deployID string) error {
	clients := p.getClients(cfg)
	asClient := clients.AutoScaling

	resourceID := fmt.Sprintf("service/%s/%s", clusterName, serviceName)

	// Delete CPU scaling policy.
	cpuPolicyName := "agent-deploy-cpu-" + deployID[:12]
	_, err := asClient.DeleteScalingPolicy(ctx, &applicationautoscaling.DeleteScalingPolicyInput{
		PolicyName:        aws.String(cpuPolicyName),
		ServiceNamespace:  astypes.ServiceNamespaceEcs,
		ResourceId:        aws.String(resourceID),
		ScalableDimension: astypes.ScalableDimensionECSServiceDesiredCount,
	})
	if err != nil {
		// Log but continue - policy may not exist.
		slog.Debug("could not delete CPU scaling policy",
			slog.String("component", "deleteAutoScaling"),
			logging.Err(err))
	}

	// Delete memory scaling policy.
	memPolicyName := "agent-deploy-memory-" + deployID[:12]
	_, err = asClient.DeleteScalingPolicy(ctx, &applicationautoscaling.DeleteScalingPolicyInput{
		PolicyName:        aws.String(memPolicyName),
		ServiceNamespace:  astypes.ServiceNamespaceEcs,
		ResourceId:        aws.String(resourceID),
		ScalableDimension: astypes.ScalableDimensionECSServiceDesiredCount,
	})
	if err != nil {
		slog.Debug("could not delete memory scaling policy",
			slog.String("component", "deleteAutoScaling"),
			logging.Err(err))
	}

	// Deregister scalable target.
	_, err = asClient.DeregisterScalableTarget(ctx, &applicationautoscaling.DeregisterScalableTargetInput{
		ServiceNamespace:  astypes.ServiceNamespaceEcs,
		ResourceId:        aws.String(resourceID),
		ScalableDimension: astypes.ScalableDimensionECSServiceDesiredCount,
	})
	if err != nil {
		// If target doesn't exist, that's fine.
		slog.Debug("could not deregister scalable target",
			slog.String("component", "deleteAutoScaling"),
			logging.Err(err))
	}

	return nil
}

// getScalingInfo retrieves current auto-scaling configuration for status reporting.
func (p *AWSProvider) getScalingInfo(ctx context.Context, cfg aws.Config, clusterName, serviceName string, currentCount int) (*scalingInfo, error) {
	clients := p.getClients(cfg)
	asClient := clients.AutoScaling

	resourceID := fmt.Sprintf("service/%s/%s", clusterName, serviceName)

	// Describe scalable targets to get min/max capacity.
	resp, err := asClient.DescribeScalableTargets(ctx, &applicationautoscaling.DescribeScalableTargetsInput{
		ServiceNamespace: astypes.ServiceNamespaceEcs,
		ResourceIds:      []string{resourceID},
	})
	if err != nil {
		return nil, err
	}

	if len(resp.ScalableTargets) == 0 {
		return nil, nil // No scaling configured.
	}

	target := resp.ScalableTargets[0]

	// Get scaling policies to extract target percentages.
	policiesResp, err := asClient.DescribeScalingPolicies(ctx, &applicationautoscaling.DescribeScalingPoliciesInput{
		ServiceNamespace: astypes.ServiceNamespaceEcs,
		ResourceId:       aws.String(resourceID),
	})
	if err != nil {
		return nil, err
	}

	info := &scalingInfo{
		MinCount:     int(*target.MinCapacity),
		MaxCount:     int(*target.MaxCapacity),
		CurrentCount: currentCount,
	}

	// Extract target percentages from policies.
	for _, policy := range policiesResp.ScalingPolicies {
		if policy.TargetTrackingScalingPolicyConfiguration != nil {
			cfg := policy.TargetTrackingScalingPolicyConfiguration
			if cfg.PredefinedMetricSpecification != nil {
				switch cfg.PredefinedMetricSpecification.PredefinedMetricType {
				case astypes.MetricTypeECSServiceAverageCPUUtilization:
					info.TargetCPUPercent = int(*cfg.TargetValue)
				case astypes.MetricTypeECSServiceAverageMemoryUtilization:
					info.TargetMemPercent = int(*cfg.TargetValue)
				}
			}
		}
	}

	return info, nil
}

// extractClusterName extracts the cluster name from an ECS cluster ARN.
// ARN format: arn:aws:ecs:region:account:cluster/cluster-name
func extractClusterName(clusterARN string) string {
	if clusterARN == "" {
		return ""
	}
	parts := strings.Split(clusterARN, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return clusterARN
}

// extractServiceName extracts the service name from an ECS service ARN.
// ARN format: arn:aws:ecs:region:account:service/cluster-name/service-name
func extractServiceName(serviceARN string) string {
	if serviceARN == "" {
		return ""
	}
	parts := strings.Split(serviceARN, "/")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return serviceARN
}

// ---------------------------------------------------------------------------
// Prompts
// ---------------------------------------------------------------------------

func (p *AWSProvider) RegisterPrompts(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "aws_deploy_plan",
		Description: "Guide the user through planning and deploying an application on AWS",
		Arguments: []*mcp.PromptArgument{
			{Name: "app_description", Description: "Brief description of the application", Required: true},
		},
	}, deployPlanPrompt)
}

func deployPlanPrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	app := req.Params.Arguments["app_description"]
	return &mcp.GetPromptResult{
		Description: "AWS deployment planning workflow",
		Messages: []*mcp.PromptMessage{
			{
				Role: "user",
				Content: &mcp.TextContent{Text: fmt.Sprintf(
					`I want to deploy the following application on AWS: %s

Please:
1. Ask me clarifying questions about expected traffic, latency requirements, and preferred region.
2. Use aws_plan_infra to generate an infrastructure plan with cost estimate.
3. Present the plan and wait for my approval before proceeding.
4. Once approved, use aws_create_infra and aws_deploy to set up everything.
5. Return the public URL(s) when the deployment is live.

Important: Do not exceed any spending limits I have set.`, app),
				},
			},
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mapToEC2Tags(tags map[string]string) []ec2types.Tag {
	result := make([]ec2types.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, ec2types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return result
}

func mapToECSTags(tags map[string]string) []ecstypes.Tag {
	result := make([]ecstypes.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, ecstypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return result
}

func mapToELBTags(tags map[string]string) []elbv2types.Tag {
	result := make([]elbv2types.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, elbv2types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return result
}

func mapToECRTags(tags map[string]string) []ecrtypes.Tag {
	result := make([]ecrtypes.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, ecrtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return result
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

// validateCertificate validates an ACM certificate ARN before using it for HTTPS.
// Per spec ralph/specs/tls-https.md: Validates ARN format, certificate existence, and issued status.
func (p *AWSProvider) validateCertificate(ctx context.Context, cfg aws.Config, certARN string) error {
	// Validate ARN format: arn:aws:acm:region:account:certificate/id
	if !strings.HasPrefix(certARN, "arn:aws:acm:") {
		return fmt.Errorf("invalid certificate ARN format: must start with 'arn:aws:acm:'")
	}
	if !strings.Contains(certARN, ":certificate/") {
		return fmt.Errorf("invalid certificate ARN format: must contain ':certificate/'")
	}

	// Call ACM to verify the certificate exists and is issued.
	clients := p.getClients(cfg)
	acmClient := clients.ACM
	resp, err := acmClient.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certARN),
	})
	if err != nil {
		// Handle common errors with clear messages.
		if strings.Contains(err.Error(), "ResourceNotFoundException") {
			return fmt.Errorf("certificate not found: %s", certARN)
		}
		return fmt.Errorf("failed to describe certificate: %w", err)
	}

	if resp.Certificate == nil {
		return fmt.Errorf("certificate not found: %s", certARN)
	}

	// Check certificate status.
	status := resp.Certificate.Status
	switch status {
	case acmtypes.CertificateStatusIssued:
		// Certificate is valid and ready to use.
		slog.Info("certificate validated",
			slog.String("component", "validateCertificate"),
			slog.String("certificate_arn", certARN),
			slog.String("status", string(status)))
		return nil

	case acmtypes.CertificateStatusPendingValidation:
		return fmt.Errorf("certificate is pending validation: complete DNS or email validation for %s", certARN)

	case acmtypes.CertificateStatusExpired:
		return fmt.Errorf("certificate is expired: %s", certARN)

	case acmtypes.CertificateStatusInactive:
		return fmt.Errorf("certificate is inactive: %s", certARN)

	case acmtypes.CertificateStatusRevoked:
		return fmt.Errorf("certificate is revoked: %s", certARN)

	case acmtypes.CertificateStatusFailed:
		return fmt.Errorf("certificate validation failed: %s", certARN)

	default:
		return fmt.Errorf("certificate is not in issued status (current: %s): %s", status, certARN)
	}
}
