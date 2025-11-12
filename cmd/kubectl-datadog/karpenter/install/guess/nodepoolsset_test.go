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
					name:             "dd-karpenter-bufp4",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-zq7bq",
					ec2NodeClass: "dd-karpenter-bufp4",
					labels:       map[string]string{"app": "web"},
					taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Several identical parameters are merged in a single node pool",
			add: []NodePoolsSetAddParams{
				{
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
					name:             "dd-karpenter-bufp4",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-zq7bq",
					ec2NodeClass: "dd-karpenter-bufp4",
					labels:       map[string]string{"app": "web"},
					taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Different labels and taints lead to different node pools but a single node class",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					CapacityType:     "on-demand",
				},
				{
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
					name:             "dd-karpenter-bufp4",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-ocnhm",
					ec2NodeClass:  "dd-karpenter-bufp4",
					labels:        map[string]string{},
					capacityTypes: set("on-demand"),
				},
				{
					name:         "dd-karpenter-zq7bq",
					ec2NodeClass: "dd-karpenter-bufp4",
					labels:       map[string]string{"app": "web"},
					taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					capacityTypes: set("on-demand"),
				},
				{
					name:         "dd-karpenter-iohjq",
					ec2NodeClass: "dd-karpenter-bufp4",
					labels:       map[string]string{"app": "api"},
					taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "api",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Different security groups lead to different node classes",
			add: []NodePoolsSetAddParams{
				{
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
					name:             "dd-karpenter-ko4kw",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
				},
				{
					name:             "dd-karpenter-7jr4o",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:         "dd-karpenter-dzimw",
					ec2NodeClass: "dd-karpenter-ko4kw",
					labels:       map[string]string{"app": "web"},
					taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					capacityTypes: set("on-demand"),
				},
				{
					name:         "dd-karpenter-7l4b6",
					ec2NodeClass: "dd-karpenter-7jr4o",
					labels:       map[string]string{"app": "api"},
					taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "api",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Subnets are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-0e08d6ea64a70ad35"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					CapacityType:     "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-bufp4",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-ocnhm",
					ec2NodeClass:  "dd-karpenter-bufp4",
					labels:        map[string]string{},
					capacityTypes: set("on-demand"),
				},
			},
		},
		{
			name: "Capacity types are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					CapacityType:     "spot",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "web"},
					CapacityType:     "spot",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					name:             "dd-karpenter-bufp4",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-4suwo",
					ec2NodeClass:  "dd-karpenter-bufp4",
					labels:        map[string]string{"app": "web"},
					capacityTypes: set("on-demand", "spot"),
				},
			},
		},
		{
			name: "Architectures are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "backend"},
					Architecture:     "amd64",
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "backend"},
					Architecture:     "arm64",
					CapacityType:     "on-demand",
				},
				{
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
					name:             "dd-karpenter-bufp4",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-aagos",
					ec2NodeClass:  "dd-karpenter-bufp4",
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
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "multi-zone"},
					Architecture:     "amd64",
					Zones:            []string{"us-east-1a"},
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "multi-zone"},
					Architecture:     "amd64",
					Zones:            []string{"us-east-1b"},
					CapacityType:     "on-demand",
				},
				{
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
					name:             "dd-karpenter-bufp4",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-y5hvs",
					ec2NodeClass:  "dd-karpenter-bufp4",
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
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					Labels:           map[string]string{"app": "frontend"},
					Architecture:     "amd64",
					CapacityType:     "on-demand",
				},
				{
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
					name:             "dd-karpenter-ko4kw",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b"),
					securityGroupIDs: set("sg-01dfd3789be8c5315"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:          "dd-karpenter-mx734",
					ec2NodeClass:  "dd-karpenter-ko4kw",
					labels:        map[string]string{"app": "frontend"},
					architectures: set("amd64"),
					capacityTypes: set("on-demand"),
				},
				{
					name:          "dd-karpenter-rjbsc",
					ec2NodeClass:  "dd-karpenter-ko4kw",
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
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "mixed-instances"},
					Architecture:     "amd64",
					InstanceTypes:    []string{"m5.large", "m5.xlarge"},
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					Labels:           map[string]string{"app": "mixed-instances"},
					Architecture:     "amd64",
					InstanceTypes:    []string{"m5.2xlarge", "t3.medium"},
					CapacityType:     "on-demand",
				},
				{
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
					name:             "dd-karpenter-bufp4",
					amiIDs:           set("ami-0bd48499820cf0df6"),
					subnetIDs:        set("subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"),
					securityGroupIDs: set("sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"),
				},
			},
			expectedNodePools: []NodePool{
				{
					name:             "dd-karpenter-ieiqw",
					ec2NodeClass:     "dd-karpenter-bufp4",
					labels:           map[string]string{"app": "mixed-instances"},
					architectures:    set("amd64"),
					instanceFamilies: set("c5", "m5", "t3"),
					capacityTypes:    set("on-demand"),
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
				assert.True(t, slices.IsSortedFunc(nodePool.taints, compareTaints))
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
		expected      []string
	}{
		{
			name:          "empty list",
			instanceTypes: []string{},
			expected:      nil,
		},
		{
			name:          "single instance type",
			instanceTypes: []string{"m5.large"},
			expected:      []string{"m5"},
		},
		{
			name:          "multiple instances of same family",
			instanceTypes: []string{"m5.large", "m5.xlarge", "m5.2xlarge"},
			expected:      []string{"m5"},
		},
		{
			name:          "mixed families",
			instanceTypes: []string{"m5.large", "t3.medium", "c5.xlarge", "t3.large"},
			expected:      []string{"c5", "m5", "t3"},
		},
		{
			name:          "with duplicates",
			instanceTypes: []string{"m5.large", "m5.large", "t3.medium", "t3.medium"},
			expected:      []string{"m5", "t3"},
		},
		{
			name:          "GPU and Graviton instances",
			instanceTypes: []string{"p3.2xlarge", "g4dn.xlarge", "t4g.micro", "m6g.medium"},
			expected:      []string{"g4dn", "m6g", "p3", "t4g"},
		},
		{
			name:          "with empty strings",
			instanceTypes: []string{"", "m5.large", "", "t3.medium"},
			expected:      []string{"m5", "t3"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := extractInstanceFamilies(tc.instanceTypes)
			assert.Equal(t, tc.expected, result)
		})
	}
}
