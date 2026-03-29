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
