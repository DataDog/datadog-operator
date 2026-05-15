package clusterinfo

import (
	"context"
	"fmt"
	"log"
	"maps"
	"regexp"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	astypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	commonaws "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterautoscaler"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/eksautomode"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/karpenter"
)

// describeASGInstancesMaxIDs is the documented per-call limit of
// autoscaling:DescribeAutoScalingInstances. Sending more triggers a
// ValidationError at the API.
const describeASGInstancesMaxIDs = 50

// awsProviderIDRegexp matches the AWS provider ID for EC2-backed nodes.
// Format: aws:///<az>/i-<hex>. Fargate nodes use a different shape and
// must therefore be classified by label before reaching this regex.
var awsProviderIDRegexp = regexp.MustCompile(`^aws:///[^/]+/(i-[0-9a-f]+)$`)

// nodePoolDatadogCreatedLabel is the label set by every Datadog autoscaling
// product (kubectl-datadog AND the cluster agent) on the NodePools they
// manage. Broader than the AND-pair `app.kubernetes.io/managed-by:
// kubectl-datadog` + this label that uninstall uses, on purpose: the
// migration tool must preserve cluster-agent-managed NodePools too.
const nodePoolDatadogCreatedLabel = "autoscaling.datadoghq.com/created"

// AutoscalingDescriber is the subset of *autoscaling.Client used by Classify.
// Defined as an interface so tests can substitute a fake without spinning up
// AWS SDK middleware.
type AutoscalingDescriber interface {
	DescribeAutoScalingInstances(ctx context.Context, in *autoscaling.DescribeAutoScalingInstancesInput, opts ...func(*autoscaling.Options)) (*autoscaling.DescribeAutoScalingInstancesOutput, error)
}

// EKSDescriber is the subset of *eks.Client used by Classify (cluster
// identity lookup and Fargate ownership detection). Defined as an
// interface so tests can substitute a fake without spinning up AWS SDK
// middleware.
type EKSDescriber interface {
	DescribeCluster(ctx context.Context, in *eks.DescribeClusterInput, opts ...func(*eks.Options)) (*eks.DescribeClusterOutput, error)
	DescribeFargateProfile(ctx context.Context, in *eks.DescribeFargateProfileInput, opts ...func(*eks.Options)) (*eks.DescribeFargateProfileOutput, error)
}

// ClassifyInput bundles the clients and parameters Classify needs. Grouped
// in a struct because the parameter list would otherwise grow long enough
// to be error-prone at the call site.
type ClassifyInput struct {
	K8sClient   kubernetes.Interface
	CtrlClient  client.Client
	Autoscaling AutoscalingDescriber
	EKS         EKSDescriber
	Discovery   discovery.DiscoveryInterface
	ClusterName string
}

// Classify inspects every node in the cluster, groups them by management
// method, detects the autoscaling solutions present, and returns the
// resulting snapshot.
func Classify(ctx context.Context, in ClassifyInput) (*ClusterInfo, error) {
	info := &ClusterInfo{
		APIVersion:     APIVersion,
		ClusterName:    in.ClusterName,
		GeneratedAt:    time.Now().UTC(),
		NodeManagement: map[NodeManager]map[string]NodeManagerEntry{},
	}
	info.ClusterARN, info.Region = describeClusterIdentity(ctx, in.EKS, in.ClusterName)

	fargateProfileByNode, err := fargateProfilesByNode(ctx, in.K8sClient)
	if err != nil {
		return nil, err
	}

	asgCandidates, err := classifyByLabels(ctx, in.K8sClient, fargateProfileByNode, info)
	if err != nil {
		return nil, err
	}

	if err = resolveASGs(ctx, in.Autoscaling, asgCandidates, info); err != nil {
		return nil, err
	}

	enrichKarpenterOwnership(ctx, in.CtrlClient, info)
	enrichFargateOwnership(ctx, in.EKS, in.ClusterName, info)

	info.Autoscaling.ClusterAutoscaler, err = detectClusterAutoscaler(ctx, in.K8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to detect cluster-autoscaler: %w", err)
	}

	info.Autoscaling.Karpenter, err = detectKarpenter(ctx, in.K8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to detect Karpenter: %w", err)
	}

	info.Autoscaling.EKSAutoMode = detectEKSAutoMode(in.Discovery)

	return info, nil
}

// fargateProfilesByNode lists Pods that carry the
// `eks.amazonaws.com/fargate-profile` label (server-side filtered) and indexes
// the profile name by the Node the Pod is scheduled on. EKS stamps that label
// on the Pod, not on the Node, so the Node listing alone cannot recover the
// profile name. A Fargate node hosts exactly one user pod, so the map is
// uncontested in practice.
func fargateProfilesByNode(ctx context.Context, k8sClient kubernetes.Interface) (map[string]string, error) {
	const fargateProfileLabel = "eks.amazonaws.com/fargate-profile"

	out := map[string]string{}
	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return k8sClient.CoreV1().Pods(corev1.NamespaceAll).List(ctx, opts)
	})
	opts := metav1.ListOptions{LabelSelector: fargateProfileLabel}
	if err := p.EachListItem(ctx, opts, func(obj runtime.Object) error {
		pod := obj.(*corev1.Pod)
		if pod.Spec.NodeName == "" {
			return nil
		}
		if profile := pod.Labels[fargateProfileLabel]; profile != "" {
			out[pod.Spec.NodeName] = profile
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list Fargate pods: %w", err)
	}
	return out, nil
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
func classifyByLabels(ctx context.Context, k8sClient kubernetes.Interface, fargateProfileByNode map[string]string, info *ClusterInfo) ([]asgCandidate, error) {
	var candidates []asgCandidate

	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return k8sClient.CoreV1().Nodes().List(ctx, opts)
	})
	if err := p.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		node := obj.(*corev1.Node)
		if mgr, entity, ok := classifyNodeByLabel(node, fargateProfileByNode); ok {
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

// classifyNodeByLabel applies steps 1-3 of the decision tree using the Node
// labels, the Node name, and a pre-built nodeName→Fargate-profile index.
// Returns false when the node needs an AWS API lookup or the unknown-bucket
// fallback.
func classifyNodeByLabel(node *corev1.Node, fargateProfileByNode map[string]string) (NodeManager, string, bool) {
	if node.Labels["eks.amazonaws.com/compute-type"] == "fargate" || strings.HasPrefix(node.Name, "fargate-ip-") {
		return NodeManagerFargate, fargateProfileByNode[node.Name], true
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
		bucket = map[string]NodeManagerEntry{}
		info.NodeManagement[mgr] = bucket
	}
	entry := bucket[entity]
	entry.Nodes = append(entry.Nodes, nodeName)
	bucket[entity] = entry
}

// enrichKarpenterOwnership lists every NodePool labelled by a Datadog
// autoscaling product and ensures the Karpenter bucket reflects them as
// ManagedByDatadog. The label is "autoscaling.datadoghq.com/created" alone
// — broader than uninstall's AND-pair, on purpose: the cluster agent
// creates NodePools without setting "app.kubernetes.io/managed-by:
// kubectl-datadog", and the migration tool must preserve them too.
//
// NodePools with zero current nodes (typical right after install) are
// surfaced too: the bucket gets an entry with empty Nodes so the migration
// tool sees the destination NodePool exists, even before any workload has
// landed on it.
//
// Best-effort: a missing CRD (Karpenter not installed at all) or a
// transient list error never fails the snapshot — the bucket simply isn't
// enriched.
func enrichKarpenterOwnership(ctx context.Context, ctrlClient client.Client, info *ClusterInfo) {
	list := &karpv1.NodePoolList{}
	err := ctrlClient.List(ctx, list, client.MatchingLabels{nodePoolDatadogCreatedLabel: "true"})
	if err != nil {
		if meta.IsNoMatchError(err) {
			return
		}
		log.Printf("Warning: failed to list Datadog-managed NodePools: %v", err)
		return
	}
	if len(list.Items) == 0 {
		return
	}

	bucket := info.NodeManagement[NodeManagerKarpenter]
	if bucket == nil {
		bucket = map[string]NodeManagerEntry{}
		info.NodeManagement[NodeManagerKarpenter] = bucket
	}
	for _, np := range list.Items {
		entry := bucket[np.Name]
		entry.ManagedByDatadog = true
		bucket[np.Name] = entry
	}
}

// enrichFargateOwnership reads the tags on every Fargate profile encountered
// in the snapshot and sets ManagedByDatadog when the profile carries the
// kubectl-datadog `managed-by` tag. The tag is propagated to the profile
// from the CloudFormation stack tags (cf. common/aws/cloudformation.go's
// buildTags). DescribeFargateProfile is one API call per unique profile —
// in practice 1, capped well under 10 even pathologically.
//
// Best-effort: if the EKS API call fails (AccessDenied, throttle), the
// profile stays `ManagedByDatadog: false` and a warning is logged.
func enrichFargateOwnership(ctx context.Context, eksClient EKSDescriber, clusterName string, info *ClusterInfo) {
	bucket := info.NodeManagement[NodeManagerFargate]
	for entity, entry := range bucket {
		if entity == "" {
			// Nodes whose Fargate profile name we couldn't read from
			// labels (entity == "") cannot be looked up via the API.
			continue
		}
		out, err := eksClient.DescribeFargateProfile(ctx, &eks.DescribeFargateProfileInput{
			ClusterName:        awssdk.String(clusterName),
			FargateProfileName: awssdk.String(entity),
		})
		if err != nil {
			log.Printf("Warning: failed to describe Fargate profile %q: %v", entity, err)
			continue
		}
		if out.FargateProfile != nil &&
			out.FargateProfile.Tags[commonaws.ManagedByTag] == commonaws.ManagedByTagValue {
			entry.ManagedByDatadog = true
			bucket[entity] = entry
		}
	}
}

// detectClusterAutoscaler returns the running cluster-autoscaler, or a
// zero-value ClusterAutoscaler if none is found.
func detectClusterAutoscaler(ctx context.Context, k8sClient kubernetes.Interface) (ClusterAutoscaler, error) {
	inst, err := clusterautoscaler.FindInstallation(ctx, k8sClient)
	if err != nil {
		return ClusterAutoscaler{}, err
	}
	if inst == nil {
		return ClusterAutoscaler{}, nil
	}
	return ClusterAutoscaler{
		Present:   true,
		Namespace: inst.Namespace,
		Name:      inst.Name,
		Version:   inst.Version,
	}, nil
}

// detectKarpenter returns the running Karpenter controller, or a zero-value
// Karpenter if none is found.
func detectKarpenter(ctx context.Context, k8sClient kubernetes.Interface) (Karpenter, error) {
	inst, err := karpenter.FindInstallation(ctx, k8sClient)
	if err != nil {
		return Karpenter{}, err
	}
	if inst == nil {
		return Karpenter{}, nil
	}
	return Karpenter{
		Present:          true,
		Namespace:        inst.Namespace,
		Name:             inst.Name,
		Version:          inst.Version,
		ManagedByDatadog: inst.IsOwn(),
		InstallerVersion: inst.InstallerVersion,
	}, nil
}

// describeClusterIdentity returns the cluster's full ARN and the AWS region
// extracted from it. Best-effort: any failure (DescribeCluster error,
// missing ARN, malformed ARN) yields ("", "") and a logged warning — the
// snapshot is still useful without these identifiers.
func describeClusterIdentity(ctx context.Context, eksClient EKSDescriber, clusterName string) (clusterARN, region string) {
	out, err := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: awssdk.String(clusterName)})
	if err != nil {
		log.Printf("Warning: failed to describe cluster %q: %v", clusterName, err)
		return "", ""
	}
	if out.Cluster == nil || out.Cluster.Arn == nil {
		log.Printf("Warning: cluster %q has no ARN in DescribeCluster response", clusterName)
		return "", ""
	}
	clusterARN = *out.Cluster.Arn
	parsed, err := arn.Parse(clusterARN)
	if err != nil {
		log.Printf("Warning: failed to parse cluster ARN %q: %v", clusterARN, err)
		return clusterARN, ""
	}
	return clusterARN, parsed.Region
}

// detectEKSAutoMode reports whether EKS auto-mode is active. Failures are
// logged and surfaced as Enabled: false — this is a best-effort signal.
func detectEKSAutoMode(disco discovery.DiscoveryInterface) EKSAutoMode {
	enabled, err := eksautomode.IsEnabled(disco)
	if err != nil {
		log.Printf("Warning: failed to detect EKS auto-mode: %v", err)
		return EKSAutoMode{}
	}
	return EKSAutoMode{Enabled: enabled}
}
