// Package spending provides cost estimation and tracking for AWS deployments.
package spending

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// PriceCacheTTL is how long pricing data is cached before refresh.
const PriceCacheTTL = 24 * time.Hour

// ServiceCost represents the estimated cost for a single AWS service.
type ServiceCost struct {
	Service     string  `json:"service"`
	Description string  `json:"description"`
	MonthlyCost float64 `json:"monthly_cost_usd"`
}

// CostEstimate represents a complete cost estimate for a deployment.
type CostEstimate struct {
	Services        []ServiceCost `json:"services"`
	TotalMonthlyUSD float64       `json:"total_monthly_usd"`
	Region          string        `json:"region"`
	Assumptions     []string      `json:"assumptions"`
	Disclaimer      string        `json:"disclaimer"`
	UsingFallback   bool          `json:"using_fallback"` // True if using hardcoded estimates
}

// PricingEstimator provides cost estimation using AWS Pricing API with caching.
// Falls back to hardcoded regional estimates if the Pricing API is unavailable.
type PricingEstimator struct {
	client *pricing.Client
	cache  map[string]cachedPrice
	mu     sync.RWMutex
}

type cachedPrice struct {
	price     float64
	fetchedAt time.Time
}

// EstimateParams contains parameters for cost estimation.
type EstimateParams struct {
	Region            string
	CPUUnits          int     // ECS task CPU (256, 512, 1024, 2048, 4096)
	MemoryMB          int     // ECS task memory in MB
	DesiredCount      int     // Number of task replicas
	ExpectedUsers     int     // Expected concurrent users (for ALB LCU estimate)
	IncludeNATGateway bool    // Whether private subnets/NAT Gateway are used
	LogRetentionDays  int     // CloudWatch Logs retention
	HoursPerMonth     float64 // Defaults to 730 (24/7 operation)
}

// NewPricingEstimator creates a new pricing estimator.
// The Pricing API is only available in us-east-1 and ap-south-1, so we override the region.
func NewPricingEstimator(ctx context.Context) (*PricingEstimator, error) {
	// Load config with us-east-1 region (Pricing API requirement).
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("load AWS config for pricing: %w", err)
	}

	return &PricingEstimator{
		client: pricing.NewFromConfig(cfg),
		cache:  make(map[string]cachedPrice),
	}, nil
}

// EstimateCosts calculates estimated monthly costs for a deployment.
// Per spec ralph/specs/cost-estimation.md: Uses Pricing API with fallback to hardcoded estimates.
func (p *PricingEstimator) EstimateCosts(ctx context.Context, params EstimateParams) (*CostEstimate, error) {
	// Set defaults.
	if params.HoursPerMonth == 0 {
		params.HoursPerMonth = 730 // 24/7 operation
	}
	if params.CPUUnits == 0 {
		params.CPUUnits = 256
	}
	if params.MemoryMB == 0 {
		params.MemoryMB = 512
	}
	if params.DesiredCount == 0 {
		params.DesiredCount = 1
	}

	estimate := &CostEstimate{
		Region: params.Region,
		Assumptions: []string{
			fmt.Sprintf("%.0f hours/month (24/7 operation)", params.HoursPerMonth),
			fmt.Sprintf("Pricing for %s", params.Region),
			fmt.Sprintf("%d task replica(s)", params.DesiredCount),
		},
		Disclaimer: "Estimate based on AWS pricing. Actual costs may vary based on usage.",
	}

	// Try to get real prices from AWS Pricing API.
	// If it fails, fall back to regional hardcoded estimates.
	fargateVCPUPrice, fargateMBPrice, err := p.getFargatePricing(ctx, params.Region)
	if err != nil {
		slog.Warn("could not fetch Fargate pricing from AWS, using fallback estimates",
			slog.String("region", params.Region),
			slog.String("error", err.Error()))
		estimate.UsingFallback = true
		fargateVCPUPrice, fargateMBPrice = getFargateFallbackPrices(params.Region)
		estimate.Assumptions = append(estimate.Assumptions, "Using fallback pricing estimates (Pricing API unavailable)")
	}

	// Calculate ECS Fargate costs.
	// Per spec: (cpu_units / 1024) × vCPU_price × hours × count
	vcpus := float64(params.CPUUnits) / 1024.0
	memoryGB := float64(params.MemoryMB) / 1024.0
	fargateCPUCost := vcpus * fargateVCPUPrice * params.HoursPerMonth * float64(params.DesiredCount)
	fargateMemCost := memoryGB * fargateMBPrice * params.HoursPerMonth * float64(params.DesiredCount)
	fargateTotalCost := fargateCPUCost + fargateMemCost

	estimate.Services = append(estimate.Services, ServiceCost{
		Service:     "ECS Fargate",
		Description: fmt.Sprintf("%.2f vCPU, %d MB × %d tasks", vcpus, params.MemoryMB, params.DesiredCount),
		MonthlyCost: fargateTotalCost,
	})

	// Calculate ALB costs (fixed hourly + LCU estimate).
	albHourlyRate := getALBFallbackPrice(params.Region)
	albFixedCost := albHourlyRate * params.HoursPerMonth
	// LCU estimate based on expected users (simplified).
	// 1 LCU ≈ 25 new connections/sec or 3000 active connections.
	lcuEstimate := float64(params.ExpectedUsers) / 3000.0
	if lcuEstimate < 1 {
		lcuEstimate = 1
	}
	lcuHourlyRate := 0.008 // $0.008 per LCU-hour
	albLCUCost := lcuEstimate * lcuHourlyRate * params.HoursPerMonth
	albTotalCost := albFixedCost + albLCUCost

	estimate.Services = append(estimate.Services, ServiceCost{
		Service:     "Application Load Balancer",
		Description: fmt.Sprintf("Fixed rate + %.1f estimated LCU", lcuEstimate),
		MonthlyCost: albTotalCost,
	})

	// NAT Gateway costs (if private subnets are used).
	if params.IncludeNATGateway {
		natHourlyRate := 0.045 // $0.045/hour in most regions
		natFixedCost := natHourlyRate * params.HoursPerMonth
		// Estimate data processing (assume 10GB/month per task for image pulls, etc.).
		natDataCost := float64(params.DesiredCount) * 10 * 0.045 // $0.045/GB
		natTotalCost := natFixedCost + natDataCost

		estimate.Services = append(estimate.Services, ServiceCost{
			Service:     "NAT Gateway",
			Description: "Private subnet egress",
			MonthlyCost: natTotalCost,
		})
	}

	// CloudWatch Logs costs (estimate based on task count).
	// Assume 1 GB logs/month per task.
	logIngestionRate := 0.50 // $0.50/GB ingestion
	logStorageRate := 0.03   // $0.03/GB-month storage
	estimatedLogGB := float64(params.DesiredCount) * 1.0
	logCost := estimatedLogGB * (logIngestionRate + logStorageRate)

	estimate.Services = append(estimate.Services, ServiceCost{
		Service:     "CloudWatch Logs",
		Description: fmt.Sprintf("~%.0f GB/month estimated", estimatedLogGB),
		MonthlyCost: logCost,
	})

	// VPC/networking (minimal cost for IGW, route tables).
	vpcCost := 0.0 // VPC, IGW, route tables are free; subnets are free.
	estimate.Services = append(estimate.Services, ServiceCost{
		Service:     "VPC & Networking",
		Description: "VPC, subnets, Internet Gateway, route tables",
		MonthlyCost: vpcCost,
	})

	// Sum total.
	for _, svc := range estimate.Services {
		estimate.TotalMonthlyUSD += svc.MonthlyCost
	}

	return estimate, nil
}

// getFargatePricing fetches Fargate pricing from AWS Pricing API.
func (p *PricingEstimator) getFargatePricing(ctx context.Context, region string) (vcpuPrice, memPrice float64, err error) {
	// Check cache first.
	p.mu.RLock()
	vcpuCached, vcpuOK := p.cache["fargate-vcpu-"+region]
	memCached, memOK := p.cache["fargate-mem-"+region]
	p.mu.RUnlock()

	now := time.Now()
	if vcpuOK && memOK && now.Sub(vcpuCached.fetchedAt) < PriceCacheTTL && now.Sub(memCached.fetchedAt) < PriceCacheTTL {
		return vcpuCached.price, memCached.price, nil
	}

	// Query AWS Pricing API.
	// Note: This is a simplified query. Production should parse the full response.
	vcpuPrice, err = p.queryFargatePrice(ctx, region, "CPU")
	if err != nil {
		return 0, 0, fmt.Errorf("query Fargate CPU price: %w", err)
	}

	memPrice, err = p.queryFargatePrice(ctx, region, "Memory")
	if err != nil {
		return 0, 0, fmt.Errorf("query Fargate Memory price: %w", err)
	}

	// Update cache.
	p.mu.Lock()
	p.cache["fargate-vcpu-"+region] = cachedPrice{price: vcpuPrice, fetchedAt: now}
	p.cache["fargate-mem-"+region] = cachedPrice{price: memPrice, fetchedAt: now}
	p.mu.Unlock()

	return vcpuPrice, memPrice, nil
}

// queryFargatePrice queries the AWS Pricing API for Fargate pricing.
// The AWS Pricing API returns JSON with a complex nested structure that we need to parse.
func (p *PricingEstimator) queryFargatePrice(ctx context.Context, region, resourceType string) (float64, error) {
	// Map region to AWS region code format for Pricing API.
	regionName := getRegionName(region)

	var usageType string
	if resourceType == "CPU" {
		usageType = "Fargate-vCPU-Hours:perCPU"
	} else {
		usageType = "Fargate-GB-Hours"
	}

	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonECS"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("regionCode"),
				Value: aws.String(region),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(regionName),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("usagetype"),
				Value: aws.String(usageType),
			},
		},
		MaxResults: aws.Int32(1),
	}

	resp, err := p.client.GetProducts(ctx, input)
	if err != nil {
		return 0, err
	}

	if len(resp.PriceList) == 0 {
		return 0, fmt.Errorf("no pricing data found for %s in %s", resourceType, region)
	}

	// Parse the price from the AWS Pricing API response.
	// The response is a JSON string with nested structures for terms and price dimensions.
	price, err := parsePricingResponse(resp.PriceList[0])
	if err != nil {
		return 0, fmt.Errorf("failed to parse pricing response for %s: %w", resourceType, err)
	}

	return price, nil
}

// pricingResponse represents the AWS Pricing API response structure.
// The actual response is complex; this captures the fields we need.
type pricingResponse struct {
	Terms struct {
		OnDemand map[string]struct {
			PriceDimensions map[string]struct {
				PricePerUnit map[string]string `json:"pricePerUnit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

// parsePricingResponse extracts the USD price from an AWS Pricing API response.
func parsePricingResponse(priceListItem string) (float64, error) {
	var resp pricingResponse
	if err := json.Unmarshal([]byte(priceListItem), &resp); err != nil {
		return 0, fmt.Errorf("unmarshal pricing response: %w", err)
	}

	// Navigate the nested structure to find the USD price.
	// Structure: terms.OnDemand.<skuTermCode>.priceDimensions.<rateCode>.pricePerUnit.USD
	for _, termData := range resp.Terms.OnDemand {
		for _, dimension := range termData.PriceDimensions {
			if usdPrice, ok := dimension.PricePerUnit["USD"]; ok {
				price, err := strconv.ParseFloat(usdPrice, 64)
				if err != nil {
					return 0, fmt.Errorf("parse USD price %q: %w", usdPrice, err)
				}
				return price, nil
			}
		}
	}

	return 0, fmt.Errorf("no USD price found in pricing response")
}

// getFargateFallbackPrices returns hardcoded Fargate prices for a region.
// These are approximate and should be updated periodically.
func getFargateFallbackPrices(region string) (vcpuPrice, memPrice float64) {
	// Prices as of 2026-03 for common regions.
	// Per hour pricing for vCPU and per GB-hour for memory.
	switch region {
	case "us-east-1", "us-east-2", "us-west-2":
		return 0.04048, 0.004445 // US regions
	case "us-west-1":
		return 0.04656, 0.005113 // US West (N. California) - slightly higher
	case "eu-west-1", "eu-west-2", "eu-central-1":
		return 0.04456, 0.004890 // EU regions
	case "ap-northeast-1": // Tokyo
		return 0.05056, 0.005553
	case "ap-southeast-1", "ap-southeast-2": // Singapore, Sydney
		return 0.04656, 0.005113
	default:
		return 0.04048, 0.004445 // Default to US pricing
	}
}

// getALBFallbackPrice returns hardcoded ALB hourly price for a region.
func getALBFallbackPrice(region string) float64 {
	// ALB fixed hourly rate varies slightly by region.
	switch region {
	case "us-east-1", "us-east-2", "us-west-2":
		return 0.0225
	case "us-west-1":
		return 0.0252
	case "eu-west-1", "eu-west-2", "eu-central-1":
		return 0.0252
	case "ap-northeast-1":
		return 0.0270
	default:
		return 0.0225
	}
}

// getRegionName maps AWS region codes to human-readable names for the Pricing API.
func getRegionName(region string) string {
	names := map[string]string{
		"us-east-1":      "US East (N. Virginia)",
		"us-east-2":      "US East (Ohio)",
		"us-west-1":      "US West (N. California)",
		"us-west-2":      "US West (Oregon)",
		"eu-west-1":      "EU (Ireland)",
		"eu-west-2":      "EU (London)",
		"eu-central-1":   "EU (Frankfurt)",
		"ap-northeast-1": "Asia Pacific (Tokyo)",
		"ap-southeast-1": "Asia Pacific (Singapore)",
		"ap-southeast-2": "Asia Pacific (Sydney)",
	}
	if name, ok := names[region]; ok {
		return name
	}
	return region
}

// EstimateFromLocalState calculates estimated monthly spend from local deployment state.
// This is a fallback when Cost Explorer data is unavailable.
func EstimateFromLocalState(deployments []DeploymentCost) float64 {
	var total float64
	for _, d := range deployments {
		total += d.EstimatedMonthlyCost
	}
	return total
}

// DeploymentCost represents a deployment with its estimated cost.
type DeploymentCost struct {
	DeploymentID       string
	EstimatedMonthlyCost float64
}
