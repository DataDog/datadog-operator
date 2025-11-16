// Package guess provides logic for inferring Karpenter NodePool and EC2NodeClass
// configurations from existing EKS cluster resources, including node groups and
// running nodes.
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

type taint struct {
	key    string
	value  string
	effect corev1.TaintEffect
}

func toTaint(t corev1.Taint, _ int) taint {
	return taint{
		key:    t.Key,
		value:  t.Value,
		effect: t.Effect,
	}
}

func toCoreTaint(t taint, _ int) corev1.Taint {
	return corev1.Taint{
		Key:    t.key,
		Value:  t.value,
		Effect: t.effect,
	}
}

func compareTaints(x, y taint) int {
	if c := cmp.Compare(x.key, y.key); c != 0 {
		return c
	}
	if c := cmp.Compare(x.value, y.value); c != 0 {
		return c
	}
	if c := cmp.Compare(x.effect, y.effect); c != 0 {
		return c
	}
	return 0
}

type EC2NodeClass struct {
	name             string
	amiFamily        string
	amiIDs           map[string]struct{}
	subnetIDs        map[string]struct{}
	securityGroupIDs map[string]struct{}
}

func (nc *EC2NodeClass) GetName() string {
	return nc.name
}

func (nc *EC2NodeClass) GetAMIFamily() string {
	return nc.amiFamily
}

func (nc *EC2NodeClass) GetAMIIDs() []string {
	return slices.Sorted(maps.Keys(nc.amiIDs))
}

func (nc *EC2NodeClass) GetSubnetIDs() []string {
	return slices.Sorted(maps.Keys(nc.subnetIDs))
}

func (nc *EC2NodeClass) GetSecurityGroupIDs() []string {
	return slices.Sorted(maps.Keys(nc.securityGroupIDs))
}

func (nc *EC2NodeClass) sum64() uint64 {
	h := fnv.New64()

	h.Write([]byte(nc.amiFamily))

	for _, x := range slices.Sorted(maps.Keys(nc.securityGroupIDs)) {
		h.Write([]byte(x))
	}

	return h.Sum64()
}

type NodePool struct {
	name             string
	ec2NodeClass     string
	labels           map[string]string
	taints           map[taint]struct{}
	architectures    map[string]struct{}
	zones            map[string]struct{}
	instanceFamilies map[string]struct{}
	capacityTypes    map[string]struct{}
}

func (np *NodePool) GetName() string {
	return np.name
}

func (np *NodePool) GetEC2NodeClass() string {
	return np.ec2NodeClass
}

func (np *NodePool) GetLabels() map[string]string {
	return np.labels
}

func (np *NodePool) GetTaints() []corev1.Taint {
	taints := lo.Map(
		slices.SortedFunc(maps.Keys(np.taints), compareTaints),
		toCoreTaint,
	)
	return taints
}

func (np *NodePool) GetArchitectures() []string {
	return slices.Sorted(maps.Keys(np.architectures))
}

func (np *NodePool) GetZones() []string {
	return slices.Sorted(maps.Keys(np.zones))
}

func (np *NodePool) GetInstanceFamilies() []string {
	return slices.Sorted(maps.Keys(np.instanceFamilies))
}

func (np *NodePool) GetCapacityTypes() []string {
	return slices.Sorted(maps.Keys(np.capacityTypes))
}

func (np *NodePool) sum64() uint64 {
	h := fnv.New64()

	h.Write([]byte(np.ec2NodeClass))

	for _, k := range slices.Sorted(maps.Keys(np.labels)) {
		h.Write([]byte(k))
		h.Write([]byte(np.labels[k]))
	}

	for _, taint := range slices.SortedFunc(maps.Keys(np.taints), compareTaints) {
		h.Write([]byte(taint.key))
		h.Write([]byte(taint.value))
		h.Write([]byte(taint.effect))
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
	AMIFamily        string
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
		amiFamily:        p.AMIFamily,
		amiIDs:           make(map[string]struct{}),
		subnetIDs:        lo.Keyify(p.SubnetIDs),
		securityGroupIDs: lo.Keyify(p.SecurityGroupIDs),
	}

	if p.AMIID != "" {
		nc.amiIDs[p.AMIID] = struct{}{}
	}

	h := nc.sum64()

	nc.name = "dd-karpenter-" + encodeUint64Base32(h)[8:]

	if n, found := nps.ec2NodeClasses[h]; found {
		if p.AMIID != "" {
			n.amiIDs[p.AMIID] = struct{}{}
		}
		maps.Copy(n.subnetIDs, lo.Keyify(p.SubnetIDs))
		nps.ec2NodeClasses[h] = n
	} else {
		nps.ec2NodeClasses[h] = nc
	}

	np := NodePool{
		ec2NodeClass:     nc.name,
		labels:           sanitizeLabels(p.Labels),
		taints:           lo.Keyify(lo.Map(p.Taints, toTaint)),
		architectures:    make(map[string]struct{}),
		zones:            lo.Keyify(p.Zones),
		instanceFamilies: extractInstanceFamilies(p.InstanceTypes),
		capacityTypes:    map[string]struct{}{p.CapacityType: {}},
	}

	if p.Architecture != "" {
		np.architectures[p.Architecture] = struct{}{}
	}

	h = np.sum64()

	np.name = "dd-karpenter-" + encodeUint64Base32(h)[8:]

	if n, found := nps.nodePools[h]; found {
		if p.Architecture != "" {
			n.architectures[p.Architecture] = struct{}{}
		}
		maps.Copy(n.zones, lo.Keyify(p.Zones))
		maps.Copy(n.instanceFamilies, extractInstanceFamilies(p.InstanceTypes))
		n.capacityTypes[p.CapacityType] = struct{}{}
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

func encodeUint64Base32(n uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
}

// extractInstanceFamilies extracts instance families from instance types by taking
// the part before the first dot (e.g., "m5" from "m5.large", "t3" from "t3.micro").
func extractInstanceFamilies(instanceTypes []string) map[string]struct{} {
	return lo.Keyify(lo.FilterMap(instanceTypes, func(instanceType string, _ int) (string, bool) {
		family, _, _ := strings.Cut(instanceType, ".")
		return family, family != ""
	}))
}
