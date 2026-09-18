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
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func Test_buildPodDisruptionBudget(t *testing.T) {
	instance := newTestInstance()
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		PodDisruptionBudget: &datadoghqv1alpha1.DatadogSPLPodDisruptionBudget{
			Enabled:      true,
			MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
	}

	pdb := buildPodDisruptionBudget(instance)
	assert.Equal(t, "my-pl", pdb.Name)
	assert.Equal(t, "default", pdb.Namespace)
	assert.Equal(t, &intstr.IntOrString{Type: intstr.Int, IntVal: 1}, pdb.Spec.MinAvailable)
	assert.Nil(t, pdb.Spec.MaxUnavailable)
	assert.Equal(t, selectorLabels(instance), pdb.Spec.Selector.MatchLabels)
}

func Test_reconcilePodDisruptionBudgetLifecycle(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	instance := newTestInstance()
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		PodDisruptionBudget: &datadoghqv1alpha1.DatadogSPLPodDisruptionBudget{
			Enabled:      true,
			MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
	}

	// Create.
	require.NoError(t, reconcilePodDisruptionBudget(context.Background(), c, s, instance))
	pdb := &policyv1.PodDisruptionBudget{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, pdb))
	assertOwnedByInstance(t, pdb, instance)

	// Update after spec change.
	instance.Spec.Worker.PodDisruptionBudget.MinAvailable = &intstr.IntOrString{Type: intstr.Int, IntVal: 2}
	require.NoError(t, reconcilePodDisruptionBudget(context.Background(), c, s, instance))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, pdb))
	assert.Equal(t, intstr.FromInt32(2), *pdb.Spec.MinAvailable)

	// Disable: the stale PDB is deleted.
	instance.Spec.Worker.PodDisruptionBudget.Enabled = false
	require.NoError(t, reconcilePodDisruptionBudget(context.Background(), c, s, instance))
	err := c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, pdb)
	assert.Error(t, err)
}

func Test_reconcilePodDisruptionBudgetDisabledWithoutObject(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	instance := newTestInstance()

	err := reconcilePodDisruptionBudget(context.Background(), c, s, instance)
	require.NoError(t, err)
	err = c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, &policyv1.PodDisruptionBudget{})
	assert.Error(t, err)
}
