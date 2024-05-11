// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonsetreplicaset

import (
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/scheduler"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/strategy"
	podutils "github.com/DataDog/extendeddaemonset/pkg/controller/utils/pod"
)

// Deprecated: This flag is deprecated and will be removed in a subsequent version.
var ignoreEvictedPods = false

func init() {
	pflag.BoolVarP(&ignoreEvictedPods, "ignoreEvictedPods", "i", ignoreEvictedPods, "Enabling this will force new pods creation on nodes where pods are evicted")
}

// FilterAndMapPodsByNode is used to map pods by associated node. It also returns the list of pods that
// should be deleted (not needed anymore), and pods that are not scheduled yet (created but not scheduled).
func (r *Reconciler) FilterAndMapPodsByNode(
	logger logr.Logger, replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, nodeList *strategy.NodeList, podList *corev1.PodList, ignoreNodes []string,
) (
	nodesByName map[string]*strategy.NodeItem, podsByNode map[*strategy.NodeItem]*corev1.Pod, podsToDelete, unscheduledPods []*corev1.Pod,
) {
	// For faster search convert nodes to ignore from a slice to a map
	ignoreMapNode := make(map[string]bool)
	for _, name := range ignoreNodes {
		ignoreMapNode[name] = true
	}

	// Create a fake pod from the current replicaset.spec.template
	// Use this pod to check fitness of nodes in nodeList
	newPod, _ := podutils.CreatePodFromDaemonSetReplicaSet(nil, replicaset, nil, nil, false)
	podsByNodeName := make(map[string][]*corev1.Pod)
	nodesByName = make(map[string]*strategy.NodeItem)
	for id := range nodeList.Items {
		nodeItem := nodeList.Items[id]
		nodesByName[nodeItem.Node.Name] = nodeItem
		if _, ok := ignoreMapNode[nodeItem.Node.Name]; ok {
			continue
		}
		// Populate podsByNodeName with nodes that are deemed schedulable
		if scheduler.CheckNodeFitness(logger.WithValues("filter", "FilterAndMapPodsByNode"), newPod, nodeItem.Node) {
			podsByNodeName[nodeItem.Node.Name] = nil
		} else {
			logger.V(1).Info("CheckNodeFitness not ok", "reason", "DeletionTimestamp==nil", "node.Name", nodeItem.Node.Name)
		}
	}

	// Associate Pods to Nodes
	for id, pod := range podList.Items {
		nodeName, err := podutils.GetNodeNameFromPod(&pod)
		if err != nil {
			continue
		}

		// Ignore pods with status phase Unknown: usually it means the pod's node
		// in unreacheable so the pod can't be deleted. It will be cleaned up by the
		// pods garbage collector.
		if pod.Status.Phase == corev1.PodUnknown {
			continue
		}

		if _, ok := podsByNodeName[nodeName]; ok {
			if pod.Status.Phase == corev1.PodFailed {
				if r.shouldDeleteFailedPod(replicaset, nodeName) {
					podsToDelete = append(podsToDelete, &podList.Items[id])
					logger.Info("Failed pod is marked for deletion", "pod.Namespace", pod.Namespace, "pod.Name", pod.Name, "nodeName", nodeName)

					continue
				} else {
					logger.V(1).Info("Failed pod deletion has been limited by backoff", pod.Namespace, "pod.Name", pod.Name, "nodeName", nodeName)
				}
			}
			podsByNodeName[nodeName] = append(podsByNodeName[nodeName], &podList.Items[id])

			if _, scheduled := podutils.IsPodScheduled(&pod); !scheduled {
				unscheduledPods = append(unscheduledPods, &podList.Items[id])
			}
		} else {
			if _, ok := ignoreMapNode[nodeName]; ok {
				continue
			}

			// Add pod with missing Node in podsToDelete slice
			// Skip pod with DeletionTimestamp already set
			if pod.DeletionTimestamp == nil {
				podsToDelete = append(podsToDelete, &podList.Items[id])
				logger.V(1).Info("PodToDelete", "reason", "DeletionTimestamp==nil", "pod.Name", pod.Name, "node.Name", nodeName)
			}
		}
	}

	// Filter pod node, remove duplicated
	var duplicatedPods []*corev1.Pod
	podsByNode, duplicatedPods = FilterPodsByNode(podsByNodeName, nodesByName)

	// Add duplicated pods to the pod deletion slice
	for _, pod := range duplicatedPods {
		nodeName, _ := podutils.GetNodeNameFromPod(pod)
		logger.V(1).Info("PodToDelete", "reason", "duplicatedPod", "pod.Name", pod.Name, "node.Name", nodeName)
	}
	podsToDelete = append(podsToDelete, duplicatedPods...)

	// Filter Pods in Terminated state
	return nodesByName, podsByNode, podsToDelete, unscheduledPods
}

func (r *Reconciler) shouldDeleteFailedPod(replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, nodeName string) bool {
	key := getBackOffKey(replicaset, nodeName)
	now := r.failedPodsBackOff.Clock.Now()
	inBackOff := r.failedPodsBackOff.IsInBackOffSinceUpdate(key, now)
	if inBackOff {
		return false
	}
	r.failedPodsBackOff.Next(key, now)

	return true
}

func getBackOffKey(replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, nodeName string) string {
	return fmt.Sprintf("%s/%s/%s", replicaset.UID, replicaset.Name, nodeName)
}

// FilterPodsByNode if several Pods are listed for the same Node select "best" Pod one, and add other pod to
// the deletion pod slice.
func FilterPodsByNode(podsByNodeName map[string][]*corev1.Pod, nodesMap map[string]*strategy.NodeItem) (map[*strategy.NodeItem]*corev1.Pod, []*corev1.Pod) {
	// Filter pod node, remove duplicated
	podByNodeName := map[*strategy.NodeItem]*corev1.Pod{}
	duplicatedPods := []*corev1.Pod{}
	for node, pods := range podsByNodeName {
		podByNodeName[nodesMap[node]] = nil
		sort.Sort(sortPodByNodeName(pods))
		for id := range pods {
			if id == 0 {
				podByNodeName[nodesMap[node]] = pods[id]
			} else {
				duplicatedPods = append(duplicatedPods, pods[id])
			}
		}
	}

	return podByNodeName, duplicatedPods
}

type sortPodByNodeName []*corev1.Pod

func (o sortPodByNodeName) Len() int      { return len(o) }
func (o sortPodByNodeName) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o sortPodByNodeName) Less(i, j int) bool {
	// Scheduled Pod first
	if len(o[i].Spec.NodeName) != 0 && len(o[j].Spec.NodeName) == 0 {
		return true
	}

	if len(o[i].Spec.NodeName) == 0 && len(o[j].Spec.NodeName) != 0 {
		return false
	}

	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}

	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}
