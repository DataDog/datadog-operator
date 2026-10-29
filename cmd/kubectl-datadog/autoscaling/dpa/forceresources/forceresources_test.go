// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forceresources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/common"
)

func dpaWith(value string) *v1alpha2.DatadogPodAutoscaler {
	dpa := &v1alpha2.DatadogPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "checkout-dpa"}}
	if value != "" {
		dpa.Annotations = map[string]string{common.ForceResourcesAnnotationKey: value}
	}
	return dpa
}

func setOptions(t *testing.T, containers []string, requests, limits string) *options {
	t.Helper()
	o := &options{containers: containers, requests: requests, limits: limits}
	require.NoError(t, o.validateSet())
	return o
}

func TestSetMerges(t *testing.T) {
	const appCPU = `[{"name":"app","limits":{"cpu":"4"},"requests":{"cpu":"2"}}]`

	for _, tc := range []struct {
		name       string
		current    string
		containers []string
		requests   string
		limits     string
		want       string
		wantErr    string
	}{
		{
			name:       "no annotation yet",
			containers: []string{"app"}, requests: "cpu=2", limits: "cpu=4",
			want: appCPU,
		},
		{
			name:       "a new container keeps the existing one",
			current:    appCPU,
			containers: []string{"sidecar"}, requests: "memory=512Mi",
			want: `[{"name":"app","limits":{"cpu":"4"},"requests":{"cpu":"2"}},{"name":"sidecar","requests":{"memory":"512Mi"}}]`,
		},
		{
			name:       "a new resource keeps the existing ones of that container",
			current:    appCPU,
			containers: []string{"app"}, requests: "memory=1Gi",
			want: `[{"name":"app","limits":{"cpu":"4"},"requests":{"cpu":"2","memory":"1Gi"}}]`,
		},
		{
			name:       "a field is overwritten, the other one kept",
			current:    appCPU,
			containers: []string{"app"}, requests: "cpu=3",
			want: `[{"name":"app","limits":{"cpu":"4"},"requests":{"cpu":"3"}}]`,
		},
		{
			name:       "the same values apply to every container",
			current:    appCPU,
			containers: []string{"app", "sidecar"}, limits: "memory=2Gi",
			want: `[{"name":"app","limits":{"cpu":"4","memory":"2Gi"},"requests":{"cpu":"2"}},{"name":"sidecar","limits":{"memory":"2Gi"}}]`,
		},
		{
			name:       "the merged result is validated",
			current:    appCPU,
			containers: []string{"app"}, requests: "cpu=6",
			wantErr: `merged with the current value: container "app": cpu request 6 is greater than limit 4`,
		},
		{
			name:       "an invalid current value is never overwritten",
			current:    `[{"name":"app","requests":{"memory":"200Mb"}}]`,
			containers: []string{"app"}, requests: "cpu=2",
			wantErr: `current value is invalid`,
		},
		{
			name:       "a hand-written value is rewritten canonically",
			current:    `[ { "requests": { "cpu": "2000m" }, "name": "app", "limits": { "cpu": "4" } } ]`,
			containers: []string{"app"}, requests: "cpu=2",
			want: appCPU,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := setOptions(t, tc.containers, tc.requests, tc.limits)
			got, err := o.setValue(dpaWith(tc.current))
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Value)
		})
	}
}

func TestSetIsStable(t *testing.T) {
	o := setOptions(t, []string{"app"}, "cpu=2", "cpu=4")
	first, err := o.setValue(dpaWith(""))
	require.NoError(t, err)
	second, err := o.setValue(dpaWith(first.Value))
	require.NoError(t, err)
	assert.Equal(t, first, second, "re-running the same set is a no-op")
}

func TestUnset(t *testing.T) {
	const both = `[{"name":"app","requests":{"cpu":"2"}},{"name":"sidecar","requests":{"memory":"512Mi"}}]`

	for _, tc := range []struct {
		name       string
		current    string
		containers []string
		want       string
		wantErr    string
	}{
		{name: "without -c, the whole annotation", current: both, want: ""},
		{name: "without -c, even if invalid", current: `[{"name": `, want: ""},
		{name: "only the named container", current: both, containers: []string{"sidecar"}, want: `[{"name":"app","requests":{"cpu":"2"}}]`},
		{name: "the last container removes the annotation", current: both, containers: []string{"app", "sidecar"}, want: ""},
		{name: "an unknown container is ignored", current: `[{"name":"app","requests":{"cpu":"2"}}]`, containers: []string{"other"}, want: `[{"name":"app","requests":{"cpu":"2"}}]`},
		{name: "with -c, an invalid value fails", current: `[{"name": `, containers: []string{"app"}, wantErr: `run "unset" without -c`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &options{containers: tc.containers}
			got, err := o.unsetValue(dpaWith(tc.current))
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want == "", got.Remove, "an empty override removes the annotation")
			assert.Equal(t, tc.want, got.Value)
		})
	}
}

// The flag checks run before any connection to the cluster.
func TestSetValidatesFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no container", args: []string{"set", "--requests", "cpu=2", "checkout-dpa"}, wantErr: "--container is required"},
		{name: "empty container", args: []string{"set", "-c", "", "--requests", "cpu=2", "checkout-dpa"}, wantErr: "--container is required"},
		{name: "zero quantity", args: []string{"set", "-c", "app", "--requests", "cpu=0", "checkout-dpa"}, wantErr: "cpu quantities must be positive"},
		{name: "no requests nor limits", args: []string{"set", "-c", "app", "checkout-dpa"}, wantErr: "at least one of --requests or --limits"},
		{name: "unsupported resource", args: []string{"set", "-c", "app", "--requests", "nvidia.com/gpu=1", "checkout-dpa"}, wantErr: `unsupported resource "nvidia.com/gpu"`},
		{name: "bad quantity", args: []string{"set", "-c", "app", "--limits", "memory=200Mb", "checkout-dpa"}, wantErr: "--limits: invalid memory quantity"},
		{name: "request above limit", args: []string{"set", "-c", "app", "--requests", "cpu=4", "--limits", "cpu=2", "checkout-dpa"}, wantErr: "cpu request 4 is greater than limit 2"},
		{name: "no DPA selected", args: []string{"set", "-c", "app", "--requests", "cpu=2"}, wantErr: "select DatadogPodAutoscalers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			streams := genericclioptions.NewTestIOStreamsDiscard()
			cmd := New(streams)
			cmd.SetArgs(tc.args)
			cmd.SetOut(streams.Out)
			cmd.SetErr(streams.ErrOut)
			assert.ErrorContains(t, cmd.Execute(), tc.wantErr)
		})
	}
}
