// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"

	dash "github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
	"github.com/DataDog/datadog-operator/pkg/plugin/dashboard/render"
	"github.com/DataDog/datadog-operator/pkg/rollout"
)

var testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

const fixtures = "../../../pkg/plugin/dashboard/testdata"

// writeKubeconfig writes a kubeconfig whose current context targets server
// in namespace datadog, so that tests never read the user's kubeconfig.
func writeKubeconfig(t *testing.T, server string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	cfg := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster: {server: %q}
contexts:
- name: kind-test
  context: {cluster: test, user: test, namespace: datadog}
current-context: kind-test
users:
- name: test
  user: {token: test}
`, server)
	require.NoError(t, os.WriteFile(path, []byte(cfg), 0o600))
	return path
}

// newTestOptions returns options writing to buffers, with a fixed clock, an
// isolated kubeconfig and an empty environment.
func newTestOptions(t *testing.T, server string) (*options, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	o := newOptions(genericclioptions.IOStreams{In: &bytes.Buffer{}, Out: out, ErrOut: errOut})
	kubeconfig := writeKubeconfig(t, server)
	cacheDir := t.TempDir()
	o.ConfigFlags.KubeConfig = &kubeconfig
	o.ConfigFlags.CacheDir = &cacheDir
	o.now = func() time.Time { return testNow }
	o.getenv = func(string) string { return "" }
	return o, out, errOut
}

// runFixture runs the command on a fixture scenario.
func runFixture(t *testing.T, scenario string, args ...string) (string, string, error) {
	t.Helper()
	o, out, errOut := newTestOptions(t, "https://127.0.0.1:1")
	o.newStore = func(*options) (dash.Store, error) {
		return dash.NewFixtureStore(filepath.Join(fixtures, scenario)), nil
	}
	cmd := newCmd(o)
	cmd.SetArgs(args)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestDashboardPlainOutput(t *testing.T) {
	out, _, err := runFixture(t, "profiles")
	require.NoError(t, err)
	assert.NotContains(t, out, "\x1b", "non-TTY output has no escape sequence")

	snap, err := dash.LoadFixture(filepath.Join(fixtures, "profiles"))
	require.NoError(t, err)
	v, err := dash.Build(snap, dash.BuildConfig{StallAfter: defaultStallAfter}, nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, render.Text(v, render.Theme{Width: render.DefaultWidth}, testNow), out)
}

func TestDashboardOptionsReachRenderer(t *testing.T) {
	out, _, err := runFixture(t, "stalled", "--ascii", "--pods", "--stall-after=10m")
	require.NoError(t, err)
	snap, err := dash.LoadFixture(filepath.Join(fixtures, "stalled"))
	require.NoError(t, err)
	v, err := dash.Build(snap, dash.FixtureBuildConfig("stalled"), nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, render.Text(v, render.Theme{ASCII: true, Width: render.DefaultWidth}, testNow), out)
}

func TestDashboardWidthFromColumns(t *testing.T) {
	o, out, _ := newTestOptions(t, "https://127.0.0.1:1")
	o.getenv = func(k string) string {
		if k == "COLUMNS" {
			return "120"
		}
		return ""
	}
	assert.Equal(t, render.Theme{Color: colorprofile.NoTTY, Width: 120}, o.theme())
	assert.Empty(t, out.String())
}

func TestDashboardJSON(t *testing.T) {
	out, _, err := runFixture(t, "steady", "-o", "json")
	require.NoError(t, err)
	var v dash.View
	require.NoError(t, json.Unmarshal([]byte(out), &v))
	assert.Equal(t, dash.SchemaVersion, v.SchemaVersion)
	assert.Equal(t, "datadog-agent", v.DDA.Name)
}

func TestDashboardSelection(t *testing.T) {
	t.Run("two DDAs", func(t *testing.T) {
		out, errOut, err := runFixture(t, "multiple-ddas")
		require.ErrorIs(t, err, dash.ErrMultipleDDAs)
		assert.Empty(t, out)
		assert.Contains(t, errOut, "Warning: more than one DatadogAgent found")
		assert.Contains(t, errOut, "  datadog/datadog-agent\n  datadog/second\n")
	})
	t.Run("two DDAs with -o json", func(t *testing.T) {
		out, _, err := runFixture(t, "multiple-ddas", "-o", "json")
		require.ErrorIs(t, err, dash.ErrMultipleDDAs)
		assert.Empty(t, out)
	})
	t.Run("named DDA", func(t *testing.T) {
		out, _, err := runFixture(t, "multiple-ddas", "second")
		require.NoError(t, err)
		assert.Contains(t, out, "second")
	})
	t.Run("no DDA", func(t *testing.T) {
		_, _, err := runFixture(t, "no-dda")
		require.ErrorIs(t, err, dash.ErrNoDDA)
		assert.ErrorContains(t, err, `in namespace "datadog"`)
	})
	t.Run("no DAP CRD", func(t *testing.T) {
		out, _, err := runFixture(t, "no-dap-crd")
		require.NoError(t, err)
		assert.Contains(t, out, "profiles not installed")
	})
	t.Run("unsupported operator", func(t *testing.T) {
		_, _, err := runFixture(t, "unsupported")
		require.ErrorIs(t, err, dash.ErrUnsupportedOperator)
		assert.ErrorContains(t, err, "unsupported operator version")
	})
}

func TestDashboardFlagValidation(t *testing.T) {
	for name, args := range map[string][]string{
		"yaml output":        {"-o", "yaml"},
		"two names":          {"a", "b"},
		"zero stall-after":   {"--stall-after=0"},
		"invalid duration":   {"--stall-after=soon"},
		"unknown flag":       {"--watch"},
		"negative stall-aft": {"--stall-after=-1m"},
		"unknown sort":       {"--sort=ready"},
		"empty sort":         {"--sort="},
		"hide-empty value":   {"--hide-empty=maybe"},
		"hide-force value":   {"--hide-empty-force=maybe"},
		"negative max-unav":  {"--max-unavailable=-1"},
		"max-unav over 100%": {"--max-unavailable=101%"},
		"max-unav fraction":  {"--max-unavailable=1.5"},
		"max-unav word":      {"--max-unavailable=some"},
		"empty max-unav":     {"--max-unavailable="},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := runFixture(t, "steady", args...)
			require.Error(t, err)
		})
	}
	_, _, err := runFixture(t, "steady", "--sort=ready")
	assert.ErrorContains(t, err, `invalid --sort: unsupported profile sort "ready": use "name" or "desired"`)
	_, _, err = runFixture(t, "steady", "--max-unavailable=101%")
	assert.ErrorContains(t, err, `invalid --max-unavailable: "101%" is not a non-negative integer or a percentage from 0% to 100%`)
}

// --max-unavailable reaches Build in the static text and JSON modes; it
// defaults to 0.
func TestDashboardMaxUnavailable(t *testing.T) {
	snap, err := dash.LoadFixture(filepath.Join(fixtures, "availability"))
	require.NoError(t, err)
	for _, tc := range []struct {
		flag string
		max  rollout.MaxUnavailable
	}{
		{"", rollout.MaxUnavailable{}},
		{"--max-unavailable=0", rollout.MaxUnavailable{}},
		{"--max-unavailable=1", rollout.MaxUnavailable{Value: 1}},
		{"--max-unavailable=1%", rollout.MaxUnavailable{Value: 1, Percent: true}},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			want, err := dash.Build(snap, dash.BuildConfig{StallAfter: defaultStallAfter, MaxUnavailable: tc.max}, nil, testNow)
			require.NoError(t, err)
			var args []string
			if tc.flag != "" {
				args = append(args, tc.flag)
			}
			out, _, err := runFixture(t, "availability", args...)
			require.NoError(t, err)
			assert.Equal(t, render.Text(want, render.Theme{Width: render.DefaultWidth}, testNow), out)
		})
	}

	out, _, err := runFixture(t, "availability", "-o", "json", "--max-unavailable=1%")
	require.NoError(t, err)
	var v dash.View
	require.NoError(t, json.Unmarshal([]byte(out), &v))
	agent := v.DDA.Default.Agent
	require.NotNil(t, agent.Rollout.AvailabilityThreshold)
	assert.Zero(t, *agent.Rollout.AvailabilityThreshold, "1% of 2 rounds down to 0")
	assert.Equal(t, "Complete", agent.Rollout.Phase)
	assert.Equal(t, dash.BadgeDegraded, agent.Health)
	large := v.DDA.Profiles[0].DDAI.Agent
	require.NotNil(t, large.Rollout.AvailabilityThreshold)
	assert.Equal(t, int32(8), *large.Rollout.AvailabilityThreshold, "1% of 807 rounds down to 8")
	assert.Equal(t, dash.BadgeHealthy, large.Health)
}

// --hide-empty and --sort reach Build in the static text and JSON modes.
func TestDashboardProfileFlags(t *testing.T) {
	snap, err := dash.LoadFixture(filepath.Join(fixtures, "stale-daps"))
	require.NoError(t, err)
	cfg := dash.BuildConfig{StallAfter: defaultStallAfter, HideEmpty: true, Sort: dash.SortDesired}
	want, err := dash.Build(snap, cfg, nil, testNow)
	require.NoError(t, err)

	out, _, err := runFixture(t, "stale-daps", "--hide-empty", "--sort=desired")
	require.NoError(t, err)
	assert.Equal(t, render.Text(want, render.Theme{Width: render.DefaultWidth}, testNow), out)
	assert.Contains(t, out, "2 profiles hidden (0 desired)")

	out, _, err = runFixture(t, "stale-daps", "-o", "json", "--hide-empty", "--sort", "desired")
	require.NoError(t, err)
	var v dash.View
	require.NoError(t, json.Unmarshal([]byte(out), &v))
	assert.Equal(t, 2, v.DDA.HiddenProfiles)
	assert.Equal(t, []string{"datadog/alpha", "datadog/delta"}, v.DDA.HiddenProfileNames)
	require.NotEmpty(t, v.DDA.Profiles)
	assert.Equal(t, "beta", v.DDA.Profiles[0].Name)

	out, _, err = runFixture(t, "stale-daps", "--sort=name")
	require.NoError(t, err)
	assert.NotContains(t, out, "hidden")
}

// --hide-empty-force reaches Build in the static text and JSON modes.
func TestDashboardHideEmptyForce(t *testing.T) {
	snap, err := dash.LoadFixture(filepath.Join(fixtures, "stale-warnings"))
	require.NoError(t, err)
	want, err := dash.Build(snap, dash.BuildConfig{StallAfter: defaultStallAfter, HideEmptyForce: true}, nil, testNow)
	require.NoError(t, err)

	out, _, err := runFixture(t, "stale-warnings", "--hide-empty-force")
	require.NoError(t, err)
	assert.Equal(t, render.Text(want, render.Theme{Width: render.DefaultWidth}, testNow), out)
	assert.Contains(t, out, "7 hidden: 2 no warnings · 5 with warnings · --hide-empty-force")
	assert.NotContains(t, out, "DDAI batch")

	out, _, err = runFixture(t, "stale-warnings", "-o", "json", "--hide-empty-force")
	require.NoError(t, err)
	var v dash.View
	require.NoError(t, json.Unmarshal([]byte(out), &v))
	assert.Equal(t, dash.FilterHideEmptyForce, v.DDA.HiddenBy)
	assert.Equal(t, 7, v.DDA.HiddenProfiles)
	assert.Equal(t, 2, v.DDA.HiddenProfilesWithoutWarnings)
	assert.Equal(t, 5, v.DDA.HiddenProfilesWithWarnings)
	assert.Equal(t, 5, v.DDA.HiddenIssues)
	assert.Len(t, v.Issues, 2)

	out, _, err = runFixture(t, "stale-warnings", "--hide-empty")
	require.NoError(t, err)
	assert.Contains(t, out, "2 profiles hidden (0 desired) · --hide-empty")
}

func TestDashboardAlias(t *testing.T) {
	cmd := New(genericclioptions.NewTestIOStreamsDiscard())
	assert.Equal(t, []string{"dash"}, cmd.Aliases)
	for _, f := range []string{"namespace", "all-namespaces", "context", "output", "ascii", "no-color", "pods", "stall-after", "no-helm", "hide-empty", "hide-empty-force", "sort", "max-unavailable"} {
		assert.NotNil(t, cmd.Flags().Lookup(f), f)
	}
}

// recorder records the requests sent to the API server by every client.
type recorder struct {
	mu   sync.Mutex
	reqs []string
}

// wrap returns a RoundTripper recording the requests sent through next.
func (r *recorder) wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		r.mu.Lock()
		r.reqs = append(r.reqs, req.Method+" "+req.URL.RequestURI())
		r.mu.Unlock()
		return next.RoundTrip(req)
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// apiServer is a minimal API server: discovery, one Helm-managed DDA, and
// empty lists for every other collection.
func apiServer(t *testing.T) *httptest.Server {
	t.Helper()
	resources := map[string][]string{
		"/api/v1":                      {"pods", "secrets", "configmaps"},
		"/apis/apps/v1":                {"daemonsets", "deployments", "controllerrevisions", "replicasets"},
		"/apis/coordination.k8s.io/v1": {"leases"},
		"/apis/datadoghq.com/v2alpha1": {"datadogagents"},
		"/apis/datadoghq.com/v1alpha1": {"datadogagentinternals", "datadogagentprofiles", "datadogmonitors", "datadogdashboards", "datadogslos", "datadoggenericresources", "datadogmetrics", "datadoginstrumentations", "datadogcsidrivers", "datadogbyocclusters"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := req.URL.Path
		write := func(code int, body string) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}
		switch {
		case path == "/version":
			write(http.StatusOK, `{"major":"1","minor":"35","gitVersion":"v1.35.1"}`)
		case resources[path] != nil:
			var rs []string
			for _, r := range resources[path] {
				rs = append(rs, fmt.Sprintf(`{"name":%q,"namespaced":true,"kind":"X","verbs":["get","list","watch"]}`, r))
			}
			gv := strings.TrimPrefix(strings.TrimPrefix(path, "/apis/"), "/api/")
			write(http.StatusOK, fmt.Sprintf(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":%q,"resources":[%s]}`, gv, strings.Join(rs, ",")))
		case strings.HasSuffix(path, "/datadogagents"):
			write(http.StatusOK, `{"apiVersion":"datadoghq.com/v2alpha1","kind":"DatadogAgentList","metadata":{},"items":[
			  {"apiVersion":"datadoghq.com/v2alpha1","kind":"DatadogAgent","metadata":{"name":"datadog-agent","namespace":"datadog","uid":"u",
			   "annotations":{"meta.helm.sh/release-name":"datadog","meta.helm.sh/release-namespace":"datadog"}}}]}`)
		case strings.HasSuffix(path, "/secrets"):
			write(http.StatusOK, `{"apiVersion":"v1","kind":"SecretList","metadata":{},"items":[]}`)
		case strings.HasSuffix(path, "/configmaps"):
			write(http.StatusOK, `{"apiVersion":"v1","kind":"ConfigMapList","metadata":{},"items":[]}`)
		case strings.HasSuffix(path, "/pods"):
			write(http.StatusOK, `{"apiVersion":"v1","kind":"PodList","metadata":{},"items":[]}`)
		case strings.Contains(path, "/namespaces/") || strings.Count(path, "/") == 4:
			if strings.HasSuffix(path, "/leases/datadog-operator-lock") {
				write(http.StatusNotFound, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
				return
			}
			write(http.StatusOK, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`)
		default:
			write(http.StatusNotFound, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// runReadOnly runs the command against apiServer and returns every request
// it sent.
func runReadOnly(t *testing.T, env map[string]string, args ...string) []string {
	t.Helper()
	srv := apiServer(t)
	o, out, _ := newTestOptions(t, srv.URL)
	o.getenv = func(k string) string { return env[k] }
	rec := &recorder{}
	o.ConfigFlags.WrapConfigFn = func(c *rest.Config) *rest.Config {
		c.Wrap(rec.wrap)
		return c
	}
	cmd := newCmd(o)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())

	var v dash.View
	require.NoError(t, json.Unmarshal(out.Bytes(), &v))
	assert.Equal(t, "v1.35.1", v.Header.ServerVersion)
	assert.Equal(t, "kind-test", v.Header.Context)
	assert.Equal(t, "datadog-agent", v.DDA.Name)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	require.NotEmpty(t, rec.reqs)
	for _, r := range rec.reqs {
		assert.True(t, strings.HasPrefix(r, http.MethodGet+" "), "only GET/LIST requests are allowed: %s", r)
		assert.NotContains(t, r, "watch=", r)
	}
	return slices.Clone(rec.reqs)
}

func requested(reqs []string, part string) bool {
	return slices.ContainsFunc(reqs, func(r string) bool { return strings.Contains(r, part) })
}

// TestDashboardReadOnly checks that the command only sends GET requests,
// and only reads Secrets through the Helm history lookup.
func TestDashboardReadOnly(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		reqs := runReadOnly(t, nil, "-o", "json", "--pods")
		assert.True(t, requested(reqs, "/datadogagents"))
		assert.True(t, requested(reqs, "/pods?labelSelector=agent.datadoghq.com%2Fname%3Ddatadog-agent"))
		assert.True(t, requested(reqs, "/namespaces/datadog/secrets?labelSelector="), "Helm history is read")
	})
	t.Run("configmap Helm driver", func(t *testing.T) {
		reqs := runReadOnly(t, map[string]string{"HELM_DRIVER": "configmap"}, "-o", "json")
		assert.True(t, requested(reqs, "/namespaces/datadog/configmaps?labelSelector="), "Helm history is read from ConfigMaps")
		assert.False(t, requested(reqs, "/secrets"))
	})
	t.Run("unsupported Helm driver", func(t *testing.T) {
		reqs := runReadOnly(t, map[string]string{"HELM_DRIVER": "sql"}, "-o", "json")
		assert.False(t, requested(reqs, "/secrets"))
		assert.False(t, requested(reqs, "/configmaps"))
	})
	t.Run("no helm", func(t *testing.T) {
		reqs := runReadOnly(t, map[string]string{"HELM_DRIVER": "configmap"}, "-o", "json", "--no-helm")
		assert.False(t, requested(reqs, "/configmaps"), "--no-helm reads no Helm storage")
		assert.False(t, requested(reqs, "/secrets"), "--no-helm reads no Secret")
		assert.False(t, requested(reqs, "/pods"), "pods are only read with --pods")
	})
}
