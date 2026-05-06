// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package renderer simulates the Datadog Operator's reconciliation loop offline.
// Given a DatadogAgent (and optional DatadogAgentProfiles), Render returns the
// complete set of Kubernetes resources the operator would create — without
// needing a running cluster. Useful for golden-file regression tests and the
// operator-render CLI.
package renderer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	datadogagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	datadogagentinternal "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// Options configures a Render invocation.
type Options struct {
	// DDA is the input DatadogAgent. Required.
	DDA *datadoghqv2alpha1.DatadogAgent
	// DAPs are optional DatadogAgentProfiles. They are passed to the
	// reconciler regardless of ProfileEnabled, but only take effect when
	// ProfileEnabled is true.
	DAPs []*datadoghqv1alpha1.DatadogAgentProfile
	// ProfileEnabled toggles DatadogAgentProfile reconciliation, mirroring
	// the operator's --datadogAgentProfileEnabled flag.
	ProfileEnabled bool
	// SupportCilium emits CiliumNetworkPolicy resources alongside NetworkPolicy.
	SupportCilium bool
}

// noopForwarder satisfies datadog.MetricsForwardersManager with no-op methods.
// All real methods are guarded by OperatorMetricsEnabled=false so they are never called.
type noopForwarder struct{}

func (noopForwarder) Register(client.Object)                                              {}
func (noopForwarder) Unregister(client.Object)                                            {}
func (noopForwarder) ProcessError(client.Object, error)                                   {}
func (noopForwarder) ProcessEvent(client.Object, datadog.Event)                           {}
func (noopForwarder) MetricsForwarderStatusForObj(client.Object) *datadog.ConditionCommon { return nil }
func (noopForwarder) SetEnabledFeatures(client.Object, []string)                          {}

// BuildScheme returns a runtime.Scheme registered with all the API groups the
// operator's reconcilers and resource builders need.
func BuildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(apiregistrationv1.AddToScheme(s))
	utilruntime.Must(datadoghqv1alpha1.AddToScheme(s))
	utilruntime.Must(edsdatadoghqv1alpha1.AddToScheme(s))
	utilruntime.Must(datadoghqv2alpha1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
	return s
}

// loadDDAICRD reads the DatadogAgentInternal CRD from config/crd/bases/v1/.
// The CRD must be pre-loaded in the fake client because newFieldManager()
// inside the V3 reconciler does a client.Get to read its OpenAPI schema.
//
// The path is resolved at compile time via runtime.Caller, so the binary
// only works when run against the source tree it was built from.
func loadDDAICRD(scheme *runtime.Scheme) (*apiextensionsv1.CustomResourceDefinition, error) {
	_, filename, _, ok := goruntime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("unable to resolve renderer source path")
	}
	// internal/controller/testutils/renderer/ → repo root is 4 levels up.
	path := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..",
		"config", "crd", "bases", "v1", "datadoghq.com_datadogagentinternals.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading DDAI CRD at %s: %w", path, err)
	}
	codecs := serializer.NewCodecFactory(scheme)
	obj, _, err := codecs.UniversalDeserializer().Decode(body, nil, &apiextensionsv1.CustomResourceDefinition{})
	if err != nil {
		return nil, fmt.Errorf("decoding DDAI CRD: %w", err)
	}
	crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
	if !ok {
		return nil, fmt.Errorf("expected CRD, got %T", obj)
	}
	return crd, nil
}

// Render runs the operator reconcilers against the provided DDA and DAPs using
// a fake client, and returns all Kubernetes resources the operator would
// create. The returned scheme can be used to restore GVK on the collected
// objects (for serialization).
func Render(opts Options) ([]client.Object, *runtime.Scheme, error) {
	if opts.DDA == nil {
		return nil, nil, fmt.Errorf("Options.DDA is required")
	}
	if opts.DDA.Name == "" {
		return nil, nil, fmt.Errorf("DatadogAgent has no name")
	}
	for i, dap := range opts.DAPs {
		if dap == nil {
			return nil, nil, fmt.Errorf("DAP at index %d is nil", i)
		}
		if dap.Name == "" {
			return nil, nil, fmt.Errorf("DAP at index %d has no name", i)
		}
	}

	// Silence all controller-runtime logging
	ctrl.SetLogger(logr.Discard())

	ctx := context.Background()
	scheme := BuildScheme()

	crd, err := loadDDAICRD(scheme)
	if err != nil {
		return nil, nil, err
	}

	// Build fake client pre-populated with DDA, DAPs, and the DDAI CRD.
	// The DDAI CRD is required by newFieldManager() inside reconcileInstanceV3.
	// StatusSubresource registration ensures Status().Update() calls work correctly.
	initObjs := []client.Object{opts.DDA, crd}
	for _, dap := range opts.DAPs {
		initObjs = append(initObjs, dap)
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initObjs...).
		WithStatusSubresource(
			&datadoghqv2alpha1.DatadogAgent{},
			&datadoghqv1alpha1.DatadogAgentInternal{},
		).
		Build()

	recorder := record.NewBroadcaster().NewRecorder(scheme, corev1.EventSource{Component: "operator-renderer"})
	platformInfo := kubernetes.PlatformInfo{} // zero value → modern k8s: policy/v1 PDB, no Cilium unless opted in

	// ── Stage 1: DDA reconciler ─────────────────────────────────────────────
	// Two passes are needed: the first pass adds the finalizer and returns
	// Requeue=true; the second pass does the real work.
	ddaOpts := datadogagent.ReconcilerOptions{
		DatadogAgentInternalEnabled: true,
		DatadogAgentProfileEnabled:  opts.ProfileEnabled,
		SupportCilium:               opts.SupportCilium,
		OperatorMetricsEnabled:      false,
		IntrospectionEnabled:        false,
	}
	ddaReconciler, err := datadogagent.NewReconciler(ddaOpts, fakeClient, platformInfo, scheme, logr.Discard(), recorder, noopForwarder{})
	if err != nil {
		return nil, nil, fmt.Errorf("creating DDA reconciler: %w", err)
	}

	// Pass 1: adds finalizer, returns Requeue=true
	if _, err = ddaReconciler.Reconcile(ctx, opts.DDA); err != nil {
		return nil, nil, fmt.Errorf("DDA reconcile (finalizer pass): %w", err)
	}

	// Re-fetch DDA so it carries the finalizer that was persisted to the fake client
	updatedDDA := &datadoghqv2alpha1.DatadogAgent{}
	if err = fakeClient.Get(ctx, client.ObjectKeyFromObject(opts.DDA), updatedDDA); err != nil {
		return nil, nil, fmt.Errorf("re-fetching DDA after finalizer pass: %w", err)
	}

	// Pass 2: real reconciliation — creates DDAIs and global dependencies
	if _, err = ddaReconciler.Reconcile(ctx, updatedDDA); err != nil {
		return nil, nil, fmt.Errorf("DDA reconcile (main pass): %w", err)
	}

	// ── Stage 2: DDAI reconciler ────────────────────────────────────────────
	// DDAIs were created by the DDA reconciler above.
	// generateDDAIFromDDA pre-sets the DDAI finalizer so no extra pass is needed.
	ddaiList := &datadoghqv1alpha1.DatadogAgentInternalList{}
	if err = fakeClient.List(ctx, ddaiList); err != nil {
		return nil, nil, fmt.Errorf("listing DDAIs: %w", err)
	}
	if len(ddaiList.Items) == 0 {
		return nil, nil, fmt.Errorf("DDA reconcile produced no DatadogAgentInternal objects")
	}

	ddaiOpts := datadogagentinternal.ReconcilerOptions{
		SupportCilium:          opts.SupportCilium,
		OperatorMetricsEnabled: false,
	}
	ddaiReconciler, err := datadogagentinternal.NewReconciler(ddaiOpts, fakeClient, platformInfo, scheme, recorder, noopForwarder{})
	if err != nil {
		return nil, nil, fmt.Errorf("creating DDAI reconciler: %w", err)
	}

	for i := range ddaiList.Items {
		ddai := &ddaiList.Items[i]
		if _, err = ddaiReconciler.Reconcile(ctx, ddai); err != nil {
			return nil, nil, fmt.Errorf("DDAI reconcile %s/%s: %w", ddai.Namespace, ddai.Name, err)
		}
	}

	// ── Stage 3: Collect ────────────────────────────────────────────────────
	resources, err := collectResources(ctx, fakeClient, platformInfo, opts.SupportCilium)
	return resources, scheme, err
}
