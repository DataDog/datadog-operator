// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"maps"
	"time"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/helm"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (r *Reconciler) reconcileV2Agent(ctx context.Context, logger logr.Logger, requiredComponents feature.RequiredComponents, features []feature.Feature,
	dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus,
	provider string, providerList map[string]struct{}, profile *v1alpha1.DatadogAgentProfile) (reconcile.Result, error) {
	var result reconcile.Result
	var eds *edsv1alpha1.ExtendedDaemonSet
	var daemonset *appsv1.DaemonSet
	var podManagers feature.PodTemplateManagers

	daemonsetLogger := logger.WithValues("component", datadoghqv2alpha1.NodeAgentComponentName)

	// requiredComponents needs to be taken into account in case a feature(s) changes and
	// a requiredComponent becomes disabled, in addition to taking into account override.Disabled
	disabledByOverride := false

	agentEnabled := requiredComponents.Agent.IsEnabled()
	singleContainerStrategyEnabled := requiredComponents.Agent.SingleContainerStrategyEnabled()
	// TODO: remove this once reconcileV2 is removed
	instanceName := GetAgentInstanceLabelValue(dda, profile)

	// When EDS is enabled and there are profiles defined, we only create an
	// EDS for the default profile, for the other profiles we create
	// DaemonSets.
	// This is to make deployments simpler. With multiple EDS there would be
	// multiple canaries, etc.
	if (r.options.ExtendedDaemonsetOptions.Enabled && !r.options.DatadogAgentProfileEnabled) || (r.options.ExtendedDaemonsetOptions.Enabled &&
		r.options.DatadogAgentProfileEnabled && agentprofile.IsDefaultProfile(profile.Namespace, profile.Name)) {
		// Start by creating the Default Agent extendeddaemonset
		eds = componentagent.NewDefaultAgentExtendedDaemonset(dda, &r.options.ExtendedDaemonsetOptions, requiredComponents.Agent)
		podManagers = feature.NewPodTemplateManagers(&eds.Spec.Template)

		// Set Global setting on the default extendeddaemonset
		global.ApplyGlobalSettingsNodeAgent(logger, podManagers, dda.GetObjectMeta(), &dda.Spec, resourcesManager, singleContainerStrategyEnabled, requiredComponents)

		// Apply features changes on the Deployment.Spec.Template
		for _, feat := range features {
			if errFeat := feat.ManageNodeAgent(podManagers, provider); errFeat != nil {
				return result, errFeat
			}
		}

		// If Override is defined for the node agent component, apply the override on the PodTemplateSpec, it will cascade to container.
		var componentOverrides []*datadoghqv2alpha1.DatadogAgentComponentOverride
		if componentOverride, ok := dda.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName]; ok {
			componentOverrides = append(componentOverrides, componentOverride)
		}

		if r.options.DatadogAgentProfileEnabled {
			// Apply overrides from profiles after override from manifest, so they can override what's defined in the DDA.
			overrideFromProfile := agentprofile.OverrideFromProfile(profile, false)
			componentOverrides = append(componentOverrides, &overrideFromProfile)
		}

		if r.options.IntrospectionEnabled {
			// use the last name override in the list to generate a provider-specific name
			overrideName := eds.Name
			for _, componentOverride := range componentOverrides {
				if componentOverride.Name != nil && *componentOverride.Name != "" {
					overrideName = *componentOverride.Name
				}
			}
			overrideFromProvider := kubernetes.ComponentOverrideFromProvider(overrideName, provider, providerList)
			componentOverrides = append(componentOverrides, &overrideFromProvider)
		} else {
			eds.Labels[constants.MD5AgentDeploymentProviderLabelKey] = kubernetes.LegacyProvider
		}

		for _, componentOverride := range componentOverrides {
			if apiutils.BoolValue(componentOverride.Disabled) {
				disabledByOverride = true
			}
			override.PodTemplateSpec(logger, podManagers, componentOverride, datadoghqv2alpha1.NodeAgentComponentName, dda.Name)
			override.ExtendedDaemonSet(eds, componentOverride)
		}

		experimental.ApplyExperimentalOverrides(logger, dda, podManagers)

		if disabledByOverride {
			if agentEnabled {
				// The override supersedes what's set in requiredComponents; update status to reflect the conflict
				condition.UpdateDatadogAgentStatusConditions(
					newStatus,
					metav1.NewTime(time.Now()),
					common.OverrideReconcileConflictConditionType,
					metav1.ConditionTrue,
					"OverrideConflict",
					"Agent component is set to disabled",
					true,
				)
			}
			if err := r.deleteV2ExtendedDaemonSet(daemonsetLogger, dda, eds, newStatus); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}

		return r.createOrUpdateExtendedDaemonset(daemonsetLogger, dda, eds, newStatus, updateEDSStatusV2WithAgent)
	}

	// Start by creating the Default Agent daemonset
	daemonset = componentagent.NewDefaultAgentDaemonset(dda, &r.options.ExtendedDaemonsetOptions, requiredComponents.Agent, instanceName)
	podManagers = feature.NewPodTemplateManagers(&daemonset.Spec.Template)

	// Check if this operator daemonset should have migration label (after Helm migration completed)
	if helm.IsHelmMigration(dda) {
		dsList := appsv1.DaemonSetList{}
		if err := r.client.List(context.TODO(), &dsList, client.MatchingLabels{
			apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
			kubernetes.AppKubernetesManageByLabelKey:   "Helm",
			apicommon.AgentDeploymentNameLabelKey:      "datadog",
		}); err == nil && len(dsList.Items) == 0 {
			nsName := types.NamespacedName{
				Name:      daemonset.GetName(),
				Namespace: daemonset.GetNamespace(),
			}
			existingDaemonset := &appsv1.DaemonSet{}
			if err := r.client.Get(context.TODO(), nsName, existingDaemonset); err != nil {
				if daemonset.Labels == nil {
					daemonset.Labels = make(map[string]string)
				}
				daemonset.Labels[constants.MD5AgentDeploymentMigratedLabelKey] = "true"
				logger.Info("Adding migration label to new operator daemonset as Helm migration has completed")
			}
			// Add Cluster Agent Service ClusterIP checksum to trigger rollout when service is recreated
			r.addDCAServiceClusterIPChecksum(ctx, logger, dda, podManagers)
		}
	}
	// Set Global setting on the default daemonset
	global.ApplyGlobalSettingsNodeAgent(logger, podManagers, dda.GetObjectMeta(), &dda.Spec, resourcesManager, singleContainerStrategyEnabled, requiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range features {
		if singleContainerStrategyEnabled {
			if errFeat := feat.ManageSingleContainerNodeAgent(podManagers, provider); errFeat != nil {
				return result, errFeat
			}
		} else {
			if errFeat := feat.ManageNodeAgent(podManagers, provider); errFeat != nil {
				return result, errFeat
			}
		}
	}

	// If Override is defined for the node agent component, apply the override on the PodTemplateSpec, it will cascade to container.
	var componentOverrides []*datadoghqv2alpha1.DatadogAgentComponentOverride
	if componentOverride, ok := dda.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName]; ok {
		componentOverrides = append(componentOverrides, componentOverride)
	}

	if r.options.DatadogAgentProfileEnabled {
		// Apply overrides from profiles after override from manifest, so they can override what's defined in the DDA.
		overrideFromProfile := agentprofile.OverrideFromProfile(profile, useV3Metadata(dda))
		componentOverrides = append(componentOverrides, &overrideFromProfile)
	}

	if r.options.IntrospectionEnabled {
		// use the last name override in the list to generate a provider-specific name
		overrideName := daemonset.Name
		for _, componentOverride := range componentOverrides {
			if componentOverride.Name != nil && *componentOverride.Name != "" {
				overrideName = *componentOverride.Name
			}
		}
		overrideFromProvider := kubernetes.ComponentOverrideFromProvider(overrideName, provider, providerList)
		componentOverrides = append(componentOverrides, &overrideFromProvider)
	} else {
		daemonset.Labels[constants.MD5AgentDeploymentProviderLabelKey] = kubernetes.LegacyProvider
	}

	for _, componentOverride := range componentOverrides {
		if apiutils.BoolValue(componentOverride.Disabled) {
			disabledByOverride = true
		}
		override.PodTemplateSpec(logger, podManagers, componentOverride, datadoghqv2alpha1.NodeAgentComponentName, dda.Name)
		override.DaemonSet(daemonset, componentOverride)
	}

	experimental.ApplyExperimentalOverrides(logger, dda, podManagers)

	if disabledByOverride {
		if agentEnabled {
			// The override supersedes what's set in requiredComponents; update status to reflect the conflict
			condition.UpdateDatadogAgentStatusConditions(
				newStatus,
				metav1.NewTime(time.Now()),
				common.OverrideReconcileConflictConditionType,
				metav1.ConditionTrue,
				"OverrideConflict",
				"Agent component is set to disabled",
				true,
			)
		}
		if err := r.deleteV2DaemonSet(daemonsetLogger, dda, daemonset, newStatus); err != nil {
			return reconcile.Result{}, err
		}
		deleteStatusWithAgent(newStatus)
		return reconcile.Result{}, nil
	}

	return r.createOrUpdateDaemonset(daemonsetLogger, dda, daemonset, newStatus, updateDSStatusV2WithAgent, profile)
}

func updateDSStatusV2WithAgent(dsName string, ds *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.AgentList = condition.UpdateDaemonSetStatus(dsName, ds, newStatus.AgentList, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.AgentReconcileConditionType, status, reason, message, true)
	newStatus.Agent = condition.UpdateCombinedDaemonSetStatus(newStatus.AgentList)
}

func updateEDSStatusV2WithAgent(eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.AgentList = condition.UpdateExtendedDaemonSetStatus(eds, newStatus.AgentList, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.AgentReconcileConditionType, status, reason, message, true)
	newStatus.Agent = condition.UpdateCombinedDaemonSetStatus(newStatus.AgentList)
}

func (r *Reconciler) deleteV2DaemonSet(logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, ds *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus) error {
	err := r.client.Delete(context.TODO(), ds)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	logger.Info("Delete DaemonSet", "daemonSet.Namespace", ds.Namespace, "daemonSet.Name", ds.Name)
	event := buildEventInfo(ds.Name, ds.Namespace, kubernetes.DaemonSetKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	removeStaleStatus(newStatus, ds.Name)

	return nil
}

func (r *Reconciler) deleteV2ExtendedDaemonSet(logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus) error {
	err := r.client.Delete(context.TODO(), eds)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	logger.Info("Delete DaemonSet", "extendedDaemonSet.Namespace", eds.Namespace, "extendedDaemonSet.Name", eds.Name)
	event := buildEventInfo(eds.Name, eds.Namespace, kubernetes.ExtendedDaemonSetKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	removeStaleStatus(newStatus, eds.Name)

	return nil
}

func deleteStatusWithAgent(newStatus *datadoghqv2alpha1.DatadogAgentStatus) {
	newStatus.Agent = nil
	condition.DeleteDatadogAgentStatusCondition(newStatus, common.AgentReconcileConditionType)
}

// removeStaleStatus removes a DaemonSet's status from a DatadogAgent's
// status based on the DaemonSet's name
func removeStaleStatus(ddaStatus *datadoghqv2alpha1.DatadogAgentStatus, name string) {
	if ddaStatus != nil {
		for i, dsStatus := range ddaStatus.AgentList {
			if dsStatus.DaemonsetName == name {
				newStatus := make([]*datadoghqv2alpha1.DaemonSetStatus, 0, len(ddaStatus.AgentList)-1)
				newStatus = append(newStatus, ddaStatus.AgentList[:i]...)
				ddaStatus.AgentList = append(newStatus, ddaStatus.AgentList[i+1:]...)
			}
		}
	}
}

func (r *Reconciler) handleProfiles(ctx context.Context, profilesByNode map[string]types.NamespacedName, ddaNamespace string) error {
	if err := r.labelNodesWithProfiles(ctx, profilesByNode); err != nil {
		return err
	}

	// TODO: re-evaluate if this is needed
	// if err := r.cleanupPodsForProfilesThatNoLongerApply(ctx, profilesByNode, ddaNamespace); err != nil {
	// 	return err
	// }

	return nil
}

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
// func (r *Reconciler) cleanupPodsForProfilesThatNoLongerApply(ctx context.Context, profilesByNode map[string]types.NamespacedName, ddaNamespace string) error {
// 	agentPods := &corev1.PodList{}
// 	err := r.client.List(
// 		ctx,
// 		agentPods,
// 		client.MatchingLabels(map[string]string{
// 			apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
// 		}),
// 		client.InNamespace(ddaNamespace),
// 	)
// 	if err != nil {
// 		return err
// 	}

// 	for _, agentPod := range agentPods.Items {
// 		profileNamespacedName, found := profilesByNode[agentPod.Spec.NodeName]
// 		if !found {
// 			continue
// 		}

// 		isDefaultProfile := agentprofile.IsDefaultProfile(profileNamespacedName.Namespace, profileNamespacedName.Name)
// 		expectedProfileLabelValue := profileNamespacedName.Name

// 		profileLabelValue, profileLabelExists := agentPod.Labels[constants.ProfileLabelKey]

// 		deletePod := (isDefaultProfile && profileLabelExists) ||
// 			(!isDefaultProfile && !profileLabelExists) ||
// 			(!isDefaultProfile && profileLabelValue != expectedProfileLabelValue)

// 		if deletePod {
// 			toDelete := corev1.Pod{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Namespace: agentPod.Namespace,
// 					Name:      agentPod.Name,
// 				},
// 			}
// 			r.log.Info("Deleting pod for profile cleanup", "pod.Namespace", toDelete.Namespace, "pod.Name", toDelete.Name)
// 			if err = r.client.Delete(ctx, &toDelete); err != nil && !errors.IsNotFound(err) {
// 				return err
// 			}
// 		}
// 	}

// 	return nil
// }

// cleanupExtraneousDaemonSets deletes DSs/EDSs that no longer apply.
// Use cases include deleting old DSs/EDSs when:
// - a DaemonSet's name is changed using node overrides
// - introspection is disabled or enabled
// - a profile is deleted
func (r *Reconciler) cleanupExtraneousDaemonSets(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus,
	providerList map[string]struct{}, profiles []v1alpha1.DatadogAgentProfile) error {
	matchLabels := client.MatchingLabels{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(dda).String(),
	}

	dsName := component.GetDaemonSetNameFromDatadogAgent(dda, &dda.Spec)

	validDaemonSetNames, validExtendedDaemonSetNames := r.getValidDaemonSetNames(dsName, providerList, profiles, useV3Metadata(dda))
	// log computed valid names for debugging
	vd := make([]string, 0, len(validDaemonSetNames))
	for n := range validDaemonSetNames {
		vd = append(vd, n)
	}
	veds := make([]string, 0, len(validExtendedDaemonSetNames))
	for n := range validExtendedDaemonSetNames {
		veds = append(veds, n)
	}

	// Safety guard: if no valid names could be computed, skip cleanup to avoid
	// deleting all DaemonSets during transient cache hiccups (e.g., empty provider list).
	if len(validDaemonSetNames) == 0 && len(validExtendedDaemonSetNames) == 0 {
		logger.Info("Skipping cleanup of DaemonSets: no valid names computed", "providersLen", len(providerList), "profilesLen", len(profiles))
		return nil
	}

	// Only the default profile uses an EDS when profiles are enabled
	// Multiple EDSs can be created with introspection
	if r.options.ExtendedDaemonsetOptions.Enabled {
		edsList := edsv1alpha1.ExtendedDaemonSetList{}
		if err := r.client.List(ctx, &edsList, matchLabels); err != nil {
			return err
		}
		logger.V(1).Info("Listed ExtendedDaemonSets for cleanup", "count", len(edsList.Items))
		for _, eds := range edsList.Items {
			if _, ok := validExtendedDaemonSetNames[eds.Name]; !ok {
				logger.Info("Candidate ExtendedDaemonSet deletion", "name", eds.Name)
				if err := r.deleteV2ExtendedDaemonSet(logger, dda, &eds, newStatus); err != nil {
					return err
				}
			}
		}
	}

	daemonSetList := appsv1.DaemonSetList{}
	if err := r.client.List(ctx, &daemonSetList, matchLabels); err != nil {
		return err
	}

	logger.V(1).Info("Listed DaemonSets for cleanup", "count", len(daemonSetList.Items))
	for _, daemonSet := range daemonSetList.Items {
		if _, ok := validDaemonSetNames[daemonSet.Name]; !ok {
			logger.Info("Candidate DaemonSet deletion", "name", daemonSet.Name)
			if err := r.deleteV2DaemonSet(logger, dda, &daemonSet, newStatus); err != nil {
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

// addDCAServiceClusterIPChecksum reads the live Cluster Agent Service ClusterIP from the API server
// and adds a checksum annotation to the pod template. This ensures that when the Service is recreated
// with a new ClusterIP (e.g., after helm uninstall during migration), a DaemonSet rollout will be triggered
// so that the agent pods pick up the new IP via Kubernetes-injected environment variables.
func (r *Reconciler) addDCAServiceClusterIPChecksum(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, podManagers feature.PodTemplateManagers) {
	serviceName := componentdca.GetClusterAgentServiceName(dda)
	serviceNsName := types.NamespacedName{
		Namespace: dda.GetNamespace(),
		Name:      serviceName,
	}

	// Read the live Service from the API server to get the actual ClusterIP
	liveService := &corev1.Service{}
	if err := r.client.Get(ctx, serviceNsName, liveService); err != nil {
		logger.V(1).Info("Could not read Cluster Agent Service for ClusterIP checksum, skipping annotation", "error", err)
		return
	}

	clusterIP := liveService.Spec.ClusterIP
	if clusterIP == "" || clusterIP == "None" {
		logger.V(1).Info("Cluster Agent Service has no ClusterIP, skipping annotation")
		return
	}

	hash, err := comparison.GenerateMD5ForSpec(map[string]string{"clusterIP": clusterIP})
	if err != nil {
		logger.Error(err, "Failed to generate hash for Cluster Agent Service ClusterIP")
		return
	}

	podManagers.Annotation().AddAnnotation(constants.MD5DCAServiceClusterIPAnnotationKey, hash)
	logger.V(2).Info("Added Cluster Agent Service ClusterIP checksum annotation", "clusterIP", clusterIP, "hash", hash)
}
