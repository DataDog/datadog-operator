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

// clusterChecksRunnerGroupLabelKey labels each dedicated runner group's Deployment
// with its group name, so groups removed from spec can be found and cleaned up.
const clusterChecksRunnerGroupLabelKey = "agent.datadoghq.com/clusterchecksrunner-group"

// noopComponentUpdateStatus satisfies createOrUpdateDeployment's status-callback
// parameter. Dedicated runner groups are an experimental feature and don't yet
// have a dedicated status surface on DatadogAgentInternalStatus.
func noopComponentUpdateStatus(*appsv1.Deployment, *v1alpha1.DatadogAgentInternalStatus, metav1.Time, metav1.ConditionStatus, string, string) {
}

// ReconcileClusterChecksRunnerGroups reconciles the additional, dedicated Cluster
// Checks Runner Deployments declared in Spec.Features.ClusterChecks.Runners. It is
// a sibling to the generic ComponentReconciler path (which continues to own the
// single default CCR Deployment unchanged) because that interface is strictly
// one-component-to-one-Deployment and is shared with ClusterAgent/OtelAgentGateway.
func (r *Reconciler) ReconcileClusterChecksRunnerGroups(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	ddai := params.DDAI

	var groups []datadoghqv2alpha1.ClusterChecksRunnerGroup
	// Groups are materialized when a CCR-family Deployment is active (default
	// CCR or the kube-checks-runner-default knob); otherwise ignored.
	if params.RequiredComponents.ClusterAgent.IsEnabled() && constants.IsCCRComponentRequired(ddai, &ddai.Spec) {
		var err error
		groups, err = datadoghqv2alpha1.GetEffectiveClusterChecksRunnerGroups(ddai)
		if err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "ignoring malformed experimental cluster checks runner groups annotation")
			groups = nil
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
	var result reconcile.Result

	deployment := componentccr.NewClusterChecksRunnerGroupDeployment(ddai, &ddai.Spec, group.Name)
	deployment.Labels[clusterChecksRunnerGroupLabelKey] = group.Name
	deployment.Spec.Template.Labels[clusterChecksRunnerGroupLabelKey] = group.Name

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
		return result, utilerrors.NewAggregate(featErrors)
	}

	applyClusterChecksRunnerGroupCompatibility(podManagers, group)

	// Component-level override first; the group's own Override is applied
	// after and wins on conflicts.
	if componentOverride := ddai.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName]; componentOverride != nil {
		override.PodTemplateSpec(objLogger, podManagers, componentOverride, datadoghqv2alpha1.ClusterChecksRunnerComponentName, ddai.Name)
		override.Deployment(deployment, componentOverride)
	}

	if componentOverride := group.Override; componentOverride != nil {
		override.PodTemplateSpec(objLogger, podManagers, componentOverride, datadoghqv2alpha1.ClusterChecksRunnerComponentName, ddai.Name)
		override.Deployment(deployment, componentOverride)
	}

	if errs := global.ValidateFIPSVersions(podManagers); len(errs) > 0 {
		return result, utilerrors.NewAggregate(errs)
	}

	if r.options.RolloutOnConfigMapChangeEnabled {
		if err := r.annotateConfigMapsChecksum(ctx, deployment.Namespace, &deployment.Spec.Template); err != nil {
			return result, err
		}
	}

	return r.createOrUpdateDeployment(ctx, ddai, deployment, params.Status, noopComponentUpdateStatus)
}

// applyClusterChecksRunnerGroupCompatibility injects the group's check
// include/exclude lists as env vars on the runner container (the agent-side
// compat contract). The exclude env is always set, even empty: the feature
// hook run earlier injects the default CCR's auto-derived exclude onto every
// runner template, which must be overwritten with the group's own list.
func applyClusterChecksRunnerGroupCompatibility(podManagers feature.PodTemplateManagers, group datadoghqv2alpha1.ClusterChecksRunnerGroup) {
	if len(group.ChecksInclude) > 0 {
		podManagers.EnvVar().AddEnvVarToContainer(
			apicommon.ClusterChecksRunnersContainerName,
			&corev1.EnvVar{
				Name:  clusterchecksfeature.DDCLCRunnerChecksInclude,
				Value: strings.Join(group.ChecksInclude, ","),
			},
		)
	}
	podManagers.EnvVar().AddEnvVarToContainer(
		apicommon.ClusterChecksRunnersContainerName,
		&corev1.EnvVar{
			Name:  clusterchecksfeature.DDCLCRunnerChecksExclude,
			Value: strings.Join(group.ChecksExclude, ","),
		},
	)
}

// cleanupOrphanedClusterChecksRunnerGroups deletes group Deployments whose
// group name is no longer present in spec (group removed, or CCR/DCA disabled).
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
		groupName, ok := deployment.Labels[clusterChecksRunnerGroupLabelKey]
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
