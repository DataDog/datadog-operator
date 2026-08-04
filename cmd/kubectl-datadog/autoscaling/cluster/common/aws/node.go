package aws

import (
	"regexp"

	corev1 "k8s.io/api/core/v1"
)

// awsProviderIDRegexp matches the AWS provider ID for EC2-backed nodes.
// Format: aws:///<az>/i-<hex> (e.g. aws:///us-east-1a/i-0abc123def456789).
// Fargate nodes use a different shape (aws:///<az>/fargate-ip-...) and must
// therefore be classified by label before reaching this regex.
var awsProviderIDRegexp = regexp.MustCompile(`^aws:///[^/]+/(i-[0-9a-f]+)$`)

// LabelEKSNodegroup is the label EKS stamps on every node that belongs to a
// managed node group. The label value is the node group name. Exposed as a
// constant so every consumer (classifier, evict-legacy-nodes, future code)
// references the same string.
const LabelEKSNodegroup = "eks.amazonaws.com/nodegroup"

// ExtractEC2InstanceID returns the EC2 instance ID (i-...) from a Node's
// providerID, or false when the providerID is not an EC2 instance (Fargate
// uses `aws:///<az>/fargate-ip-...`, GCP/Azure use entirely different shapes,
// etc.). Lives here in `common/aws` so both `common/clusterinfo` (which
// imports `common/karpenter`) and `common/karpenter` (which classifies its
// own nodes) can use it without creating an import cycle.
func ExtractEC2InstanceID(node *corev1.Node) (string, bool) {
	if node == nil {
		return "", false
	}
	m := awsProviderIDRegexp.FindStringSubmatch(node.Spec.ProviderID)
	if len(m) != 2 {
		return "", false
	}
	return m[1], true
}
