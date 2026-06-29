// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
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
			r.log.V(1).Info("modifying node labels", "node", node.Name, "labelsToRemove", labelsToRemove, "labelsToAddOrChange", labelsToAddOrChange)
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

	for _, agentPod := range agentPods.Items {
		profileNamespacedName, found := profilesByNode[agentPod.Spec.NodeName]
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
			r.log.Info("Deleting pod for profile cleanup", "pod.Namespace", toDelete.Namespace, "pod.Name", toDelete.Name)
			if err = r.client.Delete(ctx, &toDelete); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// getValidDaemonSetNames generates a list of valid DS and EDS names
func (r *Reconciler) getValidDaemonSetNames(dsName string, providerList map[string]struct{}, profiles []v1alpha1.DatadogAgentProfile, useV3Metadata bool) (map[string]struct{}, map[string]struct{}) {
	validDaemonSetNames := map[string]struct{}{}
	validExtendedDaemonSetNames := map[string]struct{}{}

	// Introspection includes names with a provider suffix
	if r.options.IntrospectionEnabled {
		if r.useDefaultDaemonset(providerList) {
			// Legacy DaemonSet uses the base name without provider suffix
			if r.options.ExtendedDaemonsetOptions.Enabled {
				validExtendedDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsName, kubernetes.DefaultProvider)] = struct{}{}
			} else {
				validDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsName, kubernetes.DefaultProvider)] = struct{}{}
			}
		} else {
			// Normal provider-specific DaemonSets
			for provider := range providerList {
				if r.options.ExtendedDaemonsetOptions.Enabled {
					validExtendedDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsName, provider)] = struct{}{}
				} else {
					validDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsName, provider)] = struct{}{}
				}
			}
		}
	}
	// Profiles include names with the profile prefix and the DS/EDS name for the default profile
	if r.options.DatadogAgentProfileEnabled {
		for _, profile := range profiles {
			name := types.NamespacedName{
				Namespace: profile.Namespace,
				Name:      profile.Name,
			}
			dsProfileName := agentprofile.DaemonSetName(name, useV3Metadata)

			// The default profile can be a DS or an EDS and uses the DS/EDS name
			if agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
				if r.options.IntrospectionEnabled {
					for provider := range providerList {
						if r.options.ExtendedDaemonsetOptions.Enabled {
							validExtendedDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsName, provider)] = struct{}{}
						} else {
							validDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsName, provider)] = struct{}{}
						}
					}
				} else {
					if r.options.ExtendedDaemonsetOptions.Enabled {
						validExtendedDaemonSetNames[dsName] = struct{}{}
					} else {
						validDaemonSetNames[dsName] = struct{}{}
					}
				}
			}
			// Non-default profiles can only be DaemonSets
			if r.options.IntrospectionEnabled {
				for provider := range providerList {
					validDaemonSetNames[kubernetes.GetAgentNameWithProvider(dsProfileName, provider)] = struct{}{}
				}
			} else {
				validDaemonSetNames[dsProfileName] = struct{}{}
			}
		}
	}

	// If neither introspection nor profiles are enabled, only the current DS/EDS name is valid
	if !r.options.IntrospectionEnabled && !r.options.DatadogAgentProfileEnabled {
		if r.options.ExtendedDaemonsetOptions.Enabled {
			validExtendedDaemonSetNames = map[string]struct{}{
				dsName: {},
			}
		} else {
			validDaemonSetNames = map[string]struct{}{
				dsName: {},
			}
		}
	}

	return validDaemonSetNames, validExtendedDaemonSetNames
}

// GetAgentInstanceLabelValue returns the instance name for the agent
// The current and default is DDA name + suffix (e.g. <dda-name>-agent)
// This is used when profiles are disabled or for the default profile
// If profiles are enabled, use the profile name (e.g. <profile-name>-agent) for profile DSs
func GetAgentInstanceLabelValue(dda, profile metav1.Object) string {
	// Always use v3 metadata for instance name
	if profile.GetName() != "" {
		if name := agentprofile.DaemonSetName(types.NamespacedName{Name: profile.GetName(), Namespace: profile.GetNamespace()}, true); name != "" {
			return name
		}
	}
	// Use default daemonset name
	return component.GetAgentName(dda)
}
