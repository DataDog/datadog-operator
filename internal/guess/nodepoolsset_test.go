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
					Labels:           map[string]string{"foo": "bar"},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
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
					Name:         "dd-karpenter-2zw2a",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{"foo": "bar"},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
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
					Labels:           map[string]string{"foo": "bar"},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					Labels:           map[string]string{"foo": "bar"},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
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
					Name:         "dd-karpenter-2zw2a",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{"foo": "bar"},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
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
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					Labels:           map[string]string{"foo": "foo"},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "foo",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					Labels:           map[string]string{"bar": "bar"},
					Taints: []corev1.Taint{
						{
							Key:    "bar",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
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
					Name:         "dd-karpenter-ocnhm",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{},
				},
				{
					Name:         "dd-karpenter-6ctga",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{"foo": "foo"},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "foo",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
				{
					Name:         "dd-karpenter-pbiti",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{"bar": "bar"},
					Taints: []corev1.Taint{
						{
							Key:    "bar",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
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
					Labels:           map[string]string{"foo": "foo"},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "foo",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-05e10de88ea36557b", "subnet-07aaca522252301b0", "subnet-0e08d6ea64a70ad35"},
					Labels:           map[string]string{"bar": "bar"},
					Taints: []corev1.Taint{
						{
							Key:    "bar",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
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
					Name:         "dd-karpenter-5z6xg",
					EC2NodeClass: "dd-karpenter-ko4kw",
					Labels:       map[string]string{"foo": "foo"},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "foo",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
				{
					Name:         "dd-karpenter-6elwg",
					EC2NodeClass: "dd-karpenter-7jr4o",
					Labels:       map[string]string{"bar": "bar"},
					Taints: []corev1.Taint{
						{
							Key:    "bar",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
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
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-07aaca522252301b0"},
				},
				{
					AMIID:            "ami-0bd48499820cf0df6",
					SecurityGroupIDs: []string{"sg-01dfd3789be8c5315", "sg-0d4942a5188f41a42"},
					SubnetIDs:        []string{"subnet-0e08d6ea64a70ad35"},
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
					Name:         "dd-karpenter-ocnhm",
					EC2NodeClass: "dd-karpenter-bufp4",
					Labels:       map[string]string{},
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
			}

			assert.ElementsMatch(t, tc.expectedEC2NodeClasses, ec2NodeClasses)
			assert.ElementsMatch(t, tc.expectedNodePools, nodePools)
		})
	}
}
