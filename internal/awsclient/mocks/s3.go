// Package mocks provides mock implementations of AWS service interfaces for testing.
package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Note: Interface compliance is verified in internal/awsclient/interfaces_test.go
// to avoid import cycles between mocks and awsclient packages.

// S3Mock provides a mock implementation of S3API for testing static site workloads (P1.37).
// WHY: Tests need to verify S3 bucket creation, object upload, and cleanup
// without making real AWS API calls.
type S3Mock struct {
	// Bucket operation results
	CreateBucketFn    func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	DeleteBucketFn    func(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error)
	HeadBucketFn      func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)

	// Public access configuration
	PutPublicAccessBlockFn func(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error)

	// Bucket policy for CloudFront OAC
	PutBucketPolicyFn    func(ctx context.Context, params *s3.PutBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error)
	DeleteBucketPolicyFn func(ctx context.Context, params *s3.DeleteBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketPolicyOutput, error)

	// Object operations
	PutObjectFn     func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObjectFn  func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	ListObjectsV2Fn func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObjectsFn func(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)

	// Call tracking for assertions
	CreateBucketCalls    []s3.CreateBucketInput
	DeleteBucketCalls    []s3.DeleteBucketInput
	PutObjectCalls       []s3.PutObjectInput
	DeleteObjectsCalls   []s3.DeleteObjectsInput
}

// CreateBucket creates an S3 bucket.
func (m *S3Mock) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	m.CreateBucketCalls = append(m.CreateBucketCalls, *params)
	if m.CreateBucketFn != nil {
		return m.CreateBucketFn(ctx, params, optFns...)
	}
	return &s3.CreateBucketOutput{}, nil
}

// DeleteBucket deletes an S3 bucket.
func (m *S3Mock) DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	m.DeleteBucketCalls = append(m.DeleteBucketCalls, *params)
	if m.DeleteBucketFn != nil {
		return m.DeleteBucketFn(ctx, params, optFns...)
	}
	return &s3.DeleteBucketOutput{}, nil
}

// HeadBucket checks if a bucket exists.
func (m *S3Mock) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if m.HeadBucketFn != nil {
		return m.HeadBucketFn(ctx, params, optFns...)
	}
	return &s3.HeadBucketOutput{}, nil
}

// PutPublicAccessBlock blocks public access to a bucket.
func (m *S3Mock) PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	if m.PutPublicAccessBlockFn != nil {
		return m.PutPublicAccessBlockFn(ctx, params, optFns...)
	}
	return &s3.PutPublicAccessBlockOutput{}, nil
}

// PutBucketPolicy sets the bucket policy.
func (m *S3Mock) PutBucketPolicy(ctx context.Context, params *s3.PutBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error) {
	if m.PutBucketPolicyFn != nil {
		return m.PutBucketPolicyFn(ctx, params, optFns...)
	}
	return &s3.PutBucketPolicyOutput{}, nil
}

// DeleteBucketPolicy deletes the bucket policy.
func (m *S3Mock) DeleteBucketPolicy(ctx context.Context, params *s3.DeleteBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketPolicyOutput, error) {
	if m.DeleteBucketPolicyFn != nil {
		return m.DeleteBucketPolicyFn(ctx, params, optFns...)
	}
	return &s3.DeleteBucketPolicyOutput{}, nil
}

// PutObject uploads an object to S3.
func (m *S3Mock) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.PutObjectCalls = append(m.PutObjectCalls, *params)
	if m.PutObjectFn != nil {
		return m.PutObjectFn(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

// DeleteObject deletes an object from S3.
func (m *S3Mock) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if m.DeleteObjectFn != nil {
		return m.DeleteObjectFn(ctx, params, optFns...)
	}
	return &s3.DeleteObjectOutput{}, nil
}

// ListObjectsV2 lists objects in a bucket.
func (m *S3Mock) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.ListObjectsV2Fn != nil {
		return m.ListObjectsV2Fn(ctx, params, optFns...)
	}
	return &s3.ListObjectsV2Output{}, nil
}

// DeleteObjects deletes multiple objects from S3.
func (m *S3Mock) DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	m.DeleteObjectsCalls = append(m.DeleteObjectsCalls, *params)
	if m.DeleteObjectsFn != nil {
		return m.DeleteObjectsFn(ctx, params, optFns...)
	}
	return &s3.DeleteObjectsOutput{}, nil
}
