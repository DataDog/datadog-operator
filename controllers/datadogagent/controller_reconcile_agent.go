// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"time"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	componentagent "github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) reconcileV2Agent(logger logr.Logger, requiredComponents feature.RequiredComponents, features []feature.Feature,
	dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus,
	provider string, profile *v1alpha1.DatadogAgentProfile) (reconcile.Result, error) {
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

	// When EDS is enabled and there are profiles defined, we only create an
	// EDS for the default profile, for the other profiles we create
	// DaemonSets.
	// This is to make deployments simpler. With multiple EDS there would be
	// multiple canaries, etc.
	if r.options.ExtendedDaemonsetOptions.Enabled && agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		// Start by creating the Default Agent extendeddaemonset
		eds = componentagent.NewDefaultAgentExtendedDaemonset(dda, &r.options.ExtendedDaemonsetOptions, requiredComponents.Agent)
		podManagers = feature.NewPodTemplateManagers(&eds.Spec.Template)

		// Set Global setting on the default extendeddaemonset
		eds.Spec.Template = *override.ApplyGlobalSettingsNodeAgent(logger, podManagers, dda, resourcesManager, singleContainerStrategyEnabled)

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

		// Apply overrides from profiles after override from manifest, so they can override what's defined in the DDA.
		overrideFromProfile := agentprofile.ComponentOverrideFromProfile(profile)
		componentOverrides = append(componentOverrides, &overrideFromProfile)

		if r.options.IntrospectionEnabled {
			// use the last name override in the list to generate a provider-specific name
			overrideName := eds.Name
			for _, componentOverride := range componentOverrides {
				if componentOverride.Name != nil && *componentOverride.Name != "" {
					overrideName = *componentOverride.Name
				}
			}
			affinity := eds.Spec.Template.Spec.Affinity.DeepCopy()
			combinedAffinity := r.generateNodeAffinity(provider, affinity)
			overrideFromProvider := kubernetes.ComponentOverrideFromProvider(overrideName, provider, combinedAffinity)
			componentOverrides = append(componentOverrides, &overrideFromProvider)
		} else {
			eds.Labels[apicommon.MD5AgentDeploymentProviderLabelKey] = kubernetes.LegacyProvider
		}

		for _, componentOverride := range componentOverrides {
			if apiutils.BoolValue(componentOverride.Disabled) {
				disabledByOverride = true
			}
			override.PodTemplateSpec(logger, podManagers, componentOverride, datadoghqv2alpha1.NodeAgentComponentName, dda.Name)
			override.ExtendedDaemonSet(eds, componentOverride)
		}

		if disabledByOverride {
			if agentEnabled {
				// The override supersedes what's set in requiredComponents; update status to reflect the conflict
				datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(
					newStatus,
					metav1.NewTime(time.Now()),
					datadoghqv2alpha1.OverrideReconcileConflictConditionType,
					metav1.ConditionTrue,
					"OverrideConflict",
					"Agent component is set to disabled",
					true,
				)
			}
			return r.cleanupV2ExtendedDaemonSet(daemonsetLogger, dda, eds, newStatus)
		}

		return r.createOrUpdateExtendedDaemonset(daemonsetLogger, dda, eds, newStatus, updateEDSStatusV2WithAgent)
	}

	// Start by creating the Default Agent daemonset
	daemonset = componentagent.NewDefaultAgentDaemonset(dda, requiredComponents.Agent)
	podManagers = feature.NewPodTemplateManagers(&daemonset.Spec.Template)
	// Set Global setting on the default daemonset
	daemonset.Spec.Template = *override.ApplyGlobalSettingsNodeAgent(logger, podManagers, dda, resourcesManager, singleContainerStrategyEnabled)

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

	// Apply overrides from profiles after override from manifest, so they can override what's defined in the DDA.
	overrideFromProfile := agentprofile.ComponentOverrideFromProfile(profile)
	componentOverrides = append(componentOverrides, &overrideFromProfile)

	if r.options.IntrospectionEnabled {
		// use the last name override in the list to generate a provider-specific name
		overrideName := daemonset.Name
		for _, componentOverride := range componentOverrides {
			if componentOverride.Name != nil && *componentOverride.Name != "" {
				overrideName = *componentOverride.Name
			}
		}
		affinity := daemonset.Spec.Template.Spec.Affinity.DeepCopy()
		combinedAffinity := r.generateNodeAffinity(provider, affinity)
		overrideFromProvider := kubernetes.ComponentOverrideFromProvider(overrideName, provider, combinedAffinity)
		componentOverrides = append(componentOverrides, &overrideFromProvider)
	} else {
		daemonset.Labels[apicommon.MD5AgentDeploymentProviderLabelKey] = kubernetes.LegacyProvider
	}

	for _, componentOverride := range componentOverrides {
		if apiutils.BoolValue(componentOverride.Disabled) {
			disabledByOverride = true
		}
		override.PodTemplateSpec(logger, podManagers, componentOverride, datadoghqv2alpha1.NodeAgentComponentName, dda.Name)
		override.DaemonSet(daemonset, componentOverride)
	}

	if disabledByOverride {
		if agentEnabled {
			// The override supersedes what's set in requiredComponents; update status to reflect the conflict
			datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(
				newStatus,
				metav1.NewTime(time.Now()),
				datadoghqv2alpha1.OverrideReconcileConflictConditionType,
				metav1.ConditionTrue,
				"OverrideConflict",
				"Agent component is set to disabled",
				true,
			)
		}
		return r.cleanupV2DaemonSet(daemonsetLogger, dda, daemonset, newStatus)
	}

	return r.createOrUpdateDaemonset(daemonsetLogger, dda, daemonset, newStatus, updateDSStatusV2WithAgent)
}

func updateDSStatusV2WithAgent(ds *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.AgentList = datadoghqv2alpha1.UpdateDaemonSetStatus(ds, newStatus.AgentList, &updateTime)
	datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, updateTime, datadoghqv2alpha1.AgentReconcileConditionType, status, reason, message, true)
	newStatus.Agent = datadoghqv2alpha1.UpdateCombinedDaemonSetStatus(newStatus.AgentList)
}

func updateEDSStatusV2WithAgent(eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.AgentList = datadoghqv2alpha1.UpdateExtendedDaemonSetStatus(eds, newStatus.AgentList, &updateTime)
	datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, updateTime, datadoghqv2alpha1.AgentReconcileConditionType, status, reason, message, true)
	newStatus.Agent = datadoghqv2alpha1.UpdateCombinedDaemonSetStatus(newStatus.AgentList)
}

func (r *Reconciler) cleanupV2DaemonSet(logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, ds *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      ds.GetName(),
		Namespace: ds.GetNamespace(),
	}

	// DS attached to this instance
	instance := &appsv1.DaemonSet{}
	if err := r.client.Get(context.TODO(), nsName, instance); err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	} else {
		err := r.client.Delete(context.TODO(), ds)
		if err != nil {
			return reconcile.Result{}, err
		}
		logger.Info("Delete DaemonSet", "daemonSet.Namespace", ds.Namespace, "daemonSet.Name", ds.Name)
		event := buildEventInfo(ds.Name, ds.Namespace, daemonSetKind, datadog.DeletionEvent)
		r.recordEvent(dda, event)
	}
	newStatus.Agent = nil

	return reconcile.Result{}, nil
}

func (r *Reconciler) cleanupV2ExtendedDaemonSet(logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      eds.GetName(),
		Namespace: eds.GetNamespace(),
	}

	// EDS attached to this instance
	instance := &edsv1alpha1.ExtendedDaemonSet{}
	if err := r.client.Get(context.TODO(), nsName, instance); err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	} else {
		err := r.client.Delete(context.TODO(), eds)
		if err != nil {
			return reconcile.Result{}, err
		}
		logger.Info("Delete DaemonSet", "extendedDaemonSet.Namespace", eds.Namespace, "extendedDaemonSet.Name", eds.Name)
		event := buildEventInfo(eds.Name, eds.Namespace, extendedDaemonSetKind, datadog.DeletionEvent)
		r.recordEvent(dda, event)
	}
	newStatus.Agent = nil

	return reconcile.Result{}, nil
}

func (r *Reconciler) generateNodeAffinity(p string, affinity *corev1.Affinity) *corev1.Affinity {
	nodeSelectorReq := r.providerStore.GenerateProviderNodeAffinity(p)
	if len(nodeSelectorReq) > 0 {
		// check for an existing affinity and merge
		if affinity == nil {
			affinity = &corev1.Affinity{}
		}
		if affinity.NodeAffinity == nil {
			affinity.NodeAffinity = &corev1.NodeAffinity{}
		}
		if affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
		}
		// NodeSelectorTerms are ORed
		if len(affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
			affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(
				affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
				corev1.NodeSelectorTerm{},
			)
		}
		// NodeSelectorTerms are ANDed
		if affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions == nil {
			affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions = []corev1.NodeSelectorRequirement{}
		}
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions = append(
			affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions,
			nodeSelectorReq...,
		)
	}

	return affinity
}

func (r *Reconciler) handleProviders(ctx context.Context, dda *datadoghqv2alpha1.DatadogAgent, ddaStatus *datadoghqv2alpha1.DatadogAgentStatus) (map[string]struct{}, error) {
	providerList, err := r.updateProviderStore(ctx)
	if err != nil {
		return nil, err
	}

	if err := r.cleanupDaemonSetsForProvidersThatNoLongerApply(ctx, dda, ddaStatus); err != nil {
		return nil, err
	}

	return providerList, nil
}

// updateProviderStore recomputes the required providers by making a
// call to the apiserver for an updated list of nodes
func (r *Reconciler) updateProviderStore(ctx context.Context) (map[string]struct{}, error) {
	nodeList := corev1.NodeList{}
	err := r.client.List(ctx, &nodeList)
	if err != nil {
		return nil, err
	}

	// recompute providers using updated node list
	providersList := make(map[string]struct{})
	for _, node := range nodeList.Items {
		provider := kubernetes.DetermineProvider(node.Labels)
		if _, ok := providersList[provider]; !ok {
			providersList[provider] = struct{}{}
			r.log.V(1).Info("New provider detected", "provider", provider)
		}
	}

	return r.providerStore.Reset(providersList), nil
}

// cleanupDaemonSetsForProvidersThatNoLongerApply deletes ds/eds from providers
// that are not present in the provider store. If there are no providers in the
// provider store, do not delete any ds/eds since that would delete all node
// agents.
func (r *Reconciler) cleanupDaemonSetsForProvidersThatNoLongerApply(ctx context.Context, dda *datadoghqv2alpha1.DatadogAgent, ddaStatus *datadoghqv2alpha1.DatadogAgentStatus) error {
	if r.options.ExtendedDaemonsetOptions.Enabled {
		edsList := edsv1alpha1.ExtendedDaemonSetList{}
		if err := r.client.List(ctx, &edsList, client.HasLabels{apicommon.MD5AgentDeploymentProviderLabelKey}); err != nil {
			return err
		}

		for _, eds := range edsList.Items {
			provider := eds.Labels[apicommon.MD5AgentDeploymentProviderLabelKey]
			if len(*r.providerStore.GetProviders()) > 0 && !r.providerStore.IsPresent(provider) {
				if err := r.client.Delete(ctx, &eds); err != nil {
					return err
				}
				r.log.Info("Deleted ExtendedDaemonSet", "extendedDaemonSet.Namespace", eds.Namespace, "extendedDaemonSet.Name", eds.Name)
				event := buildEventInfo(eds.Name, eds.Namespace, extendedDaemonSetKind, datadog.DeletionEvent)
				r.recordEvent(dda, event)

				removeStaleStatus(ddaStatus, eds.Name)
			}
		}

		return nil
	}

	daemonSetList := appsv1.DaemonSetList{}
	if err := r.client.List(ctx, &daemonSetList, client.HasLabels{apicommon.MD5AgentDeploymentProviderLabelKey}); err != nil {
		return err
	}

	for _, ds := range daemonSetList.Items {
		provider := ds.Labels[apicommon.MD5AgentDeploymentProviderLabelKey]
		if len(*r.providerStore.GetProviders()) > 0 && !r.providerStore.IsPresent(provider) {
			if err := r.client.Delete(ctx, &ds); err != nil {
				return err
			}
			r.log.Info("Deleted DaemonSet", "daemonSet.Namespace", ds.Namespace, "daemonSet.Name", ds.Name)
			event := buildEventInfo(ds.Name, ds.Namespace, daemonSetKind, datadog.DeletionEvent)
			r.recordEvent(dda, event)

			removeStaleStatus(ddaStatus, ds.Name)
		}
	}

	return nil
}

// removeStaleStatus removes a DaemonSet's status from a DatadogAgent's
// status based on the DaemonSet's name
func removeStaleStatus(ddaStatus *datadoghqv2alpha1.DatadogAgentStatus, name string) {
	if ddaStatus != nil {
		for i, dsStatus := range ddaStatus.AgentList {
			if dsStatus.DaemonsetName == name {
				newStatus := make([]*common.DaemonSetStatus, 0, len(ddaStatus.AgentList)-1)
				newStatus = append(newStatus, ddaStatus.AgentList[:i]...)
				ddaStatus.AgentList = append(newStatus, ddaStatus.AgentList[i+1:]...)
			}
		}
	}
}

func (r *Reconciler) handleProfiles(ctx context.Context, profiles []v1alpha1.DatadogAgentProfile, profilesByNode map[string]types.NamespacedName, ddaNamespace string) error {
	if err := r.cleanupDaemonSetsForProfilesThatNoLongerApply(ctx, profiles); err != nil {
		return err
	}

	if err := r.labelNodesWithProfiles(ctx, profilesByNode); err != nil {
		return err
	}

	if err := r.cleanupPodsForProfilesThatNoLongerApply(ctx, profilesByNode, ddaNamespace); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) cleanupDaemonSetsForProfilesThatNoLongerApply(ctx context.Context, profiles []v1alpha1.DatadogAgentProfile) error {
	daemonSets, err := r.agentProfileDaemonSetsCreatedByOperator(ctx)
	if err != nil {
		return err
	}

	daemonSetNamesBelongingToProfiles := map[string]struct{}{}
	for _, profile := range profiles {
		name := types.NamespacedName{
			Namespace: profile.Namespace,
			Name:      profile.Name,
		}

		if r.options.IntrospectionEnabled {
			providerList := r.providerStore.GetProviders()
			for provider := range *providerList {
				name.Name = kubernetes.GetAgentNameWithProvider(name.Name, provider)
				daemonSetNamesBelongingToProfiles[agentprofile.DaemonSetName(name)] = struct{}{}
			}
		} else {
			daemonSetNamesBelongingToProfiles[agentprofile.DaemonSetName(name)] = struct{}{}
		}
	}

	for _, daemonSet := range daemonSets {
		if _, belongsToProfile := daemonSetNamesBelongingToProfiles[daemonSet.Name]; belongsToProfile {
			continue
		}

		if err = r.client.Delete(ctx, &daemonSet); err != nil {
			return err
		}
	}

	return nil
}

// labelNodesWithProfiles sets the "agent.datadoghq.com/profile" label only in
// the nodes where a profile is applied
func (r *Reconciler) labelNodesWithProfiles(ctx context.Context, profilesByNode map[string]types.NamespacedName) error {
	for nodeName, profileNamespacedName := range profilesByNode {
		isDefaultProfile := agentprofile.IsDefaultProfile(profileNamespacedName.Namespace, profileNamespacedName.Name)

		node := &corev1.Node{}
		if err := r.client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			return err
		}

		_, profileLabelExists := node.Labels[agentprofile.ProfileLabelKey]

		var newLabels map[string]string

		// If the profile is the default one and the label exists in the node,
		// it should be removed.
		if isDefaultProfile && profileLabelExists {
			newLabels = make(map[string]string, len(node.Labels)-1)
			for label, value := range node.Labels {
				if label != agentprofile.ProfileLabelKey {
					newLabels[label] = value
				}
			}
		}

		// If the profile is not the default one and the label does not exist in
		// the node, it should be added.
		if !isDefaultProfile && !profileLabelExists {
			newLabels = make(map[string]string, len(node.Labels)+1)
			for label, value := range node.Labels {
				newLabels[label] = value
			}
			newLabels[agentprofile.ProfileLabelKey] = fmt.Sprintf("%s-%s", profileNamespacedName.Namespace, profileNamespacedName.Name)
		}

		if len(newLabels) == 0 {
			return nil
		}

		patch := corev1.Node{
			TypeMeta:   node.TypeMeta,
			ObjectMeta: node.ObjectMeta,
		}
		patch.Labels = newLabels

		err := r.client.Patch(ctx, &patch, client.MergeFrom(node))
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
	for nodeName, profileNamespacedName := range profilesByNode {
		// Get the agent pods running on the node
		podList := &corev1.PodList{}
		err := r.client.List(
			ctx,
			podList,
			client.MatchingLabels(map[string]string{
				apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultAgentResourceSuffix,
			}),
			client.InNamespace(ddaNamespace),
			client.MatchingFields{"spec.nodeName": nodeName})
		if err != nil {
			return err
		}

		// Delete the agent pods that should be in the node according to the
		// profiles that need to be applied
		isDefaultProfile := agentprofile.IsDefaultProfile(profileNamespacedName.Namespace, profileNamespacedName.Name)
		for _, pod := range podList.Items {
			profileLabelValue, profileLabelExists := pod.Labels[agentprofile.ProfileLabelKey]
			expectedProfileLabelValue := fmt.Sprintf("%s-%s", profileNamespacedName.Namespace, profileNamespacedName.Name)

			deletePod := (isDefaultProfile && profileLabelExists) ||
				(!isDefaultProfile && !profileLabelExists) ||
				(!isDefaultProfile && profileLabelValue != expectedProfileLabelValue)

			if deletePod {
				toDelete := corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: pod.Namespace,
						Name:      pod.Name,
					},
				}
				if err = r.client.Delete(ctx, &toDelete); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (r *Reconciler) agentProfileDaemonSetsCreatedByOperator(ctx context.Context) ([]appsv1.DaemonSet, error) {
	daemonSetList := appsv1.DaemonSetList{}

	if err := r.client.List(ctx, &daemonSetList, client.HasLabels{agentprofile.ProfileLabelKey}); err != nil {
		return nil, err
	}

	return daemonSetList.Items, nil
}
