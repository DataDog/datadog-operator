// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"maps"

	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func sendProfileEnabledMetric(enabled bool) {
	if enabled {
		metrics.DAPEnabled.Set(metrics.TrueValue)
	} else {
		metrics.DAPEnabled.Set(metrics.FalseValue)
	}
}

// setProfileCondition updates a single condition on the profile in place.
func setProfileCondition(profile *v1alpha1.DatadogAgentProfile, conditionType string, status metav1.ConditionStatus, now metav1.Time, reason, message string) {
	profile.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(
		profile.Status.Conditions,
		agentprofile.NewDatadogAgentProfileCondition(conditionType, status, now, reason, message),
	)
}

// reconcileProfiles reconciles all profiles
// - returns a list of profiles that should be applied (including the default profile)
// - configures node labels based on the profiles that are applied
// - applies profile status updates in k8s
func (r *Reconciler) reconcileProfiles(ctx context.Context, dsNSName types.NamespacedName, ddaEDSMaxUnavailable intstr.IntOrString, defaultDDAI *v1alpha1.DatadogAgentInternal) ([]*v1alpha1.DatadogAgentProfile, error) {
	logger := ctrl.LoggerFrom(ctx)
	now := metav1.Now()
	// start with the default profile so that on error, at minimum the default profile is applied
	defaultProfile := agentprofile.DefaultProfile()
	appliedProfiles := []*v1alpha1.DatadogAgentProfile{&defaultProfile}
	// accumulatedDefaultSpec config starts as the DDA-generated default and
	// changes with each accepted profile's shared overlay contributions.
	baseDefaultSpec := defaultDDAI.Spec.DeepCopy()
	accumulatedDefaultSpec := defaultDDAI.Spec.DeepCopy()

	// list and sort profiles
	profilesList := v1alpha1.DatadogAgentProfileList{}
	if err := r.client.List(ctx, &profilesList); err != nil {
		return appliedProfiles, fmt.Errorf("unable to list DatadogAgentProfiles: %w", err)
	}
	sortedProfiles := agentprofile.SortProfiles(profilesList.Items)

	nodeList, err := r.getNodeList(ctx)
	if err != nil {
		return appliedProfiles, fmt.Errorf("unable to get node list: %w", err)
	}

	// profilesByNode maps node name to profile
	// it is pre-populated with the default profile
	profilesByNode := make(map[string]types.NamespacedName, len(nodeList))
	for _, node := range nodeList {
		profilesByNode[node.Name] = types.NamespacedName{Namespace: "", Name: "default"}
	}

	// csInfo holds create strategy data per profile
	csInfo := make(map[types.NamespacedName]*agentprofile.CreateStrategyInfo)

	for _, profile := range sortedProfiles {
		originalStatus := profile.Status.DeepCopy()
		logger.Info("reconciling profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)

		requirements, err := agentprofile.ValidateProfileAndReturnRequirements(&profile)
		if err != nil {
			metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.FalseValue)
			setProfileCondition(&profile, agentprofile.ValidConditionType, metav1.ConditionFalse, now, agentprofile.InvalidConditionReason, err.Error())
			setProfileCondition(&profile, agentprofile.AppliedConditionType, metav1.ConditionUnknown, now, agentprofile.InvalidConditionReason, "Profile is invalid")
			logger.Error(err, "unable to reconcile profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			r.syncProfileStatus(ctx, &profile, originalStatus, now)
			continue
		}
		metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.TrueValue)
		setProfileCondition(&profile, agentprofile.ValidConditionType, metav1.ConditionTrue, now, agentprofile.ValidConditionReason, "Valid manifest")

		if err := agentprofile.CheckProfileNodeConflicts(profile.ObjectMeta, requirements, nodeList, profilesByNode); err != nil {
			setProfileCondition(&profile, agentprofile.AppliedConditionType, metav1.ConditionFalse, now, agentprofile.ConflictConditionReason, "Conflict with existing profile")
			logger.Error(err, "unable to reconcile profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			r.syncProfileStatus(ctx, &profile, originalStatus, now)
			continue
		}

		// Modify default DDAI spec with shared configs, e.g. DCA configs.
		// Spec changes are applied later if there is no error.
		// candidateDefaultSpec = accumulated spec config from default DDAI and profiles.
		// baseDefaultSpec = original default DDAI spec before any profile overlays were applied (used to detect user-configured vs defaulted configs)
		candidateDefaultSpec := accumulatedDefaultSpec.DeepCopy()
		if err := feature.ApplyProfileSharedConfigOverlays(candidateDefaultSpec, baseDefaultSpec, profile.Spec.Config); err != nil {
			setProfileCondition(&profile, agentprofile.AppliedConditionType, metav1.ConditionFalse, now, agentprofile.ConflictConditionReason, err.Error())
			logger.Error(err, "unable to reconcile profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			r.syncProfileStatus(ctx, &profile, originalStatus, now)
			continue
		}

		// The profile is valid and its shared overlay was accepted.
		// Commit node assignment and shared overlay changes.
		agentprofile.AssignNodesToProfile(profile.ObjectMeta, requirements, nodeList, profilesByNode, csInfo)
		accumulatedDefaultSpec = candidateDefaultSpec
		setProfileCondition(&profile, agentprofile.AppliedConditionType, metav1.ConditionTrue, now, agentprofile.AppliedConditionReason, "Profile applied")
		r.syncProfileStatus(ctx, &profile, originalStatus, now)
		appliedProfiles = append(appliedProfiles, &profile)
	}

	// Persist the accepted shared config back to the default DDAI used later to
	// render the default-profile DDAI object.
	defaultDDAI.Spec = *accumulatedDefaultSpec

	if err := r.enforceCreateStrategy(ctx, appliedProfiles, profilesByNode, csInfo, dsNSName, ddaEDSMaxUnavailable, len(nodeList)); err != nil {
		return appliedProfiles, err
	}

	if err := r.labelNodesWithProfiles(ctx, profilesByNode); err != nil {
		return appliedProfiles, fmt.Errorf("unable to label nodes with profiles: %w", err)
	}

	if err := r.cleanupPodsForProfilesThatNoLongerApply(ctx, profilesByNode, dsNSName.Namespace); err != nil {
		return appliedProfiles, fmt.Errorf("unable to cleanup pods for profiles that no longer apply: %w", err)
	}

	return appliedProfiles, nil
}

// enforceCreateStrategy updates create-strategy status for applied profiles and
// prunes profilesByNode so only nodes allowed by the strategy are labeled.
func (r *Reconciler) enforceCreateStrategy(ctx context.Context, appliedProfiles []*v1alpha1.DatadogAgentProfile, profilesByNode map[string]types.NamespacedName, csInfo map[types.NamespacedName]*agentprofile.CreateStrategyInfo, dsNSName types.NamespacedName, ddaEDSMaxUnavailable intstr.IntOrString, nodeCount int) error {
	if !agentprofile.CreateStrategyEnabled() {
		return nil
	}
	logger := ctrl.LoggerFrom(ctx)
	for _, profile := range appliedProfiles {
		if !agentprofile.CreateStrategyNeeded(profile, csInfo) {
			continue
		}

		// get profile ds to set create strategy status
		ds, err := r.getProfileDaemonSet(ctx, profile, dsNSName)
		if err != nil {
			return fmt.Errorf("unable to get profile daemon set: %w", err)
		}

		profileCopy := profile.DeepCopy()
		agentprofile.ApplyCreateStrategy(logger, profilesByNode, csInfo[types.NamespacedName{Namespace: profile.Namespace, Name: profile.Name}], profileCopy, ddaEDSMaxUnavailable, nodeCount, &ds.Status)
		if !agentprofile.IsEqualStatus(&profile.Status, &profileCopy.Status) {
			if err := r.client.Status().Update(ctx, profileCopy); err != nil {
				logger.Error(err, "unable to update profile status", "datadogagentprofile", profileCopy.Name, "datadogagentprofile_namespace", profileCopy.Namespace)
			}
		}
	}
	return nil
}

// syncProfileStatus generates status from conditions and persists it to k8s if it changed.
func (r *Reconciler) syncProfileStatus(ctx context.Context, profile *v1alpha1.DatadogAgentProfile, originalStatus *v1alpha1.DatadogAgentProfileStatus, now metav1.Time) {
	logger := ctrl.LoggerFrom(ctx)
	agentprofile.GenerateProfileStatusFromConditions(logger, profile, now)
	if !agentprofile.IsEqualStatus(originalStatus, &profile.Status) {
		if err := r.client.Status().Update(ctx, profile); err != nil {
			logger.Error(err, "unable to update profile status", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
		}
	}
}

func (r *Reconciler) getProfileDaemonSet(ctx context.Context, profile *v1alpha1.DatadogAgentProfile, dsName types.NamespacedName) (*appsv1.DaemonSet, error) {
	validDaemonSetNames, _ := r.getValidDaemonSetNames(dsName.Name, map[string]struct{}{}, []v1alpha1.DatadogAgentProfile{*profile}, true)
	if len(validDaemonSetNames) != 1 {
		return nil, fmt.Errorf("unexpected number of valid daemonset names: %d", len(validDaemonSetNames))
	}

	for name := range validDaemonSetNames {
		ds := &appsv1.DaemonSet{}
		if err := r.client.Get(ctx, types.NamespacedName{Namespace: dsName.Namespace, Name: name}, ds); err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		return ds, nil
	}
	return nil, fmt.Errorf("no valid daemonset found")
}

func (r *Reconciler) applyProfilesToDDAISpec(baseDDAI, defaultDDAI *v1alpha1.DatadogAgentInternal, profiles []*v1alpha1.DatadogAgentProfile) ([]*v1alpha1.DatadogAgentInternal, error) {
	ddais := []*v1alpha1.DatadogAgentInternal{}

	// For all profiles, create DDAI objects
	// Note: profiles includes the default profile to allow the default affinity to be set
	for _, profile := range profiles {
		// User profiles are merged from the original base DDAI. The synthetic
		// default profile is merged from defaultDDAI so it includes shared config
		// accepted from profiles, for example APM SSI Cluster Agent config.
		sourceDDAI := baseDDAI
		if agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
			sourceDDAI = defaultDDAI
		}
		mergedDDAI, err := r.computeProfileMerge(sourceDDAI, profile)
		if err != nil {
			return nil, err
		}
		ddais = append(ddais, mergedDDAI)
	}

	return ddais, nil
}

func (r *Reconciler) computeProfileMerge(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) (*v1alpha1.DatadogAgentInternal, error) {
	// Copy the original DDAI and apply profile spec to create a fake "DDAI" to merge
	profileDDAI := ddai.DeepCopy()
	baseDDAI := ddai.DeepCopy()

	// Add profile settings to "fake" DDAI
	setProfileSpec(profileDDAI, profile)
	if err := setProfileDDAIMeta(profileDDAI, profile); err != nil {
		return nil, err
	}

	// ensure gvk is set
	baseDDAI.GetObjectKind().SetGroupVersionKind(getDDAIGVK())
	profileDDAI.GetObjectKind().SetGroupVersionKind(getDDAIGVK())
	// Server side apply to merge DDAIs
	obj, err := r.ssaMergeCRD(baseDDAI, profileDDAI)
	if err != nil {
		return nil, err
	}

	// Convert from runtime.Object back to DDAI
	typedObj, ok := obj.(*v1alpha1.DatadogAgentInternal)
	if !ok {
		return nil, fmt.Errorf("unexpected type: %T", obj)
	}
	// Profile merging replaces the DDAI spec and can reintroduce a registry that
	// is rejected by the GKE Autopilot workload allowlist. Enforce the registry
	// constraint on the final merged object before computing its spec hash.
	if experimental.IsAutopilotEnabled(typedObj) {
		ensureGCRAutopilotRegistry(&typedObj.Spec)
	}

	// Set spec hash
	if _, err := comparison.SetMD5GenerationAnnotation(&typedObj.ObjectMeta, typedObj.Spec, constants.MD5DDAIDeploymentAnnotationKey); err != nil {
		return nil, err
	}
	return typedObj, nil
}

func setProfileSpec(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) {
	// create affinity from ddai and profile prior to re-set after replacing the ddai spec
	affinity := setProfileDDAIAffinity(ddai, profile)
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		// Capture spec.global.commonLabels from the base DDAI before replacing
		// the spec with the profile config. The profile's Config is a user-defined
		// DatadogAgentSpec that doesn't include the parent DDA's global settings,
		// so commonLabels would be silently dropped. We restore them afterward so
		// that label-enforcing admission policies (e.g. Kyverno) do not reject
		// the profile DaemonSet even when the parent DDA sets spec.global.commonLabels.
		var commonLabels map[string]string
		if ddai.Spec.Global != nil && len(ddai.Spec.Global.CommonLabels) > 0 {
			commonLabels = make(map[string]string, len(ddai.Spec.Global.CommonLabels))
			maps.Copy(commonLabels, ddai.Spec.Global.CommonLabels)
		}

		ddai.Spec = *profile.Spec.Config

		// Restore commonLabels into the replaced spec.
		if len(commonLabels) > 0 {
			if ddai.Spec.Global == nil {
				ddai.Spec.Global = &v2alpha1.GlobalConfig{}
			}
			// Profile config wins on any key conflict — only fill in keys
			// not already set by the profile itself.
			if ddai.Spec.Global.CommonLabels == nil {
				ddai.Spec.Global.CommonLabels = make(map[string]string, len(commonLabels))
			}
			for k, v := range commonLabels {
				if _, exists := ddai.Spec.Global.CommonLabels[k]; !exists {
					ddai.Spec.Global.CommonLabels[k] = v
				}
			}
		}

		// DCA, CCR, and OtelAgentGateway are auto disabled for user created profiles
		disableComponent(ddai, v2alpha1.ClusterAgentComponentName)
		disableComponent(ddai, v2alpha1.ClusterChecksRunnerComponentName)
		disableComponent(ddai, v2alpha1.OtelAgentGatewayComponentName)
		setProfileNodeAgentOverride(ddai, profile)
	}
	ensureOverrideExists(ddai, v2alpha1.NodeAgentComponentName)
	ddai.Spec.Override[v2alpha1.NodeAgentComponentName].Affinity = affinity
}

func ensureOverrideExists(ddai *v1alpha1.DatadogAgentInternal, componentName v2alpha1.ComponentName) {
	if ddai.Spec.Override == nil {
		ddai.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)
	}
	if ddai.Spec.Override[componentName] == nil {
		ddai.Spec.Override[componentName] = &v2alpha1.DatadogAgentComponentOverride{}
	}
}

func disableComponent(ddai *v1alpha1.DatadogAgentInternal, componentName v2alpha1.ComponentName) {
	ensureOverrideExists(ddai, componentName)
	ddai.Spec.Override[componentName].Disabled = new(true)
}

func setProfileDDAIAffinity(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) *corev1.Affinity {
	override, ok := ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
	if !ok || override == nil {
		override = &v2alpha1.DatadogAgentComponentOverride{}
	}
	return common.MergeAffinities(override.Affinity, agentprofile.AffinityOverride(profile))
}

func setProfileDDAIMeta(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) error {
	// Name
	ddai.Name = getProfileDDAIName(ddai.Name, profile.Name, profile.Namespace)
	// Managed fields needs to be nil for server-side apply
	ddai.ManagedFields = nil
	// Include the profile label in the DDAI metadata only for non-default profiles.
	// This is used to determine whether or not the EDS should be created (only for default profile).
	// This could possibly be used for GC of the profile DDAIs too.
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		// This should not happen, as we add the DDA label upon creation of the DDAI.
		if ddai.Labels == nil {
			ddai.Labels = make(map[string]string)
		}
		ddai.Labels[constants.ProfileLabelKey] = profile.Name
	}
	// Propagate the provider annotation from the profile onto the DDAI so a
	// DAP can declare a provider that differs from the DDA (e.g. a GKE COS
	// node pool selected by the profile). The profile value overrides the
	// DDA-inherited value when set.
	if v, ok := profile.GetAnnotations()[kubernetes.ProviderAnnotationKey]; ok {
		if ddai.Annotations == nil {
			ddai.Annotations = make(map[string]string)
		}
		ddai.Annotations[kubernetes.ProviderAnnotationKey] = v
	}
	return nil
}

// getProfileDDAIName returns the name of the DDAI when profiles are used.
// Default profiles use the DDA name. User created profiles use the profile name.
func getProfileDDAIName(ddaiName, profileName, profileNamespace string) string {
	if agentprofile.IsDefaultProfile(profileNamespace, profileName) {
		return ddaiName
	}
	return profileName
}

// The node agent component override is non-nil from the default DDAI creation
func setProfileNodeAgentOverride(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) {
	ensureOverrideExists(ddai, v2alpha1.NodeAgentComponentName)
	setProfileDDAILabels(ddai.Spec.Override[v2alpha1.NodeAgentComponentName], profile)

	// Set the DaemonSet name override for profile DDAIs to prevent conflicts
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		dsName := agentprofile.DaemonSetName(types.NamespacedName{
			Name:      profile.Name,
			Namespace: profile.Namespace,
		}, true) // Use v3 metadata naming

		if dsName != "" {
			ddai.Spec.Override[v2alpha1.NodeAgentComponentName].Name = &dsName
		}
	}
}

func setProfileDDAILabels(override *v2alpha1.DatadogAgentComponentOverride, profile *v1alpha1.DatadogAgentProfile) {
	if override.Labels == nil {
		override.Labels = make(map[string]string)
	}
	override.Labels[constants.ProfileLabelKey] = profile.Name
}
