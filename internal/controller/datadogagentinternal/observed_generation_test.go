// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func newObservedGenerationTestDDAI(annotations map[string]string, features *v2alpha1.DatadogFeatures) *v1alpha1.DatadogAgentInternal {
	return &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "profile-a",
			Namespace:   "ns",
			Labels:      map[string]string{constants.ProfileLabelKey: "profile-a"},
			Annotations: annotations,
			Finalizers:  []string{constants.DatadogAgentInternalFinalizer},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{APIKey: ptr.To("0000000000000000000000")},
			},
			Features: features,
		},
	}
}

func reconcileForObservedGeneration(t *testing.T, ddai *v1alpha1.DatadogAgentInternal) *v1alpha1.DatadogAgentInternal {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, v1alpha1.AddToScheme(s))
	require.NoError(t, v2alpha1.AddToScheme(s))
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(ddai).
		WithStatusSubresource(&v1alpha1.DatadogAgentInternal{}, &appsv1.DaemonSet{}).
		Build()
	r := NewReconciler(ReconcilerOptions{}, c, kubernetes.PlatformInfo{}, s, record.NewFakeRecorder(100), nil)

	live := &v1alpha1.DatadogAgentInternal{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Namespace: ddai.Namespace, Name: ddai.Name}, live))
	live.Generation = 5
	_, _ = r.Reconcile(context.TODO(), live)

	got := &v1alpha1.DatadogAgentInternal{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Namespace: ddai.Namespace, Name: ddai.Name}, got))
	return got
}

func TestObservedGeneration_SetAfterSuccessfulReconcile(t *testing.T) {
	got := reconcileForObservedGeneration(t, newObservedGenerationTestDDAI(nil, nil))
	assert.Equal(t, int64(5), got.Status.ObservedGeneration)
}

func TestObservedGeneration_NotAdvancedWhenBlocked(t *testing.T) {
	ddai := newObservedGenerationTestDDAI(
		map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.GKEAutopilotProvider},
		&v2alpha1.DatadogFeatures{CWS: &v2alpha1.CWSFeatureConfig{Enabled: ptr.To(true)}},
	)
	got := reconcileForObservedGeneration(t, ddai)
	assert.Equal(t, metav1.ConditionTrue, conditionStatus(&got.Status, common.FeatureNotSupportedOnProviderConditionType))
	assert.Equal(t, int64(0), got.Status.ObservedGeneration)
}
