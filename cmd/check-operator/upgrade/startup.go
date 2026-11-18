// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const startupRequestTimeout = 5 * time.Second

var startupInitContainerNames = []apicommon.AgentContainerName{
	apicommon.InitVolumeContainerName, apicommon.InitConfigContainerName,
	apicommon.SeccompSetupContainerName, apicommon.HostProfilerSeccompSetupContainerName,
}
var startupContainerNames = []apicommon.AgentContainerName{
	apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName,
	apicommon.ProcessAgentContainerName, apicommon.SecurityAgentContainerName,
	apicommon.SystemProbeContainerName, apicommon.OtelAgent, apicommon.HostProfiler,
	apicommon.AgentDataPlaneContainerName, apicommon.AgentCheckRunnerContainerName,
	apicommon.FlightRecorderContainerName, apicommon.PrivateActionRunnerContainerName,
	apicommon.UnprivilegedSingleAgentContainerName, apicommon.FIPSProxyContainerName,
}

type startupRevision struct {
	Name      string      `json:"name"`
	Hash      string      `json:"hash"`
	CreatedAt metav1.Time `json:"created_at"`
}

type startupPodOutcome struct {
	daemonset      string
	terminal       bool
	missing        bool
	nodeName       string
	currentlyReady bool
	createdAt      metav1.Time
}

// startupCollector reads status without changing the upgrade result. Optional
// observation after success shares the existing upgrade deadline.
// Repeated runs can observe the same pods; revision membership does not prove
// that an intended DDA spec has reconciled.
type startupCollector struct {
	options             *Options
	output              io.Writer
	snapshots           map[types.UID][32]byte
	attempts            map[string]struct{}
	outcomes            map[types.UID]startupPodOutcome
	expectedPods        int
	expectedByDS        map[string]int
	measurementsStarted bool
	revisionsFrozen     bool
	collectionComplete  bool
	rolloutComplete     bool
	revisions           map[string]*startupRevision
	oldestPod           *metav1.Time
	newestPod           *metav1.Time
}

type startupRecord struct {
	Image               string                      `json:"image,omitempty"`
	ImageID             string                      `json:"image_id,omitempty"`
	Event               string                      `json:"event"`
	ObservedAt          time.Time                   `json:"observed_at"`
	Namespace           string                      `json:"namespace"`
	DatadogAgent        string                      `json:"datadogagent"`
	Experiment          string                      `json:"experiment_id,omitempty"`
	Variant             string                      `json:"variant,omitempty"`
	PodName             string                      `json:"pod_name,omitempty"`
	PodUID              types.UID                   `json:"pod_uid,omitempty"`
	NodeName            string                      `json:"node_name,omitempty"`
	DaemonSet           string                      `json:"daemonset,omitempty"`
	CreationTimestamp   *metav1.Time                `json:"creation_timestamp,omitempty"`
	StartTime           *metav1.Time                `json:"start_time,omitempty"`
	PodPhase            corev1.PodPhase             `json:"pod_phase,omitempty"`
	Conditions          []startupCondition          `json:"conditions,omitempty"`
	InitContainers      []startupContainer          `json:"init_container_statuses,omitempty"`
	Containers          []startupContainer          `json:"container_statuses,omitempty"`
	Phase               string                      `json:"phase,omitempty"`
	ContainerName       string                      `json:"container_name,omitempty"`
	StartedAt           *metav1.Time                `json:"started_at,omitempty"`
	FinishedAt          *metav1.Time                `json:"finished_at,omitempty"`
	DurationSeconds     *float64                    `json:"duration_seconds,omitempty"`
	RestartCount        int32                       `json:"restart_count"`
	ExitCode            *int32                      `json:"exit_code,omitempty"`
	Reason              string                      `json:"reason,omitempty"`
	Error               string                      `json:"error,omitempty"`
	MeasurementsEnabled bool                        `json:"measurements_enabled"`
	Revision            *startupRevision            `json:"revision,omitempty"`
	Revisions           map[string]*startupRevision `json:"revisions,omitempty"`
	OldestPodCreatedAt  *metav1.Time                `json:"oldest_pod_created_at,omitempty"`
	NewestPodCreatedAt  *metav1.Time                `json:"newest_pod_created_at,omitempty"`
	ExpectedPods        int                         `json:"expected_pods,omitempty"`
	SelectedPods        int                         `json:"selected_pods,omitempty"`
	ReadyPods           int                         `json:"ready_pods,omitempty"`
	TerminalPods        int                         `json:"terminal_pods,omitempty"`
	MissingPods         int                         `json:"missing_pods,omitempty"`
	UnfinishedPods      int                         `json:"unfinished_pods,omitempty"`
	CollectionComplete  bool                        `json:"collection_complete,omitempty"`
	RolloutComplete     bool                        `json:"rollout_complete"`
	TimedOut            bool                        `json:"timed_out,omitempty"`
}

// Keep only fields needed to interpret startup timing and retries.
type startupCondition struct {
	Type               corev1.PodConditionType `json:"type"`
	Status             corev1.ConditionStatus  `json:"status"`
	LastTransitionTime metav1.Time             `json:"lastTransitionTime"`
}

type startupContainer struct {
	Name         string                `json:"name"`
	Image        string                `json:"image"`
	ImageID      string                `json:"imageID"`
	State        corev1.ContainerState `json:"state"`
	LastState    corev1.ContainerState `json:"lastState"`
	Ready        bool                  `json:"ready"`
	Started      *bool                 `json:"started,omitempty"`
	RestartCount int32                 `json:"restartCount"`
}

func startupState(state corev1.ContainerState) corev1.ContainerState {
	result := *state.DeepCopy()
	if result.Terminated != nil {
		result.Terminated.ContainerID = ""
		result.Terminated.Message = ""
	}
	if result.Waiting != nil {
		result.Waiting.Message = ""
	}
	return result
}

func startupContainers(statuses []corev1.ContainerStatus, allowed []apicommon.AgentContainerName) []startupContainer {
	result := make([]startupContainer, 0, len(statuses))
	for _, status := range statuses {
		if !slices.Contains(allowed, apicommon.AgentContainerName(status.Name)) {
			continue
		}
		result = append(result, startupContainer{
			Name: status.Name, Image: status.Image, ImageID: status.ImageID,
			State: startupState(status.State), LastState: startupState(status.LastTerminationState),
			Ready: status.Ready, Started: status.Started, RestartCount: status.RestartCount,
		})
	}
	return result
}

func (c *startupCollector) record(event string) startupRecord {
	return startupRecord{Event: event, ObservedAt: time.Now().UTC(), Namespace: c.options.UserNamespace,
		DatadogAgent: c.options.datadogAgentName, Experiment: c.options.startupExperiment, Variant: c.options.startupVariant}
}

func (c *startupCollector) emit(record startupRecord) error {
	return json.NewEncoder(c.output).Encode(record)
}

// One worker owns the collector and its output. Waiting after upgrade success is
// bounded even when an API client or output writer fails to honor cancellation.
func (o *Options) startStartupCollection(deadline time.Time) func(bool) {
	if !o.collectStartupTimings {
		return func(bool) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	initialCtx, stopInitial := context.WithCancel(ctx)
	finish := make(chan bool, 1)
	done := make(chan struct{})
	c := &startupCollector{options: o, output: o.ErrOut, snapshots: make(map[types.UID][32]byte), attempts: make(map[string]struct{}), outcomes: make(map[types.UID]startupPodOutcome)}
	go func() {
		defer close(done)
		ticker := time.NewTicker(o.checkPeriod)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			if err := c.collect(initialCtx); err != nil && initialCtx.Err() == nil {
				record := c.record("agent_startup_collection_error")
				record.Error = err.Error()
				if c.emit(record) != nil {
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case success := <-finish:
				if success {
					c.observe(ctx, deadline)
				} else {
					c.prepareFinal()
					c.finalReadAndSummary(ctx, !time.Now().Before(deadline))
				}
				return
			case <-ticker.C:
			}
		}
	}()
	return func(success bool) {
		defer cancel()
		finish <- success
		stopInitial()
		remaining := max(time.Until(deadline), 0)
		if !success {
			remaining = 0
		}
		timer := time.NewTimer(remaining + startupRequestTimeout)

		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
		}
	}
}

func (c *startupCollector) observe(ctx context.Context, deadline time.Time) {
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	c.prepareFinal()
	ticker := time.NewTicker(c.options.checkPeriod)
	defer ticker.Stop()
	for {
		err := c.collect(ctx)
		if err != nil {
			record := c.record("agent_startup_collection_error")
			record.Error = err.Error()
			if c.emit(record) != nil {
				return
			}
		}
		if err == nil && c.coverageComplete() && !c.measurementsStarted {
			c.revisionsFrozen = true
			c.measurementsStarted = true
			continue
		}

		if err == nil && c.measurementsStarted && c.coverageComplete() {
			// Confirm completion with one final bounded read before exiting.
			if err = c.collect(ctx); err == nil && c.coverageComplete() {
				c.emitSummary(false)
				return
			}
		}
		select {
		case <-ctx.Done():
			c.finalReadAndSummary(context.WithoutCancel(ctx), true)
			return
		case <-ticker.C:
		}
	}
}

// prepareFinal discards observations of intermediate revisions. Previously
// emitted snapshots remain useful diagnostics; no measurements were emitted yet.
func (c *startupCollector) prepareFinal() {
	c.revisions = nil
	c.revisionsFrozen = false
	c.measurementsStarted = false
	c.outcomes = make(map[types.UID]startupPodOutcome)
	c.oldestPod, c.newestPod = nil, nil
}

func (c *startupCollector) finalReadAndSummary(ctx context.Context, timedOut bool) {
	ctx, cancel := context.WithTimeout(ctx, startupRequestTimeout)
	defer cancel()
	// On a failed upgrade, describe current target pods without claiming a
	// completed measurement cohort. On timeout, retain the already frozen target.
	if !c.revisionsFrozen {
		c.measurementsStarted = false
	}
	if err := c.collect(ctx); err != nil {
		record := c.record("agent_startup_collection_error")
		record.Error = err.Error()
		_ = c.emit(record)
	}
	c.emitSummary(timedOut)
}

func (c *startupCollector) coverageComplete() bool {
	if !c.collectionComplete || !c.rolloutComplete {
		return false
	}
	for daemonset, expected := range c.expectedByDS {
		nodes := make(map[string]struct{})
		for _, outcome := range c.outcomes {
			if outcome.daemonset == daemonset && outcome.currentlyReady && !outcome.terminal && !outcome.missing && outcome.nodeName != "" {
				nodes[outcome.nodeName] = struct{}{}
			}
		}
		if len(nodes) < expected {
			return false
		}
	}
	return true
}

func (c *startupCollector) emitSummary(timedOut bool) {
	record := c.record("agent_startup_collection_summary")
	record.Revisions = c.revisions
	record.MeasurementsEnabled = c.measurementsStarted
	record.OldestPodCreatedAt, record.NewestPodCreatedAt = c.oldestPod, c.newestPod

	record.ExpectedPods, record.SelectedPods = c.expectedPods, len(c.outcomes)
	record.CollectionComplete, record.TimedOut = c.collectionComplete, timedOut
	record.RolloutComplete = c.coverageComplete()
	if c.expectedPods > 0 && len(c.outcomes) == 0 {
		record.Reason = "no_current_revision_pods"
	} else if !record.RolloutComplete {
		record.Reason = "current_revision_incomplete"
	}
	if c.collectionComplete && len(c.expectedByDS) == 0 {
		record.Reason = "node_agent_disabled"
	}
	for _, outcome := range c.outcomes {
		switch {

		case outcome.terminal:
			record.TerminalPods++

		case outcome.missing:
			record.MissingPods++
		case outcome.currentlyReady:
			record.ReadyPods++
		default:
			record.UnfinishedPods++
		}
	}
	_ = c.emit(record)
}

// Bound individual requests rather than the whole scan so large DaemonSets can
// be collected across multiple pages. Job completion still cancels the scan.
func (c *startupCollector) get(ctx context.Context, key client.ObjectKey, object client.Object) error {
	requestCtx, cancel := context.WithTimeout(ctx, startupRequestTimeout)
	defer cancel()
	return c.options.Client.Get(requestCtx, key, object)
}

func (c *startupCollector) list(ctx context.Context, objects client.ObjectList, opts ...client.ListOption) error {
	requestCtx, cancel := context.WithTimeout(ctx, startupRequestTimeout)
	defer cancel()
	return c.options.Client.List(requestCtx, objects, opts...)
}

func (c *startupCollector) collect(ctx context.Context) error {
	c.collectionComplete = false
	expected := 0
	expectedByDS := make(map[string]int)
	allRevisionsKnown := true
	allDaemonSetsReady := true
	// Resolve names from status, including profile-specific DaemonSets. Labels on
	// pods alone cannot establish that a pod belongs to the requested DaemonSet.
	dda := &v2alpha1.DatadogAgent{}
	if err := c.get(ctx, client.ObjectKey{Namespace: c.options.UserNamespace, Name: c.options.datadogAgentName}, dda); err != nil {
		return err
	}

	names := map[string]struct{}{}
	add := func(spec *v2alpha1.DatadogAgentSpec, status *v2alpha1.DaemonSetStatus, resource string) error {
		override := spec.Override[v2alpha1.NodeAgentComponentName]
		if override != nil && override.Disabled != nil && *override.Disabled {
			return nil
		}
		if status == nil || status.DaemonsetName == "" {
			allRevisionsKnown = false
			record := c.record("agent_startup_collection_error")
			record.Error = fmt.Sprintf("%s node Agent status is not available yet; startup targets are incomplete", resource)
			return c.emit(record)
		}
		names[status.DaemonsetName] = struct{}{}
		return nil
	}
	for _, status := range dda.Status.AgentList {
		if err := add(&dda.Spec, status, "DatadogAgent "+dda.Name+" agentList"); err != nil {
			return err
		}
	}
	ddais := &v1alpha1.DatadogAgentInternalList{}
	if err := c.list(ctx, ddais, client.InNamespace(c.options.UserNamespace), client.MatchingLabels{apicommon.DatadogAgentNameLabelKey: c.options.datadogAgentName}); err != nil {
		return err
	}
	// With DDAIs, the DDA Agent status is an aggregate and can omit the
	// DaemonSet name. Concrete targets come from the DDAIs or AgentList.
	if (dda.Status.Agent != nil && dda.Status.Agent.DaemonsetName != "") || (len(ddais.Items) == 0 && len(dda.Status.AgentList) == 0) {
		if err := add(&dda.Spec, dda.Status.Agent, "DatadogAgent "+dda.Name); err != nil {
			return err
		}
	}
	for _, ddai := range ddais.Items {
		if err := add(&ddai.Spec, ddai.Status.Agent, "DatadogAgentInternal "+ddai.Name); err != nil {
			return err
		}
	}

	seen := make(map[types.UID]struct{})
	for name := range names {
		ds := &appsv1.DaemonSet{}
		if err := c.get(ctx, client.ObjectKey{Namespace: c.options.UserNamespace, Name: name}, ds); err != nil {
			return err
		}

		revision, revisionErr := c.currentRevision(ctx, ds)
		if revisionErr != nil {
			allRevisionsKnown = false
			record := c.record("agent_startup_collection_error")
			record.DaemonSet = name
			record.Error = revisionErr.Error()
			if err := c.emit(record); err != nil {
				return err
			}
		} else {
			if c.revisions == nil {
				c.revisions = make(map[string]*startupRevision)
			}
			if previous := c.revisions[name]; c.revisionsFrozen && previous != nil && previous.Name != revision.Name {
				return fmt.Errorf("DaemonSet %s changed revision during startup collection", name)
			}
			if previous := c.revisions[name]; previous != nil && previous.Name != revision.Name {
				// Follow a rollout that reaches the DaemonSet after the checker
				// starts. Earlier revisions remain in diagnostic snapshots.
				for uid, outcome := range c.outcomes {
					if outcome.daemonset == name {
						delete(c.outcomes, uid)
					}
				}
				c.oldestPod, c.newestPod = nil, nil
			}
			c.revisions[name] = revision
		}
		expected += int(ds.Status.DesiredNumberScheduled)
		expectedByDS[name] = int(ds.Status.DesiredNumberScheduled)
		if ds.Status.ObservedGeneration < ds.Generation || ds.Status.UpdatedNumberScheduled != ds.Status.DesiredNumberScheduled || ds.Status.NumberReady < ds.Status.DesiredNumberScheduled {
			allDaemonSetsReady = false
		}
		if ds.Spec.Selector == nil {
			return fmt.Errorf("DaemonSet %s has no selector", name)
		}
		selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
		if err != nil {
			return err
		}
		continuation := ""
		for {
			pods := &corev1.PodList{}
			if err := c.list(ctx, pods, client.InNamespace(ds.Namespace), client.MatchingLabelsSelector{Selector: selector}, client.Limit(100), client.Continue(continuation)); err != nil {
				return err
			}
			for i := range pods.Items {
				if err := ctx.Err(); err != nil {
					return err
				}
				pod := &pods.Items[i]
				owner := metav1.GetControllerOf(pod)
				if owner == nil || owner.Kind != "DaemonSet" || owner.UID != ds.UID {
					continue
				}
				seen[pod.UID] = struct{}{}
				if err := c.collectPod(pod, name, revision); err != nil {
					return err
				}
			}
			continuation = pods.Continue
			if continuation == "" {
				break
			}
		}
	}
	c.expectedPods = expected
	c.expectedByDS = expectedByDS
	c.collectionComplete = allRevisionsKnown
	c.rolloutComplete = allDaemonSetsReady
	c.oldestPod, c.newestPod = nil, nil
	for uid, outcome := range c.outcomes {
		if !outcome.createdAt.IsZero() {
			if c.oldestPod == nil || outcome.createdAt.Before(c.oldestPod) {
				c.oldestPod = outcome.createdAt.DeepCopy()
			}
			if c.newestPod == nil || c.newestPod.Before(&outcome.createdAt) {
				c.newestPod = outcome.createdAt.DeepCopy()
			}
		}
		_, present := seen[uid]
		outcome.missing = !present
		c.outcomes[uid] = outcome
	}
	for uid := range c.snapshots {
		if _, ok := seen[uid]; !ok {
			delete(c.snapshots, uid)
		}
	}
	for key := range c.attempts {
		uid, _, _ := strings.Cut(key, "/")
		if _, ok := seen[types.UID(uid)]; !ok {
			delete(c.attempts, key)
		}
	}
	return nil
}

func (c *startupCollector) collectPod(pod *corev1.Pod, daemonset string, revision *startupRevision) error {
	record := c.record("agent_startup_snapshot")
	selected := revision != nil && !pod.CreationTimestamp.IsZero() && pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] == revision.Hash && !pod.CreationTimestamp.Before(&revision.CreatedAt)
	record.Revision, record.MeasurementsEnabled = revision, selected && c.measurementsStarted
	record.PodName, record.PodUID, record.NodeName, record.DaemonSet = pod.Name, pod.UID, pod.Spec.NodeName, daemonset
	record.CreationTimestamp, record.StartTime = &pod.CreationTimestamp, pod.Status.StartTime
	record.PodPhase = pod.Status.Phase
	for _, condition := range pod.Status.Conditions {
		record.Conditions = append(record.Conditions, startupCondition{Type: condition.Type, Status: condition.Status, LastTransitionTime: condition.LastTransitionTime})
	}
	record.InitContainers, record.Containers = startupContainers(pod.Status.InitContainerStatuses, startupInitContainerNames), startupContainers(pod.Status.ContainerStatuses, startupContainerNames)
	// Ignore observation time when deduplicating unchanged snapshots. Avoid
	// serializing pod specs, environment variables, or configuration contents.
	observed := record.ObservedAt
	record.ObservedAt = time.Time{}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	fingerprint := sha256.Sum256(encoded)
	record.ObservedAt = observed
	if old, ok := c.snapshots[pod.UID]; !ok || old != fingerprint {
		if err := c.emit(record); err != nil {
			return err
		}
		c.snapshots[pod.UID] = fingerprint
	}
	// Previous revisions remain visible in diagnostics, but their startup is not
	// counted in the current revision's measurements.
	if !selected {
		return nil
	}
	if c.oldestPod == nil || pod.CreationTimestamp.Before(c.oldestPod) {
		c.oldestPod = pod.CreationTimestamp.DeepCopy()
	}
	if c.newestPod == nil || c.newestPod.Before(&pod.CreationTimestamp) {
		c.newestPod = pod.CreationTimestamp.DeepCopy()
	}

	if c.outcomes == nil {
		c.outcomes = make(map[types.UID]startupPodOutcome)
	}
	outcome := c.outcomes[pod.UID]
	outcome.daemonset = daemonset
	outcome.createdAt = pod.CreationTimestamp
	outcome.nodeName = pod.Spec.NodeName
	outcome.currentlyReady = false
	outcome.missing = false
	outcome.terminal = pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded || pod.DeletionTimestamp != nil
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			outcome.currentlyReady = true
		}
	}
	c.outcomes[pod.UID] = outcome
	if !c.measurementsStarted {
		return nil
	}
	for _, status := range pod.Status.InitContainerStatuses {
		if !slices.Contains(startupInitContainerNames, apicommon.AgentContainerName(status.Name)) {
			continue
		}
		nativeSidecar := false
		for _, container := range pod.Spec.InitContainers {
			if container.Name == status.Name && container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways {
				nativeSidecar = true
				break
			}
		}
		if nativeSidecar {
			continue
		}

		// A previous attempt can survive in lastState. Preserve it in the snapshot,
		// but only derive a duration for the current completed attempt.
		terminated := status.State.Terminated
		if terminated == nil || terminated.StartedAt.IsZero() || terminated.FinishedAt.IsZero() {
			continue
		}
		seconds := terminated.FinishedAt.Sub(terminated.StartedAt.Time).Seconds()
		if seconds < 0 {
			continue
		}
		key := fmt.Sprintf("%s/%s/%d/%s/%s", pod.UID, status.Name, status.RestartCount, terminated.StartedAt.UTC().Format(time.RFC3339Nano), terminated.FinishedAt.UTC().Format(time.RFC3339Nano))
		if _, ok := c.attempts[key]; ok {
			continue
		}
		measurement := c.record("agent_startup_measurement")
		measurement.Revision, measurement.MeasurementsEnabled = revision, true
		measurement.PodName, measurement.PodUID, measurement.NodeName, measurement.DaemonSet = pod.Name, pod.UID, pod.Spec.NodeName, daemonset
		measurement.Phase, measurement.ContainerName = "init_execution", status.Name
		measurement.Image, measurement.ImageID = status.Image, status.ImageID
		measurement.StartedAt, measurement.FinishedAt, measurement.DurationSeconds = &terminated.StartedAt, &terminated.FinishedAt, &seconds
		measurement.RestartCount, measurement.ExitCode, measurement.Reason = status.RestartCount, &terminated.ExitCode, terminated.Reason
		if err := c.emit(measurement); err != nil {
			return err
		}
		c.attempts[key] = struct{}{}
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		phase := ""
		switch condition.Type {
		case corev1.PodScheduled:
			phase = "creation_to_scheduled"
		case corev1.PodInitialized:
			phase = "creation_to_initialized"
		case corev1.PodReady:
			phase = "creation_to_latest_ready"
		}
		if phase != "" {
			if err := c.emitPodDuration(pod, daemonset, phase, pod.CreationTimestamp, condition.LastTransitionTime, revision); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *startupCollector) emitPodDuration(pod *corev1.Pod, daemonset, phase string, start, finish metav1.Time, revision *startupRevision) error {
	if start.IsZero() || finish.IsZero() || finish.Before(&start) {
		return nil
	}
	key := fmt.Sprintf("%s/%s/%s/%s", pod.UID, phase, start.UTC().Format(time.RFC3339Nano), finish.UTC().Format(time.RFC3339Nano))
	if _, ok := c.attempts[key]; ok {
		return nil
	}
	seconds := finish.Sub(start.Time).Seconds()
	record := c.record("agent_startup_measurement")
	record.Revision, record.MeasurementsEnabled = revision, true
	record.PodName, record.PodUID, record.NodeName, record.DaemonSet = pod.Name, pod.UID, pod.Spec.NodeName, daemonset
	record.Phase, record.StartedAt, record.FinishedAt, record.DurationSeconds = phase, &start, &finish, &seconds
	if err := c.emit(record); err != nil {
		return err
	}
	c.attempts[key] = struct{}{}
	return nil
}

// currentRevision follows the DaemonSet controller's stored-template comparison.
// A revision's creation time is diagnostic metadata, not proof this job caused
// a new rollout: rollback can reuse history, and unchanged releases reuse pods.
func (c *startupCollector) currentRevision(ctx context.Context, ds *appsv1.DaemonSet) (*startupRevision, error) {
	revisions := &appsv1.ControllerRevisionList{}
	if err := c.list(ctx, revisions, client.InNamespace(ds.Namespace)); err != nil {
		return nil, fmt.Errorf("DaemonSet %s startup revision unavailable: %w", ds.Name, err)
	}
	var current *appsv1.ControllerRevision
	for i := range revisions.Items {
		revision := &revisions.Items[i]
		owner := metav1.GetControllerOf(revision)
		if owner == nil || owner.Kind != "DaemonSet" || owner.UID != ds.UID {
			continue
		}
		var saved struct {
			Spec struct {
				Template corev1.PodTemplateSpec `json:"template"`
			} `json:"spec"`
		}
		if err := json.Unmarshal(revision.Data.Raw, &saved); err != nil {
			return nil, fmt.Errorf("DaemonSet %s revision %s has invalid template data: %w", ds.Name, revision.Name, err)
		}
		if !apiequality.Semantic.DeepEqual(saved.Spec.Template, ds.Spec.Template) {
			continue
		}
		if current == nil || revision.Revision > current.Revision {
			current = revision
		}
	}
	if current == nil || current.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] == "" || current.CreationTimestamp.IsZero() {
		return nil, fmt.Errorf("DaemonSet %s has no owned ControllerRevision matching its current template; startup measurements are disabled", ds.Name)
	}
	return &startupRevision{Name: current.Name, Hash: current.Labels[appsv1.DefaultDaemonSetUniqueLabelKey], CreatedAt: current.CreationTimestamp}, nil
}
