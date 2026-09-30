// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
	issues := []componenthealth.ComponentIssue{
		{
			Component:    constants.DefaultClusterAgentResourceSuffix,
			IssueType:    componenthealth.IssueOOMKilled,
			Severity:     componenthealth.SeverityHigh,
			Namespace:    "datadog",
			AffectedPods: []string{"dca-1", "dca-2"},
		},
		{
			Component:    constants.DefaultClusterChecksRunnerResourceSuffix,
			IssueType:    componenthealth.IssueImagePullFailure,
			Severity:     componenthealth.SeverityMedium,
			Namespace:    "datadog",
			AffectedPods: []string{"clc-1"},
		},
	}

	report := buildHealthReport(issues, "my-cluster")

	assert.Equal(t, healthReportSchemaVersion, report.SchemaVersion)
	assert.Equal(t, healthReportEventType, report.EventType)
	assert.Equal(t, healthReportService, report.Service)
	assert.NotEmpty(t, report.EmittedAt)
	require.NotNil(t, report.Host)
	assert.Equal(t, "my-cluster", report.Host.Hostname)
	require.Len(t, report.Issues, 2)

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

	clcID := "datadog/" + constants.DefaultClusterChecksRunnerResourceSuffix + "/" + componenthealth.IssueImagePullFailure
	clc := report.Issues[clcID]
	require.NotNil(t, clc)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, clc.Severity)
}

func TestToProtoSeverity(t *testing.T) {
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, toProtoSeverity(componenthealth.SeverityHigh))
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, toProtoSeverity(componenthealth.SeverityMedium))
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW, toProtoSeverity(componenthealth.SeverityLow))
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_UNSPECIFIED, toProtoSeverity(componenthealth.Severity("bogus")))
}

// TestSnapshotEmitter_PostsProtobuf verifies the emitter resolves credentials,
// builds a protobuf HealthReport, and POSTs it to the agenthealth intake with the
// expected headers.
func TestSnapshotEmitter_PostsProtobuf(t *testing.T) {
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

	issues := []componenthealth.ComponentIssue{{
		Component:    constants.DefaultClusterAgentResourceSuffix,
		IssueType:    componenthealth.IssueOOMKilled,
		Severity:     componenthealth.SeverityHigh,
		Namespace:    "datadog",
		AffectedPods: []string{"dca-1"},
	}}

	emitter.Snapshot(context.Background(), issues)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/"+agenthealthPath, gotPath)
	assert.Equal(t, "test-api-key", gotAPIKey)
	assert.Equal(t, protobufContentType, gotContentType)

	var report healthplatform.HealthReport
	require.NoError(t, proto.Unmarshal(gotBody, &report))
	assert.Equal(t, "test-cluster", report.Host.GetHostname())
	require.Len(t, report.Issues, 1)
	for _, issue := range report.Issues {
		assert.Equal(t, componenthealth.IssueOOMKilled, issue.IssueType)
	}
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

	emitter.Snapshot(context.Background(), nil)
	assert.False(t, posted, "must not POST when credentials are missing")
}
