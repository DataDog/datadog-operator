// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	profileDDAINameTemplate = "%s-profile-%s"
)

func sendProfileEnabledMetric(enabled bool) {
	if enabled {
		metrics.DAPEnabled.Set(metrics.TrueValue)
	} else {
		metrics.DAPEnabled.Set(metrics.FalseValue)
	}
}

func (r *Reconciler) applyProfilesToDDAISpec(ctx context.Context, logger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, now metav1.Time) ([]*v1alpha1.DatadogAgentInternal, error) {
	ddais := []*v1alpha1.DatadogAgentInternal{}
	var err error

	var nodeList []corev1.Node
	nodeList, err = r.getNodeList(ctx)
	if err != nil {
		return nil, err
	}

	var profiles []v1alpha1.DatadogAgentProfile
	var profilesByNode map[string]types.NamespacedName
	profiles, profilesByNode, err = r.profilesToApply(ctx, logger, nodeList, now, &ddai.Spec)
	if err != nil {
		return nil, err
	}

	if err = r.handleProfiles(ctx, profilesByNode, ddai.Namespace); err != nil {
		return nil, err
	}

	// For all profiles, create DDAI objects
	// Note: profiles includes the default profile to allow the default affinity to be set
	for _, profile := range profiles {
		mergedDDAI, err := r.computeProfileMerge(ddai, &profile)
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
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		ddai.Spec = *profile.Spec.Config
		// DCA and CCR are auto disabled for user created profiles
		disableComponent(ddai, v2alpha1.ClusterAgentComponentName)
		disableComponent(ddai, v2alpha1.ClusterChecksRunnerComponentName)
		setProfileNodeAgentOverride(ddai, profile)
	}
	setProfileDDAIAffinity(ddai, profile)
}

func disableComponent(ddai *v1alpha1.DatadogAgentInternal, componentName v2alpha1.ComponentName) {
	if _, ok := ddai.Spec.Override[componentName]; !ok {
		ddai.Spec.Override[componentName] = &v2alpha1.DatadogAgentComponentOverride{}
	}
	ddai.Spec.Override[componentName].Disabled = apiutils.NewBoolPointer(true)
}

func setProfileDDAIAffinity(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) {
	override, ok := ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
	if !ok {
		override = &v2alpha1.DatadogAgentComponentOverride{}
	}
	override.Affinity = common.MergeAffinities(override.Affinity, agentprofile.AffinityOverride(profile))
}

func setProfileDDAIMeta(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) error {
	// Name
	ddai.Name = getProfileDDAIName(ddai.Name, profile.Name, profile.Namespace)
	// Managed fields
	ddai.ManagedFields = []metav1.ManagedFieldsEntry{}
	// Include the profile label in the DDAI metadata only for non-default profiles.
	// This is used to determine whether or not the EDS should be created (only for default profile).
	// This could possibly be used for GC of the profile DDAIs too.
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		// This should not happen, as we add the DDA label upon creation of the DDAI.
		if ddai.Labels == nil {
			ddai.Labels = make(map[string]string)
		}
		ddai.Labels[agentprofile.ProfileLabelKey] = profile.Name
	}
	return nil
}

func getProfileDDAIName(ddaiName, profileName, profileNamespace string) string {
	if agentprofile.IsDefaultProfile(profileNamespace, profileName) {
		return ddaiName
	}
	return fmt.Sprintf(profileDDAINameTemplate, ddaiName, profileName)
}

// The node agent component override is non-nil from the default DDAI creation
func setProfileNodeAgentOverride(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) {
	setProfileDSName(ddai.Spec.Override[v2alpha1.NodeAgentComponentName], profile)
	setProfileDDAILabels(ddai.Spec.Override[v2alpha1.NodeAgentComponentName], profile)
}

func setProfileDSName(override *v2alpha1.DatadogAgentComponentOverride, profile *v1alpha1.DatadogAgentProfile) {
	override.Name = apiutils.NewStringPointer(agentprofile.DaemonSetName(types.NamespacedName{Namespace: profile.Namespace, Name: profile.Name}))
}

func setProfileDDAILabels(override *v2alpha1.DatadogAgentComponentOverride, profile *v1alpha1.DatadogAgentProfile) {
	if override.Labels == nil {
		override.Labels = make(map[string]string)
	}
	override.Labels[agentprofile.ProfileLabelKey] = profile.Name
}
