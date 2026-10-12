// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import "time"

// SchemaVersion identifies the View JSON schema. Field changes must be
// additive; renames require a new version.
const SchemaVersion = "dashboard.datadoghq.com/v1"

// View is the view model consumed by every renderer and emitted as JSON.
// Times are absolute; renderers compute ages from their own clock.
type View struct {
	SchemaVersion string    `json:"schemaVersion"`
	GeneratedAt   time.Time `json:"generatedAt"`
	Header        Header    `json:"header"`
	// MultipleDDAs lists the DatadogAgents in scope ("<namespace>/<name>")
	// when there is more than one and none was named. DDA is then empty.
	MultipleDDAs []string `json:"multipleDDAs,omitempty"`
	DDA          DDAView  `json:"dda"`
	// Unattached holds the DDAIs and workloads whose parent can't be
	// resolved.
	Unattached *UnattachedView `json:"unattached,omitempty"`
	// Summary has one row per installed summary kind.
	Summary []SummaryView `json:"summary"`
	// Issues lists every Error and Warning condition once, most severe
	// first.
	Issues []Cond `json:"issues"`
	// Sources is the acquisition status of each source, sorted by key.
	Sources []SourceView `json:"sources"`
	// Notices are user-facing notes about partial data.
	Notices []string `json:"notices,omitempty"`
}

// SelectionError returns a *MultipleDDAsError when the View has no DDA
// because several are in scope, nil otherwise.
func (v View) SelectionError() error {
	if len(v.MultipleDDAs) == 0 {
		return nil
	}
	return &MultipleDDAsError{Names: v.MultipleDDAs}
}

// Header is the top of the dashboard.
type Header struct {
	Context       string        `json:"context,omitempty"`
	Namespace     string        `json:"namespace"`
	ServerVersion string        `json:"serverVersion,omitempty"`
	PluginVersion string        `json:"pluginVersion,omitempty"`
	RefreshedAt   time.Time     `json:"refreshedAt"`
	Provider      ProviderView  `json:"provider"`
	Operator      *OperatorView `json:"operator,omitempty"`
	// NextPoll is the earliest next refresh of a polled source.
	NextPoll *time.Time `json:"nextPoll,omitempty"`
}

// Provider sources.
const (
	ProviderUserSet  = "user-set"
	ProviderDetected = "detected"
)

// ProviderView is a cluster provider and where it came from.
type ProviderView struct {
	// Name is empty when no provider applies.
	Name string `json:"name,omitempty"`
	// Source is ProviderUserSet or ProviderDetected; empty when unknown
	// (e.g. detection disabled).
	Source string `json:"source,omitempty"`
	// Reason is the ClusterProviderDetected condition reason, if any.
	Reason string `json:"reason,omitempty"`
}

// OperatorView is the operator Deployment and leader Lease.
type OperatorView struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	Image     string `json:"image,omitempty"`
	Ready     int32  `json:"ready"`
	Desired   int32  `json:"desired"`
	// LeaseHolder is the identity of the current leader, if known.
	LeaseHolder string     `json:"leaseHolder,omitempty"`
	LeaseRenew  *time.Time `json:"leaseRenew,omitempty"`
}

// Badge is a health badge.
type Badge string

const (
	BadgeHealthy     Badge = "Healthy"
	BadgeProgressing Badge = "Progressing"
	BadgeDegraded    Badge = "Degraded"
	BadgeError       Badge = "Error"
	BadgeUnknown     Badge = "Unknown"
)

// Severity is a condition severity.
type Severity string

const (
	SeverityError   Severity = "Error"
	SeverityWarning Severity = "Warning"
	SeverityInfo    Severity = "Info"
)

// rank orders severities, the most severe highest.
func (s Severity) rank() int {
	switch s {
	case SeverityError:
		return 2
	case SeverityWarning:
		return 1
	default:
		return 0
	}
}

// ObjectRef identifies an object of the tree.
type ObjectRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

func (r ObjectRef) String() string {
	if r.Namespace == "" {
		return r.Kind + "/" + r.Name
	}
	return r.Kind + "/" + r.Namespace + "/" + r.Name
}

// Cond is a deduplicated condition attached to the lowest object reporting
// it.
type Cond struct {
	Severity Severity  `json:"severity"`
	Object   ObjectRef `json:"object"`
	Type     string    `json:"type"`
	Status   string    `json:"status"`
	Reason   string    `json:"reason,omitempty"`
	// Message is never truncated in the View.
	Message            string     `json:"message,omitempty"`
	LastTransitionTime *time.Time `json:"lastTransitionTime,omitempty"`
	// AlsoReportedBy lists the objects whose copy of this condition was
	// collapsed into this one.
	AlsoReportedBy []ObjectRef `json:"alsoReportedBy,omitempty"`
}

// Rollout is an evaluated rollout.
type Rollout struct {
	Phase   string `json:"phase"`
	Reason  string `json:"reason,omitempty"`
	Updated int32  `json:"updated"`
	Desired int32  `json:"desired"`
	// UpdatedReady is how many updated pods are ready (see
	// rollout.UpdatedReady).
	UpdatedReady int32 `json:"updatedReady"`
	// Percent is UpdatedReady/Desired, 100 when Desired is 0.
	Percent int `json:"percent"`
	// PercentBasis names the numerator of Percent:
	// PercentBasisUpdatedReady for every evaluated rollout.
	PercentBasis string `json:"percentBasis,omitempty"`
	// Since is the rollout start.
	Since        *time.Time `json:"since,omitempty"`
	SinceApprox  bool       `json:"sinceApprox,omitempty"`
	LastProgress *time.Time `json:"lastProgress,omitempty"`
	Deadline     *time.Time `json:"deadline,omitempty"`
	// SettlingUntil is when a Settling rollout ends its settling period.
	SettlingUntil *time.Time `json:"settlingUntil,omitempty"`
	// AvailabilityThreshold is how many unavailable pods the workload
	// tolerates once converged (--max-unavailable resolved against
	// Desired); only set on workloads.
	AvailabilityThreshold *int32 `json:"availabilityThreshold,omitempty"`
	StepOrder             *int   `json:"stepOrder,omitempty"`
	// Source is the evaluator that decided.
	Source string `json:"source,omitempty"`
}

// PercentBasisUpdatedReady means Rollout.Percent counts the updated pods
// that are ready.
const PercentBasisUpdatedReady = "updatedReady"

// Counts are workload replica counts.
type Counts struct {
	Desired     int32 `json:"desired"`
	Ready       int32 `json:"ready"`
	UpToDate    int32 `json:"upToDate"`
	Available   int32 `json:"available"`
	Unavailable int32 `json:"unavailable"`
}

// Workload components.
const (
	ComponentAgent               = "agent"
	ComponentClusterAgent        = "cluster-agent"
	ComponentClusterChecksRunner = "cluster-checks-runner"
	ComponentOtelAgentGateway    = "otel-agent-gateway"
)

// WorkloadView is a DaemonSet or Deployment.
type WorkloadView struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Component string `json:"component,omitempty"`
	// Missing is true when the workload is named in the DDAI status but
	// was not found.
	Missing  bool    `json:"missing,omitempty"`
	Health   Badge   `json:"health"`
	Counts   Counts  `json:"counts"`
	Strategy string  `json:"strategy,omitempty"`
	Rollout  Rollout `json:"rollout"`
	// FailingPods is only set when pods are watched.
	FailingPods []PodView `json:"failingPods,omitempty"`
	Conditions  []Cond    `json:"conditions,omitempty"`
}

// PodView is a failing agent pod.
type PodView struct {
	Name   string `json:"name"`
	Node   string `json:"node,omitempty"`
	Reason string `json:"reason"`
}

// DDAIView is a DatadogAgentInternal and its workloads.
type DDAIView struct {
	Name string `json:"name"`
	// Profile is the DAP name for profile DDAIs; empty for the default.
	Profile    string        `json:"profile,omitempty"`
	Health     Badge         `json:"health"`
	Rollout    Rollout       `json:"rollout"`
	Conditions []Cond        `json:"conditions,omitempty"`
	Agent      *WorkloadView `json:"agent,omitempty"`
	// Deployments are nil when the component is not enabled.
	ClusterAgent        *WorkloadView `json:"clusterAgent,omitempty"`
	ClusterChecksRunner *WorkloadView `json:"clusterChecksRunner,omitempty"`
	OtelAgentGateway    *WorkloadView `json:"otelAgentGateway,omitempty"`
}

// DAPView is a DatadogAgentProfile and its DDAI.
type DAPView struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Valid     string `json:"valid,omitempty"`
	Applied   string `json:"applied,omitempty"`
	// StatusUnknown is true when the profile has no status at all, e.g.
	// when the operator profile controller is not running. Its badge is
	// then Unknown, which does not affect the DDA health (optional data).
	StatusUnknown bool          `json:"statusUnknown,omitempty"`
	Health        Badge         `json:"health"`
	Provider      *ProviderView `json:"provider,omitempty"`
	Helm          *HelmView     `json:"helm,omitempty"`
	Conditions    []Cond        `json:"conditions,omitempty"`
	// DDAI is nil for invalid, conflicting or not applied profiles.
	DDAI *DDAIView `json:"ddai,omitempty"`
}

// HelmView is the Helm release managing an object.
type HelmView struct {
	Release   string `json:"release,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Chart     string `json:"chart,omitempty"`
	Version   string `json:"version,omitempty"`
	Revision  int    `json:"revision,omitempty"`
	// PreviousVersion and PreviousRevision are set during a rollout when
	// the release history is readable.
	PreviousVersion  string `json:"previousVersion,omitempty"`
	PreviousRevision int    `json:"previousRevision,omitempty"`
}

// ExperimentView is a Fleet experiment.
type ExperimentView struct {
	Phase     string     `json:"phase"`
	ID        string     `json:"id,omitempty"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
}

// DDAView is the DatadogAgent and its tree.
type DDAView struct {
	Namespace string     `json:"namespace"`
	Name      string     `json:"name"`
	Created   *time.Time `json:"created,omitempty"`
	Health    Badge      `json:"health"`
	// Agents sums the node agent counts over all DaemonSets,
	// hidden profiles included.
	Agents     Counts          `json:"agents"`
	Rollout    Rollout         `json:"rollout"`
	Experiment *ExperimentView `json:"experiment,omitempty"`
	Helm       *HelmView       `json:"helm,omitempty"`
	Conditions []Cond          `json:"conditions,omitempty"`
	Default    *DDAIView       `json:"default,omitempty"`
	// Profiles are ordered by BuildConfig.Sort, without the hidden ones.
	Profiles []DAPView `json:"profiles,omitempty"`
	// HiddenProfiles counts the profiles hidden by BuildConfig.HideEmpty or
	// HideEmptyForce; HiddenProfileNames lists them as
	// "<namespace>/<name>", in name order.
	HiddenProfiles     int      `json:"hiddenProfiles,omitempty"`
	HiddenProfileNames []string `json:"hiddenProfileNames,omitempty"`
	// HiddenBy is the filter that hid them; empty when none is hidden.
	HiddenBy ProfileFilter `json:"hiddenBy,omitempty"`
	// HiddenProfilesWithWarnings counts the hidden profiles that had an
	// Error or Warning issue, HiddenProfilesWithoutWarnings the others.
	HiddenProfilesWithoutWarnings int `json:"hiddenProfilesWithoutWarnings,omitempty"`
	HiddenProfilesWithWarnings    int `json:"hiddenProfilesWithWarnings,omitempty"`
	// HiddenIssues counts the issues of the hidden profiles, removed from
	// View.Issues (only with HideEmptyForce).
	HiddenIssues int `json:"hiddenIssues,omitempty"`
}

// ProfileFilter is the flag that hid profiles.
type ProfileFilter string

const (
	FilterHideEmpty      ProfileFilter = "hide-empty"
	FilterHideEmptyForce ProfileFilter = "hide-empty-force"
)

// UnattachedView groups objects whose parent can't be resolved.
type UnattachedView struct {
	DDAIs     []DDAIView     `json:"ddais,omitempty"`
	Workloads []WorkloadView `json:"workloads,omitempty"`
}

// SummaryView is the summary row of one Datadog CR kind.
type SummaryView struct {
	Kind  string      `json:"kind"`
	Label string      `json:"label"`
	State SourceState `json:"state"`
	// Notice is e.g. "listed in the DDA namespace only".
	Notice  string `json:"notice,omitempty"`
	Total   int    `json:"total"`
	OK      int    `json:"ok"`
	Errors  int    `json:"errors"`
	Unknown int    `json:"unknown"`
	// Breakdown counts objects by a kind-specific field: monitor state for
	// monitors, spec.type for generic resources.
	Breakdown map[string]int `json:"breakdown,omitempty"`
	// Failures lists the objects in error with their full message.
	Failures []SummaryFailure `json:"failures,omitempty"`
}

// SummaryFailure is a summary object in error.
type SummaryFailure struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Message   string `json:"message,omitempty"`
}

// SourceView is the acquisition status of one source.
type SourceView struct {
	Key      string      `json:"key"`
	State    SourceState `json:"state"`
	Mode     RefreshMode `json:"mode,omitempty"`
	LastOK   *time.Time  `json:"lastOK,omitempty"`
	NextPoll *time.Time  `json:"nextPoll,omitempty"`
	Notice   string      `json:"notice,omitempty"`
	Err      string      `json:"err,omitempty"`
}

// timePtr returns nil for the zero time so that it is omitted from JSON.
func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	t = t.UTC()
	return &t
}
