// Package providers implements cloud provider integrations for the MCP server.
package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/cjmartian/agent-deploy/internal/awsclient"
	apperrors "github.com/cjmartian/agent-deploy/internal/errors"
	"github.com/cjmartian/agent-deploy/internal/id"
	"github.com/cjmartian/agent-deploy/internal/spending"
	"github.com/cjmartian/agent-deploy/internal/state"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AWSProvider implements Provider for Amazon Web Services.
type AWSProvider struct {
	store *state.Store
}

// NewAWSProvider creates a new AWS provider with the given state store.
func NewAWSProvider(store *state.Store) *AWSProvider {
	return &AWSProvider{store: store}
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
	PlanID string `json:"plan_id" jsonschema:"the plan ID returned by aws_plan_infra"`
}

type createInfraOutput struct {
	InfraID string `json:"infra_id"`
	Status  string `json:"status"`
}

type deployInput struct {
	InfraID  string `json:"infra_id"  jsonschema:"infrastructure ID from aws_create_infra"`
	ImageRef string `json:"image_ref" jsonschema:"container image or artifact reference to deploy"`
}

type deployOutput struct {
	DeploymentID string `json:"deployment_id"`
	Status       string `json:"status"`
}

type statusInput struct {
	DeploymentID string `json:"deployment_id" jsonschema:"deployment ID from aws_deploy"`
}

type statusOutput struct {
	DeploymentID string   `json:"deployment_id"`
	Status       string   `json:"status"`
	URLs         []string `json:"urls"`
}

type teardownInput struct {
	DeploymentID string `json:"deployment_id" jsonschema:"deployment ID to tear down"`
}

type teardownOutput struct {
	DeploymentID string `json:"deployment_id"`
	Status       string `json:"status"`
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

	// Estimate cost based on user count and latency requirements.
	// This is a simplified estimation; real implementation would use AWS Pricing API.
	baseCost := 15.0 // VPC, basic networking
	ecsCost := float64(in.ExpectedUsers) * 0.02
	if in.LatencyMS < 100 {
		ecsCost *= 1.5 // Better instances for low latency
	}
	albCost := 20.0 // ALB base cost
	estimatedCost := baseCost + ecsCost + albCost

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

	log.Printf("[aws_plan_infra] Created plan %s for %q (est. $%.2f/mo)", plan.ID, in.AppDescription, estimatedCost)

	return nil, planInfraOutput{
		PlanID:          plan.ID,
		Services:        services,
		EstimatedCostMo: fmt.Sprintf("$%.2f", estimatedCost),
		Summary: fmt.Sprintf(
			"Proposed plan for %q: ECS Fargate in %s, targeting %d users at ≤%dms p99. Estimated cost: $%.2f/mo. Plan ID: %s (expires in 24h). Call aws_create_infra with this plan_id to provision infrastructure.",
			in.AppDescription, in.Region, in.ExpectedUsers, in.LatencyMS, estimatedCost, plan.ID,
		),
	}, nil
}

// createInfra provisions AWS infrastructure according to an approved plan.
func (p *AWSProvider) createInfra(ctx context.Context, _ *mcp.CallToolRequest, in createInfraInput) (*mcp.CallToolResult, createInfraOutput, error) {
	// Get and validate plan.
	plan, err := p.store.GetPlan(in.PlanID)
	if err != nil {
		return nil, createInfraOutput{}, err
	}

	// Auto-approve if still in created status (for convenience).
	if plan.Status == state.PlanStatusCreated {
		if err = p.store.ApprovePlan(in.PlanID); err != nil {
			return nil, createInfraOutput{}, err
		}
	}

	// Check spending limits.
	limits, _ := spending.LoadLimits()
	deployments, _ := p.store.ListDeployments()
	var currentSpend float64
	for _, d := range deployments {
		if d.Status == state.DeploymentStatusRunning {
			// Get infra to find cost (simplified: count active deployments * avg cost)
			currentSpend += 25.0
		}
	}

	check := spending.CheckBudget(plan.EstimatedCostMo, limits, currentSpend)
	if !check.Allowed {
		return nil, createInfraOutput{}, fmt.Errorf("%w: %s", apperrors.ErrBudgetExceeded, check.Reason)
	}

	// Load AWS config.
	cfg, err := awsclient.LoadConfig(ctx, plan.Region)
	if err != nil {
		return nil, createInfraOutput{}, err
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

	// Provision resources in order.
	if err := p.provisionVPC(ctx, cfg, infra, tags); err != nil {
		if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
			slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
		}
		return nil, createInfraOutput{}, fmt.Errorf("provision VPC: %w", err)
	}

	if err := p.provisionECSCluster(ctx, cfg, infra, tags); err != nil {
		if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
			slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
		}
		return nil, createInfraOutput{}, fmt.Errorf("provision ECS cluster: %w", err)
	}

	if err := p.provisionALB(ctx, cfg, infra, tags); err != nil {
		if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
			slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
		}
		return nil, createInfraOutput{}, fmt.Errorf("provision ALB: %w", err)
	}

	// Create CloudWatch log group for ECS task logs.
	if err := p.provisionLogGroup(ctx, cfg, infra, tags); err != nil {
		if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
			slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
		}
		return nil, createInfraOutput{}, fmt.Errorf("provision log group: %w", err)
	}

	// Mark infrastructure as ready.
	if err := p.store.SetInfraStatus(infraID, state.InfraStatusReady); err != nil {
		return nil, createInfraOutput{}, err
	}

	log.Printf("[aws_create_infra] Infrastructure %s ready in %s", infraID, plan.Region)

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
	taskDefARN, err := p.createTaskDefinition(ctx, cfg, infra, in.ImageRef, deployID)
	if err != nil {
		if statusErr := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusFailed, nil); statusErr != nil {
			slog.Error("failed to update deployment status", "deployID", deployID, "error", statusErr)
		}
		return nil, deployOutput{}, fmt.Errorf("task definition: %w", err)
	}

	// Create or update ECS service.
	serviceARN, err := p.createOrUpdateService(ctx, cfg, infra, taskDefARN, deployID)
	if err != nil {
		if statusErr := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusFailed, nil); statusErr != nil {
			slog.Error("failed to update deployment status", "deployID", deployID, "error", statusErr)
		}
		return nil, deployOutput{}, fmt.Errorf("ECS service: %w", err)
	}

	// Get ALB DNS name for URL.
	urls, err := p.getALBURLs(ctx, cfg, infra)
	if err != nil {
		log.Printf("[aws_deploy] Warning: could not get ALB URLs: %v", err)
	}

	// Update deployment with URLs and status.
	if err := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusRunning, urls); err != nil {
		return nil, deployOutput{}, err
	}

	// Store task def and service ARNs.
	deployment.TaskDefARN = taskDefARN
	deployment.ServiceARN = serviceARN

	log.Printf("[aws_deploy] Deployment %s running at %v", deployID, urls)

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

	// Try to get live status from AWS.
	cfg, err := awsclient.LoadConfig(ctx, infra.Region)
	if err == nil {
		// Get ECS service status.
		ecsClient := ecs.NewFromConfig(cfg)
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

	// Delete ECS service.
	if err := p.deleteECSService(ctx, cfg, infra, deployment); err != nil {
		log.Printf("[aws_teardown] Warning: failed to delete ECS service: %v", err)
	}

	// Delete ECS cluster.
	if err := p.deleteECSCluster(ctx, cfg, infra); err != nil {
		log.Printf("[aws_teardown] Warning: failed to delete ECS cluster: %v", err)
	}

	// Delete ALB and target group.
	if err := p.deleteALB(ctx, cfg, infra); err != nil {
		log.Printf("[aws_teardown] Warning: failed to delete ALB: %v", err)
	}

	// Delete ECR repository.
	if err := p.deleteECRRepository(ctx, cfg, infra); err != nil {
		log.Printf("[aws_teardown] Warning: failed to delete ECR repository: %v", err)
	}

	// Delete CloudWatch log group.
	if err := p.deleteLogGroup(ctx, cfg, infra); err != nil {
		log.Printf("[aws_teardown] Warning: failed to delete log group: %v", err)
	}

	// Delete VPC resources (security groups, subnets, internet gateway, VPC).
	if err := p.deleteVPCResources(ctx, cfg, infra); err != nil {
		log.Printf("[aws_teardown] Warning: failed to delete VPC resources: %v", err)
	}

	// Update state (best-effort cleanup, log errors but continue).
	if err := p.store.UpdateDeploymentStatus(in.DeploymentID, state.DeploymentStatusStopped, nil); err != nil {
		slog.Error("failed to update deployment status during teardown", "deploymentID", in.DeploymentID, "error", err)
	}
	if err := p.store.SetInfraStatus(infra.ID, state.InfraStatusDestroyed); err != nil {
		slog.Error("failed to set infra status during teardown", "infraID", infra.ID, "error", err)
	}

	log.Printf("[aws_teardown] Deployment %s torn down", in.DeploymentID)

	return nil, teardownOutput{
		DeploymentID: in.DeploymentID,
		Status:       "destroyed",
	}, nil
}

// ---------------------------------------------------------------------------
// AWS Provisioning Helpers
// ---------------------------------------------------------------------------

func (p *AWSProvider) provisionVPC(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string) error {
	ec2Client := ec2.NewFromConfig(cfg)

	// Create VPC.
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

	// Create public subnets in 2 AZs (required for ALB).
	var subnetIDs []string
	var subnetResp *ec2.CreateSubnetOutput
	for i := 0; i < 2; i++ {
		az := *azResp.AvailabilityZones[i].ZoneName
		cidr := fmt.Sprintf("10.0.%d.0/24", i+1)

		subnetResp, err = ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            aws.String(vpcID),
			CidrBlock:        aws.String(cidr),
			AvailabilityZone: aws.String(az),
			TagSpecifications: []ec2types.TagSpecification{{
				ResourceType: ec2types.ResourceTypeSubnet,
				Tags:         mapToEC2Tags(tags),
			}},
		})
		if err != nil {
			return fmt.Errorf("create subnet %d: %w", i, err)
		}
		subnetIDs = append(subnetIDs, *subnetResp.Subnet.SubnetId)

		// Enable auto-assign public IP.
		_, err = ec2Client.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
			SubnetId:            subnetResp.Subnet.SubnetId,
			MapPublicIpOnLaunch: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("enable public IP for subnet %d: %w", i, err)
		}
	}
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceSubnetPublic, subnetIDs[0]+","+subnetIDs[1]); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceSubnetPublic, "error", storeErr)
	}
	infra.Resources[state.ResourceSubnetPublic] = subnetIDs[0] + "," + subnetIDs[1]

	// Create route table with internet route.
	rtResp, err := ec2Client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId: aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeRouteTable,
			Tags:         mapToEC2Tags(tags),
		}},
	})
	if err != nil {
		return fmt.Errorf("create route table: %w", err)
	}
	rtID := *rtResp.RouteTable.RouteTableId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceRouteTable, rtID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceRouteTable, "error", storeErr)
	}
	infra.Resources[state.ResourceRouteTable] = rtID

	// Add route to internet gateway.
	_, err = ec2Client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         aws.String(rtID),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(igwID),
	})
	if err != nil {
		return fmt.Errorf("create route to IGW: %w", err)
	}

	// Associate subnets with route table.
	for _, subnetID := range subnetIDs {
		_, err = ec2Client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(rtID),
			SubnetId:     aws.String(subnetID),
		})
		if err != nil {
			return fmt.Errorf("associate subnet %s: %w", subnetID, err)
		}
	}

	// Create security group.
	sgResp, err := ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String("agent-deploy-" + infra.ID),
		Description: aws.String("Security group for agent-deploy infrastructure"),
		VpcId:       aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{{
			ResourceType: ec2types.ResourceTypeSecurityGroup,
			Tags:         mapToEC2Tags(tags),
		}},
	})
	if err != nil {
		return fmt.Errorf("create security group: %w", err)
	}
	sgID := *sgResp.GroupId
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceSecurityGroup, sgID); storeErr != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceSecurityGroup, "error", storeErr)
	}
	infra.Resources[state.ResourceSecurityGroup] = sgID

	// Allow inbound HTTP (80) and HTTPS (443).
	_, err = ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(sgID),
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
		return fmt.Errorf("authorize ingress: %w", err)
	}

	log.Printf("[provisionVPC] VPC %s created with subnets %v, SG %s", vpcID, subnetIDs, sgID)
	return nil
}

func (p *AWSProvider) provisionECSCluster(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string) error {
	ecsClient := ecs.NewFromConfig(cfg)

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

	log.Printf("[provisionECSCluster] Cluster %s created", clusterARN)
	return nil
}

func (p *AWSProvider) provisionALB(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string) error {
	elbClient := elbv2.NewFromConfig(cfg)

	// Parse subnet IDs.
	subnetStr := infra.Resources[state.ResourceSubnetPublic]
	var subnetIDs []string
	for _, s := range splitComma(subnetStr) {
		if s != "" {
			subnetIDs = append(subnetIDs, s)
		}
	}

	// Create ALB.
	albResp, err := elbClient.CreateLoadBalancer(ctx, &elbv2.CreateLoadBalancerInput{
		Name:           aws.String("agent-deploy-" + infra.ID[:8]),
		Subnets:        subnetIDs,
		SecurityGroups: []string{infra.Resources[state.ResourceSecurityGroup]},
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

	// Create listener.
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

	log.Printf("[provisionALB] ALB %s and target group %s created", albARN, tgARN)
	return nil
}

// provisionLogGroup creates a CloudWatch log group for ECS task logs.
func (p *AWSProvider) provisionLogGroup(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string) error {
	cwlClient := cloudwatchlogs.NewFromConfig(cfg)

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
		log.Printf("[provisionLogGroup] Note: log group already exists")
	}

	// Set log retention to 7 days to manage costs.
	_, err = cwlClient.PutRetentionPolicy(ctx, &cloudwatchlogs.PutRetentionPolicyInput{
		LogGroupName:    aws.String(logGroupName),
		RetentionInDays: aws.Int32(7),
	})
	if err != nil {
		return fmt.Errorf("failed to set retention policy: %w", err)
	}

	if err := p.store.UpdateInfraResource(infra.ID, state.ResourceLogGroup, logGroupName); err != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceLogGroup, "error", err)
	}
	infra.Resources[state.ResourceLogGroup] = logGroupName

	log.Printf("[provisionLogGroup] Log group %s created", logGroupName)
	return nil
}

func (p *AWSProvider) ensureECRRepository(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, deployID string, tags map[string]string) error {
	ecrClient := ecr.NewFromConfig(cfg)

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
		log.Printf("[ensureECRRepository] Note: repository already exists")
	}

	if err := p.store.UpdateInfraResource(infra.ID, state.ResourceECRRepository, repoName); err != nil {
		slog.Error("failed to update infra resource", "infraID", infra.ID, "resource", state.ResourceECRRepository, "error", err)
	}
	return nil
}

func (p *AWSProvider) createTaskDefinition(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, imageRef, deployID string) (string, error) {
	ecsClient := ecs.NewFromConfig(cfg)

	// Use the provided image reference directly if it looks like a full URI,
	// otherwise assume it's a Docker Hub image.
	image := imageRef
	if image == "" {
		image = "nginx:latest"
	}

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

	resp, err := ecsClient.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("agent-deploy-" + deployID[:12]),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		ExecutionRoleArn:        nil, // Use default if available.
		ContainerDefinitions: []ecstypes.ContainerDefinition{{
			Name:      aws.String("app"),
			Image:     aws.String(image),
			Essential: aws.Bool(true),
			PortMappings: []ecstypes.PortMapping{{
				ContainerPort: aws.Int32(80),
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
	log.Printf("[createTaskDefinition] Task definition %s created with logs to %s", taskDefARN, logGroupName)
	return taskDefARN, nil
}

func (p *AWSProvider) createOrUpdateService(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, taskDefARN, deployID string) (string, error) {
	ecsClient := ecs.NewFromConfig(cfg)

	subnetStr := infra.Resources[state.ResourceSubnetPublic]
	var subnetIDs []string
	for _, s := range splitComma(subnetStr) {
		if s != "" {
			subnetIDs = append(subnetIDs, s)
		}
	}

	serviceName := "agent-deploy-" + deployID[:12]

	resp, err := ecsClient.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster:        aws.String(infra.Resources[state.ResourceECSCluster]),
		ServiceName:    aws.String(serviceName),
		TaskDefinition: aws.String(taskDefARN),
		DesiredCount:   aws.Int32(1),
		LaunchType:     ecstypes.LaunchTypeFargate,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        subnetIDs,
				SecurityGroups: []string{infra.Resources[state.ResourceSecurityGroup]},
				AssignPublicIp: ecstypes.AssignPublicIpEnabled,
			},
		},
		LoadBalancers: []ecstypes.LoadBalancer{{
			TargetGroupArn: aws.String(infra.Resources[state.ResourceTargetGroup]),
			ContainerName:  aws.String("app"),
			ContainerPort:  aws.Int32(80),
		}},
	})
	if err != nil {
		return "", fmt.Errorf("create service: %w", err)
	}

	serviceARN := *resp.Service.ServiceArn
	log.Printf("[createOrUpdateService] Service %s created", serviceARN)
	return serviceARN, nil
}

func (p *AWSProvider) getALBURLs(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) ([]string, error) {
	elbClient := elbv2.NewFromConfig(cfg)

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

	var urls []string
	for _, lb := range resp.LoadBalancers {
		if lb.DNSName != nil {
			urls = append(urls, "http://"+*lb.DNSName)
		}
	}
	return urls, nil
}

func (p *AWSProvider) deleteECSService(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, deployment *state.Deployment) error {
	ecsClient := ecs.NewFromConfig(cfg)

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
		log.Printf("[deleteECSService] Warning: could not scale down service: %v", err)
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
	ecsClient := ecs.NewFromConfig(cfg)

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
	elbClient := elbv2.NewFromConfig(cfg)

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
	ecrClient := ecr.NewFromConfig(cfg)

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
	log.Printf("[deleteECRRepository] ECR repository %s deleted", repoName)
	return nil
}

func (p *AWSProvider) deleteLogGroup(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
	cwlClient := cloudwatchlogs.NewFromConfig(cfg)

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
	log.Printf("[deleteLogGroup] Log group %s deleted", logGroupName)
	return nil
}

func (p *AWSProvider) deleteVPCResources(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) error {
	ec2Client := ec2.NewFromConfig(cfg)

	// Delete security group.
	sgID := infra.Resources[state.ResourceSecurityGroup]
	if sgID != "" {
		_, err := ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: aws.String(sgID),
		})
		if err != nil {
			return fmt.Errorf("failed to delete security group: %w", err)
		}
	}

	// Delete route table associations and route table.
	rtID := infra.Resources[state.ResourceRouteTable]
	if rtID != "" {
		// Get associations.
		rtResp, err := ec2Client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
			RouteTableIds: []string{rtID},
		})
		if err != nil {
			return fmt.Errorf("failed to describe route tables: %w", err)
		}
		if len(rtResp.RouteTables) > 0 {
			for _, assoc := range rtResp.RouteTables[0].Associations {
				if assoc.RouteTableAssociationId != nil && !*assoc.Main {
					if _, err := ec2Client.DisassociateRouteTable(ctx, &ec2.DisassociateRouteTableInput{
						AssociationId: assoc.RouteTableAssociationId,
					}); err != nil {
						return fmt.Errorf("failed to disassociate route table: %w", err)
					}
				}
			}
		}
		if _, err := ec2Client.DeleteRouteTable(ctx, &ec2.DeleteRouteTableInput{
			RouteTableId: aws.String(rtID),
		}); err != nil {
			return fmt.Errorf("failed to delete route table: %w", err)
		}
	}

	// Delete subnets.
	subnetStr := infra.Resources[state.ResourceSubnetPublic]
	for _, subnetID := range splitComma(subnetStr) {
		if subnetID != "" {
			_, err := ec2Client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{
				SubnetId: aws.String(subnetID),
			})
			if err != nil {
				return fmt.Errorf("failed to delete subnet %s: %w", subnetID, err)
			}
		}
	}

	// Detach and delete internet gateway.
	igwID := infra.Resources[state.ResourceInternetGateway]
	vpcID := infra.Resources[state.ResourceVPC]
	if igwID != "" && vpcID != "" {
		if _, err := ec2Client.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{
			InternetGatewayId: aws.String(igwID),
			VpcId:             aws.String(vpcID),
		}); err != nil {
			return fmt.Errorf("failed to detach internet gateway: %w", err)
		}
		if _, err := ec2Client.DeleteInternetGateway(ctx, &ec2.DeleteInternetGatewayInput{
			InternetGatewayId: aws.String(igwID),
		}); err != nil {
			return fmt.Errorf("failed to delete internet gateway: %w", err)
		}
	}

	// Delete VPC.
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
