// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func TestSelectionValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sel     selection
		wantErr string
	}{
		{name: "nothing selected", wantErr: "select DatadogPodAutoscalers"},
		{name: "all namespaces alone", sel: selection{allNamespaces: true}, wantErr: "select DatadogPodAutoscalers"},
		{name: "names", sel: selection{names: []string{"a"}}},
		{name: "label selector", sel: selection{labelSelector: "app=web"}},
		{name: "all", sel: selection{all: true}},
		{name: "all namespaces with all", sel: selection{all: true, allNamespaces: true}},
		{name: "all namespaces with selector", sel: selection{labelSelector: "app=web", allNamespaces: true}},
		{name: "names and selector", sel: selection{names: []string{"a"}, labelSelector: "app=web"}, wantErr: "cannot be combined"},
		{name: "names and all", sel: selection{names: []string{"a"}, all: true}, wantErr: "cannot be combined"},
		{name: "names and all namespaces", sel: selection{names: []string{"a"}, allNamespaces: true}},
		{name: "selector and all", sel: selection{labelSelector: "app=web", all: true}, wantErr: "mutually exclusive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sel.validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestSelectionResolve(t *testing.T) {
	c := newFakeClient(t,
		newDPA("ns1", "b", nil, map[string]string{"app": "web"}),
		newDPA("ns1", "a", nil, map[string]string{"app": "api"}),
		newDPA("ns2", "c", nil, map[string]string{"app": "web"}),
		newDPA("ns2", "b", nil, map[string]string{"app": "api"}),
	)

	names := func(dpas []v1alpha2.DatadogPodAutoscaler) []string {
		out := make([]string, 0, len(dpas))
		for _, d := range dpas {
			out = append(out, d.Namespace+"/"+d.Name)
		}
		return out
	}

	for _, tc := range []struct {
		name    string
		sel     selection
		want    []string
		wantErr string
	}{
		{name: "names, sorted", sel: selection{names: []string{"b", "a"}}, want: []string{"ns1/a", "ns1/b"}},
		{name: "missing name", sel: selection{names: []string{"a", "nope"}}, wantErr: "ns1/nope"},
		{name: "invalid selector", sel: selection{labelSelector: "app in ("}, wantErr: "invalid --label-selector"},
		{name: "all in namespace", sel: selection{all: true}, want: []string{"ns1/a", "ns1/b"}},
		{name: "all namespaces", sel: selection{all: true, allNamespaces: true}, want: []string{"ns1/a", "ns1/b", "ns2/b", "ns2/c"}},
		{name: "selector in namespace", sel: selection{labelSelector: "app=web"}, want: []string{"ns1/b"}},
		{name: "selector in all namespaces", sel: selection{labelSelector: "app=web", allNamespaces: true}, want: []string{"ns1/b", "ns2/c"}},
		{name: "all namespaces alone", sel: selection{allNamespaces: true}, want: []string{"ns1/a", "ns1/b", "ns2/b", "ns2/c"}},
		{name: "names in all namespaces", sel: selection{names: []string{"b"}, allNamespaces: true}, want: []string{"ns1/b", "ns2/b"}},
		{name: "missing name in all namespaces", sel: selection{names: []string{"b", "nope"}, allNamespaces: true}, wantErr: "nope not found in any namespace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dpas, err := tc.sel.resolve(context.Background(), c, "ns1")
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, names(dpas))
		})
	}
}
