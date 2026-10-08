// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	helmtime "helm.sh/helm/v3/pkg/time"
)

func newTestConfig(t *testing.T, releases ...*release.Release) *action.Configuration {
	t.Helper()
	mem := driver.NewMemory()
	mem.SetNamespace("datadog")
	store := storage.Init(mem)
	for _, rel := range releases {
		require.NoError(t, store.Create(rel))
	}
	return &action.Configuration{
		Releases:   store,
		KubeClient: &kubefake.PrintingKubeClient{Out: io.Discard},
		Log:        noopLog,
	}
}

func testRelease(name string, version int, chartVersion string, status release.Status, deployed time.Time) *release.Release {
	return &release.Release{
		Name:      name,
		Namespace: "datadog",
		Version:   version,
		Chart: &chart.Chart{Metadata: &chart.Metadata{
			Name:    "datadog",
			Version: chartVersion,
		}},
		Info: &release.Info{
			Status:       status,
			LastDeployed: helmtime.Time{Time: deployed},
		},
		Config:   map[string]any{"datadog": map[string]any{"apiKey": "secret"}},
		Manifest: "apiVersion: v1\nkind: Secret\n",
	}
}

func TestHistory(t *testing.T) {
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cfg := newTestConfig(t,
		testRelease("dd", 2, "3.1.0", release.StatusSuperseded, base.Add(time.Hour)),
		testRelease("dd", 1, "3.0.0", release.StatusSuperseded, base),
		testRelease("dd", 3, "3.2.0", release.StatusDeployed, base.Add(2*time.Hour)),
		testRelease("other", 1, "1.0.0", release.StatusDeployed, base),
	)

	tests := []struct {
		name string
		max  int
		want []Revision
	}{
		{
			name: "all revisions newest first",
			max:  0,
			want: []Revision{
				{Chart: "datadog", ChartVersion: "3.2.0", Revision: 3, Status: "deployed", LastDeployed: base.Add(2 * time.Hour)},
				{Chart: "datadog", ChartVersion: "3.1.0", Revision: 2, Status: "superseded", LastDeployed: base.Add(time.Hour)},
				{Chart: "datadog", ChartVersion: "3.0.0", Revision: 1, Status: "superseded", LastDeployed: base},
			},
		},
		{
			name: "max keeps the newest",
			max:  2,
			want: []Revision{
				{Chart: "datadog", ChartVersion: "3.2.0", Revision: 3, Status: "deployed", LastDeployed: base.Add(2 * time.Hour)},
				{Chart: "datadog", ChartVersion: "3.1.0", Revision: 2, Status: "superseded", LastDeployed: base.Add(time.Hour)},
			},
		},
		{
			name: "max larger than history",
			max:  10,
			want: []Revision{
				{Chart: "datadog", ChartVersion: "3.2.0", Revision: 3, Status: "deployed", LastDeployed: base.Add(2 * time.Hour)},
				{Chart: "datadog", ChartVersion: "3.1.0", Revision: 2, Status: "superseded", LastDeployed: base.Add(time.Hour)},
				{Chart: "datadog", ChartVersion: "3.0.0", Revision: 1, Status: "superseded", LastDeployed: base},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := history(context.Background(), cfg, "dd", tt.max)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHistoryNotFound(t *testing.T) {
	cfg := newTestConfig(t, testRelease("other", 1, "1.0.0", release.StatusDeployed, time.Now()))

	_, err := history(context.Background(), cfg, "dd", 0)
	require.ErrorIs(t, err, ErrReleaseNotFound)
}

func TestHistoryCanceledContext(t *testing.T) {
	cfg := newTestConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := history(ctx, cfg, "dd", 0)
	require.ErrorIs(t, err, context.Canceled)
}

// TestRevisionCarriesNoReleaseContent enforces by type that Revision can
// not hold values, config or manifests.
func TestRevisionCarriesNoReleaseContent(t *testing.T) {
	allowed := map[string]reflect.Kind{
		"Chart":        reflect.String,
		"ChartVersion": reflect.String,
		"Revision":     reflect.Int,
		"Status":       reflect.String,
		"LastDeployed": reflect.Struct,
	}
	forbidden := []string{"config", "value", "manifest", "hook", "secret"}

	typ := reflect.TypeFor[Revision]()
	require.Equal(t, len(allowed), typ.NumField(), "Revision fields changed; make sure Revision cannot carry release values, config or manifests")
	for i := range typ.NumField() {
		f := typ.Field(i)
		kind, ok := allowed[f.Name]
		require.True(t, ok, "unexpected field %s", f.Name)
		assert.Equal(t, kind, f.Type.Kind(), "field %s", f.Name)
		for _, word := range forbidden {
			assert.NotContains(t, strings.ToLower(f.Name), word, "field %s", f.Name)
		}
	}
	assert.Equal(t, reflect.TypeFor[time.Time](), typ.Field(4).Type)
}

func TestStorageDriver(t *testing.T) {
	for in, want := range map[string]string{"": DriverSecret, "secret": DriverSecret, "secrets": DriverSecret, "configmap": DriverConfigMap, "configmaps": DriverConfigMap} {
		got, err := storageDriver(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"sql", "memory", "Secret"} {
		_, err := storageDriver(in)
		require.ErrorContains(t, err, "unsupported Helm storage driver", in)
	}
	_, err := HistoryFrom(context.Background(), nil, "sql", "ns", "rel", 0)
	require.ErrorContains(t, err, `unsupported Helm storage driver "sql"`)
}
