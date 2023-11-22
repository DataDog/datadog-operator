// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// NewDaemonset use to generate the skeleton of a new daemonset based on few information
func NewDaemonset(owner metav1.Object, componentKind, componentName, version string, selector *metav1.LabelSelector) *appsv1.DaemonSet {
	labels, annotations, selector := component.GetDefaultMetadata(owner, componentKind, componentName, version, selector)

	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentName,
			Namespace:   owner.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: selector,
		},
	}
	return daemonset
}

// NewExtendedDaemonset use to generate the skeleton of a new extended daemonset based on few information
func NewExtendedDaemonset(owner metav1.Object, edsOptions *ExtendedDaemonsetOptions, componentKind, componentName, version string, selector *metav1.LabelSelector) *edsv1alpha1.ExtendedDaemonSet {
	// FIXME (@CharlyF): The EDS controller uses the Spec.Selector as a node selector to get the NodeList to rollout the agent.
	// Per https://github.com/DataDog/extendeddaemonset/blob/28a8e082cee9890ae6d925a7d6247a36c6f6ba5d/controllers/extendeddaemonsetreplicaset/controller.go#L344-L360
	// Up until v0.8.2, the Datadog Operator set the selector to nil, which circumvented this case.
	// Until the EDS controller uses the Affinity field to get the NodeList instead of Spec.Selector, let's keep the previous behavior.
	labels, annotations, _ := component.GetDefaultMetadata(owner, componentKind, componentName, version, selector)

	daemonset := &edsv1alpha1.ExtendedDaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentName,
			Namespace:   owner.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: defaultEDSSpec(edsOptions),
	}

	return daemonset
}

// ExtendedDaemonsetOptions defines ExtendedDaemonset options
type ExtendedDaemonsetOptions struct {
	Enabled bool

	MaxPodUnavailable      string
	MaxPodSchedulerFailure string

	CanaryDuration                      time.Duration
	CanaryReplicas                      string
	CanaryAutoPauseEnabled              bool
	CanaryAutoPauseMaxRestarts          int32
	CanaryAutoFailEnabled               bool
	CanaryAutoFailMaxRestarts           int32
	CanaryAutoPauseMaxSlowStartDuration time.Duration
}

func defaultEDSSpec(options *ExtendedDaemonsetOptions) edsv1alpha1.ExtendedDaemonSetSpec {
	spec := edsv1alpha1.ExtendedDaemonSetSpec{
		Strategy: edsv1alpha1.ExtendedDaemonSetSpecStrategy{
			Canary: &edsv1alpha1.ExtendedDaemonSetSpecStrategyCanary{},
		},
	}
	edsv1alpha1.DefaultExtendedDaemonSetSpec(&spec, edsv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)

	if options.MaxPodUnavailable != "" {
		spec.Strategy.RollingUpdate.MaxUnavailable = newIntOrStringPointer(options.MaxPodUnavailable)
	}

	if options.MaxPodSchedulerFailure != "" {
		spec.Strategy.RollingUpdate.MaxPodSchedulerFailure = newIntOrStringPointer(options.MaxPodSchedulerFailure)
	}

	if options.CanaryDuration != 0 {
		spec.Strategy.Canary.Duration = &metav1.Duration{Duration: options.CanaryDuration}
	}

	if options.CanaryReplicas != "" {
		spec.Strategy.Canary.Replicas = newIntOrStringPointer(options.CanaryReplicas)
	}

	spec.Strategy.Canary.AutoFail.Enabled = edsv1alpha1.NewBool(options.CanaryAutoFailEnabled)
	if options.CanaryAutoFailMaxRestarts > 0 {
		spec.Strategy.Canary.AutoFail.MaxRestarts = edsv1alpha1.NewInt32(options.CanaryAutoFailMaxRestarts)
	}

	if options.CanaryAutoPauseMaxSlowStartDuration != 0 {
		spec.Strategy.Canary.AutoPause.MaxSlowStartDuration = &metav1.Duration{Duration: options.CanaryAutoPauseMaxSlowStartDuration}
	}

	spec.Strategy.Canary.AutoPause.Enabled = edsv1alpha1.NewBool(options.CanaryAutoPauseEnabled)
	if options.CanaryAutoPauseMaxRestarts > 0 {
		spec.Strategy.Canary.AutoPause.MaxRestarts = edsv1alpha1.NewInt32(options.CanaryAutoPauseMaxRestarts)
	}
	return spec
}

func newIntOrStringPointer(str string) *intstr.IntOrString {
	val := intstr.Parse(str)
	return &val
}
