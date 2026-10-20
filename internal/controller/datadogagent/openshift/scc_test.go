// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openshift

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// grantShape decides a simulated SubjectAccessReview verdict.
//
// The fake client performs no authorization of its own: it is an object store, so
// seeding RBAC objects into it would change nothing. The Create interceptor below
// therefore *is* the authorizer, answering the way an API server backed by the given
// grant shape would.
type grantShape int

const (
	// grantDirect: a RoleBinding naming the ServiceAccount user directly.
	grantDirect grantShape = iota
	// grantGroupOnly: a binding to a service-account group, with no user subject.
	// This is the case that fails if the implementation omits spec.groups.
	grantGroupOnly
	// grantNone: no binding at all.
	grantNone
)

const (
	testNamespace = "datadog"
	testSA        = "datadog-agent-scc"
)

// newFakeAuthorizer builds an SCCAuthorizer whose API server simulates shape, and
// captures the SubjectAccessReview the implementation actually sent.
func newFakeAuthorizer(t *testing.T, shape grantShape, createErr error) (SCCAuthorizer, *authorizationv1.SubjectAccessReview) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))

	sent := &authorizationv1.SubjectAccessReview{}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if createErr != nil {
					return createErr
				}
				review, ok := obj.(*authorizationv1.SubjectAccessReview)
				if !ok {
					t.Fatalf("expected a SubjectAccessReview, got %T", obj)
				}
				review.Spec.DeepCopyInto(&sent.Spec)

				switch shape {
				case grantDirect:
					// Only the user subject is bound; groups are irrelevant.
					review.Status.Allowed = review.Spec.User == ServiceAccountUsername(testNamespace, testSA)
				case grantGroupOnly:
					// Deliberately ignores spec.User. An implementation that omits
					// spec.Groups gets denied here, reproducing the false denial that a
					// real cluster shows for group-granted SCC access.
					review.Status.Allowed = slices.Contains(review.Spec.Groups, groupServiceAccounts+":"+testNamespace)
				case grantNone:
					review.Status.Allowed = false
				}
				return nil
			},
		}).
		Build()

	return NewSARAuthorizer(c), sent
}

func TestSARAuthorizer_SendsServiceAccountGroups(t *testing.T) {
	auth, sent := newFakeAuthorizer(t, grantDirect, nil)

	_, err := auth.CanUseSCC(context.Background(), testNamespace, testSA, RequiredSCCName)
	require.NoError(t, err)

	assert.Equal(t, "system:serviceaccount:datadog:datadog-agent-scc", sent.Spec.User)

	// Mandatory: a SubjectAccessReview does not synthesize a ServiceAccount's implicit
	// groups from spec.User, so omitting these makes any group-granted SCC access
	// invisible and reports a false denial.
	assert.ElementsMatch(t, []string{
		"system:authenticated",
		"system:serviceaccounts",
		"system:serviceaccounts:datadog",
	}, sent.Spec.Groups)

	require.NotNil(t, sent.Spec.ResourceAttributes)
	assert.Equal(t, "security.openshift.io", sent.Spec.ResourceAttributes.Group)
	assert.Equal(t, "securitycontextconstraints", sent.Spec.ResourceAttributes.Resource)
	assert.Equal(t, "use", sent.Spec.ResourceAttributes.Verb)
	assert.Equal(t, RequiredSCCName, sent.Spec.ResourceAttributes.Name)
}

func TestSARAuthorizer_Verdicts(t *testing.T) {
	tests := []struct {
		name    string
		shape   grantShape
		allowed bool
	}{
		{
			name:    "direct user binding is allowed",
			shape:   grantDirect,
			allowed: true,
		},
		{
			// The regression test for the groups omission. A real cluster grants
			// restricted-v2 through system:authenticated exactly this way, and a
			// user-only SubjectAccessReview reports false for it.
			name:    "group-only binding is allowed",
			shape:   grantGroupOnly,
			allowed: true,
		},
		{
			name:    "no binding is denied",
			shape:   grantNone,
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, _ := newFakeAuthorizer(t, tt.shape, nil)

			allowed, err := auth.CanUseSCC(context.Background(), testNamespace, testSA, RequiredSCCName)

			require.NoError(t, err)
			assert.Equal(t, tt.allowed, allowed)
		})
	}
}

func TestSARAuthorizer_APIError(t *testing.T) {
	wantErr := errors.New("apiserver unavailable")
	auth, _ := newFakeAuthorizer(t, grantDirect, wantErr)

	allowed, err := auth.CanUseSCC(context.Background(), testNamespace, testSA, RequiredSCCName)

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.False(t, allowed, "a failed check must not be reported as authorized")
}

func TestServiceAccountGroups(t *testing.T) {
	assert.Equal(t, []string{
		"system:authenticated",
		"system:serviceaccounts",
		"system:serviceaccounts:kube-system",
	}, ServiceAccountGroups("kube-system"))
}
