// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// Source keys.
const (
	SourceDDA                 = "dda"
	SourceDDAI                = "ddai"
	SourceDAP                 = "dap"
	SourceDaemonSets          = "ds"
	SourceDeployments         = "deploy"
	SourceControllerRevisions = "crev"
	SourceReplicaSets         = "rs"
	SourcePods                = "pods"
	SourceOperator            = "operator"
	SourceLease               = "lease"

	// summarySourcePrefix prefixes the key of each summary kind source.
	summarySourcePrefix = "sum/"
)

const (
	appNameLabel    = "app.kubernetes.io/name"
	operatorAppName = "datadog-operator"
	// OperatorLabelSelector selects the operator Deployment.
	OperatorLabelSelector = appNameLabel + "=" + operatorAppName
	// OperatorLeaseName is the name of the operator leader election Lease.
	OperatorLeaseName = "datadog-operator-lock"

	lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"
)

// Scope is where a source is listed.
type Scope string

const (
	// ScopeTarget is the namespace given on the command line, or all
	// namespaces with -A.
	ScopeTarget Scope = "Target"
	// ScopeDDANamespace is the namespace of the selected DDA.
	ScopeDDANamespace Scope = "DDANamespace"
	// ScopeClusterFallback is all namespaces, falling back to the DDA
	// namespace with a notice when a cluster-wide list is forbidden.
	ScopeClusterFallback Scope = "ClusterFallback"
	// ScopeOperatorNamespace is the namespace of the operator Deployment.
	ScopeOperatorNamespace Scope = "OperatorNamespace"
)

// TransformFunc reduces an object before it is stored. It has the same
// signature as client-go's cache.TransformFunc.
type TransformFunc func(any) (any, error)

// Source describes one kind of object the dashboard reads.
type Source struct {
	Key  string
	Kind string
	GVR  schema.GroupVersionResource
	// Scope is where the source is listed.
	Scope Scope
	// Selector returns the label selector for the selected DDA; nil means
	// no selector.
	Selector func(dda string) string
	// Name restricts the source to one object name; empty means all.
	Name string
	// Required sources make the command fail when their CRD is missing.
	// Profiles are optional: without the DAP CRD the tree has no DAP nodes.
	Required bool
	// Mode is how the source is refreshed in live mode.
	Mode RefreshMode
	// Transform reduces each object before it is stored.
	Transform TransformFunc
}

// SummaryKind is a Datadog CR kind shown in the summary panel.
type SummaryKind struct {
	Kind     string
	Resource string
	// Label is the panel row name.
	Label string
}

// SourceKey returns the source key of the summary kind.
func (k SummaryKind) SourceKey() string { return summarySourcePrefix + k.Kind }

// IsSummarySource reports whether key is the source key of a summary kind.
func IsSummarySource(key string) bool { return strings.HasPrefix(key, summarySourcePrefix) }

// SummaryKinds lists the summary kinds in display order.
var SummaryKinds = []SummaryKind{
	{Kind: "DatadogMonitor", Resource: "datadogmonitors", Label: "Monitors"},
	{Kind: "DatadogDashboard", Resource: "datadogdashboards", Label: "Dashboards"},
	{Kind: "DatadogSLO", Resource: "datadogslos", Label: "SLOs"},
	{Kind: "DatadogGenericResource", Resource: "datadoggenericresources", Label: "Generic resources"},
	{Kind: "DatadogMetric", Resource: "datadogmetrics", Label: "DatadogMetrics"},
	{Kind: "DatadogInstrumentation", Resource: "datadoginstrumentations", Label: "Instrumentations"},
	{Kind: "DatadogCSIDriver", Resource: "datadogcsidrivers", Label: "CSI drivers"},
	{Kind: "DatadogBYOCCluster", Resource: "datadogbyocclusters", Label: "BYOC clusters"},
}

// agentSelector selects the workloads and pods of a DDA.
func agentSelector(dda string) string {
	return apicommon.AgentDeploymentNameLabelKey + "=" + dda
}

func operatorSelector(string) string { return OperatorLabelSelector }

// Sources returns the source table driving every Store.
func Sources() []Source {
	v1alpha1 := func(resource string) schema.GroupVersionResource {
		return schema.GroupVersionResource{Group: "datadoghq.com", Version: "v1alpha1", Resource: resource}
	}
	apps := func(resource string) schema.GroupVersionResource {
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: resource}
	}
	core := []Source{
		{Key: SourceDDA, Kind: "DatadogAgent", GVR: schema.GroupVersionResource{Group: "datadoghq.com", Version: "v2alpha1", Resource: "datadogagents"}, Scope: ScopeTarget, Required: true, Mode: RefreshWatch, Transform: StripObject},
		{Key: SourceDDAI, Kind: "DatadogAgentInternal", GVR: v1alpha1("datadogagentinternals"), Scope: ScopeDDANamespace, Required: true, Mode: RefreshWatch, Transform: StripObject},
		{Key: SourceDAP, Kind: "DatadogAgentProfile", GVR: v1alpha1("datadogagentprofiles"), Scope: ScopeClusterFallback, Mode: RefreshWatch, Transform: StripObject},
		{Key: SourceDaemonSets, Kind: "DaemonSet", GVR: apps("daemonsets"), Scope: ScopeDDANamespace, Selector: agentSelector, Required: true, Mode: RefreshWatch, Transform: StripObject},
		{Key: SourceDeployments, Kind: "Deployment", GVR: apps("deployments"), Scope: ScopeDDANamespace, Selector: agentSelector, Required: true, Mode: RefreshWatch, Transform: StripObject},
		{Key: SourceControllerRevisions, Kind: "ControllerRevision", GVR: apps("controllerrevisions"), Scope: ScopeDDANamespace, Selector: agentSelector, Required: true, Mode: RefreshWatch, Transform: StripControllerRevision},
		{Key: SourceReplicaSets, Kind: "ReplicaSet", GVR: apps("replicasets"), Scope: ScopeDDANamespace, Selector: agentSelector, Required: true, Mode: RefreshWatch, Transform: StripObject},
		{Key: SourcePods, Kind: "Pod", GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Scope: ScopeDDANamespace, Selector: agentSelector, Mode: RefreshWatch, Transform: StripPod},
		{Key: SourceOperator, Kind: "Deployment", GVR: apps("deployments"), Scope: ScopeClusterFallback, Selector: operatorSelector, Mode: RefreshPoll, Transform: StripObject},
		{Key: SourceLease, Kind: "Lease", GVR: schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}, Scope: ScopeOperatorNamespace, Name: OperatorLeaseName, Mode: RefreshPoll, Transform: StripObject},
	}
	summary := make([]Source, 0, len(SummaryKinds))
	for _, k := range SummaryKinds {
		summary = append(summary, Source{Key: k.SourceKey(), Kind: k.Kind, GVR: v1alpha1(k.Resource), Scope: ScopeClusterFallback, Mode: RefreshPoll, Transform: StripObject})
	}
	return slices.Concat(core, summary)
}

// StripObject drops managedFields and the last-applied annotation from any
// Kubernetes object. Other values are returned unchanged.
func StripObject(obj any) (any, error) {
	if o, ok := obj.(metav1.Object); ok {
		stripMeta(o)
	}
	return obj, nil
}

// StripControllerRevision is StripObject plus dropping the revision data,
// which holds a full copy of the pod template.
func StripControllerRevision(obj any) (any, error) {
	if u, ok := obj.(interface{ UnstructuredContent() map[string]any }); ok {
		delete(u.UnstructuredContent(), "data")
	}
	return StripObject(obj)
}

func stripMeta(o metav1.Object) {
	o.SetManagedFields(nil)
	if ann := o.GetAnnotations(); ann != nil {
		if _, found := ann[lastAppliedAnnotation]; found {
			delete(ann, lastAppliedAnnotation)
			o.SetAnnotations(ann)
		}
	}
}

// StripPod reduces a Pod to the fields the dashboard uses: labels, owner
// references, creation time, node name, phase, conditions and container
// states. Other values are returned unchanged.
func StripPod(obj any) (any, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return obj, nil
	}
	out := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              pod.Name,
			Namespace:         pod.Namespace,
			UID:               pod.UID,
			ResourceVersion:   pod.ResourceVersion,
			Labels:            pod.Labels,
			OwnerReferences:   pod.OwnerReferences,
			CreationTimestamp: pod.CreationTimestamp,
			DeletionTimestamp: pod.DeletionTimestamp,
		},
		Spec: corev1.PodSpec{NodeName: pod.Spec.NodeName},
		Status: corev1.PodStatus{
			Phase:                 pod.Status.Phase,
			Conditions:            pod.Status.Conditions,
			InitContainerStatuses: stripContainerStatuses(pod.Status.InitContainerStatuses),
			ContainerStatuses:     stripContainerStatuses(pod.Status.ContainerStatuses),
		},
	}
	return out, nil
}

func stripContainerStatuses(in []corev1.ContainerStatus) []corev1.ContainerStatus {
	if in == nil {
		return nil
	}
	out := make([]corev1.ContainerStatus, len(in))
	for i, cs := range in {
		out[i] = corev1.ContainerStatus{
			Name:                 cs.Name,
			Ready:                cs.Ready,
			RestartCount:         cs.RestartCount,
			State:                cs.State,
			LastTerminationState: cs.LastTerminationState,
		}
	}
	return out
}
