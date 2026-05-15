package eks

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
)

// DescribeRouteTablesAPI is the subset of the EC2 client used by
// GetClusterPrivateSubnets. Defined as an interface to allow mocking in tests.
type DescribeRouteTablesAPI interface {
	DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
}

// GetClusterPrivateSubnets returns the subnets of the given EKS cluster that
// do not have an active default route (0.0.0.0/0 or ::/0) going through an
// Internet Gateway. Fargate profiles only accept private subnets.
//
// Subnets without an explicit route table association fall back to the VPC's
// main route table. Routes going through NAT gateways, transit gateways,
// egress-only internet gateways, VPC peerings, or VPC endpoints are not
// considered public.
func GetClusterPrivateSubnets(ctx context.Context, ec2Client DescribeRouteTablesAPI, cluster *ekstypes.Cluster) ([]string, error) {
	if cluster == nil || cluster.ResourcesVpcConfig == nil {
		return nil, fmt.Errorf("cluster has no VPC configuration")
	}
	subnetIDs := cluster.ResourcesVpcConfig.SubnetIds
	vpcID := aws.ToString(cluster.ResourcesVpcConfig.VpcId)
	if len(subnetIDs) == 0 || vpcID == "" {
		return nil, fmt.Errorf("cluster has no subnets")
	}

	rtOut, err := ec2Client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe route tables for VPC %s: %w", vpcID, err)
	}

	explicitByAssoc := map[string]ec2types.RouteTable{}
	var mainRT *ec2types.RouteTable
	for _, rt := range rtOut.RouteTables {
		for _, assoc := range rt.Associations {
			// Skip associations that are not currently associated (e.g.
			// disassociating, disassociated, failed).
			if assoc.AssociationState != nil && assoc.AssociationState.State != ec2types.RouteTableAssociationStateCodeAssociated {
				continue
			}
			if aws.ToBool(assoc.Main) {
				rtCopy := rt
				mainRT = &rtCopy
			}
			if assoc.SubnetId != nil {
				explicitByAssoc[*assoc.SubnetId] = rt
			}
		}
	}

	var privateSubnets []string
	for _, subnetID := range subnetIDs {
		rt, ok := explicitByAssoc[subnetID]
		if !ok {
			if mainRT == nil {
				return nil, fmt.Errorf("subnet %s has no explicit route table association and the VPC %s has no main route table", subnetID, vpcID)
			}
			rt = *mainRT
		}
		if !hasDefaultRouteThroughIGW(rt) {
			privateSubnets = append(privateSubnets, subnetID)
		}
	}

	if len(privateSubnets) == 0 {
		return nil, fmt.Errorf("no private subnet found for cluster %s; pass --fargate-subnets to override auto-detection", aws.ToString(cluster.Name))
	}

	return privateSubnets, nil
}

// hasDefaultRouteThroughIGW returns true if the route table has an active
// default route (0.0.0.0/0 or ::/0) going through an Internet Gateway.
// EgressOnlyInternetGatewayId (prefix "eigw-") is intentionally excluded: EIGWs
// only allow outbound IPv6 traffic and do not make a subnet publicly reachable.
func hasDefaultRouteThroughIGW(rt ec2types.RouteTable) bool {
	for _, route := range rt.Routes {
		if route.State != ec2types.RouteStateActive {
			continue
		}
		if route.GatewayId == nil || !strings.HasPrefix(*route.GatewayId, "igw-") {
			continue
		}
		if aws.ToString(route.DestinationCidrBlock) == "0.0.0.0/0" ||
			aws.ToString(route.DestinationIpv6CidrBlock) == "::/0" {
			return true
		}
	}
	return false
}
