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

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
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
	// FIXME (@CharlyF): The EDS controller uses the Spec.Selector as a node selector to get the NodeList to rollout the agent.
	// Per https://github.com/DataDog/extendeddaemonset/blob/28a8e082cee9890ae6d925a7d6247a36c6f6ba5d/controllers/extendeddaemonsetreplicaset/controller.go#L344-L360
	// Up until v0.8.2, the Datadog Operator set the selector to nil, which circumvented this case.
	// Until the EDS controller uses the Affinity field to get the NodeList instead of Spec.Selector, let's keep the previous behavior.
	labels, annotations, _ := getDefaultMetadata(owner, componentKind, componentName, version, selector)

	daemonset := &edsv1alpha1.ExtendedDaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentName,
			Namespace:   owner.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: edsv1alpha1.ExtendedDaemonSetSpec{
			Selector: nil,
			Strategy: defaultEDSStrategy(),
		},
	}

	return daemonset
}

func defaultEDSStrategy() edsv1alpha1.ExtendedDaemonSetSpecStrategy {
	return edsv1alpha1.ExtendedDaemonSetSpecStrategy{
		Canary: edsv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(
			&edsv1alpha1.ExtendedDaemonSetSpecStrategyCanary{},
			edsv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto,
		),
		RollingUpdate: *edsv1alpha1.DefaultExtendedDaemonSetSpecStrategyRollingUpdate(&edsv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{}),
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
	labels[kubernetes.AppKubernetesComponentLabelKey] = componentName
	labels[apicommon.AgentDeploymentNameLabelKey] = owner.GetName()
	labels[apicommon.AgentDeploymentComponentLabelKey] = componentKind


	return labels
}
