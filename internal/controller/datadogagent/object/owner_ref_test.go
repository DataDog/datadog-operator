// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package object

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestSetOwnerReference(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v2alpha1.AddToScheme(scheme))

	owner := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "datadog",
			UID:  types.UID("owner-uid"),
		},
	}
	child := &metav1.ObjectMeta{Name: "child"}

	require.NoError(t, SetOwnerReference(owner, child, scheme))
	require.True(t, CheckOwnerReference(owner, child))
	require.Equal(t, []metav1.OwnerReference{{
		APIVersion:         v2alpha1.GroupVersion.String(),
		Kind:               "DatadogAgent",
		Name:               "datadog",
		UID:                types.UID("owner-uid"),
		Controller:         boolPtr(true),
		BlockOwnerDeletion: boolPtr(true),
	}}, child.OwnerReferences)

	owner.UID = types.UID("new-owner-uid")
	require.NoError(t, SetOwnerReference(owner, child, scheme))
	require.Len(t, child.OwnerReferences, 1)
	require.Equal(t, types.UID("new-owner-uid"), child.OwnerReferences[0].UID)
}

func TestCreateOwnerRefRejectsNonRuntimeObject(t *testing.T) {
	ref, err := CreateOwnerRef(&metav1.ObjectMeta{Name: "not-runtime"}, runtime.NewScheme())

	require.Error(t, err)
	require.Nil(t, ref)
}

func TestReferSameObject(t *testing.T) {
	base := metav1.OwnerReference{
		APIVersion: "datadoghq.com/v2alpha1",
		Kind:       "DatadogAgent",
		Name:       "datadog",
	}

	require.True(t, referSameObject(base, base))
	require.False(t, referSameObject(base, metav1.OwnerReference{
		APIVersion: "datadoghq.com/v2alpha1",
		Kind:       "DatadogAgent",
		Name:       "other",
	}))
	require.False(t, referSameObject(base, metav1.OwnerReference{
		APIVersion: "not a group version",
		Kind:       "DatadogAgent",
		Name:       "datadog",
	}))
}

func boolPtr(value bool) *bool {
	return &value
}
