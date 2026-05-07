package guess

import (
	"context"
	"fmt"
	"log"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"
)

// awsProviderIDRegexp matches the AWS provider ID format for EC2 instances.
// Format: aws:///ZONE/INSTANCE_ID (e.g., aws:///us-east-1a/i-0abc123def456789)
var awsProviderIDRegexp = regexp.MustCompile(`^aws:///[^/]+/(i-[0-9a-f]+)$`)

// ec2DescribeBatchSize bounds the number of instance IDs we hand to a single
// ec2:DescribeInstances / ec2:DescribeImages call. The K8s pager streams
// nodes individually, so without an explicit cap we'd send every cluster
// node in one EC2 request — risking request-size, throttling, or response
// pagination issues on dense clusters.
const ec2DescribeBatchSize = 100

// pendingNode captures the subset of corev1.Node that processNodeBatch needs.
// Storing only Labels and Taints avoids retaining the pager page's Items
// backing array (which a *corev1.Node would pin through its embedded slice
// header) and avoids copying the whole Node struct.
type pendingNode struct {
	labels map[string]string
	taints []corev1.Taint
}

func GetNodesProperties(ctx context.Context, clientset *kubernetes.Clientset, ec2Client *ec2.Client) (*NodePoolsSet, error) {
	nps := NewNodePoolsSet()

	pending := map[string]pendingNode{}
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if err := processNodeBatch(ctx, ec2Client, nps, pending); err != nil {
			return err
		}
		clear(pending)
		return nil
	}

	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return clientset.CoreV1().Nodes().List(ctx, opts)
	})
	if err := p.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		node := obj.(*corev1.Node)
		if _, isKarpenter := node.Labels["karpenter.k8s.aws/ec2nodeclass"]; isKarpenter {
			return nil
		}
		matches := awsProviderIDRegexp.FindStringSubmatch(node.Spec.ProviderID)
		if len(matches) != 2 {
			log.Printf("Skipping node %s with unexpected provider ID: %s", node.Name, node.Spec.ProviderID)
			return nil
		}
		pending[matches[1]] = pendingNode{labels: node.Labels, taints: node.Spec.Taints}
		if len(pending) >= ec2DescribeBatchSize {
			return flush()
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	if err := flush(); err != nil {
		return nil, err
	}
	return nps, nil
}

func processNodeBatch(ctx context.Context, ec2Client *ec2.Client, nps *NodePoolsSet, instanceToNode map[string]pendingNode) error {
	instances, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: slices.Collect(maps.Keys(instanceToNode)),
	})
	if err != nil {
		return fmt.Errorf("failed to describe instances: %w", err)
	}

	imageIds := lo.Uniq(lo.FlatMap(instances.Reservations, func(reservation ec2types.Reservation, _ int) []string {
		return lo.Map(reservation.Instances, func(instance ec2types.Instance, _ int) string {
			return *instance.ImageId
		})
	}))

	images, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: imageIds,
	})
	if err != nil {
		return fmt.Errorf("failed to describe images: %w", err)
	}
	amiIDsToFamily := lo.Associate(images.Images, func(image ec2types.Image) (string, string) {
		return *image.ImageId, detectAMIFamilyFromImage(*image.Name)
	})

	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			node := instanceToNode[*instance.InstanceId]

			amiFamily := "Custom"
			if family, ok := amiIDsToFamily[*instance.ImageId]; ok {
				amiFamily = family
			}

			blockDeviceMappings, err := extractBlockDeviceMappingsWithVolumeDetails(ctx, ec2Client, instance.BlockDeviceMappings)
			if err != nil {
				log.Printf("Failed to get volume details for instance %s: %v", *instance.InstanceId, err)
				blockDeviceMappings = extractBasicBlockDeviceMappings(instance.BlockDeviceMappings)
			}

			nps.Add(NodePoolsSetAddParams{
				AMIFamily:           amiFamily,
				AMIID:               *instance.ImageId,
				SubnetIDs:           []string{*instance.SubnetId},
				SecurityGroupIDs:    lo.Map(instance.SecurityGroups, func(sg ec2types.GroupIdentifier, _ int) string { return *sg.GroupId }),
				MetadataOptions:     extractMetadataOptions(instance.MetadataOptions),
				BlockDeviceMappings: blockDeviceMappings,
				Labels:              node.labels,
				Taints:              node.taints,
				Architecture:        convertArchitecture(instance.Architecture),
				Zones:               extractZones(instance.Placement),
				InstanceTypes:       []string{string(instance.InstanceType)},
				CapacityType:        convertInstanceLifecycleType(instance.InstanceLifecycle),
			})
		}
	}
	return nil
}

func detectAMIFamilyFromImage(imageName string) string {
	containsAny := func(s string, patterns ...string) bool {
		return slices.ContainsFunc(patterns, func(pattern string) bool {
			return strings.Contains(s, pattern)
		})
	}

	lowerName := strings.ToLower(imageName)

	switch {
	case containsAny(imageName, "amazon-linux-2023", "al2023"):
		return "AL2023"
	case containsAny(imageName, "amazon-linux-2-", "amzn2-ami"):
		return "AL2"
	case strings.Contains(lowerName, "bottlerocket"):
		return "Bottlerocket"
	case strings.Contains(imageName, "Windows_Server-2025"):
		return "Windows2025"
	case strings.Contains(imageName, "Windows_Server-2022"):
		return "Windows2022"
	case strings.Contains(imageName, "Windows_Server-2019"):
		return "Windows2019"
	default:
		return "Custom"
	}
}

func extractMetadataOptions(opts *ec2types.InstanceMetadataOptionsResponse) *MetadataOptions {
	if opts == nil {
		return nil
	}

	var hopLimit *int64
	if opts.HttpPutResponseHopLimit != nil {
		hopLimit = lo.ToPtr(int64(*opts.HttpPutResponseHopLimit))
	}

	return &MetadataOptions{
		HTTPEndpoint:            lo.Ternary(opts.HttpEndpoint != "", lo.ToPtr(string(opts.HttpEndpoint)), nil),
		HTTPTokens:              lo.Ternary(opts.HttpTokens != "", lo.ToPtr(string(opts.HttpTokens)), nil),
		HTTPPutResponseHopLimit: hopLimit,
		HTTPProtocolIPv6:        lo.Ternary(opts.HttpProtocolIpv6 != "", lo.ToPtr(string(opts.HttpProtocolIpv6)), nil),
	}
}

func convertArchitecture(arch ec2types.ArchitectureValues) string {
	switch arch {
	case ec2types.ArchitectureValuesX8664:
		return "amd64"
	case ec2types.ArchitectureValuesArm64:
		return "arm64"
	case ec2types.ArchitectureValuesI386:
		return "386"
	default:
		return ""
	}
}

func extractZones(placement *ec2types.Placement) []string {
	if placement != nil && placement.AvailabilityZone != nil {
		return []string{*placement.AvailabilityZone}
	}
	return []string{}
}

func convertInstanceLifecycleType(ilt ec2types.InstanceLifecycleType) string {
	switch ilt {
	case ec2types.InstanceLifecycleTypeScheduled:
		return "on-demand"
	case ec2types.InstanceLifecycleTypeSpot:
		return "spot"
	case ec2types.InstanceLifecycleTypeCapacityBlock:
		return "reserved"
	default:
		return "on-demand"
	}
}

func extractBlockDeviceMappingsWithVolumeDetails(ctx context.Context, ec2Client *ec2.Client, mappings []ec2types.InstanceBlockDeviceMapping) ([]BlockDeviceMapping, error) {
	if len(mappings) == 0 {
		return nil, nil
	}

	volumeIDs := lo.FilterMap(mappings, func(mapping ec2types.InstanceBlockDeviceMapping, _ int) (string, bool) {
		if mapping.Ebs != nil && mapping.Ebs.VolumeId != nil {
			return *mapping.Ebs.VolumeId, true
		}
		return "", false
	})

	if len(volumeIDs) == 0 {
		return extractBasicBlockDeviceMappings(mappings), nil
	}

	volumesResp, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		VolumeIds: volumeIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe volumes: %w", err)
	}

	volumeDetails := lo.Associate(volumesResp.Volumes, func(vol ec2types.Volume) (string, ec2types.Volume) {
		if vol.VolumeId != nil {
			return *vol.VolumeId, vol
		}
		return "", vol
	})

	return lo.FilterMap(mappings, func(mapping ec2types.InstanceBlockDeviceMapping, _ int) (BlockDeviceMapping, bool) {
		// Skip non-EBS volumes (e.g., instance store volumes)
		if mapping.Ebs == nil || mapping.Ebs.VolumeId == nil {
			return BlockDeviceMapping{}, false
		}

		if vol, ok := volumeDetails[*mapping.Ebs.VolumeId]; ok {
			return BlockDeviceMapping{
				DeviceName:          mapping.DeviceName,
				RootVolume:          isRootDevice(mapping.DeviceName),
				DeleteOnTermination: mapping.Ebs.DeleteOnTermination,
				VolumeSize:          lo.Ternary(vol.Size != nil, lo.ToPtr(fmt.Sprintf("%dGi", lo.FromPtr(vol.Size))), nil),
				VolumeType:          lo.Ternary(vol.VolumeType != "", lo.ToPtr(string(vol.VolumeType)), nil),
				IOPS:                lo.Ternary(vol.Iops != nil, lo.ToPtr(int64(lo.FromPtr(vol.Iops))), nil),
				Throughput:          lo.Ternary(vol.Throughput != nil, lo.ToPtr(int64(lo.FromPtr(vol.Throughput))), nil),
				Encrypted:           vol.Encrypted,
				KMSKeyID:            vol.KmsKeyId,
				SnapshotID:          vol.SnapshotId,
			}, true
		} else {
			return BlockDeviceMapping{
				DeviceName:          mapping.DeviceName,
				RootVolume:          isRootDevice(mapping.DeviceName),
				DeleteOnTermination: mapping.Ebs.DeleteOnTermination,
			}, true
		}
	}), nil
}

func extractBasicBlockDeviceMappings(mappings []ec2types.InstanceBlockDeviceMapping) []BlockDeviceMapping {
	if len(mappings) == 0 {
		return nil
	}

	return lo.FilterMap(mappings, func(mapping ec2types.InstanceBlockDeviceMapping, _ int) (BlockDeviceMapping, bool) {
		// Skip non-EBS volumes (e.g., instance store volumes)
		if mapping.Ebs == nil {
			return BlockDeviceMapping{}, false
		}

		return BlockDeviceMapping{
			DeviceName:          mapping.DeviceName,
			RootVolume:          isRootDevice(mapping.DeviceName),
			DeleteOnTermination: mapping.Ebs.DeleteOnTermination,
			// Note: Other properties like size, type, IOPS, etc. are not available
			// from InstanceBlockDeviceMapping without calling DescribeVolumes
		}, true
	})
}

func isRootDevice(deviceName *string) bool {
	if deviceName == nil {
		return false
	}
	return *deviceName == "/dev/xvda" || // Amazon Linux, Ubuntu on Nitro
		*deviceName == "/dev/sda1" // Older instances, Windows
}
