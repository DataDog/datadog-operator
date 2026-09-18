// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func Test_reconcileServiceAccountCreateByDefault(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	instance := newTestInstance()

	saName, err := reconcileServiceAccount(context.Background(), c, s, instance)
	require.NoError(t, err)
	assert.Equal(t, "my-pl", saName)

	sa := &corev1.ServiceAccount{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, sa))
	assertOwnedByInstance(t, sa, instance)
}

func Test_reconcileServiceAccountExplicitCreateWithAnnotations(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	instance := newTestInstance()
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		ServiceAccount: &datadoghqv1alpha1.DatadogSPLServiceAccount{
			Name:        "custom-sa",
			Create:      true,
			Annotations: map[string]string{"eks.amazonaws.com/role-arn": "arn:aws:iam::123:role/x"},
		},
	}

	saName, err := reconcileServiceAccount(context.Background(), c, s, instance)
	require.NoError(t, err)
	assert.Equal(t, "custom-sa", saName)

	sa := &corev1.ServiceAccount{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "custom-sa", Namespace: "default"}, sa))
	assertOwnedByInstance(t, sa, instance)
	assert.Equal(t, "arn:aws:iam::123:role/x", sa.Annotations["eks.amazonaws.com/role-arn"])
}

func Test_reconcileServiceAccountReference(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	instance := newTestInstance()
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		ServiceAccount: &datadoghqv1alpha1.DatadogSPLServiceAccount{
			Name:   "existing-sa",
			Create: false,
		},
	}

	saName, err := reconcileServiceAccount(context.Background(), c, s, instance)
	require.NoError(t, err)
	assert.Equal(t, "existing-sa", saName)

	// Nothing is created for a referenced ServiceAccount.
	err = c.Get(context.Background(), client.ObjectKey{Name: "existing-sa", Namespace: "default"}, &corev1.ServiceAccount{})
	assert.Error(t, err)
}

func Test_reconcileServiceAccountUpdate(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	instance := newTestInstance()

	_, err := reconcileServiceAccount(context.Background(), c, s, instance)
	require.NoError(t, err)

	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		ServiceAccount: &datadoghqv1alpha1.DatadogSPLServiceAccount{
			Annotations: map[string]string{"foo": "bar"},
		},
	}
	_, err = reconcileServiceAccount(context.Background(), c, s, instance)
	require.NoError(t, err)

	sa := &corev1.ServiceAccount{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, sa))
	assert.Equal(t, "bar", sa.Annotations["foo"])
}
