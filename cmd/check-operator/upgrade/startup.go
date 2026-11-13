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
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const startupRequestTimeout = 5 * time.Second

// startupCollector only reads status. It never waits for readiness or changes the
// upgrade result. The job may exit before a collection finishes.
type startupCollector struct {
	options   *Options
	output    io.Writer
	snapshots map[types.UID][32]byte
	attempts  map[string]struct{}
}

type startupRecord struct {
	Image             string             `json:"image,omitempty"`
	ImageID           string             `json:"image_id,omitempty"`
	Event             string             `json:"event"`
	ObservedAt        time.Time          `json:"observed_at"`
	Namespace         string             `json:"namespace"`
	DatadogAgent      string             `json:"datadogagent"`
	Experiment        string             `json:"experiment_id,omitempty"`
	Variant           string             `json:"variant,omitempty"`
	PodName           string             `json:"pod_name,omitempty"`
	PodUID            types.UID          `json:"pod_uid,omitempty"`
	NodeName          string             `json:"node_name,omitempty"`
	DaemonSet         string             `json:"daemonset,omitempty"`
	CreationTimestamp *metav1.Time       `json:"creation_timestamp,omitempty"`
	StartTime         *metav1.Time       `json:"start_time,omitempty"`
	PodPhase          corev1.PodPhase    `json:"pod_phase,omitempty"`
	Conditions        []startupCondition `json:"conditions,omitempty"`
	InitContainers    []startupContainer `json:"init_container_statuses,omitempty"`
	Containers        []startupContainer `json:"container_statuses,omitempty"`
	Phase             string             `json:"phase,omitempty"`
	ContainerName     string             `json:"container_name,omitempty"`
	StartedAt         *metav1.Time       `json:"started_at,omitempty"`
	FinishedAt        *metav1.Time       `json:"finished_at,omitempty"`
	DurationSeconds   *float64           `json:"duration_seconds,omitempty"`
	RestartCount      int32              `json:"restart_count"`
	ExitCode          *int32             `json:"exit_code,omitempty"`
	Reason            string             `json:"reason,omitempty"`
	Error             string             `json:"error,omitempty"`
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

func startupContainers(statuses []corev1.ContainerStatus) []startupContainer {
	result := make([]startupContainer, 0, len(statuses))
	for _, status := range statuses {
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

// startStartupCollection returns immediately, and cancellation does not join the
// worker. Slow API requests or log output must not delay rollout completion.
func (o *Options) startStartupCollection() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	if !o.collectStartupTimings {
		return cancel
	}
	c := &startupCollector{options: o, output: o.ErrOut, snapshots: make(map[types.UID][32]byte), attempts: make(map[string]struct{})}
	go func() {
		ticker := time.NewTicker(o.checkPeriod)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			err := c.collect(ctx)
			if err != nil && ctx.Err() == nil {
				record := c.record("agent_startup_collection_error")
				record.Error = err.Error()
				if c.emit(record) != nil {
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
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
	// Resolve names from status, including profile-specific DaemonSets. Labels on
	// pods alone cannot establish that a pod belongs to the requested DaemonSet.
	dda := &v2alpha1.DatadogAgent{}
	if err := c.get(ctx, client.ObjectKey{Namespace: c.options.UserNamespace, Name: c.options.datadogAgentName}, dda); err != nil {
		return err
	}
	names := map[string]struct{}{}
	add := func(status *v2alpha1.DaemonSetStatus) {
		if status != nil && status.DaemonsetName != "" {
			names[status.DaemonsetName] = struct{}{}
		}
	}
	add(dda.Status.Agent)
	ddais := &v1alpha1.DatadogAgentInternalList{}
	if err := c.list(ctx, ddais, client.InNamespace(c.options.UserNamespace), client.MatchingLabels{apicommon.DatadogAgentNameLabelKey: c.options.datadogAgentName}); err != nil {
		return err
	}
	for _, ddai := range ddais.Items {
		add(ddai.Status.Agent)
	}
	seen := make(map[types.UID]struct{})
	for name := range names {
		ds := &appsv1.DaemonSet{}
		if err := c.get(ctx, client.ObjectKey{Namespace: c.options.UserNamespace, Name: name}, ds); err != nil {
			return err
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
				if err := c.collectPod(pod, name); err != nil {
					return err
				}
			}
			continuation = pods.Continue
			if continuation == "" {
				break
			}
		}
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

func (c *startupCollector) collectPod(pod *corev1.Pod, daemonset string) error {
	record := c.record("agent_startup_snapshot")
	record.PodName, record.PodUID, record.NodeName, record.DaemonSet = pod.Name, pod.UID, pod.Spec.NodeName, daemonset
	record.CreationTimestamp, record.StartTime = &pod.CreationTimestamp, pod.Status.StartTime
	record.PodPhase = pod.Status.Phase
	for _, condition := range pod.Status.Conditions {
		record.Conditions = append(record.Conditions, startupCondition{Type: condition.Type, Status: condition.Status, LastTransitionTime: condition.LastTransitionTime})
	}
	record.InitContainers, record.Containers = startupContainers(pod.Status.InitContainerStatuses), startupContainers(pod.Status.ContainerStatuses)
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
	for _, status := range pod.Status.InitContainerStatuses {
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
			if err := c.emitPodDuration(pod, daemonset, phase, pod.CreationTimestamp, condition.LastTransitionTime); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *startupCollector) emitPodDuration(pod *corev1.Pod, daemonset, phase string, start, finish metav1.Time) error {
	if start.IsZero() || finish.IsZero() || finish.Before(&start) {
		return nil
	}
	key := fmt.Sprintf("%s/%s/%s/%s", pod.UID, phase, start.UTC().Format(time.RFC3339Nano), finish.UTC().Format(time.RFC3339Nano))
	if _, ok := c.attempts[key]; ok {
		return nil
	}
	seconds := finish.Sub(start.Time).Seconds()
	record := c.record("agent_startup_measurement")
	record.PodName, record.PodUID, record.NodeName, record.DaemonSet = pod.Name, pod.UID, pod.Spec.NodeName, daemonset
	record.Phase, record.StartedAt, record.FinishedAt, record.DurationSeconds = phase, &start, &finish, &seconds
	if err := c.emit(record); err != nil {
		return err
	}
	c.attempts[key] = struct{}{}
	return nil
}
