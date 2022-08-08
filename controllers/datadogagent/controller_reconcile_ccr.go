// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	componentccr "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/override"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) reconcileV2ClusterChecksRunner(logger logr.Logger, features []feature.Feature, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var result reconcile.Result
	var err error

	// Start by creating the Default Cluster-Agent deployment
	deployment := componentccr.NewDefaultClusterChecksRunnerDeployment(dda)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	deployment.Spec.Template = *override.ApplyGlobalSettings(logger, podManagers, dda, resourcesManager, datadoghqv2alpha1.ClusterChecksRunnerComponentName)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range features {
		if errFeat := feat.ManageClusterChecksRunner(podManagers); errFeat != nil {
			return result, errFeat
		}
	}

	// If Override is define for the cluster-checks-runner component, apply the override on the PodTemplateSpec, it will cascade to container.
	if componentOverride, ok := dda.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName]; ok {
		_, err = override.PodTemplateSpec(podManagers, componentOverride)
		if err != nil {
			return result, err
		}

		override.Deployment(deployment, componentOverride)
	}

	deploymentLogger := logger.WithValues("component", datadoghqv2alpha1.ClusterChecksRunnerReconcileConditionType)
	return r.createOrUpdateDeployment(deploymentLogger, dda, deployment, newStatus, updateStatusV2WithClusterChecksRunner)
}

func updateStatusV2WithClusterChecksRunner(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.ClusterChecksRunner = datadoghqv2alpha1.UpdateDeploymentStatus(deployment, newStatus.ClusterChecksRunner, &updateTime)
	datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, updateTime, datadoghqv2alpha1.ClusterChecksRunnerReconcileConditionType, status, reason, message, true)
}
