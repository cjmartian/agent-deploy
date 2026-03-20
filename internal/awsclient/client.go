// Package awsclient provides shared AWS SDK configuration loading.
package awsclient

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// LoadConfig loads AWS configuration for the specified region.
// It supports credentials from:
// - Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
// - AWS credentials file (~/.aws/credentials)
// - IAM roles (when running on AWS)
func LoadConfig(ctx context.Context, region string) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return aws.Config{}, fmt.Errorf("load AWS config: %w", err)
	}
	return cfg, nil
}

// ResourceTags returns the standard tags for all agent-deploy resources.
// These tags enable cost tracking and resource identification.
func ResourceTags(planID, infraID, deploymentID string) map[string]string {
	tags := map[string]string{
		"agent-deploy:created-by": "agent-deploy",
	}
	if planID != "" {
		tags["agent-deploy:plan-id"] = planID
	}
	if infraID != "" {
		tags["agent-deploy:infra-id"] = infraID
	}
	if deploymentID != "" {
		tags["agent-deploy:deployment-id"] = deploymentID
	}
	return tags
}
