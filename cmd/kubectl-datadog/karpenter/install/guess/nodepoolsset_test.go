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
					name:             "dd-karpenter-rnm5q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-5sqeq",
					ec2NodeClass: "dd-karpenter-rnm5q",
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
					name:             "dd-karpenter-rnm5q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-abd3o",
					ec2NodeClass: "dd-karpenter-rnm5q",
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
					name:             "dd-karpenter-rnm5q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-5sqeq",
					ec2NodeClass: "dd-karpenter-rnm5q",
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
					name:             "dd-karpenter-rnm5q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-ioaom",
					ec2NodeClass:  "dd-karpenter-rnm5q",
					labels:        map[string]string{},
					capacityTypes: set("on-demand"),
				},
				{
					name:         "dd-karpenter-5sqeq",
					ec2NodeClass: "dd-karpenter-rnm5q",
					labels:       map[string]string{"app": "web"},
					taints: set(taint{
						key:    "app",
						value:  "web",
						effect: corev1.TaintEffectNoSchedule,
					}),
					capacityTypes: set("on-demand"),
				},
				{
					name:         "dd-karpenter-fpuaq",
					ec2NodeClass: "dd-karpenter-rnm5q",
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
					name:             "dd-karpenter-ovpes",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
				},
				{
					name:             "dd-karpenter-32bqc",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-2bvne",
					ec2NodeClass: "dd-karpenter-ovpes",
					labels:       map[string]string{"app": "web"},
					taints: set(taint{
						key:    "app",
						value:  "web",
						effect: corev1.TaintEffectNoSchedule,
					}),
					capacityTypes: set("on-demand"),
				},
				{
					name:         "dd-karpenter-tugly",
					ec2NodeClass: "dd-karpenter-32bqc",
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
					name:             "dd-karpenter-rnm5q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-ioaom",
					ec2NodeClass:  "dd-karpenter-rnm5q",
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
					name:             "dd-karpenter-rnm5q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-abd3o",
					ec2NodeClass:  "dd-karpenter-rnm5q",
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
					name:             "dd-karpenter-rnm5q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-c2sbs",
					ec2NodeClass:  "dd-karpenter-rnm5q",
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
					name:             "dd-karpenter-rnm5q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-2pe4s",
					ec2NodeClass:  "dd-karpenter-rnm5q",
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
					name:             "dd-karpenter-ovpes",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-bd7ck",
					ec2NodeClass:  "dd-karpenter-ovpes",
					labels:        map[string]string{"app": "frontend"},
					architectures: set("amd64"),
					capacityTypes: set("on-demand"),
				},
				{
					name:          "dd-karpenter-fj5wm",
					ec2NodeClass:  "dd-karpenter-ovpes",
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
					name:             "dd-karpenter-rnm5q",
					amiFamily:        "AL2",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:             "dd-karpenter-ntbhw",
					ec2NodeClass:     "dd-karpenter-rnm5q",
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
					name:             "dd-karpenter-cjfk2",
					amiFamily:        "AL2023",
					amiIDs:           set("ami-0a1b2c3d4e5f6g7h8", "ami-9h8g7f6e5d4c3b2a1", "ami-1a2b3c4d5e6f7g8h9"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-kw2x2",
					ec2NodeClass:  "dd-karpenter-cjfk2",
					labels:        map[string]string{"env": "prod"},
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
