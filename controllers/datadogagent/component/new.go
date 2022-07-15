// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package component

import (
	appsv1 "k8s.io/api/apps/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// NewDeployment use to generate the skeleton of a new deployment based on few information
func NewDeployment(owner metav1.Object, componentKind, componentName, version string, inputSelector *metav1.LabelSelector) *appsv1.Deployment {
	labels, annotations, selector := getDefaultMetadata(owner, componentKind, componentName, version, inputSelector)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentName,
			Namespace:   owner.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: selector,
		},
	}

	return deployment
}

// NewDaemonset use to generate the skeleton of a new daemonset based on few information
func NewDaemonset(owner metav1.Object, componentKind, componentName, version string, selector *metav1.LabelSelector) *appsv1.DaemonSet {
	labels, annotations, selector := getDefaultMetadata(owner, componentKind, componentName, version, selector)

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
func NewExtendedDaemonset(owner metav1.Object, componentKind, componentName, version string, selector *metav1.LabelSelector) *edsv1alpha1.ExtendedDaemonSet {
	labels, annotations, selector := getDefaultMetadata(owner, componentKind, componentName, version, selector)

	daemonset := &edsv1alpha1.ExtendedDaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentName,
			Namespace:   owner.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: edsv1alpha1.ExtendedDaemonSetSpec{
			Selector: selector,
			Strategy: defaultEDSStrategy(),
		},
	}

	return daemonset
}

func defaultEDSStrategy() edsv1alpha1.ExtendedDaemonSetSpecStrategy {
	return edsv1alpha1.ExtendedDaemonSetSpecStrategy{
		Canary:        edsv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(nil, edsv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto),
		RollingUpdate: *edsv1alpha1.DefaultExtendedDaemonSetSpecStrategyRollingUpdate(nil),
		ReconcileFrequency: &metav1.Duration{
			Duration: apicommon.DefaultReconcileFrequency,
		},
	}
}

func getDefaultMetadata(owner metav1.Object, componentKind, componentName, version string, selector *metav1.LabelSelector) (map[string]string, map[string]string, *metav1.LabelSelector) {
	labels := getDefaultLabels(owner, componentKind, componentName, version)
	annotations := object.GetDefaultAnnotations(owner)

	if selector != nil {
		for key, val := range selector.MatchLabels {
			labels[key] = val
		}
	} else {
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				apicommon.AgentDeploymentNameLabelKey:      owner.GetName(),
				apicommon.AgentDeploymentComponentLabelKey: componentKind,
			},
		}
	}
	return labels, annotations, selector
}

func getDefaultLabels(owner metav1.Object, componentKind, componentName, version string) map[string]string {
	labels := object.GetDefaultLabels(owner, componentName, version)
	labels[apicommon.AgentDeploymentNameLabelKey] = owner.GetName()
	labels[apicommon.AgentDeploymentComponentLabelKey] = componentKind

	return labels
}
