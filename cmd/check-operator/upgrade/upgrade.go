// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

const (
	// daemonSetTemplateGenerationAnnotation and podTemplateGenerationLabel are set by Kubernetes
	// on DaemonSets and their pods. A pod is up to date when both values match.
	daemonSetTemplateGenerationAnnotation = "deprecated.daemonset.template.generation"
	podTemplateGenerationLabel            = "pod-template-generation"
)

// daemonSetDefaultTolerations are the tolerations that the DaemonSet controller adds to every
// DaemonSet pod (AddOrUpdateDaemonPodTolerations). The network-unavailable toleration is added
// only for host-network pods.
var daemonSetDefaultTolerations = []corev1.Toleration{
	{Key: corev1.TaintNodeNotReady, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
	{Key: corev1.TaintNodeUnreachable, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
	{Key: corev1.TaintNodeDiskPressure, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
	{Key: corev1.TaintNodeMemoryPressure, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
	{Key: corev1.TaintNodePIDPressure, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
	{Key: corev1.TaintNodeUnschedulable, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
}

// Options provides information required to manage canary.
type Options struct {
	genericclioptions.IOStreams
	common.Options
	args               []string
	datadogAgentName   string
	checkPeriod        time.Duration
	checkTimeout       time.Duration
	agentCompletionPct float64
	agentCompletionMin int32
	dcaMinUpToDate     int32
	clcMinUpToDate     int32
	// skipNotReadyNodes discounts out-of-date Agent pods on not-Ready nodes. These nodes usually
	// cannot run the new pod, so they would otherwise hold the check until its timeout.
	skipNotReadyNodes bool
	// maxNodeLookups caps the node GETs done in one check, to bound API calls (client-go default
	// rate limit is 5 QPS). Above this cap the check does not discount any node.
	maxNodeLookups int
	// maxSkippedRatio caps the share of scheduled Agent nodes that the check can discount. Above it,
	// the not-Ready nodes are more likely an outage than background churn, so the check does not
	// discount any node.
	maxSkippedRatio float64
}

// NewOptions provides an instance of Options with default values.
func NewOptions(streams genericclioptions.IOStreams) *Options {
	opts := &Options{
		IOStreams:          streams,
		checkPeriod:        30 * time.Second,
		checkTimeout:       2 * time.Hour,
		agentCompletionPct: 0.95,
		agentCompletionMin: 10,
		dcaMinUpToDate:     1,
		clcMinUpToDate:     2,
		maxNodeLookups:     300,
		maxSkippedRatio:    0.10,
	}

	opts.SetConfigFlags()

	if val, found := os.LookupEnv("AGENT_COMPLETION_PCT"); found {
		if iVal, err := strconv.ParseFloat(val, 64); err == nil {
			opts.agentCompletionPct = iVal / 100
		}
	}

	if val, found := os.LookupEnv("AGENT_COMPLETION_MIN"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.agentCompletionMin = int32(iVal)
		}
	}

	if val, found := os.LookupEnv("DCA_MIN_UP_TO_DATE"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.dcaMinUpToDate = int32(iVal)
		}
	}

	if val, found := os.LookupEnv("CLC_MIN_UP_TO_DATE"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.clcMinUpToDate = int32(iVal)
		}
	}

	if val, found := os.LookupEnv("AGENT_SKIP_NOT_READY_NODES"); found {
		if bVal, err := strconv.ParseBool(val); err == nil {
			opts.skipNotReadyNodes = bVal
		}
	}

	if val, found := os.LookupEnv("AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.maxNodeLookups = int(iVal)
		}
	}

	if val, found := os.LookupEnv("AGENT_SKIP_NOT_READY_MAX_PCT"); found {
		if fVal, err := strconv.ParseFloat(val, 64); err == nil && fVal >= 0 && fVal <= 100 {
			opts.maxSkippedRatio = fVal / 100
		}
	}

	if val, found := os.LookupEnv("CHECK_TIMEOUT_MINUTES"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.checkTimeout = time.Duration(iVal) * time.Minute
		}
	}

	return opts
}

// NewCmdUpgrade provides a cobra command wrapping Options.
func NewCmdUpgrade(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewOptions(streams)

	cmd := &cobra.Command{
		Use:          "upgrade [DatadogAgent name]",
		Short:        "Wait until the rolling-update of all agent components is finished",
		Example:      "./check-operator upgrade datadog-agent",
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}

			return o.Run()
		},
	}

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// Complete sets all information required for processing the command.
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.datadogAgentName = args[0]
	}

	return o.Init(cmd)
}

// Validate ensures that all required arguments and flag values are provided.
func (o *Options) Validate() error {
	if o.datadogAgentName == "" {
		return fmt.Errorf("the DatadogAgent name is required")
	}

	return nil
}

func (o *Options) getV2Status() (common.StatusWrapper, error) {
	datadogAgent := &v2alpha1.DatadogAgent{}
	err := o.Client.Get(context.TODO(), client.ObjectKey{Namespace: o.UserNamespace, Name: o.datadogAgentName}, datadogAgent)
	if err != nil {
		if errors.IsNotFound(err) {

			return nil, err
		}

		return nil, fmt.Errorf("unable to get DatadogAgent, err: %w", err)
	}
	return common.NewV2StatusWrapper(datadogAgent), nil
}

func (o *Options) getDDAIs(ctx context.Context) ([]v1alpha1.DatadogAgentInternal, error) {
	ddais := &v1alpha1.DatadogAgentInternalList{}
	if err := o.Client.List(
		ctx,
		ddais,
		client.InNamespace(o.UserNamespace),
		client.MatchingLabels{apicommon.DatadogAgentNameLabelKey: o.datadogAgentName},
	); err != nil {
		return nil, fmt.Errorf("unable to list DatadogAgentInternals: %w", err)
	}

	return ddais.Items, nil
}

func isReconcileError(conditions []metav1.Condition) error {
	for _, condition := range conditions {
		switch {
		case condition.Type == "DatadogAgentReconcileError" && condition.Status == metav1.ConditionTrue:
			return fmt.Errorf("datadogAgent reconciliation error message: %s", condition.Message)
		case condition.Type == "DatadogAgentInternalReconcileError" && condition.Status == metav1.ConditionTrue:
			return fmt.Errorf("datadogAgentInternal reconciliation error message: %s", condition.Message)
		case condition.Type == "AgentReconcile" && condition.Status == metav1.ConditionFalse:
			return fmt.Errorf("agent reconciliation error message: %s", condition.Message)
		case condition.Type == "ClusterAgentReconcile" && condition.Status == metav1.ConditionFalse:
			return fmt.Errorf("cluster Agent reconciliation error message: %s", condition.Message)
		case condition.Type == "ClusterChecksRunnerReconcile" && condition.Status == metav1.ConditionFalse:
			return fmt.Errorf("cluster Check Runner reconciliation error message: %s", condition.Message)
		}
	}
	return nil
}

// Run use to run the command.
func (o *Options) Run() error {
	o.printOutf("Start checking rolling-update status (timeout: %s, agent pct: %f, agent min: %d, dca min: %d, clc min: %d, skip not-Ready nodes: %t, max node lookups: %d, max skipped ratio: %f)",
		o.checkTimeout, o.agentCompletionPct, o.agentCompletionMin, o.dcaMinUpToDate, o.clcMinUpToDate, o.skipNotReadyNodes, o.maxNodeLookups, o.maxSkippedRatio)
	checkFunc := func(ctx context.Context) (bool, error) {
		var agentDone, dcaDone, ccrDone, reconcileError bool
		var status common.StatusWrapper
		o.printOutf("v2alpha1 is available")
		status, err := o.getV2Status()

		if errors.IsNotFound(err) {
			o.printOutf("Got a not found error while getting %s/%s. Assuming this DatadogAgent CR has never been deployed in this environment", o.UserNamespace, o.datadogAgentName)
			return true, nil
		} else if err != nil {
			return false, fmt.Errorf("unable to get the DatadogAgent.status, err:%w", err)
		}

		if err = isReconcileError(status.GetStatusCondition()); err != nil {
			o.printOutf("received a reconcile error: %v", err)
			reconcileError = true
		}

		ddais, err := o.getDDAIs(ctx)
		if err != nil {
			return false, err
		}
		for i := range ddais {
			if err = isReconcileError(ddais[i].Status.Conditions); err != nil {
				o.printOutf("received a reconcile error for DatadogAgentInternal %s/%s: %v", ddais[i].Namespace, ddais[i].Name, err)
				reconcileError = true
			}
		}

		if !agentDone {
			agentDone = o.isAgentDoneSkippingNotReadyNodes(ctx, status.GetAgentStatus())
		}

		if !dcaDone {
			dcaDone = o.isDeploymentDone(status.GetClusterAgentStatus(), o.dcaMinUpToDate, "Cluster Agent")
		}

		if !ccrDone {
			ccrDone = o.isDeploymentDone(status.GetClusterChecksRunnerStatus(), o.clcMinUpToDate, "Cluster Check Runner")
		}

		if agentDone && dcaDone && ccrDone && !reconcileError {
			return true, nil
		}

		o.printOutf("One or multiple components are still upgrading...")

		if status.GetAgentStatus() != nil {
			o.printOutf("[Agent] nb pods: %d, nb updated pods: %d", status.GetAgentStatus().Current, status.GetAgentStatus().UpToDate)
		}

		if status.GetClusterAgentStatus() != nil {
			o.printOutf("[Cluster Agent] nb pods: %d, nb updated pods: %d", status.GetClusterAgentStatus().Replicas, status.GetClusterAgentStatus().UpdatedReplicas)
		}

		if status.GetClusterChecksRunnerStatus() != nil {
			o.printOutf("[Cluster Check Runner] nb pods: %d, nb updated pods: %d", status.GetClusterChecksRunnerStatus().Replicas, status.GetClusterChecksRunnerStatus().UpdatedReplicas)
		}

		return false, nil
	}

	return wait.PollUntilContextTimeout(context.Background(), o.checkPeriod, o.checkTimeout, false, checkFunc)
}

func (o *Options) isAgentDone(status *v2alpha1.DaemonSetStatus) bool {
	if status == nil {
		return true
	}

	if o.reachedAgentThreshold(status) {
		o.printOutf("[Agent] upgrade is now finished (reached threshold): %d, nb updated pods: %d, threshold pct: %f, min threshold: %d", status.Current, status.UpToDate, o.agentCompletionPct, o.agentCompletionMin)

		return true
	}

	return false
}

// isAgentDoneSkippingNotReadyNodes runs isAgentDone. If it fails and skipNotReadyNodes is set, it
// runs it again without the nodes that cannot finish the rollout: existing not-Ready nodes whose
// Agent pod is out of date. Errors fall back to the plain result.
func (o *Options) isAgentDoneSkippingNotReadyNodes(ctx context.Context, status *v2alpha1.DaemonSetStatus) bool {
	if o.isAgentDone(status) {
		return true
	}
	if !o.skipNotReadyNodes {
		return false
	}

	// Skip the API calls if the check cannot pass even with the most nodes skipped.
	maxSkipped := int32(float64(status.Current) * o.maxSkippedRatio)
	bestCase := *status
	bestCase.Current -= maxSkipped
	if !o.reachedAgentThreshold(&bestCase) {
		return false
	}

	skipped, err := o.countOutdatedNotReadyNodes(ctx, maxSkipped)
	if err != nil {
		o.printOutf("[Agent] unable to count not-Ready nodes, not skipping any: %v", err)
		return false
	}
	if skipped == 0 {
		return false
	}

	adjusted := *status
	adjusted.Current -= skipped
	o.printOutf("[Agent] skipping %d not-Ready nodes with an out-of-date Agent pod", skipped)
	return o.isAgentDone(&adjusted)
}

// countOutdatedNotReadyNodes returns, summed over the Agent DaemonSets, the number of existing
// not-Ready nodes whose oldest Agent pod is out of date. It follows the DaemonSet controller, which
// counts nodes, not pods, in currentNumberScheduled and updatedNumberScheduled:
//   - only pods that are both terminal and deleted are ignored. A pod stuck in Terminating on an
//     unreachable node is still counted;
//   - a pod is up to date if its template generation or its revision hash matches;
//   - deleted nodes are not counted, as the DaemonSet status does not count them;
//   - a DaemonSet with no up-to-date pod yet is not counted, as its revision hash is unknown.
//
// It lists the out-of-date pods, then the pods of each not-Ready candidate node, instead of all
// Agent pods: a list of all Agent pods can take minutes on large clusters. It does not check node
// affinity, as the node already runs a pod of the DaemonSet. It returns 0 if more than maxSkipped
// nodes would be skipped.
func (o *Options) countOutdatedNotReadyNodes(ctx context.Context, maxSkipped int32) (int32, error) {
	daemonSets := &appsv1.DaemonSetList{}
	if err := o.Client.List(
		ctx,
		daemonSets,
		client.InNamespace(o.UserNamespace),
		client.MatchingLabels{
			apicommon.AgentDeploymentNameLabelKey:      o.datadogAgentName,
			apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		},
	); err != nil {
		return 0, fmt.Errorf("unable to list Agent DaemonSets: %w", err)
	}

	candidates := map[*appsv1.DaemonSet]map[string]struct{}{}
	nodeNames := map[string]struct{}{}
	for i := range daemonSets.Items {
		ds := &daemonSets.Items[i]
		nodes, err := o.getOutdatedPodNodes(ctx, ds)
		if err != nil {
			return 0, err
		}
		candidates[ds] = nodes
		for name := range nodes {
			nodeNames[name] = struct{}{}
		}
	}

	if len(nodeNames) > o.maxNodeLookups {
		o.printOutf("[Agent] %d nodes have an out-of-date Agent pod, above the %d node lookups cap: not skipping any", len(nodeNames), o.maxNodeLookups)
		return 0, nil
	}

	notReady := map[string]bool{}
	nodes := map[string]*corev1.Node{}
	for name := range nodeNames {
		node := &corev1.Node{}
		if err := o.Client.Get(ctx, client.ObjectKey{Name: name}, node); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return 0, fmt.Errorf("unable to get node %s: %w", name, err)
		}
		notReady[name] = !isNodeReady(node)
		nodes[name] = node
	}

	var skipped int32
	for ds, dsNodes := range candidates {
		var currentHash *string
		for name := range dsNodes {
			// The DaemonSet status does not count a node whose taints the pod does not tolerate.
			if !notReady[name] || !toleratesNodeTaints(ds, nodes[name]) {
				continue
			}
			if currentHash == nil {
				hash, err := o.getCurrentRevisionHash(ctx, ds)
				if err != nil {
					return 0, err
				}
				currentHash = &hash
			}
			// Without an up-to-date pod the current revision hash is unknown, and after a template
			// revert a pod that the DaemonSet status counts as updated would be skipped.
			if *currentHash == "" {
				break
			}
			outdated, err := o.isOldestPodOutdated(ctx, ds, name, *currentHash)
			if err != nil {
				return 0, err
			}
			if outdated {
				skipped++
			}
		}
	}

	if skipped > maxSkipped {
		o.printOutf("[Agent] %d not-Ready nodes have an out-of-date Agent pod, above the %d cap (%.0f%% of scheduled nodes): not skipping any", skipped, maxSkipped, o.maxSkippedRatio*100)
		return 0, nil
	}

	return skipped, nil
}

// getOutdatedPodNodes returns the nodes that run an out-of-date pod of the DaemonSet.
func (o *Options) getOutdatedPodNodes(ctx context.Context, ds *appsv1.DaemonSet) (map[string]struct{}, error) {
	generation, found := ds.Annotations[daemonSetTemplateGenerationAnnotation]
	if !found || ds.Spec.Selector == nil {
		return nil, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid selector on DaemonSet %s: %w", ds.Name, err)
	}
	outdatedReq, err := labels.NewRequirement(podTemplateGenerationLabel, selection.NotEquals, []string{generation})
	if err != nil {
		return nil, err
	}

	pods := &corev1.PodList{}
	if err = o.Client.List(ctx, pods, client.InNamespace(o.UserNamespace), client.MatchingLabelsSelector{Selector: selector.Add(*outdatedReq)}); err != nil {
		return nil, fmt.Errorf("unable to list out-of-date pods of DaemonSet %s: %w", ds.Name, err)
	}

	nodes := map[string]struct{}{}
	for i := range pods.Items {
		if pod := &pods.Items[i]; isCountedDaemonPod(pod, ds) {
			nodes[pod.Spec.NodeName] = struct{}{}
		}
	}
	return nodes, nil
}

// isOldestPodOutdated returns true if the oldest counted pod of the DaemonSet on the node is out of
// date. The DaemonSet controller only looks at this pod to count the node as updated.
func (o *Options) isOldestPodOutdated(ctx context.Context, ds *appsv1.DaemonSet, nodeName, currentHash string) (bool, error) {
	selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		return false, fmt.Errorf("invalid selector on DaemonSet %s: %w", ds.Name, err)
	}

	pods := &corev1.PodList{}
	if err = o.Client.List(
		ctx,
		pods,
		client.InNamespace(o.UserNamespace),
		client.MatchingLabelsSelector{Selector: selector},
		client.MatchingFields{"spec.nodeName": nodeName},
	); err != nil {
		return false, fmt.Errorf("unable to list pods of DaemonSet %s on node %s: %w", ds.Name, nodeName, err)
	}

	var oldest *corev1.Pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !isCountedDaemonPod(pod, ds) {
			continue
		}
		// Same order as the DaemonSet controller: creation time, then name.
		if oldest == nil || pod.CreationTimestamp.Before(&oldest.CreationTimestamp) ||
			(pod.CreationTimestamp.Equal(&oldest.CreationTimestamp) && pod.Name < oldest.Name) {
			oldest = pod
		}
	}
	if oldest == nil {
		return false, nil
	}

	if oldest.Labels[podTemplateGenerationLabel] == ds.Annotations[daemonSetTemplateGenerationAnnotation] {
		return false, nil
	}
	return oldest.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] != currentHash, nil
}

// getCurrentRevisionHash returns the revision hash of a pod on the current template generation, or
// "" if there is none.
func (o *Options) getCurrentRevisionHash(ctx context.Context, ds *appsv1.DaemonSet) (string, error) {
	selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("invalid selector on DaemonSet %s: %w", ds.Name, err)
	}
	currentReq, err := labels.NewRequirement(podTemplateGenerationLabel, selection.Equals, []string{ds.Annotations[daemonSetTemplateGenerationAnnotation]})
	if err != nil {
		return "", err
	}
	pods := &corev1.PodList{}
	if err = o.Client.List(ctx, pods, client.InNamespace(o.UserNamespace), client.MatchingLabelsSelector{Selector: selector.Add(*currentReq)}, client.Limit(1)); err != nil {
		return "", fmt.Errorf("unable to list current pods of DaemonSet %s: %w", ds.Name, err)
	}
	if len(pods.Items) == 0 {
		return "", nil
	}
	return pods.Items[0].Labels[appsv1.DefaultDaemonSetUniqueLabelKey], nil
}

// isCountedDaemonPod returns true if the DaemonSet controller counts this pod in its status: it is
// controlled by the DaemonSet, bound to a node, and not both terminal and deleted.
func isCountedDaemonPod(pod *corev1.Pod, ds *appsv1.DaemonSet) bool {
	if !metav1.IsControlledBy(pod, ds) || pod.Spec.NodeName == "" {
		return false
	}
	terminal := pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
	return pod.DeletionTimestamp == nil || !terminal
}

// toleratesNodeTaints returns true if a pod of the DaemonSet tolerates every NoSchedule and
// NoExecute taint of the node. Like the DaemonSet controller, it adds the default DaemonSet
// tolerations to the pod template ones.
func toleratesNodeTaints(ds *appsv1.DaemonSet, node *corev1.Node) bool {
	tolerations := append(append([]corev1.Toleration{}, ds.Spec.Template.Spec.Tolerations...), daemonSetDefaultTolerations...)
	if ds.Spec.Template.Spec.HostNetwork {
		tolerations = append(tolerations, corev1.Toleration{Key: corev1.TaintNodeNetworkUnavailable, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule})
	}
	for i := range node.Spec.Taints {
		taint := &node.Spec.Taints[i]
		if taint.Effect != corev1.TaintEffectNoSchedule && taint.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		tolerated := false
		for j := range tolerations {
			if tolerations[j].ToleratesTaint(klog.Background(), taint, false) {
				tolerated = true
				break
			}
		}
		if !tolerated {
			return false
		}
	}
	return true
}

func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (o *Options) reachedAgentThreshold(status *v2alpha1.DaemonSetStatus) bool {
	return float64(status.UpToDate) > float64(status.Current)*o.agentCompletionPct || status.Current-status.UpToDate <= o.agentCompletionMin
}

func (o *Options) isDeploymentDone(status *v2alpha1.DeploymentStatus, minUpToDate int32, component string) bool {
	if status == nil {
		return true
	}

	if status.UpdatedReplicas >= minUpToDate {
		o.printOutf("[%s] upgrade is now finished (reached threshold): %d, nb updated pods: %d, min up-to-date threshold: %d", component, status.Replicas, status.UpdatedReplicas, o.dcaMinUpToDate)

		return true
	}

	return false
}

func (o *Options) printOutf(format string, a ...any) {
	args := make([]any, 0, 3+len(a))
	args = append(args, time.Now().UTC().Format("2006-01-02T15:04:05.999Z"), o.UserNamespace, o.datadogAgentName)
	args = append(args, a...)
	_, _ = fmt.Fprintf(o.Out, "[%s] DatadogAgent '%s/%s': "+format+"\n", args...)
}
