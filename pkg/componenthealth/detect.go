// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package componenthealth contains the pure detection logic that turns the
// Kubernetes status of the managed cluster-level components (the Datadog
// Cluster Agent and Cluster Check Runners) into health issues. It is kept free
// of controller-runtime types so it can be unit-tested in isolation.
package componenthealth

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

// Severity mirrors the health-platform severity levels.
type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
)

// Issue types (snake_case), aligned with the health-platform issue_type
// convention so the backend can group instances of the same type.
const (
	IssueOOMKilled        = "component_oomkilled"
	IssueCrashLooping     = "component_crash_looping"
	IssueUnschedulable    = "component_unschedulable"
	IssueImagePullFailure = "image_pull_failure"
)

const (
	reasonOOMKilled        = "OOMKilled"
	reasonCrashLoopBackOff = "CrashLoopBackOff"
	reasonHighRestartCount = "HighRestartCount"
)

var imagePullReasons = map[string]struct{}{
	"ImagePullBackOff": {},
	"ErrImagePull":     {},
}

// DetectedIssue is a single health problem detected on one managed component
// pod. It is the output of per-pod detection; the controller aggregates these
// into component-level ComponentIssues before reporting.
type DetectedIssue struct {
	Component string // cluster-agent | cluster-checks-runner
	IssueType string
	Severity  Severity
	Namespace string
	PodName   string
	Container string // container the issue relates to, empty for pod-level issues
	Reason    string // raw Kubernetes reason (OOMKilled, CrashLoopBackOff, Unschedulable, ...)
	Message   string // human-readable detail
}

// ComponentIssue is a health issue reported at the component (deployment) level:
// the same IssueType affecting one or more pods of a component is a single issue
// instance. This is the unit the operator reports because it has stable identity
// across pod churn — pod names are ephemeral (a restart mints a new name), but
// the component is not. The affected pods are carried as detail.
//
// It is emitter-agnostic: the caller decides how to report it (log, metric, or a
// health-platform payload sent to the backend).
type ComponentIssue struct {
	Component    string // cluster-agent | cluster-checks-runner
	IssueType    string
	Severity     Severity
	Namespace    string
	AffectedPods []string // names of the pods currently exhibiting the issue
}

// ManagedComponent returns the cluster-level component name for a pod, or "" if
// the pod is not one of the managed DCA/CLC components. Node Agents are
// intentionally excluded (see the engineering brief: they are node-level, high
// pod count, and already run the health platform themselves).
func ManagedComponent(pod *corev1.Pod) string {
	switch pod.Labels[common.AgentDeploymentComponentLabelKey] {
	case constants.DefaultClusterAgentResourceSuffix:
		return constants.DefaultClusterAgentResourceSuffix
	case constants.DefaultClusterChecksRunnerResourceSuffix:
		return constants.DefaultClusterChecksRunnerResourceSuffix
	default:
		return ""
	}
}

// DetectPodIssues inspects a managed component pod's status and returns the
// health issues it exhibits. restartThreshold is the cumulative RestartCount at
// or above which a container is flagged as crash-looping even when it is not
// currently in CrashLoopBackOff; a value <= 0 disables that heuristic.
//
// It returns nil for pods that are not managed DCA/CLC components.
func DetectPodIssues(pod *corev1.Pod, restartThreshold int32) []DetectedIssue {
	component := ManagedComponent(pod)
	if component == "" {
		return nil
	}

	var issues []DetectedIssue
	add := func(issueType string, sev Severity, container, reason, msg string) {
		issues = append(issues, DetectedIssue{
			Component: component,
			IssueType: issueType,
			Severity:  sev,
			Namespace: pod.Namespace,
			PodName:   pod.Name,
			Container: container,
			Reason:    reason,
			Message:   msg,
		})
	}

	// Scheduling failure: the pod is Pending and the scheduler reported it as
	// Unschedulable (insufficient resources, taints, affinity, ...).
	if pod.Status.Phase == corev1.PodPending {
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse && c.Reason == corev1.PodReasonUnschedulable {
				add(IssueUnschedulable, SeverityHigh, "", c.Reason, c.Message)
				break
			}
		}
	}

	// Container-level signals: inspect both regular and init containers.
	statuses := make([]corev1.ContainerStatus, 0, len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	for _, cs := range statuses {
		// OOMKilled — check the last termination first (the definitive "ran out
		// of memory" signal, still visible after the container restarted), then
		// the current terminated state.
		if term := oomTermination(cs); term != nil {
			add(IssueOOMKilled, SeverityHigh, cs.Name, reasonOOMKilled,
				fmt.Sprintf("container %q was OOMKilled (exit code %d)", cs.Name, term.ExitCode))
		}

		if w := cs.State.Waiting; w != nil {
			switch {
			case w.Reason == reasonCrashLoopBackOff:
				add(IssueCrashLooping, SeverityHigh, cs.Name, w.Reason,
					fmt.Sprintf("container %q is crash looping: %s", cs.Name, w.Message))
			case isImagePullReason(w.Reason):
				add(IssueImagePullFailure, SeverityMedium, cs.Name, w.Reason,
					fmt.Sprintf("container %q cannot pull its image: %s", cs.Name, w.Message))
			}
		}

		// Sustained restarts even when the container is not currently backing
		// off. Skip if we already flagged this container as crash-looping.
		if restartThreshold > 0 && cs.RestartCount >= restartThreshold &&
			!hasIssueForContainer(issues, IssueCrashLooping, cs.Name) {
			add(IssueCrashLooping, SeverityHigh, cs.Name, reasonHighRestartCount,
				fmt.Sprintf("container %q has restarted %d times", cs.Name, cs.RestartCount))
		}
	}

	return issues
}

// oomTermination returns the terminated state of a container that was OOMKilled,
// preferring the last-termination record over the current state, or nil.
func oomTermination(cs corev1.ContainerStatus) *corev1.ContainerStateTerminated {
	if t := cs.LastTerminationState.Terminated; t != nil && t.Reason == reasonOOMKilled {
		return t
	}
	if t := cs.State.Terminated; t != nil && t.Reason == reasonOOMKilled {
		return t
	}
	return nil
}

func isImagePullReason(reason string) bool {
	_, ok := imagePullReasons[reason]
	return ok
}

func hasIssueForContainer(issues []DetectedIssue, issueType, container string) bool {
	for _, i := range issues {
		if i.IssueType == issueType && i.Container == container {
			return true
		}
	}
	return false
}
