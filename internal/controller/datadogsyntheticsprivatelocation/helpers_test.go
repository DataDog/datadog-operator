// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func newSchemeWithSPL(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(s))
	return s
}

func assertOwnedByInstance(t *testing.T, obj client.Object, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) {
	t.Helper()
	ownerRefs := obj.GetOwnerReferences()
	require.Len(t, ownerRefs, 1)
	assert.Equal(t, instance.Name, ownerRefs[0].Name)
	assert.Equal(t, "DatadogSyntheticsPrivateLocation", ownerRefs[0].Kind)
	assert.True(t, *ownerRefs[0].Controller)
}
