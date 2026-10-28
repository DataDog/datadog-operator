// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

// podTemplateGenerationLabel is the DaemonSet template generation that Kubernetes sets on the pods.
const podTemplateGenerationLabel = "pod-template-generation"

// Options provides information required to manage canary.
type Options struct {
	genericclioptions.IOStreams
	common.Options
	args               []string
	datadogAgentName   string
	checkPeriod        time.Duration
	checkTimeout       time.Duration
	agentCompletionPct float64
	agentCompletionMin int
	dcaMinUpToDate     int32
	clcMinUpToDate     int32
	skipNotReadyNodes  bool
	maxSkippedPct      float64
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
		maxSkippedPct:      10,
	}

	opts.SetConfigFlags()

	if val, found := os.LookupEnv("AGENT_COMPLETION_PCT"); found {
		if iVal, err := strconv.ParseFloat(val, 64); err == nil {
			opts.agentCompletionPct = iVal / 100
		}
	}

	if val, found := os.LookupEnv("AGENT_COMPLETION_MIN"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.agentCompletionMin = int(iVal)
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

	if val, found := os.LookupEnv("AGENT_SKIP_NOT_READY_MAX_PCT"); found {
		if fVal, err := strconv.ParseFloat(val, 64); err == nil && fVal >= 0 && fVal <= 100 {
			opts.maxSkippedPct = fVal
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
		if apierrors.IsNotFound(err) {

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
	o.printOutf("Start checking rolling-update status (timeout: %s, agent pct: %f, agent min: %d, dca min: %d, clc min: %d, skip not-Ready nodes: %t, max skipped pct: %.0f)",
		o.checkTimeout, o.agentCompletionPct, o.agentCompletionMin, o.dcaMinUpToDate, o.clcMinUpToDate, o.skipNotReadyNodes, o.maxSkippedPct)
	checkFunc := func(ctx context.Context) (bool, error) {
		var agentDone, dcaDone, ccrDone, reconcileError bool
		var status common.StatusWrapper
		o.printOutf("v2alpha1 is available")
		status, err := o.getV2Status()

		if apierrors.IsNotFound(err) {
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

	if o.reachedAgentThreshold(int(status.Current), int(status.UpToDate)) {
		o.printOutf("[Agent] upgrade is now finished (reached threshold): %d, nb updated pods: %d, threshold pct: %f, min threshold: %d", status.Current, status.UpToDate, o.agentCompletionPct, o.agentCompletionMin)

		return true
	}

	return false
}

// errNoDiscount ends the not-Ready node discount without an error: the check uses the plain threshold.
var errNoDiscount = errors.New("not skipping not-Ready nodes")

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

	current, upToDate, skipped, err := o.getNotReadyNodeDiscount(ctx, status)
	if errors.Is(err, errNoDiscount) {
		o.printOutf("[Agent] %v", err)
		return false
	}
	if err != nil {
		o.printOutf("[Agent] unable to count not-Ready nodes, not skipping any: %v", err)
		return false
	}

	o.printOutf("[Agent] skipping %d not-Ready nodes with an out-of-date Agent pod, counting %d nodes instead of %d", len(skipped), current-len(skipped), current)
	return o.isAgentDone(&v2alpha1.DaemonSetStatus{Current: int32(current - len(skipped)), UpToDate: int32(upToDate)})
}

// getNotReadyNodeDiscount returns the live Agent node counts and the "<DaemonSet>/<node>" pairs to
// remove from Current.
func (o *Options) getNotReadyNodeDiscount(ctx context.Context, status *v2alpha1.DaemonSetStatus) (current, upToDate int, skipped []string, err error) {
	// Free test on the DatadogAgent status first, so that polls make no API call early in a rollout.
	if _, _, err = o.getSkipBounds(int(status.Current), int(status.UpToDate)); err != nil {
		return 0, 0, nil, err
	}

	daemonSets, err := o.getAgentDaemonSets(ctx)
	if err != nil {
		return 0, 0, nil, err
	}
	// The DatadogAgent status is copied from the DaemonSets at reconcile time and can lag behind
	// them, so the discount uses the live DaemonSet status.
	for i := range daemonSets {
		current += int(daemonSets[i].Status.CurrentNumberScheduled)
		upToDate += int(daemonSets[i].Status.UpdatedNumberScheduled)
	}
	minSkipped, maxSkipped, err := o.getSkipBounds(current, upToDate)
	if err != nil {
		return 0, 0, nil, err
	}

	pods, err := o.getAgentPods(ctx)
	if err != nil {
		return 0, 0, nil, err
	}
	nodes, err := o.getNodes(ctx)
	if err != nil {
		return 0, 0, nil, err
	}
	skipped, err = computeNotReadyNodeDiscount(daemonSets, pods, nodes, minSkipped, maxSkipped)
	if err != nil {
		return 0, 0, nil, err
	}
	return current, upToDate, skipped, nil
}

// getSkipBounds returns the fewest nodes to skip for the Agent threshold to pass, and the most nodes
// the check can skip.
func (o *Options) getSkipBounds(current, upToDate int) (minSkipped, maxSkipped int, err error) {
	maxSkipped = int(float64(current) * o.maxSkippedPct / 100)
	for minSkipped = 1; minSkipped <= maxSkipped; minSkipped++ {
		if o.reachedAgentThreshold(current-minSkipped, upToDate) {
			return minSkipped, maxSkipped, nil
		}
	}
	return 0, 0, fmt.Errorf("%w: %d/%d nodes up to date, not enough even with %d nodes skipped", errNoDiscount, upToDate, current, maxSkipped)
}

// getAgentDaemonSets returns the Agent DaemonSets of the DatadogAgent, if all their statuses match
// their spec and count every node that runs one of their pods.
func (o *Options) getAgentDaemonSets(ctx context.Context) ([]appsv1.DaemonSet, error) {
	dsList := &appsv1.DaemonSetList{}
	if err := o.Client.List(ctx, dsList, client.InNamespace(o.UserNamespace), o.agentLabels()); err != nil {
		return nil, fmt.Errorf("unable to list Agent DaemonSets: %w", err)
	}
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		if ds.DeletionTimestamp != nil || ds.Status.ObservedGeneration != ds.Generation {
			return nil, fmt.Errorf("%w: the status of DaemonSet %s does not match its spec yet", errNoDiscount, ds.Name)
		}
		// A misscheduled node runs a pod but is not in Current, for example a not-Ready node with a
		// taint that the Agent does not tolerate. It must not be removed from Current again.
		if ds.Status.NumberMisscheduled > 0 {
			return nil, fmt.Errorf("%w: DaemonSet %s has %d misscheduled nodes", errNoDiscount, ds.Name, ds.Status.NumberMisscheduled)
		}
	}
	return dsList.Items, nil
}

// getAgentPods returns the pods of the Agent DaemonSets of the DatadogAgent.
func (o *Options) getAgentPods(ctx context.Context) ([]corev1.Pod, error) {
	pods := &corev1.PodList{}
	if err := o.Client.List(ctx, pods, client.InNamespace(o.UserNamespace), o.agentLabels(), fromWatchCache()); err != nil {
		return nil, fmt.Errorf("unable to list Agent pods: %w", err)
	}
	return pods.Items, nil
}

func (o *Options) getNodes(ctx context.Context) ([]corev1.Node, error) {
	nodes := &corev1.NodeList{}
	if err := o.Client.List(ctx, nodes, fromWatchCache()); err != nil {
		return nil, fmt.Errorf("unable to list nodes: %w", err)
	}
	return nodes.Items, nil
}

func (o *Options) agentLabels() client.MatchingLabels {
	return client.MatchingLabels{
		apicommon.AgentDeploymentNameLabelKey:      o.datadogAgentName,
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
	}
}

// computeNotReadyNodeDiscount returns the "<DaemonSet>/<node>" pairs that the DaemonSet status counts
// in Current but not in UpToDate, on not-Ready nodes. It returns errNoDiscount if fewer than
// minSkipped or more than maxSkipped pairs qualify.
func computeNotReadyNodeDiscount(daemonSets []appsv1.DaemonSet, pods []corev1.Pod, nodes []corev1.Node, minSkipped, maxSkipped int) ([]string, error) {
	notReadyNodes := map[string]bool{}
	for i := range nodes {
		notReadyNodes[nodes[i].Name] = !isNodeReady(&nodes[i])
	}
	daemonSetsByUID := map[types.UID]*appsv1.DaemonSet{}
	for i := range daemonSets {
		daemonSetsByUID[daemonSets[i].UID] = &daemonSets[i]
	}

	// Group the pods by DaemonSet and node, and read the current revision hash of each DaemonSet.
	type dsNode struct {
		ds   *appsv1.DaemonSet
		node string
	}
	podsByDSNode := map[dsNode][]*corev1.Pod{}
	currentHashes := map[*appsv1.DaemonSet]string{}
	for i := range pods {
		pod := &pods[i]
		owner := metav1.GetControllerOf(pod)
		if owner == nil || !isCountedPod(pod) {
			continue
		}
		ds, found := daemonSetsByUID[owner.UID]
		if !found {
			continue
		}
		key := dsNode{ds: ds, node: pod.Spec.NodeName}
		podsByDSNode[key] = append(podsByDSNode[key], pod)
		if pod.Labels[podTemplateGenerationLabel] == ds.Annotations[appsv1.DeprecatedTemplateGeneration] {
			currentHashes[ds] = pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey]
		}
	}

	var skipped []string
	for key, nodePods := range podsByDSNode {
		currentHash := currentHashes[key.ds]
		switch {
		// The status counts a node with several pods by its oldest one. This is rare with maxSurge 0,
		// and not skipping these nodes is the safe side.
		case len(nodePods) != 1:
		// Unknown hash: after a template revert, a pod that the status counts as updated would be skipped.
		case currentHash == "":
		case nodePods[0].Labels[podTemplateGenerationLabel] == key.ds.Annotations[appsv1.DeprecatedTemplateGeneration],
			nodePods[0].Labels[appsv1.DefaultDaemonSetUniqueLabelKey] == currentHash:
		// Ready and deleted nodes.
		case !notReadyNodes[key.node]:
		default:
			skipped = append(skipped, key.ds.Name+"/"+key.node)
		}
	}
	slices.Sort(skipped)

	if len(skipped) < minSkipped {
		return nil, fmt.Errorf("%w: %d not-Ready nodes with an out-of-date Agent pod, %d needed", errNoDiscount, len(skipped), minSkipped)
	}
	if len(skipped) > maxSkipped {
		return nil, fmt.Errorf("%w: %d not-Ready nodes with an out-of-date Agent pod, above the %d cap", errNoDiscount, len(skipped), maxSkipped)
	}
	return skipped, nil
}

// fromWatchCache serves a List from the API server watch cache, not etcd. Before Kubernetes 1.31, a
// List with a selector otherwise reads every object of its type from etcd. It is built for each call,
// as the client writes the request options into Raw.
func fromWatchCache() client.ListOption {
	return &client.ListOptions{Raw: &metav1.ListOptions{ResourceVersion: "0"}}
}

// isCountedPod returns true if the pod is bound to a node, and not both terminal and deleted. The
// controller also counts unbound pods; ignoring them skips fewer nodes.
func isCountedPod(pod *corev1.Pod) bool {
	if pod.Spec.NodeName == "" {
		return false
	}
	terminal := pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
	return pod.DeletionTimestamp == nil || !terminal
}

func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (o *Options) reachedAgentThreshold(current, upToDate int) bool {
	return float64(upToDate) > float64(current)*o.agentCompletionPct || current-upToDate <= o.agentCompletionMin
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
