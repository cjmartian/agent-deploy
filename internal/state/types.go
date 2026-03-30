// Package state provides the data model and storage for deployment state.
package state

import "time"

// Plan represents an infrastructure plan proposed by the system.
// Once created, a plan must be approved before infrastructure can be provisioned.
type Plan struct {
	ID              string    `json:"id"`
	AppDescription  string    `json:"app_description"`
	ExpectedUsers   int       `json:"expected_users"`
	LatencyMS       int       `json:"latency_ms"`
	Region          string    `json:"region"`
	Services        []string  `json:"services"`
	EstimatedCostMo float64   `json:"estimated_cost_monthly"`
	Status          string    `json:"status"` // created, approved, rejected, expired
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	// VpcCIDR is the VPC CIDR block to use (P1.9). Empty means use default 10.0.0.0/16.
	// WHY: Allow custom VPC CIDRs for VPC peering scenarios.
	VpcCIDR string `json:"vpc_cidr,omitempty"`
	// DomainName is the custom domain name for the deployment (P1.29). Empty means ALB DNS only.
	// WHY: Custom domains provide user-friendly URLs instead of ALB-generated names.
	DomainName string `json:"domain_name,omitempty"`
	// Backend specifies which AWS backend to use: "ecs-fargate" or "lightsail" (P1.34).
	// WHY: Lightsail offers $7-25/mo deployments vs $65+/mo with ECS Fargate.
	// The planner auto-selects based on workload signals (users, scaling needs, etc.).
	Backend string `json:"backend,omitempty"`
	// WorkloadType is the detected or specified workload type (web-service, static-site, etc.) (P1.37).
	// WHY: Different workload types require different infrastructure shapes and cost models.
	WorkloadType string `json:"workload_type,omitempty"`
}

// PlanStatus constants.
const (
	PlanStatusCreated  = "created"
	PlanStatusApproved = "approved"
	PlanStatusRejected = "rejected"
	PlanStatusExpired  = "expired"
)

// Infrastructure represents provisioned AWS resources for a deployment.
type Infrastructure struct {
	ID        string            `json:"id"`
	PlanID    string            `json:"plan_id"`
	Region    string            `json:"region"`
	Resources map[string]string `json:"resources"` // type -> ARN (e.g., "vpc" -> "vpc-123")
	Status    string            `json:"status"`    // provisioning, ready, failed, destroyed
	CreatedAt time.Time         `json:"created_at"`
}

// InfraStatus constants.
const (
	InfraStatusProvisioning = "provisioning"
	InfraStatusReady        = "ready"
	InfraStatusFailed       = "failed"
	InfraStatusDestroyed    = "destroyed"
)

// Resource type constants for Infrastructure.Resources map keys.
const (
	ResourceVPC               = "vpc"
	ResourceSubnetPublic      = "subnet_public"
	ResourceSubnetPrivate     = "subnet_private"
	ResourceSecurityGroup     = "security_group"
	ResourceSecurityGroupALB  = "security_group_alb"  // ALB security group (public HTTP/HTTPS)
	ResourceSecurityGroupTask = "security_group_task" // ECS task security group (internal only)
	ResourceECSCluster        = "ecs_cluster"
	ResourceALB               = "alb"
	ResourceTargetGroup       = "target_group"
	ResourceECRRepository     = "ecr_repository"
	ResourceLogGroup          = "log_group"
	ResourceInternetGateway   = "internet_gateway"
	ResourceRouteTable        = "route_table"
	ResourceRouteTablePrivate = "route_table_private" // Private subnet route table (via NAT GW)
	ResourceNATGateway        = "nat_gateway"         // NAT Gateway for private subnet egress
	ResourceElasticIP         = "elastic_ip"          // Elastic IP for NAT Gateway
	ResourceExecutionRole     = "execution_role"
	ResourceTLSEnabled        = "tls_enabled"      // "true" or "false" - whether HTTPS is configured
	ResourceCertificateARN    = "certificate_arn"  // ACM certificate ARN when TLS is enabled
	// Custom DNS resources (P1.29).
	ResourceDomainName      = "domain_name"       // Custom domain name (e.g. "app.example.com")
	ResourceHostedZoneID    = "hosted_zone_id"    // Route 53 hosted zone ID
	ResourceCertAutoCreated = "cert_auto_created" // "true" if ACM cert was auto-provisioned
	ResourceDNSRecordName   = "dns_record_name"   // The A record created in Route 53
	// ALB DNS resources for Route 53 alias record deletion (P1.33).
	ResourceALBDNSName      = "alb_dns_name"       // ALB DNS name (e.g., "dualstack-alb-123.elb.amazonaws.com")
	ResourceALBHostedZoneID = "alb_hosted_zone_id" // ALB's canonical hosted zone ID (region-specific)
)

// Backend constants define which AWS backend is used for deployment.
// WHY: Lightsail offers simpler, cheaper deployments for small apps ($7-25/mo)
// compared to ECS Fargate ($65+/mo). The planner auto-selects based on workload.
const (
	BackendECSFargate = "ecs-fargate"
	BackendLightsail  = "lightsail"
	BackendStaticSite = "s3-cloudfront" // P1.37: S3 + CloudFront for static sites
)

// Lightsail resource type constants for Infrastructure.Resources map keys.
// WHY: Track Lightsail-specific resources separately from ECS resources.
const (
	ResourceLightsailService  = "lightsail_service"  // Lightsail container service name
	ResourceLightsailEndpoint = "lightsail_endpoint" // Public HTTPS endpoint URL
	ResourceLightsailPower    = "lightsail_power"    // Power level (nano, micro, small, etc.)
	ResourceLightsailNodes    = "lightsail_nodes"    // Number of nodes (1-20)
)

// Static site resource type constants for Infrastructure.Resources map keys (P1.37).
// WHY: Track S3 and CloudFront resources for static site workloads.
const (
	ResourceS3Bucket            = "s3_bucket"             // S3 bucket name
	ResourceCloudFrontDist      = "cloudfront_dist"       // CloudFront distribution ID
	ResourceCloudFrontDomain    = "cloudfront_domain"     // CloudFront distribution domain name
	ResourceCloudFrontOAC       = "cloudfront_oac"        // Origin Access Control ID
	ResourceStaticSiteBuildCmd  = "static_build_cmd"      // Build command used (npm run build, etc.)
	ResourceStaticSiteOutputDir = "static_output_dir"     // Build output directory (dist, build, etc.)
	ResourceStaticSiteIsSPA     = "static_is_spa"         // "true" if single-page app (SPA routing)
)

// Background worker resource type constants for Infrastructure.Resources map keys (P1.38).
// WHY: Track SQS queues and worker-specific IAM roles for background worker workloads.
const (
	BackendBackgroundWorker  = "background-worker" // Backend type for workers
	ResourceSQSQueue         = "sqs_queue"         // Main SQS queue URL
	ResourceSQSQueueARN      = "sqs_queue_arn"     // Main queue ARN
	ResourceSQSDLQ           = "sqs_dlq"           // Dead letter queue URL
	ResourceSQSDLQARN        = "sqs_dlq_arn"       // DLQ ARN
	ResourceWorkerRole       = "worker_role"       // IAM role ARN for queue access
	ResourceWorkerRoleName   = "worker_role_name"  // IAM role name (for cleanup)
	ResourceWorkerPolicyARN  = "worker_policy_arn" // IAM policy ARN (for cleanup)
)

// Deployment represents an application deployed onto infrastructure.
type Deployment struct {
	ID          string    `json:"id"`
	InfraID     string    `json:"infra_id"`
	ImageRef    string    `json:"image_ref"`
	Status      string    `json:"status"` // deploying, running, failed, stopped
	URLs        []string  `json:"urls"`
	TaskDefARN  string    `json:"task_def_arn"`
	ServiceARN  string    `json:"service_arn"`
	CreatedAt   time.Time `json:"created_at"`
	LastUpdated time.Time `json:"last_updated"`
}

// DeploymentStatus constants.
const (
	DeploymentStatusDeploying = "deploying"
	DeploymentStatusRunning   = "running"
	DeploymentStatusFailed    = "failed"
	DeploymentStatusStopped   = "stopped"
)
