// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"bytes"
	"context"
	"encoding/json"
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
	// healthReportEventType categorizes the report envelope for the intake. It
	// reuses the value the Datadog Agent's health platform sends
	// (comp/healthplatform), which is the proven-accepted contract on
	// /api/v2/agenthealth.
	//
	// TODO(CONTP-2137): confirm with #fleet-remediation that the operator should
	// reuse "agent-health-issues" rather than an operator-specific event type.
	healthReportEventType = "agent-health-issues"
	// healthReportService identifies the operator as the reporting flavor
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
	jsonContentType      = "application/json"

	// snapshotHTTPTimeout bounds a single intake POST.
	snapshotHTTPTimeout = 10 * time.Second
)

// issueTitles maps each detected issue type to a human-readable title carried on
// the payload.
var issueTitles = map[string]string{
	componenthealth.IssueOOMKilled:        "Component OOMKilled",
	componenthealth.IssueCrashLooping:     "Component Crash Looping",
	componenthealth.IssueUnschedulable:    "Component Unschedulable",
	componenthealth.IssueImagePullFailure: "Image Pull Failure",
}

// issueRemediations maps each issue type to its step-1 diagnostic remediation.
// The backend may override or enrich these.
//
// TODO: use backend-based remediation once available instead of hardcoding
// remediations in the operator code.
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
// fallback), builds a HealthReport of the current active component-level issues
// plus explicit RESOLVED entries for the issues that just cleared, and POSTs it as
// JSON to the agenthealth intake (matching the Datadog Agent's health platform,
// which sends encoding/json on the same endpoint).
//
// The emitter itself is stateless; the active-issue set and the pending-resolve
// set it serializes live in the ComponentHealthReconciler. The backend remains the
// authoritative owner of issue lifecycle — dedup and first_seen/last_seen
// tracking, and auto-resolution via TTL as a backstop — but the operator sends
// explicit resolve signals so a cleared issue is closed promptly instead of
// waiting for that TTL to expire.
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

// Snapshot builds and POSTs a HealthReport containing the current active issues
// (as current-state) and the recently-resolved issues (carrying an explicit
// RESOLVED state). It returns an error when the report was not delivered, so the
// controller can retry the resolve signals on the next tick. An otherwise-empty
// report is still sent: it doubles as the liveness signal the backend uses to
// distinguish "healthy" from "stale/unknown".
func (e *snapshotEmitter) Snapshot(ctx context.Context, active, resolved []componenthealth.ComponentIssue) error {
	creds, err := e.credsManager.GetCredsWithDDAFallback(e.getDDA)
	if err != nil {
		e.log.V(1).Info("skipping component health snapshot: no credentials", "error", err)
		return err
	}

	report := buildHealthReport(active, resolved, os.Getenv(constants.DDClusterName))
	payload, err := json.Marshal(report)
	if err != nil {
		e.log.Error(err, "failed to marshal component health report")
		return err
	}

	req, err := e.newRequest(ctx, creds, payload)
	if err != nil {
		e.log.V(1).Info("failed to build component health request", "error", err)
		return err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.log.V(1).Info("failed to send component health snapshot", "error", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		e.log.V(1).Info("component health intake returned non-success status",
			"status", resp.StatusCode, "body", string(body))
		return fmt.Errorf("agenthealth intake returned status %d", resp.StatusCode)
	}

	e.log.V(2).Info("sent component health snapshot",
		"active", len(active), "resolved", len(resolved), "status", resp.StatusCode)
	return nil
}

// newRequest builds the POST request to the agenthealth intake for the given
// payload.
func (e *snapshotEmitter) newRequest(ctx context.Context, creds config.Creds, payload []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agenthealthURL(creds), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set(apiKeyHeaderKey, creds.APIKey)
	req.Header.Set(contentTypeHeaderKey, jsonContentType)
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

// buildHealthReport turns the current active and recently-resolved component-level
// issues into a HealthReport envelope keyed by stable issue id. Active issues
// carry no explicit lifecycle state (the backend treats their presence as
// active/ongoing); resolved issues carry an explicit RESOLVED state so the backend
// closes them immediately instead of waiting for TTL expiry.
func buildHealthReport(active, resolved []componenthealth.ComponentIssue, clusterName string) *healthplatform.HealthReport {
	now := time.Now().UTC().Format(time.RFC3339)

	report := &healthplatform.HealthReport{
		SchemaVersion: healthReportSchemaVersion,
		EventType:     healthReportEventType,
		EmittedAt:     now,
		Service:       healthReportService,
		Host:          &healthplatform.HostInfo{Hostname: clusterName},
		Issues:        make(map[string]*healthplatform.Issue, len(active)+len(resolved)),
	}

	for _, issue := range active {
		id := issueID(issue)
		report.Issues[id] = buildIssue(id, issue, now, false)
	}
	for _, issue := range resolved {
		id := issueID(issue)
		report.Issues[id] = buildIssue(id, issue, now, true)
	}

	return report
}

// buildIssue maps one component-level issue onto the health-platform Issue proto.
// When resolved is true it stamps an explicit RESOLVED lifecycle state so the
// backend closes the issue.
func buildIssue(id string, issue componenthealth.ComponentIssue, now string, resolved bool) *healthplatform.Issue {
	out := &healthplatform.Issue{
		Id:          id,
		IssueName:   issue.IssueType,
		IssueType:   issue.IssueType,
		Title:       issueTitle(issue.IssueType),
		Description: issueDescription(issue),
		Severity:    toProtoSeverity(issue.Severity),
		Source:      healthReportSource,
		DetectedAt:  now,
		Tags: []string{
			"kube_namespace:" + issue.Namespace,
			"component:" + issue.Component,
			"issue_type:" + issue.IssueType,
		},
	}
	if summary, ok := issueRemediations[issue.IssueType]; ok {
		out.Remediation = &healthplatform.Remediation{Summary: summary}
	}
	if resolved {
		out.PersistedIssue = &healthplatform.PersistedIssue{
			State:      healthplatform.IssueState_ISSUE_STATE_RESOLVED,
			ResolvedAt: &now,
		}
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
