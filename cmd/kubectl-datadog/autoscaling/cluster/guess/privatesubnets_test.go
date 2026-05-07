package guess

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEC2DescribeRouteTables struct {
	routeTables []ec2types.RouteTable
	err         error
}

func (f *fakeEC2DescribeRouteTables) DescribeRouteTables(_ context.Context, _ *ec2.DescribeRouteTablesInput, _ ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ec2.DescribeRouteTablesOutput{RouteTables: f.routeTables}, nil
}

func association(subnetID string, main bool) ec2types.RouteTableAssociation {
	a := ec2types.RouteTableAssociation{Main: aws.Bool(main)}
	if subnetID != "" {
		a.SubnetId = aws.String(subnetID)
	}
	return a
}

func igwRoute() ec2types.Route {
	return ec2types.Route{
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String("igw-0123456789abcdef0"),
		State:                ec2types.RouteStateActive,
	}
}

func natRoute() ec2types.Route {
	return ec2types.Route{
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		NatGatewayId:         aws.String("nat-0123456789abcdef0"),
		State:                ec2types.RouteStateActive,
	}
}

func TestGetClusterPrivateSubnets(t *testing.T) {
	const vpcID = "vpc-aaaa"

	for _, tc := range []struct {
		name            string
		cluster         *ekstypes.Cluster // overrides the default cluster built from clusterSubnets
		clusterSubnets  []string
		routeTables     []ec2types.RouteTable
		ec2Err          error
		expectedPrivate []string
		expectError     bool
		errorContains   string
	}{
		{
			name:           "mix of public and private with explicit associations",
			clusterSubnets: []string{"subnet-pub-a", "subnet-pri-a"},
			routeTables: []ec2types.RouteTable{
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-pub-a", false)},
					Routes:       []ec2types.Route{igwRoute()},
				},
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-pri-a", false)},
					Routes:       []ec2types.Route{natRoute()},
				},
			},
			expectedPrivate: []string{"subnet-pri-a"},
		},
		{
			name:           "subnet without explicit association falls back to main RT (private)",
			clusterSubnets: []string{"subnet-impl", "subnet-pub"},
			routeTables: []ec2types.RouteTable{
				{
					Associations: []ec2types.RouteTableAssociation{association("", true)},
					Routes:       []ec2types.Route{natRoute()},
				},
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-pub", false)},
					Routes:       []ec2types.Route{igwRoute()},
				},
			},
			expectedPrivate: []string{"subnet-impl"},
		},
		{
			name:           "subnet without explicit association falls back to main RT (public)",
			clusterSubnets: []string{"subnet-impl", "subnet-pri"},
			routeTables: []ec2types.RouteTable{
				{
					Associations: []ec2types.RouteTableAssociation{association("", true)},
					Routes:       []ec2types.Route{igwRoute()},
				},
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-pri", false)},
					Routes:       []ec2types.Route{natRoute()},
				},
			},
			expectedPrivate: []string{"subnet-pri"},
		},
		{
			name:           "IPv6-only public default route",
			clusterSubnets: []string{"subnet-v6pub", "subnet-v6pri"},
			routeTables: []ec2types.RouteTable{
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-v6pub", false)},
					Routes: []ec2types.Route{{
						DestinationIpv6CidrBlock: aws.String("::/0"),
						GatewayId:                aws.String("igw-0123456789abcdef0"),
						State:                    ec2types.RouteStateActive,
					}},
				},
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-v6pri", false)},
					Routes: []ec2types.Route{{
						DestinationIpv6CidrBlock:    aws.String("::/0"),
						EgressOnlyInternetGatewayId: aws.String("eigw-0123456789abcdef0"),
						State:                       ec2types.RouteStateActive,
					}},
				},
			},
			expectedPrivate: []string{"subnet-v6pri"},
		},
		{
			name:           "blackhole IGW route does not mark subnet public",
			clusterSubnets: []string{"subnet-dead"},
			routeTables: []ec2types.RouteTable{
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-dead", false)},
					Routes: []ec2types.Route{{
						DestinationCidrBlock: aws.String("0.0.0.0/0"),
						GatewayId:            aws.String("igw-0123456789abcdef0"),
						State:                ec2types.RouteStateBlackhole,
					}},
				},
			},
			expectedPrivate: []string{"subnet-dead"},
		},
		{
			name:           "egress-only IGW is not treated as public",
			clusterSubnets: []string{"subnet-eigw"},
			routeTables: []ec2types.RouteTable{
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-eigw", false)},
					Routes: []ec2types.Route{{
						DestinationIpv6CidrBlock:    aws.String("::/0"),
						EgressOnlyInternetGatewayId: aws.String("eigw-0123456789abcdef0"),
						State:                       ec2types.RouteStateActive,
					}},
				},
			},
			expectedPrivate: []string{"subnet-eigw"},
		},
		{
			name:           "no private subnet returns error",
			clusterSubnets: []string{"subnet-pub-a", "subnet-pub-b"},
			routeTables: []ec2types.RouteTable{
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-pub-a", false)},
					Routes:       []ec2types.Route{igwRoute()},
				},
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-pub-b", false)},
					Routes:       []ec2types.Route{igwRoute()},
				},
			},
			expectError:   true,
			errorContains: "no private subnet found",
		},
		{
			name:           "transit gateway route does not mark subnet public",
			clusterSubnets: []string{"subnet-tgw"},
			routeTables: []ec2types.RouteTable{
				{
					Associations: []ec2types.RouteTableAssociation{association("subnet-tgw", false)},
					Routes: []ec2types.Route{{
						DestinationCidrBlock: aws.String("0.0.0.0/0"),
						TransitGatewayId:     aws.String("tgw-0123456789abcdef0"),
						State:                ec2types.RouteStateActive,
					}},
				},
			},
			expectedPrivate: []string{"subnet-tgw"},
		},
		{
			name:          "cluster without VPC configuration",
			cluster:       &ekstypes.Cluster{},
			expectError:   true,
			errorContains: "no VPC configuration",
		},
		{
			name:           "DescribeRouteTables error propagates",
			clusterSubnets: []string{"subnet-a"},
			ec2Err:         errors.New("boom"),
			expectError:    true,
			errorContains:  "failed to describe route tables",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cluster := tc.cluster
			if cluster == nil {
				cluster = &ekstypes.Cluster{
					Name: aws.String("my-cluster"),
					ResourcesVpcConfig: &ekstypes.VpcConfigResponse{
						VpcId:     aws.String(vpcID),
						SubnetIds: tc.clusterSubnets,
					},
				}
			}
			ec2Client := &fakeEC2DescribeRouteTables{routeTables: tc.routeTables, err: tc.ec2Err}

			subnets, err := GetClusterPrivateSubnets(t.Context(), ec2Client, cluster)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedPrivate, subnets)
		})
	}
}
