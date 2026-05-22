// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package metadata

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

// crdTestHarness sets up everything processKey needs to run end-to-end:
// - kube-system Namespace seeded for GetOrCreateClusterUID
// - DD_API_KEY env var set so credential lookup doesn't need a DDA
// - httptest.Server intercepts the metadata POST; sendCount counts hits
// - fake k8s client wired through SharedMetadata
type crdTestHarness struct {
	cmf       *CRDMetadataForwarder
	sendCount *atomic.Int32
	srv       *httptest.Server
	client    client.Client
}

func newCRDTestHarness(t *testing.T, seedObjs ...client.Object) *crdTestHarness {
	t.Helper()

	t.Setenv(constants.DDAPIKey, "test-api-key")

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	if err := v2alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v2alpha1: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha1: %v", err)
	}

	kubeSystem := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: "kube-system-uid"},
	}
	objs := append([]client.Object{kubeSystem}, seedObjs...)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

	var sendCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sendCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Setenv(constants.DDURL, srv.URL)

	cmf := &CRDMetadataForwarder{
		SharedMetadata: NewSharedMetadata(zap.New(zap.UseDevMode(true)), c, "v1.28.0", "v1.19.0", config.NewCredentialManager(c)),
		enabledCRDs:    EnabledCRDKindsConfig{DatadogAgentEnabled: true, DatadogAgentInternalEnabled: true, DatadogAgentProfileEnabled: true},
	}
	return &crdTestHarness{cmf: cmf, sendCount: &sendCount, srv: srv, client: c}
}

func Test_CRDProcessKey_NewCRDSends(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "datadog", UID: "uid-1"},
	}
	dda.APIVersion = "datadoghq.com/v2alpha1"

	h := newCRDTestHarness(t, dda)
	if err := h.cmf.processKey(t.Context(), "DatadogAgent", "datadog", "agent"); err != nil {
		t.Fatalf("processKey: %v", err)
	}
	if got := h.sendCount.Load(); got != 1 {
		t.Errorf("send count = %d, want 1", got)
	}
	if _, ok := h.cmf.crdSnapshots.Load(EncodeKey("DatadogAgent", "datadog", "agent")); !ok {
		t.Errorf("snapshot not stored")
	}
}

func Test_CRDProcessKey_UnchangedSkipsSend(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "datadog", UID: "uid-1"},
	}
	h := newCRDTestHarness(t, dda)

	if err := h.cmf.processKey(t.Context(), "DatadogAgent", "datadog", "agent"); err != nil {
		t.Fatalf("first processKey: %v", err)
	}
	if err := h.cmf.processKey(t.Context(), "DatadogAgent", "datadog", "agent"); err != nil {
		t.Fatalf("second processKey: %v", err)
	}
	if got := h.sendCount.Load(); got != 1 {
		t.Errorf("send count = %d, want 1 (unchanged should not re-send)", got)
	}
}

func Test_CRDProcessKey_NotFoundIsNoOp(t *testing.T) {
	h := newCRDTestHarness(t)
	if err := h.cmf.processKey(t.Context(), "DatadogAgent", "datadog", "ghost"); err != nil {
		t.Errorf("processKey on missing object should be no-op, got %v", err)
	}
	if got := h.sendCount.Load(); got != 0 {
		t.Errorf("send count = %d, want 0", got)
	}
}

func Test_CRDHandleDelete(t *testing.T) {
	h := newCRDTestHarness(t)
	key := EncodeKey("DatadogAgent", "datadog", "agent")
	h.cmf.crdSnapshots.Store(key, &crdSnapshot{})

	h.cmf.handleDelete("DatadogAgent", "datadog", "agent")

	if _, ok := h.cmf.crdSnapshots.Load(key); ok {
		t.Errorf("snapshot still present after delete")
	}
}

func Test_CRDHeartbeatResendsAllSnapshots(t *testing.T) {
	h := newCRDTestHarness(t)
	h.cmf.crdSnapshots.Store(
		EncodeKey("DatadogAgent", "ns", "a"),
		&crdSnapshot{instance: CRDInstance{Kind: "DatadogAgent", Name: "a", Namespace: "ns"}, hash: "h-a"},
	)
	h.cmf.crdSnapshots.Store(
		EncodeKey("DatadogAgent", "ns", "b"),
		&crdSnapshot{instance: CRDInstance{Kind: "DatadogAgent", Name: "b", Namespace: "ns"}, hash: "h-b"},
	)

	h.cmf.heartbeat(t.Context())

	if got := h.sendCount.Load(); got != 2 {
		t.Errorf("heartbeat sent %d, want 2", got)
	}
}

// Test hashCRD function
func Test_HashCRD(t *testing.T) {
	crd1 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test",
		Namespace: "default",
		Spec: map[string]any{
			"version": "7.50.0",
			"image":   "datadog/agent:7.50.0",
		},
		Labels:      map[string]string{"app": "agent"},
		Annotations: map[string]string{"owner": "team"},
	}

	crd2 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test",
		Namespace: "default",
		Spec: map[string]any{
			"version": "7.50.0",
			"image":   "datadog/agent:7.50.0",
		},
		Labels:      map[string]string{"app": "agent"},
		Annotations: map[string]string{"owner": "team"},
	}

	crd3 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test",
		Namespace: "default",
		Spec: map[string]any{
			"version": "7.51.0",
			"image":   "datadog/agent:7.51.0",
		},
		Labels:      map[string]string{"app": "agent"},
		Annotations: map[string]string{"owner": "team"},
	}

	crd4 := CRDInstance{
		Kind:      "DatadogAgent",
		Name:      "test",
		Namespace: "default",
		Spec: map[string]any{
			"version": "7.50.0",
			"image":   "datadog/agent:7.50.0",
		},
		Labels:      map[string]string{"app": "agent", "env": "prod"}, // Different labels
		Annotations: map[string]string{"owner": "team"},
	}

	hash1, err := hashCRD(crd1)
	if err != nil {
		t.Fatalf("hashCRD failed: %v", err)
	}

	hash2, err := hashCRD(crd2)
	if err != nil {
		t.Fatalf("hashCRD failed: %v", err)
	}

	hash3, err := hashCRD(crd3)
	if err != nil {
		t.Fatalf("hashCRD failed: %v", err)
	}

	hash4, err := hashCRD(crd4)
	if err != nil {
		t.Fatalf("hashCRD failed: %v", err)
	}

	// Same CRDs (spec, labels, annotations) should produce same hash
	if hash1 != hash2 {
		t.Errorf("Expected same hash for identical CRDs, got %s and %s", hash1, hash2)
	}

	// Different specs should produce different hash
	if hash1 == hash3 {
		t.Errorf("Expected different hash for different specs, both got %s", hash1)
	}

	// Different labels should produce different hash
	if hash1 == hash4 {
		t.Errorf("Expected different hash for different labels, both got %s", hash1)
	}
}
