package guess

import (
	"cmp"
	"encoding/base32"
	"encoding/binary"
	"hash/fnv"
	"maps"
	"slices"
	"strings"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
)

type EC2NodeClass struct {
	Name             string
	AMIIDs           []string
	SubnetIDs        []string
	SecurityGroupIDs []string
}

func (nc *EC2NodeClass) sum64() uint64 {
	h := fnv.New64()

	for _, x := range nc.AMIIDs {
		h.Write([]byte(x))
	}

	for _, x := range nc.SecurityGroupIDs {
		h.Write([]byte(x))
	}

	return h.Sum64()
}

type NodePool struct {
	Name             string
	EC2NodeClass     string
	Labels           map[string]string
	Taints           []corev1.Taint
	Architectures    []string
	Zones            []string
	InstanceFamilies []string
	CapacityTypes    []string
}

func (np *NodePool) sum64() uint64 {
	h := fnv.New64()

	h.Write([]byte(np.EC2NodeClass))

	for _, k := range slices.Sorted(maps.Keys(np.Labels)) {
		h.Write([]byte(k))
		h.Write([]byte(np.Labels[k]))
	}

	for _, taint := range np.Taints {
		h.Write([]byte(taint.Key))
		h.Write([]byte(taint.Value))
		h.Write([]byte(taint.Effect))
	}

	return h.Sum64()
}

type NodePoolsSet struct {
	ec2NodeClasses map[uint64]EC2NodeClass
	nodePools      map[uint64]NodePool
}

func NewNodePoolsSet() *NodePoolsSet {
	return &NodePoolsSet{
		ec2NodeClasses: make(map[uint64]EC2NodeClass),
		nodePools:      make(map[uint64]NodePool),
	}
}

type NodePoolsSetAddParams struct {
	AMIID            string
	SubnetIDs        []string
	SecurityGroupIDs []string
	Labels           map[string]string
	Taints           []corev1.Taint
	Architecture     string
	Zones            []string
	InstanceTypes    []string
	CapacityType     string
}

func (nps *NodePoolsSet) Add(p NodePoolsSetAddParams) {
	nc := EC2NodeClass{
		AMIIDs:           []string{p.AMIID},
		SubnetIDs:        slices.Sorted(slices.Values(p.SubnetIDs)),
		SecurityGroupIDs: slices.Sorted(slices.Values(p.SecurityGroupIDs)),
	}

	h := nc.sum64()

	nc.Name = "dd-karpenter-" + encodeUint64Base32(h)[8:]

	if n, found := nps.ec2NodeClasses[h]; found {
		n.SubnetIDs = slices.Compact(slices.Sorted(slices.Values(append(n.SubnetIDs, p.SubnetIDs...))))
		nps.ec2NodeClasses[h] = n
	} else {
		nps.ec2NodeClasses[h] = nc
	}

	np := NodePool{
		EC2NodeClass:     nc.Name,
		Labels:           sanitizeLabels(p.Labels),
		Taints:           slices.SortedFunc(slices.Values(p.Taints), compareTaints),
		Architectures:    []string{},
		Zones:            slices.Sorted(slices.Values(p.Zones)),
		InstanceFamilies: extractInstanceFamilies(p.InstanceTypes),
		CapacityTypes:    []string{p.CapacityType},
	}

	if p.Architecture != "" {
		np.Architectures = []string{p.Architecture}
	}

	h = np.sum64()

	np.Name = "dd-karpenter-" + encodeUint64Base32(h)[8:]

	if n, found := nps.nodePools[h]; found {
		if p.Architecture != "" {
			n.Architectures = slices.Compact(slices.Sorted(slices.Values(append(n.Architectures, p.Architecture))))
		}
		n.Zones = slices.Compact(slices.Sorted(slices.Values(append(n.Zones, p.Zones...))))
		n.InstanceFamilies = slices.Compact(slices.Sorted(slices.Values(append(n.InstanceFamilies, extractInstanceFamilies(p.InstanceTypes)...))))
		n.CapacityTypes = slices.Compact(slices.Sorted(slices.Values(append(n.CapacityTypes, p.CapacityType))))
		nps.nodePools[h] = n
	} else {
		nps.nodePools[h] = np
	}
}

func (nps *NodePoolsSet) GetEC2NodeClasses() []EC2NodeClass {
	return slices.Collect(maps.Values(nps.ec2NodeClasses))
}

func (nps *NodePoolsSet) GetNodePools() []NodePool {
	return slices.Collect(maps.Values(nps.nodePools))
}

func sanitizeLabels(labels map[string]string) map[string]string {
	return lo.OmitBy(labels, func(key, _ string) bool {
		return strings.HasPrefix(key, "alpha.eksctl.io/") ||
			strings.HasPrefix(key, "beta.kubernetes.io/") ||
			strings.HasPrefix(key, "eks.amazonaws.com/") ||
			strings.HasPrefix(key, "failure-domain.beta.kubernetes.io/") ||
			strings.HasPrefix(key, "k8s.io/") ||
			strings.HasPrefix(key, "kubernetes.io/") ||
			strings.HasPrefix(key, "node.kubernetes.io/") ||
			strings.HasPrefix(key, "topology.k8s.aws/") ||
			strings.HasPrefix(key, "topology.kubernetes.io/")
	})
}

func compareTaints(x, y corev1.Taint) int {
	if c := cmp.Compare(x.Key, y.Key); c != 0 {
		return c
	}
	if c := cmp.Compare(x.Value, y.Value); c != 0 {
		return c
	}
	if c := cmp.Compare(x.Effect, y.Effect); c != 0 {
		return c
	}
	return 0
}

func encodeUint64Base32(n uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
}

func extractInstanceFamilies(instanceTypes []string) []string {
	families := make(map[string]bool)
	for _, instanceType := range instanceTypes {
		// Extract family: "m5.large" -> "m5", "t3.medium" -> "t3"
		if family, _, _ := strings.Cut(instanceType, "."); family != "" {
			families[family] = true
		}
	}
	if len(families) == 0 {
		return nil
	}
	result := make([]string, 0, len(families))
	for family := range families {
		result = append(result, family)
	}
	slices.Sort(result)
	return result
}
