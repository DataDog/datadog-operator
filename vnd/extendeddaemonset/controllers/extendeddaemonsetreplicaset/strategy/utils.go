// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package strategy

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilserrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/conditions"
	podaffinity "github.com/DataDog/extendeddaemonset/pkg/controller/utils/affinity"
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils/comparison"
	podutils "github.com/DataDog/extendeddaemonset/pkg/controller/utils/pod"
)

func compareCurrentPodWithNewPod(params *Parameters, pod *corev1.Pod, node *NodeItem) bool {
	// check that the pod corresponds to the replicaset. if not return false
	if !compareSpecTemplateMD5Hash(params.Replicaset.Spec.TemplateGeneration, pod) {
		return false
	}
	if !compareWithExtendedDaemonsetSettingOverwrite(pod, node) {
		return false
	}
	if !compareNodeResourcesOverwriteMD5Hash(params.EDSName, params.Replicaset, pod, node) {
		return false
	}

	return true
}

func compareNodeResourcesOverwriteMD5Hash(edsName string, replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, pod *corev1.Pod, node *NodeItem) bool {
	nodeHash := comparison.GenerateHashFromEDSResourceNodeAnnotation(replicaset.Namespace, edsName, node.Node.GetAnnotations())
	if val, ok := pod.Annotations[datadoghqv1alpha1.MD5NodeExtendedDaemonSetAnnotationKey]; !ok && nodeHash == "" || ok && val == nodeHash {
		return true
	}

	return false
}

func compareWithExtendedDaemonsetSettingOverwrite(pod *corev1.Pod, node *NodeItem) bool {
	if node.ExtendedDaemonsetSetting != nil {
		specCopy := pod.Spec.DeepCopy()
		for id, container := range specCopy.Containers {
			for _, container2 := range node.ExtendedDaemonsetSetting.Spec.Containers {
				if container.Name == container2.Name {
					for key, val := range container2.Resources.Limits {
						if specCopy.Containers[id].Resources.Limits == nil {
							specCopy.Containers[id].Resources.Limits = make(corev1.ResourceList)
						}
						specCopy.Containers[id].Resources.Limits[key] = val
					}
					for key, val := range container2.Resources.Requests {
						if specCopy.Containers[id].Resources.Requests == nil {
							specCopy.Containers[id].Resources.Requests = make(corev1.ResourceList)
						}
						specCopy.Containers[id].Resources.Requests[key] = val
					}

					break
				}
			}
		}
		if !apiequality.Semantic.DeepEqual(&pod.Spec, specCopy) {
			return false
		}
	}

	return true
}

func compareSpecTemplateMD5Hash(hash string, pod *corev1.Pod) bool {
	if val, ok := pod.Annotations[datadoghqv1alpha1.MD5ExtendedDaemonSetAnnotationKey]; ok && val == hash {
		return true
	}

	return false
}

func cleanupPods(client client.Client, logger logr.Logger, status *datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus, pods []*corev1.Pod) error {
	errs := deletePodSlice(client, logger, pods)
	now := metav1.NewTime(time.Now())
	conditionStatus := corev1.ConditionTrue
	if len(errs) > 0 {
		conditionStatus = corev1.ConditionFalse
	}
	if len(pods) != 0 {
		conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(status, now, datadoghqv1alpha1.ConditionTypePodsCleanupDone, conditionStatus, "", "", false, false)
	}

	return utilserrors.NewAggregate(errs)
}

func deletePodSlice(client client.Client, logger logr.Logger, podsToDelete []*corev1.Pod) []error {
	var errs []error
	var wg sync.WaitGroup
	for id, pod := range podsToDelete {
		if pod.DeletionTimestamp != nil {
			// already in deletion phase
			continue
		}
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pod := podsToDelete[id]
			logger.Info("cleanupPods delete pod", "pod_name", pod.Name)
			err := client.Delete(context.TODO(), pod)
			if err != nil {
				errs = append(errs, err)
			}
		}(id)
	}
	wg.Wait()

	return errs
}

func manageUnscheduledPodNodes(pods []*corev1.Pod) []string {
	var output []string
	for _, pod := range pods {
		idcond, condition := podutils.GetPodCondition(&pod.Status, corev1.PodScheduled)
		if idcond == -1 {
			continue
		}
		if condition.Status == corev1.ConditionFalse && condition.Reason == corev1.PodReasonUnschedulable {
			nodeName := pod.Spec.NodeName
			if nodeName == "" {
				nodeName = podaffinity.GetNodeNameFromAffinity(pod.Spec.Affinity)
			}
			output = append(output, nodeName)
		}
	}

	return output
}

// addPodLabel adds a given label to a pod, no-op if the pod is nil or if the label exists.
func addPodLabel(logger logr.Logger, c client.Client, pod *corev1.Pod, k, v string) error {
	if pod == nil {
		return nil
	}

	if label, found := pod.GetLabels()[k]; found && label == v {
		logger.V(1).Info("Canary labels already present", "pod.name", pod.Name)
		// The label is there, nothing to do
		return nil
	}
	newPod := pod.DeepCopy()
	// A merge patch will preserve other fields modified at runtime.
	patch := client.MergeFrom(pod)
	if newPod.Labels == nil {
		newPod.Labels = make(map[string]string)
	}
	newPod.Labels[k] = v
	data, err := patch.Data(newPod)
	logger.V(1).Info("Add canary label patch", "data", string(data), "err", err, "pod.name", pod.Name)

	return c.Patch(context.TODO(), newPod, patch)
}

// deletePodLabel deletes a given pod label, no-op if the pod is nil or if the label doesn't exists.
func deletePodLabel(logger logr.Logger, c client.Client, pod *corev1.Pod, k string) error {
	if pod == nil {
		return nil
	}

	if _, found := pod.GetLabels()[k]; !found {
		logger.V(1).Info("Canary labels not present", "pod.name", pod.Name)
		// The label is not there, nothing to do
		return nil
	}

	// A merge patch will preserve other fields modified at runtime.
	newPod := pod.DeepCopy()
	patch := client.MergeFrom(pod)
	delete(newPod.Labels, k)
	data, err := patch.Data(newPod)
	logger.V(1).Info("Delete canary label patch", "data", string(data), "err", err, "pod.name", pod.Name)

	return c.Patch(context.TODO(), newPod, patch)
}
