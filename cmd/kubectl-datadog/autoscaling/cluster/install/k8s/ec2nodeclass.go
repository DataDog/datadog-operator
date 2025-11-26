// Package k8s provides Kubernetes-specific functionality for creating and managing
// Karpenter custom resources including EC2NodeClass and NodePool objects.
package k8s

import (
	"context"

	karpawsv1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
	"github.com/DataDog/datadog-operator/pkg/version"
)

func CreateOrUpdateEC2NodeClass(ctx context.Context, client client.Client, clusterName string, nc guess.EC2NodeClass) error {
	var amiSelectorTerms []karpawsv1.AMISelectorTerm

	if amiIDs := nc.GetAMIIDs(); len(amiIDs) > 0 {
		amiSelectorTerms = lo.Map(amiIDs, func(ami string, _ int) karpawsv1.AMISelectorTerm {
			return karpawsv1.AMISelectorTerm{
				ID: ami,
			}
		})
	} else if alias := amiFamilyToAlias(nc.GetAMIFamily()); alias != "" {
		amiSelectorTerms = []karpawsv1.AMISelectorTerm{
			{
				Alias: alias,
			},
		}
	} else {
		amiSelectorTerms = []karpawsv1.AMISelectorTerm{
			{
				Alias: "al2023@latest",
			},
		}
	}

	return createOrUpdate(ctx, client, &karpawsv1.EC2NodeClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "karpenter.k8s.aws/v1",
			Kind:       "EC2NodeClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: nc.GetName(),
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":      "kubectl-datadog",
				"app.kubernetes.io/version":         version.GetVersion(),
				"autoscaling.datadoghq.com/created": "true",
			},
		},
		Spec: karpawsv1.EC2NodeClassSpec{
			Role:             "KarpenterNodeRole-" + clusterName,
			AMIFamily:        lo.ToPtr(nc.GetAMIFamily()),
			AMISelectorTerms: amiSelectorTerms,
			SubnetSelectorTerms: lo.Map(nc.GetSubnetIDs(), func(subnetID string, _ int) karpawsv1.SubnetSelectorTerm {
				return karpawsv1.SubnetSelectorTerm{
					ID: subnetID,
				}
			}),
			SecurityGroupSelectorTerms: lo.Map(nc.GetSecurityGroupIDs(), func(sgID string, _ int) karpawsv1.SecurityGroupSelectorTerm {
				return karpawsv1.SecurityGroupSelectorTerm{
					ID: sgID,
				}
			}),
			MetadataOptions:     convertMetadataOptions(nc.GetMetadataOptions()),
			BlockDeviceMappings: convertBlockDeviceMappings(nc.GetBlockDeviceMappings()),
		},
	})
}

func amiFamilyToAlias(amiFamily string) string {
	switch amiFamily {
	case "AL2":
		return "al2@latest"
	case "AL2023":
		return "al2023@latest"
	case "Bottlerocket":
		return "bottlerocket@latest"
	case "Windows2019":
		return "windows2019@latest"
	case "Windows2022":
		return "windows2022@latest"
	default:
		return "" // Custom or unknown families don't have aliases
	}
}

func convertMetadataOptions(opts *guess.MetadataOptions) *karpawsv1.MetadataOptions {
	if opts == nil {
		return nil
	}

	return &karpawsv1.MetadataOptions{
		HTTPEndpoint:            opts.HTTPEndpoint,
		HTTPTokens:              opts.HTTPTokens,
		HTTPPutResponseHopLimit: opts.HTTPPutResponseHopLimit,
		HTTPProtocolIPv6:        opts.HTTPProtocolIPv6,
	}
}

func convertBlockDeviceMappings(mappings []guess.BlockDeviceMapping) []*karpawsv1.BlockDeviceMapping {
	if len(mappings) == 0 {
		return nil
	}

	return lo.Map(mappings, func(bdm guess.BlockDeviceMapping, _ int) *karpawsv1.BlockDeviceMapping {
		var volumeSize *resource.Quantity
		if bdm.VolumeSize != nil {
			if quantity, err := resource.ParseQuantity(*bdm.VolumeSize); err == nil {
				volumeSize = &quantity
			}
		}

		return &karpawsv1.BlockDeviceMapping{
			DeviceName: bdm.DeviceName,
			RootVolume: bdm.RootVolume,
			EBS: &karpawsv1.BlockDevice{
				VolumeSize:          volumeSize,
				VolumeType:          bdm.VolumeType,
				IOPS:                bdm.IOPS,
				Throughput:          bdm.Throughput,
				Encrypted:           bdm.Encrypted,
				DeleteOnTermination: bdm.DeleteOnTermination,
				KMSKeyID:            bdm.KMSKeyID,
				SnapshotID:          bdm.SnapshotID,
			},
		}
	})
}
