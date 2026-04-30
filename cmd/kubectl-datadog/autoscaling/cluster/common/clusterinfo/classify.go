package clusterinfo

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	astypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"
)

// describeASGInstancesMaxIDs is the documented per-call limit of
// autoscaling:DescribeAutoScalingInstances. Sending more triggers a
// ValidationError at the API.
const describeASGInstancesMaxIDs = 50

// awsProviderIDRegexp matches the AWS provider ID for EC2-backed nodes.
// Format: aws:///<az>/i-<hex>. Fargate nodes use a different shape and
// must therefore be classified by label before reaching this regex.
var awsProviderIDRegexp = regexp.MustCompile(`^aws:///[^/]+/(i-[0-9a-f]+)$`)

// AutoscalingDescriber is the subset of *autoscaling.Client used by Classify.
// Defined as an interface so tests can substitute a fake without spinning up
// AWS SDK middleware.
type AutoscalingDescriber interface {
	DescribeAutoScalingInstances(ctx context.Context, in *autoscaling.DescribeAutoScalingInstancesInput, opts ...func(*autoscaling.Options)) (*autoscaling.DescribeAutoScalingInstancesOutput, error)
}

// Classify inspects every node in the cluster, groups them by management
// method, and returns the resulting snapshot.
func Classify(ctx context.Context, k8sClient kubernetes.Interface, asg AutoscalingDescriber, clusterName string) (*ClusterInfo, error) {
	info := &ClusterInfo{
		APIVersion:     APIVersion,
		ClusterName:    clusterName,
		GeneratedAt:    time.Now().UTC(),
		NodeManagement: map[NodeManager]map[string][]string{},
	}

	asgCandidates, err := classifyByLabels(ctx, k8sClient, info)
	if err != nil {
		return nil, err
	}

	if err = resolveASGs(ctx, asg, asgCandidates, info); err != nil {
		return nil, err
	}

	info.ClusterAutoscaler, err = detectClusterAutoscaler(ctx, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to detect cluster-autoscaler: %w", err)
	}

	return info, nil
}

// asgCandidate is a node that needs an AWS API call to determine whether
// it's in an ASG (asg bucket) or not (standalone bucket).
type asgCandidate struct {
	instanceID string
	nodeName   string
}

// classifyByLabels walks all nodes and applies the label-only branches of the
// decision tree (Fargate, Karpenter, EKS managed node group, unknown). Nodes
// with an AWS EC2 providerID that don't match any of the above are returned
// as ASG candidates for resolveASGs to bucket as asg or standalone.
func classifyByLabels(ctx context.Context, k8sClient kubernetes.Interface, info *ClusterInfo) ([]asgCandidate, error) {
	var candidates []asgCandidate

	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return k8sClient.CoreV1().Nodes().List(ctx, opts)
	})
	if err := p.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		node := obj.(*corev1.Node)
		if mgr, entity, ok := classifyNodeByLabel(node); ok {
			addToBucket(info, mgr, entity, node.Name)
			return nil
		}

		matches := awsProviderIDRegexp.FindStringSubmatch(node.Spec.ProviderID)
		if len(matches) == 2 {
			candidates = append(candidates, asgCandidate{
				instanceID: matches[1],
				nodeName:   node.Name,
			})
		} else {
			addToBucket(info, NodeManagerUnknown, node.Spec.ProviderID, node.Name)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	return candidates, nil
}

// classifyNodeByLabel applies steps 1-3 of the decision tree using only the
// Node labels and name. Returns false when the node needs an AWS API lookup
// or the unknown-bucket fallback.
func classifyNodeByLabel(node *corev1.Node) (NodeManager, string, bool) {
	if node.Labels["eks.amazonaws.com/compute-type"] == "fargate" || strings.HasPrefix(node.Name, "fargate-ip-") {
		return NodeManagerFargate, node.Labels["eks.amazonaws.com/fargate-profile"], true
	}

	if v := node.Labels["karpenter.sh/nodepool"]; v != "" {
		return NodeManagerKarpenter, v, true
	}
	if v := node.Labels["karpenter.k8s.aws/ec2nodeclass"]; v != "" {
		return NodeManagerKarpenter, v, true
	}
	// Legacy Karpenter v0.x (pre-NodePool) uses Provisioner instead.
	if v := node.Labels["karpenter.sh/provisioner-name"]; v != "" {
		return NodeManagerKarpenter, v, true
	}

	if v := node.Labels["eks.amazonaws.com/nodegroup"]; v != "" {
		return NodeManagerEKSManagedNodeGroup, v, true
	}

	return "", "", false
}

// resolveASGs batches DescribeAutoScalingInstances calls (50 IDs per call,
// the documented limit) to map instance IDs to ASGs. Instances reported
// without an AutoScalingGroupName fall into the standalone bucket.
func resolveASGs(ctx context.Context, asg AutoscalingDescriber, candidates []asgCandidate, info *ClusterInfo) error {
	byInstance := make(map[string]string, len(candidates))

	for _, batch := range lo.Chunk(candidates, describeASGInstancesMaxIDs) {
		ids := lo.Map(batch, func(c asgCandidate, _ int) string { return c.instanceID })
		out, err := asg.DescribeAutoScalingInstances(ctx, &autoscaling.DescribeAutoScalingInstancesInput{
			InstanceIds: ids,
		})
		if err != nil {
			return fmt.Errorf("failed to describe autoscaling instances: %w", err)
		}
		maps.Copy(byInstance, lo.FilterSliceToMap(out.AutoScalingInstances, func(ai astypes.AutoScalingInstanceDetails) (string, string, bool) {
			if ai.InstanceId == nil || ai.AutoScalingGroupName == nil {
				return "", "", false
			}
			return *ai.InstanceId, *ai.AutoScalingGroupName, true
		}))
	}

	for _, c := range candidates {
		if name := byInstance[c.instanceID]; name != "" {
			addToBucket(info, NodeManagerASG, name, c.nodeName)
		} else {
			addToBucket(info, NodeManagerStandalone, "", c.nodeName)
		}
	}
	return nil
}

func addToBucket(info *ClusterInfo, mgr NodeManager, entity, nodeName string) {
	bucket := info.NodeManagement[mgr]
	if bucket == nil {
		bucket = map[string][]string{}
		info.NodeManagement[mgr] = bucket
	}
	bucket[entity] = append(bucket[entity], nodeName)
}

// detectClusterAutoscaler scans Deployments cluster-wide and returns the
// first matching one encountered. A match is any Deployment with name
// "cluster-autoscaler", a well-known label, or a container image referencing
// "cluster-autoscaler". The cluster-wide enumeration order is unspecified, so
// when several matches coexist (a configuration we don't expect in practice),
// which one wins is non-deterministic — we accept that to keep the scan
// short-circuited and avoid materialising every Deployment in memory.
func detectClusterAutoscaler(ctx context.Context, k8sClient kubernetes.Interface) (ClusterAutoscaler, error) {
	errFound := errors.New("cluster-autoscaler found")

	var result ClusterAutoscaler
	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return k8sClient.AppsV1().Deployments(corev1.NamespaceAll).List(ctx, opts)
	})
	err := p.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		dep := obj.(*appsv1.Deployment)
		if !isClusterAutoscaler(*dep) {
			return nil
		}
		result = ClusterAutoscaler{
			Present:   true,
			Namespace: dep.Namespace,
			Name:      dep.Name,
			Version:   extractClusterAutoscalerVersion(*dep),
		}
		return errFound
	})
	if err != nil && !errors.Is(err, errFound) {
		return ClusterAutoscaler{}, fmt.Errorf("failed to list Deployments: %w", err)
	}
	return result, nil
}

func isClusterAutoscaler(d appsv1.Deployment) bool {
	// A Deployment scaled to zero is effectively disabled; ignoring it lets
	// users who already stopped CA (per the Karpenter migration guide) get
	// `Present: false` in the snapshot. A nil Replicas defaults to 1 per the
	// Kubernetes API, so it counts as active.
	if d.Spec.Replicas != nil && *d.Spec.Replicas == 0 {
		return false
	}
	if d.Name == "cluster-autoscaler" ||
		d.Labels["app.kubernetes.io/name"] == "cluster-autoscaler" ||
		d.Labels["k8s-app"] == "cluster-autoscaler" {
		return true
	}
	return slices.ContainsFunc(d.Spec.Template.Spec.Containers, func(c corev1.Container) bool {
		return strings.Contains(c.Image, "cluster-autoscaler")
	})
}

// extractClusterAutoscalerVersion returns the running cluster-autoscaler
// version. Prefers the image tag of the matching container (the source of
// truth) and falls back to the `app.kubernetes.io/version` label on the
// Deployment or its pod template (set by most Helm charts). Empty when
// neither is available — e.g. an image referenced by digest only and no
// version label.
func extractClusterAutoscalerVersion(d appsv1.Deployment) string {
	for _, c := range d.Spec.Template.Spec.Containers {
		if !strings.Contains(c.Image, "cluster-autoscaler") {
			continue
		}
		if tag := imageTag(c.Image); tag != "" {
			return tag
		}
	}
	if v := d.Labels["app.kubernetes.io/version"]; v != "" {
		return v
	}
	return d.Spec.Template.Labels["app.kubernetes.io/version"]
}

// imageTag extracts the tag portion of an OCI image reference, stripping
// any `@sha256:...` digest. Returns empty when no tag is set (for instance,
// digest-only references or bare image names).
func imageTag(image string) string {
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	// The last colon is the tag separator only if it is not followed by a
	// path component — otherwise it's a registry port (e.g. `localhost:5000/foo`).
	if i := strings.LastIndex(image, ":"); i >= 0 && !strings.Contains(image[i+1:], "/") {
		return image[i+1:]
	}
	return ""
}
