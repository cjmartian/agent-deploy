// Package providers implements cloud provider integrations for the MCP server.
package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
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
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	lstypes "github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/cjmartian/agent-deploy/internal/awsclient"
	apperrors "github.com/cjmartian/agent-deploy/internal/errors"
	"github.com/cjmartian/agent-deploy/internal/id"
	"github.com/cjmartian/agent-deploy/internal/logging"
	"github.com/cjmartian/agent-deploy/internal/spending"
	"github.com/cjmartian/agent-deploy/internal/state"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	dockerclient "github.com/docker/docker/client"
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
		Lightsail:      lightsail.NewFromConfig(cfg), // P1.34: Low-cost container deployments
	}
}

func (p *AWSProvider) Name() string { return "aws" }

// checkStore validates that the store is initialized.
// Returns ErrInvalidState if the store is nil, preventing nil pointer panics.
// WHY: If store initialization fails or is deferred, provider methods must not panic.
// Instead they should return a clear error that can be handled by callers.
func (p *AWSProvider) checkStore() error {
	if p.store == nil {
		return fmt.Errorf("%w: state store is not initialized", apperrors.ErrInvalidState)
	}
	return nil
}

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
	// Auto-scaling parameters (P1.22): Allow cost range calculation during planning.
	// WHY: Users need to understand min/max cost impact before committing to auto-scaling.
	MinCount int `json:"min_count,omitempty" jsonschema:"minimum task count for auto scaling (default: 1)"`
	MaxCount int `json:"max_count,omitempty" jsonschema:"maximum task count for auto scaling (default: same as min_count, no scaling)"`
	// Per-request spending override (P1.21): Allow deployment-specific budget caps.
	// WHY: Users may want different budget limits for different deployments.
	PerDeploymentBudgetUSD float64 `json:"per_deployment_budget_usd,omitempty" jsonschema:"maximum monthly cost for this deployment (overrides global config)"`
	// VPC CIDR configuration (P1.9): Allow custom VPC CIDR for VPC peering scenarios.
	// WHY: Default 10.0.0.0/16 may conflict with existing VPCs in peering scenarios.
	VpcCIDR string `json:"vpc_cidr,omitempty" jsonschema:"VPC CIDR block (default: 10.0.0.0/16). Must be /16 to /24."`
	// Custom DNS (P1.29): Optional custom domain name for user-friendly URLs.
	// WHY: ALB-generated DNS names are opaque and unsuitable for production use.
	DomainName string `json:"domain_name,omitempty" jsonschema:"custom domain name (e.g. app.example.com). Requires Route 53 hosted zone for parent domain."`
}

type planInfraOutput struct {
	PlanID          string     `json:"plan_id"`
	Services        []string   `json:"services"`
	EstimatedCostMo string     `json:"estimated_cost_monthly"`
	Summary         string     `json:"summary"`
	// Cost range fields (P1.22): Show min/max costs when auto-scaling is configured.
	// WHY: When max_count > min_count, costs can vary significantly based on load.
	CostRange *costRange `json:"cost_range,omitempty"`
	// Custom domain (P1.29): Show custom domain in plan output if configured.
	CustomDomain string `json:"custom_domain,omitempty"`
	// Spending confirmation (P1.36): Warn when using default limits.
	// WHY: Spec requires user confirmation when no spending limits are explicitly configured.
	RequiresConfirmation bool   `json:"requires_confirmation,omitempty"`
	ConfirmationReason   string `json:"confirmation_reason,omitempty"`
}

// costRange represents the minimum and maximum monthly cost range for auto-scaling deployments.
// WHY (P1.22): Users need to understand worst-case costs when auto-scaling is configured.
type costRange struct {
	MinimumCostMo float64 `json:"minimum_monthly"`
	MaximumCostMo float64 `json:"maximum_monthly"`
	Note          string  `json:"note"`
}

type createInfraInput struct {
	PlanID           string `json:"plan_id"             jsonschema:"the plan ID returned by aws_plan_infra"`
	LogRetentionDays int    `json:"log_retention_days,omitempty" jsonschema:"CloudWatch log retention in days (default: 7). Valid: 1,3,5,7,14,30,60,90,120,150,180,365,400,545,731,1096,1827,2192,2557,2922,3288,3653"`
	CertificateARN   string `json:"certificate_arn,omitempty" jsonschema:"ACM certificate ARN for HTTPS (optional). When provided, creates HTTPS listener on port 443 and redirects HTTP to HTTPS"`
	// Per-request spending override (P1.21): Allow deployment-specific budget cap at create time.
	// WHY: User may want to enforce a tighter budget than the global config for specific infrastructure.
	PerDeploymentBudgetUSD float64 `json:"per_deployment_budget_usd,omitempty" jsonschema:"maximum monthly cost for this deployment (overrides global config)"`
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
	MinCount         int `json:"min_count,omitempty"           jsonschema:"minimum task count for auto scaling (default: same as desired_count)"`
	MaxCount         int `json:"max_count,omitempty"           jsonschema:"maximum task count for auto scaling (default: same as desired_count, no scaling)"`
	TargetCPUPercent int `json:"target_cpu_percent,omitempty"  jsonschema:"target CPU utilization percentage for scaling (default: 70)"`
	TargetMemPercent int `json:"target_memory_percent,omitempty" jsonschema:"target memory utilization percentage for scaling (default: 70)"`
	// Container health check parameters (P1.28).
	// WHY: ECS container health checks detect unhealthy tasks independently of ALB.
	// If a container becomes unhealthy, ECS will stop and replace it automatically.
	HealthCheckGracePeriod int `json:"health_check_grace_period,omitempty" jsonschema:"seconds before ECS starts checking container health (default: 60)"`
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
	MinCount         int `json:"min_count"`
	MaxCount         int `json:"max_count"`
	CurrentCount     int `json:"current_count"`
	TargetCPUPercent int `json:"target_cpu_percent"`
	TargetMemPercent int `json:"target_memory_percent"`
}

type statusOutput struct {
	DeploymentID string       `json:"deployment_id"`
	Status       string       `json:"status"`
	URLs         []string     `json:"urls"`
	CustomDomain string       `json:"custom_domain,omitempty"` // P1.35: Include custom domain in status output
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

// --- input validation functions (per spec ralph/specs/deploy-configuration.md) ---

// validFargateConfigs defines the valid CPU/memory combinations for Fargate.
// Per AWS documentation: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-cpu-memory-error.html
var validFargateConfigs = map[string][]string{
	"256":  {"512", "1024", "2048"},
	"512":  {"1024", "2048", "3072", "4096"},
	"1024": {"2048", "3072", "4096", "5120", "6144", "7168", "8192"},
	"2048": {"4096", "5120", "6144", "7168", "8192", "9216", "10240", "11264", "12288", "13312", "14336", "15360", "16384"},
	"4096": {"8192", "9216", "10240", "11264", "12288", "13312", "14336", "15360", "16384", "17408", "18432", "19456", "20480", "21504", "22528", "23552", "24576", "25600", "26624", "27648", "28672", "29696", "30720"},
}

// ValidateFargateResources checks that the CPU/memory combination is valid for Fargate.
func ValidateFargateResources(cpu, memory string) error {
	validMemory, cpuValid := validFargateConfigs[cpu]
	if !cpuValid {
		return fmt.Errorf("invalid Fargate CPU value: %q. Valid values: 256, 512, 1024, 2048, 4096", cpu)
	}

	for _, vm := range validMemory {
		if vm == memory {
			return nil
		}
	}

	return fmt.Errorf("invalid Fargate CPU/memory combination: CPU=%s, Memory=%s. Valid memory values for CPU %s: %v",
		cpu, memory, cpu, validMemory)
}

// validLogRetentionDays contains the CloudWatch-accepted retention values.
var validLogRetentionDays = map[int]bool{
	1: true, 3: true, 5: true, 7: true, 14: true, 30: true,
	60: true, 90: true, 120: true, 150: true, 180: true, 365: true,
	400: true, 545: true, 731: true, 1096: true, 1827: true,
	2192: true, 2557: true, 2922: true, 3288: true, 3653: true,
}

// ValidateLogRetention checks that the log retention days value is accepted by CloudWatch.
func ValidateLogRetention(days int) error {
	if validLogRetentionDays[days] {
		return nil
	}
	return fmt.Errorf("invalid log_retention_days: %d. Valid values: 1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653", days)
}

// ValidateContainerPort checks that the port is in valid range (1-65535).
func ValidateContainerPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid container_port: %d. Must be between 1 and 65535", port)
	}
	return nil
}

// ValidateHealthCheckPath checks that the health check path starts with /.
func ValidateHealthCheckPath(path string) error {
	if path == "" {
		return nil // Empty path uses default "/"
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("invalid health_check_path: %q. Must start with /", path)
	}
	return nil
}

// validAWSRegions contains valid AWS region codes.
var validAWSRegions = map[string]bool{
	"us-east-1": true, "us-east-2": true, "us-west-1": true, "us-west-2": true,
	"eu-west-1": true, "eu-west-2": true, "eu-west-3": true, "eu-central-1": true,
	"eu-north-1": true, "eu-south-1": true,
	"ap-northeast-1": true, "ap-northeast-2": true, "ap-northeast-3": true,
	"ap-southeast-1": true, "ap-southeast-2": true, "ap-south-1": true,
	"sa-east-1":    true,
	"ca-central-1": true,
	"me-south-1":   true,
	"af-south-1":   true,
}

// ValidateAWSRegion checks that the region is a valid AWS region code.
func ValidateAWSRegion(region string) error {
	if validAWSRegions[region] {
		return nil
	}
	return fmt.Errorf("invalid AWS region: %q; use a valid region code like 'us-east-1', 'eu-west-1', etc", region)
}

// ValidateDesiredCount checks that the desired task count is reasonable.
// Upper limit prevents accidental runaway costs.
func ValidateDesiredCount(count int) error {
	if count < 1 {
		return fmt.Errorf("invalid desired_count: %d. Must be at least 1", count)
	}
	if count > 100 {
		return fmt.Errorf("desired_count %d exceeds maximum of 100. For higher counts, use auto-scaling with max_count instead", count)
	}
	return nil
}

// envVarNameRegex matches valid environment variable names.
// AWS ECS requires: letters, digits, underscores; must start with letter or underscore.
var envVarNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// reservedEnvVars are AWS-reserved environment variable prefixes that users shouldn't set.
var reservedEnvVars = []string{
	"AWS_", "ECS_", "FARGATE_",
}

// ValidateEnvironmentVariables checks that environment variable names are valid.
func ValidateEnvironmentVariables(env map[string]string) error {
	for name := range env {
		// Check format.
		if !envVarNameRegex.MatchString(name) {
			return fmt.Errorf("invalid environment variable name: %q. Must contain only letters, digits, and underscores, and start with a letter or underscore", name)
		}
		// Check for reserved prefixes.
		for _, prefix := range reservedEnvVars {
			if strings.HasPrefix(name, prefix) {
				return fmt.Errorf("environment variable %q uses reserved prefix %q. AWS ECS reserves variables starting with AWS_, ECS_, and FARGATE_", name, prefix)
			}
		}
	}
	return nil
}

// ValidateVpcCIDR validates a VPC CIDR block (P1.9).
// WHY: Invalid CIDR blocks will cause AWS API errors; validating early provides better UX.
func ValidateVpcCIDR(cidr string) error {
	if cidr == "" {
		return nil // Empty = use default
	}
	// Parse CIDR.
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid vpc_cidr: %q. Must be a valid IPv4 CIDR block (e.g., 10.0.0.0/16)", cidr)
	}
	// Check it's IPv4.
	if ipnet.IP.To4() == nil {
		return fmt.Errorf("invalid vpc_cidr: %q. Must be an IPv4 CIDR block, not IPv6", cidr)
	}
	// Check prefix length is between /16 and /24.
	ones, _ := ipnet.Mask.Size()
	if ones < 16 || ones > 24 {
		return fmt.Errorf("invalid vpc_cidr: %q. Prefix length must be between /16 and /24 (got /%d)", cidr, ones)
	}
	return nil
}

// ValidateDomainName validates a custom domain name for Route 53 (P1.29).
// WHY: Invalid domain names will cause Route 53 API errors; validating early provides better UX.
func ValidateDomainName(domain string) error {
	if domain == "" {
		return nil // Empty = no custom domain
	}
	// RFC 1123 compliant domain name validation.
	// Domain must be 1-253 chars, labels 1-63 chars, alphanumeric with hyphens.
	if len(domain) > 253 {
		return fmt.Errorf("invalid domain_name: %q. Domain name must be <= 253 characters", domain)
	}
	// Domain name pattern: labels separated by dots.
	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain_name: %q. Must be a valid domain name (e.g., app.example.com)", domain)
	}
	// Must have at least 2 labels (subdomain.tld).
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return fmt.Errorf("invalid domain_name: %q. Must include at least a subdomain and TLD (e.g., app.example.com)", domain)
	}
	return nil
}

// ValidateID validates a ULID-based identifier format (P1.31).
// WHY: Invalid IDs can cause lookup failures or security issues; validating format catches bugs early.
// Valid formats: "plan-{ULID}", "infra-{ULID}", "deploy-{ULID}"
// For backwards compatibility, also accepts legacy short-form IDs like "plan-test-001".
func ValidateID(id, expectedPrefix string) error {
	if id == "" {
		return fmt.Errorf("invalid %s_id: cannot be empty", expectedPrefix)
	}
	// Check prefix
	if !strings.HasPrefix(id, expectedPrefix+"-") {
		return fmt.Errorf("invalid %s_id: %q. Must start with %q prefix", expectedPrefix, id, expectedPrefix+"-")
	}
	// Extract ID part after prefix
	idPart := strings.TrimPrefix(id, expectedPrefix+"-")
	if idPart == "" {
		return fmt.Errorf("invalid %s_id: %q. Missing ID portion after prefix", expectedPrefix, id)
	}
	// ULID is 26 characters (Crockford's Base32)
	// If it's exactly 26 chars, validate as ULID format
	if len(idPart) == 26 {
		ulidRegex := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
		if !ulidRegex.MatchString(idPart) {
			return fmt.Errorf("invalid %s_id: %q. ULID contains invalid characters", expectedPrefix, id)
		}
	}
	// For non-ULID IDs (legacy format), just ensure they're alphanumeric with allowed chars
	idRegex := regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)
	if !idRegex.MatchString(idPart) {
		return fmt.Errorf("invalid %s_id: %q. ID contains invalid characters", expectedPrefix, id)
	}
	return nil
}

// ValidateImageRef validates a Docker image reference (P1.31).
// WHY: Invalid image refs will cause ECS task failures; validating early provides better UX.
// Valid formats: "nginx", "nginx:latest", "user/repo:tag", "registry.io/user/repo:tag@sha256:..."
func ValidateImageRef(imageRef string) error {
	if imageRef == "" {
		return fmt.Errorf("invalid image_ref: cannot be empty. Provide a Docker image reference (e.g., nginx:latest, ghcr.io/user/repo:tag)")
	}
	// Max length check (Docker has a practical limit around 2048 chars)
	if len(imageRef) > 2048 {
		return fmt.Errorf("invalid image_ref: %q... Image reference too long (max 2048 characters)", imageRef[:50])
	}
	// Basic structure validation: [registry/][user/]repo[:tag][@digest]
	// This is a permissive check - AWS/Docker will do final validation
	imageRefRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._\-/:@]*[a-zA-Z0-9])?$`)
	if !imageRefRegex.MatchString(imageRef) {
		return fmt.Errorf("invalid image_ref: %q. Must be a valid Docker image reference", imageRef)
	}
	return nil
}

// ValidateAppDescription validates the application description length (P1.31).
// WHY: Unbounded descriptions could cause display/storage issues; reasonable limit improves UX.
func ValidateAppDescription(desc string) error {
	const maxLength = 1024
	if len(desc) > maxLength {
		return fmt.Errorf("invalid app_description: too long (%d characters). Maximum is %d characters", len(desc), maxLength)
	}
	return nil
}

// ValidateExpectedUsers validates the expected users count (P1.31).
// WHY: Unreasonable values could cause cost estimation issues; upper bound ensures sensible input.
func ValidateExpectedUsers(users int) error {
	const maxUsers = 100_000_000 // 100 million is reasonable upper bound for planning
	if users <= 0 {
		return fmt.Errorf("invalid expected_users: %d. Must be a positive integer", users)
	}
	if users > maxUsers {
		return fmt.Errorf("invalid expected_users: %d. Maximum is %d (100 million)", users, maxUsers)
	}
	return nil
}

// ValidateLatencyMS validates the target latency in milliseconds (P1.31).
// WHY: Unreasonable latency targets could indicate input errors; bounds ensure sensible values.
func ValidateLatencyMS(latencyMS int) error {
	const minLatency = 1
	const maxLatency = 60000 // 60 seconds is reasonable upper bound
	if latencyMS < minLatency {
		return fmt.Errorf("invalid latency_ms: %d. Minimum is %d ms", latencyMS, minLatency)
	}
	if latencyMS > maxLatency {
		return fmt.Errorf("invalid latency_ms: %d. Maximum is %d ms (60 seconds)", latencyMS, maxLatency)
	}
	return nil
}

// ValidateCertificateARNRegion validates that a certificate ARN matches the deployment region (P1.31).
// WHY: ACM certificates are regional; using a cert from wrong region causes provisioning failure.
func ValidateCertificateARNRegion(certARN, deploymentRegion string) error {
	if certARN == "" {
		return nil // No certificate provided
	}
	// ARN format: arn:aws:acm:REGION:ACCOUNT:certificate/ID
	parts := strings.Split(certARN, ":")
	if len(parts) < 4 {
		return fmt.Errorf("invalid certificate_arn format: %q", certARN)
	}
	certRegion := parts[3]
	if certRegion != deploymentRegion {
		return fmt.Errorf("certificate_arn region mismatch: certificate is in %q but deployment is in %q. ACM certificates must be in the same region as the deployment", certRegion, deploymentRegion)
	}
	return nil
}

// extractParentDomain extracts the parent domain from a subdomain (P1.29).
// For "app.example.com" returns "example.com".
// For "example.com" returns "example.com" (apex record).
func extractParentDomain(domain string) string {
	labels := strings.Split(domain, ".")
	if len(labels) <= 2 {
		return domain // Already at parent level (e.g., example.com)
	}
	return strings.Join(labels[1:], ".")
}

// SubnetLayout represents the calculated subnet CIDRs derived from a VPC CIDR.
// WHY (P1.9): Dynamic subnet calculation allows custom VPC CIDRs for peering scenarios.
type SubnetLayout struct {
	VpcCIDR      string   // The VPC CIDR being used
	PublicCIDRs  []string // 2 public subnet CIDRs
	PrivateCIDRs []string // 2 private subnet CIDRs
}

// CalculateSubnetLayout derives 4 subnet CIDRs from a VPC CIDR (P1.9).
// WHY: When a custom VPC CIDR is provided, subnet CIDRs must be derived dynamically.
// Layout for /16 CIDR X.Y.0.0/16:
//   - Public:  X.Y.1.0/24, X.Y.2.0/24
//   - Private: X.Y.10.0/24, X.Y.11.0/24
func CalculateSubnetLayout(vpcCIDR string) (*SubnetLayout, error) {
	if vpcCIDR == "" {
		vpcCIDR = "10.0.0.0/16"
	}
	// Parse VPC CIDR.
	ip, ipnet, err := net.ParseCIDR(vpcCIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid VPC CIDR: %w", err)
	}
	// Get base IP as 4 bytes.
	baseIP := ip.To4()
	if baseIP == nil {
		return nil, fmt.Errorf("VPC CIDR must be IPv4")
	}

	// Calculate subnet CIDRs.
	// For a /16 like 10.0.0.0/16:
	//   Public:  10.0.1.0/24, 10.0.2.0/24
	//   Private: 10.0.10.0/24, 10.0.11.0/24
	// For other prefix lengths, we adjust the third octet similarly.
	ones, bits := ipnet.Mask.Size()
	if ones > 24 || bits != 32 {
		return nil, fmt.Errorf("VPC CIDR must have prefix length /16 to /24")
	}

	layout := &SubnetLayout{
		VpcCIDR:      vpcCIDR,
		PublicCIDRs:  make([]string, 2),
		PrivateCIDRs: make([]string, 2),
	}

	// Public subnets: .1.0/24 and .2.0/24
	for i := 0; i < 2; i++ {
		subnetIP := net.IPv4(baseIP[0], baseIP[1], byte(i+1), 0)
		layout.PublicCIDRs[i] = fmt.Sprintf("%s/24", subnetIP.String())
	}

	// Private subnets: .10.0/24 and .11.0/24
	for i := 0; i < 2; i++ {
		subnetIP := net.IPv4(baseIP[0], baseIP[1], byte(i+10), 0)
		layout.PrivateCIDRs[i] = fmt.Sprintf("%s/24", subnetIP.String())
	}

	return layout, nil
}

// --- tool handlers ---

// planInfra analyzes requirements and creates an infrastructure plan with cost estimate.
func (p *AWSProvider) planInfra(ctx context.Context, _ *mcp.CallToolRequest, in planInfraInput) (*mcp.CallToolResult, planInfraOutput, error) {
	// P0.3: Guard against nil store to prevent panic.
	if err := p.checkStore(); err != nil {
		return nil, planInfraOutput{}, err
	}

	// Validate input.
	if strings.TrimSpace(in.AppDescription) == "" {
		return nil, planInfraOutput{}, fmt.Errorf("app_description is required and cannot be empty")
	}
	// Validate app_description length (P1.31).
	if err := ValidateAppDescription(in.AppDescription); err != nil {
		return nil, planInfraOutput{}, err
	}
	if strings.TrimSpace(in.Region) == "" {
		return nil, planInfraOutput{}, fmt.Errorf("region is required and cannot be empty")
	}
	// Validate AWS region (P1.26).
	if err := ValidateAWSRegion(in.Region); err != nil {
		return nil, planInfraOutput{}, err
	}
	// Validate expected_users bounds (P1.31).
	if err := ValidateExpectedUsers(in.ExpectedUsers); err != nil {
		return nil, planInfraOutput{}, err
	}
	// Validate latency_ms bounds (P1.31).
	if err := ValidateLatencyMS(in.LatencyMS); err != nil {
		return nil, planInfraOutput{}, err
	}

	// Validate VPC CIDR (P1.9).
	if err := ValidateVpcCIDR(in.VpcCIDR); err != nil {
		return nil, planInfraOutput{}, err
	}
	// Use default if not specified.
	vpcCIDR := in.VpcCIDR
	if vpcCIDR == "" {
		vpcCIDR = "10.0.0.0/16"
	}

	// Validate domain name (P1.29).
	if err := ValidateDomainName(in.DomainName); err != nil {
		return nil, planInfraOutput{}, err
	}
	domainName := strings.TrimSpace(in.DomainName)

	// Apply auto-scaling defaults (P1.22).
	// WHY: Allow users to specify scaling during planning to see cost range.
	minCount := in.MinCount
	if minCount <= 0 {
		minCount = 1
	}
	maxCount := in.MaxCount
	if maxCount <= 0 {
		maxCount = minCount // Default: no auto-scaling
	}
	if maxCount < minCount {
		return nil, planInfraOutput{}, fmt.Errorf("max_count (%d) must be >= min_count (%d)", maxCount, minCount)
	}
	autoScalingEnabled := maxCount > minCount

	// P1.34: Select backend based on workload signals.
	// WHY: Lightsail is cheaper ($7-25/mo) for simple apps; ECS Fargate for production workloads.
	backend := selectBackend(in.ExpectedUsers, autoScalingEnabled, in.AppDescription)

	// Select services based on backend and requirements.
	var services []string
	var estimatedCost float64
	var costEstimate *spending.CostEstimate

	if backend == state.BackendLightsail {
		// Lightsail: simple fixed pricing based on power level.
		power := selectLightsailPower(in.ExpectedUsers)
		nodes := calculateLightsailNodes(in.ExpectedUsers)
		pricePerNode := lightsailPowerPricing[power]
		estimatedCost = pricePerNode * float64(nodes)

		services = []string{
			fmt.Sprintf("Lightsail Container (%s)", power),
			"Built-in HTTPS/TLS",
			"Built-in Load Balancing",
		}
		if nodes > 1 {
			services = append(services, fmt.Sprintf("%d Nodes", nodes))
		}

		// Build cost estimate for Lightsail.
		costEstimate = &spending.CostEstimate{
			TotalMonthlyUSD: estimatedCost,
			Region:          in.Region,
			UsingFallback:   false,
			Services: []spending.ServiceCost{
				{
					Service:     "Lightsail Container",
					Description: fmt.Sprintf("%s power × %d node(s)", power, nodes),
					MonthlyCost: estimatedCost,
				},
			},
		}

		slog.Info("selected Lightsail backend",
			slog.String("component", "aws_plan_infra"),
			slog.String("power", string(power)),
			slog.Int("nodes", int(nodes)),
			logging.Cost(estimatedCost))
	} else {
		// ECS Fargate: use pricing estimator.
		services = []string{"VPC", "ECS Fargate", "ALB", "CloudWatch Logs"}
		// Add Auto Scaling to services if enabled via parameters OR high expected users.
		if autoScalingEnabled || in.ExpectedUsers > 1000 {
			services = append(services, "Auto Scaling")
		}

		// Per spec ralph/specs/cost-estimation.md: Use PricingEstimator for accurate cost estimates.
		// Falls back to hardcoded regional estimates if Pricing API is unavailable.
		estimator, err := spending.NewPricingEstimator(ctx)
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
				CPUUnits:          256, // Default for planning
				MemoryMB:          512, // Default for planning
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

		estimatedCost = costEstimate.TotalMonthlyUSD
	}

	// Calculate cost range for auto-scaling (P1.22).
	// WHY: Users need to understand min/max costs before committing to auto-scaling.
	var costRangeOutput *costRange
	var minCostMo, maxCostMo float64

	// Cost range only applies to ECS Fargate with auto-scaling.
	if backend == state.BackendECSFargate && autoScalingEnabled {
		// Get per-task cost by assuming single task cost = total cost for 1 task.
		// Subtract fixed infrastructure costs (ALB, NAT Gateway) to get per-task cost.
		fixedCosts := 35.0 // Approximate: ALB ($20) + NAT Gateway ($15)
		perTaskCost := estimatedCost - fixedCosts
		if perTaskCost < 10 {
			perTaskCost = 10 // Minimum reasonable per-task cost
		}

		minCostMo = fixedCosts + (perTaskCost * float64(minCount))
		maxCostMo = fixedCosts + (perTaskCost * float64(maxCount))
		costRangeOutput = &costRange{
			MinimumCostMo: minCostMo,
			MaximumCostMo: maxCostMo,
			Note:          fmt.Sprintf("Range reflects auto scaling from %d to %d tasks", minCount, maxCount),
		}
		// Update the single estimate to show range in text.
		estimatedCost = minCostMo // Use minimum as the base estimate
	}

	// Check spending limits before creating plan.
	// WHY: Check against max cost when auto-scaling is enabled to prevent budget overruns.
	limitsWithSource, _ := spending.LoadLimitsWithSource()
	limits := limitsWithSource.Limits

	// P1.36: Track if confirmation is needed (using default limits without explicit config).
	requiresConfirmation := !limitsWithSource.ExplicitlyConfigured
	var confirmationReason string
	if requiresConfirmation {
		confirmationReason = "No spending limits configured. Using defaults: $100/mo monthly budget, $25/deployment limit. Configure limits in ~/.agent-deploy/config.json or via AGENT_DEPLOY_* environment variables."
	}

	// Per-request spending override (P1.21): Use provided limit if valid.
	// WHY: Allow deployment-specific budget caps that may be tighter than global limits.
	perDeploymentLimit := limits.PerDeploymentUSD
	if in.PerDeploymentBudgetUSD > 0 {
		// Validate override doesn't exceed global limit.
		if in.PerDeploymentBudgetUSD > limits.PerDeploymentUSD {
			return nil, planInfraOutput{}, fmt.Errorf("per_deployment_budget_usd ($%.2f) exceeds global per-deployment limit ($%.2f)", in.PerDeploymentBudgetUSD, limits.PerDeploymentUSD)
		}
		perDeploymentLimit = in.PerDeploymentBudgetUSD
		// Per-request override counts as explicit configuration.
		requiresConfirmation = false
		confirmationReason = ""
		slog.Info("using per-request spending override",
			slog.String("component", "aws_plan_infra"),
			logging.Cost(perDeploymentLimit))
	}

	costToCheck := estimatedCost
	if autoScalingEnabled {
		costToCheck = maxCostMo // Check max cost against limit
	}
	if costToCheck > perDeploymentLimit {
		if autoScalingEnabled {
			return nil, planInfraOutput{}, fmt.Errorf("maximum estimated cost $%.2f/mo (at %d tasks) exceeds per-deployment limit of $%.2f", maxCostMo, maxCount, perDeploymentLimit)
		}
		return nil, planInfraOutput{}, fmt.Errorf("estimated cost $%.2f/mo exceeds per-deployment limit of $%.2f", estimatedCost, perDeploymentLimit)
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
		VpcCIDR:         vpcCIDR,    // P1.9: Store VPC CIDR for use in createInfra
		DomainName:      domainName, // P1.29: Store custom domain for createInfra
		Backend:         backend,    // P1.34: Store selected backend (lightsail or ecs-fargate)
	}

	if err := p.store.CreatePlan(plan); err != nil {
		return nil, planInfraOutput{}, fmt.Errorf("save plan: %w", err)
	}

	slog.Info("created plan",
		slog.String("component", "aws_plan_infra"),
		logging.PlanID(plan.ID),
		slog.String("app_description", in.AppDescription),
		slog.String("vpc_cidr", vpcCIDR),
		slog.String("domain_name", domainName),
		slog.String("backend", backend),
		logging.Cost(estimatedCost),
		slog.Bool("using_fallback_pricing", costEstimate.UsingFallback),
		slog.Bool("auto_scaling_enabled", autoScalingEnabled))

	// Build detailed summary including cost breakdown.
	var summaryBuilder string
	if backend == state.BackendLightsail {
		power := selectLightsailPower(in.ExpectedUsers)
		nodes := calculateLightsailNodes(in.ExpectedUsers)
		summaryBuilder = fmt.Sprintf(
			"Proposed plan for %q: Lightsail Container (%s × %d nodes) in %s, targeting %d users at ≤%dms p99. Estimated cost: $%.2f/mo. Plan ID: %s (expires in 24h).\n\n",
			in.AppDescription, power, nodes, in.Region, in.ExpectedUsers, in.LatencyMS, estimatedCost, plan.ID,
		)
		summaryBuilder += fmt.Sprintf("💡 Using Lightsail for cost efficiency ($%.2f/mo vs ~$65/mo with ECS Fargate)\n\n", estimatedCost)
	} else if autoScalingEnabled {
		summaryBuilder = fmt.Sprintf(
			"Proposed plan for %q: ECS Fargate in %s with auto-scaling (%d–%d tasks), targeting %d users at ≤%dms p99. Estimated cost: $%.2f–$%.2f/mo. Plan ID: %s (expires in 24h).\n\n",
			in.AppDescription, in.Region, minCount, maxCount, in.ExpectedUsers, in.LatencyMS, minCostMo, maxCostMo, plan.ID,
		)
	} else {
		summaryBuilder = fmt.Sprintf(
			"Proposed plan for %q: ECS Fargate in %s, targeting %d users at ≤%dms p99. Estimated cost: $%.2f/mo. Plan ID: %s (expires in 24h).\n\n",
			in.AppDescription, in.Region, in.ExpectedUsers, in.LatencyMS, estimatedCost, plan.ID,
		)
	}
	// Add custom domain info to summary (P1.29).
	if domainName != "" {
		summaryBuilder += fmt.Sprintf("Custom domain: %s (requires Route 53 hosted zone for %s)\n\n", domainName, extractParentDomain(domainName))
	}
	if len(costEstimate.Services) > 0 {
		summaryBuilder += "Cost breakdown:\n"
		for _, svc := range costEstimate.Services {
			if svc.MonthlyCost > 0 {
				summaryBuilder += fmt.Sprintf("  - %s (%s): $%.2f/mo\n", svc.Service, svc.Description, svc.MonthlyCost)
			}
		}
		summaryBuilder += "\n"
	}
	if autoScalingEnabled {
		summaryBuilder += fmt.Sprintf("Auto-scaling range: %d–%d tasks (costs scale with load)\n\n", minCount, maxCount)
	}
	if costEstimate.Disclaimer != "" {
		summaryBuilder += "Note: " + costEstimate.Disclaimer + "\n\n"
	}
	// P1.36: Add warning when using default limits.
	if requiresConfirmation {
		summaryBuilder += "⚠️ WARNING: No spending limits configured. Using defaults ($100/mo budget, $25/deployment). Configure limits to remove this warning.\n\n"
	}
	summaryBuilder += "⚠️ Review the cost estimate above. Call aws_approve_plan with plan_id and confirmed: true to approve, then aws_create_infra to provision infrastructure."

	// Format estimated cost display.
	var estimatedCostDisplay string
	if autoScalingEnabled {
		estimatedCostDisplay = fmt.Sprintf("$%.2f–$%.2f", minCostMo, maxCostMo)
	} else {
		estimatedCostDisplay = fmt.Sprintf("$%.2f", estimatedCost)
	}

	output := planInfraOutput{
		PlanID:               plan.ID,
		Services:             services,
		EstimatedCostMo:      estimatedCostDisplay,
		Summary:              summaryBuilder,
		CostRange:            costRangeOutput,
		RequiresConfirmation: requiresConfirmation,
		ConfirmationReason:   confirmationReason,
	}
	// P1.29: Include custom domain in output if configured.
	if domainName != "" {
		output.CustomDomain = domainName
	}

	return nil, output, nil
}

// approvePlan allows the user to approve or reject an infrastructure plan after review.
// Per spec ralph/specs/plan-approval.md: explicit approval is required before provisioning.
func (p *AWSProvider) approvePlan(_ context.Context, _ *mcp.CallToolRequest, in approvePlanInput) (*mcp.CallToolResult, approvePlanOutput, error) {
	// P0.3: Guard against nil store to prevent panic.
	if err := p.checkStore(); err != nil {
		return nil, approvePlanOutput{}, err
	}

	// Validate input.
	if strings.TrimSpace(in.PlanID) == "" {
		return nil, approvePlanOutput{}, fmt.Errorf("plan_id is required and cannot be empty")
	}
	// Validate plan_id format (P1.31).
	if err := ValidateID(in.PlanID, "plan"); err != nil {
		return nil, approvePlanOutput{}, err
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
	// P0.3: Guard against nil store to prevent panic.
	if err := p.checkStore(); err != nil {
		return nil, createInfraOutput{}, err
	}

	// Validate plan_id format (P1.31).
	if strings.TrimSpace(in.PlanID) == "" {
		return nil, createInfraOutput{}, fmt.Errorf("plan_id is required and cannot be empty")
	}
	if err := ValidateID(in.PlanID, "plan"); err != nil {
		return nil, createInfraOutput{}, err
	}

	// Validate log retention if provided (P1.20).
	if in.LogRetentionDays != 0 {
		if err := ValidateLogRetention(in.LogRetentionDays); err != nil {
			return nil, createInfraOutput{}, err
		}
	}

	// Get and validate plan.
	plan, err := p.store.GetPlan(in.PlanID)
	if err != nil {
		return nil, createInfraOutput{}, err
	}

	// Validate certificate ARN region matches deployment region (P1.31).
	if in.CertificateARN != "" {
		if err := ValidateCertificateARNRegion(in.CertificateARN, plan.Region); err != nil {
			return nil, createInfraOutput{}, err
		}
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
		// P0.3: Log errors instead of silently ignoring with _, _
		deployments, listDeployErr := p.store.ListDeployments()
		if listDeployErr != nil {
			slog.Warn("could not list deployments for cost estimate",
				slog.String("component", "aws_create_infra"),
				logging.Err(listDeployErr))
		}
		plans, listPlanErr := p.store.ListPlans()
		if listPlanErr != nil {
			slog.Warn("could not list plans for cost estimate",
				slog.String("component", "aws_create_infra"),
				logging.Err(listPlanErr))
		}
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

	// Per-request spending override (P1.21): Use provided limit if valid.
	// WHY: Allow deployment-specific budget caps at create time.
	effectiveLimits := limits
	if in.PerDeploymentBudgetUSD > 0 {
		// Validate override doesn't exceed global limit.
		if in.PerDeploymentBudgetUSD > limits.PerDeploymentUSD {
			return nil, createInfraOutput{}, fmt.Errorf("per_deployment_budget_usd ($%.2f) exceeds global per-deployment limit ($%.2f)", in.PerDeploymentBudgetUSD, limits.PerDeploymentUSD)
		}
		// Create a copy of limits with the override.
		effectiveLimits = spending.Limits{
			MonthlyBudgetUSD:      limits.MonthlyBudgetUSD,
			PerDeploymentUSD:      in.PerDeploymentBudgetUSD,
			AlertThresholdPercent: limits.AlertThresholdPercent,
		}
		slog.Info("using per-request spending override",
			slog.String("component", "aws_create_infra"),
			logging.Cost(in.PerDeploymentBudgetUSD))
	}

	check := spending.CheckBudget(plan.EstimatedCostMo, effectiveLimits, currentSpend)
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
	clients := p.getClients(cfg)

	// P1.34: Branch based on backend selection.
	if plan.Backend == state.BackendLightsail {
		// Lightsail path: simpler, single-resource provisioning.
		slog.Info("provisioning Lightsail infrastructure",
			slog.String("component", "aws_create_infra"),
			logging.InfraID(infraID))

		power := selectLightsailPower(plan.ExpectedUsers)
		nodes := calculateLightsailNodes(plan.ExpectedUsers)

		serviceName, endpoint, err := p.createLightsailService(ctx, clients, infraID, power, nodes)
		if err != nil {
			if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
				slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
			}
			return nil, createInfraOutput{}, fmt.Errorf("%w: provision Lightsail service: %w", apperrors.ErrProvisioningFailed, err)
		}

		// Store Lightsail resources.
		infra.Resources[state.ResourceLightsailService] = serviceName
		infra.Resources[state.ResourceLightsailEndpoint] = endpoint
		infra.Resources[state.ResourceLightsailPower] = string(power)
		infra.Resources[state.ResourceLightsailNodes] = fmt.Sprintf("%d", nodes)

		// Save updated infra resources.
		for k, v := range infra.Resources {
			if err := p.store.UpdateInfraResource(infraID, k, v); err != nil {
				slog.Error("failed to save Lightsail resource", logging.InfraID(infraID), slog.String("resource", k), logging.Err(err))
			}
		}

		// Set status to ready.
		if err := p.store.SetInfraStatus(infraID, state.InfraStatusReady); err != nil {
			return nil, createInfraOutput{}, fmt.Errorf("set infra status: %w", err)
		}

		slog.Info("Lightsail infrastructure ready",
			slog.String("component", "aws_create_infra"),
			logging.InfraID(infraID),
			slog.String("service", serviceName),
			slog.String("endpoint", endpoint))

		return nil, createInfraOutput{
			InfraID: infraID,
			Status:  state.InfraStatusReady,
		}, nil
	}

	// ECS Fargate path: provision VPC, ECS cluster, ALB, etc.

	// Provision resources in order. On failure, rollback already-created resources.
	// WHY: Per spec ralph/specs/error-handling.md - partial failures must clean up
	// to prevent orphaned AWS resources and unexpected costs.
	if err := p.provisionVPC(ctx, cfg, infra, tags, plan.VpcCIDR); err != nil {
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

	// Provision custom DNS if domain name is configured (P1.29).
	if plan.DomainName != "" {
		// Step 1: Find hosted zone.
		hostedZoneID, _, err := p.findHostedZone(ctx, cfg, plan.DomainName)
		if err != nil {
			rollbackErr := p.rollbackInfra(ctx, cfg, infra)
			if rollbackErr != nil {
				slog.Error("rollback failed after hosted zone lookup error", logging.Err(rollbackErr), logging.InfraID(infraID))
			}
			if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
				slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
			}
			return nil, createInfraOutput{}, fmt.Errorf("%w: find hosted zone: %w", apperrors.ErrProvisioningFailed, err)
		}

		// Step 2: Provision certificate if not provided.
		certARN := in.CertificateARN
		if certARN == "" {
			certARN, err = p.provisionCertificate(ctx, cfg, plan.DomainName, hostedZoneID, infra)
			if err != nil {
				rollbackErr := p.rollbackInfra(ctx, cfg, infra)
				if rollbackErr != nil {
					slog.Error("rollback failed after certificate error", logging.Err(rollbackErr), logging.InfraID(infraID))
				}
				if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
					slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
				}
				return nil, createInfraOutput{}, fmt.Errorf("%w: provision certificate: %w", apperrors.ErrProvisioningFailed, err)
			}
		}

		// Step 3: Create ALB HTTPS listener with the certificate (if not already done).
		// Note: provisionALB handles certificate; we need to get ALB DNS name for Route 53.
		albARN := infra.Resources[state.ResourceALB]
		if albARN != "" {
			// Get ALB details for DNS record creation.
			elbClient := elbv2.NewFromConfig(cfg)
			albResp, albErr := elbClient.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
				LoadBalancerArns: []string{albARN},
			})
			if albErr == nil && len(albResp.LoadBalancers) > 0 {
				alb := albResp.LoadBalancers[0]
				if alb.DNSName != nil && alb.CanonicalHostedZoneId != nil {
					// Step 4: Create DNS alias record.
					if err := p.createDNSRecord(ctx, cfg, hostedZoneID, plan.DomainName, *alb.DNSName, *alb.CanonicalHostedZoneId, infra); err != nil {
						rollbackErr := p.rollbackInfra(ctx, cfg, infra)
						if rollbackErr != nil {
							slog.Error("rollback failed after DNS record error", logging.Err(rollbackErr), logging.InfraID(infraID))
						}
						if statusErr := p.store.SetInfraStatus(infraID, state.InfraStatusFailed); statusErr != nil {
							slog.Error("failed to set infra status", "infraID", infraID, "error", statusErr)
						}
						return nil, createInfraOutput{}, fmt.Errorf("%w: create DNS record: %w", apperrors.ErrProvisioningFailed, err)
					}

					// Update TLS status if certificate was provisioned.
					if certARN != "" {
						if storeErr := p.store.UpdateInfraResource(infraID, state.ResourceTLSEnabled, "true"); storeErr != nil {
							slog.Error("failed to set TLS enabled", "infraID", infraID, "error", storeErr)
						}
						infra.Resources[state.ResourceTLSEnabled] = "true"
					}
				}
			}
		}
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
	// P0.3: Guard against nil store to prevent panic.
	if err := p.checkStore(); err != nil {
		return nil, deployOutput{}, err
	}

	// Validate infra_id format (P1.31).
	if strings.TrimSpace(in.InfraID) == "" {
		return nil, deployOutput{}, fmt.Errorf("infra_id is required and cannot be empty")
	}
	if err := ValidateID(in.InfraID, "infra"); err != nil {
		return nil, deployOutput{}, err
	}
	// Validate image_ref format (P1.31).
	if err := ValidateImageRef(in.ImageRef); err != nil {
		return nil, deployOutput{}, err
	}

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

	// Apply defaults for CPU/memory.
	cpu := in.CPU
	if cpu == "" {
		cpu = "256"
	}
	memory := in.Memory
	if memory == "" {
		memory = "512"
	}

	// Validate all input parameters (per spec ralph/specs/deploy-configuration.md).
	if valErr := ValidateContainerPort(containerPort); valErr != nil {
		return nil, deployOutput{}, valErr
	}
	if valErr := ValidateHealthCheckPath(healthCheckPath); valErr != nil {
		return nil, deployOutput{}, valErr
	}
	if valErr := ValidateDesiredCount(desiredCount); valErr != nil {
		return nil, deployOutput{}, valErr
	}
	if valErr := ValidateFargateResources(cpu, memory); valErr != nil {
		return nil, deployOutput{}, valErr
	}
	if valErr := ValidateEnvironmentVariables(in.Environment); valErr != nil {
		return nil, deployOutput{}, valErr
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
	if valErr := validateAutoScalingParams(minCount, maxCount, targetCPU, targetMem); valErr != nil {
		return nil, deployOutput{}, valErr
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

	clients := p.getClients(cfg)

	// P1.34: Check if this is a Lightsail deployment.
	if serviceName, ok := infra.Resources[state.ResourceLightsailService]; ok && serviceName != "" {
		// Lightsail deployment path.
		slog.Info("deploying to Lightsail",
			slog.String("component", "aws_deploy"),
			logging.DeploymentID(deployID),
			slog.String("service", serviceName))

		_, err := p.deployToLightsail(ctx, clients, serviceName, in.ImageRef, containerPort, healthCheckPath, in.Environment)
		if err != nil {
			if statusErr := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusFailed, nil); statusErr != nil {
				slog.Error("failed to update deployment status", "deployID", deployID, "error", statusErr)
			}
			return nil, deployOutput{}, fmt.Errorf("lightsail deployment: %w", err)
		}

		// Get the endpoint URL from infra resources.
		endpoint := infra.Resources[state.ResourceLightsailEndpoint]
		urls := []string{}
		if endpoint != "" {
			urls = append(urls, endpoint)
		}

		// Update deployment status.
		if err := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusRunning, urls); err != nil {
			slog.Error("failed to update deployment status", "deployID", deployID, "error", err)
		}

		slog.Info("Lightsail deployment complete",
			slog.String("component", "aws_deploy"),
			logging.DeploymentID(deployID),
			slog.String("endpoint", endpoint))

		return nil, deployOutput{
			DeploymentID: deployID,
			Status:       state.DeploymentStatusRunning,
		}, nil
	}

	// ECS Fargate deployment path.
	tags := awsclient.ResourceTags("", infra.ID, deployID)

	// Create ECR repository if needed.
	if err = p.ensureECRRepository(ctx, cfg, infra, deployID, tags); err != nil {
		if statusErr := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusFailed, nil); statusErr != nil {
			slog.Error("failed to update deployment status", "deployID", deployID, "error", statusErr)
		}
		return nil, deployOutput{}, fmt.Errorf("ECR setup: %w", err)
	}

	// Push local image to ECR if needed (per spec ralph/specs/ecr-image-push.md).
	// This step detects if the image is a local-only reference and pushes it to ECR.
	imageForTask := in.ImageRef
	if isLocalImage(in.ImageRef) {
		slog.Info("detected local image, pushing to ECR",
			slog.String("component", "aws_deploy"),
			slog.String("imageRef", in.ImageRef),
			logging.DeploymentID(deployID))

		ecrImageURI, pushErr := p.pushImageToECR(ctx, cfg, infra, in.ImageRef, deployID)
		if pushErr != nil {
			if statusErr := p.store.UpdateDeploymentStatus(deployID, state.DeploymentStatusFailed, nil); statusErr != nil {
				slog.Error("failed to update deployment status", "deployID", deployID, "error", statusErr)
			}
			return nil, deployOutput{}, fmt.Errorf("push image to ECR: %w", pushErr)
		}
		imageForTask = ecrImageURI
	}

	// Create ECS task definition.
	// WHY (P1.28): Container health check runs inside ECS, independent of ALB health checks.
	// If a container fails its health check, ECS replaces it even if ALB doesn't detect the issue.
	taskDefARN, err := p.createTaskDefinition(ctx, cfg, infra, imageForTask, deployID, containerPort, in.Environment, in.CPU, in.Memory, healthCheckPath, in.HealthCheckGracePeriod)
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
	// P0.3: Guard against nil store to prevent panic.
	if err := p.checkStore(); err != nil {
		return nil, statusOutput{}, err
	}

	// Validate deployment_id format (P1.31).
	if strings.TrimSpace(in.DeploymentID) == "" {
		return nil, statusOutput{}, fmt.Errorf("deployment_id is required and cannot be empty")
	}
	if err := ValidateID(in.DeploymentID, "deploy"); err != nil {
		return nil, statusOutput{}, err
	}

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
		clients := p.getClients(cfg)

		// P1.34: Check if this is a Lightsail deployment.
		if serviceName, ok := infra.Resources[state.ResourceLightsailService]; ok && serviceName != "" {
			// Lightsail status path.
			status, urls, power, nodes, err := p.getLightsailStatus(ctx, clients, serviceName)
			if err == nil {
				deployment.Status = status
				deployment.URLs = urls
				slog.Debug("Lightsail status retrieved",
					slog.String("component", "aws_status"),
					slog.String("status", status),
					slog.String("power", power),
					slog.Int("nodes", nodes))
			}
		} else {
			// ECS Fargate status path.
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
	}

	// P1.35: Include custom domain in status output when configured.
	customDomain := infra.Resources[state.ResourceDomainName]

	return nil, statusOutput{
		DeploymentID: deployment.ID,
		Status:       deployment.Status,
		URLs:         deployment.URLs,
		CustomDomain: customDomain,
		Scaling:      scaling,
	}, nil
}

// teardown tears down all AWS resources for a deployment.
func (p *AWSProvider) teardown(ctx context.Context, _ *mcp.CallToolRequest, in teardownInput) (*mcp.CallToolResult, teardownOutput, error) {
	// P0.3: Guard against nil store to prevent panic.
	if err := p.checkStore(); err != nil {
		return nil, teardownOutput{}, err
	}

	// Validate deployment_id format (P1.31).
	if strings.TrimSpace(in.DeploymentID) == "" {
		return nil, teardownOutput{}, fmt.Errorf("deployment_id is required and cannot be empty")
	}
	if err := ValidateID(in.DeploymentID, "deploy"); err != nil {
		return nil, teardownOutput{}, err
	}

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

	clients := p.getClients(cfg)

	// P1.34: Check if this is a Lightsail deployment.
	if serviceName, ok := infra.Resources[state.ResourceLightsailService]; ok && serviceName != "" {
		// Lightsail teardown path — simpler, single API call.
		slog.Info("tearing down Lightsail deployment",
			slog.String("component", "aws_teardown"),
			logging.DeploymentID(in.DeploymentID),
			slog.String("service", serviceName))

		if err := p.teardownLightsail(ctx, clients, serviceName); err != nil {
			slog.Warn("failed to delete Lightsail service",
				slog.String("component", "aws_teardown"),
				logging.Err(err))
		}

		// Update state.
		if err := p.store.UpdateDeploymentStatus(in.DeploymentID, state.DeploymentStatusStopped, nil); err != nil {
			slog.Error("failed to update deployment status during teardown", "deploymentID", in.DeploymentID, "error", err)
		}
		if err := p.store.SetInfraStatus(infra.ID, state.InfraStatusDestroyed); err != nil {
			slog.Error("failed to set infra status during teardown", "infraID", infra.ID, "error", err)
		}

		slog.Info("Lightsail deployment torn down",
			slog.String("component", "aws_teardown"),
			logging.DeploymentID(in.DeploymentID))

		return nil, teardownOutput{
			DeploymentID: in.DeploymentID,
			Status:       "destroyed",
		}, nil
	}

	// ECS Fargate teardown path.
	// Delete DNS and certificate resources first (P1.29).
	// WHY: DNS record must be deleted before ALB, certificate before DNS cleanup.
	p.deleteDNSResources(ctx, cfg, infra)

	// Delete auto-scaling configuration BEFORE deleting ECS service.
	// Per spec ralph/specs/auto-scaling.md: must deregister scalable target first.
	clusterName := extractClusterName(infra.Resources[state.ResourceECSCluster])
	serviceName := extractServiceName(deployment.ServiceARN)
	if clusterName != "" && serviceName != "" {
		p.deleteAutoScaling(ctx, cfg, clusterName, serviceName, in.DeploymentID)
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

func (p *AWSProvider) provisionVPC(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, tags map[string]string, vpcCIDR string) error {
	clients := p.getClients(cfg)
	ec2Client := clients.EC2

	// Calculate subnet layout from VPC CIDR (P1.9).
	// WHY: Dynamic subnet calculation allows custom VPC CIDRs for peering scenarios.
	layout, err := CalculateSubnetLayout(vpcCIDR)
	if err != nil {
		return fmt.Errorf("calculate subnet layout: %w", err)
	}

	// Create VPC.
	// Per spec ralph/specs/networking.md: Configurable CIDR (default 10.0.0.0/16).
	vpcResp, err := ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String(layout.VpcCIDR),
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
	// Subnet CIDRs derived from VPC CIDR via CalculateSubnetLayout (P1.9).

	// Create public subnets in 2 AZs (required for ALB).
	var publicSubnetIDs []string
	for i := 0; i < 2; i++ {
		az := *azResp.AvailabilityZones[i].ZoneName
		cidr := layout.PublicCIDRs[i]

		subnetResp, subnetErr := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            aws.String(vpcID),
			CidrBlock:        aws.String(cidr),
			AvailabilityZone: aws.String(az),
			TagSpecifications: []ec2types.TagSpecification{{
				ResourceType: ec2types.ResourceTypeSubnet,
				Tags:         mapToEC2Tags(mergeTags(tags, map[string]string{"Name": fmt.Sprintf("agent-deploy-public-%d", i+1)})),
			}},
		})
		if subnetErr != nil {
			return fmt.Errorf("create public subnet %d: %w", i, subnetErr)
		}
		publicSubnetIDs = append(publicSubnetIDs, *subnetResp.Subnet.SubnetId)

		// Enable auto-assign public IP for public subnets.
		_, subnetErr = ec2Client.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
			SubnetId:            subnetResp.Subnet.SubnetId,
			MapPublicIpOnLaunch: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
		})
		if subnetErr != nil {
			return fmt.Errorf("enable public IP for public subnet %d: %w", i, subnetErr)
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
		cidr := layout.PrivateCIDRs[i]

		subnetResp, subnetErr := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:            aws.String(vpcID),
			CidrBlock:        aws.String(cidr),
			AvailabilityZone: aws.String(az),
			TagSpecifications: []ec2types.TagSpecification{{
				ResourceType: ec2types.ResourceTypeSubnet,
				Tags:         mapToEC2Tags(mergeTags(tags, map[string]string{"Name": fmt.Sprintf("agent-deploy-private-%d", i+1)})),
			}},
		})
		if subnetErr != nil {
			return fmt.Errorf("create private subnet %d: %w", i, subnetErr)
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
		Name:            aws.String("agent-deploy-" + infra.ID[:8]),
		Protocol:        elbv2types.ProtocolEnumHttp,
		Port:            aws.Int32(80),
		VpcId:           aws.String(infra.Resources[state.ResourceVPC]),
		TargetType:      elbv2types.TargetTypeEnumIp,
		HealthCheckPath: aws.String("/"),
		Tags:            mapToELBTags(tags),
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

	repoName := "agent-deploy-" + strings.ToLower(deployID[:12])

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

func (p *AWSProvider) createTaskDefinition(ctx context.Context, cfg aws.Config, infra *state.Infrastructure, imageRef, deployID string, containerPort int, environment map[string]string, cpu, memory, healthCheckPath string, healthCheckGracePeriod int) (string, error) {
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

	// Default health check path if not specified.
	if healthCheckPath == "" {
		healthCheckPath = "/"
	}

	// Default health check grace period if not specified.
	// WHY: Give containers time to start before health checks begin failing them.
	if healthCheckGracePeriod <= 0 {
		healthCheckGracePeriod = 60
	}

	// Build environment variables for the container.
	envVars := make([]ecstypes.KeyValuePair, 0, len(environment))
	for k, v := range environment {
		envVars = append(envVars, ecstypes.KeyValuePair{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	// WHY (P1.28): Container-level health check runs inside the task, independent of ALB.
	// ECS will mark the container as unhealthy and stop/replace the task if health check fails.
	// This catches issues that ALB health checks might miss (e.g., internal deadlocks, memory issues).
	// Using curl to check the health endpoint; wget is an alternative if curl isn't available.
	containerHealthCheck := &ecstypes.HealthCheck{
		Command: []string{
			"CMD-SHELL",
			fmt.Sprintf("curl -f http://localhost:%d%s || exit 1", containerPort, healthCheckPath),
		},
		Interval:    aws.Int32(30),                              // Check every 30 seconds
		Timeout:     aws.Int32(5),                               // Timeout after 5 seconds
		Retries:     aws.Int32(3),                               // Mark unhealthy after 3 consecutive failures
		StartPeriod: aws.Int32(int32(healthCheckGracePeriod)),   // Grace period before checks start
	}

	resp, err := ecsClient.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("agent-deploy-" + strings.ToLower(deployID[:12])),
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
			HealthCheck: containerHealthCheck,
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
		slog.String("log_group_name", logGroupName),
		slog.String("health_check_path", healthCheckPath),
		slog.Int("health_check_grace_period", healthCheckGracePeriod))
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

	serviceName := "agent-deploy-" + strings.ToLower(deployID[:12])

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

	// P1.35: Include custom domain URL first (primary) when configured.
	// Per spec ralph/specs/custom-dns.md: Custom domain should appear in status URL list.
	if customDomain := infra.Resources[state.ResourceDomainName]; customDomain != "" {
		urls = append(urls, scheme+"://"+customDomain)
	}

	// Add ALB DNS name as secondary/fallback URL.
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
	// P0.3: Guard against nil store to prevent panic.
	if err := p.checkStore(); err != nil {
		return nil, err
	}

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
	cpuPolicyName := "agent-deploy-cpu-" + strings.ToLower(deployID[:12])
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
	memPolicyName := "agent-deploy-memory-" + strings.ToLower(deployID[:12])
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
func (p *AWSProvider) deleteAutoScaling(ctx context.Context, cfg aws.Config, clusterName, serviceName, deployID string) {
	clients := p.getClients(cfg)
	asClient := clients.AutoScaling

	resourceID := fmt.Sprintf("service/%s/%s", clusterName, serviceName)

	// Delete CPU scaling policy.
	cpuPolicyName := "agent-deploy-cpu-" + strings.ToLower(deployID[:12])
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
	memPolicyName := "agent-deploy-memory-" + strings.ToLower(deployID[:12])
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

// backoffWithJitter calculates the next backoff duration with exponential backoff and jitter.
// baseDelay is the starting delay, attempt is the 0-indexed attempt number (0, 1, 2, ...),
// maxDelay is the maximum delay cap. Adds ±25% jitter to prevent thundering herd.
func backoffWithJitter(baseDelay time.Duration, attempt int, maxDelay time.Duration) time.Duration {
	// Calculate exponential backoff: baseDelay * 2^attempt
	delay := baseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
			break
		}
	}

	// Add jitter: ±25%
	jitterRange := float64(delay) * 0.25
	jitter := time.Duration((rand.Float64() * 2 * jitterRange) - jitterRange)
	delay += jitter

	// Ensure we don't go below minimum or above maximum
	if delay < baseDelay/2 {
		delay = baseDelay / 2
	}
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

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

// isLocalImage determines whether an image reference is a local-only image (needs to be pushed to ECR)
// or a fully-qualified registry reference that can be used directly.
//
// Classification:
// - ECR URI (*.dkr.ecr.*.amazonaws.com/*): use as-is
// - Public registries (docker.io, ghcr.io, public.ecr.aws, gcr.io, quay.io): use as-is
// - Local image (name:tag or just name, no registry prefix): push to ECR
func isLocalImage(imageRef string) bool {
	if imageRef == "" {
		return false
	}

	// ECR URI pattern: <account>.dkr.ecr.<region>.amazonaws.com/<repo>
	if strings.Contains(imageRef, ".dkr.ecr.") && strings.Contains(imageRef, ".amazonaws.com") {
		return false
	}

	// Common public registries.
	publicRegistries := []string{
		"docker.io/",
		"index.docker.io/",
		"ghcr.io/",
		"public.ecr.aws/",
		"gcr.io/",
		"quay.io/",
		"registry.hub.docker.com/",
		"mcr.microsoft.com/",
	}

	for _, registry := range publicRegistries {
		if strings.HasPrefix(imageRef, registry) {
			return false
		}
	}

	// Check for any registry prefix (contains '/' and first part has a '.' or ':')
	// e.g., "myregistry.com/myimage:tag" or "localhost:5000/myimage:tag"
	parts := strings.SplitN(imageRef, "/", 2)
	if len(parts) == 2 {
		firstPart := parts[0]
		// If the first part looks like a domain (contains '.' or ':'), it's a registry reference.
		if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
			return false
		}
	}

	// It's a local image (e.g., "myimage:tag", "myimage", "library/nginx").
	return true
}

// pushImageToECR pushes a local Docker image to the ECR repository.
// It authenticates with ECR, tags the local image with the ECR URI, and pushes it.
// Returns the full ECR image URI to use in the task definition.
func (p *AWSProvider) pushImageToECR(
	ctx context.Context,
	cfg aws.Config,
	infra *state.Infrastructure,
	imageRef string,
	deployID string,
) (string, error) {
	clients := p.getClients(cfg)
	ecrClient := clients.ECR

	slog.Info("pushing image to ECR",
		slog.String("component", "pushImageToECR"),
		slog.String("imageRef", imageRef),
		logging.DeploymentID(deployID))

	// Step 1: Get ECR authorization token.
	authResp, err := ecrClient.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get ECR authorization token: %w", err)
	}
	if len(authResp.AuthorizationData) == 0 {
		return "", fmt.Errorf("no authorization data returned from ECR")
	}

	authData := authResp.AuthorizationData[0]
	if authData.AuthorizationToken == nil || authData.ProxyEndpoint == nil {
		return "", fmt.Errorf("invalid authorization data from ECR")
	}

	// Step 2: Decode the base64 authorization token (format: "username:password").
	decodedToken, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return "", fmt.Errorf("failed to decode ECR authorization token: %w", err)
	}
	tokenParts := strings.SplitN(string(decodedToken), ":", 2)
	if len(tokenParts) != 2 {
		return "", fmt.Errorf("invalid ECR authorization token format")
	}
	username := tokenParts[0]
	password := tokenParts[1]

	// Step 3: Construct the ECR image URI.
	// Repository name format: agent-deploy-<deployID[:12]>
	repoName := infra.Resources[state.ResourceECRRepository]
	if repoName == "" {
		repoName = "agent-deploy-" + strings.ToLower(deployID[:12])
	}

	// Extract the registry endpoint (e.g., "123456789012.dkr.ecr.us-east-1.amazonaws.com").
	registryEndpoint := strings.TrimPrefix(*authData.ProxyEndpoint, "https://")

	// Extract tag from imageRef (default to "latest").
	tag := "latest"
	if idx := strings.LastIndex(imageRef, ":"); idx != -1 {
		// Check it's not a port number (e.g., localhost:5000/image).
		potentialTag := imageRef[idx+1:]
		if !strings.Contains(potentialTag, "/") {
			tag = potentialTag
		}
	}

	ecrImageURI := fmt.Sprintf("%s/%s:%s", registryEndpoint, repoName, tag)

	slog.Info("ECR image URI constructed",
		slog.String("component", "pushImageToECR"),
		slog.String("ecrURI", ecrImageURI),
		slog.String("originalImage", imageRef))

	// Step 4: Create Docker client.
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return "", fmt.Errorf("failed to create Docker client - is Docker daemon running? %w", err)
	}
	defer func() { _ = dockerCli.Close() }()

	// Step 5: Tag the local image with the ECR URI.
	if err = dockerCli.ImageTag(ctx, imageRef, ecrImageURI); err != nil {
		return "", fmt.Errorf("image '%s' not found locally; build it first or provide a full registry URI: %w", imageRef, err)
	}

	slog.Info("image tagged for ECR",
		slog.String("component", "pushImageToECR"),
		slog.String("sourceImage", imageRef),
		slog.String("ecrURI", ecrImageURI))

	// Step 6: Build the registry auth config for push.
	authConfig := registry.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: registryEndpoint,
	}
	authConfigBytes, err := json.Marshal(authConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth config: %w", err)
	}
	encodedAuth := base64.URLEncoding.EncodeToString(authConfigBytes)

	// Step 7: Push the image to ECR.
	pushResp, err := dockerCli.ImagePush(ctx, ecrImageURI, image.PushOptions{
		RegistryAuth: encodedAuth,
	})
	if err != nil {
		return "", fmt.Errorf("failed to push image to ECR: %w", err)
	}
	defer func() { _ = pushResp.Close() }()

	// Step 8: Read the push response stream to check for errors.
	// The Docker API returns JSON messages in the stream.
	type pushMessage struct {
		Status   string `json:"status"`
		Progress string `json:"progress,omitempty"`
		Error    string `json:"error,omitempty"`
	}

	decoder := json.NewDecoder(pushResp)
	for {
		var msg pushMessage
		if err = decoder.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("failed to read push response: %w", err)
		}
		if msg.Error != "" {
			return "", fmt.Errorf("image push failed: %s", msg.Error)
		}
		// Log progress for debugging.
		if msg.Status != "" {
			slog.Debug("push progress",
				slog.String("component", "pushImageToECR"),
				slog.String("status", msg.Status),
				slog.String("progress", msg.Progress))
		}
	}

	slog.Info("image pushed to ECR",
		slog.String("component", "pushImageToECR"),
		slog.String("ecrURI", ecrImageURI))

	return ecrImageURI, nil
}

// ---------------------------------------------------------------------------
// Route 53 DNS Functions (P1.29)
// ---------------------------------------------------------------------------

// findHostedZone looks up a Route 53 hosted zone for a domain (P1.29).
// It walks up the domain tree to find the nearest parent hosted zone.
// For "app.example.com", it first tries "app.example.com", then "example.com".
func (p *AWSProvider) findHostedZone(ctx context.Context, cfg aws.Config, domainName string) (hostedZoneID string, zoneName string, err error) {
	clients := p.getClients(cfg)
	r53Client := clients.Route53
	if r53Client == nil {
		r53Client = route53.NewFromConfig(cfg)
	}

	// Walk up the domain tree to find a hosted zone.
	domain := domainName
	for {
		// Route 53 hosted zones have trailing dots.
		searchName := domain + "."

		resp, err := r53Client.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
			DNSName:  aws.String(domain),
			MaxItems: aws.Int32(1),
		})
		if err != nil {
			return "", "", fmt.Errorf("list hosted zones: %w", err)
		}

		// Check if any zone matches.
		for _, zone := range resp.HostedZones {
			if zone.Name != nil && *zone.Name == searchName {
				// Extract zone ID (remove "/hostedzone/" prefix).
				zoneID := strings.TrimPrefix(*zone.Id, "/hostedzone/")
				slog.Info("found Route 53 hosted zone",
					slog.String("component", "findHostedZone"),
					slog.String("domain", domainName),
					slog.String("zone_name", *zone.Name),
					slog.String("zone_id", zoneID))
				return zoneID, *zone.Name, nil
			}
		}

		// Walk up to parent domain.
		parent := extractParentDomain(domain)
		if parent == domain {
			// Reached the top-level domain with no hosted zone found.
			break
		}
		domain = parent
	}

	return "", "", fmt.Errorf("no Route 53 hosted zone found for %q or its parent domains. Create a hosted zone in Route 53 first", domainName)
}

// provisionCertificate requests an ACM certificate with DNS validation (P1.29).
// It creates the DNS validation record in Route 53 and waits for the certificate to be issued.
func (p *AWSProvider) provisionCertificate(
	ctx context.Context,
	cfg aws.Config,
	domainName string,
	hostedZoneID string,
	infra *state.Infrastructure,
) (certificateARN string, err error) {
	clients := p.getClients(cfg)
	acmClient := clients.ACM
	if acmClient == nil {
		acmClient = acm.NewFromConfig(cfg)
	}
	r53Client := clients.Route53
	if r53Client == nil {
		r53Client = route53.NewFromConfig(cfg)
	}

	slog.Info("requesting ACM certificate",
		slog.String("component", "provisionCertificate"),
		slog.String("domain", domainName))

	// Step 1: Request certificate.
	certResp, err := acmClient.RequestCertificate(ctx, &acm.RequestCertificateInput{
		DomainName:       aws.String(domainName),
		ValidationMethod: acmtypes.ValidationMethodDns,
		Tags: []acmtypes.Tag{
			{Key: aws.String("agent-deploy"), Value: aws.String("true")},
			{Key: aws.String("infra_id"), Value: aws.String(infra.ID)},
		},
	})
	if err != nil {
		return "", fmt.Errorf("request certificate: %w", err)
	}
	certARN := *certResp.CertificateArn

	// Store certificate ARN in infrastructure state for teardown.
	// This is critical: if storage fails, teardown won't know to delete the certificate.
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceCertificateARN, certARN); storeErr != nil {
		slog.Error("failed to store certificate ARN", logging.InfraID(infra.ID), logging.Err(storeErr))
		// Attempt to delete the certificate we just created since we can't track it
		if _, delErr := acmClient.DeleteCertificate(ctx, &acm.DeleteCertificateInput{
			CertificateArn: aws.String(certARN),
		}); delErr != nil {
			slog.Error("failed to cleanup orphaned certificate", logging.InfraID(infra.ID), logging.Err(delErr))
		}
		return "", fmt.Errorf("failed to store certificate ARN: %w (certificate rolled back)", storeErr)
	}
	infra.Resources[state.ResourceCertificateARN] = certARN

	// Mark as auto-created for cleanup.
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceCertAutoCreated, "true"); storeErr != nil {
		slog.Error("failed to store cert auto-created flag", logging.InfraID(infra.ID), logging.Err(storeErr))
		// Non-critical: certificate ARN is stored, teardown will still work
		// Just log the error and continue
	}
	infra.Resources[state.ResourceCertAutoCreated] = "true"

	slog.Info("ACM certificate requested",
		slog.String("component", "provisionCertificate"),
		slog.String("certificate_arn", certARN))

	// Step 2: Wait for DomainValidationOptions to be populated.
	// Uses exponential backoff with jitter to avoid overloading the API.
	var validationRecord *acmtypes.ResourceRecord
	const maxValidationAttempts = 15 // With exponential backoff, covers ~2 minutes
	for i := 0; i < maxValidationAttempts; i++ {
		descResp, descErr := acmClient.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
			CertificateArn: aws.String(certARN),
		})
		if descErr != nil {
			return "", fmt.Errorf("describe certificate: %w", descErr)
		}
		if len(descResp.Certificate.DomainValidationOptions) > 0 &&
			descResp.Certificate.DomainValidationOptions[0].ResourceRecord != nil {
			validationRecord = descResp.Certificate.DomainValidationOptions[0].ResourceRecord
			break
		}
		// Exponential backoff: 1s, 2s, 4s, 8s, ... up to 15s max
		delay := backoffWithJitter(1*time.Second, i, 15*time.Second)
		time.Sleep(delay)
	}
	if validationRecord == nil {
		return "", fmt.Errorf("timeout waiting for certificate DNS validation record")
	}

	slog.Info("creating DNS validation record",
		slog.String("component", "provisionCertificate"),
		slog.String("name", *validationRecord.Name),
		slog.String("value", *validationRecord.Value))

	// Step 3: Create DNS validation CNAME record.
	_, err = r53Client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53types.ChangeBatch{
			Changes: []route53types.Change{{
				Action: route53types.ChangeActionUpsert,
				ResourceRecordSet: &route53types.ResourceRecordSet{
					Name: validationRecord.Name,
					Type: route53types.RRTypeCname,
					TTL:  aws.Int64(300),
					ResourceRecords: []route53types.ResourceRecord{{
						Value: validationRecord.Value,
					}},
				},
			}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create DNS validation record: %w", err)
	}

	// Step 4: Wait for certificate to be issued (up to 5 minutes).
	// Uses exponential backoff with jitter to avoid overloading the API.
	slog.Info("waiting for certificate validation",
		slog.String("component", "provisionCertificate"),
		slog.String("certificate_arn", certARN))

	const maxIssuanceAttempts = 20 // With exponential backoff (5s base, 30s max), covers ~5 minutes
	for i := 0; i < maxIssuanceAttempts; i++ {
		descResp, descErr := acmClient.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
			CertificateArn: aws.String(certARN),
		})
		if descErr != nil {
			return "", fmt.Errorf("describe certificate: %w", descErr)
		}
		if descResp.Certificate.Status == acmtypes.CertificateStatusIssued {
			slog.Info("ACM certificate issued",
				slog.String("component", "provisionCertificate"),
				slog.String("certificate_arn", certARN))
			return certARN, nil
		}
		if descResp.Certificate.Status == acmtypes.CertificateStatusFailed {
			reason := string(descResp.Certificate.FailureReason)
			return "", fmt.Errorf("certificate validation failed: %s", reason)
		}
		// Exponential backoff: 5s, 10s, 20s, ... up to 30s max
		delay := backoffWithJitter(5*time.Second, i, 30*time.Second)
		time.Sleep(delay)
	}

	return "", fmt.Errorf("timeout waiting for certificate validation (5 minutes). Check ACM console for status")
}

// createDNSRecord creates a Route 53 alias record pointing the custom domain to the ALB (P1.29).
func (p *AWSProvider) createDNSRecord(
	ctx context.Context,
	cfg aws.Config,
	hostedZoneID string,
	domainName string,
	albDNSName string,
	albHostedZoneID string,
	infra *state.Infrastructure,
) error {
	clients := p.getClients(cfg)
	r53Client := clients.Route53
	if r53Client == nil {
		r53Client = route53.NewFromConfig(cfg)
	}

	slog.Info("creating Route 53 alias record",
		slog.String("component", "createDNSRecord"),
		slog.String("domain", domainName),
		slog.String("alb_dns", albDNSName))

	_, err := r53Client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
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
						EvaluateTargetHealth: true,
					},
				},
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("create DNS alias record: %w", err)
	}

	// Store DNS resources for teardown.
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceDomainName, domainName); storeErr != nil {
		slog.Error("failed to store domain name", logging.InfraID(infra.ID), logging.Err(storeErr))
	}
	infra.Resources[state.ResourceDomainName] = domainName
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceHostedZoneID, hostedZoneID); storeErr != nil {
		slog.Error("failed to store hosted zone ID", logging.InfraID(infra.ID), logging.Err(storeErr))
	}
	infra.Resources[state.ResourceHostedZoneID] = hostedZoneID
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceDNSRecordName, domainName); storeErr != nil {
		slog.Error("failed to store DNS record name", logging.InfraID(infra.ID), logging.Err(storeErr))
	}
	infra.Resources[state.ResourceDNSRecordName] = domainName
	// Store ALB DNS data for Route 53 alias record deletion (P1.33).
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceALBDNSName, albDNSName); storeErr != nil {
		slog.Error("failed to store ALB DNS name", logging.InfraID(infra.ID), logging.Err(storeErr))
	}
	infra.Resources[state.ResourceALBDNSName] = albDNSName
	if storeErr := p.store.UpdateInfraResource(infra.ID, state.ResourceALBHostedZoneID, albHostedZoneID); storeErr != nil {
		slog.Error("failed to store ALB hosted zone ID", logging.InfraID(infra.ID), logging.Err(storeErr))
	}
	infra.Resources[state.ResourceALBHostedZoneID] = albHostedZoneID

	slog.Info("Route 53 alias record created",
		slog.String("component", "createDNSRecord"),
		slog.String("domain", domainName))

	return nil
}

// deleteDNSResources cleans up Route 53 and ACM resources during teardown (P1.29).
func (p *AWSProvider) deleteDNSResources(ctx context.Context, cfg aws.Config, infra *state.Infrastructure) {
	clients := p.getClients(cfg)
	r53Client := clients.Route53
	if r53Client == nil {
		r53Client = route53.NewFromConfig(cfg)
	}
	acmClient := clients.ACM
	if acmClient == nil {
		acmClient = acm.NewFromConfig(cfg)
	}

	hostedZoneID := infra.Resources[state.ResourceHostedZoneID]
	domainName := infra.Resources[state.ResourceDNSRecordName]
	certARN := infra.Resources[state.ResourceCertificateARN]
	certAutoCreated := infra.Resources[state.ResourceCertAutoCreated] == "true"
	// Retrieve stored ALB DNS data for Route 53 alias record deletion (P1.33 fix).
	albDNSName := infra.Resources[state.ResourceALBDNSName]
	albHostedZoneID := infra.Resources[state.ResourceALBHostedZoneID]

	// Step 1: Delete the A alias record.
	if hostedZoneID != "" && domainName != "" && albDNSName != "" && albHostedZoneID != "" {
		slog.Info("deleting Route 53 alias record",
			slog.String("component", "deleteDNSResources"),
			slog.String("domain", domainName),
			slog.String("alb_dns", albDNSName))

		_, err := r53Client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(hostedZoneID),
			ChangeBatch: &route53types.ChangeBatch{
				Changes: []route53types.Change{{
					Action: route53types.ChangeActionDelete,
					ResourceRecordSet: &route53types.ResourceRecordSet{
						Name: aws.String(domainName),
						Type: route53types.RRTypeA,
						AliasTarget: &route53types.AliasTarget{
							// Use stored ALB DNS data instead of placeholder (P1.33 fix).
							DNSName:              aws.String(albDNSName),
							HostedZoneId:         aws.String(albHostedZoneID),
							EvaluateTargetHealth: true,
						},
					},
				}},
			},
		})
		if err != nil {
			slog.Warn("failed to delete DNS alias record (may have been deleted manually)",
				slog.String("component", "deleteDNSResources"),
				slog.String("domain", domainName),
				logging.Err(err))
		}
	} else if hostedZoneID != "" && domainName != "" {
		// Log warning if ALB DNS data is missing (older deployments before P1.33 fix).
		slog.Warn("skipping Route 53 alias record deletion - ALB DNS data not found in state",
			slog.String("component", "deleteDNSResources"),
			slog.String("domain", domainName),
			slog.String("hint", "delete manually via AWS Console or re-deploy to populate state"))
	}

	// Step 2: Delete the ACM certificate (only if auto-created).
	if certARN != "" && certAutoCreated {
		slog.Info("deleting auto-created ACM certificate",
			slog.String("component", "deleteDNSResources"),
			slog.String("certificate_arn", certARN))

		// First, get validation record to delete it.
		descResp, err := acmClient.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
			CertificateArn: aws.String(certARN),
		})
		if err == nil && len(descResp.Certificate.DomainValidationOptions) > 0 {
			valOpt := descResp.Certificate.DomainValidationOptions[0]
			if valOpt.ResourceRecord != nil && hostedZoneID != "" {
				// Delete the validation CNAME record.
				_, delErr := r53Client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
					HostedZoneId: aws.String(hostedZoneID),
					ChangeBatch: &route53types.ChangeBatch{
						Changes: []route53types.Change{{
							Action: route53types.ChangeActionDelete,
							ResourceRecordSet: &route53types.ResourceRecordSet{
								Name: valOpt.ResourceRecord.Name,
								Type: route53types.RRTypeCname,
								TTL:  aws.Int64(300),
								ResourceRecords: []route53types.ResourceRecord{{
									Value: valOpt.ResourceRecord.Value,
								}},
							},
						}},
					},
				})
				if delErr != nil {
					slog.Warn("failed to delete DNS validation record",
						slog.String("component", "deleteDNSResources"),
						logging.Err(delErr))
				}
			}
		}

		// Delete the certificate.
		_, err = acmClient.DeleteCertificate(ctx, &acm.DeleteCertificateInput{
			CertificateArn: aws.String(certARN),
		})
		if err != nil {
			slog.Warn("failed to delete ACM certificate (may still be in use)",
				slog.String("component", "deleteDNSResources"),
				slog.String("certificate_arn", certARN),
				logging.Err(err))
		}
	}
}

// ---------------------------------------------------------------------------
// Lightsail Provider Functions (P1.34)
// WHY: Lightsail containers provide $7-25/mo deployments vs $65+/mo with ECS Fargate,
// making it ideal for personal projects and small applications.
// ---------------------------------------------------------------------------

// lightsailPowerPricing maps Lightsail container power levels to monthly cost per node.
// These are fixed prices from AWS (as of 2024) — no need for Pricing API.
var lightsailPowerPricing = map[lstypes.ContainerServicePowerName]float64{
	lstypes.ContainerServicePowerNameNano:   7.0,
	lstypes.ContainerServicePowerNameMicro:  10.0,
	lstypes.ContainerServicePowerNameSmall:  25.0,
	lstypes.ContainerServicePowerNameMedium: 50.0,
	lstypes.ContainerServicePowerNameLarge:  100.0,
	lstypes.ContainerServicePowerNameXlarge: 200.0,
}

// lightsailPowerResources maps power levels to their compute resources.
// WHY: Helps users understand what they get at each tier.
var lightsailPowerResources = map[lstypes.ContainerServicePowerName]struct{ VCPU, MemoryGB float64 }{
	lstypes.ContainerServicePowerNameNano:   {0.25, 0.5},
	lstypes.ContainerServicePowerNameMicro:  {0.5, 1.0},
	lstypes.ContainerServicePowerNameSmall:  {1.0, 2.0},
	lstypes.ContainerServicePowerNameMedium: {2.0, 4.0},
	lstypes.ContainerServicePowerNameLarge:  {4.0, 8.0},
	lstypes.ContainerServicePowerNameXlarge: {8.0, 16.0},
}

// selectBackend determines whether to use Lightsail or ECS Fargate based on workload signals.
// WHY: Lightsail is preferred for simple, low-traffic apps; ECS Fargate for production workloads.
// Per spec ralph/specs/lightsail-provider.md: selection is automatic based on signals.
func selectBackend(expectedUsers int, autoScalingEnabled bool, appDescription string) string {
	// ECS Fargate preferred signals:
	// - >500 expected users
	// - Auto-scaling required (maxCount > 1 explicitly set)
	// - Explicit production keywords in description
	// - Custom VPC/networking requirements (implied by certain keywords)

	// Check for production signals in app description.
	prodKeywords := []string{"production", "prod", "enterprise", "high-availability", "ha ", "mission-critical"}
	descLower := strings.ToLower(appDescription)
	for _, kw := range prodKeywords {
		if strings.Contains(descLower, kw) {
			return state.BackendECSFargate
		}
	}

	// High user count requires ECS Fargate for scaling.
	if expectedUsers > 500 {
		return state.BackendECSFargate
	}

	// Auto-scaling required — Lightsail doesn't support auto-scaling.
	if autoScalingEnabled {
		return state.BackendECSFargate
	}

	// Default: Lightsail for simpler, cheaper deployments.
	return state.BackendLightsail
}

// selectLightsailPower chooses the appropriate Lightsail power level based on expected users.
// WHY: Match compute resources to workload requirements cost-effectively.
func selectLightsailPower(expectedUsers int) lstypes.ContainerServicePowerName {
	switch {
	case expectedUsers <= 50:
		return lstypes.ContainerServicePowerNameNano
	case expectedUsers <= 200:
		return lstypes.ContainerServicePowerNameMicro
	case expectedUsers <= 500:
		return lstypes.ContainerServicePowerNameSmall
	default:
		// For >500 users, we'd typically recommend ECS Fargate.
		// But if forced to Lightsail, use Small with multiple nodes.
		return lstypes.ContainerServicePowerNameSmall
	}
}

// calculateLightsailNodes determines the number of nodes needed.
// WHY: More nodes provide redundancy and handle more traffic.
func calculateLightsailNodes(expectedUsers int) int32 {
	switch {
	case expectedUsers <= 100:
		return 1
	case expectedUsers <= 300:
		return 2
	case expectedUsers <= 500:
		return 3
	default:
		// Cap at 4 nodes for Lightsail; beyond this, use ECS Fargate.
		return 4
	}
}

// createLightsailService provisions a Lightsail container service.
// WHY: Lightsail bundles compute, load balancing, TLS, and HTTPS endpoint in one resource.
func (p *AWSProvider) createLightsailService(
	ctx context.Context,
	clients *awsclient.AWSClients,
	infraID string,
	power lstypes.ContainerServicePowerName,
	nodes int32,
) (serviceName string, endpoint string, err error) {
	log := slog.With(
		slog.String("component", "lightsail"),
		logging.InfraID(infraID),
	)

	// Generate service name from infra ID (max 63 chars for Lightsail).
	// Format: agent-deploy-{first12chars}
	serviceName = fmt.Sprintf("agent-deploy-%s", infraID[:12])

	log.Info("creating Lightsail container service",
		slog.String("service_name", serviceName),
		slog.String("power", string(power)),
		slog.Int("scale", int(nodes)))

	// Build tags.
	tags := []lstypes.Tag{
		{Key: aws.String("agent-deploy:created-by"), Value: aws.String("agent-deploy")},
		{Key: aws.String("agent-deploy:infra-id"), Value: aws.String(infraID)},
	}

	// Create the container service.
	createResp, err := clients.Lightsail.CreateContainerService(ctx, &lightsail.CreateContainerServiceInput{
		ServiceName: aws.String(serviceName),
		Power:       power,
		Scale:       aws.Int32(nodes),
		Tags:        tags,
	})
	if err != nil {
		return "", "", fmt.Errorf("create lightsail container service: %w", err)
	}

	// Wait for service to become ready.
	// Lightsail services can take 2-5 minutes to provision.
	log.Info("waiting for Lightsail service to become ready")
	maxAttempts := 30 // 30 * 10s = 5 minutes
	for i := 0; i < maxAttempts; i++ {
		getResp, err := clients.Lightsail.GetContainerServices(ctx, &lightsail.GetContainerServicesInput{
			ServiceName: aws.String(serviceName),
		})
		if err != nil {
			return "", "", fmt.Errorf("check lightsail service status: %w", err)
		}

		if len(getResp.ContainerServices) > 0 {
			svc := getResp.ContainerServices[0]
			switch svc.State {
			case lstypes.ContainerServiceStateReady, lstypes.ContainerServiceStateRunning:
				endpoint = aws.ToString(svc.Url)
				log.Info("Lightsail service ready",
					slog.String("endpoint", endpoint),
					slog.String("state", string(svc.State)))
				return serviceName, endpoint, nil
			case lstypes.ContainerServiceStateDisabled, lstypes.ContainerServiceStateDeleting:
				return "", "", fmt.Errorf("lightsail service entered unexpected state: %s", svc.State)
			}
			// Still pending/deploying, continue waiting.
			log.Debug("Lightsail service still provisioning",
				slog.String("state", string(svc.State)),
				slog.Int("attempt", i+1))
		}

		time.Sleep(10 * time.Second)
	}

	// If we have a response with URL even if not fully ready, return it.
	if createResp.ContainerService != nil && createResp.ContainerService.Url != nil {
		return serviceName, aws.ToString(createResp.ContainerService.Url), nil
	}

	return "", "", fmt.Errorf("lightsail service did not become ready within 5 minutes")
}

// deployToLightsail deploys a container image to a Lightsail container service.
// WHY: Lightsail deployment is simpler than ECS — no task definitions or service updates.
func (p *AWSProvider) deployToLightsail(
	ctx context.Context,
	clients *awsclient.AWSClients,
	serviceName string,
	imageRef string,
	containerPort int,
	healthCheckPath string,
	environment map[string]string,
) (deploymentVersion int, err error) {
	log := slog.With(
		slog.String("component", "lightsail"),
		slog.String("service", serviceName),
	)

	log.Info("deploying to Lightsail",
		slog.String("image", imageRef),
		slog.Int("port", containerPort))

	// Build container definition.
	containers := map[string]lstypes.Container{
		"app": {
			Image:   aws.String(imageRef),
			Command: nil, // Use image default
			Ports: map[string]lstypes.ContainerServiceProtocol{
				fmt.Sprintf("%d", containerPort): lstypes.ContainerServiceProtocolHttp,
			},
		},
	}

	// Add environment variables if provided.
	if len(environment) > 0 {
		containers["app"] = lstypes.Container{
			Image:       aws.String(imageRef),
			Environment: environment,
			Ports: map[string]lstypes.ContainerServiceProtocol{
				fmt.Sprintf("%d", containerPort): lstypes.ContainerServiceProtocolHttp,
			},
		}
	}

	// Build endpoint configuration (routes traffic to container).
	endpointConfig := &lstypes.EndpointRequest{
		ContainerName: aws.String("app"),
		ContainerPort: aws.Int32(int32(containerPort)),
		HealthCheck: &lstypes.ContainerServiceHealthCheckConfig{
			Path:               aws.String(healthCheckPath),
			IntervalSeconds:    aws.Int32(30),
			TimeoutSeconds:     aws.Int32(5),
			HealthyThreshold:   aws.Int32(2),
			UnhealthyThreshold: aws.Int32(3),
			SuccessCodes:       aws.String("200-399"),
		},
	}

	// Create deployment.
	deployResp, err := clients.Lightsail.CreateContainerServiceDeployment(ctx, &lightsail.CreateContainerServiceDeploymentInput{
		ServiceName:    aws.String(serviceName),
		Containers:     containers,
		PublicEndpoint: endpointConfig,
	})
	if err != nil {
		return 0, fmt.Errorf("create lightsail deployment: %w", err)
	}

	// Wait for deployment to become active.
	log.Info("waiting for Lightsail deployment to become active")
	maxAttempts := 30 // 30 * 10s = 5 minutes
	for i := 0; i < maxAttempts; i++ {
		getResp, err := clients.Lightsail.GetContainerServices(ctx, &lightsail.GetContainerServicesInput{
			ServiceName: aws.String(serviceName),
		})
		if err != nil {
			return 0, fmt.Errorf("check lightsail deployment status: %w", err)
		}

		if len(getResp.ContainerServices) > 0 {
			svc := getResp.ContainerServices[0]
			if svc.CurrentDeployment != nil {
				switch svc.CurrentDeployment.State {
				case lstypes.ContainerServiceDeploymentStateActive:
					version := aws.ToInt32(svc.CurrentDeployment.Version)
					log.Info("Lightsail deployment active",
						slog.Int("version", int(version)))
					return int(version), nil
				case lstypes.ContainerServiceDeploymentStateFailed:
					return 0, fmt.Errorf("lightsail deployment failed")
				}
			}
			log.Debug("Lightsail deployment still activating",
				slog.Int("attempt", i+1))
		}

		time.Sleep(10 * time.Second)
	}

	// Return version from initial response if available.
	if deployResp.ContainerService != nil && deployResp.ContainerService.CurrentDeployment != nil {
		return int(aws.ToInt32(deployResp.ContainerService.CurrentDeployment.Version)), nil
	}

	return 0, fmt.Errorf("lightsail deployment did not become active within 5 minutes")
}

// teardownLightsail deletes a Lightsail container service and all associated resources.
// WHY: Lightsail teardown is simpler than ECS — one API call removes everything.
func (p *AWSProvider) teardownLightsail(
	ctx context.Context,
	clients *awsclient.AWSClients,
	serviceName string,
) error {
	log := slog.With(
		slog.String("component", "lightsail"),
		slog.String("service", serviceName),
	)

	log.Info("deleting Lightsail container service")

	_, err := clients.Lightsail.DeleteContainerService(ctx, &lightsail.DeleteContainerServiceInput{
		ServiceName: aws.String(serviceName),
	})
	if err != nil {
		return fmt.Errorf("delete lightsail container service: %w", err)
	}

	log.Info("Lightsail container service deleted successfully")
	return nil
}

// getLightsailStatus retrieves the current status of a Lightsail deployment.
// WHY: Status output shape should match ECS deployments for consistent UX.
func (p *AWSProvider) getLightsailStatus(
	ctx context.Context,
	clients *awsclient.AWSClients,
	serviceName string,
) (status string, urls []string, power string, nodes int, err error) {
	getResp, err := clients.Lightsail.GetContainerServices(ctx, &lightsail.GetContainerServicesInput{
		ServiceName: aws.String(serviceName),
	})
	if err != nil {
		return "", nil, "", 0, fmt.Errorf("get lightsail service status: %w", err)
	}

	if len(getResp.ContainerServices) == 0 {
		return "", nil, "", 0, fmt.Errorf("lightsail service not found: %s", serviceName)
	}

	svc := getResp.ContainerServices[0]

	// Map Lightsail state to deployment status.
	switch svc.State {
	case lstypes.ContainerServiceStateRunning:
		status = state.DeploymentStatusRunning
	case lstypes.ContainerServiceStateDeploying, lstypes.ContainerServiceStatePending:
		status = state.DeploymentStatusDeploying
	case lstypes.ContainerServiceStateDisabled, lstypes.ContainerServiceStateDeleting:
		status = state.DeploymentStatusStopped
	default:
		status = "unknown"
	}

	// Get public URL.
	if svc.Url != nil {
		urls = []string{aws.ToString(svc.Url)}
	}

	power = string(svc.Power)
	nodes = int(aws.ToInt32(svc.Scale))

	return status, urls, power, nodes, nil
}
