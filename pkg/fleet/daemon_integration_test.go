//go:build integration

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
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
	"k8s.io/client-go/tools/record"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
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

// createIntegrationDDA creates a fresh-baseline DatadogAgent against the real
// envtest API server and returns the live object (with a server-assigned
// resourceVersion).
func createIntegrationDDA(t *testing.T, name string) *v2alpha1.DatadogAgent {
	t.Helper()
	dda := testDDAObject("")
	dda.Name = name
	dda.ResourceVersion = ""
	require.NoError(t, integrationClient.Create(context.Background(), dda))

	require.NoError(t, integrationClient.Get(context.Background(), types.NamespacedName{Namespace: dda.Namespace, Name: dda.Name}, dda))
	stampFreshBaseline(dda)
	require.NoError(t, integrationClient.Status().Update(context.Background(), dda))

	t.Cleanup(func() {
		_ = integrationClient.Delete(context.Background(), dda)
	})
	return dda
}

// TestApplyOperation_StaleResourceVersionConflictsAndReplans proves that a
// merge patch naming a stale resourceVersion precondition is rejected by a
// real API server with a 409, and that applyOperation's replan-once recovery
// works against a live apiserver — not just the fake client, which does not
// enforce the resourceVersion precondition embedded in a merge patch body.
func TestApplyOperation_StaleResourceVersionConflictsAndReplans(t *testing.T) {
	dda := createIntegrationDDA(t, "stale-rv-conflict")
	nsn := types.NamespacedName{Namespace: dda.Namespace, Name: dda.Name}

	staleResourceVersion := dda.ResourceVersion

	// Advance the live object's resourceVersion out from under the plan, the
	// same way a concurrent reconcile would.
	live := &v2alpha1.DatadogAgent{}
	require.NoError(t, integrationClient.Get(context.Background(), nsn, live))
	live.Annotations = map[string]string{"unrelated.datadoghq.com/bump": "1"}
	require.NoError(t, integrationClient.Update(context.Background(), live))
	require.NotEqual(t, staleResourceVersion, live.ResourceVersion)

	d := &Daemon{client: integrationClient}

	calls := 0
	plan := func(ctx context.Context) (planResult, error) {
		calls++
		resourceVersion := staleResourceVersion
		if calls > 1 {
			// Replan: read the live object's current resourceVersion.
			current := &v2alpha1.DatadogAgent{}
			if err := integrationClient.Get(ctx, nsn, current); err != nil {
				return planResult{}, err
			}
			resourceVersion = current.ResourceVersion
		}
		return planResult{
			pending:         &pendingOperation{taskID: "task-1", intent: pendingIntentStart, nsn: nsn, experimentID: testExperimentID},
			patch:           []byte(`{}`),
			resourceVersion: resourceVersion,
		}, nil
	}

	pending, err := d.applyOperation(context.Background(), nsn, "test signal", plan)
	require.NoError(t, err)
	require.NotNil(t, pending)
	assert.Equal(t, 2, calls, "the stale resourceVersion must produce exactly one real-apiserver conflict, recovered by one replan")
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

// noopMetricsForwardersManager satisfies datadog.MetricsForwardersManager
// without wiring a real forwarder — the reconciler under test never inspects
// forwarder state, it just needs a non-nil implementation.
type noopMetricsForwardersManager struct{}

func (noopMetricsForwardersManager) Register(client.Object)                    {}
func (noopMetricsForwardersManager) Unregister(client.Object)                  {}
func (noopMetricsForwardersManager) ProcessError(client.Object, error)         {}
func (noopMetricsForwardersManager) ProcessEvent(client.Object, datadog.Event) {}
func (noopMetricsForwardersManager) MetricsForwarderStatusForObj(client.Object) *datadog.ConditionCommon {
	return nil
}
func (noopMetricsForwardersManager) SetEnabledFeatures(client.Object, []string) {}

// TestPlanStartApplyOperation_ProducesReconcilerReadableCheckpoint drives the
// real Fleet daemon (planStart via startDatadogAgentExperiment/applyOperation)
// and the real DatadogAgent Reconciler together against one envtest apiserver,
// to prove the two halves of the system agree on the annotation/status
// contract instead of each being tested in isolation against hand-rolled
// inputs.
func TestPlanStartApplyOperation_ProducesReconcilerReadableCheckpoint(t *testing.T) {
	dda := testutils.NewDatadogAgent(testDDANSN.Namespace, "seam-contract", nil)
	require.NoError(t, integrationClient.Create(context.Background(), dda))
	t.Cleanup(func() { _ = integrationClient.Delete(context.Background(), dda) })

	nsn := types.NamespacedName{Namespace: dda.Namespace, Name: dda.Name}

	// The full DDA reconcile path touches many built-in kinds (NetworkPolicy,
	// PodDisruptionBudget, RBAC, ...) that testFleetScheme() (deliberately
	// minimal — Fleet only ever touches DatadogAgent) does not register. Use
	// the same scheme the reconciler's own integration tests use, against the
	// same envtest apiserver, for the reconciler side of this test.
	reconcilerClient, err := client.New(integrationTestEnv.Config, client.Options{Scheme: reconcilerScheme()})
	require.NoError(t, err)

	r, err := datadogagent.NewReconciler(
		datadogagent.ReconcilerOptions{CreateControllerRevisions: true, APIReader: reconcilerClient},
		reconcilerClient,
		kubernetes.PlatformInfo{},
		reconcilerScheme(),
		logr.Discard(),
		record.NewFakeRecorder(100),
		noopMetricsForwardersManager{},
	)
	require.NoError(t, err)

	// First reconcile: the controller publishes the real current-revision
	// baseline (no stampFreshBaseline shortcut — this is the actual barrier
	// planStart's checkBaselineFreshness will read).
	live := &v2alpha1.DatadogAgent{}
	require.NoError(t, reconcilerClient.Get(context.Background(), nsn, live))
	_, err = r.Reconcile(context.Background(), live)
	require.NoError(t, err)

	require.NoError(t, integrationClient.Get(context.Background(), nsn, live))
	require.NotEmpty(t, live.Status.CurrentRevision, "reconciler must publish a baseline before an experiment can start")
	baselineRevision := live.Status.CurrentRevision

	// Drive the real daemon start path against that baseline.
	d := &Daemon{
		client:           integrationClient,
		apiReader:        integrationClient,
		revisionsEnabled: true,
		configs:          testInstallerConfigWithDDA(),
		statusUpdates:    make(chan ddaStatusSnapshot, 32),
	}
	req := testStartRequest()
	req.Params.NamespacedName = nsn

	pending, err := d.startDatadogAgentExperiment(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, pending)

	afterFleet := &v2alpha1.DatadogAgent{}
	require.NoError(t, integrationClient.Get(context.Background(), nsn, afterFleet))
	assert.Equal(t, v2alpha1.ExperimentSignalStart, afterFleet.Annotations[v2alpha1.AnnotationExperimentSignal])
	assert.Equal(t, testExperimentID, afterFleet.Annotations[v2alpha1.AnnotationExperimentID])
	assert.Equal(t, baselineRevision, afterFleet.Annotations[v2alpha1.AnnotationExperimentRollbackTargetRevision],
		"the rollback-target Fleet wrote must be the baseline the reconciler just published")
	pinnedHash := afterFleet.Annotations[v2alpha1.AnnotationExperimentExpectedSpecHash]
	mergedHash, err := v2alpha1.ComputeSpecHash(afterFleet.Spec, afterFleet.GetAnnotations())
	require.NoError(t, err)
	assert.Equal(t, mergedHash, pinnedHash,
		"Fleet must pin the hash of the merged experiment spec it wrote, in the same patch")
	assert.Equal(t, pending.taskID, afterFleet.Annotations[v2alpha1.AnnotationPendingTaskID])
	assert.Equal(t, string(pendingIntentStart), afterFleet.Annotations[v2alpha1.AnnotationPendingAction])
	assert.Equal(t, testExperimentID, afterFleet.Annotations[v2alpha1.AnnotationPendingExperimentID])
	assert.Equal(t, req.Package, afterFleet.Annotations[v2alpha1.AnnotationPendingPackage])

	// Second reconcile: the controller processes the start signal Fleet wrote.
	_, err = r.Reconcile(context.Background(), afterFleet)
	require.NoError(t, err)

	afterReconcile := &v2alpha1.DatadogAgent{}
	require.NoError(t, integrationClient.Get(context.Background(), nsn, afterReconcile))
	require.NotNil(t, afterReconcile.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, afterReconcile.Status.Experiment.Phase)
	assert.Equal(t, testExperimentID, afterReconcile.Status.Experiment.ID)
	assert.Equal(t, pending.taskID, afterReconcile.Status.Experiment.StartTaskID)
	require.NotNil(t, afterReconcile.Status.Experiment.Checkpoint)
	assert.Equal(t, baselineRevision, afterReconcile.Status.Experiment.Checkpoint.RollbackTargetRevision,
		"the reconciler's checkpoint must agree with the baseline Fleet wrote")
	assert.Equal(t, pinnedHash, afterReconcile.Status.Experiment.Checkpoint.ExpectedSpecHash,
		"the reconciler must copy the hash Fleet pinned, not recompute one of its own")
}

// TestPlanExpectedSpecHash_AccountsForApiserverDefaulting is the pr3443
// Finding 2 regression test. An experiment config that introduces a struct
// with a CRD-defaulted field the caller omitted (here: a container port with
// no protocol, which apiserver defaults to "TCP") makes an in-memory merge
// diverge from the persisted spec. The reconciler hashes the persisted spec,
// so an in-memory pin becomes a false ManualSpecChange abort on the first
// reconcile.
//
// This test asserts the dry-run path in planExpectedSpecHash agrees with the
// hash of the post-apply persisted object. Under the old in-memory design
// (expectedSpecHashAfterMerge on a plain json.Unmarshal) the two would
// differ, and the test would fail.
func TestPlanExpectedSpecHash_AccountsForApiserverDefaulting(t *testing.T) {
	dda := createIntegrationDDA(t, "structural-defaulting")
	nsn := types.NamespacedName{Namespace: dda.Namespace, Name: dda.Name}

	// A config that adds a container port without a protocol -- apiserver's
	// structural defaulting fills in "TCP" during admission.
	config := json.RawMessage(`{"spec":{"override":{"nodeAgent":{"containers":{"agent":{"ports":[{"containerPort":8126,"name":"trace-agent"}]}}}}}}`)

	d := &Daemon{
		client:        integrationClient,
		apiReader:     integrationClient,
		statusUpdates: make(chan ddaStatusSnapshot, 1),
	}
	pinned, err := d.planExpectedSpecHash(context.Background(), dda, testExperimentID, config, "rev-placeholder")
	require.NoError(t, err)

	// Now actually apply the same config so we can compare Fleet's dry-run
	// derived pin against the hash of the persisted object.
	patch, err := BuildStartPatch(dda, testExperimentID, config, "rev-placeholder", pinned)
	require.NoError(t, err)
	require.NoError(t, integrationClient.Patch(context.Background(), dda.DeepCopy(), client.RawPatch(types.MergePatchType, patch)))

	persisted := &v2alpha1.DatadogAgent{}
	require.NoError(t, integrationClient.Get(context.Background(), nsn, persisted))

	// The port must have been TCP-defaulted by apiserver, otherwise this
	// scenario doesn't exercise the bug it claims to.
	require.Len(t, persisted.Spec.Override, 1)
	nodeAgent := persisted.Spec.Override["nodeAgent"]
	require.NotNil(t, nodeAgent)
	require.Contains(t, nodeAgent.Containers, apicommon.CoreAgentContainerName)
	require.Len(t, nodeAgent.Containers["agent"].Ports, 1)
	require.NotNil(t, nodeAgent.Containers["agent"].Ports[0].Protocol,
		"apiserver should have defaulted containerPort.protocol; otherwise this fixture is wrong")
	assert.Equal(t, corev1.ProtocolTCP, nodeAgent.Containers["agent"].Ports[0].Protocol)

	// Reconciler-equivalent computation over the persisted spec + filtered
	// annotations. Fleet's dry-run pin must match this exactly; an in-memory
	// hash would omit the TCP default and fail.
	reconcilerHash, err := v2alpha1.ComputeSpecHash(persisted.Spec, persisted.GetAnnotations())
	require.NoError(t, err)
	assert.Equal(t, reconcilerHash, pinned,
		"dry-run pin must equal the hash the reconciler computes from the persisted (post-defaulting) spec")

	// Confirm the old in-memory helper would have diverged. This locks in
	// the fact that structural defaulting is what matters here -- if this
	// assertion ever inverts, the CRD stopped defaulting protocol or the
	// in-memory helper started reproducing admission, and the whole
	// scenario needs a fresh look.
	inMemory, err := ExpectedSpecHashAfterInMemoryMerge(dda, config)
	require.NoError(t, err)
	assert.NotEqual(t, inMemory, reconcilerHash,
		"scenario invariant: in-memory hash must diverge from the persisted-spec hash, otherwise this test isn't exercising the bug")
}
