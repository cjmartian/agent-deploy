// Package mocks provides mock implementations of AWS SDK interfaces for testing.
package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	lstypes "github.com/aws/aws-sdk-go-v2/service/lightsail/types"
)

// LightsailMock is a mock implementation of awsclient.LightsailAPI for testing.
// WHY: Enables unit testing of Lightsail provider without AWS credentials.
type LightsailMock struct {
	// Container service operations
	CreateContainerServiceFunc func(ctx context.Context, params *lightsail.CreateContainerServiceInput, optFns ...func(*lightsail.Options)) (*lightsail.CreateContainerServiceOutput, error)
	GetContainerServicesFunc   func(ctx context.Context, params *lightsail.GetContainerServicesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetContainerServicesOutput, error)
	UpdateContainerServiceFunc func(ctx context.Context, params *lightsail.UpdateContainerServiceInput, optFns ...func(*lightsail.Options)) (*lightsail.UpdateContainerServiceOutput, error)
	DeleteContainerServiceFunc func(ctx context.Context, params *lightsail.DeleteContainerServiceInput, optFns ...func(*lightsail.Options)) (*lightsail.DeleteContainerServiceOutput, error)

	// Container deployment operations
	CreateContainerServiceDeploymentFunc func(ctx context.Context, params *lightsail.CreateContainerServiceDeploymentInput, optFns ...func(*lightsail.Options)) (*lightsail.CreateContainerServiceDeploymentOutput, error)
	GetContainerServiceDeploymentsFunc   func(ctx context.Context, params *lightsail.GetContainerServiceDeploymentsInput, optFns ...func(*lightsail.Options)) (*lightsail.GetContainerServiceDeploymentsOutput, error)

	// Certificate operations
	CreateCertificateFunc func(ctx context.Context, params *lightsail.CreateCertificateInput, optFns ...func(*lightsail.Options)) (*lightsail.CreateCertificateOutput, error)
	GetCertificatesFunc   func(ctx context.Context, params *lightsail.GetCertificatesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetCertificatesOutput, error)
	DeleteCertificateFunc func(ctx context.Context, params *lightsail.DeleteCertificateInput, optFns ...func(*lightsail.Options)) (*lightsail.DeleteCertificateOutput, error)

	// Tracking fields for verification
	CreateContainerServiceCalls           int
	GetContainerServicesCalls             int
	CreateContainerServiceDeploymentCalls int
}

// CreateContainerService creates a Lightsail container service.
func (m *LightsailMock) CreateContainerService(ctx context.Context, params *lightsail.CreateContainerServiceInput, optFns ...func(*lightsail.Options)) (*lightsail.CreateContainerServiceOutput, error) {
	m.CreateContainerServiceCalls++
	if m.CreateContainerServiceFunc != nil {
		return m.CreateContainerServiceFunc(ctx, params, optFns...)
	}
	// Default success response
	return &lightsail.CreateContainerServiceOutput{
		ContainerService: &lstypes.ContainerService{
			ContainerServiceName: params.ServiceName,
			Power:                params.Power,
			Scale:                params.Scale,
			State:                lstypes.ContainerServiceStateRunning,
			Url:                  aws.String("https://test-service.12345.us-east-1.cs.amazonlightsail.com"),
		},
	}, nil
}

// GetContainerServices gets Lightsail container services.
func (m *LightsailMock) GetContainerServices(ctx context.Context, params *lightsail.GetContainerServicesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetContainerServicesOutput, error) {
	m.GetContainerServicesCalls++
	if m.GetContainerServicesFunc != nil {
		return m.GetContainerServicesFunc(ctx, params, optFns...)
	}
	// Default: return empty list
	return &lightsail.GetContainerServicesOutput{
		ContainerServices: []lstypes.ContainerService{},
	}, nil
}

// UpdateContainerService updates a Lightsail container service.
func (m *LightsailMock) UpdateContainerService(ctx context.Context, params *lightsail.UpdateContainerServiceInput, optFns ...func(*lightsail.Options)) (*lightsail.UpdateContainerServiceOutput, error) {
	if m.UpdateContainerServiceFunc != nil {
		return m.UpdateContainerServiceFunc(ctx, params, optFns...)
	}
	return &lightsail.UpdateContainerServiceOutput{
		ContainerService: &lstypes.ContainerService{
			ContainerServiceName: params.ServiceName,
			State:                lstypes.ContainerServiceStateRunning,
		},
	}, nil
}

// DeleteContainerService deletes a Lightsail container service.
func (m *LightsailMock) DeleteContainerService(ctx context.Context, params *lightsail.DeleteContainerServiceInput, optFns ...func(*lightsail.Options)) (*lightsail.DeleteContainerServiceOutput, error) {
	if m.DeleteContainerServiceFunc != nil {
		return m.DeleteContainerServiceFunc(ctx, params, optFns...)
	}
	return &lightsail.DeleteContainerServiceOutput{}, nil
}

// CreateContainerServiceDeployment creates a deployment to a Lightsail container service.
func (m *LightsailMock) CreateContainerServiceDeployment(ctx context.Context, params *lightsail.CreateContainerServiceDeploymentInput, optFns ...func(*lightsail.Options)) (*lightsail.CreateContainerServiceDeploymentOutput, error) {
	m.CreateContainerServiceDeploymentCalls++
	if m.CreateContainerServiceDeploymentFunc != nil {
		return m.CreateContainerServiceDeploymentFunc(ctx, params, optFns...)
	}
	return &lightsail.CreateContainerServiceDeploymentOutput{
		ContainerService: &lstypes.ContainerService{
			ContainerServiceName: params.ServiceName,
			State:                lstypes.ContainerServiceStateDeploying,
			Url:                  aws.String("https://test-service.12345.us-east-1.cs.amazonlightsail.com"),
		},
	}, nil
}

// GetContainerServiceDeployments gets deployments for a Lightsail container service.
func (m *LightsailMock) GetContainerServiceDeployments(ctx context.Context, params *lightsail.GetContainerServiceDeploymentsInput, optFns ...func(*lightsail.Options)) (*lightsail.GetContainerServiceDeploymentsOutput, error) {
	if m.GetContainerServiceDeploymentsFunc != nil {
		return m.GetContainerServiceDeploymentsFunc(ctx, params, optFns...)
	}
	return &lightsail.GetContainerServiceDeploymentsOutput{
		Deployments: []lstypes.ContainerServiceDeployment{},
	}, nil
}

// CreateCertificate creates a Lightsail certificate for custom DNS.
func (m *LightsailMock) CreateCertificate(ctx context.Context, params *lightsail.CreateCertificateInput, optFns ...func(*lightsail.Options)) (*lightsail.CreateCertificateOutput, error) {
	if m.CreateCertificateFunc != nil {
		return m.CreateCertificateFunc(ctx, params, optFns...)
	}
	return &lightsail.CreateCertificateOutput{
		Certificate: &lstypes.CertificateSummary{
			CertificateName: params.CertificateName,
			DomainName:      params.DomainName,
		},
	}, nil
}

// GetCertificates gets Lightsail certificates.
func (m *LightsailMock) GetCertificates(ctx context.Context, params *lightsail.GetCertificatesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetCertificatesOutput, error) {
	if m.GetCertificatesFunc != nil {
		return m.GetCertificatesFunc(ctx, params, optFns...)
	}
	return &lightsail.GetCertificatesOutput{
		Certificates: []lstypes.CertificateSummary{},
	}, nil
}

// DeleteCertificate deletes a Lightsail certificate.
func (m *LightsailMock) DeleteCertificate(ctx context.Context, params *lightsail.DeleteCertificateInput, optFns ...func(*lightsail.Options)) (*lightsail.DeleteCertificateOutput, error) {
	if m.DeleteCertificateFunc != nil {
		return m.DeleteCertificateFunc(ctx, params, optFns...)
	}
	return &lightsail.DeleteCertificateOutput{}, nil
}
