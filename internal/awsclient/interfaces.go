// Package awsclient provides shared AWS SDK configuration and interfaces.
package awsclient

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// EC2API defines the EC2 API methods used by agent-deploy.
// This interface enables mocking for unit tests without requiring
// real AWS credentials or LocalStack.
type EC2API interface {
	// VPC operations
	CreateVpc(ctx context.Context, params *ec2.CreateVpcInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error)
	DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	ModifyVpcAttribute(ctx context.Context, params *ec2.ModifyVpcAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyVpcAttributeOutput, error)
	DeleteVpc(ctx context.Context, params *ec2.DeleteVpcInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error)

	// Internet Gateway operations
	CreateInternetGateway(ctx context.Context, params *ec2.CreateInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateInternetGatewayOutput, error)
	AttachInternetGateway(ctx context.Context, params *ec2.AttachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.AttachInternetGatewayOutput, error)
	DetachInternetGateway(ctx context.Context, params *ec2.DetachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DetachInternetGatewayOutput, error)
	DeleteInternetGateway(ctx context.Context, params *ec2.DeleteInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteInternetGatewayOutput, error)

	// Availability Zone operations
	DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)

	// Subnet operations
	CreateSubnet(ctx context.Context, params *ec2.CreateSubnetInput, optFns ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error)
	ModifySubnetAttribute(ctx context.Context, params *ec2.ModifySubnetAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifySubnetAttributeOutput, error)
	DeleteSubnet(ctx context.Context, params *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error)

	// Route Table operations
	CreateRouteTable(ctx context.Context, params *ec2.CreateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteTableOutput, error)
	CreateRoute(ctx context.Context, params *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error)
	AssociateRouteTable(ctx context.Context, params *ec2.AssociateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.AssociateRouteTableOutput, error)
	DisassociateRouteTable(ctx context.Context, params *ec2.DisassociateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DisassociateRouteTableOutput, error)
	DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
	DeleteRouteTable(ctx context.Context, params *ec2.DeleteRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteTableOutput, error)

	// NAT Gateway operations
	CreateNatGateway(ctx context.Context, params *ec2.CreateNatGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateNatGatewayOutput, error)
	DeleteNatGateway(ctx context.Context, params *ec2.DeleteNatGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteNatGatewayOutput, error)
	DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error)

	// Elastic IP operations
	AllocateAddress(ctx context.Context, params *ec2.AllocateAddressInput, optFns ...func(*ec2.Options)) (*ec2.AllocateAddressOutput, error)
	ReleaseAddress(ctx context.Context, params *ec2.ReleaseAddressInput, optFns ...func(*ec2.Options)) (*ec2.ReleaseAddressOutput, error)

	// Security Group operations
	CreateSecurityGroup(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngress(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	DeleteSecurityGroup(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
}

// ECSAPI defines the ECS API methods used by agent-deploy.
type ECSAPI interface {
	// Cluster operations
	CreateCluster(ctx context.Context, params *ecs.CreateClusterInput, optFns ...func(*ecs.Options)) (*ecs.CreateClusterOutput, error)
	DeleteCluster(ctx context.Context, params *ecs.DeleteClusterInput, optFns ...func(*ecs.Options)) (*ecs.DeleteClusterOutput, error)
	ListClusters(ctx context.Context, params *ecs.ListClustersInput, optFns ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeClusters(ctx context.Context, params *ecs.DescribeClustersInput, optFns ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)

	// Service operations
	CreateService(ctx context.Context, params *ecs.CreateServiceInput, optFns ...func(*ecs.Options)) (*ecs.CreateServiceOutput, error)
	DescribeServices(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	UpdateService(ctx context.Context, params *ecs.UpdateServiceInput, optFns ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
	DeleteService(ctx context.Context, params *ecs.DeleteServiceInput, optFns ...func(*ecs.Options)) (*ecs.DeleteServiceOutput, error)

	// Task Definition operations
	RegisterTaskDefinition(ctx context.Context, params *ecs.RegisterTaskDefinitionInput, optFns ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error)
}

// ELBV2API defines the ELBv2 (Application Load Balancer) API methods used by agent-deploy.
type ELBV2API interface {
	// Load Balancer operations
	CreateLoadBalancer(ctx context.Context, params *elasticloadbalancingv2.CreateLoadBalancerInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateLoadBalancerOutput, error)
	DescribeLoadBalancers(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error)
	DeleteLoadBalancer(ctx context.Context, params *elasticloadbalancingv2.DeleteLoadBalancerInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DeleteLoadBalancerOutput, error)

	// Target Group operations
	CreateTargetGroup(ctx context.Context, params *elasticloadbalancingv2.CreateTargetGroupInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateTargetGroupOutput, error)
	ModifyTargetGroup(ctx context.Context, params *elasticloadbalancingv2.ModifyTargetGroupInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.ModifyTargetGroupOutput, error)
	DescribeTargetHealth(ctx context.Context, params *elasticloadbalancingv2.DescribeTargetHealthInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTargetHealthOutput, error)
	DeleteTargetGroup(ctx context.Context, params *elasticloadbalancingv2.DeleteTargetGroupInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DeleteTargetGroupOutput, error)

	// Listener operations
	CreateListener(ctx context.Context, params *elasticloadbalancingv2.CreateListenerInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.CreateListenerOutput, error)

	// Tag operations (used by reconciler)
	DescribeTags(ctx context.Context, params *elasticloadbalancingv2.DescribeTagsInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTagsOutput, error)
}

// IAMAPI defines the IAM API methods used by agent-deploy.
type IAMAPI interface {
	// Role operations
	CreateRole(ctx context.Context, params *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	GetRole(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	AttachRolePolicy(ctx context.Context, params *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
	DetachRolePolicy(ctx context.Context, params *iam.DetachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error)
	DeleteRole(ctx context.Context, params *iam.DeleteRoleInput, optFns ...func(*iam.Options)) (*iam.DeleteRoleOutput, error)
}

// ECRAPI defines the ECR API methods used by agent-deploy.
type ECRAPI interface {
	CreateRepository(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error)
	DeleteRepository(ctx context.Context, params *ecr.DeleteRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.DeleteRepositoryOutput, error)
	GetAuthorizationToken(ctx context.Context, params *ecr.GetAuthorizationTokenInput, optFns ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error)
}

// CloudWatchLogsAPI defines the CloudWatch Logs API methods used by agent-deploy.
type CloudWatchLogsAPI interface {
	CreateLogGroup(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error)
	PutRetentionPolicy(ctx context.Context, params *cloudwatchlogs.PutRetentionPolicyInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutRetentionPolicyOutput, error)
	DeleteLogGroup(ctx context.Context, params *cloudwatchlogs.DeleteLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error)
}

// AutoScalingAPI defines the Application Auto Scaling API methods used by agent-deploy.
type AutoScalingAPI interface {
	RegisterScalableTarget(ctx context.Context, params *applicationautoscaling.RegisterScalableTargetInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.RegisterScalableTargetOutput, error)
	DeregisterScalableTarget(ctx context.Context, params *applicationautoscaling.DeregisterScalableTargetInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DeregisterScalableTargetOutput, error)
	PutScalingPolicy(ctx context.Context, params *applicationautoscaling.PutScalingPolicyInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.PutScalingPolicyOutput, error)
	DeleteScalingPolicy(ctx context.Context, params *applicationautoscaling.DeleteScalingPolicyInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DeleteScalingPolicyOutput, error)
	DescribeScalableTargets(ctx context.Context, params *applicationautoscaling.DescribeScalableTargetsInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DescribeScalableTargetsOutput, error)
	DescribeScalingPolicies(ctx context.Context, params *applicationautoscaling.DescribeScalingPoliciesInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DescribeScalingPoliciesOutput, error)
}

// ACMAPI defines the ACM API methods used by agent-deploy.
type ACMAPI interface {
	DescribeCertificate(ctx context.Context, params *acm.DescribeCertificateInput, optFns ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error)
	RequestCertificate(ctx context.Context, params *acm.RequestCertificateInput, optFns ...func(*acm.Options)) (*acm.RequestCertificateOutput, error)
	DeleteCertificate(ctx context.Context, params *acm.DeleteCertificateInput, optFns ...func(*acm.Options)) (*acm.DeleteCertificateOutput, error)
}

// Route53API defines the Route 53 API methods used by agent-deploy for custom DNS (P1.29).
type Route53API interface {
	ListHostedZonesByName(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error)
	ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
	GetChange(ctx context.Context, params *route53.GetChangeInput, optFns ...func(*route53.Options)) (*route53.GetChangeOutput, error)
}

// LightsailAPI defines the Lightsail API methods used by agent-deploy for low-cost deployments (P1.34).
// WHY: Lightsail containers provide $7-25/mo deployments vs $65+/mo with ECS Fargate,
// making it ideal for personal projects and small applications.
type LightsailAPI interface {
	// Container service operations
	CreateContainerService(ctx context.Context, params *lightsail.CreateContainerServiceInput, optFns ...func(*lightsail.Options)) (*lightsail.CreateContainerServiceOutput, error)
	GetContainerServices(ctx context.Context, params *lightsail.GetContainerServicesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetContainerServicesOutput, error)
	UpdateContainerService(ctx context.Context, params *lightsail.UpdateContainerServiceInput, optFns ...func(*lightsail.Options)) (*lightsail.UpdateContainerServiceOutput, error)
	DeleteContainerService(ctx context.Context, params *lightsail.DeleteContainerServiceInput, optFns ...func(*lightsail.Options)) (*lightsail.DeleteContainerServiceOutput, error)

	// Container deployment operations
	CreateContainerServiceDeployment(ctx context.Context, params *lightsail.CreateContainerServiceDeploymentInput, optFns ...func(*lightsail.Options)) (*lightsail.CreateContainerServiceDeploymentOutput, error)
	GetContainerServiceDeployments(ctx context.Context, params *lightsail.GetContainerServiceDeploymentsInput, optFns ...func(*lightsail.Options)) (*lightsail.GetContainerServiceDeploymentsOutput, error)

	// Certificate operations for custom DNS
	CreateCertificate(ctx context.Context, params *lightsail.CreateCertificateInput, optFns ...func(*lightsail.Options)) (*lightsail.CreateCertificateOutput, error)
	GetCertificates(ctx context.Context, params *lightsail.GetCertificatesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetCertificatesOutput, error)
	DeleteCertificate(ctx context.Context, params *lightsail.DeleteCertificateInput, optFns ...func(*lightsail.Options)) (*lightsail.DeleteCertificateOutput, error)
}

// S3API defines the S3 API methods used by agent-deploy for static site hosting (P1.37).
// WHY: Static sites use S3 for storage at ~$0.023/GB/month, far cheaper than container hosting.
type S3API interface {
	// Bucket operations
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error)
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	
	// Public access configuration
	PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error)
	
	// Bucket policy for CloudFront OAC
	PutBucketPolicy(ctx context.Context, params *s3.PutBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error)
	DeleteBucketPolicy(ctx context.Context, params *s3.DeleteBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketPolicyOutput, error)
	
	// Object operations for deploy
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

// CloudFrontAPI defines the CloudFront API methods used by agent-deploy for static site CDN (P1.37).
// WHY: CloudFront provides global edge caching, HTTPS, and compression for static sites at ~$0.085/GB.
type CloudFrontAPI interface {
	// Distribution operations
	CreateDistribution(ctx context.Context, params *cloudfront.CreateDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateDistributionOutput, error)
	GetDistribution(ctx context.Context, params *cloudfront.GetDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetDistributionOutput, error)
	UpdateDistribution(ctx context.Context, params *cloudfront.UpdateDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.UpdateDistributionOutput, error)
	DeleteDistribution(ctx context.Context, params *cloudfront.DeleteDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.DeleteDistributionOutput, error)
	
	// Origin Access Control for secure S3 access
	CreateOriginAccessControl(ctx context.Context, params *cloudfront.CreateOriginAccessControlInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateOriginAccessControlOutput, error)
	DeleteOriginAccessControl(ctx context.Context, params *cloudfront.DeleteOriginAccessControlInput, optFns ...func(*cloudfront.Options)) (*cloudfront.DeleteOriginAccessControlOutput, error)
	GetOriginAccessControl(ctx context.Context, params *cloudfront.GetOriginAccessControlInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetOriginAccessControlOutput, error)
	
	// Cache invalidation after deploy
	CreateInvalidation(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error)
}

// AWSClients holds all AWS service clients used by the provider.
// This struct can be populated with either real clients or mocks for testing.
type AWSClients struct {
	EC2            EC2API
	ECS            ECSAPI
	ELBV2          ELBV2API
	IAM            IAMAPI
	ECR            ECRAPI
	CloudWatchLogs CloudWatchLogsAPI
	AutoScaling    AutoScalingAPI
	ACM            ACMAPI
	Route53        Route53API
	Lightsail      LightsailAPI      // P1.34: Low-cost container deployments
	S3             S3API             // P1.37: Static site storage
	CloudFront     CloudFrontAPI     // P1.37: Static site CDN
}
