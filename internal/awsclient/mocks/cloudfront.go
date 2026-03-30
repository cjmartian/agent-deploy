// Package mocks provides mock implementations of AWS service interfaces for testing.
package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
)

// Note: Interface compliance is verified in internal/awsclient/interfaces_test.go
// to avoid import cycles between mocks and awsclient packages.

// CloudFrontMock provides a mock implementation of CloudFrontAPI for testing static site workloads (P1.37).
// WHY: Tests need to verify CloudFront distribution creation, cache invalidation, and teardown
// without making real AWS API calls.
type CloudFrontMock struct {
	// Distribution operation results
	CreateDistributionFn func(ctx context.Context, params *cloudfront.CreateDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateDistributionOutput, error)
	GetDistributionFn    func(ctx context.Context, params *cloudfront.GetDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetDistributionOutput, error)
	UpdateDistributionFn func(ctx context.Context, params *cloudfront.UpdateDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.UpdateDistributionOutput, error)
	DeleteDistributionFn func(ctx context.Context, params *cloudfront.DeleteDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.DeleteDistributionOutput, error)

	// Origin Access Control operations
	CreateOriginAccessControlFn func(ctx context.Context, params *cloudfront.CreateOriginAccessControlInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateOriginAccessControlOutput, error)
	DeleteOriginAccessControlFn func(ctx context.Context, params *cloudfront.DeleteOriginAccessControlInput, optFns ...func(*cloudfront.Options)) (*cloudfront.DeleteOriginAccessControlOutput, error)
	GetOriginAccessControlFn    func(ctx context.Context, params *cloudfront.GetOriginAccessControlInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetOriginAccessControlOutput, error)

	// Cache invalidation
	CreateInvalidationFn func(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error)

	// Call tracking for assertions
	CreateDistributionCalls       []cloudfront.CreateDistributionInput
	GetDistributionCalls          []cloudfront.GetDistributionInput
	UpdateDistributionCalls       []cloudfront.UpdateDistributionInput
	DeleteDistributionCalls       []cloudfront.DeleteDistributionInput
	CreateOriginAccessControlCalls []cloudfront.CreateOriginAccessControlInput
	DeleteOriginAccessControlCalls []cloudfront.DeleteOriginAccessControlInput
	CreateInvalidationCalls       []cloudfront.CreateInvalidationInput
}

// CreateDistribution creates a CloudFront distribution.
func (m *CloudFrontMock) CreateDistribution(ctx context.Context, params *cloudfront.CreateDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateDistributionOutput, error) {
	m.CreateDistributionCalls = append(m.CreateDistributionCalls, *params)
	if m.CreateDistributionFn != nil {
		return m.CreateDistributionFn(ctx, params, optFns...)
	}
	return &cloudfront.CreateDistributionOutput{}, nil
}

// GetDistribution gets distribution details.
func (m *CloudFrontMock) GetDistribution(ctx context.Context, params *cloudfront.GetDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetDistributionOutput, error) {
	m.GetDistributionCalls = append(m.GetDistributionCalls, *params)
	if m.GetDistributionFn != nil {
		return m.GetDistributionFn(ctx, params, optFns...)
	}
	return &cloudfront.GetDistributionOutput{}, nil
}

// UpdateDistribution updates a distribution (used for disabling before delete).
func (m *CloudFrontMock) UpdateDistribution(ctx context.Context, params *cloudfront.UpdateDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.UpdateDistributionOutput, error) {
	m.UpdateDistributionCalls = append(m.UpdateDistributionCalls, *params)
	if m.UpdateDistributionFn != nil {
		return m.UpdateDistributionFn(ctx, params, optFns...)
	}
	return &cloudfront.UpdateDistributionOutput{}, nil
}

// DeleteDistribution deletes a CloudFront distribution.
func (m *CloudFrontMock) DeleteDistribution(ctx context.Context, params *cloudfront.DeleteDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.DeleteDistributionOutput, error) {
	m.DeleteDistributionCalls = append(m.DeleteDistributionCalls, *params)
	if m.DeleteDistributionFn != nil {
		return m.DeleteDistributionFn(ctx, params, optFns...)
	}
	return &cloudfront.DeleteDistributionOutput{}, nil
}

// CreateOriginAccessControl creates an OAC for secure S3 access.
func (m *CloudFrontMock) CreateOriginAccessControl(ctx context.Context, params *cloudfront.CreateOriginAccessControlInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateOriginAccessControlOutput, error) {
	m.CreateOriginAccessControlCalls = append(m.CreateOriginAccessControlCalls, *params)
	if m.CreateOriginAccessControlFn != nil {
		return m.CreateOriginAccessControlFn(ctx, params, optFns...)
	}
	return &cloudfront.CreateOriginAccessControlOutput{}, nil
}

// DeleteOriginAccessControl deletes an OAC.
func (m *CloudFrontMock) DeleteOriginAccessControl(ctx context.Context, params *cloudfront.DeleteOriginAccessControlInput, optFns ...func(*cloudfront.Options)) (*cloudfront.DeleteOriginAccessControlOutput, error) {
	m.DeleteOriginAccessControlCalls = append(m.DeleteOriginAccessControlCalls, *params)
	if m.DeleteOriginAccessControlFn != nil {
		return m.DeleteOriginAccessControlFn(ctx, params, optFns...)
	}
	return &cloudfront.DeleteOriginAccessControlOutput{}, nil
}

// GetOriginAccessControl gets an OAC.
func (m *CloudFrontMock) GetOriginAccessControl(ctx context.Context, params *cloudfront.GetOriginAccessControlInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetOriginAccessControlOutput, error) {
	if m.GetOriginAccessControlFn != nil {
		return m.GetOriginAccessControlFn(ctx, params, optFns...)
	}
	return &cloudfront.GetOriginAccessControlOutput{}, nil
}

// CreateInvalidation creates a cache invalidation after deploy.
func (m *CloudFrontMock) CreateInvalidation(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error) {
	m.CreateInvalidationCalls = append(m.CreateInvalidationCalls, *params)
	if m.CreateInvalidationFn != nil {
		return m.CreateInvalidationFn(ctx, params, optFns...)
	}
	return &cloudfront.CreateInvalidationOutput{}, nil
}
