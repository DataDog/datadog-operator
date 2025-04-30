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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	ddaiCRDName             = "datadogagentinternals.datadoghq.com"
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
	for _, profile := range profiles {
		ddai, err = r.computeProfileMerge(ddai, &profile)
		if err != nil {
			return nil, err
		}
		ddais = append(ddais, ddai)
	}

	return ddais, nil
}

func (r *Reconciler) computeProfileMerge(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) (*v1alpha1.DatadogAgentInternal, error) {
	// Copy the original DDAI and apply profile spec to create a fake "DDAI" to merge
	profileDDAI := ddai.DeepCopy()
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		// Clear owner reference since we want to tie GC to the profile
		ddai.OwnerReferences = []metav1.OwnerReference{}
		profileDDAI.Spec = *profile.Spec.Config
	}

	// Add profile settings to "fake" DDAI
	setProfileDDAIAffinity(profileDDAI, profile)
	setProfileDDAIMeta(profileDDAI, profile, r.scheme)

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: ddaiCRDName}, crd); err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %w", ddaiCRDName, err)
	}

	// Server side apply to merge DDAIs
	obj, err := ssaMergeCRD(ddai, profileDDAI, crd, r.scheme)
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

func setProfileDDAIAffinity(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) {
	override, ok := ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
	if !ok {
		override = &v2alpha1.DatadogAgentComponentOverride{}
	}
	override.Affinity = common.MergeAffinities(override.Affinity, agentprofile.AffinityOverride(profile))
}

func setProfileDDAIMeta(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile, scheme *runtime.Scheme) error {
	// Name
	ddai.Name = getProfileDDAIName(ddai.Name, profile.Name, profile.Namespace)
	// Managed fields
	ddai.ManagedFields = []metav1.ManagedFieldsEntry{}
	// Owner reference
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		ownerRef, err := object.CreateOwnerRef(profile, scheme)
		if err != nil {
			return err
		}
		ddai.SetOwnerReferences([]metav1.OwnerReference{*ownerRef})
	}
	return nil
}

func getProfileDDAIName(ddaiName, profileName, profileNamespace string) string {
	if agentprofile.IsDefaultProfile(profileNamespace, profileName) {
		return ddaiName
	}
	return fmt.Sprintf(profileDDAINameTemplate, ddaiName, profileName)
}
