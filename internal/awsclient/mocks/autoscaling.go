package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	astypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
)

// AutoScalingMock is a mock implementation of awsclient.AutoScalingAPI for testing.
type AutoScalingMock struct {
	RegisterScalableTargetFunc   func(ctx context.Context, params *applicationautoscaling.RegisterScalableTargetInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.RegisterScalableTargetOutput, error)
	DeregisterScalableTargetFunc func(ctx context.Context, params *applicationautoscaling.DeregisterScalableTargetInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DeregisterScalableTargetOutput, error)
	PutScalingPolicyFunc         func(ctx context.Context, params *applicationautoscaling.PutScalingPolicyInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.PutScalingPolicyOutput, error)
	DeleteScalingPolicyFunc      func(ctx context.Context, params *applicationautoscaling.DeleteScalingPolicyInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DeleteScalingPolicyOutput, error)
	DescribeScalableTargetsFunc  func(ctx context.Context, params *applicationautoscaling.DescribeScalableTargetsInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DescribeScalableTargetsOutput, error)
	DescribeScalingPoliciesFunc  func(ctx context.Context, params *applicationautoscaling.DescribeScalingPoliciesInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DescribeScalingPoliciesOutput, error)

	RegisterScalableTargetCalls   int
	DeregisterScalableTargetCalls int
	PutScalingPolicyCalls         int
	DeleteScalingPolicyCalls      int
}

func (m *AutoScalingMock) RegisterScalableTarget(ctx context.Context, params *applicationautoscaling.RegisterScalableTargetInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.RegisterScalableTargetOutput, error) {
	m.RegisterScalableTargetCalls++
	if m.RegisterScalableTargetFunc != nil {
		return m.RegisterScalableTargetFunc(ctx, params, optFns...)
	}
	return &applicationautoscaling.RegisterScalableTargetOutput{
		ScalableTargetARN: aws.String("arn:aws:application-autoscaling:us-east-1:123456789012:scalable-target/0abc123"),
	}, nil
}

func (m *AutoScalingMock) DeregisterScalableTarget(ctx context.Context, params *applicationautoscaling.DeregisterScalableTargetInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DeregisterScalableTargetOutput, error) {
	m.DeregisterScalableTargetCalls++
	if m.DeregisterScalableTargetFunc != nil {
		return m.DeregisterScalableTargetFunc(ctx, params, optFns...)
	}
	return &applicationautoscaling.DeregisterScalableTargetOutput{}, nil
}

func (m *AutoScalingMock) PutScalingPolicy(ctx context.Context, params *applicationautoscaling.PutScalingPolicyInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.PutScalingPolicyOutput, error) {
	m.PutScalingPolicyCalls++
	if m.PutScalingPolicyFunc != nil {
		return m.PutScalingPolicyFunc(ctx, params, optFns...)
	}
	return &applicationautoscaling.PutScalingPolicyOutput{
		PolicyARN: aws.String("arn:aws:autoscaling:us-east-1:123456789012:scalingPolicy:0abc123:resource/" + *params.PolicyName),
	}, nil
}

func (m *AutoScalingMock) DeleteScalingPolicy(ctx context.Context, params *applicationautoscaling.DeleteScalingPolicyInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DeleteScalingPolicyOutput, error) {
	m.DeleteScalingPolicyCalls++
	if m.DeleteScalingPolicyFunc != nil {
		return m.DeleteScalingPolicyFunc(ctx, params, optFns...)
	}
	return &applicationautoscaling.DeleteScalingPolicyOutput{}, nil
}

func (m *AutoScalingMock) DescribeScalableTargets(ctx context.Context, params *applicationautoscaling.DescribeScalableTargetsInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DescribeScalableTargetsOutput, error) {
	if m.DescribeScalableTargetsFunc != nil {
		return m.DescribeScalableTargetsFunc(ctx, params, optFns...)
	}
	// Return a registered scalable target by default
	return &applicationautoscaling.DescribeScalableTargetsOutput{
		ScalableTargets: []astypes.ScalableTarget{
			{
				ResourceId:        aws.String("service/test-cluster/test-service"),
				ScalableDimension: astypes.ScalableDimensionECSServiceDesiredCount,
				MinCapacity:       aws.Int32(1),
				MaxCapacity:       aws.Int32(4),
			},
		},
	}, nil
}

func (m *AutoScalingMock) DescribeScalingPolicies(ctx context.Context, params *applicationautoscaling.DescribeScalingPoliciesInput, optFns ...func(*applicationautoscaling.Options)) (*applicationautoscaling.DescribeScalingPoliciesOutput, error) {
	if m.DescribeScalingPoliciesFunc != nil {
		return m.DescribeScalingPoliciesFunc(ctx, params, optFns...)
	}
	// Return scaling policies by default
	return &applicationautoscaling.DescribeScalingPoliciesOutput{
		ScalingPolicies: []astypes.ScalingPolicy{
			{
				PolicyName: aws.String("cpu-tracking-policy"),
				PolicyType: astypes.PolicyTypeTargetTrackingScaling,
				TargetTrackingScalingPolicyConfiguration: &astypes.TargetTrackingScalingPolicyConfiguration{
					TargetValue: aws.Float64(70.0),
				},
			},
		},
	}, nil
}

// ACMMock is a mock implementation of awsclient.ACMAPI for testing.
type ACMMock struct {
	DescribeCertificateFunc  func(ctx context.Context, params *acm.DescribeCertificateInput, optFns ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error)
	RequestCertificateFunc   func(ctx context.Context, params *acm.RequestCertificateInput, optFns ...func(*acm.Options)) (*acm.RequestCertificateOutput, error)
	DeleteCertificateFunc    func(ctx context.Context, params *acm.DeleteCertificateInput, optFns ...func(*acm.Options)) (*acm.DeleteCertificateOutput, error)

	DescribeCertificateCalls int
	RequestCertificateCalls  int
	DeleteCertificateCalls   int
}

func (m *ACMMock) DescribeCertificate(ctx context.Context, params *acm.DescribeCertificateInput, optFns ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error) {
	m.DescribeCertificateCalls++
	if m.DescribeCertificateFunc != nil {
		return m.DescribeCertificateFunc(ctx, params, optFns...)
	}
	return &acm.DescribeCertificateOutput{
		Certificate: &acmtypes.CertificateDetail{
			CertificateArn: params.CertificateArn,
			Status:         acmtypes.CertificateStatusIssued,
			DomainName:     aws.String("example.com"),
		},
	}, nil
}

func (m *ACMMock) RequestCertificate(ctx context.Context, params *acm.RequestCertificateInput, optFns ...func(*acm.Options)) (*acm.RequestCertificateOutput, error) {
	m.RequestCertificateCalls++
	if m.RequestCertificateFunc != nil {
		return m.RequestCertificateFunc(ctx, params, optFns...)
	}
	return &acm.RequestCertificateOutput{
		CertificateArn: aws.String("arn:aws:acm:us-east-1:123456789012:certificate/mock-cert-id"),
	}, nil
}

func (m *ACMMock) DeleteCertificate(ctx context.Context, params *acm.DeleteCertificateInput, optFns ...func(*acm.Options)) (*acm.DeleteCertificateOutput, error) {
	m.DeleteCertificateCalls++
	if m.DeleteCertificateFunc != nil {
		return m.DeleteCertificateFunc(ctx, params, optFns...)
	}
	return &acm.DeleteCertificateOutput{}, nil
}
