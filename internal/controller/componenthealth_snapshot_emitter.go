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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/componenthealth"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/version"
)

// The following values describe the payload/transport contract with the
// agenthealth intake. The proto shape (agent-payload/v5/healthplatform) is fixed,
// but the exact envelope values and endpoint are still being confirmed with
// #fleet-remediation (CONTP-2137); they are centralized here so a later tweak is a
// one-line change.
const (
	// healthReportSchemaVersion is the schema version stamped on every report.
	healthReportSchemaVersion = "v1"
	// healthReportEventType categorizes the report envelope for the intake.
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

	// kubeSystemNamespace holds the UID used as the cluster identity
	kubeSystemNamespace = "kube-system"

	// clusterNameTagKey tags each issue with the human-readable cluster name when
	// one is configured (optional; the cluster ID is the reliable identifier).
	clusterNameTagKey = "kube_cluster_name"
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
// JSON to the agenthealth intake.
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

	// cachedClusterID memoizes the kube-system UID (immutable for a cluster). Only
	// accessed from the single snapshot-loop goroutine.
	cachedClusterID string

	// intakeURL, when set, overrides the computed agenthealth URL. Used by tests to
	// target a local server; empty in production (the URL is derived from the site).
	intakeURL string
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

	report := buildHealthReport(active, resolved, e.clusterID(ctx), e.clusterName())
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

// clusterID resolves the cluster's stable identity: the UID of the kube-system
// namespace.
//
// It is memoized since it is immutable for the lifetime of a cluster. Returns ""
// if it cannot be read (the report is still sent; the backend degrades to its
// other signals).
func (e *snapshotEmitter) clusterID(ctx context.Context) string {
	if e.cachedClusterID != "" {
		return e.cachedClusterID
	}
	ns := &corev1.Namespace{}
	if err := e.k8sClient.Get(ctx, types.NamespacedName{Name: kubeSystemNamespace}, ns); err != nil {
		e.log.V(1).Info("could not resolve cluster ID (kube-system UID)", "error", err)
		return ""
	}
	e.cachedClusterID = string(ns.UID)
	return e.cachedClusterID
}

// clusterName resolves the optional, human-readable cluster name reported as a
// tag: the operator config (DD_CLUSTER_NAME) takes precedence, falling back to the
// DatadogAgent's spec.global.clusterName when the operator environment does not
// set it (the same CR used for the credential fallback). Returns "" if neither is
// available. Unlike the cluster ID, the name is optional and not the identity key.
func (e *snapshotEmitter) clusterName() string {
	if name := os.Getenv(constants.DDClusterName); name != "" {
		return name
	}
	if e.getDDA != nil {
		if dda, err := e.getDDA(); err == nil && dda != nil && dda.Spec.Global != nil && dda.Spec.Global.ClusterName != nil {
			return *dda.Spec.Global.ClusterName
		}
	}
	return ""
}

// newRequest builds the POST request to the agenthealth intake for the given
// payload.
func (e *snapshotEmitter) newRequest(ctx context.Context, creds config.Creds, payload []byte) (*http.Request, error) {
	target := e.intakeURL
	if target == "" {
		target = agenthealthURL(creds)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
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
// cluster; if several DatadogAgents exist it uses the first and warns.
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

// agenthealthURL builds the agenthealth intake URL from the configured site:
// agenthealth-intake.<site> (defaulting to the US1 site).
func agenthealthURL(creds config.Creds) string {
	u := url.URL{
		Scheme: agenthealthScheme,
		Host:   agenthealthDefaultURL,
		Path:   agenthealthPath,
	}
	if creds.Site != nil && strings.TrimSpace(*creds.Site) != "" {
		u.Host = agenthealthHostPrefix + strings.TrimSpace(*creds.Site)
	}
	return u.String()
}

// buildHealthReport turns the current active and recently-resolved component-level
// issues into a HealthReport envelope keyed by stable issue id. clusterID (the
// kube-system UID) is the host identity; clusterName, when set, is attached to each
// issue as an optional kube_cluster_name tag. Every issue carries a PersistedIssue
// with its lifecycle state (ACTIVE or RESOLVED) and first_seen / last_seen
// timestamps; resolved issues additionally carry resolved_at so the backend closes
// them immediately instead of waiting for TTL expiry.
func buildHealthReport(active, resolved []componenthealth.ComponentIssue, clusterID, clusterName string) *healthplatform.HealthReport {
	now := time.Now().UTC().Format(time.RFC3339)

	report := &healthplatform.HealthReport{
		SchemaVersion: healthReportSchemaVersion,
		EventType:     healthReportEventType,
		EmittedAt:     now,
		Service:       healthReportService,
		Host:          &healthplatform.HostInfo{Hostname: clusterID},
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

	if clusterName != "" {
		tag := clusterNameTagKey + ":" + clusterName
		for _, issue := range report.GetIssues() {
			issue.Tags = append(issue.Tags, tag)
		}
	}

	return report
}

// buildIssue maps one component-level issue onto the health-platform Issue proto.
// It attaches a PersistedIssue carrying the lifecycle state and timestamps: ACTIVE
// with first_seen/last_seen for active issues, RESOLVED with resolved_at as well
// for resolved ones.
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

	persisted := &healthplatform.PersistedIssue{
		State:     healthplatform.IssueState_ISSUE_STATE_ACTIVE,
		FirstSeen: formatTime(issue.FirstSeen),
		LastSeen:  formatTime(issue.LastSeen),
	}
	if resolved {
		persisted.State = healthplatform.IssueState_ISSUE_STATE_RESOLVED
		resolvedAt := formatTime(issue.ResolvedAt)
		persisted.ResolvedAt = &resolvedAt
	}
	out.PersistedIssue = persisted
	return out
}

// formatTime renders a timestamp in the RFC3339/UTC form the intake expects, or ""
// for a zero time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
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
