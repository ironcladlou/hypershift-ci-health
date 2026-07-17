package collector

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

const (
	tagProwJobID   = "hypershift.openshift.io/prow-job-id"
	tagInfraID     = "hypershift.openshift.io/infra-id"
	tagClusterName = "hypershift.openshift.io/cluster-name"
	tagName        = "Name"
)

// DiscoverEC2 queries each EC2 resource type using native Describe APIs with
// tag filters. This returns only resources that actually exist, avoiding the
// staleness problem of the Resource Groups Tagging API.
func DiscoverEC2(ctx context.Context, client *ec2.Client, region, jobIDFilter string) (*ResourceGraph, error) {
	jobs := make(map[string]*JobNode)
	filters := prowJobTagFilter(jobIDFilter)

	discoverers := []struct {
		name string
		fn   func(context.Context, *ec2.Client, string, []ec2types.Filter, map[string]*JobNode) error
	}{
		{"VPCs", discoverVPCs},
		{"Subnets", discoverSubnets},
		{"Route Tables", discoverRouteTables},
		{"Internet Gateways", discoverInternetGateways},
		{"NAT Gateways", discoverNATGateways},
		{"DHCP Options", discoverDHCPOptions},
		{"VPC Endpoints", discoverVPCEndpoints},
		{"Security Groups", discoverSecurityGroups},
		{"Instances", discoverInstances},
		{"Capacity Reservations", discoverCapacityReservations},
		{"Elastic IPs", discoverElasticIPs},
		{"Key Pairs", discoverKeyPairs},
	}

	for _, d := range discoverers {
		if err := d.fn(ctx, client, region, filters, jobs); err != nil {
			return nil, fmt.Errorf("discovering %s: %w", d.name, err)
		}
	}

	total := 0
	for _, j := range jobs {
		total += len(j.Resources)
	}
	fmt.Fprintf(os.Stderr, "  Found %d EC2 resources across %d prow jobs\n", total, len(jobs))

	graph := &ResourceGraph{}
	for _, j := range jobs {
		graph.Jobs = append(graph.Jobs, j)
	}
	return graph, nil
}

type TaggingAPI interface {
	GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error)
}

// DiscoverTagged uses the Resource Groups Tagging API scoped to specific
// resource type prefixes. Used for IAM and Route53 where native APIs don't
// support tag-based filtering.
func DiscoverTagged(ctx context.Context, client TaggingAPI, jobIDFilter string, resourceTypeFilters []string) (*ResourceGraph, error) {
	jobs := make(map[string]*JobNode)

	input := &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters: []tagtypes.TagFilter{
			{Key: aws.String(tagProwJobID)},
		},
		ResourceTypeFilters: resourceTypeFilters,
	}

	for {
		out, err := client.GetResources(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("GetResources: %w", err)
		}

		for _, mapping := range out.ResourceTagMappingList {
			var prowJobID, infraID, clusterName, name string
			for _, tag := range mapping.Tags {
				switch aws.ToString(tag.Key) {
				case tagProwJobID:
					prowJobID = aws.ToString(tag.Value)
				case tagInfraID:
					infraID = aws.ToString(tag.Value)
				case tagClusterName:
					clusterName = aws.ToString(tag.Value)
				case tagName:
					name = aws.ToString(tag.Value)
				}
			}
			if prowJobID == "" {
				continue
			}
			if jobIDFilter != "" && prowJobID != jobIDFilter {
				continue
			}

			arn := aws.ToString(mapping.ResourceARN)
			addResource(jobs, prowJobID, &ResourceNode{
				ARN:         arn,
				Type:        arnToResourceType(arn),
				Region:      arnToRegion(arn),
				ID:          arnToResourceID(arn),
				ConsoleURL:  consoleURLFromARN(arn),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			})
		}

		if out.PaginationToken == nil || aws.ToString(out.PaginationToken) == "" {
			break
		}
		input.PaginationToken = out.PaginationToken
	}

	total := 0
	for _, j := range jobs {
		total += len(j.Resources)
	}
	if total > 0 {
		fmt.Fprintf(os.Stderr, "  Found %d tagged resources (%v) across %d prow jobs\n", total, resourceTypeFilters, len(jobs))
	}

	graph := &ResourceGraph{}
	for _, j := range jobs {
		graph.Jobs = append(graph.Jobs, j)
	}
	return graph, nil
}

// --- EC2 resource discoverers ---

func discoverVPCs(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	paginator := ec2.NewDescribeVpcsPaginator(client, &ec2.DescribeVpcsInput{Filters: filters})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, v := range page.Vpcs {
			prowJobID, infraID, clusterName, name := extractEC2Tags(v.Tags)
			if prowJobID == "" {
				continue
			}
			id := aws.ToString(v.VpcId)
			addResource(jobs, prowJobID, &ResourceNode{
				Type:        "ec2:vpc",
				Region:      region,
				ID:          id,
				ConsoleURL:  ec2VPCConsoleURL(region, "VpcDetails:VpcId", id),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			})
		}
	}
	return nil
}

func discoverSubnets(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	paginator := ec2.NewDescribeSubnetsPaginator(client, &ec2.DescribeSubnetsInput{Filters: filters})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, s := range page.Subnets {
			prowJobID, infraID, clusterName, name := extractEC2Tags(s.Tags)
			if prowJobID == "" {
				continue
			}
			id := aws.ToString(s.SubnetId)
			addResource(jobs, prowJobID, &ResourceNode{
				Type:        "ec2:subnet",
				Region:      region,
				ID:          id,
				ConsoleURL:  ec2VPCConsoleURL(region, "SubnetDetails:subnetId", id),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			})
		}
	}
	return nil
}

func discoverRouteTables(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	paginator := ec2.NewDescribeRouteTablesPaginator(client, &ec2.DescribeRouteTablesInput{Filters: filters})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, rt := range page.RouteTables {
			prowJobID, infraID, clusterName, name := extractEC2Tags(rt.Tags)
			if prowJobID == "" {
				continue
			}
			id := aws.ToString(rt.RouteTableId)
			addResource(jobs, prowJobID, &ResourceNode{
				Type:        "ec2:route-table",
				Region:      region,
				ID:          id,
				ConsoleURL:  ec2VPCConsoleURL(region, "RouteTableDetails:RouteTableId", id),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			})
		}
	}
	return nil
}

func discoverInternetGateways(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	paginator := ec2.NewDescribeInternetGatewaysPaginator(client, &ec2.DescribeInternetGatewaysInput{Filters: filters})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, igw := range page.InternetGateways {
			prowJobID, infraID, clusterName, name := extractEC2Tags(igw.Tags)
			if prowJobID == "" {
				continue
			}
			id := aws.ToString(igw.InternetGatewayId)
			addResource(jobs, prowJobID, &ResourceNode{
				Type:        "ec2:internet-gateway",
				Region:      region,
				ID:          id,
				ConsoleURL:  ec2VPCConsoleURL(region, "InternetGateway:internetGatewayId", id),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			})
		}
	}
	return nil
}

func discoverNATGateways(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	ngwFilters := append([]ec2types.Filter{{Name: aws.String("state"), Values: []string{"pending", "available", "failed"}}}, filters...)
	paginator := ec2.NewDescribeNatGatewaysPaginator(client, &ec2.DescribeNatGatewaysInput{Filter: ngwFilters})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, ngw := range page.NatGateways {
			prowJobID, infraID, clusterName, name := extractEC2Tags(ngw.Tags)
			if prowJobID == "" {
				continue
			}
			id := aws.ToString(ngw.NatGatewayId)
			addResource(jobs, prowJobID, &ResourceNode{
				Type:        "ec2:natgateway",
				Region:      region,
				ID:          id,
				ConsoleURL:  ec2VPCConsoleURL(region, "NatGatewayDetails:natGatewayId", id),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			})
		}
	}
	return nil
}

func discoverDHCPOptions(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	out, err := client.DescribeDhcpOptions(ctx, &ec2.DescribeDhcpOptionsInput{Filters: filters})
	if err != nil {
		return err
	}
	for _, d := range out.DhcpOptions {
		prowJobID, infraID, clusterName, name := extractEC2Tags(d.Tags)
		if prowJobID == "" {
			continue
		}
		id := aws.ToString(d.DhcpOptionsId)
		addResource(jobs, prowJobID, &ResourceNode{
			Type:        "ec2:dhcp-options",
			Region:      region,
			ID:          id,
			ConsoleURL:  ec2VPCConsoleURL(region, "DHCPOptionSet:DhcpOptionsId", id),
			InfraID:     infraID,
			ClusterName: clusterName,
			Name:        name,
		})
	}
	return nil
}

func discoverVPCEndpoints(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	paginator := ec2.NewDescribeVpcEndpointsPaginator(client, &ec2.DescribeVpcEndpointsInput{Filters: filters})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, v := range page.VpcEndpoints {
			prowJobID, infraID, clusterName, name := extractEC2Tags(v.Tags)
			if prowJobID == "" {
				continue
			}
			id := aws.ToString(v.VpcEndpointId)
			addResource(jobs, prowJobID, &ResourceNode{
				Type:        "ec2:vpc-endpoint",
				Region:      region,
				ID:          id,
				ConsoleURL:  ec2VPCConsoleURL(region, "EndpointDetails:vpcEndpointId", id),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			})
		}
	}
	return nil
}

func discoverSecurityGroups(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	paginator := ec2.NewDescribeSecurityGroupsPaginator(client, &ec2.DescribeSecurityGroupsInput{Filters: filters})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, sg := range page.SecurityGroups {
			prowJobID, infraID, clusterName, name := extractEC2Tags(sg.Tags)
			if prowJobID == "" {
				continue
			}
			id := aws.ToString(sg.GroupId)
			addResource(jobs, prowJobID, &ResourceNode{
				Type:        "ec2:security-group",
				Region:      region,
				ID:          id,
				ConsoleURL:  ec2ConsoleURL(region, "SecurityGroup:groupId", id),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			})
		}
	}
	return nil
}

func discoverInstances(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	instFilters := append([]ec2types.Filter{{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "stopping", "stopped"}}}, filters...)
	paginator := ec2.NewDescribeInstancesPaginator(client, &ec2.DescribeInstancesInput{Filters: instFilters})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, res := range page.Reservations {
			for _, inst := range res.Instances {
				prowJobID, infraID, clusterName, name := extractEC2Tags(inst.Tags)
				if prowJobID == "" {
					continue
				}
				id := aws.ToString(inst.InstanceId)
				addResource(jobs, prowJobID, &ResourceNode{
					Type:        "ec2:instance",
					Region:      region,
					ID:          id,
					ConsoleURL:  ec2ConsoleURL(region, "InstanceDetails:instanceId", id),
					InfraID:     infraID,
					ClusterName: clusterName,
					Name:        name,
				})
			}
		}
	}
	return nil
}

func discoverCapacityReservations(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	crFilters := append([]ec2types.Filter{{Name: aws.String("state"), Values: []string{"active"}}}, filters...)
	paginator := ec2.NewDescribeCapacityReservationsPaginator(client, &ec2.DescribeCapacityReservationsInput{Filters: crFilters})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, cr := range page.CapacityReservations {
			prowJobID, infraID, clusterName, name := extractEC2Tags(cr.Tags)
			if prowJobID == "" {
				continue
			}
			id := aws.ToString(cr.CapacityReservationId)
			addResource(jobs, prowJobID, &ResourceNode{
				Type:        "ec2:capacity-reservation",
				Region:      region,
				ID:          id,
				ConsoleURL:  ec2ConsoleURL(region, "CapacityReservationDetails:capacityReservationId", id),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			})
		}
	}
	return nil
}

func discoverElasticIPs(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	out, err := client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{Filters: filters})
	if err != nil {
		return err
	}
	for _, addr := range out.Addresses {
		prowJobID, infraID, clusterName, name := extractEC2Tags(addr.Tags)
		if prowJobID == "" {
			continue
		}
		id := aws.ToString(addr.AllocationId)
		addResource(jobs, prowJobID, &ResourceNode{
			Type:        "ec2:elastic-ip",
			Region:      region,
			ID:          id,
			ConsoleURL:  ec2ConsoleURL(region, "ElasticIpDetails:AllocationId", id),
			InfraID:     infraID,
			ClusterName: clusterName,
			Name:        name,
		})
	}
	return nil
}

func discoverKeyPairs(ctx context.Context, client *ec2.Client, region string, filters []ec2types.Filter, jobs map[string]*JobNode) error {
	out, err := client.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{Filters: filters})
	if err != nil {
		return err
	}
	for _, kp := range out.KeyPairs {
		prowJobID, infraID, clusterName, name := extractEC2Tags(kp.Tags)
		if prowJobID == "" {
			continue
		}
		id := aws.ToString(kp.KeyPairId)
		addResource(jobs, prowJobID, &ResourceNode{
			Type:        "ec2:key-pair",
			Region:      region,
			ID:          id,
			ConsoleURL:  ec2ConsoleURL(region, "KeyPairs:search", aws.ToString(kp.KeyName)),
			InfraID:     infraID,
			ClusterName: clusterName,
			Name:        name,
		})
	}
	return nil
}

// --- helpers ---

func prowJobTagFilter(jobIDFilter string) []ec2types.Filter {
	if jobIDFilter != "" {
		return []ec2types.Filter{
			{Name: aws.String("tag:" + tagProwJobID), Values: []string{jobIDFilter}},
		}
	}
	return []ec2types.Filter{
		{Name: aws.String("tag-key"), Values: []string{tagProwJobID}},
	}
}

func extractEC2Tags(tags []ec2types.Tag) (prowJobID, infraID, clusterName, name string) {
	for _, tag := range tags {
		switch aws.ToString(tag.Key) {
		case tagProwJobID:
			prowJobID = aws.ToString(tag.Value)
		case tagInfraID:
			infraID = aws.ToString(tag.Value)
		case tagClusterName:
			clusterName = aws.ToString(tag.Value)
		case tagName:
			name = aws.ToString(tag.Value)
		}
	}
	return
}

func addResource(jobs map[string]*JobNode, prowJobID string, node *ResourceNode) {
	job, ok := jobs[prowJobID]
	if !ok {
		job = &JobNode{
			ID:    prowJobID,
			State: JobUnknown,
		}
		jobs[prowJobID] = job
	}
	job.Resources = append(job.Resources, node)
}

func ec2VPCConsoleURL(region, fragment, id string) string {
	return fmt.Sprintf("https://%s.console.aws.amazon.com/vpc/home?region=%s#%s=%s", region, region, fragment, id)
}

func ec2ConsoleURL(region, fragment, id string) string {
	return fmt.Sprintf("https://%s.console.aws.amazon.com/ec2/home?region=%s#%s=%s", region, region, fragment, id)
}

// ARN-based helpers for the tagging API path (IAM/Route53).

func consoleURLFromARN(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return ""
	}
	service := parts[2]
	resource := parts[5]

	var resType, resID string
	if idx := strings.Index(resource, "/"); idx != -1 {
		resType = resource[:idx]
		resID = resource[idx+1:]
	} else {
		resType = resource
	}

	switch service {
	case "iam":
		switch resType {
		case "instance-profile", "role":
			return "https://console.aws.amazon.com/iam/home#/roles/" + resID
		}
	case "route53":
		switch resType {
		case "hostedzone":
			return "https://console.aws.amazon.com/route53/v2/hostedzones#ListRecordSets/" + resID
		}
	}
	return ""
}

func arnToResourceType(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return "unknown"
	}
	service := parts[2]
	resource := parts[5]
	if idx := strings.IndexAny(resource, "/:"); idx != -1 {
		resource = resource[:idx]
	}
	return service + ":" + resource
}

func arnToRegion(arn string) string {
	parts := strings.SplitN(arn, ":", 5)
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}

func arnToResourceID(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return arn
	}
	resource := parts[5]
	if idx := strings.LastIndexAny(resource, "/:"); idx != -1 {
		return resource[idx+1:]
	}
	return resource
}
