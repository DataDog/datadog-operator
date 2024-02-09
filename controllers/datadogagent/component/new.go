// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package component

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewDeployment use to generate the skeleton of a new deployment based on few information
func NewDeployment(owner metav1.Object, componentKind, componentName, version string, inputSelector *metav1.LabelSelector) *appsv1.Deployment {
	labels, annotations, selector := GetDefaultMetadata(owner, componentKind, componentName, version, inputSelector)

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

func GetDefaultMetadata(owner metav1.Object, componentKind, componentName, version string, selector *metav1.LabelSelector) (map[string]string, map[string]string, *metav1.LabelSelector) {
	labels := GetDefaultLabels(owner, componentKind, componentName, version)
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

func GetDefaultLabels(owner metav1.Object, componentKind, componentName, version string) map[string]string {
	labels := object.GetDefaultLabels(owner, componentName, version)
	labels[apicommon.AgentDeploymentNameLabelKey] = owner.GetName()
	labels[apicommon.AgentDeploymentComponentLabelKey] = componentKind
	labels[kubernetes.AppKubernetesComponentLabelKey] = componentKind

	return labels
}
