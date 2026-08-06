// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func newTestCSIInstance(commonLabels map[string]string) *datadoghqv1alpha1.DatadogCSIDriver {
	return &datadoghqv1alpha1.DatadogCSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "default",
		},
		Spec: datadoghqv1alpha1.DatadogCSIDriverSpec{
			CommonLabels: commonLabels,
		},
	}
}

func TestBuildDaemonSet_CommonLabels(t *testing.T) {
	instance := newTestCSIInstance(map[string]string{
		"team":        "platform",
		"cost-center": "ops",
	})

	ds := buildDaemonSet(instance)

	// Extra labels must appear on the DaemonSet ObjectMeta
	assert.Equal(t, "platform", ds.Labels["team"])
	assert.Equal(t, "ops", ds.Labels["cost-center"])
	// Extra labels must appear on the pod template
	assert.Equal(t, "platform", ds.Spec.Template.Labels["team"])
	assert.Equal(t, "ops", ds.Spec.Template.Labels["cost-center"])
	// Operator-owned labels must still be present and not overridden
	assert.Equal(t, csiDsName, ds.Labels[AppLabelKey])
	assert.Equal(t, csiDsName, ds.Spec.Template.Labels[AppLabelKey])
}

func TestBuildDaemonSet_CommonLabels_CannotOverrideOperatorKeys(t *testing.T) {
	instance := newTestCSIInstance(map[string]string{
		AppLabelKey: "my-override", // attempt to override reserved CSI key
		"team":      "platform",
	})

	ds := buildDaemonSet(instance)

	// Operator key must not be overridden
	assert.Equal(t, csiDsName, ds.Labels[AppLabelKey])
	assert.Equal(t, csiDsName, ds.Spec.Template.Labels[AppLabelKey])
	// Non-conflicting custom key still passes through
	assert.Equal(t, "platform", ds.Labels["team"])
}

func TestBuildDaemonSet_NoCommonLabels(t *testing.T) {
	instance := newTestCSIInstance(nil)
	ds := buildDaemonSet(instance)
	assert.Equal(t, csiDsName, ds.Labels[AppLabelKey])
}

func TestBuildCSIDriverObject_CommonLabels(t *testing.T) {
	instance := newTestCSIInstance(map[string]string{
		"team":        "platform",
		"cost-center": "ops",
	})

	csiDriver := buildCSIDriverObject(instance)

	assert.Equal(t, "platform", csiDriver.Labels["team"])
	assert.Equal(t, "ops", csiDriver.Labels["cost-center"])
	// Operator-owned labels must still be present
	assert.Equal(t, "datadog-operator", csiDriver.Labels["app.kubernetes.io/managed-by"])
}

func TestBuildCSIDriverObject_CommonLabels_CannotOverrideOperatorKeys(t *testing.T) {
	instance := newTestCSIInstance(map[string]string{
		"app.kubernetes.io/managed-by": "my-operator",
		"team":                         "platform",
	})

	csiDriver := buildCSIDriverObject(instance)

	assert.Equal(t, "datadog-operator", csiDriver.Labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "platform", csiDriver.Labels["team"])
}
