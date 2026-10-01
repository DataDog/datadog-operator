// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openshift

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// stubAuthorizer returns a fixed verdict and records what it was asked about.
type stubAuthorizer struct {
	allowed  bool
	err      error
	askedFor string
}

func (s *stubAuthorizer) CanUseSCC(_ context.Context, _, serviceAccount, _ string) (bool, error) {
	s.askedFor = serviceAccount
	if s.err != nil {
		return false, s.err
	}
	return s.allowed, nil
}

func specWithNodeAgentSA(name string) *v2alpha1.DatadogAgentSpec {
	spec := &v2alpha1.DatadogAgentSpec{}
	if name != "" {
		setNodeAgentServiceAccount(spec, name)
	}
	return spec
}

func TestReconcileAgentServiceAccount(t *testing.T) {
	sarErr := errors.New("apiserver unavailable")

	tests := []struct {
		name string
		// userSA is spec.override.nodeAgent.serviceAccountName; "" means unset.
		userSA      string
		allowed     bool
		err         error
		wantChecked string
		wantSpecSA  string
		wantStatus  metav1.ConditionStatus
		wantReason  string
	}{
		{
			name:        "no user value, authorized: bundle SA is applied",
			userSA:      "",
			allowed:     true,
			wantChecked: SCCServiceAccountName,
			wantSpecSA:  SCCServiceAccountName,
			wantStatus:  metav1.ConditionTrue,
			wantReason:  ReasonServiceAccountConfigured,
		},
		{
			// Nothing is written when we could not confirm authorization: an
			// unverified ServiceAccount fails admission exactly like a missing one,
			// but is harder to diagnose.
			name:        "no user value, denied: nothing is applied",
			userSA:      "",
			allowed:     false,
			wantChecked: SCCServiceAccountName,
			wantSpecSA:  "",
			wantStatus:  metav1.ConditionFalse,
			wantReason:  ReasonNoAuthorizedServiceAccount,
		},
		{
			name:        "user value, authorized: preserved and reported healthy",
			userSA:      "my-own-sa",
			allowed:     true,
			wantChecked: "my-own-sa",
			wantSpecSA:  "my-own-sa",
			wantStatus:  metav1.ConditionTrue,
			wantReason:  ReasonUserManaged,
		},
		{
			// The false-healthy guard: the user's value is still respected, but it is
			// verified too, so a typo'd or under-privileged ServiceAccount is reported
			// rather than sitting green next to a DaemonSet failing admission.
			name:        "user value, denied: preserved but reported unhealthy",
			userSA:      "my-own-sa",
			allowed:     false,
			wantChecked: "my-own-sa",
			wantSpecSA:  "my-own-sa",
			wantStatus:  metav1.ConditionFalse,
			wantReason:  ReasonServiceAccountCannotUseSCC,
		},
		{
			name:        "check fails: spec untouched, verdict unknown",
			userSA:      "",
			err:         sarErr,
			wantChecked: SCCServiceAccountName,
			wantSpecSA:  "",
			wantStatus:  metav1.ConditionUnknown,
			wantReason:  ReasonSCCCheckFailed,
		},
		{
			name:        "check fails with a user value: user value preserved",
			userSA:      "my-own-sa",
			err:         sarErr,
			wantChecked: "my-own-sa",
			wantSpecSA:  "my-own-sa",
			wantStatus:  metav1.ConditionUnknown,
			wantReason:  ReasonSCCCheckFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := specWithNodeAgentSA(tt.userSA)
			auth := &stubAuthorizer{allowed: tt.allowed, err: tt.err}

			outcome := ReconcileAgentServiceAccount(context.Background(), auth, "datadog", spec, false)

			assert.Equal(t, tt.wantChecked, auth.askedFor,
				"must verify whichever ServiceAccount will actually be used")
			assert.Equal(t, tt.wantSpecSA, nodeAgentServiceAccount(spec))

			status, reason, message := outcome.Condition()
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantReason, reason)
			assert.NotEmpty(t, message)
			assert.Contains(t, message, tt.wantChecked,
				"the message should name the ServiceAccount so it is actionable")
		})
	}
}

func TestReconcileAgentServiceAccount_LeavesClusterAgentAlone(t *testing.T) {
	spec := &v2alpha1.DatadogAgentSpec{}
	auth := &stubAuthorizer{allowed: true}

	ReconcileAgentServiceAccount(context.Background(), auth, "datadog", spec, false)

	// The cluster agent has no hostPath volumes and no pod securityContext, so
	// restricted-v2 already admits it. Only the node agent needs the elevated SCC.
	_, ok := spec.Override[v2alpha1.ClusterAgentComponentName]
	assert.False(t, ok, "cluster agent override must not be created")
}

func TestReconcileAgentServiceAccount_PreservesOtherOverrides(t *testing.T) {
	spec := &v2alpha1.DatadogAgentSpec{
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {HostNetwork: ptr.To(true)},
		},
	}
	auth := &stubAuthorizer{allowed: true}

	ReconcileAgentServiceAccount(context.Background(), auth, "datadog", spec, false)

	override := spec.Override[v2alpha1.NodeAgentComponentName]
	require.NotNil(t, override)
	assert.Equal(t, SCCServiceAccountName, *override.ServiceAccountName)
	require.NotNil(t, override.HostNetwork, "existing override fields must survive")
	assert.True(t, *override.HostNetwork)
}

// TestReconcileAgentServiceAccount_Durability covers the anti-flap behaviour: once a
// ServiceAccount has been verified, a later unverified round must keep it.
//
// Reverting would change serviceAccountName on the DaemonSet, roll it, and start the
// replacement pods under an account without SCC access — so a transient API error
// would take the Agent down.
func TestReconcileAgentServiceAccount_Durability(t *testing.T) {
	sarErr := errors.New("apiserver unavailable")

	tests := []struct {
		name                 string
		previouslyConfigured bool
		allowed              bool
		err                  error
		wantSpecSA           string
		wantRetained         bool
		wantStatus           metav1.ConditionStatus
		wantReason           string
	}{
		{
			name:                 "transient error after a prior success: SA kept",
			previouslyConfigured: true,
			err:                  sarErr,
			wantSpecSA:           SCCServiceAccountName,
			wantRetained:         true,
			wantStatus:           metav1.ConditionUnknown,
			wantReason:           ReasonServiceAccountRetained,
		},
		{
			name:                 "grant revoked after a prior success: SA kept, reported False",
			previouslyConfigured: true,
			allowed:              false,
			wantSpecSA:           SCCServiceAccountName,
			wantRetained:         true,
			wantStatus:           metav1.ConditionFalse,
			wantReason:           ReasonServiceAccountRetained,
		},
		{
			name:                 "still authorized: normal configured path, not retained",
			previouslyConfigured: true,
			allowed:              true,
			wantSpecSA:           SCCServiceAccountName,
			wantRetained:         false,
			wantStatus:           metav1.ConditionTrue,
			wantReason:           ReasonServiceAccountConfigured,
		},
		{
			name:                 "error with NO prior success: nothing written",
			previouslyConfigured: false,
			err:                  sarErr,
			wantSpecSA:           "",
			wantRetained:         false,
			wantStatus:           metav1.ConditionUnknown,
			wantReason:           ReasonSCCCheckFailed,
		},
		{
			name:                 "denied with NO prior success: nothing written",
			previouslyConfigured: false,
			allowed:              false,
			wantSpecSA:           "",
			wantRetained:         false,
			wantStatus:           metav1.ConditionFalse,
			wantReason:           ReasonNoAuthorizedServiceAccount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &v2alpha1.DatadogAgentSpec{}
			auth := &stubAuthorizer{allowed: tt.allowed, err: tt.err}

			outcome := ReconcileAgentServiceAccount(context.Background(), auth, "datadog", spec, tt.previouslyConfigured)

			assert.Equal(t, tt.wantSpecSA, nodeAgentServiceAccount(spec))
			assert.Equal(t, tt.wantRetained, outcome.Retained)

			status, reason, _ := outcome.Condition()
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantReason, reason)

			// The reason a retained round reports must itself count as "previously
			// configured", or the next reconcile forgets and reverts — reintroducing
			// exactly the flap this guards against.
			if tt.wantRetained {
				assert.True(t, WasServiceAccountConfigured(reason),
					"retained reason must keep the decision durable across reconciles")
			}
		})
	}
}

// TestReconcileAgentServiceAccount_RetentionIsStable walks consecutive reconciles to
// prove the resolved ServiceAccount does not oscillate while the API is failing.
//
// Each round starts from a fresh spec, because that is what the controller does:
// internalReconcile deep-copies the DatadogAgent every time and never writes the spec
// back, so a value this package wrote last round is not visible in the next one. Only
// the condition reason carries across, which is precisely why retention is driven by
// the reason rather than by inspecting the spec.
func TestReconcileAgentServiceAccount_RetentionIsStable(t *testing.T) {
	sarFails := func() *stubAuthorizer { return &stubAuthorizer{err: errors.New("apiserver unavailable")} }

	// Round 1: authorized.
	spec := &v2alpha1.DatadogAgentSpec{}
	out := ReconcileAgentServiceAccount(context.Background(), &stubAuthorizer{allowed: true}, "datadog", spec, false)
	_, reason, _ := out.Condition()
	require.Equal(t, SCCServiceAccountName, nodeAgentServiceAccount(spec))
	require.Equal(t, ReasonServiceAccountConfigured, reason)

	// Rounds 2-4: the API keeps failing. The resolved account must stay put, and the
	// reason fed forward must keep sustaining the retention.
	for i := range 3 {
		spec = &v2alpha1.DatadogAgentSpec{}
		out = ReconcileAgentServiceAccount(context.Background(), sarFails(), "datadog", spec,
			WasServiceAccountConfigured(reason))
		_, reason, _ = out.Condition()

		require.True(t, out.Retained, "round %d should have retained", i+2)
		require.Equal(t, SCCServiceAccountName, nodeAgentServiceAccount(spec),
			"round %d changed the ServiceAccount, which would roll the DaemonSet", i+2)
	}

	// Recovery: back to the plain configured state.
	spec = &v2alpha1.DatadogAgentSpec{}
	out = ReconcileAgentServiceAccount(context.Background(), &stubAuthorizer{allowed: true}, "datadog", spec,
		WasServiceAccountConfigured(reason))
	_, reason, _ = out.Condition()
	assert.False(t, out.Retained)
	assert.Equal(t, ReasonServiceAccountConfigured, reason)
	assert.Equal(t, SCCServiceAccountName, nodeAgentServiceAccount(spec))
}

const testOwnerUID = types.UID("owner-uid")

func TestIsBundleManagedServiceAccount(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		saName   string
		want     bool
	}{
		{"detected openshift, bundle account", "openshift-rhcos", SCCServiceAccountName, true},
		{"bare openshift, bundle account", kubernetes.OpenshiftProvider, SCCServiceAccountName, true},
		{"openshift, operator-created account", "openshift-rhcos", "datadog-agent-agent", false},
		{"other provider reusing the name", kubernetes.GKECosProvider, SCCServiceAccountName, false},
		{"no provider", "", SCCServiceAccountName, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsBundleManagedServiceAccount(tt.provider, tt.saName))
		})
	}
}

func TestDisownBundleServiceAccount(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	adopted := func() *corev1.ServiceAccount {
		return &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      SCCServiceAccountName,
				Namespace: "ns",
				Labels: map[string]string{
					store.OperatorStoreLabelKey:            "true",
					kubernetes.AppKubernetesPartOfLabelKey: "dd",
				},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "DatadogAgentInternal", Name: "dd", UID: testOwnerUID},
				},
			},
		}
	}

	t.Run("adopted account loses both markers", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(adopted()).Build()
		require.NoError(t, DisownBundleServiceAccount(context.Background(), c, "ns", testOwnerUID))

		got := &corev1.ServiceAccount{}
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: SCCServiceAccountName}, got))
		assert.NotContains(t, got.Labels, store.OperatorStoreLabelKey)
		assert.Empty(t, got.OwnerReferences)
		// Unrelated labels are left alone.
		assert.Equal(t, "dd", got.Labels[kubernetes.AppKubernetesPartOfLabelKey])
	})

	t.Run("another owner's reference is kept", func(t *testing.T) {
		sa := adopted()
		sa.OwnerReferences = append(sa.OwnerReferences, metav1.OwnerReference{Kind: "ClusterServiceVersion", Name: "csv", UID: "csv-uid"})
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sa).Build()
		require.NoError(t, DisownBundleServiceAccount(context.Background(), c, "ns", testOwnerUID))

		got := &corev1.ServiceAccount{}
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: SCCServiceAccountName}, got))
		require.Len(t, got.OwnerReferences, 1)
		assert.Equal(t, "ClusterServiceVersion", got.OwnerReferences[0].Kind)
	})

	t.Run("unmarked account is untouched", func(t *testing.T) {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: SCCServiceAccountName, Namespace: "ns"}}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sa).Build()
		require.NoError(t, DisownBundleServiceAccount(context.Background(), c, "ns", testOwnerUID))

		got := &corev1.ServiceAccount{}
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: SCCServiceAccountName}, got))
		assert.Equal(t, sa.ResourceVersion, got.ResourceVersion)
	})

	t.Run("absent account is not an error", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		assert.NoError(t, DisownBundleServiceAccount(context.Background(), c, "ns", testOwnerUID))
	})
}
