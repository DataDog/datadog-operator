// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"time"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (r *Reconciler) reconcileV2Agent(logger logr.Logger, requiredComponents feature.RequiredComponents, features []feature.Feature,
	ddai *datadoghqv1alpha1.DatadogAgentInternal, resourcesManager feature.ResourceManagers, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus) (reconcile.Result, error) {
	var result reconcile.Result
	var eds *edsv1alpha1.ExtendedDaemonSet
	var daemonset *appsv1.DaemonSet
	var podManagers feature.PodTemplateManagers

	// TODO: temporary fix for DDAI object name
	// Use DDA name instead of DDAI name
	ddaiCopy := ddai.DeepCopy()
	ddaiCopy.Name = ddaiCopy.Labels[apicommon.DatadogAgentNameLabelKey]

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
	if r.options.ExtendedDaemonsetOptions.Enabled && !isDDAILabeledWithProfile(ddaiCopy) {
		// Start by creating the Default Agent extendeddaemonset
		eds = componentagent.NewDefaultAgentExtendedDaemonset(ddaiCopy, &r.options.ExtendedDaemonsetOptions, requiredComponents.Agent)
		podManagers = feature.NewPodTemplateManagers(&eds.Spec.Template)

		// Set Global setting on the default extendeddaemonset
		global.ApplyGlobalSettingsNodeAgent(logger, podManagers, ddaiCopy.GetObjectMeta(), &ddaiCopy.Spec, resourcesManager, singleContainerStrategyEnabled, requiredComponents)

		// Apply features changes on the Deployment.Spec.Template
		for _, feat := range features {
			if errFeat := feat.ManageNodeAgent(podManagers, ""); errFeat != nil {
				return result, errFeat
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
			override.PodTemplateSpec(logger, podManagers, componentOverride, datadoghqv2alpha1.NodeAgentComponentName, ddaiCopy.Name)
			override.ExtendedDaemonSet(eds, componentOverride)
		}

		experimental.ApplyExperimentalOverrides(logger, ddaiCopy, podManagers)

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
			if err := r.deleteV2ExtendedDaemonSet(daemonsetLogger, ddai, eds, newStatus); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}

		return r.createOrUpdateExtendedDaemonset(daemonsetLogger, ddai, eds, newStatus, updateEDSStatusV2WithAgent)
	}

	// Start by creating the Default Agent daemonset
	daemonset = componentagent.NewDefaultAgentDaemonset(ddaiCopy, &r.options.ExtendedDaemonsetOptions, requiredComponents.Agent)
	podManagers = feature.NewPodTemplateManagers(&daemonset.Spec.Template)
	// Set Global setting on the default daemonset
	global.ApplyGlobalSettingsNodeAgent(logger, podManagers, ddaiCopy.GetObjectMeta(), &ddaiCopy.Spec, resourcesManager, singleContainerStrategyEnabled, requiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range features {
		if singleContainerStrategyEnabled {
			if errFeat := feat.ManageSingleContainerNodeAgent(podManagers, ""); errFeat != nil {
				return result, errFeat
			}
		} else {
			if errFeat := feat.ManageNodeAgent(podManagers, ""); errFeat != nil {
				return result, errFeat
			}
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
		override.PodTemplateSpec(logger, podManagers, componentOverride, datadoghqv2alpha1.NodeAgentComponentName, ddaiCopy.Name)
		override.DaemonSet(daemonset, componentOverride)
	}

	experimental.ApplyExperimentalOverrides(logger, ddaiCopy, podManagers)

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
		if err := r.deleteV2DaemonSet(daemonsetLogger, ddai, daemonset, newStatus); err != nil {
			return reconcile.Result{}, err
		}
		deleteStatusWithAgent(newStatus)
		return reconcile.Result{}, nil
	}

	return r.createOrUpdateDaemonset(daemonsetLogger, ddai, daemonset, newStatus, updateDSStatusV2WithAgent)
}

func updateDSStatusV2WithAgent(dsName string, ds *appsv1.DaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.Agent = condition.UpdateDaemonSetStatusDDAI(dsName, ds, newStatus.Agent, &updateTime)
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, updateTime, common.AgentReconcileConditionType, status, reason, message, true)
}

func updateEDSStatusV2WithAgent(eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.Agent = condition.UpdateExtendedDaemonSetStatusDDAI(eds, newStatus.Agent, &updateTime)
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, updateTime, common.AgentReconcileConditionType, status, reason, message, true)
}

func (r *Reconciler) deleteV2DaemonSet(logger logr.Logger, ddai *datadoghqv1alpha1.DatadogAgentInternal, ds *appsv1.DaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus) error {
	err := r.client.Delete(context.TODO(), ds)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	logger.Info("Delete DaemonSet", "daemonSet.Namespace", ds.Namespace, "daemonSet.Name", ds.Name)
	event := buildEventInfo(ds.Name, ds.Namespace, kubernetes.DaemonSetKind, datadog.DeletionEvent)
	r.recordEvent(ddai, event)
	newStatus.Agent = nil

	return nil
}

func (r *Reconciler) deleteV2ExtendedDaemonSet(logger logr.Logger, ddai *datadoghqv1alpha1.DatadogAgentInternal, eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus) error {
	err := r.client.Delete(context.TODO(), eds)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	logger.Info("Delete DaemonSet", "extendedDaemonSet.Namespace", eds.Namespace, "extendedDaemonSet.Name", eds.Name)
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
	return labels[agentprofile.ProfileLabelKey] != ""
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
