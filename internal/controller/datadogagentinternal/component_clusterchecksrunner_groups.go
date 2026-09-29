// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	clusterchecksfeature "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/clusterchecks"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// noopComponentUpdateStatus satisfies createOrUpdateDeployment's status-callback
// parameter. Dedicated runner groups are an experimental feature and don't yet
// have a dedicated status surface on DatadogAgentInternalStatus.
func noopComponentUpdateStatus(*appsv1.Deployment, *v1alpha1.DatadogAgentInternalStatus, metav1.Time, metav1.ConditionStatus, string, string) {
}

// ReconcileClusterChecksRunnerGroups reconciles the dedicated Cluster Checks
// Runner Deployments declared by the experimental runner groups annotations. It
// is a sibling to the generic ComponentReconciler path (which owns the single
// default CCR Deployment) because that interface is strictly
// one-component-to-one-Deployment.
func (r *Reconciler) ReconcileClusterChecksRunnerGroups(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	ddai := params.DDAI

	var groups []datadoghqv2alpha1.ClusterChecksRunnerGroup
	if params.RequiredComponents.ClusterAgent.IsEnabled() && constants.IsClusterChecksEnabled(&ddai.Spec) {
		var err error
		if groups, err = datadoghqv2alpha1.GetEffectiveClusterChecksRunnerGroups(ddai); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "ignoring experimental cluster checks runner groups")
		}
	}

	var result reconcile.Result
	for _, group := range groups {
		res, err := r.reconcileClusterChecksRunnerGroup(ctx, params, group)
		if err != nil {
			return res, err
		}
		if !res.IsZero() {
			result = res
		}
	}

	res, err := r.cleanupOrphanedClusterChecksRunnerGroups(ctx, ddai, groups)
	if err != nil {
		return res, err
	}
	if !res.IsZero() {
		result = res
	}

	return result, nil
}

func (r *Reconciler) reconcileClusterChecksRunnerGroup(ctx context.Context, params *ReconcileComponentParams, group datadoghqv2alpha1.ClusterChecksRunnerGroup) (reconcile.Result, error) {
	ddai := params.DDAI
	deployment := componentccr.NewClusterChecksRunnerGroupDeployment(ddai, &ddai.Spec, group.Name)

	objLogger := ctrl.LoggerFrom(ctx).WithValues("object.kind", "Deployment", "object.namespace", deployment.Namespace, "object.name", deployment.Name, "clusterChecksRunnerGroup", group.Name)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	global.ApplyGlobalSettingsClusterChecksRunner(objLogger, podManagers, ddai, &ddai.Spec, params.ResourceManagers, params.RequiredComponents)

	var featErrors []error
	for _, feat := range params.Features {
		if err := feat.ManageClusterChecksRunner(podManagers); err != nil {
			featErrors = append(featErrors, err)
		}
	}
	if len(featErrors) > 0 {
		return reconcile.Result{}, utilerrors.NewAggregate(featErrors)
	}

	applyClusterChecksRunnerGroupCompatibility(podManagers, group)

	// Component-level override first; the group's own Override wins on conflicts.
	for _, componentOverride := range []*datadoghqv2alpha1.DatadogAgentComponentOverride{ddai.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName], group.Override} {
		if componentOverride != nil {
			override.PodTemplateSpec(objLogger, podManagers, componentOverride, datadoghqv2alpha1.ClusterChecksRunnerComponentName, ddai.Name)
			override.Deployment(deployment, componentOverride)
		}
	}

	if errs := global.ValidateFIPSVersions(podManagers); len(errs) > 0 {
		return reconcile.Result{}, utilerrors.NewAggregate(errs)
	}

	if r.options.RolloutOnConfigMapChangeEnabled {
		if err := r.annotateConfigMapsChecksum(ctx, deployment.Namespace, &deployment.Spec.Template); err != nil {
			return reconcile.Result{}, err
		}
	}

	return r.createOrUpdateDeployment(ctx, ddai, deployment, params.Status, noopComponentUpdateStatus)
}

// applyClusterChecksRunnerGroupCompatibility sets the group's include list on
// the runner container, and clears the exclude env that the clusterchecks
// feature injects on every runner template for the default CCR.
func applyClusterChecksRunnerGroupCompatibility(podManagers feature.PodTemplateManagers, group datadoghqv2alpha1.ClusterChecksRunnerGroup) {
	for _, env := range []*corev1.EnvVar{
		{Name: clusterchecksfeature.DDCLCRunnerChecksInclude, Value: strings.Join(group.ChecksInclude, " ")},
		{Name: clusterchecksfeature.DDCLCRunnerChecksExclude, Value: ""},
	} {
		podManagers.EnvVar().AddEnvVarToContainer(apicommon.ClusterChecksRunnersContainerName, env)
	}
}

// cleanupOrphanedClusterChecksRunnerGroups deletes group Deployments whose
// group is no longer declared (group removed, or cluster checks/DCA disabled).
func (r *Reconciler) cleanupOrphanedClusterChecksRunnerGroups(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, groups []datadoghqv2alpha1.ClusterChecksRunnerGroup) (reconcile.Result, error) {
	wanted := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		wanted[group.Name] = struct{}{}
	}

	matchLabels := client.MatchingLabels{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
	}
	deploymentList := appsv1.DeploymentList{}
	if err := r.client.List(ctx, &deploymentList, matchLabels); err != nil {
		return reconcile.Result{}, err
	}

	for i := range deploymentList.Items {
		deployment := &deploymentList.Items[i]
		groupName, ok := deployment.Labels[componentccr.ClusterChecksRunnerGroupLabelKey]
		if !ok {
			// Not a group Deployment (e.g. the default CCR Deployment shares the component label).
			continue
		}
		if _, stillWanted := wanted[groupName]; stillWanted {
			continue
		}
		if _, err := r.deleteDeploymentWithEvent(ctx, ddai, deployment); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
