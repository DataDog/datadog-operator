// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/componenthealth"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func TestAgenthealthURL(t *testing.T) {
	site := "datadoghq.eu"
	customURL := "https://custom.example.com:8443"
	emptySite := ""

	tests := []struct {
		name  string
		creds config.Creds
		want  string
	}{
		{
			name:  "default host when no site or url",
			creds: config.Creds{},
			want:  "https://agenthealth-intake.datadoghq.com/api/v2/agenthealth",
		},
		{
			name:  "site overrides host",
			creds: config.Creds{Site: &site},
			want:  "https://agenthealth-intake.datadoghq.eu/api/v2/agenthealth",
		},
		{
			name:  "empty site falls back to default host",
			creds: config.Creds{Site: &emptySite},
			want:  "https://agenthealth-intake.datadoghq.com/api/v2/agenthealth",
		},
		{
			name:  "url overrides host and scheme",
			creds: config.Creds{URL: &customURL},
			want:  "https://custom.example.com:8443/api/v2/agenthealth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, agenthealthURL(tt.creds))
		})
	}
}

func TestBuildHealthReport(t *testing.T) {
	seen := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	issues := []componenthealth.ComponentIssue{
		{
			Component:    constants.DefaultClusterAgentResourceSuffix,
			IssueType:    componenthealth.IssueOOMKilled,
			Severity:     componenthealth.SeverityHigh,
			Namespace:    "datadog",
			AffectedPods: []string{"dca-1", "dca-2"},
			FirstSeen:    seen,
			LastSeen:     seen.Add(time.Minute),
		},
		{
			Component:    constants.DefaultClusterChecksRunnerResourceSuffix,
			IssueType:    componenthealth.IssueImagePullFailure,
			Severity:     componenthealth.SeverityMedium,
			Namespace:    "datadog",
			AffectedPods: []string{"clc-1"},
			FirstSeen:    seen,
			LastSeen:     seen,
		},
	}

	resolvedIssues := []componenthealth.ComponentIssue{
		{
			Component:  constants.DefaultClusterAgentResourceSuffix,
			IssueType:  componenthealth.IssueCrashLooping,
			Severity:   componenthealth.SeverityHigh,
			Namespace:  "datadog",
			FirstSeen:  seen,
			LastSeen:   seen.Add(time.Minute),
			ResolvedAt: seen.Add(2 * time.Minute),
		},
	}

	report := buildHealthReport(issues, resolvedIssues, "my-cluster")

	assert.Equal(t, healthReportSchemaVersion, report.SchemaVersion)
	assert.Equal(t, healthReportEventType, report.EventType)
	assert.Equal(t, healthReportService, report.Service)
	assert.NotEmpty(t, report.EmittedAt)
	require.NotNil(t, report.Host)
	assert.Equal(t, "my-cluster", report.Host.Hostname)
	require.Len(t, report.Issues, 3)

	oomID := "datadog/" + constants.DefaultClusterAgentResourceSuffix + "/" + componenthealth.IssueOOMKilled
	oom := report.Issues[oomID]
	require.NotNil(t, oom, "issue keyed by (namespace, component, issue_type)")
	assert.Equal(t, oomID, oom.Id)
	assert.Equal(t, componenthealth.IssueOOMKilled, oom.IssueType)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, oom.Severity)
	assert.Equal(t, healthReportSource, oom.Source)
	assert.Contains(t, oom.Description, "dca-1")
	assert.Contains(t, oom.Description, "dca-2")
	assert.ElementsMatch(t, []string{
		"kube_namespace:datadog",
		"component:" + constants.DefaultClusterAgentResourceSuffix,
		"issue_type:" + componenthealth.IssueOOMKilled,
	}, oom.Tags)
	require.NotNil(t, oom.Remediation)
	assert.NotEmpty(t, oom.Remediation.Summary)
	require.NotNil(t, oom.PersistedIssue, "active issues carry lifecycle state")
	assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_ACTIVE, oom.PersistedIssue.State)
	assert.NotEmpty(t, oom.PersistedIssue.FirstSeen)
	assert.NotEmpty(t, oom.PersistedIssue.LastSeen)
	assert.Nil(t, oom.PersistedIssue.ResolvedAt, "active issues have no resolved_at")

	clcID := "datadog/" + constants.DefaultClusterChecksRunnerResourceSuffix + "/" + componenthealth.IssueImagePullFailure
	clc := report.Issues[clcID]
	require.NotNil(t, clc)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, clc.Severity)

	resolvedID := "datadog/" + constants.DefaultClusterAgentResourceSuffix + "/" + componenthealth.IssueCrashLooping
	resolved := report.Issues[resolvedID]
	require.NotNil(t, resolved, "resolved issue is included in the report")
	require.NotNil(t, resolved.PersistedIssue, "resolved issue carries an explicit lifecycle state")
	assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_RESOLVED, resolved.PersistedIssue.State)
	assert.NotNil(t, resolved.PersistedIssue.ResolvedAt)
}

func TestSnapshotEmitter_ClusterName(t *testing.T) {
	ddaClusterName := "dda-cluster"
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "system"},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{ClusterName: &ddaClusterName},
		},
	}
	scheme := runtime.NewScheme()
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dda).Build()
	emitter := newSnapshotEmitter(logr.Discard(), c, config.NewCredentialManager(c))

	// No DD_CLUSTER_NAME in the operator env -> fall back to the DatadogAgent's
	// spec.global.clusterName.
	t.Setenv(constants.DDClusterName, "")
	assert.Equal(t, ddaClusterName, emitter.clusterName())

	// Operator env takes precedence when set.
	t.Setenv(constants.DDClusterName, "env-cluster")
	assert.Equal(t, "env-cluster", emitter.clusterName())
}

func TestToProtoSeverity(t *testing.T) {
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, toProtoSeverity(componenthealth.SeverityHigh))
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, toProtoSeverity(componenthealth.SeverityMedium))
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW, toProtoSeverity(componenthealth.SeverityLow))
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_UNSPECIFIED, toProtoSeverity(componenthealth.Severity("bogus")))
}

// TestSnapshotEmitter_PostsJSON verifies the emitter resolves credentials, builds
// a JSON HealthReport, and POSTs it to the agenthealth intake with the expected
// headers.
func TestSnapshotEmitter_PostsJSON(t *testing.T) {
	var (
		gotAPIKey      string
		gotContentType string
		gotMethod      string
		gotPath        string
		gotBody        []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get(apiKeyHeaderKey)
		gotContentType = r.Header.Get(contentTypeHeaderKey)
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	t.Setenv(constants.DDAPIKey, "test-api-key")
	t.Setenv(constants.DDURL, srv.URL)
	t.Setenv(constants.DDClusterName, "test-cluster")

	fakeClient := fake.NewClientBuilder().Build()
	emitter := newSnapshotEmitter(logr.Discard(), fakeClient, config.NewCredentialManager(fakeClient))
	emitter.httpClient = srv.Client()

	active := []componenthealth.ComponentIssue{{
		Component:    constants.DefaultClusterAgentResourceSuffix,
		IssueType:    componenthealth.IssueOOMKilled,
		Severity:     componenthealth.SeverityHigh,
		Namespace:    "datadog",
		AffectedPods: []string{"dca-1"},
	}}
	resolved := []componenthealth.ComponentIssue{{
		Component: constants.DefaultClusterChecksRunnerResourceSuffix,
		IssueType: componenthealth.IssueCrashLooping,
		Severity:  componenthealth.SeverityHigh,
		Namespace: "datadog",
	}}

	require.NoError(t, emitter.Snapshot(context.Background(), active, resolved))

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/"+agenthealthPath, gotPath)
	assert.Equal(t, "test-api-key", gotAPIKey)
	assert.Equal(t, jsonContentType, gotContentType)

	var report healthplatform.HealthReport
	require.NoError(t, json.Unmarshal(gotBody, &report))
	assert.Equal(t, "test-cluster", report.Host.GetHostname())
	require.Len(t, report.Issues, 2)

	activeID := "datadog/" + constants.DefaultClusterAgentResourceSuffix + "/" + componenthealth.IssueOOMKilled
	require.NotNil(t, report.Issues[activeID])
	require.NotNil(t, report.Issues[activeID].PersistedIssue)
	assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_ACTIVE, report.Issues[activeID].PersistedIssue.State)

	resolvedID := "datadog/" + constants.DefaultClusterChecksRunnerResourceSuffix + "/" + componenthealth.IssueCrashLooping
	require.NotNil(t, report.Issues[resolvedID])
	require.NotNil(t, report.Issues[resolvedID].PersistedIssue)
	assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_RESOLVED, report.Issues[resolvedID].PersistedIssue.State)
}

// TestSnapshotEmitter_SkipsWithoutCredentials verifies the emitter does not POST
// when no credentials can be resolved.
func TestSnapshotEmitter_SkipsWithoutCredentials(t *testing.T) {
	t.Setenv(constants.DDAPIKey, "")
	t.Setenv(constants.DDURL, "")

	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	fakeClient := fake.NewClientBuilder().Build()
	emitter := newSnapshotEmitter(logr.Discard(), fakeClient, config.NewCredentialManager(fakeClient))
	emitter.httpClient = srv.Client()

	err := emitter.Snapshot(context.Background(), nil, nil)
	assert.Error(t, err, "must return an error when credentials are missing")
	assert.False(t, posted, "must not POST when credentials are missing")
}
