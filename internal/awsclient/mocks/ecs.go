package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// ECSMock is a mock implementation of awsclient.ECSAPI for testing.
type ECSMock struct {
	// Cluster operations
	CreateClusterFunc func(ctx context.Context, params *ecs.CreateClusterInput, optFns ...func(*ecs.Options)) (*ecs.CreateClusterOutput, error)
	DeleteClusterFunc func(ctx context.Context, params *ecs.DeleteClusterInput, optFns ...func(*ecs.Options)) (*ecs.DeleteClusterOutput, error)

	// Service operations
	CreateServiceFunc   func(ctx context.Context, params *ecs.CreateServiceInput, optFns ...func(*ecs.Options)) (*ecs.CreateServiceOutput, error)
	DescribeServicesFunc func(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	UpdateServiceFunc   func(ctx context.Context, params *ecs.UpdateServiceInput, optFns ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
	DeleteServiceFunc   func(ctx context.Context, params *ecs.DeleteServiceInput, optFns ...func(*ecs.Options)) (*ecs.DeleteServiceOutput, error)

	// Task Definition operations
	RegisterTaskDefinitionFunc func(ctx context.Context, params *ecs.RegisterTaskDefinitionInput, optFns ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error)

	// Tracking fields
	CreateClusterCalls   int
	DeleteClusterCalls   int
	CreateServiceCalls   int
	DeleteServiceCalls   int
	DescribeServicesCalls int
}

func (m *ECSMock) CreateCluster(ctx context.Context, params *ecs.CreateClusterInput, optFns ...func(*ecs.Options)) (*ecs.CreateClusterOutput, error) {
	m.CreateClusterCalls++
	if m.CreateClusterFunc != nil {
		return m.CreateClusterFunc(ctx, params, optFns...)
	}
	return &ecs.CreateClusterOutput{
		Cluster: &ecstypes.Cluster{
			ClusterArn:  aws.String("arn:aws:ecs:us-east-1:123456789012:cluster/" + *params.ClusterName),
			ClusterName: params.ClusterName,
			Status:      aws.String("ACTIVE"),
		},
	}, nil
}

func (m *ECSMock) DeleteCluster(ctx context.Context, params *ecs.DeleteClusterInput, optFns ...func(*ecs.Options)) (*ecs.DeleteClusterOutput, error) {
	m.DeleteClusterCalls++
	if m.DeleteClusterFunc != nil {
		return m.DeleteClusterFunc(ctx, params, optFns...)
	}
	return &ecs.DeleteClusterOutput{
		Cluster: &ecstypes.Cluster{
			ClusterArn: params.Cluster,
			Status:     aws.String("INACTIVE"),
		},
	}, nil
}

func (m *ECSMock) CreateService(ctx context.Context, params *ecs.CreateServiceInput, optFns ...func(*ecs.Options)) (*ecs.CreateServiceOutput, error) {
	m.CreateServiceCalls++
	if m.CreateServiceFunc != nil {
		return m.CreateServiceFunc(ctx, params, optFns...)
	}
	return &ecs.CreateServiceOutput{
		Service: &ecstypes.Service{
			ServiceArn:  aws.String("arn:aws:ecs:us-east-1:123456789012:service/" + *params.ServiceName),
			ServiceName: params.ServiceName,
			Status:      aws.String("ACTIVE"),
		},
	}, nil
}

func (m *ECSMock) DescribeServices(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	m.DescribeServicesCalls++
	if m.DescribeServicesFunc != nil {
		return m.DescribeServicesFunc(ctx, params, optFns...)
	}
	// Return a healthy running service by default
	var services []ecstypes.Service
	for _, svc := range params.Services {
		services = append(services, ecstypes.Service{
			ServiceArn:   aws.String("arn:aws:ecs:us-east-1:123456789012:service/" + svc),
			ServiceName:  aws.String(svc),
			Status:       aws.String("ACTIVE"),
			RunningCount: 1,
			DesiredCount: 1,
			Deployments: []ecstypes.Deployment{
				{
					Status:       aws.String("PRIMARY"),
					RunningCount: 1,
					DesiredCount: 1,
					RolloutState: ecstypes.DeploymentRolloutStateCompleted,
				},
			},
		})
	}
	return &ecs.DescribeServicesOutput{
		Services: services,
	}, nil
}

func (m *ECSMock) UpdateService(ctx context.Context, params *ecs.UpdateServiceInput, optFns ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error) {
	if m.UpdateServiceFunc != nil {
		return m.UpdateServiceFunc(ctx, params, optFns...)
	}
	return &ecs.UpdateServiceOutput{
		Service: &ecstypes.Service{
			ServiceArn:  params.Service,
			Status:      aws.String("ACTIVE"),
			DesiredCount: int32(*params.DesiredCount),
		},
	}, nil
}

func (m *ECSMock) DeleteService(ctx context.Context, params *ecs.DeleteServiceInput, optFns ...func(*ecs.Options)) (*ecs.DeleteServiceOutput, error) {
	m.DeleteServiceCalls++
	if m.DeleteServiceFunc != nil {
		return m.DeleteServiceFunc(ctx, params, optFns...)
	}
	return &ecs.DeleteServiceOutput{
		Service: &ecstypes.Service{
			ServiceArn: params.Service,
			Status:     aws.String("DRAINING"),
		},
	}, nil
}

func (m *ECSMock) RegisterTaskDefinition(ctx context.Context, params *ecs.RegisterTaskDefinitionInput, optFns ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error) {
	if m.RegisterTaskDefinitionFunc != nil {
		return m.RegisterTaskDefinitionFunc(ctx, params, optFns...)
	}
	return &ecs.RegisterTaskDefinitionOutput{
		TaskDefinition: &ecstypes.TaskDefinition{
			TaskDefinitionArn: aws.String("arn:aws:ecs:us-east-1:123456789012:task-definition/" + *params.Family + ":1"),
			Family:            params.Family,
			Revision:          1,
		},
	}, nil
}
