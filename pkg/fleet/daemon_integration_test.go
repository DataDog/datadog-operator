//go:build integration

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// reconcilerScheme mirrors the production scheme built in cmd/main.go: the
// full client-go aggregate scheme (covers the built-in kinds — NetworkPolicy,
// PodDisruptionBudget, RBAC, ... — the DDA reconcile path touches) plus the
// Datadog CRDs and the apiextensions CRD type the reconciler reads to check
// DDAI CRD availability.
func reconcilerScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(apiregistrationv1.AddToScheme(s))
	utilruntime.Must(v1alpha1.AddToScheme(s))
	utilruntime.Must(v2alpha1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
	return s
}

// integrationTestEnv/integrationClient back every test in this file with a
// real kube-apiserver + etcd (via envtest), so CRD schema validation and
// optimistic-lock conflicts behave like production instead of the fake
// client's more permissive emulation.
var (
	integrationTestEnv *envtest.Environment
	integrationClient  client.Client
)

func TestMain(m *testing.M) {
	integrationTestEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases", "v1")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := integrationTestEnv.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to start envtest environment:", err)
		os.Exit(1)
	}

	integrationClient, err = client.New(cfg, client.Options{Scheme: testFleetScheme()})
	if err != nil {
		_ = integrationTestEnv.Stop()
		fmt.Fprintln(os.Stderr, "failed to build envtest client:", err)
		os.Exit(1)
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testDDANSN.Namespace}}
	if err := integrationClient.Create(context.Background(), ns); err != nil && !apierrors.IsAlreadyExists(err) {
		_ = integrationTestEnv.Stop()
		fmt.Fprintln(os.Stderr, "failed to create test namespace:", err)
		os.Exit(1)
	}

	code := m.Run()
	_ = integrationTestEnv.Stop()
	os.Exit(code)
}

// createIntegrationDDA creates a DatadogAgent against the real envtest API
// server and returns the live object (with a server-assigned resourceVersion).
func createIntegrationDDA(t *testing.T, name string) *v2alpha1.DatadogAgent {
	t.Helper()
	dda := testDDAObject("")
	dda.Name = name
	dda.ResourceVersion = ""
	require.NoError(t, integrationClient.Create(context.Background(), dda))

	t.Cleanup(func() {
		_ = integrationClient.Delete(context.Background(), dda)
	})
	return dda
}

// TestExperimentCheckpoint_SchemaRejectsHalfCheckpoint proves that the CRD's
// generated schema makes status.experiment.checkpoint all-or-nothing: a write
// naming rollbackTargetRevision without expectedSpecHash is rejected by the
// API server, so no reconciler bug can leave an experiment running on half a
// checkpoint. The fake client does not run CRD OpenAPI validation, so this
// requires a real apiserver.
func TestExperimentCheckpoint_SchemaRejectsHalfCheckpoint(t *testing.T) {
	dda := createIntegrationDDA(t, "half-checkpoint")

	live := &v2alpha1.DatadogAgent{}
	require.NoError(t, integrationClient.Get(context.Background(), types.NamespacedName{Namespace: dda.Namespace, Name: dda.Name}, live))

	// A typed Status().Update() cannot express this case: both checkpoint
	// fields are non-pointer and non-omitempty, so a Go zero value would send
	// expectedSpecHash as "" and trip its Pattern marker instead of the
	// required-field check. Patch raw JSON so the key is absent entirely.
	patch := []byte(`{"status":{"experiment":{"checkpoint":{"rollbackTargetRevision":"dda-abc123"}}}}`)
	err := integrationClient.Status().Patch(context.Background(), live, client.RawPatch(types.MergePatchType, patch))
	require.Error(t, err)
	assert.True(t, apierrors.IsInvalid(err), "expected a schema validation error, got: %v", err)
	assert.Contains(t, err.Error(), "expectedSpecHash",
		"the rejection must name the missing checkpoint half, proving the ExperimentCheckpoint markers rejected it")
}
