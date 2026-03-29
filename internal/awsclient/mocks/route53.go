package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// Route53Mock is a mock implementation of awsclient.Route53API for testing.
type Route53Mock struct {
	ListHostedZonesByNameFunc   func(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error)
	ChangeResourceRecordSetsFunc func(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
	GetChangeFunc               func(ctx context.Context, params *route53.GetChangeInput, optFns ...func(*route53.Options)) (*route53.GetChangeOutput, error)

	ListHostedZonesByNameCalls    int
	ChangeResourceRecordSetsCalls int
	GetChangeCalls                int

	// Default hosted zone for tests.
	DefaultHostedZoneID   string
	DefaultHostedZoneName string
}

func (m *Route53Mock) ListHostedZonesByName(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error) {
	m.ListHostedZonesByNameCalls++
	if m.ListHostedZonesByNameFunc != nil {
		return m.ListHostedZonesByNameFunc(ctx, params, optFns...)
	}
	// Default: return a mock hosted zone.
	hostedZoneID := m.DefaultHostedZoneID
	if hostedZoneID == "" {
		hostedZoneID = "/hostedzone/Z1234567890ABC"
	}
	hostedZoneName := m.DefaultHostedZoneName
	if hostedZoneName == "" && params.DNSName != nil {
		hostedZoneName = *params.DNSName + "."
	}
	if hostedZoneName == "" {
		hostedZoneName = "example.com."
	}
	return &route53.ListHostedZonesByNameOutput{
		HostedZones: []route53types.HostedZone{
			{
				Id:   aws.String(hostedZoneID),
				Name: aws.String(hostedZoneName),
			},
		},
	}, nil
}

func (m *Route53Mock) ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	m.ChangeResourceRecordSetsCalls++
	if m.ChangeResourceRecordSetsFunc != nil {
		return m.ChangeResourceRecordSetsFunc(ctx, params, optFns...)
	}
	return &route53.ChangeResourceRecordSetsOutput{
		ChangeInfo: &route53types.ChangeInfo{
			Id:     aws.String("/change/C1234567890ABC"),
			Status: route53types.ChangeStatusInsync,
		},
	}, nil
}

func (m *Route53Mock) GetChange(ctx context.Context, params *route53.GetChangeInput, optFns ...func(*route53.Options)) (*route53.GetChangeOutput, error) {
	m.GetChangeCalls++
	if m.GetChangeFunc != nil {
		return m.GetChangeFunc(ctx, params, optFns...)
	}
	return &route53.GetChangeOutput{
		ChangeInfo: &route53types.ChangeInfo{
			Id:     params.Id,
			Status: route53types.ChangeStatusInsync,
		},
	}, nil
}
