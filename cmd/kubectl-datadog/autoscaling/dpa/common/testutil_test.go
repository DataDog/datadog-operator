// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(s))
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func newDPA(namespace, name string, annotations map[string]string, labels map[string]string) *v1alpha2.DatadogPodAutoscaler {
	return &v1alpha2.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: annotations,
			Labels:      labels,
		},
	}
}

func withCondition(dpa *v1alpha2.DatadogPodAutoscaler, conditionType datadoghqcommon.DatadogPodAutoscalerConditionType, status corev1.ConditionStatus, reason, message string) *v1alpha2.DatadogPodAutoscaler {
	dpa.Status.Conditions = append(dpa.Status.Conditions, datadoghqcommon.DatadogPodAutoscalerCondition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
	return dpa
}
