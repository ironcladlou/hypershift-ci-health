package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

const (
	tagProwJobID   = "hypershift.openshift.io/prow-job-id"
	tagInfraID     = "hypershift.openshift.io/infra-id"
	tagClusterName = "hypershift.openshift.io/cluster-name"
	tagName        = "Name"
)

type TaggingAPI interface {
	GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error)
}

func Discover(ctx context.Context, client TaggingAPI, jobIDFilter string) (*ResourceGraph, error) {
	jobs := make(map[string]*JobNode)

	input := &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters: []tagtypes.TagFilter{
			{Key: aws.String(tagProwJobID)},
		},
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
			node := &ResourceNode{
				ARN:         arn,
				Type:        arnToResourceType(arn),
				Region:      arnToRegion(arn),
				ID:          arnToResourceID(arn),
				ConsoleURL:  arnToConsoleURL(arn),
				InfraID:     infraID,
				ClusterName: clusterName,
				Name:        name,
			}

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

		if out.PaginationToken == nil || aws.ToString(out.PaginationToken) == "" {
			break
		}
		input.PaginationToken = out.PaginationToken
	}

	total := 0
	for _, j := range jobs {
		total += len(j.Resources)
	}
	fmt.Fprintf(os.Stderr, "  Found %d resources across %d prow jobs\n", total, len(jobs))

	graph := &ResourceGraph{}
	for _, j := range jobs {
		graph.Jobs = append(graph.Jobs, j)
	}
	return graph, nil
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

func arnToConsoleURL(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return ""
	}
	service := parts[2]
	region := parts[3]
	resource := parts[5]

	var resType, resID string
	if idx := strings.Index(resource, "/"); idx != -1 {
		resType = resource[:idx]
		resID = resource[idx+1:]
	} else {
		resType = resource
	}

	switch service {
	case "ec2":
		base := fmt.Sprintf("https://%s.console.aws.amazon.com", region)
		switch resType {
		case "vpc":
			return base + "/vpc/home?region=" + region + "#VpcDetails:VpcId=" + resID
		case "subnet":
			return base + "/vpc/home?region=" + region + "#SubnetDetails:subnetId=" + resID
		case "route-table":
			return base + "/vpc/home?region=" + region + "#RouteTableDetails:RouteTableId=" + resID
		case "internet-gateway":
			return base + "/vpc/home?region=" + region + "#InternetGateway:internetGatewayId=" + resID
		case "natgateway":
			return base + "/vpc/home?region=" + region + "#NatGatewayDetails:natGatewayId=" + resID
		case "dhcp-options":
			return base + "/vpc/home?region=" + region + "#DHCPOptionSet:DhcpOptionsId=" + resID
		case "vpc-endpoint":
			return base + "/vpc/home?region=" + region + "#EndpointDetails:vpcEndpointId=" + resID
		case "security-group":
			return base + "/ec2/home?region=" + region + "#SecurityGroup:groupId=" + resID
		case "instance":
			return base + "/ec2/home?region=" + region + "#InstanceDetails:instanceId=" + resID
		case "capacity-reservation":
			return base + "/ec2/home?region=" + region + "#CapacityReservationDetails:capacityReservationId=" + resID
		case "elastic-ip":
			return base + "/ec2/home?region=" + region + "#ElasticIpDetails:AllocationId=" + resID
		case "key-pair":
			return base + "/ec2/home?region=" + region + "#KeyPairs:search=" + resID
		}
	case "iam":
		switch resType {
		case "instance-profile":
			return "https://console.aws.amazon.com/iam/home#/roles/" + resID
		case "role":
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
