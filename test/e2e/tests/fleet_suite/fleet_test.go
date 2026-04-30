// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleetsuite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	operatorcontroller "github.com/DataDog/datadog-operator/internal/controller"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	"github.com/DataDog/datadog-operator/pkg/fleet"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
)

type fleetSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

type fleetHarness struct {
	backend  *fleetBackend
	client   client.Client
	rcClient *fakeRCClient
}

func TestFleetSuite(t *testing.T) {
	e2e.Run(t, &fleetSuite{}, e2e.WithStackName(fmt.Sprintf("operator-fleet-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))), e2e.WithProvisioner(provisioners.KubernetesProvisioner(fleetProvisionerOptions(false)...)))
}

func TestLocalFleetSuite(t *testing.T) {
	e2e.Run(t, &fleetSuite{}, e2e.WithStackName(fmt.Sprintf("operator-local-fleet-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))), e2e.WithProvisioner(provisioners.KubernetesProvisioner(fleetProvisionerOptions(true)...)))
}

func fleetProvisionerOptions(local bool) []provisioners.KubernetesProvisionerOption {
	return []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("fleet-signals"),
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithoutOperator(),
		provisioners.WithoutDDA(),
		provisioners.WithoutFakeIntake(),
		provisioners.WithLocal(local),
	}
}

func (s *fleetSuite) TestFleetSignalStartAndStop() {
	t := s.T()
	ctx := context.Background()

	harness := s.newFleetHarness(0)
	require.NoError(t, createDatadogAgent(ctx, harness.client, "fleet-agent-start-stop"))
	t.Cleanup(func() {
		cleanupDatadogResources(context.Background(), t, harness.client, common.NamespaceName)
	})

	target := types.NamespacedName{Namespace: common.NamespaceName, Name: "fleet-agent-start-stop"}
	waitForControllerRevision(t, harness.client, target)

	experimentConfig := json.RawMessage(`{"spec":{"features":{"logCollection":{"enabled":true}}}}`)
	taskCtx, cancelTask := context.WithTimeout(ctx, 90*time.Second)
	require.NoError(t, harness.backend.StartExperiment(taskCtx, "start-log-collection", "log-collection-enabled", target, experimentConfig))
	cancelTask()

	assertDatadogAgent(t, harness.client, target, func(dda *datadoghqv2alpha1.DatadogAgent) bool {
		return dda.Status.Experiment != nil &&
			dda.Status.Experiment.ID == "start-log-collection" &&
			dda.Status.Experiment.Phase == datadoghqv2alpha1.ExperimentPhaseRunning &&
			dda.Spec.Features != nil &&
			dda.Spec.Features.LogCollection != nil &&
			ptr.Deref(dda.Spec.Features.LogCollection.Enabled, false)
	})
	assertPackageState(t, harness.rcClient, pbgo.TaskState_DONE, "0.0.1", "log-collection-enabled")

	taskCtx, cancelTask = context.WithTimeout(ctx, 90*time.Second)
	err := harness.backend.SendTaskWithExpectedState(taskCtx, "stale-start", methodStartDatadogAgentExperiment, "log-collection-enabled", target, expectedState{StableConfig: "stale", ExperimentConfig: "log-collection-enabled"})
	cancelTask()
	require.Error(t, err)
	assertPackageState(t, harness.rcClient, pbgo.TaskState_INVALID_STATE, "0.0.1", "log-collection-enabled")

	taskCtx, cancelTask = context.WithTimeout(ctx, 90*time.Second)
	require.NoError(t, harness.backend.StopExperiment(taskCtx, "stop-log-collection", "log-collection-enabled", target))
	cancelTask()
	assertDatadogAgent(t, harness.client, target, func(dda *datadoghqv2alpha1.DatadogAgent) bool {
		return dda.Status.Experiment != nil &&
			dda.Status.Experiment.ID == "start-log-collection" &&
			dda.Status.Experiment.Phase == datadoghqv2alpha1.ExperimentPhaseTerminated &&
			dda.Status.Experiment.TerminationReason == datadogagent.ExperimentTerminationReasonStopped &&
			(dda.Spec.Features == nil || dda.Spec.Features.LogCollection == nil || !ptr.Deref(dda.Spec.Features.LogCollection.Enabled, false))
	})
	assertPackageState(t, harness.rcClient, pbgo.TaskState_DONE, "0.0.1", "")
}

func (s *fleetSuite) TestFleetSignalStartAndPromote() {
	t := s.T()
	ctx := context.Background()

	harness := s.newFleetHarness(0)
	require.NoError(t, createDatadogAgent(ctx, harness.client, "fleet-agent-start-promote"))
	t.Cleanup(func() {
		cleanupDatadogResources(context.Background(), t, harness.client, common.NamespaceName)
	})

	target := types.NamespacedName{Namespace: common.NamespaceName, Name: "fleet-agent-start-promote"}
	waitForControllerRevision(t, harness.client, target)

	experimentConfig := json.RawMessage(`{"spec":{"features":{"logCollection":{"enabled":true}}}}`)
	taskCtx, cancelTask := context.WithTimeout(ctx, 90*time.Second)
	require.NoError(t, harness.backend.StartExperiment(taskCtx, "start-promote-log-collection", "promote-log-collection-enabled", target, experimentConfig))
	cancelTask()
	assertDatadogAgent(t, harness.client, target, func(dda *datadoghqv2alpha1.DatadogAgent) bool {
		return dda.Status.Experiment != nil &&
			dda.Status.Experiment.ID == "start-promote-log-collection" &&
			dda.Status.Experiment.Phase == datadoghqv2alpha1.ExperimentPhaseRunning &&
			dda.Spec.Features != nil &&
			dda.Spec.Features.LogCollection != nil &&
			ptr.Deref(dda.Spec.Features.LogCollection.Enabled, false)
	})
	assertPackageState(t, harness.rcClient, pbgo.TaskState_DONE, "0.0.1", "promote-log-collection-enabled")

	taskCtx, cancelTask = context.WithTimeout(ctx, 90*time.Second)
	require.NoError(t, harness.backend.PromoteExperiment(taskCtx, "promote-log-collection", "promote-log-collection-enabled", target))
	cancelTask()
	assertDatadogAgent(t, harness.client, target, func(dda *datadoghqv2alpha1.DatadogAgent) bool {
		return dda.Status.Experiment != nil &&
			dda.Status.Experiment.ID == "start-promote-log-collection" &&
			dda.Status.Experiment.Phase == datadoghqv2alpha1.ExperimentPhasePromoted &&
			dda.Spec.Features != nil &&
			dda.Spec.Features.LogCollection != nil &&
			ptr.Deref(dda.Spec.Features.LogCollection.Enabled, false)
	})
	assertPackageState(t, harness.rcClient, pbgo.TaskState_DONE, "promote-log-collection-enabled", "")
}

func (s *fleetSuite) TestFleetExperimentTimeoutRollback() {
	t := s.T()
	ctx := context.Background()

	const experimentTimeout = 500 * time.Millisecond
	harness := s.newFleetHarness(experimentTimeout)
	require.NoError(t, createDatadogAgent(ctx, harness.client, "fleet-agent-timeout"))
	t.Cleanup(func() {
		cleanupDatadogResources(context.Background(), t, harness.client, common.NamespaceName)
	})

	target := types.NamespacedName{Namespace: common.NamespaceName, Name: "fleet-agent-timeout"}
	waitForControllerRevision(t, harness.client, target)

	experimentConfig := json.RawMessage(`{"spec":{"features":{"logCollection":{"enabled":true}}}}`)
	taskCtx, cancelTask := context.WithTimeout(ctx, 90*time.Second)
	require.NoError(t, harness.backend.StartExperiment(taskCtx, "start-timeout-log-collection", "timeout-log-collection-enabled", target, experimentConfig))
	cancelTask()
	waitForControllerRevisionCount(t, harness.client, target, 2)

	time.Sleep(2 * experimentTimeout)
	require.NoError(t, triggerDatadogAgentReconcile(ctx, harness.client, target))

	assertDatadogAgent(t, harness.client, target, func(dda *datadoghqv2alpha1.DatadogAgent) bool {
		return dda.Status.Experiment != nil &&
			dda.Status.Experiment.ID == "start-timeout-log-collection" &&
			dda.Status.Experiment.Phase == datadoghqv2alpha1.ExperimentPhaseTerminated &&
			dda.Status.Experiment.TerminationReason == datadogagent.ExperimentTerminationReasonTimedOut &&
			(dda.Spec.Features == nil || dda.Spec.Features.LogCollection == nil || !ptr.Deref(dda.Spec.Features.LogCollection.Enabled, false))
	})
}

func (s *fleetSuite) newFleetHarness(experimentTimeout time.Duration) *fleetHarness {
	t := s.T()
	ctx := context.Background()

	scheme := fleetScheme(t)
	k8sConfig := rest.CopyConfig(s.Env().KubernetesCluster.KubernetesClient.K8sConfig)
	mgr, err := ctrl.NewManager(k8sConfig, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Controller:             ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
	})
	require.NoError(t, err)

	platformInfo := platformInfo(t, rest.CopyConfig(k8sConfig))
	setupDatadogAgentController(t, mgr, platformInfo, experimentTimeout)

	rc := newFakeRCClient([]*pbgo.PackageState{{
		Package:             operatorPackage,
		StableConfigVersion: "0.0.1",
	}})
	require.NoError(t, mgr.Add(fleet.NewDaemon(rc, mgr, true)))

	mgrCtx, cancelMgr := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(mgrCtx)
	}()
	t.Cleanup(func() {
		cancelMgr()
		require.NoError(t, <-errCh)
	})

	syncCtx, cancelSync := context.WithTimeout(ctx, 30*time.Second)
	defer cancelSync()
	require.True(t, mgr.GetCache().WaitForCacheSync(syncCtx))

	return &fleetHarness{
		backend:  newFleetBackend(rc),
		client:   mgr.GetClient(),
		rcClient: rc,
	}
}

func fleetScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, apiregistrationv1.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))
	require.NoError(t, datadoghqv2alpha1.AddToScheme(scheme))
	require.NoError(t, storagev1.AddToScheme(scheme))
	return scheme
}

func setupDatadogAgentController(t *testing.T, mgr ctrl.Manager, platformInfo kubernetes.PlatformInfo, experimentTimeout time.Duration) {
	t.Helper()

	reconciler := &operatorcontroller.DatadogAgentReconciler{
		Client:       mgr.GetClient(),
		PlatformInfo: platformInfo,
		Log:          ctrl.Log.WithName("controllers").WithName("DatadogAgent"),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor("DatadogAgent"),
		Options: datadogagent.ReconcilerOptions{
			DatadogAgentInternalEnabled: true,
			CreateControllerRevisions:   true,
			ExperimentTimeout:           experimentTimeout,
		},
	}
	require.NoError(t, reconciler.SetupWithManager(mgr, nil))
}

func platformInfo(t *testing.T, config *rest.Config) kubernetes.PlatformInfo {
	t.Helper()

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	require.NoError(t, err)
	versionInfo, err := discoveryClient.ServerVersion()
	require.NoError(t, err)
	groups, resources, err := discoveryClient.ServerGroupsAndResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		require.NoError(t, err)
	}
	return kubernetes.NewPlatformInfo(versionInfo, groups, resources)
}

func createDatadogAgent(ctx context.Context, k8sClient client.Client, name string) error {
	dda := testutils.NewDatadogAgent(common.NamespaceName, name, nil)
	dda.Spec.Global.Kubelet = &datadoghqv2alpha1.KubeletConfig{TLSVerify: ptr.To(false)}
	return k8sClient.Create(ctx, dda)
}

func waitForControllerRevision(t *testing.T, k8sClient client.Client, nsn types.NamespacedName) {
	t.Helper()

	waitForControllerRevisionCount(t, k8sClient, nsn, 1)
}

func waitForControllerRevisionCount(t *testing.T, k8sClient client.Client, nsn types.NamespacedName, count int) {
	t.Helper()

	require.Eventually(t, func() bool {
		revisions := &appsv1.ControllerRevisionList{}
		err := k8sClient.List(context.Background(), revisions, client.InNamespace(nsn.Namespace), client.MatchingLabels{apicommon.DatadogAgentNameLabelKey: nsn.Name})
		return err == nil && len(revisions.Items) >= count
	}, time.Minute, time.Second)
}

func triggerDatadogAgentReconcile(ctx context.Context, k8sClient client.Client, nsn types.NamespacedName) error {
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"experiment.datadoghq.com/reconcile-trigger":"%d"}}}`, time.Now().UnixNano()))
	dda := &datadoghqv2alpha1.DatadogAgent{}
	dda.Name = nsn.Name
	dda.Namespace = nsn.Namespace
	return k8sClient.Patch(ctx, dda, client.RawPatch(types.MergePatchType, patch))
}

func assertDatadogAgent(t *testing.T, k8sClient client.Client, nsn types.NamespacedName, check func(*datadoghqv2alpha1.DatadogAgent) bool) {
	t.Helper()

	require.Eventually(t, func() bool {
		dda := &datadoghqv2alpha1.DatadogAgent{}
		if err := k8sClient.Get(context.Background(), nsn, dda); err != nil {
			return false
		}
		return check(dda)
	}, 2*time.Minute, time.Second)
}

func assertPackageState(t *testing.T, rc *fakeRCClient, taskState pbgo.TaskState, stableConfig string, experimentConfig string) {
	t.Helper()

	pkg := rc.packageState(operatorPackage)
	require.NotNil(t, pkg)
	require.NotNil(t, pkg.GetTask())
	assert.Equal(t, taskState, pkg.GetTask().GetState())
	assert.Equal(t, stableConfig, pkg.GetStableConfigVersion())
	assert.Equal(t, experimentConfig, pkg.GetExperimentConfigVersion())
}

func cleanupDatadogResources(ctx context.Context, t *testing.T, k8sClient client.Client, namespace string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	ddaList := &datadoghqv2alpha1.DatadogAgentList{}
	if err := k8sClient.List(ctx, ddaList, client.InNamespace(namespace)); err == nil {
		for i := range ddaList.Items {
			dda := &ddaList.Items[i]
			_ = k8sClient.Patch(ctx, dda, client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"finalizers":[]}}`)))
			_ = k8sClient.Delete(ctx, dda)
		}
	}

	ddaiList := &datadoghqv1alpha1.DatadogAgentInternalList{}
	if err := k8sClient.List(ctx, ddaiList, client.InNamespace(namespace)); err == nil {
		for i := range ddaiList.Items {
			ddai := &ddaiList.Items[i]
			_ = k8sClient.Patch(ctx, ddai, client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"finalizers":[]}}`)))
			_ = k8sClient.Delete(ctx, ddai)
		}
	}

	revisions := &appsv1.ControllerRevisionList{}
	if err := k8sClient.List(ctx, revisions, client.InNamespace(namespace)); err == nil {
		for i := range revisions.Items {
			if _, ok := revisions.Items[i].Labels[apicommon.DatadogAgentNameLabelKey]; ok {
				_ = k8sClient.Delete(ctx, &revisions.Items[i])
			}
		}
	}
}
