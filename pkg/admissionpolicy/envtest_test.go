// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package admissionpolicy

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// warningCollector captures the HTTP Warning headers the API server returns.
// A Warn binding reports through those and nothing else, so this is the only
// way to observe the policy actually running.
type warningCollector struct {
	mu       sync.Mutex
	warnings []string
}

func (w *warningCollector) HandleWarningHeader(code int, _ string, text string) {
	if code != 299 || text == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.warnings = append(w.warnings, text)
}

func (w *warningCollector) take() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := append([]string(nil), w.warnings...)
	w.warnings = nil
	return out
}

func (w *warningCollector) contains(substr string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, warning := range w.warnings {
		if strings.Contains(warning, substr) {
			return true
		}
	}
	return false
}

// TestPoliciesAgainstRealAPIServer is what the unit tests cannot be: the rules
// evaluated by an actual API server rather than by our own copy of its code.
//
// It closes two gaps. Creating the policy makes the API server compile every
// expression at its own compatibility version, which is the check that a rule
// using something newer than the pin would fail - and the one that would
// otherwise only show up on a customer's cluster, as a rejected policy object
// and a retry in the logs. Then applying a bad DatadogAgent shows the Warn
// binding end to end: the rule runs, and our Message is what the user sees.
func TestPoliciesAgainstRealAPIServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases", "v1")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	require.NoError(t, err, "envtest needs KUBEBUILDER_ASSETS; run via make integration-tests")
	t.Cleanup(func() { _ = env.Stop() })

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, admregv1.AddToScheme(scheme))
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	warnings := &warningCollector{}
	warned := rest.CopyConfig(cfg)
	warned.WarningHandler = warnings
	cl, err := client.New(warned, client.Options{Scheme: scheme})
	require.NoError(t, err)

	t.Run("the API server accepts every policy and binding", func(t *testing.T) {
		for _, target := range Targets() {
			// Create, not apply: a rejection here is the failure we want, and
			// it carries the API server's own reason for it.
			require.NoErrorf(t, cl.Create(ctx, BuildPolicy(target)),
				"API server rejected the policy for %s", target.name)
			require.NoErrorf(t, cl.Create(ctx, BuildBinding(target)),
				"API server rejected the binding for %s", target.name)
		}
	})

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vap-envtest"}}
	require.NoError(t, cl.Create(ctx, namespace))

	t.Run("a DatadogAgent breaking a rule is warned about, not rejected", func(t *testing.T) {
		const want = "credentials not configured"

		// A policy is not enforced the instant it is created: the API server
		// has to observe it and compile it. Retry with a fresh name rather
		// than sleeping on a guess.
		var lastErr error
		deadline := time.Now().Add(90 * time.Second)
		for attempt := 0; time.Now().Before(deadline); attempt++ {
			warnings.take()
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "no-credentials-" + string(rune('a'+attempt%26)) + "-" + time.Now().Format("150405.000000"),
			}}
			lastErr = cl.Create(ctx, dda)
			// Warn never rejects. If this errors, something other than the
			// policy is wrong and the message is worth seeing.
			require.NoError(t, lastErr)
			if warnings.contains(want) {
				return
			}
			time.Sleep(2 * time.Second)
		}
		t.Fatalf("no warning containing %q after 90s; last warnings: %v", want, warnings.take())
	})

	t.Run("a valid DatadogAgent is not warned about", func(t *testing.T) {
		warnings.take()
		dda := &v2alpha1.DatadogAgent{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace.Name, Name: "valid"},
			Spec: v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Credentials: &v2alpha1.DatadogCredentials{APIKey: ptrTo("key")},
				},
			},
		}
		require.NoError(t, cl.Create(ctx, dda))
		assert.Empty(t, warnings.take(), "a valid DatadogAgent should produce no policy warning")
	})
}

func ptrTo[T any](v T) *T { return &v }
