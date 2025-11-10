package guess

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

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
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
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
					Name:             "dd-karpenter-bufp4",
					AMIIDs:           []string{"ami-0bd48499820cf0df6"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
				},
			},
			expectedNodePools: []NodePool{
				{
					Name:         "dd-karpenter-zq7bq",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{"app": "web"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{},
				},
			},
		},
		{
			name: "Several identical parameters are merged in a single node pool",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
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
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
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
					Name:             "dd-karpenter-bufp4",
					AMIIDs:           []string{"ami-0bd48499820cf0df6"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
				},
			},
			expectedNodePools: []NodePool{
				{
					Name:         "dd-karpenter-zq7bq",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{"app": "web"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{},
				},
			},
		},
		{
			name: "Different labels and taints lead to different node pools but a single node class",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
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
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
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
					Name:             "dd-karpenter-bufp4",
					AMIIDs:           []string{"ami-0bd48499820cf0df6"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
				},
			},
			expectedNodePools: []NodePool{
				{
					Name:          "dd-karpenter-ocnhm",
					EC2NodeClass:  "dd-karpenter-bufp4",
					Labels:        map[string]string{},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{},
				},
				{
					Name:         "dd-karpenter-zq7bq",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{"app": "web"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{},
				},
				{
					Name:         "dd-karpenter-iohjq",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{"app": "api"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "api",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{},
				},
			},
		},
		{
			name: "Different security groups lead to different node classes",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
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
					SecurityGroupIDs: []string{"sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
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
					Name:             "dd-karpenter-ko4kw",
					AMIIDs:           []string{"ami-0bd48499820cf0df6"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
				},
				{
					Name:             "dd-karpenter-7jr4o",
					AMIIDs:           []string{"ami-0bd48499820cf0df6"},
					SecurityGroupIDs: []string{"sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
				},
			},
			expectedNodePools: []NodePool{
				{
					Name:         "dd-karpenter-dzimw",
					EC2NodeClass: "dd-karpenter-ko4kw",
					Labels:       map[string]string{"app": "web"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "web",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{},
				},
				{
					Name:         "dd-karpenter-7l4b6",
					EC2NodeClass: "dd-karpenter-7jr4o",
					Labels:       map[string]string{"app": "api"},
					Taints: []corev1.Taint{
						{
							Key:    "app",
							Value:  "api",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{},
				},
			},
		},
		{
			name: "Subnets are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-0e08d6ea64a70ad35"},
					CapacityType:     "on-demand",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					Name:             "dd-karpenter-bufp4",
					AMIIDs:           []string{"ami-0bd48499820cf0df6"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
				},
			},
			expectedNodePools: []NodePool{
				{
					Name:          "dd-karpenter-ocnhm",
					EC2NodeClass:  "dd-karpenter-bufp4",
					Labels:        map[string]string{},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{},
				},
			},
		},
		{
			name: "Capacity types are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					Labels:           map[string]string{"app": "web"},
					CapacityType:     "spot",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					Labels:           map[string]string{"app": "web"},
					CapacityType:     "on-demand",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					Labels:           map[string]string{"app": "web"},
					CapacityType:     "spot",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					Name:             "dd-karpenter-bufp4",
					AMIIDs:           []string{"ami-0bd48499820cf0df6"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
				},
			},
			expectedNodePools: []NodePool{
				{
					Name:          "dd-karpenter-4suwo",
					EC2NodeClass:  "dd-karpenter-bufp4",
					Labels:        map[string]string{"app": "web"},
					CapacityTypes: []string{"on-demand", "spot"},
					Architectures: []string{},
				},
			},
		},
		{
			name: "Architectures are merged",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					Labels:           map[string]string{"app": "backend"},
					CapacityType:     "on-demand",
					Architecture:     "amd64",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
					Labels:           map[string]string{"app": "backend"},
					CapacityType:     "on-demand",
					Architecture:     "arm64",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-0e08d6ea64a70ad35"},
					Labels:           map[string]string{"app": "backend"},
					CapacityType:     "on-demand",
					Architecture:     "amd64",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					Name:             "dd-karpenter-bufp4",
					AMIIDs:           []string{"ami-0bd48499820cf0df6"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
				},
			},
			expectedNodePools: []NodePool{
				{
					Name:          "dd-karpenter-aagos",
					EC2NodeClass:  "dd-karpenter-bufp4",
					Labels:        map[string]string{"app": "backend"},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{"amd64", "arm64"},
				},
			},
		},
		{
			name: "Different architectures with different labels create separate pools",
			add: []NodePoolsSetAddParams{
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					Labels:           map[string]string{"app": "frontend"},
					CapacityType:     "on-demand",
					Architecture:     "amd64",
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
					Labels:           map[string]string{"app": "ml"},
					CapacityType:     "on-demand",
					Architecture:     "arm64",
				},
			},
			expectedEC2NodeClasses: []EC2NodeClass{
				{
					Name:             "dd-karpenter-ko4kw",
					AMIIDs:           []string{"ami-0bd48499820cf0df6"},
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b"},
				},
			},
			expectedNodePools: []NodePool{
				{
					Name:          "dd-karpenter-mx734",
					EC2NodeClass:  "dd-karpenter-ko4kw",
					Labels:        map[string]string{"app": "frontend"},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{"amd64"},
				},
				{
					Name:          "dd-karpenter-rjbsc",
					EC2NodeClass:  "dd-karpenter-ko4kw",
					Labels:        map[string]string{"app": "ml"},
					CapacityTypes: []string{"on-demand"},
					Architectures: []string{"arm64"},
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
				assert.True(t, slices.IsSorted(ec2NodeClass.SecurityGroupIDs))
				assert.True(t, slices.IsSorted(ec2NodeClass.SubnetIDs))
			}

			for _, nodePool := range nodePools {
				assert.True(t, slices.IsSortedFunc(nodePool.Taints, compareTaints))
				assert.True(t, slices.IsSorted(nodePool.CapacityTypes))
				assert.True(t, slices.IsSorted(nodePool.Architectures))
			}

			assert.ElementsMatch(t, tc.expectedEC2NodeClasses, ec2NodeClasses)
			assert.ElementsMatch(t, tc.expectedNodePools, nodePools)
		})
	}
}
