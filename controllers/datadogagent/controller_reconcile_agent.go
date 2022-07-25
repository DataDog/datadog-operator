// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/override"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) reconcileV2Agent(logger logr.Logger, features []feature.Feature, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus, requiredContainers []common.AgentContainerName) (reconcile.Result, error) {
	var result reconcile.Result
	var err error

	// TODO for now only support Daemonset (not EDS)

	// Start by creating the Default Cluster-Agent deployment
	daemonset := componentagent.NewDefaultAgentDaemonset(dda, requiredContainers)
	podManagers := feature.NewPodTemplateManagers(&daemonset.Spec.Template)

	// Set Global setting on the default deployment
	daemonset.Spec.Template = *override.ApplyGlobalSettings(logger, podManagers, dda, resourcesManager, datadoghqv2alpha1.NodeAgentComponentName)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range features {
		if errFeat := feat.ManageNodeAgent(podManagers); errFeat != nil {
			return result, errFeat
		}
	}

	// If Override is define for the cluster-checks-runner component, apply the override on the PodTemplateSpec, it will cascade to container.
	if _, ok := dda.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName]; ok {
		_, err = override.PodTemplateSpec(podManagers, dda.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName])
		if err != nil {
			return result, err
		}
	}

	daemonsetLogger := logger.WithValues("component", datadoghqv2alpha1.NodeAgentComponentName)
	return r.createOrUpdateDaemonset(daemonsetLogger, dda, daemonset, newStatus, updateStatusV2WithAgent)
}

func updateStatusV2WithAgent(dda *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.Agent = datadoghqv2alpha1.UpdateDaemonSetStatus(dda, newStatus.Agent, &updateTime)
	datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, updateTime, datadoghqv2alpha1.AgentReconcileConditionType, status, reason, message, true)
}
