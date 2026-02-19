// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

func sendProfileEnabledMetric(enabled bool) {
	if enabled {
		metrics.DAPEnabled.Set(metrics.TrueValue)
	} else {
		metrics.DAPEnabled.Set(metrics.FalseValue)
	}
}

// reconcileProfiles reconciles all profiles
// - returns a list of profiles that should be applied (including the default profile)
// - configures node labels based on the profiles that are applied
// - applies profile status updates in k8s
func (r *Reconciler) reconcileProfiles(ctx context.Context, dsNSName types.NamespacedName, ddaEDSMaxUnavailable intstr.IntOrString) ([]*v1alpha1.DatadogAgentProfile, error) {
	now := metav1.Now()
	// start with the default profile so that on error, at minimum the default profile is applied
	defaultProfile := agentprofile.DefaultProfile()
	appliedProfiles := []*v1alpha1.DatadogAgentProfile{&defaultProfile}
	// get and sort all profiles
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
		profileCopy := profile.DeepCopy() // deep copy to avoid modifying status of original profile
		if err := r.reconcileProfile(ctx, profileCopy, nodeList, profilesByNode, csInfo, now); err != nil {
			// errors will be validation or conflict errors
			r.log.Error(err, "unable to reconcile profile", "datadogagentprofile", profileCopy.Name, "datadogagentprofile_namespace", profileCopy.Namespace)
		}
		agentprofile.GenerateProfileStatusFromConditions(r.log, profileCopy, now)
		if !agentprofile.IsEqualStatus(&profile.Status, &profileCopy.Status) {
			if err := r.client.Status().Update(ctx, profileCopy); err != nil {
				r.log.Error(err, "unable to update profile status", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			}
		}
		// add profile to list of applied profiles only if it was applied successfully
		if profileCopy.Status.Applied == metav1.ConditionTrue {
			appliedProfiles = append(appliedProfiles, profileCopy)
		}
	}

	// create strategy
	if agentprofile.CreateStrategyEnabled() {
		for _, profile := range appliedProfiles {
			if !agentprofile.CreateStrategyNeeded(profile, csInfo) {
				continue
			}

			// get ds to set create strategy status
			ds, err := r.getProfileDaemonSet(ctx, profile, dsNSName)
			if err != nil {
				return appliedProfiles, fmt.Errorf("unable to get profile daemon set: %w", err)
			}

			profileCopy := profile.DeepCopy()
			agentprofile.ApplyCreateStrategy(r.log, profilesByNode, csInfo[types.NamespacedName{Namespace: profile.Namespace, Name: profile.Name}], profileCopy, ddaEDSMaxUnavailable, len(nodeList), &ds.Status)
			if !agentprofile.IsEqualStatus(&profile.Status, &profileCopy.Status) {
				if err := r.client.Status().Update(ctx, profileCopy); err != nil {
					r.log.Error(err, "unable to update profile status", "datadogagentprofile", profileCopy.Name, "datadogagentprofile_namespace", profileCopy.Namespace)
				}
			}
		}
	}

	// label nodes
	if err := r.labelNodesWithProfiles(ctx, profilesByNode); err != nil {
		return appliedProfiles, fmt.Errorf("unable to label nodes with profiles: %w", err)
	}
	return appliedProfiles, nil
}

// reconcileProfile reconciles a single profile
// - validates the profile
// - checks for conflicts with existing profiles
// - updates the profile status based on profile validation and application success
func (r *Reconciler) reconcileProfile(ctx context.Context, profile *v1alpha1.DatadogAgentProfile, nodeList []corev1.Node, profilesByNode map[string]types.NamespacedName, csInfo map[types.NamespacedName]*agentprofile.CreateStrategyInfo, now metav1.Time) error {
	r.log.Info("reconciling profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
	// validate profile name, spec, and selectors
	requirements, err := agentprofile.ValidateProfileAndReturnRequirements(profile, r.options.DatadogAgentInternalEnabled)
	if err != nil {
		metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.FalseValue)
		profile.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(profile.Status.Conditions, agentprofile.NewDatadogAgentProfileCondition(agentprofile.ValidConditionType, metav1.ConditionFalse, now, agentprofile.InvalidConditionReason, err.Error()))
		profile.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(profile.Status.Conditions, agentprofile.NewDatadogAgentProfileCondition(agentprofile.AppliedConditionType, metav1.ConditionUnknown, now, "", ""))
		return err
	}
	metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.TrueValue)
	profile.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(profile.Status.Conditions, agentprofile.NewDatadogAgentProfileCondition(agentprofile.ValidConditionType, metav1.ConditionTrue, now, agentprofile.ValidConditionReason, "Valid manifest"))

	// err can only be conflict
	if err := agentprofile.ApplyProfileToNodes(profile.ObjectMeta, requirements, nodeList, profilesByNode, csInfo); err != nil {
		profile.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(profile.Status.Conditions, agentprofile.NewDatadogAgentProfileCondition(agentprofile.AppliedConditionType, metav1.ConditionFalse, now, agentprofile.ConflictConditionReason, "Conflict with existing profile"))
		return err
	}
	profile.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(profile.Status.Conditions, agentprofile.NewDatadogAgentProfileCondition(agentprofile.AppliedConditionType, metav1.ConditionTrue, now, agentprofile.AppliedConditionReason, "Profile applied"))

	return nil
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

func (r *Reconciler) applyProfilesToDDAISpec(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, profiles []*v1alpha1.DatadogAgentProfile) ([]*v1alpha1.DatadogAgentInternal, error) {
	ddais := []*v1alpha1.DatadogAgentInternal{}

	// Profiles includes the default profile so that default node-agent affinity is set.
	for _, profile := range profiles {
		mergedSpec, err := r.mergeProfileSpec(ctx, ddai, profile)
		if err != nil {
			return nil, err
		}
		mergedDDAI, err := buildProfileDDAI(ddai, mergedSpec, profile)
		if err != nil {
			return nil, err
		}
		ddais = append(ddais, mergedDDAI)
	}

	return ddais, nil
}

func (r *Reconciler) mergeProfileSpec(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) (*v2alpha1.DatadogAgentSpec, error) {
	// Default profile: no API call needed
	if agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		spec := ddai.Spec.DeepCopy()
		affinity := setProfileDDAIAffinity(spec, profile)
		ensureOverrideExists(spec, v2alpha1.NodeAgentComponentName)
		spec.Override[v2alpha1.NodeAgentComponentName].Affinity = affinity
		return spec, nil
	}

	// User profile: SSA dry-run against the live default DDAI
	patch := &v1alpha1.DatadogAgentInternal{
		// TypeMeta required for SSA patch body
		TypeMeta: metav1.TypeMeta{
			APIVersion: "datadoghq.com/v1alpha1",
			Kind:       "DatadogAgentInternal",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ddai.Name,
			Namespace: ddai.Namespace,
		},
	}

	setProfileSpec(&patch.Spec, &ddai.Spec, profile)

	if err := r.ssaDryRunPatch(ctx, patch); err != nil {
		return nil, fmt.Errorf("failed to merge profile spec: %w", err)
	}
	return &patch.Spec, nil
}

func buildProfileDDAI(base *v1alpha1.DatadogAgentInternal, spec *v2alpha1.DatadogAgentSpec, profile *v1alpha1.DatadogAgentProfile) (*v1alpha1.DatadogAgentInternal, error) {
	ddai := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: *base.ObjectMeta.DeepCopy(),
		Spec:       *spec,
	}
	setProfileDDAIMeta(ddai, profile)
	if _, err := comparison.SetMD5GenerationAnnotation(&ddai.ObjectMeta, ddai.Spec, constants.MD5DDAIDeploymentAnnotationKey); err != nil {
		return nil, err
	}
	return ddai, nil
}

// ssaDryRunPatch performs a server-side apply dry-run patch on obj.
// The live object is unchanged by the patch
func (r *Reconciler) ssaDryRunPatch(ctx context.Context, obj client.Object) error {
	return r.client.Patch(ctx, obj, client.Apply,
		client.DryRunAll,
		client.ForceOwnership,
		client.FieldOwner("datadog-operator"))
}

func setProfileSpec(spec *v2alpha1.DatadogAgentSpec, ddaSpec *v2alpha1.DatadogAgentSpec, profile *v1alpha1.DatadogAgentProfile) {
	if profile.Spec.Config != nil {
		*spec = *profile.Spec.Config.DeepCopy()
	}
	// DCA, CCR, and OtelAgentGateway are auto-disabled for user-created profiles.
	disableComponent(spec, v2alpha1.ClusterAgentComponentName)
	disableComponent(spec, v2alpha1.ClusterChecksRunnerComponentName)
	disableComponent(spec, v2alpha1.OtelAgentGatewayComponentName)
	setProfileNodeAgentOverride(spec, profile)
	// setProfileNodeAgentOverride guarantees node agent override is non-nil
	// merge affinity from live DDA and profile
	spec.Override[v2alpha1.NodeAgentComponentName].Affinity = setProfileDDAIAffinity(ddaSpec, profile)
}

func ensureOverrideExists(spec *v2alpha1.DatadogAgentSpec, componentName v2alpha1.ComponentName) {
	if spec.Override == nil {
		spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)
	}
	if spec.Override[componentName] == nil {
		spec.Override[componentName] = &v2alpha1.DatadogAgentComponentOverride{}
	}
}

func disableComponent(spec *v2alpha1.DatadogAgentSpec, componentName v2alpha1.ComponentName) {
	ensureOverrideExists(spec, componentName)
	spec.Override[componentName].Disabled = apiutils.NewBoolPointer(true)
}

func setProfileDDAIAffinity(spec *v2alpha1.DatadogAgentSpec, profile *v1alpha1.DatadogAgentProfile) *corev1.Affinity {
	override, ok := spec.Override[v2alpha1.NodeAgentComponentName]
	if !ok || override == nil {
		override = &v2alpha1.DatadogAgentComponentOverride{}
	}
	return common.MergeAffinities(override.Affinity, agentprofile.AffinityOverride(profile))
}

func setProfileDDAIMeta(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) {
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
func setProfileNodeAgentOverride(spec *v2alpha1.DatadogAgentSpec, profile *v1alpha1.DatadogAgentProfile) {
	ensureOverrideExists(spec, v2alpha1.NodeAgentComponentName)
	setProfileDDAILabels(spec.Override[v2alpha1.NodeAgentComponentName], profile)

	// Set the DaemonSet name override for profile DDAIs to prevent conflicts
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		dsName := agentprofile.DaemonSetName(types.NamespacedName{
			Name:      profile.Name,
			Namespace: profile.Namespace,
		}, true) // Use v3 metadata naming

		if dsName != "" {
			spec.Override[v2alpha1.NodeAgentComponentName].Name = &dsName
		}
	}
}

func setProfileDDAILabels(override *v2alpha1.DatadogAgentComponentOverride, profile *v1alpha1.DatadogAgentProfile) {
	if override.Labels == nil {
		override.Labels = make(map[string]string)
	}
	override.Labels[constants.ProfileLabelKey] = profile.Name
}
