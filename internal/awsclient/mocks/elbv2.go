package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// ELBV2Mock is a mock implementation of awsclient.ELBV2API for testing.
type ELBV2Mock struct {
	// Load Balancer operations
	CreateLoadBalancerFunc    func(ctx context.Context, params *elbv2.CreateLoadBalancerInput, optFns ...func(*elbv2.Options)) (*elbv2.CreateLoadBalancerOutput, error)
	DescribeLoadBalancersFunc func(ctx context.Context, params *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
	DeleteLoadBalancerFunc    func(ctx context.Context, params *elbv2.DeleteLoadBalancerInput, optFns ...func(*elbv2.Options)) (*elbv2.DeleteLoadBalancerOutput, error)

	// Target Group operations
	CreateTargetGroupFunc    func(ctx context.Context, params *elbv2.CreateTargetGroupInput, optFns ...func(*elbv2.Options)) (*elbv2.CreateTargetGroupOutput, error)
	ModifyTargetGroupFunc    func(ctx context.Context, params *elbv2.ModifyTargetGroupInput, optFns ...func(*elbv2.Options)) (*elbv2.ModifyTargetGroupOutput, error)
	DescribeTargetHealthFunc func(ctx context.Context, params *elbv2.DescribeTargetHealthInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeTargetHealthOutput, error)
	DeleteTargetGroupFunc    func(ctx context.Context, params *elbv2.DeleteTargetGroupInput, optFns ...func(*elbv2.Options)) (*elbv2.DeleteTargetGroupOutput, error)

	// Listener operations
	CreateListenerFunc func(ctx context.Context, params *elbv2.CreateListenerInput, optFns ...func(*elbv2.Options)) (*elbv2.CreateListenerOutput, error)

	// Tag operations
	DescribeTagsFunc func(ctx context.Context, params *elbv2.DescribeTagsInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeTagsOutput, error)

	// Tracking fields
	CreateLoadBalancerCalls    int
	DeleteLoadBalancerCalls    int
	DescribeLoadBalancersCalls int
	CreateTargetGroupCalls     int
	DeleteTargetGroupCalls     int
	DescribeTargetHealthCalls  int
	DescribeTagsCalls          int
}

func (m *ELBV2Mock) CreateLoadBalancer(ctx context.Context, params *elbv2.CreateLoadBalancerInput, optFns ...func(*elbv2.Options)) (*elbv2.CreateLoadBalancerOutput, error) {
	m.CreateLoadBalancerCalls++
	if m.CreateLoadBalancerFunc != nil {
		return m.CreateLoadBalancerFunc(ctx, params, optFns...)
	}
	return &elbv2.CreateLoadBalancerOutput{
		LoadBalancers: []elbv2types.LoadBalancer{
			{
				LoadBalancerArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/" + *params.Name + "/1234567890123456"),
				LoadBalancerName: params.Name,
				DNSName:          aws.String(*params.Name + "-1234567890.us-east-1.elb.amazonaws.com"),
				State:            &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumActive},
			},
		},
	}, nil
}

func (m *ELBV2Mock) DescribeLoadBalancers(ctx context.Context, params *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error) {
	m.DescribeLoadBalancersCalls++
	if m.DescribeLoadBalancersFunc != nil {
		return m.DescribeLoadBalancersFunc(ctx, params, optFns...)
	}
	loadBalancers := make([]elbv2types.LoadBalancer, 0, len(params.LoadBalancerArns))
	for _, arn := range params.LoadBalancerArns {
		loadBalancers = append(loadBalancers, elbv2types.LoadBalancer{
			LoadBalancerArn: aws.String(arn),
			DNSName:         aws.String("mock-alb-1234567890.us-east-1.elb.amazonaws.com"),
			State:           &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumActive},
		})
	}
	return &elbv2.DescribeLoadBalancersOutput{
		LoadBalancers: loadBalancers,
	}, nil
}

func (m *ELBV2Mock) DeleteLoadBalancer(ctx context.Context, params *elbv2.DeleteLoadBalancerInput, optFns ...func(*elbv2.Options)) (*elbv2.DeleteLoadBalancerOutput, error) {
	m.DeleteLoadBalancerCalls++
	if m.DeleteLoadBalancerFunc != nil {
		return m.DeleteLoadBalancerFunc(ctx, params, optFns...)
	}
	return &elbv2.DeleteLoadBalancerOutput{}, nil
}

func (m *ELBV2Mock) CreateTargetGroup(ctx context.Context, params *elbv2.CreateTargetGroupInput, optFns ...func(*elbv2.Options)) (*elbv2.CreateTargetGroupOutput, error) {
	m.CreateTargetGroupCalls++
	if m.CreateTargetGroupFunc != nil {
		return m.CreateTargetGroupFunc(ctx, params, optFns...)
	}
	return &elbv2.CreateTargetGroupOutput{
		TargetGroups: []elbv2types.TargetGroup{
			{
				TargetGroupArn:  aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/" + *params.Name + "/1234567890123456"),
				TargetGroupName: params.Name,
				VpcId:           params.VpcId,
			},
		},
	}, nil
}

func (m *ELBV2Mock) ModifyTargetGroup(ctx context.Context, params *elbv2.ModifyTargetGroupInput, optFns ...func(*elbv2.Options)) (*elbv2.ModifyTargetGroupOutput, error) {
	if m.ModifyTargetGroupFunc != nil {
		return m.ModifyTargetGroupFunc(ctx, params, optFns...)
	}
	return &elbv2.ModifyTargetGroupOutput{
		TargetGroups: []elbv2types.TargetGroup{
			{
				TargetGroupArn: params.TargetGroupArn,
			},
		},
	}, nil
}

func (m *ELBV2Mock) DescribeTargetHealth(ctx context.Context, params *elbv2.DescribeTargetHealthInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeTargetHealthOutput, error) {
	m.DescribeTargetHealthCalls++
	if m.DescribeTargetHealthFunc != nil {
		return m.DescribeTargetHealthFunc(ctx, params, optFns...)
	}
	// Return healthy targets by default
	return &elbv2.DescribeTargetHealthOutput{
		TargetHealthDescriptions: []elbv2types.TargetHealthDescription{
			{
				TargetHealth: &elbv2types.TargetHealth{
					State: elbv2types.TargetHealthStateEnumHealthy,
				},
			},
		},
	}, nil
}

func (m *ELBV2Mock) DeleteTargetGroup(ctx context.Context, params *elbv2.DeleteTargetGroupInput, optFns ...func(*elbv2.Options)) (*elbv2.DeleteTargetGroupOutput, error) {
	m.DeleteTargetGroupCalls++
	if m.DeleteTargetGroupFunc != nil {
		return m.DeleteTargetGroupFunc(ctx, params, optFns...)
	}
	return &elbv2.DeleteTargetGroupOutput{}, nil
}

func (m *ELBV2Mock) CreateListener(ctx context.Context, params *elbv2.CreateListenerInput, optFns ...func(*elbv2.Options)) (*elbv2.CreateListenerOutput, error) {
	if m.CreateListenerFunc != nil {
		return m.CreateListenerFunc(ctx, params, optFns...)
	}
	return &elbv2.CreateListenerOutput{
		Listeners: []elbv2types.Listener{
			{
				ListenerArn:     aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:listener/app/mock-alb/1234567890123456/1234567890123456"),
				LoadBalancerArn: params.LoadBalancerArn,
				Port:            params.Port,
				Protocol:        params.Protocol,
			},
		},
	}, nil
}

func (m *ELBV2Mock) DescribeTags(ctx context.Context, params *elbv2.DescribeTagsInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeTagsOutput, error) {
	m.DescribeTagsCalls++
	if m.DescribeTagsFunc != nil {
		return m.DescribeTagsFunc(ctx, params, optFns...)
	}
	// Default: return empty tags for each resource
	tagDescriptions := make([]elbv2types.TagDescription, 0, len(params.ResourceArns))
	for _, arn := range params.ResourceArns {
		tagDescriptions = append(tagDescriptions, elbv2types.TagDescription{
			ResourceArn: aws.String(arn),
			Tags:        []elbv2types.Tag{},
		})
	}
	return &elbv2.DescribeTagsOutput{
		TagDescriptions: tagDescriptions,
	}, nil
}
