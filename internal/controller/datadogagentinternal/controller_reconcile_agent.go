// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"fmt"
	"time"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (r *Reconciler) reconcileV2Agent(ctx context.Context, requiredComponents feature.RequiredComponents, features []feature.Feature,
	ddai *datadoghqv1alpha1.DatadogAgentInternal, resourcesManager feature.ResourceManagers, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, provider string) (reconcile.Result, error) {
	var result reconcile.Result
	var eds *edsv1alpha1.ExtendedDaemonSet
	var daemonset *appsv1.DaemonSet
	var podManagers feature.PodTemplateManagers

	daemonsetLogger := ctrl.LoggerFrom(ctx).WithValues("component", datadoghqv2alpha1.NodeAgentComponentName)
	ctx = ctrl.LoggerInto(ctx, daemonsetLogger)

	// requiredComponents needs to be taken into account in case a feature(s) changes and
	// a requiredComponent becomes disabled, in addition to taking into account override.Disabled
	disabledByOverride := false

	agentEnabled := requiredComponents.Agent.IsEnabled()
	singleContainerStrategyEnabled := requiredComponents.Agent.SingleContainerStrategyEnabled()

	// Windows is enabled ONLY on a profile-labeled DDAI carrying provider=windows (generated from
	// a DatadogAgentProfile that targets Windows nodes). A default DDAI must never be Windows-ified
	// — that would turn the cluster-wide node agent Windows-only and strand Linux nodes — so a
	// provider=windows annotation set directly on the DatadogAgent is intentionally ignored.
	windowsProfile := provider == kubernetes.WindowsProvider && isDDAILabeledWithProfile(ddai)

	// When EDS is enabled and there are profiles defined, we only create an
	// EDS for the default profile, for the other profiles we create
	// DaemonSets.
	// This is to make deployments simpler. With multiple EDS there would be
	// multiple canaries, etc.
	if r.options.ExtendedDaemonsetOptions.Enabled && !isDDAILabeledWithProfile(ddai) {
		// Note: Windows is only applied on profile-labeled DDAIs (see windowsProfile below), which
		// never reach this branch. A default DDAI with a stray provider=windows annotation builds a
		// normal Linux ExtendedDaemonSet here (the annotation is ignored) so Linux nodes keep their
		// agent rather than being stranded.
		// Start by creating the Default Agent extendeddaemonset
		eds = componentagent.NewDefaultAgentExtendedDaemonset(ddai, &r.options.ExtendedDaemonsetOptions, requiredComponents.Agent)
		objLogger := daemonsetLogger.WithValues("object.kind", "ExtendedDaemonSet", "object.namespace", eds.Namespace, "object.name", eds.Name)
		podManagers = feature.NewPodTemplateManagers(&eds.Spec.Template)

		// Set Global setting on the default extendeddaemonset
		global.ApplyGlobalSettingsNodeAgent(objLogger, podManagers, ddai.GetObjectMeta(), &ddai.Spec, resourcesManager, singleContainerStrategyEnabled, requiredComponents)

		// Apply provider-conditional global (default-layer) mutations. Runs after
		// global settings.
		global.ApplyGlobalNodeAgentSpec(podManagers, provider)

		// Apply features changes on the Deployment.Spec.Template.
		// Provider capabilities are applied immediately after each feature's ManageNodeAgent
		// so that each feature owns its provider correctness independently.
		for _, feat := range features {
			if errFeat := feat.ManageNodeAgent(podManagers); errFeat != nil {
				return result, errFeat
			}
			if paf, ok := feat.(feature.ProviderAwareFeature); ok {
				providercaps.ApplyProviderCapabilities(podManagers, provider, paf.NodeAgentProviderCapabilities())
			}
		}

		// If Override is defined for the node agent component, apply the override on the PodTemplateSpec, it will cascade to container.
		var componentOverrides []*datadoghqv2alpha1.DatadogAgentComponentOverride
		if componentOverride, ok := ddai.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName]; ok {
			componentOverrides = append(componentOverrides, componentOverride)
		}

		for _, componentOverride := range componentOverrides {
			if apiutils.BoolValue(componentOverride.Disabled) {
				disabledByOverride = true
			}
			override.PodTemplateSpec(objLogger, podManagers, componentOverride, datadoghqv2alpha1.NodeAgentComponentName, ddai.Name)
			override.ExtendedDaemonSet(eds, componentOverride)
		}

		experimental.ApplyExperimentalOverrides(objLogger, ddai, podManagers)

		if r.options.UntaintControllerEnabled {
			componentagent.EnsureAgentNotReadyStartupToleration(objLogger, &podManagers.PodTemplateSpec().Spec)
		}

		if disabledByOverride {
			if agentEnabled {
				// The override supersedes what's set in requiredComponents; update status to reflect the conflict
				condition.UpdateDatadogAgentInternalStatusConditions(
					newStatus,
					metav1.NewTime(time.Now()),
					common.OverrideReconcileConflictConditionType,
					metav1.ConditionTrue,
					"OverrideConflict",
					"Agent component is set to disabled",
					true,
				)
			}
			if err := r.deleteV2ExtendedDaemonSet(ctx, ddai, eds, newStatus); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}

		return r.createOrUpdateExtendedDaemonset(ctx, ddai, eds, newStatus, updateEDSStatusV2WithAgent)
	}

	// Start by creating the Default Agent daemonset
	daemonset = componentagent.NewDefaultAgentDaemonset(ddai, &r.options.ExtendedDaemonsetOptions, requiredComponents.Agent, component.GetAgentName(ddai))
	objLogger := daemonsetLogger.WithValues("object.kind", "DaemonSet", "object.namespace", daemonset.Namespace, "object.name", daemonset.Name)
	podManagers = feature.NewPodTemplateManagers(&daemonset.Spec.Template)

	// Set Global setting on the default daemonset
	global.ApplyGlobalSettingsNodeAgent(objLogger, podManagers, ddai.GetObjectMeta(), &ddai.Spec, resourcesManager, singleContainerStrategyEnabled, requiredComponents)

	// Apply provider-conditional global (default-layer) mutations. Runs after
	// global settings.
	global.ApplyGlobalNodeAgentSpec(podManagers, provider)

	// Apply features changes on the Deployment.Spec.Template.
	// Provider capabilities are applied immediately after each feature's ManageNodeAgent
	// so that each feature owns its provider correctness independently.
	for _, feat := range features {
		// On the Windows provider path, features the support matrix marks Excluded do not run
		// their node-agent hooks; they'd inject config the Windows agent can't use
		// (eBPF/system-probe, etc.). The subsequent strip is the safety net for what slips through.
		if windowsProfile && feature.FeatureSupportLevel(provider, feat.ID()) == feature.Excluded {
			continue
		}
		if singleContainerStrategyEnabled {
			if errFeat := feat.ManageSingleContainerNodeAgent(podManagers); errFeat != nil {
				return result, errFeat
			}
		} else {
			if errFeat := feat.ManageNodeAgent(podManagers); errFeat != nil {
				return result, errFeat
			}
		}
		// Apply provider capabilities after the feature's manage step regardless of
		// container strategy, so colocated provider mutations are not silently
		// dropped in single-container mode.
		if paf, ok := feat.(feature.ProviderAwareFeature); ok {
			providercaps.ApplyProviderCapabilities(podManagers, provider, paf.NodeAgentProviderCapabilities())
		}
	}

	// If Override is defined for the node agent component, apply the override on the PodTemplateSpec, it will cascade to container.
	var componentOverrides []*datadoghqv2alpha1.DatadogAgentComponentOverride
	if componentOverride, ok := ddai.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName]; ok {
		componentOverrides = append(componentOverrides, componentOverride)
	}

	for _, componentOverride := range componentOverrides {
		if apiutils.BoolValue(componentOverride.Disabled) {
			disabledByOverride = true
		}
		override.PodTemplateSpec(objLogger, podManagers, componentOverride, datadoghqv2alpha1.NodeAgentComponentName, ddai.Name)
		override.DaemonSet(daemonset, componentOverride)
	}

	experimental.ApplyExperimentalOverrides(objLogger, ddai, podManagers)

	if r.options.UntaintControllerEnabled {
		componentagent.EnsureAgentNotReadyStartupToleration(objLogger, &podManagers.PodTemplateSpec().Spec)
	}

	// Windows profile (DatadogAgentProfile targeting Windows nodes): the DaemonSet was built,
	// featured and overridden above like a Linux agent; now Windows-ify the pod contents.
	if windowsProfile {
		// FIPS + Windows is not supported: there is no -servercore-fips image, and the Linux
		// fips-proxy sidecar/config is stripped on Windows. Both FIPS mechanisms must be caught —
		// useFIPSAgent (FIPS agent flavor) AND fips.enabled (fips-proxy sidecar) — otherwise the
		// Windows agent would silently run non-FIPS (a compliance risk). Surface a condition and
		// remove the Windows DaemonSet instead of downgrading.
		fipsEnabled := ddai.Spec.Global != nil &&
			(apiutils.BoolValue(ddai.Spec.Global.UseFIPSAgent) ||
				(ddai.Spec.Global.FIPS != nil && apiutils.BoolValue(ddai.Spec.Global.FIPS.Enabled)))
		// Also catch FIPS requested via a -fips agent image tag override, which would otherwise be
		// silently rewritten to a non-FIPS -servercore image.
		if fipsEnabled || componentagent.HasFIPSAgentImage(&daemonset.Spec.Template.Spec) {
			condition.UpdateDatadogAgentInternalStatusConditions(
				newStatus,
				metav1.NewTime(time.Now()),
				"WindowsAgentReconcile",
				metav1.ConditionFalse,
				"WindowsFIPSUnsupported",
				"the windows provider is not supported with FIPS (global.useFIPSAgent or global.fips.enabled): no -servercore-fips image",
				true,
			)
			daemonsetLogger.Info("Removing Windows DaemonSet: FIPS (useFIPSAgent or fips.enabled) is enabled and FIPS is unsupported on Windows")
			// Delete any existing Windows DaemonSet so a non-FIPS agent does not keep running
			// under a FIPS-required configuration (compliance), rather than silently downgrading.
			if err := r.deleteV2DaemonSet(ctx, ddai, daemonset, newStatus); err != nil {
				return reconcile.Result{}, err
			}
			// Clear the Agent status unconditionally: deleteV2DaemonSet returns early (without
			// clearing status) if the DaemonSet is already gone, which would otherwise leave a
			// stale Agent status on the profile while FIPS blocks recreation.
			newStatus.Agent = nil
			return reconcile.Result{}, nil
		}
		// Windows is supported for this DDAI: clear any prior WindowsFIPSUnsupported condition
		// (e.g. after FIPS is turned back off) so the status reflects the healthy state.
		condition.UpdateDatadogAgentInternalStatusConditions(
			newStatus,
			metav1.NewTime(time.Now()),
			"WindowsAgentReconcile",
			metav1.ConditionTrue,
			"WindowsAgentReconcile",
			"windows agent reconciled",
			false,
		)
		logCollectionEnabled := false
		for _, feat := range features {
			if feat.ID() == feature.LogCollectionIDType {
				logCollectionEnabled = true
				break
			}
		}
		skippedContainerLogsPath := componentagent.ApplyWindowsPodTransformation(&daemonset.Spec.Template, ddai, logCollectionEnabled, windowsLogPaths(ddai))
		// Surface a configured containerLogsPath that overlaps the agent config dir and was dropped
		// (mounting it would re-shadow the seeded config); clear the condition otherwise so a fixed
		// path doesn't leave a stale warning. See AddWindowsLogCollectionVolumes.
		skipStatus := metav1.ConditionFalse
		skipMsg := "no overlapping containerLogsPath"
		if skippedContainerLogsPath != "" {
			skipStatus = metav1.ConditionTrue
			skipMsg = fmt.Sprintf("logCollection.containerLogsPath %q overlaps the Windows agent config dir C:/ProgramData/Datadog and was not mounted; set it to the container-runtime log-store subdirectory (a sibling of the config dir) instead", skippedContainerLogsPath)
			daemonsetLogger.Info("Windows log collection: skipping overlapping containerLogsPath", "containerLogsPath", skippedContainerLogsPath)
		}
		condition.UpdateDatadogAgentInternalStatusConditions(
			newStatus,
			metav1.NewTime(time.Now()),
			"WindowsLogCollectionPathSkipped",
			skipStatus,
			"OverlappingContainerLogsPath",
			skipMsg,
			// Only record True (a real skip) or clear an existing condition; don't create a
			// permanent False on healthy clusters that never configured an overlapping path.
			false,
		)
	}

	if disabledByOverride {
		if agentEnabled {
			// The override supersedes what's set in requiredComponents; update status to reflect the conflict
			condition.UpdateDatadogAgentInternalStatusConditions(
				newStatus,
				metav1.NewTime(time.Now()),
				common.OverrideReconcileConflictConditionType,
				metav1.ConditionTrue,
				"OverrideConflict",
				"Agent component is set to disabled",
				true,
			)
		}
		if err := r.deleteV2DaemonSet(ctx, ddai, daemonset, newStatus); err != nil {
			return reconcile.Result{}, err
		}
		deleteStatusWithAgent(newStatus)
		return reconcile.Result{}, nil
	}

	rolloutBudget := preparedRolloutBudget(ddai, &r.options.ExtendedDaemonsetOptions)
	rolloutEnabled := preparedRolloutEnabled(ddai)
	var currentDaemonSet *appsv1.DaemonSet
	if rolloutEnabled {
		reader := r.apiReader
		if reader == nil {
			reader = r.client
		}
		currentDaemonSet = &appsv1.DaemonSet{}
		if getErr := reader.Get(ctx, client.ObjectKeyFromObject(daemonset), currentDaemonSet); getErr != nil {
			if !errors.IsNotFound(getErr) {
				return reconcile.Result{}, getErr
			}
			currentDaemonSet = nil
		}
	}
	affinityMigration, prepareErr := configurePreparedRollout(ddai, daemonset, currentDaemonSet, rolloutBudget)
	if prepareErr != nil {
		objLogger.Error(prepareErr, "Prepared Agent rollout request is incompatible with the rendered Pod template")
		if r.recorder != nil {
			r.recorder.Eventf(ddai, corev1.EventTypeWarning, "AgentPreparedRolloutRejected", "Prepared Agent rollout is disabled for this template: %v", prepareErr)
		}
		return reconcile.Result{}, prepareErr
	}
	result, err := r.createOrUpdateDaemonset(ctx, ddai, daemonset, newStatus, updateDSStatusV2WithAgent)
	if err != nil || !rolloutEnabled {
		return result, err
	}
	if affinityMigration {
		if result.RequeueAfter == 0 || result.RequeueAfter > time.Second {
			result.RequeueAfter = time.Second
		}
		return result, nil
	}
	fallbackResult, fallbackErr := r.reconcileResourceFallback(ctx, ddai, daemonset, rolloutBudget)
	if fallbackErr != nil {
		return reconcile.Result{}, fallbackErr
	}
	if fallbackResult.RequeueAfter > 0 && (result.RequeueAfter == 0 || fallbackResult.RequeueAfter < result.RequeueAfter) {
		result.RequeueAfter = fallbackResult.RequeueAfter
	}
	return result, nil
}

func updateDSStatusV2WithAgent(dsName string, ds *appsv1.DaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.Agent = condition.UpdateDaemonSetStatusDDAI(dsName, ds, newStatus.Agent, &updateTime)
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, updateTime, common.AgentReconcileConditionType, status, reason, message, true)
}

func updateEDSStatusV2WithAgent(eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.Agent = condition.UpdateExtendedDaemonSetStatusDDAI(eds, newStatus.Agent, &updateTime)
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, updateTime, common.AgentReconcileConditionType, status, reason, message, true)
}

func (r *Reconciler) deleteV2DaemonSet(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, ds *appsv1.DaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus) error {
	err := r.client.Delete(ctx, ds)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	ctrl.LoggerFrom(ctx).WithValues("object.kind", "DaemonSet", "object.namespace", ds.Namespace, "object.name", ds.Name).Info("Deleted DaemonSet")
	event := buildEventInfo(ds.Name, ds.Namespace, kubernetes.DaemonSetKind, datadog.DeletionEvent)
	r.recordEvent(ddai, event)
	newStatus.Agent = nil

	return nil
}

func (r *Reconciler) deleteV2ExtendedDaemonSet(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus) error {
	err := r.client.Delete(ctx, eds)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	ctrl.LoggerFrom(ctx).WithValues("object.kind", "ExtendedDaemonSet", "object.namespace", eds.Namespace, "object.name", eds.Name).Info("Deleted ExtendedDaemonSet")
	event := buildEventInfo(eds.Name, eds.Namespace, kubernetes.ExtendedDaemonSetKind, datadog.DeletionEvent)
	r.recordEvent(ddai, event)
	newStatus.Agent = nil

	return nil
}

func deleteStatusWithAgent(newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus) {
	newStatus.Agent = nil
	condition.DeleteDatadogAgentInternalStatusCondition(newStatus, common.AgentReconcileConditionType)
}

// isDDAILabeledWithProfile returns true if the DDAI is labeled with a non-default profile.
// This is used to determine whether or not the EDS should be created.
func isDDAILabeledWithProfile(ddai *datadoghqv1alpha1.DatadogAgentInternal) bool {
	labels := ddai.GetLabels()
	if labels == nil {
		return false
	}
	return labels[constants.ProfileLabelKey] != ""
}

// cleanupExtraneousDaemonSets deletes DSs/EDSs that no longer apply.
// Use cases include deleting old DSs/EDSs when:
// - a DaemonSet's name is changed using node overrides
// - introspection is disabled or enabled
// - a profile is deleted
// func (r *Reconciler) cleanupExtraneousDaemonSets(ctx context.Context, logger logr.Logger, ddai *datadoghqv1alpha1.DatadogAgentInternal, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus) error {
// 	matchLabels := client.MatchingLabels{
// 		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
// 		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
// 		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
// 	}

// 	dsName := component.GetDaemonSetNameFromDatadogAgent(ddai)
// 	validDaemonSetNames, validExtendedDaemonSetNames := r.getValidDaemonSetNames(dsName)

// 	// Only the default profile uses an EDS when profiles are enabled
// 	// Multiple EDSs can be created with introspection
// 	if r.options.ExtendedDaemonsetOptions.Enabled {
// 		edsList := edsv1alpha1.ExtendedDaemonSetList{}
// 		if err := r.client.List(ctx, &edsList, matchLabels); err != nil {
// 			return err
// 		}

// 		for _, eds := range edsList.Items {
// 			if _, ok := validExtendedDaemonSetNames[eds.Name]; !ok {
// 				if err := r.deleteV2ExtendedDaemonSet(logger, ddai, &eds, newStatus); err != nil {
// 					return err
// 				}
// 			}
// 		}
// 	}

// 	daemonSetList := appsv1.DaemonSetList{}
// 	if err := r.client.List(ctx, &daemonSetList, matchLabels); err != nil {
// 		return err
// 	}

// 	for _, daemonSet := range daemonSetList.Items {
// 		if _, ok := validDaemonSetNames[daemonSet.Name]; !ok {
// 			if err := r.deleteV2DaemonSet(logger, ddai, &daemonSet, newStatus); err != nil {
// 				return err
// 			}
// 		}
// 	}

// 	return nil
// }

// getValidDaemonSetNames generates a list of valid DS and EDS names
// func (r *Reconciler) getValidDaemonSetNames(dsName string) (map[string]struct{}, map[string]struct{}) {
// 	validDaemonSetNames := map[string]struct{}{}
// 	validExtendedDaemonSetNames := map[string]struct{}{}

// 	if r.options.ExtendedDaemonsetOptions.Enabled {
// 		validExtendedDaemonSetNames = map[string]struct{}{
// 			dsName: {},
// 		}
// 	} else {
// 		validDaemonSetNames = map[string]struct{}{
// 			dsName: {},
// 		}
// 	}

// 	return validDaemonSetNames, validExtendedDaemonSetNames
// }
