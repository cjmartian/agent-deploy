package spending

import (
	"testing"
)

func TestGetFargateFallbackPrices(t *testing.T) {
	tests := []struct {
		region      string
		wantVCPUMin float64
		wantVCPUMax float64
		wantMemMin  float64
		wantMemMax  float64
	}{
		{
			region:      "us-east-1",
			wantVCPUMin: 0.04,
			wantVCPUMax: 0.05,
			wantMemMin:  0.004,
			wantMemMax:  0.006,
		},
		{
			region:      "eu-west-1",
			wantVCPUMin: 0.04,
			wantVCPUMax: 0.05,
			wantMemMin:  0.004,
			wantMemMax:  0.006,
		},
		{
			region:      "ap-northeast-1",
			wantVCPUMin: 0.05,
			wantVCPUMax: 0.06,
			wantMemMin:  0.005,
			wantMemMax:  0.006,
		},
		{
			region:      "unknown-region",
			wantVCPUMin: 0.04,
			wantVCPUMax: 0.05,
			wantMemMin:  0.004,
			wantMemMax:  0.006,
		},
	}

	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			vcpu, mem := getFargateFallbackPrices(tt.region)

			if vcpu < tt.wantVCPUMin || vcpu > tt.wantVCPUMax {
				t.Errorf("vCPU price for %s = %f, want in range [%f, %f]",
					tt.region, vcpu, tt.wantVCPUMin, tt.wantVCPUMax)
			}
			if mem < tt.wantMemMin || mem > tt.wantMemMax {
				t.Errorf("Memory price for %s = %f, want in range [%f, %f]",
					tt.region, mem, tt.wantMemMin, tt.wantMemMax)
			}
		})
	}
}

func TestGetALBFallbackPrice(t *testing.T) {
	tests := []struct {
		region  string
		wantMin float64
		wantMax float64
	}{
		{"us-east-1", 0.02, 0.03},
		{"eu-west-1", 0.02, 0.03},
		{"ap-northeast-1", 0.02, 0.03},
		{"unknown", 0.02, 0.03},
	}

	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			price := getALBFallbackPrice(tt.region)
			if price < tt.wantMin || price > tt.wantMax {
				t.Errorf("ALB price for %s = %f, want in range [%f, %f]",
					tt.region, price, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestGetRegionName(t *testing.T) {
	tests := []struct {
		region   string
		wantName string
	}{
		{"us-east-1", "US East (N. Virginia)"},
		{"us-west-2", "US West (Oregon)"},
		{"eu-west-1", "EU (Ireland)"},
		{"ap-northeast-1", "Asia Pacific (Tokyo)"},
		{"unknown-region", "unknown-region"}, // Falls back to region code
	}

	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			got := getRegionName(tt.region)
			if got != tt.wantName {
				t.Errorf("getRegionName(%q) = %q, want %q", tt.region, got, tt.wantName)
			}
		})
	}
}

func TestCostEstimate_Defaults(t *testing.T) {
	// Test that EstimateParams has sensible defaults applied
	params := EstimateParams{
		Region:        "us-east-1",
		ExpectedUsers: 100,
	}

	if params.Region != "us-east-1" {
		t.Errorf("Region = %q, want %q", params.Region, "us-east-1")
	}
	if params.ExpectedUsers != 100 {
		t.Errorf("ExpectedUsers = %d, want 100", params.ExpectedUsers)
	}

	// Without a real pricing client, we can't call EstimateCosts.
	// But we can verify the struct works correctly.
	if params.CPUUnits != 0 {
		t.Errorf("CPUUnits default should be 0 (applied in EstimateCosts)")
	}
	if params.MemoryMB != 0 {
		t.Errorf("MemoryMB default should be 0 (applied in EstimateCosts)")
	}
	if params.DesiredCount != 0 {
		t.Errorf("DesiredCount default should be 0 (applied in EstimateCosts)")
	}
}

func TestServiceCost_Structure(t *testing.T) {
	cost := ServiceCost{
		Service:     "ECS Fargate",
		Description: "0.25 vCPU, 512 MB × 1 tasks",
		MonthlyCost: 14.26,
	}

	if cost.Service != "ECS Fargate" {
		t.Errorf("Service = %q, want %q", cost.Service, "ECS Fargate")
	}
	if cost.MonthlyCost != 14.26 {
		t.Errorf("MonthlyCost = %f, want %f", cost.MonthlyCost, 14.26)
	}
	if cost.Description != "0.25 vCPU, 512 MB × 1 tasks" {
		t.Errorf("Description = %q, want %q", cost.Description, "0.25 vCPU, 512 MB × 1 tasks")
	}
}

func TestCostEstimate_Structure(t *testing.T) {
	estimate := CostEstimate{
		Region:          "us-east-1",
		TotalMonthlyUSD: 50.00,
		UsingFallback:   true,
		Disclaimer:      "Test disclaimer",
		Services: []ServiceCost{
			{Service: "ECS Fargate", MonthlyCost: 25.00},
			{Service: "ALB", MonthlyCost: 25.00},
		},
		Assumptions: []string{"730 hours/month"},
	}

	if estimate.Region != "us-east-1" {
		t.Errorf("Region = %q, want %q", estimate.Region, "us-east-1")
	}
	if estimate.TotalMonthlyUSD != 50.00 {
		t.Errorf("TotalMonthlyUSD = %f, want %f", estimate.TotalMonthlyUSD, 50.00)
	}
	if !estimate.UsingFallback {
		t.Error("UsingFallback should be true")
	}
	if len(estimate.Services) != 2 {
		t.Errorf("len(Services) = %d, want 2", len(estimate.Services))
	}
	if estimate.Disclaimer != "Test disclaimer" {
		t.Errorf("Disclaimer = %q, want %q", estimate.Disclaimer, "Test disclaimer")
	}
	if len(estimate.Assumptions) != 1 || estimate.Assumptions[0] != "730 hours/month" {
		t.Errorf("Assumptions = %v, want [730 hours/month]", estimate.Assumptions)
	}
}

func TestEstimateFromLocalState(t *testing.T) {
	deployments := []DeploymentCost{
		{DeploymentID: "deploy-1", EstimatedMonthlyCost: 25.00},
		{DeploymentID: "deploy-2", EstimatedMonthlyCost: 35.00},
		{DeploymentID: "deploy-3", EstimatedMonthlyCost: 40.00},
	}

	total := EstimateFromLocalState(deployments)
	expected := 100.00

	if total != expected {
		t.Errorf("EstimateFromLocalState() = %f, want %f", total, expected)
	}
}

func TestEstimateFromLocalState_Empty(t *testing.T) {
	total := EstimateFromLocalState(nil)
	if total != 0 {
		t.Errorf("EstimateFromLocalState(nil) = %f, want 0", total)
	}

	total = EstimateFromLocalState([]DeploymentCost{})
	if total != 0 {
		t.Errorf("EstimateFromLocalState([]) = %f, want 0", total)
	}
}

func TestPriceCacheTTL(t *testing.T) {
	// Verify the cache TTL constant is reasonable.
	if PriceCacheTTL.Hours() != 24 {
		t.Errorf("PriceCacheTTL = %v, want 24 hours", PriceCacheTTL)
	}
}

// TestParsePricingResponse tests parsing of AWS Pricing API responses.
func TestParsePricingResponse(t *testing.T) {
	tests := []struct {
		name      string
		jsonInput string
		wantPrice float64
		wantErr   bool
	}{
		{
			name: "valid_fargate_cpu_pricing",
			jsonInput: `{
				"terms": {
					"OnDemand": {
						"ABCD.1234": {
							"priceDimensions": {
								"ABCD.1234.RATE": {
									"pricePerUnit": {
										"USD": "0.04048"
									}
								}
							}
						}
					}
				}
			}`,
			wantPrice: 0.04048,
			wantErr:   false,
		},
		{
			name: "valid_fargate_memory_pricing",
			jsonInput: `{
				"terms": {
					"OnDemand": {
						"EFGH.5678": {
							"priceDimensions": {
								"EFGH.5678.RATE": {
									"pricePerUnit": {
										"USD": "0.004445"
									}
								}
							}
						}
					}
				}
			}`,
			wantPrice: 0.004445,
			wantErr:   false,
		},
		{
			name: "multiple_terms_takes_first_price",
			jsonInput: `{
				"terms": {
					"OnDemand": {
						"SKU1.TERM1": {
							"priceDimensions": {
								"SKU1.TERM1.RATE": {
									"pricePerUnit": {
										"USD": "0.05"
									}
								}
							}
						},
						"SKU2.TERM2": {
							"priceDimensions": {
								"SKU2.TERM2.RATE": {
									"pricePerUnit": {
										"USD": "0.06"
									}
								}
							}
						}
					}
				}
			}`,
			wantPrice: 0.05, // Could be either, but should succeed
			wantErr:   false,
		},
		{
			name:      "invalid_json",
			jsonInput: `not valid json`,
			wantPrice: 0,
			wantErr:   true,
		},
		{
			name: "missing_terms",
			jsonInput: `{
				"product": {}
			}`,
			wantPrice: 0,
			wantErr:   true,
		},
		{
			name: "empty_on_demand",
			jsonInput: `{
				"terms": {
					"OnDemand": {}
				}
			}`,
			wantPrice: 0,
			wantErr:   true,
		},
		{
			name: "missing_usd_price",
			jsonInput: `{
				"terms": {
					"OnDemand": {
						"SKU.TERM": {
							"priceDimensions": {
								"SKU.TERM.RATE": {
									"pricePerUnit": {
										"EUR": "0.04"
									}
								}
							}
						}
					}
				}
			}`,
			wantPrice: 0,
			wantErr:   true,
		},
		{
			name: "invalid_price_format",
			jsonInput: `{
				"terms": {
					"OnDemand": {
						"SKU.TERM": {
							"priceDimensions": {
								"SKU.TERM.RATE": {
									"pricePerUnit": {
										"USD": "not-a-number"
									}
								}
							}
						}
					}
				}
			}`,
			wantPrice: 0,
			wantErr:   true,
		},
		{
			name: "zero_price_valid",
			jsonInput: `{
				"terms": {
					"OnDemand": {
						"SKU.TERM": {
							"priceDimensions": {
								"SKU.TERM.RATE": {
									"pricePerUnit": {
										"USD": "0.0000000000"
									}
								}
							}
						}
					}
				}
			}`,
			wantPrice: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, err := parsePricingResponse(tt.jsonInput)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parsePricingResponse() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("parsePricingResponse() unexpected error: %v", err)
				return
			}

			// For tests with multiple possible prices, just check it's > 0.
			if tt.name == "multiple_terms_takes_first_price" {
				if price <= 0 {
					t.Errorf("parsePricingResponse() = %f, want > 0", price)
				}
				return
			}

			if price != tt.wantPrice {
				t.Errorf("parsePricingResponse() = %f, want %f", price, tt.wantPrice)
			}
		})
	}
}
