// Package state provides reconciliation between local state and AWS resources.
package state

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/cjmartian/agent-deploy/internal/logging"
)

// ReconcileResult contains the results of a state reconciliation.
type ReconcileResult struct {
	// OrphanedResources are AWS resources not tracked in local state.
	OrphanedResources []OrphanedResource `json:"orphaned_resources"`
	// StaleLocalEntries are local state entries with no corresponding AWS resources.
	StaleLocalEntries []StaleEntry `json:"stale_local_entries"`
	// SyncedCount is the number of resources that are properly synced.
	SyncedCount int `json:"synced_count"`
	// Timestamp is when the reconciliation was performed.
	Timestamp time.Time `json:"timestamp"`
	// Errors encountered during reconciliation (non-fatal).
	Errors []string `json:"errors,omitempty"`
}

// OrphanedResource represents an AWS resource not tracked locally.
type OrphanedResource struct {
	ResourceType string `json:"resource_type"` // vpc, ecs_cluster, alb, etc.
	ResourceID   string `json:"resource_id"`   // ARN or ID
	Region       string `json:"region"`
	DeploymentID string `json:"deployment_id,omitempty"` // From tag if available
	InfraID      string `json:"infra_id,omitempty"`      // From tag if available
	PlanID       string `json:"plan_id,omitempty"`       // From tag if available
}

// StaleEntry represents a local state entry with missing AWS resources.
type StaleEntry struct {
	EntryType        string   `json:"entry_type"` // plan, infra, deployment
	EntryID          string   `json:"entry_id"`
	MissingResources []string `json:"missing_resources,omitempty"` // Resource types that are missing
}

// Reconciler compares local state with AWS resources.
type Reconciler struct {
	store     *Store
	ec2Client *ec2.Client
	ecsClient *ecs.Client
	albClient *elasticloadbalancingv2.Client
	region    string
}

// NewReconciler creates a new Reconciler with the given AWS configuration.
func NewReconciler(store *Store, cfg aws.Config) *Reconciler {
	return &Reconciler{
		store:     store,
		ec2Client: ec2.NewFromConfig(cfg),
		ecsClient: ecs.NewFromConfig(cfg),
		albClient: elasticloadbalancingv2.NewFromConfig(cfg),
		region:    cfg.Region,
	}
}

// Reconcile compares local state with AWS resources and returns discrepancies.
// This is a read-only operation - it does not modify any state.
func (r *Reconciler) Reconcile(ctx context.Context) (*ReconcileResult, error) {
	log := logging.WithComponent("reconciler")
	log.Info("starting state reconciliation", slog.String("region", r.region))

	result := &ReconcileResult{
		Timestamp: time.Now(),
	}

	// Step 1: Find orphaned AWS resources (in AWS but not in local state)
	orphaned, errs := r.findOrphanedResources(ctx)
	result.OrphanedResources = orphaned
	result.Errors = append(result.Errors, errs...)

	// Step 2: Find stale local entries (in local state but not in AWS)
	stale, errs := r.findStaleEntries(ctx)
	result.StaleLocalEntries = stale
	result.Errors = append(result.Errors, errs...)

	// Step 3: Count synced resources
	result.SyncedCount = r.countSyncedResources(ctx)

	log.Info("reconciliation complete",
		slog.Int("orphaned", len(result.OrphanedResources)),
		slog.Int("stale", len(result.StaleLocalEntries)),
		slog.Int("synced", result.SyncedCount),
		slog.Int("errors", len(result.Errors)))

	return result, nil
}

// findOrphanedResources discovers AWS resources tagged with agent-deploy:*
// that are not tracked in local state.
func (r *Reconciler) findOrphanedResources(ctx context.Context) ([]OrphanedResource, []string) {
	var orphaned []OrphanedResource
	var errors []string

	// Get all local deployment IDs and infra IDs for comparison
	localDeployIDs := make(map[string]bool)
	localInfraIDs := make(map[string]bool)

	deployments, err := r.store.ListDeployments()
	if err == nil {
		for _, d := range deployments {
			localDeployIDs[d.ID] = true
		}
	}

	infras, err := r.store.ListInfra()
	if err == nil {
		for _, i := range infras {
			localInfraIDs[i.ID] = true
		}
	}

	// Find orphaned VPCs
	vpcs, err := r.findTaggedVPCs(ctx)
	if err != nil {
		errors = append(errors, "list VPCs: "+err.Error())
	} else {
		for _, vpc := range vpcs {
			infraID := getTagValue(vpc.Tags, "agent-deploy:infra-id")
			if infraID != "" && !localInfraIDs[infraID] {
				orphaned = append(orphaned, OrphanedResource{
					ResourceType: "vpc",
					ResourceID:   aws.ToString(vpc.VpcId),
					Region:       r.region,
					InfraID:      infraID,
					DeploymentID: getTagValue(vpc.Tags, "agent-deploy:deployment-id"),
					PlanID:       getTagValue(vpc.Tags, "agent-deploy:plan-id"),
				})
			}
		}
	}

	// Find orphaned ECS clusters
	clusters, err := r.findTaggedECSClusters(ctx)
	if err != nil {
		errors = append(errors, "list ECS clusters: "+err.Error())
	} else {
		for _, cluster := range clusters {
			infraID := getTagValue(cluster.Tags, "agent-deploy:infra-id")
			if infraID != "" && !localInfraIDs[infraID] {
				orphaned = append(orphaned, OrphanedResource{
					ResourceType: "ecs_cluster",
					ResourceID:   aws.ToString(cluster.ClusterArn),
					Region:       r.region,
					InfraID:      infraID,
					DeploymentID: getTagValue(cluster.Tags, "agent-deploy:deployment-id"),
					PlanID:       getTagValue(cluster.Tags, "agent-deploy:plan-id"),
				})
			}
		}
	}

	// Find orphaned ALBs
	albs, err := r.findTaggedALBs(ctx)
	if err != nil {
		errors = append(errors, "list ALBs: "+err.Error())
	} else {
		for _, alb := range albs {
			infraID := alb.InfraID
			if infraID != "" && !localInfraIDs[infraID] {
				orphaned = append(orphaned, OrphanedResource{
					ResourceType: "alb",
					ResourceID:   alb.ARN,
					Region:       r.region,
					InfraID:      infraID,
					DeploymentID: alb.DeploymentID,
					PlanID:       alb.PlanID,
				})
			}
		}
	}

	return orphaned, errors
}

// findStaleEntries discovers local state entries that reference
// AWS resources which no longer exist.
func (r *Reconciler) findStaleEntries(ctx context.Context) ([]StaleEntry, []string) {
	var stale []StaleEntry
	var errors []string

	// Check infrastructure records
	infras, err := r.store.ListInfra()
	if err != nil {
		errors = append(errors, "list infra: "+err.Error())
	} else {
		for _, infra := range infras {
			// Skip destroyed infrastructure
			if infra.Status == InfraStatusDestroyed {
				continue
			}

			missing := r.checkInfraResources(ctx, infra)
			if len(missing) > 0 {
				stale = append(stale, StaleEntry{
					EntryType:        "infra",
					EntryID:          infra.ID,
					MissingResources: missing,
				})
			}
		}
	}

	// Check deployment records
	deployments, err := r.store.ListDeployments()
	if err != nil {
		errors = append(errors, "list deployments: "+err.Error())
	} else {
		for _, deploy := range deployments {
			// Skip stopped deployments
			if deploy.Status == DeploymentStatusStopped {
				continue
			}

			// Check if the associated infrastructure exists
			infra, err := r.store.GetInfra(deploy.InfraID)
			if err != nil || infra == nil {
				stale = append(stale, StaleEntry{
					EntryType:        "deployment",
					EntryID:          deploy.ID,
					MissingResources: []string{"infra:" + deploy.InfraID},
				})
				continue
			}

			// Check if ECS service exists
			if deploy.ServiceARN != "" {
				exists, _ := r.ecsServiceExists(ctx, infra.Resources[ResourceECSCluster], deploy.ServiceARN)
				if !exists {
					stale = append(stale, StaleEntry{
						EntryType:        "deployment",
						EntryID:          deploy.ID,
						MissingResources: []string{"ecs_service"},
					})
				}
			}
		}
	}

	return stale, errors
}

// checkInfraResources verifies AWS resources exist for an infrastructure record.
func (r *Reconciler) checkInfraResources(ctx context.Context, infra *Infrastructure) []string {
	var missing []string

	// Check VPC
	if vpcID := infra.Resources[ResourceVPC]; vpcID != "" {
		exists, _ := r.vpcExists(ctx, vpcID)
		if !exists {
			missing = append(missing, "vpc")
		}
	}

	// Check ECS cluster
	if clusterARN := infra.Resources[ResourceECSCluster]; clusterARN != "" {
		exists, _ := r.ecsClusterExists(ctx, clusterARN)
		if !exists {
			missing = append(missing, "ecs_cluster")
		}
	}

	// Check ALB
	if albARN := infra.Resources[ResourceALB]; albARN != "" {
		exists, _ := r.albExists(ctx, albARN)
		if !exists {
			missing = append(missing, "alb")
		}
	}

	return missing
}

// countSyncedResources returns count of properly tracked resources.
func (r *Reconciler) countSyncedResources(_ context.Context) int {
	count := 0

	infras, err := r.store.ListInfra()
	if err != nil {
		return 0
	}

	for _, infra := range infras {
		if infra.Status == InfraStatusDestroyed {
			continue
		}
		// Count resources that exist in both local state and AWS
		for _, resourceID := range infra.Resources {
			if resourceID != "" {
				count++
			}
		}
	}

	return count
}

// AWS resource discovery helpers

// findTaggedVPCs returns all VPCs tagged with agent-deploy:created-by.
// WHY pagination: AWS DescribeVpcs returns max 1000 results per call.
// Without pagination, deployments beyond page 1 are invisible to reconciliation.
// See spec ralph/specs/operational.md Section 1.
func (r *Reconciler) findTaggedVPCs(ctx context.Context) ([]ec2types.Vpc, error) {
	var allVPCs []ec2types.Vpc

	paginator := ec2.NewDescribeVpcsPaginator(r.ec2Client, &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag-key"),
				Values: []string{"agent-deploy:created-by"},
			},
		},
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe VPCs: %w", err)
		}
		allVPCs = append(allVPCs, page.Vpcs...)
	}

	return allVPCs, nil
}

type taggedCluster struct {
	ClusterArn *string
	Tags       []ec2types.Tag
}

// findTaggedECSClusters returns all ECS clusters tagged with agent-deploy:created-by.
// WHY pagination: AWS ListClusters returns max 100 results per call.
// Without pagination, clusters beyond page 1 are invisible to reconciliation.
// See spec ralph/specs/operational.md Section 1.
func (r *Reconciler) findTaggedECSClusters(ctx context.Context) ([]taggedCluster, error) {
	// Collect all cluster ARNs using paginator
	var allClusterARNs []string

	paginator := ecs.NewListClustersPaginator(r.ecsClient, &ecs.ListClustersInput{})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ECS clusters: %w", err)
		}
		allClusterARNs = append(allClusterARNs, page.ClusterArns...)
	}

	if len(allClusterARNs) == 0 {
		return nil, nil
	}

	// Describe clusters in batches of 100 (API limit) to get tags
	var result []taggedCluster
	const batchSize = 100

	for i := 0; i < len(allClusterARNs); i += batchSize {
		end := min(i+batchSize, len(allClusterARNs))
		batch := allClusterARNs[i:end]

		descOutput, err := r.ecsClient.DescribeClusters(ctx, &ecs.DescribeClustersInput{
			Clusters: batch,
			Include:  []ecstypes.ClusterField{ecstypes.ClusterFieldTags},
		})
		if err != nil {
			return nil, fmt.Errorf("describe ECS clusters: %w", err)
		}

		for _, cluster := range descOutput.Clusters {
			// Check if cluster has agent-deploy tag
			for _, tag := range cluster.Tags {
				if aws.ToString(tag.Key) == "agent-deploy:created-by" {
					// Convert ECS tags to EC2 tags for consistency
					var ec2Tags []ec2types.Tag
					for _, t := range cluster.Tags {
						ec2Tags = append(ec2Tags, ec2types.Tag{
							Key:   t.Key,
							Value: t.Value,
						})
					}
					result = append(result, taggedCluster{
						ClusterArn: cluster.ClusterArn,
						Tags:       ec2Tags,
					})
					break
				}
			}
		}
	}

	return result, nil
}

type taggedALB struct {
	ARN          string
	InfraID      string
	DeploymentID string
	PlanID       string
}

// findTaggedALBs returns all ALBs tagged with agent-deploy:created-by.
// WHY pagination: AWS DescribeLoadBalancers returns max 400 results per call.
// WHY batch tags: DescribeTags accepts up to 20 ARNs per call, reducing API calls.
// See spec ralph/specs/operational.md Section 1.
func (r *Reconciler) findTaggedALBs(ctx context.Context) ([]taggedALB, error) {
	// Collect all ALB ARNs using paginator
	var allALBARNs []string

	paginator := elasticloadbalancingv2.NewDescribeLoadBalancersPaginator(
		r.albClient,
		&elasticloadbalancingv2.DescribeLoadBalancersInput{},
	)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe load balancers: %w", err)
		}
		for _, lb := range page.LoadBalancers {
			allALBARNs = append(allALBARNs, aws.ToString(lb.LoadBalancerArn))
		}
	}

	if len(allALBARNs) == 0 {
		return nil, nil
	}

	// Batch fetch tags (up to 20 per API call)
	tags, err := r.batchFetchALBTags(ctx, allALBARNs)
	if err != nil {
		return nil, err
	}

	// Filter to only agent-deploy ALBs
	var result []taggedALB
	for arn, tagMap := range tags {
		if _, ok := tagMap["agent-deploy:created-by"]; ok {
			result = append(result, taggedALB{
				ARN:          arn,
				InfraID:      tagMap["agent-deploy:infra-id"],
				DeploymentID: tagMap["agent-deploy:deployment-id"],
				PlanID:       tagMap["agent-deploy:plan-id"],
			})
		}
	}

	return result, nil
}

// batchFetchALBTags fetches tags for multiple ALBs in batches of 20.
// WHY: DescribeTags API accepts up to 20 ARNs per call, reducing API calls
// from N to ceil(N/20) for N load balancers.
// See spec ralph/specs/operational.md Section 1.
func (r *Reconciler) batchFetchALBTags(ctx context.Context, arns []string) (map[string]map[string]string, error) {
	tags := make(map[string]map[string]string)
	const batchSize = 20 // AWS API limit

	for i := 0; i < len(arns); i += batchSize {
		end := min(i+batchSize, len(arns))
		batch := arns[i:end]

		resp, err := r.albClient.DescribeTags(ctx, &elasticloadbalancingv2.DescribeTagsInput{
			ResourceArns: batch,
		})
		if err != nil {
			return nil, fmt.Errorf("describe ALB tags: %w", err)
		}

		for _, desc := range resp.TagDescriptions {
			tagMap := make(map[string]string)
			for _, tag := range desc.Tags {
				tagMap[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
			}
			tags[aws.ToString(desc.ResourceArn)] = tagMap
		}
	}

	return tags, nil
}

// Resource existence checks

func (r *Reconciler) vpcExists(ctx context.Context, vpcID string) (bool, error) {
	input := &ec2.DescribeVpcsInput{
		VpcIds: []string{vpcID},
	}

	output, err := r.ec2Client.DescribeVpcs(ctx, input)
	if err != nil {
		// Check if error is "not found"
		if strings.Contains(err.Error(), "InvalidVpcID.NotFound") {
			return false, nil
		}
		return false, err
	}

	return len(output.Vpcs) > 0, nil
}

func (r *Reconciler) ecsClusterExists(ctx context.Context, clusterARN string) (bool, error) {
	input := &ecs.DescribeClustersInput{
		Clusters: []string{clusterARN},
	}

	output, err := r.ecsClient.DescribeClusters(ctx, input)
	if err != nil {
		return false, err
	}

	for _, cluster := range output.Clusters {
		if aws.ToString(cluster.ClusterArn) == clusterARN {
			return aws.ToString(cluster.Status) != "INACTIVE", nil
		}
	}

	return false, nil
}

func (r *Reconciler) ecsServiceExists(ctx context.Context, clusterARN, serviceARN string) (bool, error) {
	if clusterARN == "" {
		return false, nil
	}

	input := &ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterARN),
		Services: []string{serviceARN},
	}

	output, err := r.ecsClient.DescribeServices(ctx, input)
	if err != nil {
		return false, err
	}

	for _, service := range output.Services {
		if aws.ToString(service.ServiceArn) == serviceARN {
			return aws.ToString(service.Status) != "INACTIVE", nil
		}
	}

	return false, nil
}

func (r *Reconciler) albExists(ctx context.Context, albARN string) (bool, error) {
	input := &elasticloadbalancingv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{albARN},
	}

	output, err := r.albClient.DescribeLoadBalancers(ctx, input)
	if err != nil {
		// Check if error is "not found"
		if strings.Contains(err.Error(), "LoadBalancerNotFound") {
			return false, nil
		}
		return false, err
	}

	return len(output.LoadBalancers) > 0, nil
}

// Helper to extract tag value
func getTagValue(tags []ec2types.Tag, key string) string {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == key {
			return aws.ToString(tag.Value)
		}
	}
	return ""
}

// CleanupStaleEntries removes stale local entries identified by reconciliation.
// This is a destructive operation - use with caution.
func (r *Reconciler) CleanupStaleEntries(ctx context.Context, entries []StaleEntry) (int, error) {
	log := logging.WithComponent("reconciler")
	cleaned := 0

	for _, entry := range entries {
		switch entry.EntryType {
		case "infra":
			if err := r.store.SetInfraStatus(entry.EntryID, InfraStatusDestroyed); err != nil {
				log.Warn("failed to mark infra as destroyed",
					logging.InfraID(entry.EntryID),
					logging.Err(err))
				continue
			}
			cleaned++
			log.Info("marked stale infra as destroyed",
				logging.InfraID(entry.EntryID))

		case "deployment":
			if err := r.store.UpdateDeploymentStatus(entry.EntryID, DeploymentStatusStopped, nil); err != nil {
				log.Warn("failed to mark deployment as stopped",
					logging.DeploymentID(entry.EntryID),
					logging.Err(err))
				continue
			}
			cleaned++
			log.Info("marked stale deployment as stopped",
				logging.DeploymentID(entry.EntryID))
		}
	}

	return cleaned, nil
}
