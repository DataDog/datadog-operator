// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestMetricsPort(t *testing.T) {
	tests := []struct {
		name      string
		container corev1.Container
		want      string
		wantErr   bool
	}{
		{name: "default", container: corev1.Container{}, want: "8080"},
		{name: "flag with equal", container: corev1.Container{Args: []string{"-metrics-addr=:8383"}}, want: "8383"},
		{name: "flag with space", container: corev1.Container{Args: []string{"--metrics-addr", "0.0.0.0:9090"}}, want: "9090"},
		{name: "env", container: corev1.Container{Env: []corev1.EnvVar{{Name: "DD_METRICS_ADDR", Value: ":8384"}}}, want: "8384"},
		{name: "flag wins over env", container: corev1.Container{Args: []string{"-metrics-addr=:8385"}, Env: []corev1.EnvVar{{Name: "DD_METRICS_ADDR", Value: ":8384"}}}, want: "8385"},
		{name: "disabled", container: corev1.Container{Args: []string{"-metrics-addr=0"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, err := metricsPort(&corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{tt.container}}})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, port)
		})
	}
}

func TestValidateSite(t *testing.T) {
	email, apiKey = "user@example.com", "key"
	t.Cleanup(func() { email, apiKey, ddSite = "", "", "us" })
	for site, want := range map[string]string{"us": "datadoghq.com", "EU": "datadoghq.eu"} {
		ddSite = site
		o := &options{}
		require.NoError(t, o.validate())
		assert.Equal(t, want, o.site)
	}
	ddSite = "mars"
	require.Error(t, (&options{}).validate())
}
