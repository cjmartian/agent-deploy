package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// IAMMock is a mock implementation of awsclient.IAMAPI for testing.
type IAMMock struct {
	CreateRoleFunc       func(ctx context.Context, params *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	GetRoleFunc          func(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	AttachRolePolicyFunc func(ctx context.Context, params *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
	DetachRolePolicyFunc func(ctx context.Context, params *iam.DetachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error)
	DeleteRoleFunc       func(ctx context.Context, params *iam.DeleteRoleInput, optFns ...func(*iam.Options)) (*iam.DeleteRoleOutput, error)

	CreateRoleCalls int
	DeleteRoleCalls int
}

func (m *IAMMock) CreateRole(ctx context.Context, params *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	m.CreateRoleCalls++
	if m.CreateRoleFunc != nil {
		return m.CreateRoleFunc(ctx, params, optFns...)
	}
	return &iam.CreateRoleOutput{
		Role: &iamtypes.Role{
			RoleName: params.RoleName,
			Arn:      aws.String("arn:aws:iam::123456789012:role/" + *params.RoleName),
		},
	}, nil
}

func (m *IAMMock) GetRole(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	if m.GetRoleFunc != nil {
		return m.GetRoleFunc(ctx, params, optFns...)
	}
	return &iam.GetRoleOutput{
		Role: &iamtypes.Role{
			RoleName: params.RoleName,
			Arn:      aws.String("arn:aws:iam::123456789012:role/" + *params.RoleName),
		},
	}, nil
}

func (m *IAMMock) AttachRolePolicy(ctx context.Context, params *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
	if m.AttachRolePolicyFunc != nil {
		return m.AttachRolePolicyFunc(ctx, params, optFns...)
	}
	return &iam.AttachRolePolicyOutput{}, nil
}

func (m *IAMMock) DetachRolePolicy(ctx context.Context, params *iam.DetachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error) {
	if m.DetachRolePolicyFunc != nil {
		return m.DetachRolePolicyFunc(ctx, params, optFns...)
	}
	return &iam.DetachRolePolicyOutput{}, nil
}

func (m *IAMMock) DeleteRole(ctx context.Context, params *iam.DeleteRoleInput, optFns ...func(*iam.Options)) (*iam.DeleteRoleOutput, error) {
	m.DeleteRoleCalls++
	if m.DeleteRoleFunc != nil {
		return m.DeleteRoleFunc(ctx, params, optFns...)
	}
	return &iam.DeleteRoleOutput{}, nil
}

// ECRMock is a mock implementation of awsclient.ECRAPI for testing.
type ECRMock struct {
	CreateRepositoryFunc func(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error)
	DeleteRepositoryFunc func(ctx context.Context, params *ecr.DeleteRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.DeleteRepositoryOutput, error)

	CreateRepositoryCalls int
	DeleteRepositoryCalls int
}

func (m *ECRMock) CreateRepository(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error) {
	m.CreateRepositoryCalls++
	if m.CreateRepositoryFunc != nil {
		return m.CreateRepositoryFunc(ctx, params, optFns...)
	}
	return &ecr.CreateRepositoryOutput{
		Repository: &ecrtypes.Repository{
			RepositoryName: params.RepositoryName,
			RepositoryArn:  aws.String("arn:aws:ecr:us-east-1:123456789012:repository/" + *params.RepositoryName),
			RepositoryUri:  aws.String("123456789012.dkr.ecr.us-east-1.amazonaws.com/" + *params.RepositoryName),
		},
	}, nil
}

func (m *ECRMock) DeleteRepository(ctx context.Context, params *ecr.DeleteRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.DeleteRepositoryOutput, error) {
	m.DeleteRepositoryCalls++
	if m.DeleteRepositoryFunc != nil {
		return m.DeleteRepositoryFunc(ctx, params, optFns...)
	}
	return &ecr.DeleteRepositoryOutput{
		Repository: &ecrtypes.Repository{
			RepositoryName: params.RepositoryName,
		},
	}, nil
}

// CloudWatchLogsMock is a mock implementation of awsclient.CloudWatchLogsAPI for testing.
type CloudWatchLogsMock struct {
	CreateLogGroupFunc    func(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error)
	PutRetentionPolicyFunc func(ctx context.Context, params *cloudwatchlogs.PutRetentionPolicyInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutRetentionPolicyOutput, error)
	DeleteLogGroupFunc    func(ctx context.Context, params *cloudwatchlogs.DeleteLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error)

	CreateLogGroupCalls int
	DeleteLogGroupCalls int
}

func (m *CloudWatchLogsMock) CreateLogGroup(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	m.CreateLogGroupCalls++
	if m.CreateLogGroupFunc != nil {
		return m.CreateLogGroupFunc(ctx, params, optFns...)
	}
	return &cloudwatchlogs.CreateLogGroupOutput{}, nil
}

func (m *CloudWatchLogsMock) PutRetentionPolicy(ctx context.Context, params *cloudwatchlogs.PutRetentionPolicyInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutRetentionPolicyOutput, error) {
	if m.PutRetentionPolicyFunc != nil {
		return m.PutRetentionPolicyFunc(ctx, params, optFns...)
	}
	return &cloudwatchlogs.PutRetentionPolicyOutput{}, nil
}

func (m *CloudWatchLogsMock) DeleteLogGroup(ctx context.Context, params *cloudwatchlogs.DeleteLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error) {
	m.DeleteLogGroupCalls++
	if m.DeleteLogGroupFunc != nil {
		return m.DeleteLogGroupFunc(ctx, params, optFns...)
	}
	return &cloudwatchlogs.DeleteLogGroupOutput{}, nil
}
