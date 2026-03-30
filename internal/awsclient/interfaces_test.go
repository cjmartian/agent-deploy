package awsclient

import (
	"testing"

	"github.com/cjmartian/agent-deploy/internal/awsclient/mocks"
)

// TestMocksImplementInterfaces verifies that all mock types implement their corresponding interfaces.
// This is a compile-time check that ensures mocks stay in sync with interfaces.
func TestMocksImplementInterfaces(t *testing.T) {
	// These assignments will fail to compile if mocks don't implement interfaces
	var _ EC2API = (*mocks.EC2Mock)(nil)
	var _ ECSAPI = (*mocks.ECSMock)(nil)
	var _ ELBV2API = (*mocks.ELBV2Mock)(nil)
	var _ IAMAPI = (*mocks.IAMMock)(nil)
	var _ ECRAPI = (*mocks.ECRMock)(nil)
	var _ CloudWatchLogsAPI = (*mocks.CloudWatchLogsMock)(nil)
	var _ AutoScalingAPI = (*mocks.AutoScalingMock)(nil)
	var _ ACMAPI = (*mocks.ACMMock)(nil)
	var _ Route53API = (*mocks.Route53Mock)(nil)
	// P1.37: Static site mocks
	var _ S3API = (*mocks.S3Mock)(nil)
	var _ CloudFrontAPI = (*mocks.CloudFrontMock)(nil)
}

// TestAWSClientsStruct verifies the AWSClients struct can hold all interfaces.
func TestAWSClientsStruct(t *testing.T) {
	clients := &AWSClients{
		EC2:            &mocks.EC2Mock{},
		ECS:            &mocks.ECSMock{},
		ELBV2:          &mocks.ELBV2Mock{},
		IAM:            &mocks.IAMMock{},
		ECR:            &mocks.ECRMock{},
		CloudWatchLogs: &mocks.CloudWatchLogsMock{},
		AutoScaling:    &mocks.AutoScalingMock{},
		ACM:            &mocks.ACMMock{},
		Route53:        &mocks.Route53Mock{},
	}

	// Verify all fields are non-nil
	if clients.EC2 == nil {
		t.Error("EC2 client is nil")
	}
	if clients.ECS == nil {
		t.Error("ECS client is nil")
	}
	if clients.ELBV2 == nil {
		t.Error("ELBV2 client is nil")
	}
	if clients.IAM == nil {
		t.Error("IAM client is nil")
	}
	if clients.ECR == nil {
		t.Error("ECR client is nil")
	}
	if clients.CloudWatchLogs == nil {
		t.Error("CloudWatchLogs client is nil")
	}
	if clients.AutoScaling == nil {
		t.Error("AutoScaling client is nil")
	}
	if clients.ACM == nil {
		t.Error("ACM client is nil")
	}
	if clients.Route53 == nil {
		t.Error("Route53 client is nil")
	}
}
