// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package openshift holds the OpenShift-specific reconcile logic that depends on
// live cluster authorization state, currently the SecurityContextConstraints
// check that decides which ServiceAccount the node agent can run under.
package openshift

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// SCCServiceAccountName is the ServiceAccount the operator bundle provisions on
	// OpenShift. OLM materializes the CSV's clusterPermissions entry (see
	// hack/patch-bundle-csv-scc.yaml) as this ServiceAccount plus a ClusterRole
	// granting it `use` on the hostaccess and privileged SCCs, and a binding.
	SCCServiceAccountName = "datadog-agent-scc"

	// RequiredSCCName is the SCC the node agent needs. It is privileged, not
	// hostaccess: hostaccess allows hostPath but constrains runAsUser to
	// MustRunAsRange, which excludes the UID 0 the node agent always runs as.
	RequiredSCCName = "privileged"

	sccAPIGroup  = "security.openshift.io"
	sccResource  = "securitycontextconstraints"
	sccUseVerb   = "use"
	saUserPrefix = "system:serviceaccount:"

	groupAuthenticated   = "system:authenticated"
	groupServiceAccounts = "system:serviceaccounts"
)

// SCCAuthorizer answers whether a ServiceAccount is permitted to use a given
// SecurityContextConstraints. It is an interface so the live SubjectAccessReview
// can be stubbed in tests and in the golden-manifest renderer.
type SCCAuthorizer interface {
	CanUseSCC(ctx context.Context, namespace, serviceAccount, sccName string) (bool, error)
}

// sarAuthorizer answers via a SubjectAccessReview against the API server.
type sarAuthorizer struct {
	client client.Client
}

// NewSARAuthorizer returns an SCCAuthorizer backed by SubjectAccessReview. Needs no
// extra operator RBAC: the controller already holds subjectaccessreviews get;create.
func NewSARAuthorizer(c client.Client) SCCAuthorizer {
	return &sarAuthorizer{client: c}
}

// CanUseSCC asks the API server the same question SCC admission will ask later.
//
// A SubjectAccessReview beats reading the ServiceAccount/ClusterRole/binding and
// diffing them against the bundle: OLM names the binding "<csv>.v<version>-<random>",
// so it is not discoverable and changes every release, and the same grant can arrive
// through an aggregated role or a group.
func (a *sarAuthorizer) CanUseSCC(ctx context.Context, namespace, serviceAccount, sccName string) (bool, error) {
	review := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   ServiceAccountUsername(namespace, serviceAccount),
			Groups: ServiceAccountGroups(namespace),
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Group:    sccAPIGroup,
				Resource: sccResource,
				Name:     sccName,
				Verb:     sccUseVerb,
			},
		},
	}

	if err := a.client.Create(ctx, review); err != nil {
		return false, fmt.Errorf("unable to create SubjectAccessReview for %s on SCC %q: %w",
			ServiceAccountUsername(namespace, serviceAccount), sccName, err)
	}

	return review.Status.Allowed, nil
}

// ServiceAccountUsername renders the API-server username for a ServiceAccount.
func ServiceAccountUsername(namespace, serviceAccount string) string {
	return saUserPrefix + namespace + ":" + serviceAccount
}

// ServiceAccountGroups returns the groups a real ServiceAccount request carries.
//
// These must be sent explicitly: a SubjectAccessReview does not synthesize them from
// spec.user, so a grant made through a group binding would report a false denial. A
// direct user binding is allowed either way, which is why the omission is easy to miss.
func ServiceAccountGroups(namespace string) []string {
	return []string{
		groupAuthenticated,
		groupServiceAccounts,
		groupServiceAccounts + ":" + namespace,
	}
}
