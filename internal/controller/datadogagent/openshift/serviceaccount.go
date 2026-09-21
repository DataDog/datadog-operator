// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openshift

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// Condition reasons for the OpenShiftSCC condition.
const (
	// ReasonServiceAccountConfigured: the operator set the bundle ServiceAccount and
	// verified it can use the required SCC.
	ReasonServiceAccountConfigured = "ServiceAccountConfigured"
	// ReasonUserManaged: the user supplied a ServiceAccount and it verified.
	ReasonUserManaged = "UserManaged"
	// ReasonServiceAccountCannotUseSCC: the user supplied a ServiceAccount that cannot
	// use the required SCC. Left in place, but the node agent will not be admitted.
	ReasonServiceAccountCannotUseSCC = "ServiceAccountCannotUseSCC"
	// ReasonNoAuthorizedServiceAccount: no user value, and the bundle ServiceAccount
	// could not be verified. Nothing was set.
	ReasonNoAuthorizedServiceAccount = "NoAuthorizedServiceAccount"
	// ReasonSCCCheckFailed: the authorization check itself failed, so the verdict is
	// unknown and the spec was left untouched.
	ReasonSCCCheckFailed = "SCCCheckFailed"
	// ReasonServiceAccountRetained: a previously verified ServiceAccount was kept even
	// though this round could not confirm it. Counts as "previously configured" for
	// WasServiceAccountConfigured, so the decision stays stable across reconciles
	// instead of alternating.
	ReasonServiceAccountRetained = "ServiceAccountRetained"
)

// WasServiceAccountConfigured reports whether an earlier reconcile resolved the node
// agent to the bundle ServiceAccount, read from the OpenShiftSCC condition reason —
// the same trick wasProviderDetected uses, so no status field is needed.
//
// Retained counts alongside Configured: otherwise the first retained reconcile erases
// the memory that made it retain, and the next one reverts.
func WasServiceAccountConfigured(reason string) bool {
	return reason == ReasonServiceAccountConfigured || reason == ReasonServiceAccountRetained
}

// Outcome is the result of the node-agent ServiceAccount / SCC reconciliation.
type Outcome struct {
	// ServiceAccount is the account that was checked: the user's if they set one,
	// otherwise the bundle default.
	ServiceAccount string
	// UserManaged reports that ServiceAccount came from spec.override rather than
	// from this package.
	UserManaged bool
	// Allowed is the authorization verdict. Meaningless when Err is set.
	Allowed bool
	// Applied reports that this package wrote the ServiceAccount into the spec after
	// verifying it.
	Applied bool
	// Retained reports that a previously verified ServiceAccount was kept even though
	// this round could not confirm it.
	Retained bool
	// Err is a failure of the check itself, not a denial.
	Err error
}

// ReconcileAgentServiceAccount verifies that the node agent will run under a
// ServiceAccount permitted to use the required SCC, and fills in the bundle
// ServiceAccount when the user has not chosen one.
//
// Two rules that are easy to conflate are kept separate here:
//
//  1. A user-supplied ServiceAccount is never overwritten.
//  2. Whichever ServiceAccount will be used is always verified. Reporting healthy just
//     because the user chose the value would sit a green condition next to a DaemonSet
//     whose pods are being rejected.
//
// Nothing is written when the check cannot confirm authorization: an unverified
// ServiceAccount is as broken as none, and harder to debug.
// previouslyConfigured makes the decision durable: once verified, an unverified round
// keeps the ServiceAccount rather than reverting. Reverting would change
// serviceAccountName, roll the DaemonSet, and start the replacement pods under an
// account without SCC access — so a transient API error would take the Agent down.
func ReconcileAgentServiceAccount(ctx context.Context, auth SCCAuthorizer, namespace string, spec *v2alpha1.DatadogAgentSpec, previouslyConfigured bool) Outcome {
	userSA := nodeAgentServiceAccount(spec)

	sa := userSA
	if sa == "" {
		sa = SCCServiceAccountName
	}

	allowed, err := auth.CanUseSCC(ctx, namespace, sa, RequiredSCCName)

	out := Outcome{ServiceAccount: sa, UserManaged: userSA != ""}
	if err != nil {
		out.Err = err
	} else {
		out.Allowed = allowed
	}

	// A user-supplied account is reported on but never rewritten.
	if userSA != "" {
		return out
	}

	switch {
	case out.Err == nil && out.Allowed:
		setNodeAgentServiceAccount(spec, sa)
		out.Applied = true
	case previouslyConfigured:
		setNodeAgentServiceAccount(spec, sa)
		out.Retained = true
	}
	return out
}

// Condition renders the outcome as a status condition.
func (o Outcome) Condition() (metav1.ConditionStatus, string, string) {
	switch {
	// Retained is checked first: the reason must stay in the "previously configured"
	// set, otherwise the next reconcile forgets and reverts.
	case o.Retained && o.Err != nil:
		return metav1.ConditionUnknown, ReasonServiceAccountRetained, fmt.Sprintf(
			"Kept previously verified ServiceAccount %q; could not re-check %q access: %v",
			o.ServiceAccount, RequiredSCCName, o.Err)

	case o.Retained:
		return metav1.ConditionFalse, ReasonServiceAccountRetained, fmt.Sprintf(
			"ServiceAccount %q may no longer use the %q SecurityContextConstraints. Kept, because "+
				"reverting would restart the node agent under an account that also cannot be "+
				"admitted. Grant it, or set spec.override.nodeAgent.serviceAccountName.",
			o.ServiceAccount, RequiredSCCName)

	case o.Err != nil:
		return metav1.ConditionUnknown, ReasonSCCCheckFailed, fmt.Sprintf(
			"Could not determine whether ServiceAccount %q may use %q: %v",
			o.ServiceAccount, RequiredSCCName, o.Err)

	case o.Allowed && o.UserManaged:
		return metav1.ConditionTrue, ReasonUserManaged, fmt.Sprintf(
			"Node agent uses the configured ServiceAccount %q, which may use %q.",
			o.ServiceAccount, RequiredSCCName)

	case o.Allowed:
		return metav1.ConditionTrue, ReasonServiceAccountConfigured, fmt.Sprintf(
			"Node agent ServiceAccount set to %q, which may use %q.",
			o.ServiceAccount, RequiredSCCName)

	case o.UserManaged:
		return metav1.ConditionFalse, ReasonServiceAccountCannotUseSCC, fmt.Sprintf(
			"ServiceAccount %q may not use the %q SecurityContextConstraints. The node agent mounts "+
				"hostPath volumes and runs as UID 0, so its pods will not be admitted until it is granted.",
			o.ServiceAccount, RequiredSCCName)

	default:
		return metav1.ConditionFalse, ReasonNoAuthorizedServiceAccount, fmt.Sprintf(
			"ServiceAccount %q may not use the %q SecurityContextConstraints, so it was not applied. "+
				"The node agent will not be admitted until one that may is set via "+
				"spec.override.nodeAgent.serviceAccountName.",
			o.ServiceAccount, RequiredSCCName)
	}
}

// nodeAgentServiceAccount returns the user-specified node agent ServiceAccount, or
// "" when unset.
func nodeAgentServiceAccount(spec *v2alpha1.DatadogAgentSpec) string {
	override, ok := spec.Override[v2alpha1.NodeAgentComponentName]
	if !ok || override == nil || override.ServiceAccountName == nil {
		return ""
	}
	return *override.ServiceAccountName
}

// setNodeAgentServiceAccount writes the node agent ServiceAccount into the spec.
// Downstream this is read back through constants.GetAgentServiceAccount, so the pod
// spec and the RBAC binding stay consistent without either needing to know about
// OpenShift.
func setNodeAgentServiceAccount(spec *v2alpha1.DatadogAgentSpec, name string) {
	if spec.Override == nil {
		spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{}
	}
	if spec.Override[v2alpha1.NodeAgentComponentName] == nil {
		spec.Override[v2alpha1.NodeAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{}
	}
	spec.Override[v2alpha1.NodeAgentComponentName].ServiceAccountName = &name
}
