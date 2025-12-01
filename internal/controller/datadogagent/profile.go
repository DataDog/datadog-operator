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

	r.log.Info("waffles profiles by node", "profilesByNode", profilesByNode)

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
		return nil, fmt.Errorf("unexpected number of valid daemon set names: %d", len(validDaemonSetNames))
	}

	for name := range validDaemonSetNames {
		ds := &appsv1.DaemonSet{}
		if err := r.client.Get(ctx, types.NamespacedName{Namespace: dsName.Namespace, Name: name}, ds); err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		return ds, nil
	}
	return nil, fmt.Errorf("no valid daemon set found")
}

func (r *Reconciler) applyProfilesToDDAISpec(ddai *v1alpha1.DatadogAgentInternal, profiles []*v1alpha1.DatadogAgentProfile) ([]*v1alpha1.DatadogAgentInternal, error) {
	ddais := []*v1alpha1.DatadogAgentInternal{}

	// For all profiles, create DDAI objects
	// Note: profiles includes the default profile to allow the default affinity to be set
	for _, profile := range profiles {
		mergedDDAI, err := r.computeProfileMerge(ddai, profile)
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
		ddai.Spec = *profile.Spec.Config
		// DCA and CCR are auto disabled for user created profiles
		disableComponent(ddai, v2alpha1.ClusterAgentComponentName)
		disableComponent(ddai, v2alpha1.ClusterChecksRunnerComponentName)
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
	ddai.Spec.Override[componentName].Disabled = apiutils.NewBoolPointer(true)
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
