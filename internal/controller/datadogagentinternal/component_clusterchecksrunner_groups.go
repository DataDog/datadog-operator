// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	clusterchecksfeature "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/clusterchecks"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// Runner groups don't report status yet.
func noopComponentUpdateStatus(*appsv1.Deployment, *v1alpha1.DatadogAgentInternalStatus, metav1.Time, metav1.ConditionStatus, string, string) {
}

// ReconcileClusterChecksRunnerGroups reconciles the runner group Deployments and deletes the ones no longer declared.
// A group that fails doesn't stop the others; the errors are returned together.
func (r *Reconciler) ReconcileClusterChecksRunnerGroups(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	ddai := params.DDAI

	var groups []componentccr.Group
	if params.RequiredComponents.ClusterAgent.IsEnabled() {
		var err error
		if groups, err = componentccr.Groups(ddai, &ddai.Spec); err != nil {
			ctrl.LoggerFrom(ctx).Error(err, "skipping experimental cluster checks runner groups")
		}
	}

	var result reconcile.Result
	var errs []error
	for _, group := range groups {
		res, err := r.reconcileClusterChecksRunnerGroup(ctx, params, group)
		if err != nil {
			errs = append(errs, fmt.Errorf("cluster checks runner group %q: %w", group.Name, err))
		}
		if !res.IsZero() {
			result = res
		}
	}

	if err := r.cleanupOrphanedClusterChecksRunnerGroups(ctx, ddai, groups); err != nil {
		errs = append(errs, err)
	}

	return result, utilerrors.NewAggregate(errs)
}

// activateClusterChecksRunnerGroups returns the deployed groups that need activation and have had an available runner,
// and records them in newStatus. Activation is sticky: a group keeps it while it stays deployed, so an outage doesn't
// move its checks back and restart the Cluster Agent. A group that is no longer deployed loses it.
func (r *Reconciler) activateClusterChecksRunnerGroups(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, newStatus *v1alpha1.DatadogAgentInternalStatus, now metav1.Time) []string {
	logger := ctrl.LoggerFrom(ctx)
	groups, _ := componentccr.Groups(ddai, &ddai.Spec)

	var activated []string
	var statuses []v1alpha1.ClusterChecksRunnerGroupStatus
	var latest []v1alpha1.ClusterChecksRunnerGroupStatus
	latestRead := false
	for _, group := range groups {
		if !componentccr.NeedsActivation(group.Name) {
			continue
		}
		status, found := findGroupStatus(ddai.Status.ClusterChecksRunnerGroups, group.Name)
		if !found {
			// The cached DDAI can lag the status written by the previous reconcile, which also changed the group's
			// Deployment so it no longer looks available; deactivating it then would roll the Cluster Agent back.
			if !latestRead {
				latest, latestRead = r.latestClusterChecksRunnerGroupStatuses(ctx, ddai), true
			}
			status, found = findGroupStatus(latest, group.Name)
		}
		if found {
			activated = append(activated, group.Name)
			statuses = append(statuses, status)
			continue
		}

		available, err := r.clusterChecksRunnerGroupAvailable(ctx, ddai, group.Name)
		if err != nil {
			logger.Error(err, "cannot read the cluster checks runner group Deployment", "clusterChecksRunnerGroup", group.Name)
			continue
		}
		if !available {
			logger.V(1).Info("Waiting for an available runner before sending checks to the cluster checks runner group", "clusterChecksRunnerGroup", group.Name)
			continue
		}

		msg := fmt.Sprintf("Cluster checks runner group %q has an available runner; the Cluster Agent now sends it checks %v", group.Name, group.Checks)
		logger.Info(msg, "clusterChecksRunnerGroup", group.Name)
		r.recorder.Event(ddai, corev1.EventTypeNormal, "ClusterChecksRunnerGroupActivated", msg)
		activated = append(activated, group.Name)
		statuses = append(statuses, v1alpha1.ClusterChecksRunnerGroupStatus{Name: group.Name, ActivatedAt: now})
	}

	newStatus.ClusterChecksRunnerGroups = statuses
	return activated
}

func findGroupStatus(statuses []v1alpha1.ClusterChecksRunnerGroupStatus, groupName string) (v1alpha1.ClusterChecksRunnerGroupStatus, bool) {
	i := slices.IndexFunc(statuses, func(s v1alpha1.ClusterChecksRunnerGroupStatus) bool { return s.Name == groupName })
	if i < 0 {
		return v1alpha1.ClusterChecksRunnerGroupStatus{}, false
	}
	return statuses[i], true
}

// latestClusterChecksRunnerGroupStatuses reads the DDAI's recorded activations from the API server, bypassing the cache.
func (r *Reconciler) latestClusterChecksRunnerGroupStatuses(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal) []v1alpha1.ClusterChecksRunnerGroupStatus {
	reader := r.apiReader
	if reader == nil {
		reader = r.client
	}
	latest := &v1alpha1.DatadogAgentInternal{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(ddai), latest); err != nil {
		ctrl.LoggerFrom(ctx).V(1).Info("cannot read the latest DatadogAgentInternal status", "error", err.Error())
		return nil
	}
	return latest.Status.ClusterChecksRunnerGroups
}

// clusterChecksRunnerGroupAvailable returns whether the group's Deployment, at its current generation, has an
// available runner.
func (r *Reconciler) clusterChecksRunnerGroupAvailable(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, groupName string) (bool, error) {
	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Namespace: ddai.Namespace, Name: componentccr.GetClusterChecksRunnerGroupName(ddai, groupName)}
	if err := r.client.Get(ctx, key, deployment); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas > 0 &&
		deployment.Status.AvailableReplicas > 0, nil
}

func (r *Reconciler) reconcileClusterChecksRunnerGroup(ctx context.Context, params *ReconcileComponentParams, group componentccr.Group) (reconcile.Result, error) {
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

	podManagers.EnvVar().AddEnvVarToContainer(apicommon.ClusterChecksRunnersContainerName, &corev1.EnvVar{
		Name:  clusterchecksfeature.DDCLCRunnerGroup,
		Value: group.Name,
	})

	// The group's own override wins over the component-level one.
	for _, componentOverride := range []*datadoghqv2alpha1.DatadogAgentComponentOverride{runnerOverrideForGroup(ddai.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName]), groupOverride(group.Override)} {
		if componentOverride != nil {
			override.PodTemplateSpec(objLogger, podManagers, componentOverride, datadoghqv2alpha1.ClusterChecksRunnerComponentName, ddai.Name)
			override.Deployment(deployment, componentOverride)
		}
	}

	// The DDA-to-DDAI conversion only stamps the token checksum on the runner override when the default runners are on.
	if dcaOverride := ddai.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName]; dcaOverride != nil {
		if checksum, ok := dcaOverride.Annotations[global.GetDCATokenChecksumAnnotationKey()]; ok {
			podManagers.Annotation().AddAnnotation(global.GetDCATokenChecksumAnnotationKey(), checksum)
		}
	}

	if r.options.UntaintControllerEnabled {
		componentagent.EnsureAgentNotReadyStartupToleration(objLogger, &podManagers.PodTemplateSpec().Spec)
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

// runnerOverrideForGroup returns the component-level override without the fields each group owns: its Deployment name
// and replica count.
func runnerOverrideForGroup(runnerOverride *datadoghqv2alpha1.DatadogAgentComponentOverride) *datadoghqv2alpha1.DatadogAgentComponentOverride {
	if runnerOverride == nil {
		return nil
	}
	runnerOverride = runnerOverride.DeepCopy()
	runnerOverride.Name = nil
	runnerOverride.Replicas = nil
	return runnerOverride
}

// groupOverride returns the group's override without a Deployment name, which the Operator derives from the group name.
func groupOverride(groupOverride *datadoghqv2alpha1.DatadogAgentComponentOverride) *datadoghqv2alpha1.DatadogAgentComponentOverride {
	if groupOverride == nil || groupOverride.Name == nil {
		return groupOverride
	}
	groupOverride = groupOverride.DeepCopy()
	groupOverride.Name = nil
	return groupOverride
}

func (r *Reconciler) cleanupOrphanedClusterChecksRunnerGroups(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, groups []componentccr.Group) error {
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
		return err
	}

	for i := range deploymentList.Items {
		deployment := &deploymentList.Items[i]
		groupName, ok := deployment.Labels[componentccr.ClusterChecksRunnerGroupLabelKey]
		if !ok {
			continue
		}
		if _, stillWanted := wanted[groupName]; stillWanted {
			continue
		}
		if _, err := r.deleteDeploymentWithEvent(ctx, ddai, deployment); err != nil {
			return err
		}
	}

	return nil
}
