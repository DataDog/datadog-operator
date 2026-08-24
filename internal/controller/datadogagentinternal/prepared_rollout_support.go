// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"fmt"
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	defaultPreparedRolloutMaxUnavailable = 1
)

func configureConventionalMigration(ds *appsv1.DaemonSet, budget intstr.IntOrString) {
	zero := intstr.FromInt(0)
	value := budget
	ds.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDaemonSet{
			MaxUnavailable: &value,
			MaxSurge:       &zero,
		},
	}
}

// validatePreparedMigrationSource keeps prepared-mode enablement separate from
// an Agent image update. The one-time arming rollout is delete-first, so it
// must not also introduce an unproven Agent image.
func validatePreparedMigrationSource(current, desired corev1.PodTemplateSpec, fullyRolledOut bool) error {
	if !maps.Equal(preparedContainerImages(current), preparedContainerImages(desired)) {
		return fmt.Errorf("prepared blue/green rollout requires the desired Agent images to be deployed conventionally first; current and desired container images differ")
	}
	if !fullyRolledOut {
		return fmt.Errorf("prepared blue/green rollout requires the current Agent image rollout to finish before enabling prepared mode")
	}
	return nil
}

func preparedContainerImages(template corev1.PodTemplateSpec) map[string]string {
	images := make(map[string]string, len(template.Spec.InitContainers)+len(template.Spec.Containers))
	for i := range template.Spec.InitContainers {
		container := &template.Spec.InitContainers[i]
		images["init/"+container.Name] = container.Image
	}
	for i := range template.Spec.Containers {
		container := &template.Spec.Containers[i]
		images["container/"+container.Name] = container.Image
	}
	return images
}

// prepareProfileAntiAffinityForOverlap narrows the standard DAP anti-affinity so
// old and new revisions of the same profile may overlap. Unknown user-supplied
// anti-affinity fails closed.
func prepareProfileAntiAffinityForOverlap(template *corev1.PodTemplateSpec) bool {
	if template.Spec.Affinity == nil || template.Spec.Affinity.PodAntiAffinity == nil {
		return true
	}
	if !apiequality.Semantic.DeepEqual(template.Spec.Affinity.PodAntiAffinity, broadAgentPodAntiAffinity()) {
		return false
	}
	narrowed, ok := profileOverlapPodAntiAffinity(template.Labels)
	if !ok {
		return false
	}
	template.Spec.Affinity.PodAntiAffinity = narrowed
	return true
}

func broadAgentPodAntiAffinity() *corev1.PodAntiAffinity {
	return &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
		LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key: datadoghqcommon.AgentDeploymentComponentLabelKey, Operator: metav1.LabelSelectorOpIn, Values: []string{constants.DefaultAgentResourceSuffix},
		}}},
		TopologyKey: corev1.LabelHostname,
	}}}
}

func profileOverlapPodAntiAffinity(podLabels map[string]string) (*corev1.PodAntiAffinity, bool) {
	ddaName := podLabels[datadoghqcommon.AgentDeploymentNameLabelKey]
	if ddaName == "" {
		return nil, false
	}

	profileRequirement := metav1.LabelSelectorRequirement{Key: constants.ProfileLabelKey}
	if profileName := podLabels[constants.ProfileLabelKey]; profileName != "" {
		profileRequirement.Operator = metav1.LabelSelectorOpNotIn
		profileRequirement.Values = []string{profileName}
	} else {
		profileRequirement.Operator = metav1.LabelSelectorOpExists
	}
	componentRequirement := metav1.LabelSelectorRequirement{
		Key: datadoghqcommon.AgentDeploymentComponentLabelKey, Operator: metav1.LabelSelectorOpIn, Values: []string{constants.DefaultAgentResourceSuffix},
	}

	return &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
		{
			LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				componentRequirement,
				{Key: datadoghqcommon.AgentDeploymentNameLabelKey, Operator: metav1.LabelSelectorOpIn, Values: []string{ddaName}},
				profileRequirement,
			}},
			TopologyKey: corev1.LabelHostname,
		},
		{
			LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				componentRequirement,
				{Key: datadoghqcommon.AgentDeploymentNameLabelKey, Operator: metav1.LabelSelectorOpNotIn, Values: []string{ddaName}},
			}},
			TopologyKey: corev1.LabelHostname,
		},
	}}, true
}

func positiveIntOrPercent(value *intstr.IntOrString) bool {
	if value == nil {
		return false
	}
	scaled, err := intstr.GetScaledValueFromIntOrPercent(value, 100, true)
	return err == nil && scaled > 0
}

func preparedRolloutBudget(ddai *datadoghqv1alpha1.DatadogAgentInternal, options *componentagent.ExtendedDaemonsetOptions) intstr.IntOrString {
	if override, ok := ddai.Spec.Override[datadoghqv2alpha1.NodeAgentComponentName]; ok && override != nil && override.UpdateStrategy != nil && override.UpdateStrategy.RollingUpdate != nil && override.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
		return *override.UpdateStrategy.RollingUpdate.MaxUnavailable
	}
	if options != nil && options.MaxPodUnavailable != "" {
		return intstr.Parse(options.MaxPodUnavailable)
	}
	return intstr.FromInt(defaultPreparedRolloutMaxUnavailable)
}
