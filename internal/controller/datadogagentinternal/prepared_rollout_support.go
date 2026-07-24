// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	defaultPreparedRolloutMaxUnavailable = 1
)

func configurePreparedSurge(ds *appsv1.DaemonSet, budget intstr.IntOrString) bool {
	strategy := &ds.Spec.UpdateStrategy
	if strategy.Type != "" && strategy.Type != appsv1.RollingUpdateDaemonSetStrategyType {
		return false
	}
	if strategy.RollingUpdate == nil {
		strategy.RollingUpdate = &appsv1.RollingUpdateDaemonSet{}
	}
	if _, err := intstr.GetScaledValueFromIntOrPercent(&budget, 100, true); err != nil {
		return false
	}

	strategy.Type = appsv1.RollingUpdateDaemonSetStrategyType
	zero := intstr.FromInt(0)
	strategy.RollingUpdate.MaxUnavailable = &zero
	surge := budget
	strategy.RollingUpdate.MaxSurge = &surge
	return positiveIntOrPercent(&surge)
}

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

// prepareProfileAntiAffinityForSurge narrows the standard DAP anti-affinity so
// old and new revisions of the same profile may overlap. Unknown user-supplied
// anti-affinity fails closed.
func prepareProfileAntiAffinityForSurge(template *corev1.PodTemplateSpec) bool {
	if template.Spec.Affinity == nil || template.Spec.Affinity.PodAntiAffinity == nil {
		return true
	}
	if !apiequality.Semantic.DeepEqual(template.Spec.Affinity.PodAntiAffinity, broadAgentPodAntiAffinity()) {
		return false
	}
	narrowed, ok := profileSurgePodAntiAffinity(template.Labels)
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

func profileSurgePodAntiAffinity(podLabels map[string]string) (*corev1.PodAntiAffinity, bool) {
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

func daemonSetControlledByDDAI(ds *appsv1.DaemonSet, ddai *datadoghqv1alpha1.DatadogAgentInternal) bool {
	owner := metav1.GetControllerOf(ds)
	return owner != nil && owner.APIVersion == datadoghqv1alpha1.GroupVersion.String() && owner.Kind == "DatadogAgentInternal" && owner.UID == ddai.UID
}

func preparedRolloutDaemonSetEligible(ds *appsv1.DaemonSet) bool {
	if ds.DeletionTimestamp != nil || ds.Status.DesiredNumberScheduled <= 0 || ds.Status.ObservedGeneration != ds.Generation {
		return false
	}
	return ds.Spec.UpdateStrategy.Type == appsv1.RollingUpdateDaemonSetStrategyType &&
		ds.Spec.UpdateStrategy.RollingUpdate != nil && positiveIntOrPercent(ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
}

func currentDaemonSetRevision(ctx context.Context, reader client.Reader, ds *appsv1.DaemonSet) (string, error) {
	revisions := &appsv1.ControllerRevisionList{}
	if err := reader.List(ctx, revisions, client.InNamespace(ds.Namespace)); err != nil {
		return "", fmt.Errorf("list revisions for Agent DaemonSet %s/%s: %w", ds.Namespace, ds.Name, err)
	}
	var current *appsv1.ControllerRevision
	for i := range revisions.Items {
		revision := &revisions.Items[i]
		if !controlledByUID(revision, ds.UID) {
			continue
		}
		matches, err := controllerRevisionMatchesTemplate(revision, &ds.Spec.Template)
		if err != nil {
			return "", fmt.Errorf("decode revision %s for Agent DaemonSet %s/%s: %w", revision.Name, ds.Namespace, ds.Name, err)
		}
		if matches && (current == nil || revision.Revision > current.Revision) {
			current = revision
		}
	}
	if current == nil {
		return "", nil
	}
	return current.Labels[appsv1.DefaultDaemonSetUniqueLabelKey], nil
}

func controllerRevisionMatchesTemplate(revision *appsv1.ControllerRevision, template *corev1.PodTemplateSpec) (bool, error) {
	var patch struct {
		Spec struct {
			Template corev1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(revision.Data.Raw, &patch); err != nil {
		return false, err
	}
	return apiequality.Semantic.DeepEqual(patch.Spec.Template, *template), nil
}

func daemonSetPods(ctx context.Context, reader client.Reader, ds *appsv1.DaemonSet) ([]corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("build selector for Agent DaemonSet %s/%s: %w", ds.Namespace, ds.Name, err)
	}
	list := &corev1.PodList{}
	if err := reader.List(ctx, list, client.InNamespace(ds.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, fmt.Errorf("list Pods for Agent DaemonSet %s/%s: %w", ds.Namespace, ds.Name, err)
	}
	result := make([]corev1.Pod, 0, len(list.Items))
	for i := range list.Items {
		if controlledByUID(&list.Items[i], ds.UID) {
			result = append(result, list.Items[i])
		}
	}
	return result, nil
}

func controlledByUID(obj metav1.Object, uid types.UID) bool {
	owner := metav1.GetControllerOf(obj)
	return owner != nil && owner.UID == uid
}

func podAvailable(pod *corev1.Pod, minReadySeconds int32, now time.Time) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for i := range pod.Status.Conditions {
		condition := &pod.Status.Conditions[i]
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return minReadySeconds == 0 || !condition.LastTransitionTime.IsZero() && condition.LastTransitionTime.Add(time.Duration(minReadySeconds)*time.Second).Before(now)
		}
	}
	return false
}
