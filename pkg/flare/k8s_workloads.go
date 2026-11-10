// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// maxPodsPerWorkload caps the pods listed per workload.
const maxPodsPerWorkload = 1000

var extendedDaemonSetGVK = schema.GroupVersionKind{Group: datadogGroup, Version: "v1alpha1", Kind: "ExtendedDaemonSet"}

// workload is a DaemonSet, ExtendedDaemonSet or Deployment the Operator
// manages.
type workload struct {
	kind      string
	namespace string
	name      string
	selector  labels.Selector
	rollout   map[string]any
}

// PodSummary is a pod of a managed workload in a flare.
type PodSummary struct {
	Name       string             `json:"name"`
	Node       string             `json:"node,omitempty"`
	Phase      string             `json:"phase"`
	Ready      string             `json:"ready"`
	Restarts   int32              `json:"restarts"`
	StartTime  *metav1.Time       `json:"startTime,omitempty"`
	Reason     string             `json:"reason,omitempty"`
	Containers []ContainerSummary `json:"containers,omitempty"`
}

// ContainerSummary is a container of a pod in a flare.
type ContainerSummary struct {
	Name         string `json:"name"`
	Image        string `json:"image,omitempty"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        string `json:"state,omitempty"`
	LastState    string `json:"lastState,omitempty"`
}

// WorkloadsCollector returns the collector of the workloads the Operator
// manages: the node Agent DaemonSets (or ExtendedDaemonSets), and the Cluster
// Agent and Cluster Checks Runner Deployments. For each one, it collects the
// manifest, the rollout status, its pods (phase, readiness and restarts, but
// not their logs) and its events.
func WorkloadsCollector() Collector {
	return Collector{Name: "workloads", Timeout: 45 * time.Second, Collect: collectWorkloads}
}

func collectWorkloads(ctx context.Context, c *Collection) error {
	managed := client.MatchingLabels{managedByLabel: managedByOperator}
	var workloads []workload
	listed := 0

	daemonSets := &appsv1.DaemonSetList{}
	if err := c.Env.Reader.List(ctx, daemonSets, managed, client.Limit(maxListItems)); err != nil {
		c.Warn(fmt.Errorf("unable to list DaemonSets: %w", err))
	} else {
		listed++
		for i := range daemonSets.Items {
			ds := &daemonSets.Items[i]
			cleanTyped(ds)
			c.Warn(c.AddYAML(objectPath("workloads/daemonsets", ds.Namespace, ds.Name), ds))
			selector, _ := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
			workloads = append(workloads, workload{kind: "DaemonSet", namespace: ds.Namespace, name: ds.Name, selector: selector, rollout: map[string]any{
				"desired":            ds.Status.DesiredNumberScheduled,
				"current":            ds.Status.CurrentNumberScheduled,
				"ready":              ds.Status.NumberReady,
				"available":          ds.Status.NumberAvailable,
				"unavailable":        ds.Status.NumberUnavailable,
				"updated":            ds.Status.UpdatedNumberScheduled,
				"misscheduled":       ds.Status.NumberMisscheduled,
				"generation":         ds.Generation,
				"observedGeneration": ds.Status.ObservedGeneration,
				"updateStrategy":     ds.Spec.UpdateStrategy.Type,
				"conditions":         ds.Status.Conditions,
			}})
		}
	}

	deployments := &appsv1.DeploymentList{}
	if err := c.Env.Reader.List(ctx, deployments, managed, client.Limit(maxListItems)); err != nil {
		c.Warn(fmt.Errorf("unable to list Deployments: %w", err))
	} else {
		listed++
		for i := range deployments.Items {
			deploy := &deployments.Items[i]
			cleanTyped(deploy)
			c.Warn(c.AddYAML(objectPath("workloads/deployments", deploy.Namespace, deploy.Name), deploy))
			selector, _ := metav1.LabelSelectorAsSelector(deploy.Spec.Selector)
			workloads = append(workloads, workload{kind: "Deployment", namespace: deploy.Namespace, name: deploy.Name, selector: selector, rollout: map[string]any{
				"replicas":           deploy.Status.Replicas,
				"ready":              deploy.Status.ReadyReplicas,
				"available":          deploy.Status.AvailableReplicas,
				"unavailable":        deploy.Status.UnavailableReplicas,
				"updated":            deploy.Status.UpdatedReplicas,
				"generation":         deploy.Generation,
				"observedGeneration": deploy.Status.ObservedGeneration,
				"conditions":         deploy.Status.Conditions,
			}})
		}
	}

	eds, found, _, err := listUnstructured(ctx, c.Env.Reader, extendedDaemonSetGVK, managed)
	if err != nil {
		c.Warn(fmt.Errorf("unable to list ExtendedDaemonSets: %w", err))
	} else if found {
		for i := range eds {
			obj := cleanObject(eds[i].Object)
			c.Warn(c.AddYAML(objectPath("workloads/extendeddaemonsets", eds[i].GetNamespace(), eds[i].GetName()), obj))
			status, _, _ := unstructured.NestedMap(obj, "status")
			selector := labels.Nothing()
			if rawSelector, ok, _ := unstructured.NestedMap(obj, "spec", "selector"); ok {
				ls := &metav1.LabelSelector{}
				if convErr := runtime.DefaultUnstructuredConverter.FromUnstructured(rawSelector, ls); convErr == nil {
					selector, _ = metav1.LabelSelectorAsSelector(ls)
				}
			}
			workloads = append(workloads, workload{kind: "ExtendedDaemonSet", namespace: eds[i].GetNamespace(), name: eds[i].GetName(), selector: selector, rollout: status})
		}
	}

	if listed == 0 {
		return fmt.Errorf("unable to list the managed workloads")
	}

	rollouts := map[string]any{}
	for _, w := range workloads {
		key := fmt.Sprintf("%s %s/%s", w.kind, w.namespace, w.name)
		rollouts[key] = w.rollout
		podNames := collectWorkloadPods(ctx, c, w)
		collectWorkloadEvents(ctx, c, w, podNames)
	}
	if len(workloads) == 0 {
		return c.AddText("workloads/NONE.txt", []byte("No workload managed by the Datadog Operator found.\n"))
	}
	return c.AddYAML("workloads/rollout-status.yaml", rollouts)
}

func collectWorkloadPods(ctx context.Context, c *Collection, w workload) map[string]struct{} {
	podNames := map[string]struct{}{}
	if w.selector == nil || w.selector.Empty() {
		return podNames
	}
	pods := &corev1.PodList{}
	if err := c.Env.Reader.List(ctx, pods, client.InNamespace(w.namespace), client.MatchingLabelsSelector{Selector: w.selector}, client.Limit(maxPodsPerWorkload)); err != nil {
		c.Warn(fmt.Errorf("unable to list the pods of %s %s/%s: %w", w.kind, w.namespace, w.name, err))
		return podNames
	}
	summaries := make([]PodSummary, 0, len(pods.Items))
	for i := range pods.Items {
		podNames[pods.Items[i].Name] = struct{}{}
		summaries = append(summaries, SummarizePod(&pods.Items[i]))
	}
	if len(pods.Items) >= maxPodsPerWorkload {
		c.Warn(fmt.Errorf("only the first %d pods of %s %s/%s were collected", maxPodsPerWorkload, w.kind, w.namespace, w.name))
	}
	c.Warn(c.AddYAML(path.Join("workloads/pods", safeName(w.namespace), strings.ToLower(w.kind)+"-"+safeName(w.name)+".yaml"), summaries))
	return podNames
}

func collectWorkloadEvents(ctx context.Context, c *Collection, w workload, podNames map[string]struct{}) {
	events, err := listEvents(ctx, c.Env, w.namespace, nil, func(e *corev1.Event) bool {
		switch e.InvolvedObject.Kind {
		case w.kind:
			return e.InvolvedObject.Name == w.name
		case "Pod":
			_, ok := podNames[e.InvolvedObject.Name]
			return ok
		case "ReplicaSet":
			return strings.HasPrefix(e.InvolvedObject.Name, w.name+"-")
		}
		return false
	})
	if err != nil {
		c.Warn(fmt.Errorf("unable to list the events of %s %s/%s: %w", w.kind, w.namespace, w.name, err))
		return
	}
	if len(events) == 0 {
		return
	}
	c.Warn(c.AddYAML(path.Join("workloads/events", safeName(w.namespace), strings.ToLower(w.kind)+"-"+safeName(w.name)+".yaml"), events))
}

// SummarizePod returns the phase, readiness and restarts of a pod.
func SummarizePod(pod *corev1.Pod) PodSummary {
	summary := PodSummary{
		Name:      pod.Name,
		Node:      pod.Spec.NodeName,
		Phase:     string(pod.Status.Phase),
		StartTime: pod.Status.StartTime,
		Reason:    pod.Status.Reason,
	}
	images := map[string]string{}
	for _, container := range pod.Spec.Containers {
		images[container.Name] = container.Image
	}
	ready := 0
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			ready++
		}
		summary.Restarts += status.RestartCount
		summary.Containers = append(summary.Containers, ContainerSummary{
			Name:         status.Name,
			Image:        images[status.Name],
			Ready:        status.Ready,
			RestartCount: status.RestartCount,
			State:        containerState(status.State),
			LastState:    containerState(status.LastTerminationState),
		})
	}
	summary.Ready = fmt.Sprintf("%d/%d", ready, len(pod.Spec.Containers))
	return summary
}

func containerState(state corev1.ContainerState) string {
	switch {
	case state.Waiting != nil:
		return strings.TrimSpace("waiting " + state.Waiting.Reason)
	case state.Terminated != nil:
		return fmt.Sprintf("terminated %s (exit code %d) at %s", state.Terminated.Reason, state.Terminated.ExitCode, state.Terminated.FinishedAt.UTC().Format(time.RFC3339))
	case state.Running != nil:
		return "running since " + state.Running.StartedAt.UTC().Format(time.RFC3339)
	}
	return ""
}
