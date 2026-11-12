// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

// NewDaemonset use to generate the skeleton of a new daemonset based on few information
func NewDaemonset(owner metav1.Object, componentKind, componentName, version string, selector *metav1.LabelSelector, instanceName string) *appsv1.DaemonSet {
	labels, annotations, selector := common.GetDefaultMetadata(owner, componentKind, instanceName, version, selector)

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
