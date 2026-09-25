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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func Test_reconcileServiceAccountUpdateKeepsAnnotations(t *testing.T) {
	s := newSchemeWithSPL(t)
	instance := newTestInstance()
	existing := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pl",
			Namespace:   "default",
			Labels:      map[string]string{"stale": "label"},
			Annotations: map[string]string{"eks.amazonaws.com/role-arn": "arn:aws:iam::123:role/x"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(existing).Build()

	_, err := reconcileServiceAccount(context.Background(), c, s, instance)
	require.NoError(t, err)

	sa := &corev1.ServiceAccount{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, sa))
	assert.Equal(t, splLabels(instance), sa.Labels)
	assert.Equal(t, "arn:aws:iam::123:role/x", sa.Annotations["eks.amazonaws.com/role-arn"])
}
