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
	ResourceVPC             = "vpc"
	ResourceSubnetPublic    = "subnet_public"
	ResourceSubnetPrivate   = "subnet_private"
	ResourceSecurityGroup   = "security_group"
	ResourceECSCluster      = "ecs_cluster"
	ResourceALB             = "alb"
	ResourceTargetGroup     = "target_group"
	ResourceECRRepository   = "ecr_repository"
	ResourceLogGroup        = "log_group"
	ResourceInternetGateway = "internet_gateway"
	ResourceRouteTable      = "route_table"
	ResourceExecutionRole   = "execution_role"
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
