// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"time"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	componentagent "github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) reconcileV2Agent(logger logr.Logger, requiredComponents feature.RequiredComponents, features []feature.Feature, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var result reconcile.Result
	var eds *edsv1alpha1.ExtendedDaemonSet
	var daemonset *appsv1.DaemonSet
	var podManagers feature.PodTemplateManagers

	daemonsetLogger := logger.WithValues("component", datadoghqv2alpha1.NodeAgentComponentName)

	// requiredComponents needs to be taken into account in case a feature(s) changes and
	// a requiredComponent becomes disabled, in addition to taking into account override.Disabled
	disabledByOverride := false

	agentEnabled := requiredComponents.Agent.IsEnabled()

	if r.options.ExtendedDaemonsetOptions.Enabled {
		// Start by creating the Default Agent extendeddaemonset
		eds = componentagent.NewDefaultAgentExtendedDaemonset(dda, &r.options.ExtendedDaemonsetOptions, requiredComponents.Agent.Containers)
		podManagers = feature.NewPodTemplateManagers(&eds.Spec.Template)

		// Set Global setting on the default extendeddaemonset
		eds.Spec.Template = *override.ApplyGlobalSettings(logger, podManagers, dda, resourcesManager, datadoghqv2alpha1.NodeAgentComponentName)

		// Apply features changes on the Deployment.Spec.Template
		for _, feat := range features {
			if errFeat := feat.ManageNodeAgent(podManagers); errFeat != nil {
				return result, errFeat
			}
		}

		// If Override is defined for the node agent component, apply the override on the PodTemplateSpec, it will cascade to container.
		if componentOverride, ok := dda.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName]; ok {
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
	daemonset = componentagent.NewDefaultAgentDaemonset(dda, requiredComponents)
	podManagers = feature.NewPodTemplateManagers(&daemonset.Spec.Template)
	// Set Global setting on the default daemonset
	daemonset.Spec.Template = *override.ApplyGlobalSettingsMonoSupport(logger, podManagers, dda, resourcesManager,
		datadoghqv2alpha1.NodeAgentComponentName, requiredComponents.Agent.UsesMultiProcessContainer())

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range features {
		if requiredComponents.Agent.UsesMultiProcessContainer() {
			if errFeat := feat.ManageMonoContainerNodeAgent(podManagers); errFeat != nil {
				return result, errFeat
			}
		} else {
			if errFeat := feat.ManageNodeAgent(podManagers); errFeat != nil {
				return result, errFeat
			}
		}
	}

	// If Override is defined for the node agent component, apply the override on the PodTemplateSpec, it will cascade to container.
	if componentOverride, ok := dda.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName]; ok {
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

func updateDSStatusV2WithAgent(dda *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.Agent = datadoghqv2alpha1.UpdateDaemonSetStatus(dda, newStatus.Agent, &updateTime)
	datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, updateTime, datadoghqv2alpha1.AgentReconcileConditionType, status, reason, message, true)
}

func updateEDSStatusV2WithAgent(eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.Agent = datadoghqv2alpha1.UpdateExtendedDaemonSetStatus(eds, newStatus.Agent, &updateTime)
	datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, updateTime, datadoghqv2alpha1.AgentReconcileConditionType, status, reason, message, true)
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
