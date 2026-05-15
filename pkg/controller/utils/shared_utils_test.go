// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestShouldReturn(t *testing.T) {
	tests := []struct {
		name   string
		result reconcile.Result
		err    error
		want   bool
	}{
		{name: "keeps reconciling when result and error are empty", want: false},
		{name: "returns on error", err: errors.New("boom"), want: true},
		{name: "returns on non-empty result", result: reconcile.Result{Requeue: true}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ShouldReturn(tt.result, tt.err))
		})
	}
}

func TestUseCustomSeccompConfig(t *testing.T) {
	configData := "{}"

	require.False(t, UseCustomSeccompConfigMap(nil))
	require.False(t, UseCustomSeccompConfigData(nil))

	require.True(t, UseCustomSeccompConfigMap(&v2alpha1.SeccompConfig{
		CustomProfile: &v2alpha1.CustomConfig{
			ConfigMap: &v2alpha1.ConfigMapConfig{Name: "profile"},
		},
	}))
	require.False(t, UseCustomSeccompConfigData(&v2alpha1.SeccompConfig{
		CustomProfile: &v2alpha1.CustomConfig{
			ConfigMap:  &v2alpha1.ConfigMapConfig{Name: "profile"},
			ConfigData: ptr.To(configData),
		},
	}))

	require.True(t, UseCustomSeccompConfigData(&v2alpha1.SeccompConfig{
		CustomProfile: &v2alpha1.CustomConfig{
			ConfigData: ptr.To(configData),
		},
	}))
}
