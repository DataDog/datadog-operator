// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// labelNodesWithProfiles sets the "agent.datadoghq.com/datadogagentprofile" label only in
// the nodes where a profile is applied
func (r *Reconciler) labelNodesWithProfiles(ctx context.Context, profilesByNode map[string]types.NamespacedName) error {
	for nodeName, profileNamespacedName := range profilesByNode {
		isDefaultProfile := agentprofile.IsDefaultProfile(profileNamespacedName.Namespace, profileNamespacedName.Name)

		// in the refactor, we don't explicitly set the default profile so empty profile nodes fall under the default profile
		if profileNamespacedName.Name == "" && profileNamespacedName.Namespace == "" {
			isDefaultProfile = true
		}

		node := &corev1.Node{}
		if err := r.client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			return err
		}

		newLabels := map[string]string{}
		labelsToRemove := map[string]bool{}
		labelsToAddOrChange := map[string]string{}

		// If the profile is the default one and the label exists in the node,
		// it should be removed.
		if isDefaultProfile {
			if _, profileLabelExists := node.Labels[constants.ProfileLabelKey]; profileLabelExists {
				labelsToRemove[constants.ProfileLabelKey] = true
			}
		} else {
			// If the profile is not the default one and the label does not exist in
			// the node, it should be added. If the label value is outdated, it
			// should be updated.
			if profileLabelValue := node.Labels[constants.ProfileLabelKey]; profileLabelValue != profileNamespacedName.Name {
				labelsToAddOrChange[constants.ProfileLabelKey] = profileNamespacedName.Name
			}
		}

		// Remove old profile label key if it is present
		if _, oldProfileLabelExists := node.Labels[agentprofile.OldProfileLabelKey]; oldProfileLabelExists {
			labelsToRemove[agentprofile.OldProfileLabelKey] = true
		}

		if len(labelsToRemove) > 0 || len(labelsToAddOrChange) > 0 {
			ctrl.LoggerFrom(ctx).V(1).Info("modifying node labels", "node", node.Name, "labelsToRemove", labelsToRemove, "labelsToAddOrChange", labelsToAddOrChange)
			for k, v := range node.Labels {
				if _, ok := labelsToRemove[k]; ok {
					continue
				}
				newLabels[k] = v
			}

			maps.Copy(newLabels, labelsToAddOrChange)
		}

		if len(newLabels) == 0 {
			continue
		}

		modifiedNode := node.DeepCopy()
		modifiedNode.Labels = newLabels

		err := r.client.Patch(ctx, modifiedNode, client.MergeFrom(node))
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// cleanupPodsForProfilesThatNoLongerApply deletes the agent pods that should
// not be running according to the profiles that need to be applied. This is
// needed because in the affinities we use
// "RequiredDuringSchedulingIgnoredDuringExecution" which means that the pods
// might not always be evicted when there's a change in the profiles to apply.
// Notice that "RequiredDuringSchedulingRequiredDuringExecution" is not
// available in Kubernetes yet.
// Unschedulable pods have no node name yet, so their node is read from their
// node affinity.
func (r *Reconciler) cleanupPodsForProfilesThatNoLongerApply(ctx context.Context, profilesByNode map[string]types.NamespacedName, ddaNamespace string) error {
	agentPods := &corev1.PodList{}
	err := r.client.List(
		ctx,
		agentPods,
		client.MatchingLabels(map[string]string{
			apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		}),
		client.InNamespace(ddaNamespace),
	)
	if err != nil {
		return err
	}

	// The pod cache only keeps names, labels and node names, so unscheduled
	// pods are read straight from the API server to get their node affinity.
	// This only happens when the cache shows a pod with no node yet.
	hasUnscheduledPod := slices.ContainsFunc(agentPods.Items, func(pod corev1.Pod) bool {
		return pod.Spec.NodeName == ""
	})
	if hasUnscheduledPod && r.options.APIReader != nil {
		unscheduledPods := &corev1.PodList{}
		err = r.options.APIReader.List(
			ctx,
			unscheduledPods,
			client.MatchingLabels(map[string]string{
				apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
			}),
			client.MatchingFields{"spec.nodeName": ""},
			client.InNamespace(ddaNamespace),
		)
		if err != nil {
			return err
		}
		agentPods.Items = append(agentPods.Items, unscheduledPods.Items...)
	}

	for _, agentPod := range agentPods.Items {
		// Already being deleted
		if agentPod.DeletionTimestamp != nil {
			continue
		}

		profileNamespacedName, found := profilesByNode[agentPodNodeName(&agentPod)]
		if !found {
			continue
		}

		isDefaultProfile := agentprofile.IsDefaultProfile(profileNamespacedName.Namespace, profileNamespacedName.Name)
		expectedProfileLabelValue := profileNamespacedName.Name

		profileLabelValue, profileLabelExists := agentPod.Labels[constants.ProfileLabelKey]

		deletePod := (isDefaultProfile && profileLabelExists) ||
			(!isDefaultProfile && !profileLabelExists) ||
			(!isDefaultProfile && profileLabelValue != expectedProfileLabelValue)

		if deletePod {
			toDelete := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: agentPod.Namespace,
					Name:      agentPod.Name,
				},
			}
			ctrl.LoggerFrom(ctx).Info("Deleting pod for profile cleanup", "pod.Namespace", toDelete.Namespace, "pod.Name", toDelete.Name)
			if err = r.client.Delete(ctx, &toDelete); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// agentPodNodeName returns the node the agent pod runs on. For an unschedulable
// pod created by the operator, it returns the node in the pod's "metadata.name"
// node affinity.
func agentPodNodeName(pod *corev1.Pod) string {
	if pod.Spec.NodeName != "" {
		return pod.Spec.NodeName
	}

	unschedulable := slices.ContainsFunc(pod.Status.Conditions, func(cond corev1.PodCondition) bool {
		return cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse && cond.Reason == corev1.PodReasonUnschedulable
	})
	managedByOperator := pod.Labels[kubernetes.AppKubernetesManageByLabelKey] == "datadog-operator"
	if !managedByOperator || !unschedulable || pod.Spec.Affinity == nil || pod.Spec.Affinity.NodeAffinity == nil {
		return ""
	}

	nodeSelector := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if nodeSelector == nil {
		return ""
	}
	for _, term := range nodeSelector.NodeSelectorTerms {
		for _, field := range term.MatchFields {
			if field.Key == common.FieldPathMetaName && field.Operator == corev1.NodeSelectorOpIn && len(field.Values) == 1 {
				return field.Values[0]
			}
		}
	}

	return ""
}

// getValidDaemonSetNames generates the list of valid DaemonSet names.
func (r *Reconciler) getValidDaemonSetNames(dsName string, providerList map[string]struct{}, profiles []v1alpha1.DatadogAgentProfile, useV3Metadata bool) map[string]struct{} {
	validDaemonSetNames := map[string]struct{}{}

	// Introspection includes names with a provider suffix
	if r.options.IntrospectionEnabled {
		if r.useDefaultDaemonset(providerList) {
			validDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsName, kubernetes.DefaultProvider)] = struct{}{}
		} else {
			for provider := range providerList {
				validDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsName, provider)] = struct{}{}
			}
		}
	}
	// Profiles include names with the profile prefix and the DaemonSet name for the default profile.
	if r.options.DatadogAgentProfileEnabled {
		for _, profile := range profiles {
			name := types.NamespacedName{
				Namespace: profile.Namespace,
				Name:      profile.Name,
			}
			dsProfileName := agentprofile.DaemonSetName(name, useV3Metadata)

			// The default profile uses the base DaemonSet name.
			if agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
				if r.options.IntrospectionEnabled {
					for provider := range providerList {
						validDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsName, provider)] = struct{}{}
					}
				} else {
					validDaemonSetNames[dsName] = struct{}{}
				}
			}
			// Non-default profiles use profile-specific DaemonSet names.
			if r.options.IntrospectionEnabled {
				for provider := range providerList {
					validDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsProfileName, provider)] = struct{}{}
				}
			} else {
				validDaemonSetNames[dsProfileName] = struct{}{}
			}
		}
	}

	// If neither introspection nor profiles are enabled, only the current DaemonSet name is valid.
	if !r.options.IntrospectionEnabled && !r.options.DatadogAgentProfileEnabled {
		validDaemonSetNames = map[string]struct{}{
			dsName: {},
		}
	}

	return validDaemonSetNames
}
