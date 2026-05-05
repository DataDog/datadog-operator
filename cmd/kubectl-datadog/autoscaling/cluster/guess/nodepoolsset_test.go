package guess

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func set[T comparable](vals ...T) map[T]struct{} {
	return lo.Keyify(vals)
}

func blockDeviceMappingsMap(bdms ...BlockDeviceMapping) map[string]*BlockDeviceMapping {
	return lo.Associate(bdms, func(bdm BlockDeviceMapping) (string, *BlockDeviceMapping) {
		return lo.FromPtr(bdm.DeviceName), lo.ToPtr(bdm)
	})
}

func applyEC2NodeClassDefaults(ec2NodeClasses []EC2NodeClass) []EC2NodeClass {
	return lo.Map(ec2NodeClasses, func(nc EC2NodeClass, _ int) EC2NodeClass {
		if nc.amiIDs == nil {
			nc.amiIDs = make(map[string]struct{})
		}
		if nc.subnetIDs == nil {
			nc.subnetIDs = make(map[string]struct{})
		}
		if nc.securityGroupIDs == nil {
			nc.securityGroupIDs = make(map[string]struct{})
		}
		if nc.blockDeviceMappings == nil {
			nc.blockDeviceMappings = make(map[string]*BlockDeviceMapping)
		}
		return nc
	})
}

func applyNodePoolDefaults(nodePools []NodePool) []NodePool {
	return lo.Map(nodePools, func(np NodePool, _ int) NodePool {
		if np.taints == nil {
			np.taints = make(map[taint]struct{})
		}
		if np.architectures == nil {
			np.architectures = make(map[string]struct{})
		}
		if np.zones == nil {
			np.zones = make(map[string]struct{})
		}
		if np.instanceFamilies == nil {
			np.instanceFamilies = make(map[string]struct{})
		}
		if np.capacityTypes == nil {
			np.capacityTypes = make(map[string]struct{})
		}
		return np
	})
}

func TestNodePoolsSet(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		add                    []NodePoolsSetAddParams
		expectedEC2NodeClasses []EC2NodeClass
		expectedNodePools      []NodePool
	}{
		{
			name: "Single node or node group",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-obxiu",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-wtyrq",
					ec2NodeClass: "dd-karpenter-obxiu",
					labels:       map[string]string{"app": "web"},
					taints: set(taint{
						key:    "app",
						value:  "web",
						effect: corev1.TaintEffectNoSchedule,
					}),
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Kubernetes scoped labels are properly filtered out",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels: map[string]string{
						"app":                                           "web",
						"alpha.eksctl.io/cluster-name":                  "lenaic-karpenter-test",
						"alpha.eksctl.io/nodegroup-name":                "ng",
						"beta.kubernetes.io/arch":                       "arm64",
						"beta.kubernetes.io/instance-type":              "t4g.large",
						"beta.kubernetes.io/os":                         "linux",
						"eks.amazonaws.com/capacityType":                "ON_DEMAND",
						"eks.amazonaws.com/nodegroup":                   "ng",
						"eks.amazonaws.com/nodegroup-image":             "ami-0ff91f79e4fd0a745",
						"eks.amazonaws.com/sourceLaunchTemplateId":      "lt-031142f896fbf6764",
						"eks.amazonaws.com/sourceLaunchTemplateVersion": "1",
						"failure-domain.beta.kubernetes.io/region":      "eu-west-3",
						"failure-domain.beta.kubernetes.io/zone":        "eu-west-3b",
						"k8s.io/cloud-provider-aws":                     "b4d26d758837407c7f287bf292ecf7ed",
						"kubernetes.io/arch":                            "arm64",
						"kubernetes.io/hostname":                        "ip-10-11-233-112.eu-west-3.compute.internal",
						"kubernetes.io/os":                              "linux",
						"node.kubernetes.io/instance-type":              "t4g.large",
						"topology.k8s.aws/zone-id":                      "euw3-az2",
						"topology.kubernetes.io/region":                 "eu-west-3",
						"topology.kubernetes.io/zone":                   "eu-west-3b",
					},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-obxiu",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-bcogm",
					ec2NodeClass: "dd-karpenter-obxiu",
					labels: map[string]string{
						"app": "web",
					},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Several identical parameters are merged in a single node pool",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityType: "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-obxiu",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-wtyrq",
					ec2NodeClass: "dd-karpenter-obxiu",
					labels:       map[string]string{"app": "web"},
					taints: set(taint{
						key:    "app",
						value:  "web",
						effect: corev1.TaintEffectNoSchedule,
					}),
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Different labels and taints lead to different node pools but a single node class",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityType: "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "api"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "api",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-obxiu",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-dpwb4",
					ec2NodeClass:  "dd-karpenter-obxiu",
					labels:        map[string]string{},
					capacityTypes: set("on-demand"),
				},
				{
					name:         "dd-karpenter-wtyrq",
					ec2NodeClass: "dd-karpenter-obxiu",
					labels:       map[string]string{"app": "web"},
					taints: set(taint{
						key:    "app",
						value:  "web",
						effect: corev1.TaintEffectNoSchedule,
					}),
					capacityTypes: set("on-demand"),
				},
				{
					name:         "dd-karpenter-dvkxy",
					ec2NodeClass: "dd-karpenter-obxiu",
					labels:       map[string]string{"app": "api"},
					taints: set(taint{
						key:    "app",
						value:  "api",
						effect: corev1.TaintEffectNoSchedule,
					}),
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Different security groups lead to different node classes",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					Labels:           map[string]string{"app": "web"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityType: "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "api"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "api",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-plw4q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
				},
				{
					name:             "dd-karpenter-r4oia",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-b6be2",
					ec2NodeClass: "dd-karpenter-plw4q",
					labels:       map[string]string{"app": "web"},
					taints: set(taint{
						key:    "app",
						value:  "web",
						effect: corev1.TaintEffectNoSchedule,
					}),
					capacityTypes: set("on-demand"),
				},
				{
					name:         "dd-karpenter-3s4qq",
					ec2NodeClass: "dd-karpenter-r4oia",
					labels:       map[string]string{"app": "api"},
					taints: set(taint{
						key:    "app",
						value:  "api",
						effect: corev1.TaintEffectNoSchedule,
					}),
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Subnets are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					CapacityType:     "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-obxiu",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-dpwb4",
					ec2NodeClass:  "dd-karpenter-obxiu",
					labels:        map[string]string{},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Capacity types are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					CapacityType:     "spot",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					CapacityType:     "spot",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-obxiu",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-bcogm",
					ec2NodeClass:  "dd-karpenter-obxiu",
					labels:        map[string]string{"app": "web"},
					capacityTypes: set("on-demand", "spot"),
				},
			},
		},
		{
			name: "Architectures are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "backend"},
					Architecture:     "amd64",
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "backend"},
					Architecture:     "arm64",
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "backend"},
					Architecture:     "amd64",
					CapacityType:     "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-obxiu",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-dcvbi",
					ec2NodeClass:  "dd-karpenter-obxiu",
					labels:        map[string]string{"app": "backend"},
					architectures: set("amd64", "arm64"),
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Zones are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "multi-zone"},
					Architecture:     "amd64",
					Zones:            []string{"us-east-1a"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "multi-zone"},
					Architecture:     "amd64",
					Zones:            []string{"us-east-1b"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "multi-zone"},
					Architecture:     "amd64",
					Zones:            []string{"us-east-1c"},
					CapacityType:     "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-obxiu",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-2t4om",
					ec2NodeClass:  "dd-karpenter-obxiu",
					labels:        map[string]string{"app": "multi-zone"},
					architectures: set("amd64"),
					zones:         set("us-east-1a", "us-east-1b", "us-east-1c"),
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Different architectures with different labels create separate pools",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					Labels:           map[string]string{"app": "frontend"},
					Architecture:     "amd64",
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					Labels:           map[string]string{"app": "ml"},
					Architecture:     "arm64",
					CapacityType:     "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-plw4q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-t3bag",
					ec2NodeClass:  "dd-karpenter-plw4q",
					labels:        map[string]string{"app": "frontend"},
					architectures: set("amd64"),
					capacityTypes: set("on-demand"),
				},
				{
					name:          "dd-karpenter-xzhca",
					ec2NodeClass:  "dd-karpenter-plw4q",
					labels:        map[string]string{"app": "ml"},
					architectures: set("arm64"),
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Instance families are extracted and merged",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "mixed-instances"},
					Architecture:     "amd64",
					InstanceTypes:    []string{"m5.large", "m5.xlarge"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "mixed-instances"},
					Architecture:     "amd64",
					InstanceTypes:    []string{"m5.2xlarge", "t3.medium"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2",
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "mixed-instances"},
					Architecture:     "amd64",
					InstanceTypes:    []string{"t3.large", "c5.xlarge"},
					CapacityType:     "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-obxiu",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:             "dd-karpenter-e5y34",
					ec2NodeClass:     "dd-karpenter-obxiu",
					labels:           map[string]string{"app": "mixed-instances"},
					architectures:    set("amd64"),
					instanceFamilies: set("c5", "m5", "t3"),
					capacityTypes:    set("on-demand"),
				},
			},
		},
		{
			name: "Different AMI IDs with same AMI family merge into single EC2NodeClass",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"env": "prod"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-9h8g7f6e5d4c3b2a1",
					SubnetIDs:        []string{"subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"env": "prod"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-1a2b3c4d5e6f7g8h9",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"env": "prod"},
					CapacityType:     "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-vslxc",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8", "ami-9h8g7f6e5d4c3b2a1", "ami-1a2b3c4d5e6f7g8h9"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-rkiwm",
					ec2NodeClass:  "dd-karpenter-vslxc",
					labels:        map[string]string{"env": "prod"},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Different MetadataOptions lead to different EC2NodeClasses",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					MetadataOptions: &MetadataOptions{
						HTTPEndpoint: lo.ToPtr("enabled"),
						HTTPTokens:   lo.ToPtr("required"),
					},
					Labels:       map[string]string{"env": "dev"},
					CapacityType: "on-demand",
				},
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					MetadataOptions: &MetadataOptions{
						HTTPEndpoint: lo.ToPtr("enabled"),
						HTTPTokens:   lo.ToPtr("optional"),
					},
					Labels:       map[string]string{"env": "dev"},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-x3ium",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					metadataOptions: &MetadataOptions{
						HTTPEndpoint: lo.ToPtr("enabled"),
						HTTPTokens:   lo.ToPtr("required"),
					},
				},
				{
					name:             "dd-karpenter-oxuyk",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					metadataOptions: &MetadataOptions{
						HTTPEndpoint: lo.ToPtr("enabled"),
						HTTPTokens:   lo.ToPtr("optional"),
					},
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-mjjsy",
					ec2NodeClass:  "dd-karpenter-x3ium",
					labels:        map[string]string{"env": "dev"},
					capacityTypes: set("on-demand"),
				},
				{
					name:          "dd-karpenter-pkvlq",
					ec2NodeClass:  "dd-karpenter-oxuyk",
					labels:        map[string]string{"env": "dev"},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Same MetadataOptions merge into single EC2NodeClass",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					MetadataOptions: &MetadataOptions{
						HTTPEndpoint:            lo.ToPtr("enabled"),
						HTTPTokens:              lo.ToPtr("required"),
						HTTPPutResponseHopLimit: lo.ToPtr(int64(1)),
					},
					Labels:       map[string]string{"env": "prod"},
					CapacityType: "on-demand",
				},
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-9h8g7f6e5d4c3b2a1",
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					MetadataOptions: &MetadataOptions{
						HTTPEndpoint:            lo.ToPtr("enabled"),
						HTTPTokens:              lo.ToPtr("required"),
						HTTPPutResponseHopLimit: lo.ToPtr(int64(1)),
					},
					Labels:       map[string]string{"env": "staging"},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-6sg5o",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8", "ami-9h8g7f6e5d4c3b2a1"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					metadataOptions: &MetadataOptions{
						HTTPEndpoint:            lo.ToPtr("enabled"),
						HTTPTokens:              lo.ToPtr("required"),
						HTTPPutResponseHopLimit: lo.ToPtr(int64(1)),
					},
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-kkile",
					ec2NodeClass:  "dd-karpenter-6sg5o",
					labels:        map[string]string{"env": "prod"},
					capacityTypes: set("on-demand"),
				},
				{
					name:          "dd-karpenter-va2ui",
					ec2NodeClass:  "dd-karpenter-6sg5o",
					labels:        map[string]string{"env": "staging"},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Nil MetadataOptions vs empty MetadataOptions treated differently",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					MetadataOptions:  nil,
					Labels:           map[string]string{"env": "test"},
					CapacityType:     "on-demand",
				},
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-9h8g7f6e5d4c3b2a1",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					MetadataOptions:  &MetadataOptions{},
					Labels:           map[string]string{"env": "test"},
					CapacityType:     "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-qew66",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					metadataOptions:  nil,
				},
				{
					name:             "dd-karpenter-ider4",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-9h8g7f6e5d4c3b2a1"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					metadataOptions:  &MetadataOptions{},
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-erfd4",
					ec2NodeClass:  "dd-karpenter-qew66",
					labels:        map[string]string{"env": "test"},
					capacityTypes: set("on-demand"),
				},
				{
					name:          "dd-karpenter-2hyyo",
					ec2NodeClass:  "dd-karpenter-ider4",
					labels:        map[string]string{"env": "test"},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "MetadataOptions with all fields populated",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					MetadataOptions: &MetadataOptions{
						HTTPEndpoint:            lo.ToPtr("enabled"),
						HTTPTokens:              lo.ToPtr("required"),
						HTTPPutResponseHopLimit: lo.ToPtr(int64(2)),
						HTTPProtocolIPv6:        lo.ToPtr("enabled"),
					},
					Labels:       map[string]string{"app": "api"},
					CapacityType: "spot",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-jeth6",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					metadataOptions: &MetadataOptions{
						HTTPEndpoint:            lo.ToPtr("enabled"),
						HTTPTokens:              lo.ToPtr("required"),
						HTTPPutResponseHopLimit: lo.ToPtr(int64(2)),
						HTTPProtocolIPv6:        lo.ToPtr("enabled"),
					},
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-5taay",
					ec2NodeClass:  "dd-karpenter-jeth6",
					labels:        map[string]string{"app": "api"},
					capacityTypes: set("spot"),
				},
			},
		},
		{
			name: "Single BlockDeviceMapping creates EC2NodeClass",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					BlockDeviceMappings: []BlockDeviceMapping{
						{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("100Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							DeleteOnTermination: lo.ToPtr(true),
						},
					},
					Labels:       map[string]string{"env": "prod"},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-ffvck",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					blockDeviceMappings: blockDeviceMappingsMap(
						BlockDeviceMapping{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("100Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							DeleteOnTermination: lo.ToPtr(true),
						},
					),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-uk3ie",
					ec2NodeClass:  "dd-karpenter-ffvck",
					labels:        map[string]string{"env": "prod"},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Different BlockDeviceMappings create different EC2NodeClasses",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					BlockDeviceMappings: []BlockDeviceMapping{
						{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("100Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							DeleteOnTermination: lo.ToPtr(true),
						},
					},
					Labels:       map[string]string{"app": "web"},
					CapacityType: "on-demand",
				},
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					BlockDeviceMappings: []BlockDeviceMapping{
						{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("200Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							DeleteOnTermination: lo.ToPtr(true),
						},
					},
					Labels:       map[string]string{"app": "db"},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-ffvck",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					blockDeviceMappings: blockDeviceMappingsMap(
						BlockDeviceMapping{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("100Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							DeleteOnTermination: lo.ToPtr(true),
						},
					),
				},
				{
					name:             "dd-karpenter-yoqeq",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					blockDeviceMappings: blockDeviceMappingsMap(
						BlockDeviceMapping{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("200Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							DeleteOnTermination: lo.ToPtr(true),
						},
					),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-3zifc",
					ec2NodeClass:  "dd-karpenter-ffvck",
					labels:        map[string]string{"app": "web"},
					capacityTypes: set("on-demand"),
				},
				{
					name:          "dd-karpenter-6y3r4",
					ec2NodeClass:  "dd-karpenter-yoqeq",
					labels:        map[string]string{"app": "db"},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "BlockDeviceMappings with full properties",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					BlockDeviceMappings: []BlockDeviceMapping{
						{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("100Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							IOPS:                lo.ToPtr(int64(3000)),
							Throughput:          lo.ToPtr(int64(125)),
							Encrypted:           lo.ToPtr(true),
							DeleteOnTermination: lo.ToPtr(true),
							KMSKeyID:            lo.ToPtr("arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"),
						},
						{
							DeviceName:          lo.ToPtr("/dev/xvdb"),
							RootVolume:          false,
							VolumeSize:          lo.ToPtr("500Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							IOPS:                lo.ToPtr(int64(10000)),
							Throughput:          lo.ToPtr(int64(250)),
							Encrypted:           lo.ToPtr(true),
							DeleteOnTermination: lo.ToPtr(false),
						},
					},
					Labels:       map[string]string{"app": "database"},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-lqvvm",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					blockDeviceMappings: blockDeviceMappingsMap(
						BlockDeviceMapping{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("100Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							IOPS:                lo.ToPtr(int64(3000)),
							Throughput:          lo.ToPtr(int64(125)),
							Encrypted:           lo.ToPtr(true),
							DeleteOnTermination: lo.ToPtr(true),
							KMSKeyID:            lo.ToPtr("arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"),
						},
						BlockDeviceMapping{
							DeviceName:          lo.ToPtr("/dev/xvdb"),
							RootVolume:          false,
							VolumeSize:          lo.ToPtr("500Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							IOPS:                lo.ToPtr(int64(10000)),
							Throughput:          lo.ToPtr(int64(250)),
							Encrypted:           lo.ToPtr(true),
							DeleteOnTermination: lo.ToPtr(false),
						},
					),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-smtfa",
					ec2NodeClass:  "dd-karpenter-lqvvm",
					labels:        map[string]string{"app": "database"},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "BlockDeviceMappings with snapshot",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:        "AL2023",
					AMIID:            "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					BlockDeviceMappings: []BlockDeviceMapping{
						{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("100Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							SnapshotID:          lo.ToPtr("snap-1234567890abcdef0"),
							DeleteOnTermination: lo.ToPtr(true),
						},
					},
					Labels:       map[string]string{"env": "restore"},
					CapacityType: "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-b5mqq",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
					blockDeviceMappings: blockDeviceMappingsMap(
						BlockDeviceMapping{
							DeviceName:          lo.ToPtr("/dev/xvda"),
							RootVolume:          true,
							VolumeSize:          lo.ToPtr("100Gi"),
							VolumeType:          lo.ToPtr("gp3"),
							SnapshotID:          lo.ToPtr("snap-1234567890abcdef0"),
							DeleteOnTermination: lo.ToPtr(true),
						},
					),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-jgajg",
					ec2NodeClass:  "dd-karpenter-b5mqq",
					labels:        map[string]string{"env": "restore"},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Empty BlockDeviceMappings vs nil BlockDeviceMappings treated the same",
			add: []NodePoolsSetAddParams{
				{
					AMIFamily:           "AL2023",
					AMIID:               "ami-0a1b2c3d4e5f6g7h8",
					SubnetIDs:           []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs:    []string{"sg-01dfd3789be8c5315"},
					BlockDeviceMappings: nil,
					Labels:              map[string]string{"env": "test"},
					CapacityType:        "on-demand",
				},
				{
					AMIFamily:           "AL2023",
					AMIID:               "ami-9h8g7f6e5d4c3b2a1",
					SubnetIDs:           []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs:    []string{"sg-01dfd3789be8c5315"},
					BlockDeviceMappings: []BlockDeviceMapping{},
					Labels:              map[string]string{"env": "test"},
					CapacityType:        "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:                "dd-karpenter-qew66",
					amiFamily:           "AL2023",
					amiIDs:              set("ami-0a1b2c3d4e5f6g7h8", "ami-9h8g7f6e5d4c3b2a1"),
					subnetIDs:           set("subnet-05e10de88ea36557b"),
					securityGroupIDs:    set("sg-01dfd3789be8c5315"),
					blockDeviceMappings: blockDeviceMappingsMap(),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-erfd4",
					ec2NodeClass:  "dd-karpenter-qew66",
					labels:        map[string]string{"env": "test"},
					capacityTypes: set("on-demand"),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rand.Shuffle(len(tc.add), func(i, j int) {
				tc.add[i], tc.add[j] = tc.add[j], tc.add[i]
			})

			nps := NewNodePoolsSet()

			for _, a := range tc.add {
				nps.Add(a)
			}

			ec2NodeClasses := nps.GetEC2NodeClasses()
			nodePools := nps.GetNodePools()

			for _, ec2NodeClass := range ec2NodeClasses {
				assert.True(t, slices.IsSorted(ec2NodeClass.GetSubnetIDs()))
				assert.True(t, slices.IsSorted(ec2NodeClass.GetSecurityGroupIDs()))
			}

			for _, nodePool := range nodePools {
				assert.True(t, slices.IsSortedFunc(lo.Map(nodePool.GetTaints(), toTaint), compareTaints))
				assert.True(t, slices.IsSorted(nodePool.GetArchitectures()))
				assert.True(t, slices.IsSorted(nodePool.GetZones()))
				assert.True(t, slices.IsSorted(nodePool.GetInstanceFamilies()))
				assert.True(t, slices.IsSorted(nodePool.GetCapacityTypes()))
			}

			tc.expectedEC2NodeClasses = applyEC2NodeClassDefaults(tc.expectedEC2NodeClasses)
			tc.expectedNodePools = applyNodePoolDefaults(tc.expectedNodePools)

			assert.ElementsMatch(t, tc.expectedEC2NodeClasses, ec2NodeClasses)
			assert.ElementsMatch(t, tc.expectedNodePools, nodePools)
		})
	}
}

func TestExtractInstanceFamilies(t *testing.T) {
	for _, tc := range []struct {
		name          string
		instanceTypes []string
		expected      map[string]struct{}
	}{
		{
			name:          "empty list",
			instanceTypes: []string{},
			expected:      set[string](),
		},
		{
			name:          "single instance type",
			instanceTypes: []string{"m5.large"},
			expected:      set("m5"),
		},
		{
			name:          "multiple instances of same family",
			instanceTypes: []string{"m5.large", "m5.xlarge", "m5.2xlarge"},
			expected:      set("m5"),
		},
		{
			name:          "mixed families",
			instanceTypes: []string{"m5.large", "t3.medium", "c5.xlarge", "t3.large"},
			expected:      set("c5", "m5", "t3"),
		},
		{
			name:          "with duplicates",
			instanceTypes: []string{"m5.large", "m5.large", "t3.medium", "t3.medium"},
			expected:      set("m5", "t3"),
		},
		{
			name:          "GPU and Graviton instances",
			instanceTypes: []string{"p3.2xlarge", "g4dn.xlarge", "t4g.micro", "m6g.medium"},
			expected:      set("g4dn", "m6g", "p3", "t4g"),
		},
		{
			name:          "with empty strings",
			instanceTypes: []string{"", "m5.large", "", "t3.medium"},
			expected:      set("m5", "t3"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := extractInstanceFamilies(tc.instanceTypes)
			assert.Equal(t, tc.expected, result)
		})
	}
}
