// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/go-logr/logr"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/componenthealth"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/version"
)

// The following values describe the payload/transport contract with the
// agenthealth intake. The RFC fixes the proto shape (agent-payload/v5/
// healthplatform) but the exact envelope values and endpoint are still being
// confirmed with #fleet-remediation (CONTP-2137); they are centralized here so a
// later tweak is a one-line change.
const (
	// healthReportSchemaVersion is the schema version stamped on every report.
	healthReportSchemaVersion = "v1"
	// healthReportEventType marks these reports as full current-state snapshots
	// (as opposed to per-issue edges), which is what the backend reconciles.
	healthReportEventType = "snapshot"
	// healthReportService identifies the operator as the reporting flavor, the
	// operator-side analogue of the agent "Service = agent flavor" field.
	healthReportService = "datadog-operator"
	// healthReportSource identifies the operator as the issue source.
	healthReportSource = "datadog-operator"

	// agenthealth intake endpoint. The intake lives on a dedicated subdomain
	// (agenthealth-intake.<site>) distinct from the api.<site> host the metrics
	// forwarder uses.
	agenthealthScheme     = "https"
	agenthealthDefaultURL = "agenthealth-intake.datadoghq.com"
	agenthealthHostPrefix = "agenthealth-intake."
	agenthealthPath       = "api/v2/agenthealth"

	apiKeyHeaderKey      = "Dd-Api-Key"
	contentTypeHeaderKey = "Content-Type"
	userAgentHeaderKey   = "User-Agent"
	protobufContentType  = "application/x-protobuf"

	// snapshotHTTPTimeout bounds a single intake POST.
	snapshotHTTPTimeout = 10 * time.Second
)

// issueTitles maps each detected issue type to a human-readable title carried on
// the payload. Values mirror the signal catalog in the RFC.
var issueTitles = map[string]string{
	componenthealth.IssueOOMKilled:        "Component OOMKilled",
	componenthealth.IssueCrashLooping:     "Component Crash Looping",
	componenthealth.IssueUnschedulable:    "Component Unschedulable",
	componenthealth.IssueImagePullFailure: "Image Pull Failure",
}

// issueRemediations maps each issue type to its step-1 diagnostic remediation,
// from the RFC signal catalog. The backend may override or enrich these.
var issueRemediations = map[string]string{
	componenthealth.IssueOOMKilled:        "Run `kubectl describe pod` on the affected pods, then raise the memory limit or investigate a memory leak.",
	componenthealth.IssueCrashLooping:     "Run `kubectl logs --previous` on the affected pods to inspect the crash cause.",
	componenthealth.IssueUnschedulable:    "Run `kubectl describe pod` on the affected pods and check resource requests, taints, and affinity.",
	componenthealth.IssueImagePullFailure: "Verify the image tag, registry credentials, and cluster egress to the registry.",
}

// ddaGetter resolves the DatadogAgent used as the credential fallback source.
type ddaGetter func() (*v2alpha1.DatadogAgent, error)

// snapshotEmitter is the production componentHealthEmitter. On each snapshot it
// resolves credentials and site (operator config first, DatadogAgent CR as
// fallback), builds a HealthReport of the current active component-level issues,
// and POSTs it as protobuf to the agenthealth intake. It keeps no local state:
// the backend owns the ACTIVE/RESOLVED lifecycle, dedup, and first_seen/last_seen
// tracking, so the operator only ships raw current-state snapshots.
type snapshotEmitter struct {
	log          logr.Logger
	k8sClient    client.Reader
	credsManager *config.CredentialManager
	httpClient   *http.Client
	getDDA       ddaGetter
}

var _ componentHealthEmitter = &snapshotEmitter{}

// newSnapshotEmitter builds a snapshotEmitter that reports to the agenthealth
// intake using credentials resolved via the shared CredentialManager.
func newSnapshotEmitter(log logr.Logger, k8sClient client.Reader, credsManager *config.CredentialManager) *snapshotEmitter {
	e := &snapshotEmitter{
		log:          log,
		k8sClient:    k8sClient,
		credsManager: credsManager,
		httpClient:   &http.Client{Timeout: snapshotHTTPTimeout},
	}
	e.getDDA = e.getDatadogAgent
	return e
}

// Snapshot builds and POSTs a HealthReport of the current active issues. An empty
// snapshot is still sent: it is how the backend learns that previously-active
// issues have cleared, and it doubles as the liveness signal the backend uses to
// distinguish "healthy" from "stale/unknown".
func (e *snapshotEmitter) Snapshot(ctx context.Context, issues []componenthealth.ComponentIssue) {
	creds, err := e.credsManager.GetCredsWithDDAFallback(e.getDDA)
	if err != nil {
		e.log.V(1).Info("skipping component health snapshot: no credentials", "error", err)
		return
	}

	report := buildHealthReport(issues, os.Getenv(constants.DDClusterName))
	payload, err := proto.Marshal(report)
	if err != nil {
		e.log.Error(err, "failed to marshal component health report")
		return
	}

	req, err := e.newRequest(ctx, creds, payload)
	if err != nil {
		e.log.V(1).Info("failed to build component health request", "error", err)
		return
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.log.V(1).Info("failed to send component health snapshot", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		e.log.V(1).Info("component health intake returned non-success status",
			"status", resp.StatusCode, "body", string(body))
		return
	}

	e.log.V(2).Info("sent component health snapshot", "issues", len(issues), "status", resp.StatusCode)
}

// newRequest builds the POST request to the agenthealth intake for the given
// payload.
func (e *snapshotEmitter) newRequest(ctx context.Context, creds config.Creds, payload []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agenthealthURL(creds), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set(apiKeyHeaderKey, creds.APIKey)
	req.Header.Set(contentTypeHeaderKey, protobufContentType)
	req.Header.Set(userAgentHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return req, nil
}

// getDatadogAgent returns a DatadogAgent to source credentials from when the
// operator config does not carry them. It assumes a single DCA/CLC set per
// cluster (the RFC's current-state assumption); if several DatadogAgents exist it
// uses the first and warns.
func (e *snapshotEmitter) getDatadogAgent() (*v2alpha1.DatadogAgent, error) {
	ddaList := v2alpha1.DatadogAgentList{}
	if err := e.k8sClient.List(context.TODO(), &ddaList); err != nil {
		return nil, err
	}
	if len(ddaList.Items) == 0 {
		return nil, errors.New("no DatadogAgent found")
	}
	if len(ddaList.Items) > 1 {
		e.log.V(1).Info("multiple DatadogAgents found, using the first for credential fallback",
			"count", len(ddaList.Items), "using", ddaList.Items[0].Name)
	}
	return &ddaList.Items[0], nil
}

// agenthealthURL builds the agenthealth intake URL from the resolved credentials,
// mirroring the metadata forwarder: default to the US1 intake host, override the
// host from the configured site, and fully override host+scheme from an explicit
// DD_URL when set.
func agenthealthURL(creds config.Creds) string {
	u := url.URL{
		Scheme: agenthealthScheme,
		Host:   agenthealthDefaultURL,
		Path:   agenthealthPath,
	}
	if creds.Site != nil && strings.TrimSpace(*creds.Site) != "" {
		u.Host = agenthealthHostPrefix + strings.TrimSpace(*creds.Site)
	}
	if creds.URL != nil && strings.TrimSpace(*creds.URL) != "" {
		if parsed, err := url.Parse(strings.TrimSpace(*creds.URL)); err == nil && parsed.Host != "" {
			u.Host = parsed.Host
			u.Scheme = parsed.Scheme
		}
	}
	return u.String()
}

// buildHealthReport turns the current active component-level issues into a
// HealthReport envelope keyed by stable issue id.
func buildHealthReport(issues []componenthealth.ComponentIssue, clusterName string) *healthplatform.HealthReport {
	now := time.Now().UTC().Format(time.RFC3339)

	report := &healthplatform.HealthReport{
		SchemaVersion: healthReportSchemaVersion,
		EventType:     healthReportEventType,
		EmittedAt:     now,
		Service:       healthReportService,
		Host:          &healthplatform.HostInfo{Hostname: clusterName},
		Issues:        make(map[string]*healthplatform.Issue, len(issues)),
	}

	for _, issue := range issues {
		id := issueID(issue)
		report.Issues[id] = buildIssue(id, issue, now)
	}

	return report
}

// buildIssue maps one component-level issue onto the health-platform Issue proto.
func buildIssue(id string, issue componenthealth.ComponentIssue, detectedAt string) *healthplatform.Issue {
	out := &healthplatform.Issue{
		Id:          id,
		IssueName:   issue.IssueType,
		IssueType:   issue.IssueType,
		Title:       issueTitle(issue.IssueType),
		Description: issueDescription(issue),
		Severity:    toProtoSeverity(issue.Severity),
		Source:      healthReportSource,
		DetectedAt:  detectedAt,
		Tags: []string{
			"kube_namespace:" + issue.Namespace,
			"component:" + issue.Component,
			"issue_type:" + issue.IssueType,
		},
	}
	if summary, ok := issueRemediations[issue.IssueType]; ok {
		out.Remediation = &healthplatform.Remediation{Summary: summary}
	}
	return out
}

// issueID is the stable identity of a component-level issue across pod churn:
// (namespace, component, issue_type). The backend keys its lifecycle state on it.
func issueID(issue componenthealth.ComponentIssue) string {
	return fmt.Sprintf("%s/%s/%s", issue.Namespace, issue.Component, issue.IssueType)
}

func issueTitle(issueType string) string {
	if t, ok := issueTitles[issueType]; ok {
		return t
	}
	return issueType
}

func issueDescription(issue componenthealth.ComponentIssue) string {
	if len(issue.AffectedPods) == 0 {
		return fmt.Sprintf("component %q in namespace %q is affected", issue.Component, issue.Namespace)
	}
	return fmt.Sprintf("component %q in namespace %q has %d affected pod(s): %s",
		issue.Component, issue.Namespace, len(issue.AffectedPods), strings.Join(issue.AffectedPods, ", "))
}

func toProtoSeverity(s componenthealth.Severity) healthplatform.IssueSeverity {
	switch s {
	case componenthealth.SeverityHigh:
		return healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	case componenthealth.SeverityMedium:
		return healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM
	case componenthealth.SeverityLow:
		return healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW
	default:
		return healthplatform.IssueSeverity_ISSUE_SEVERITY_UNSPECIFIED
	}
}
