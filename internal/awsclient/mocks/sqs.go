// Package mocks provides mock implementations of AWS service interfaces for testing.
package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Note: Interface compliance is verified in internal/awsclient/interfaces_test.go
// to avoid import cycles between mocks and awsclient packages.

// SQSMock provides a mock implementation of SQSAPI for testing background workers (P1.38).
// WHY: Tests need to verify SQS queue creation, message operations, and teardown
// without making real AWS API calls.
type SQSMock struct {
	// Queue management
	CreateQueueFn        func(ctx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error)
	DeleteQueueFn        func(ctx context.Context, params *sqs.DeleteQueueInput, optFns ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error)
	GetQueueAttributesFn func(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
	SetQueueAttributesFn func(ctx context.Context, params *sqs.SetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.SetQueueAttributesOutput, error)
	GetQueueUrlFn        func(ctx context.Context, params *sqs.GetQueueUrlInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error)

	// Message operations
	SendMessageFn    func(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	ReceiveMessageFn func(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessageFn  func(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)

	// Queue cleanup
	PurgeQueueFn func(ctx context.Context, params *sqs.PurgeQueueInput, optFns ...func(*sqs.Options)) (*sqs.PurgeQueueOutput, error)

	// Call tracking for assertions
	CreateQueueCalls        []sqs.CreateQueueInput
	DeleteQueueCalls        []sqs.DeleteQueueInput
	GetQueueAttributesCalls []sqs.GetQueueAttributesInput
	SetQueueAttributesCalls []sqs.SetQueueAttributesInput
	GetQueueUrlCalls        []sqs.GetQueueUrlInput
	SendMessageCalls        []sqs.SendMessageInput
	ReceiveMessageCalls     []sqs.ReceiveMessageInput
	DeleteMessageCalls      []sqs.DeleteMessageInput
	PurgeQueueCalls         []sqs.PurgeQueueInput
}

// CreateQueue creates an SQS queue.
func (m *SQSMock) CreateQueue(ctx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error) {
	m.CreateQueueCalls = append(m.CreateQueueCalls, *params)
	if m.CreateQueueFn != nil {
		return m.CreateQueueFn(ctx, params, optFns...)
	}
	return &sqs.CreateQueueOutput{}, nil
}

// DeleteQueue deletes an SQS queue.
func (m *SQSMock) DeleteQueue(ctx context.Context, params *sqs.DeleteQueueInput, optFns ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error) {
	m.DeleteQueueCalls = append(m.DeleteQueueCalls, *params)
	if m.DeleteQueueFn != nil {
		return m.DeleteQueueFn(ctx, params, optFns...)
	}
	return &sqs.DeleteQueueOutput{}, nil
}

// GetQueueAttributes gets queue attributes (ARN, message counts, etc.).
func (m *SQSMock) GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	m.GetQueueAttributesCalls = append(m.GetQueueAttributesCalls, *params)
	if m.GetQueueAttributesFn != nil {
		return m.GetQueueAttributesFn(ctx, params, optFns...)
	}
	return &sqs.GetQueueAttributesOutput{}, nil
}

// SetQueueAttributes sets queue attributes (redrive policy, etc.).
func (m *SQSMock) SetQueueAttributes(ctx context.Context, params *sqs.SetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.SetQueueAttributesOutput, error) {
	m.SetQueueAttributesCalls = append(m.SetQueueAttributesCalls, *params)
	if m.SetQueueAttributesFn != nil {
		return m.SetQueueAttributesFn(ctx, params, optFns...)
	}
	return &sqs.SetQueueAttributesOutput{}, nil
}

// GetQueueUrl gets the URL for a queue by name.
func (m *SQSMock) GetQueueUrl(ctx context.Context, params *sqs.GetQueueUrlInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error) {
	m.GetQueueUrlCalls = append(m.GetQueueUrlCalls, *params)
	if m.GetQueueUrlFn != nil {
		return m.GetQueueUrlFn(ctx, params, optFns...)
	}
	return &sqs.GetQueueUrlOutput{}, nil
}

// SendMessage sends a message to a queue.
func (m *SQSMock) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	m.SendMessageCalls = append(m.SendMessageCalls, *params)
	if m.SendMessageFn != nil {
		return m.SendMessageFn(ctx, params, optFns...)
	}
	return &sqs.SendMessageOutput{}, nil
}

// ReceiveMessage receives messages from a queue.
func (m *SQSMock) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	m.ReceiveMessageCalls = append(m.ReceiveMessageCalls, *params)
	if m.ReceiveMessageFn != nil {
		return m.ReceiveMessageFn(ctx, params, optFns...)
	}
	return &sqs.ReceiveMessageOutput{}, nil
}

// DeleteMessage deletes a message from a queue.
func (m *SQSMock) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	m.DeleteMessageCalls = append(m.DeleteMessageCalls, *params)
	if m.DeleteMessageFn != nil {
		return m.DeleteMessageFn(ctx, params, optFns...)
	}
	return &sqs.DeleteMessageOutput{}, nil
}

// PurgeQueue purges all messages from a queue.
func (m *SQSMock) PurgeQueue(ctx context.Context, params *sqs.PurgeQueueInput, optFns ...func(*sqs.Options)) (*sqs.PurgeQueueOutput, error) {
	m.PurgeQueueCalls = append(m.PurgeQueueCalls, *params)
	if m.PurgeQueueFn != nil {
		return m.PurgeQueueFn(ctx, params, optFns...)
	}
	return &sqs.PurgeQueueOutput{}, nil
}
