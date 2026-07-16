// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	operatorremoteconfig "github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

var testManagedAgentInstallationIdentity = operatorremoteconfig.ManagedAgentInstallationIdentity{
	InstallationID: "123e4567-e89b-42d3-a456-426614174000",
	TargetHash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
}

const testRCClientID = "operator-client-id"

func testUninstallFenceConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       uninstallFenceKey.Namespace,
			Name:            uninstallFenceKey.Name,
			ResourceVersion: "1",
		},
		Data: map[string]string{
			uninstallFenceStateKey:                  uninstallFenceStateInactive,
			uninstallFenceWebhookResourceVersionKey: "2",
		},
	}
}

func testUninstallFenceWebhookConfiguration() *admissionregistrationv1.ValidatingWebhookConfiguration {
	failurePolicy := admissionregistrationv1.Fail
	path := uninstallFenceAdmissionPath
	sideEffects := admissionregistrationv1.SideEffectClassNone
	return &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:            uninstallFenceWebhookConfigurationName,
			ResourceVersion: "2",
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			Name:          uninstallFenceWebhookName,
			FailurePolicy: &failurePolicy,
			SideEffects:   &sideEffects,
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				CABundle: []byte("ca"),
				Service: &admissionregistrationv1.ServiceReference{
					Namespace: uninstallFenceWebhookDefaultNamespace,
					Name:      uninstallFenceWebhookServiceName,
					Path:      &path,
				},
			},
			Rules: []admissionregistrationv1.RuleWithOperations{
				{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
					Rule: admissionregistrationv1.Rule{
						APIGroups:   []string{v2alpha1.GroupVersion.Group},
						APIVersions: []string{v2alpha1.GroupVersion.Version},
						Resources:   []string{"datadogagents"},
					},
				},
				{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
					Rule: admissionregistrationv1.Rule{
						APIGroups:   []string{v1alpha1.GroupVersion.Group},
						APIVersions: []string{v1alpha1.GroupVersion.Version},
						Resources:   []string{"datadogagentprofiles"},
					},
				},
			},
		}},
	}
}

type transientGetReader struct {
	client.Reader
	failures int
}

type autoReadyManagedAgentInstallationClient struct {
	client.Client
}

type assignUIDManagedAgentInstallationClient struct {
	client.Client
}

func (c *assignUIDManagedAgentInstallationClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if dda, ok := obj.(*v2alpha1.DatadogAgent); ok && dda.UID == "" {
		dda.UID = types.UID("created-dda-uid")
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *autoReadyManagedAgentInstallationClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if dda, ok := obj.(*v2alpha1.DatadogAgent); ok && dda.UID == "" {
		dda.UID = types.UID("created-dda-uid")
	}
	if profile, ok := obj.(*v1alpha1.DatadogAgentProfile); ok {
		profile.Status.Valid = metav1.ConditionTrue
		profile.Status.Applied = metav1.ConditionTrue
		profile.Status.CreateStrategy = &v1alpha1.CreateStrategy{Status: v1alpha1.CompletedStatus}
	}
	if err := c.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}
	created, ok := obj.(*v2alpha1.DatadogAgent)
	if ok {
		current := &v2alpha1.DatadogAgent{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(created), current); err != nil {
			return err
		}
		setTestDatadogAgentReady(current, false)
		return c.Client.Status().Update(ctx, current)
	}
	if _, ok := obj.(*v1alpha1.DatadogAgentProfile); !ok {
		return nil
	}
	current := &v2alpha1.DatadogAgent{}
	if err := c.Client.Get(ctx, testDDANSN, current); err != nil {
		return err
	}
	setTestDatadogAgentReady(current, true)
	return c.Client.Status().Update(ctx, current)
}

func setTestDatadogAgentReady(dda *v2alpha1.DatadogAgent, includeWindows bool) {
	dda.Status.Conditions = []metav1.Condition{{
		Type:               datadogAgentReconcileErrorCondition,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: dda.Generation,
		Reason:             "DatadogAgent_reconcile_ok",
		LastTransitionTime: metav1.Now(),
	}}
	base := &v2alpha1.DaemonSetStatus{Desired: 1, Current: 1, Ready: 1, Available: 1, UpToDate: 1, DaemonsetName: fleetDatadogAgentName}
	dda.Status.Agent = base.DeepCopy()
	dda.Status.AgentList = []*v2alpha1.DaemonSetStatus{base}
	if includeWindows {
		dda.Status.AgentList = append(dda.Status.AgentList, &v2alpha1.DaemonSetStatus{DaemonsetName: managedAgentInstallationWindowsProfileName + "-agent"})
	}
	dda.Status.ClusterAgent = &v2alpha1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}
}

func TestFleetDatadogAgentReadinessWithWindowsAgent(t *testing.T) {
	tests := []struct {
		name        string
		windows     *v2alpha1.DaemonSetStatus
		wantReady   bool
		observation string
	}{
		{
			name:      "zero desired Windows agents",
			windows:   &v2alpha1.DaemonSetStatus{},
			wantReady: true,
		},
		{
			name:      "ready Windows agents",
			windows:   &v2alpha1.DaemonSetStatus{Desired: 2, Current: 2, Ready: 2, Available: 2, UpToDate: 2},
			wantReady: true,
		},
		{
			name:        "unready Windows agents",
			windows:     &v2alpha1.DaemonSetStatus{Desired: 2, Current: 2, Ready: 1, Available: 1, UpToDate: 2},
			observation: `Agent DaemonSet "windows-agent" is not ready`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid := types.UID("dda-uid")
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{UID: uid, Generation: 3}}
			setTestDatadogAgentReady(dda, false)
			tt.windows.DaemonsetName = "windows-agent"
			dda.Status.AgentList = append(dda.Status.AgentList, tt.windows)

			ready, observation, err := fleetDatadogAgentReadiness(dda, uid)

			require.NoError(t, err)
			assert.Equal(t, tt.wantReady, ready)
			assert.Equal(t, tt.observation, observation)
		})
	}
}

type lateDDAICreateOnDDADeleteClient struct {
	client.Client
	late    *v1alpha1.DatadogAgentInternal
	created bool
}

type ownershipChangingDeleteClient struct {
	client.Client
	changed bool
}

type concurrentUnmanagedDatadogAgentClient struct {
	client.Client
	unmanaged *v2alpha1.DatadogAgent
	created   bool
}

type recreateFleetDatadogAgentOnFinalListClient struct {
	client.Client
	replacement *v2alpha1.DatadogAgent
	ddaLists    int
}

func (c *recreateFleetDatadogAgentOnFinalListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*v2alpha1.DatadogAgentList); ok {
		c.ddaLists++
		if c.ddaLists == 3 {
			if err := c.Client.Create(ctx, c.replacement.DeepCopy()); err != nil {
				return err
			}
		}
	}
	return c.Client.List(ctx, list, opts...)
}

type unmanagedDatadogAgentOnReadyPatchClient struct {
	client.Client
	unmanaged *v2alpha1.DatadogAgent
	created   bool
}

type replaceDatadogAgentOnReadyPatchClient struct {
	client.Client
	replacement *v2alpha1.DatadogAgent
	replaced    bool
}

type invalidateReadinessOnReadyPatchClient struct {
	client.Client
	changed bool
}

type replaceDatadogAgentAfterFirstGetClient struct {
	client.Client
	replacement *v2alpha1.DatadogAgent
	replaced    bool
}

type driftDatadogAgentAfterReadyGetClient struct {
	client.Client
	mutated bool
}

type mutateDatadogAgentAfterFirstGetClient struct {
	client.Client
	mutate  func(*v2alpha1.DatadogAgent)
	mutated bool
}

func (c *mutateDatadogAgentAfterFirstGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := c.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	dda, ok := obj.(*v2alpha1.DatadogAgent)
	if !ok || c.mutated || key != testDDANSN {
		return nil
	}
	mutated := dda.DeepCopy()
	c.mutate(mutated)
	if err := c.Client.Update(ctx, mutated); err != nil {
		return err
	}
	c.mutated = true
	return nil
}

func (c *driftDatadogAgentAfterReadyGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := c.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	dda, ok := obj.(*v2alpha1.DatadogAgent)
	if !ok || c.mutated || dda.Labels[fleetManagedAgentInstallationStateLabel] != fleetManagedAgentInstallationStateReady {
		return nil
	}
	if _, pending := pendingOperationFromAnnotations(client.ObjectKeyFromObject(dda), dda.Annotations); !pending {
		return nil
	}
	drifted := dda.DeepCopy()
	site := "datadoghq.eu"
	drifted.Spec.Global.Site = &site
	if err := c.Client.Update(ctx, drifted); err != nil {
		return err
	}
	c.mutated = true
	return c.Client.Get(ctx, key, obj, opts...)
}

type unmanagedDatadogAgentOnDDADeleteClient struct {
	client.Client
	unmanaged *v2alpha1.DatadogAgent
	created   bool
}

type failDatadogAgentListClient struct {
	client.Client
	failOn int
	lists  int
}

type mutateDatadogAgentAfterConflictListClient struct {
	client.Client
	changed bool
}

type datadogAgentInternalVersionChangingDeleteClient struct {
	client.Client
	changed bool
}

type persistThenErrorCreateClient struct {
	client.Client
	failed      bool
	defaultSpec bool
	unmanaged   *v2alpha1.DatadogAgent
}

type recoverForeignCreateClient struct {
	client.Client
	recovered *v2alpha1.DatadogAgent
	unmanaged *v2alpha1.DatadogAgent
	created   bool
}

func (c *recoverForeignCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if _, ok := obj.(*v2alpha1.DatadogAgent); !ok || c.created {
		return c.Client.Create(ctx, obj, opts...)
	}
	c.created = true
	if err := c.Client.Create(ctx, c.recovered.DeepCopy()); err != nil {
		return err
	}
	if err := c.Client.Create(ctx, c.unmanaged.DeepCopy()); err != nil {
		return err
	}
	return fmt.Errorf("connection reset while another actor created the target")
}

func (c *persistThenErrorCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := c.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}
	if _, ok := obj.(*v2alpha1.DatadogAgent); ok && !c.failed {
		c.failed = true
		if c.defaultSpec {
			current := &v2alpha1.DatadogAgent{}
			if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
				return err
			}
			site := "datadoghq.com"
			current.Spec.Global.Site = &site
			if err := c.Client.Update(ctx, current); err != nil {
				return err
			}
		}
		if c.unmanaged != nil {
			if err := c.Client.Create(ctx, c.unmanaged.DeepCopy()); err != nil {
				return err
			}
		}
		return fmt.Errorf("connection reset after create")
	}
	return nil
}

type failExperimentHashPatchClient struct {
	client.Client
	failed bool
}

func (c *failExperimentHashPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	dda, isDDA := obj.(*v2alpha1.DatadogAgent)
	if !c.failed && isDDA && dda.Annotations[fleetExperimentHashAnnotation] != "" {
		c.failed = true
		return fmt.Errorf("experiment hash patch failed")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

type driftBeforeExperimentPatchClient struct {
	client.Client
	changed bool
}

type replaceBeforeExperimentPatchClient struct {
	client.Client
	replacement *v2alpha1.DatadogAgent
	changed     bool
}

type persistThenErrorExperimentPatchClient struct {
	client.Client
	failed bool
}

type consumeThenErrorExperimentPatchClient struct {
	client.Client
	failed bool
	phase  v2alpha1.ExperimentPhase
}

func (c *consumeThenErrorExperimentPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	if !c.failed && bytes.Contains(data, []byte(v2alpha1.AnnotationExperimentSignal)) {
		if err := c.Client.Patch(ctx, obj, patch, opts...); err != nil {
			return err
		}
		current := &v2alpha1.DatadogAgent{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
			return err
		}
		delete(current.Annotations, v2alpha1.AnnotationExperimentSignal)
		delete(current.Annotations, v2alpha1.AnnotationExperimentID)
		if err := c.Client.Update(ctx, current); err != nil {
			return err
		}
		phase := c.phase
		if phase == "" {
			phase = v2alpha1.ExperimentPhaseRunning
		}
		current.Status.Experiment = &v2alpha1.ExperimentStatus{ID: "test-config", Phase: phase}
		if isTerminalPhase(phase) {
			current.Status.Experiment.TerminationReason = "timed_out"
			current.Status.Experiment.StartTaskID = "exp-abc"
		}
		if err := c.Client.Status().Update(ctx, current); err != nil {
			return err
		}
		c.failed = true
		return fmt.Errorf("connection reset after consumed experiment patch")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

type replacePendingBeforeCleanupClient struct {
	client.Client
	replacementTaskID string
	changed           bool
}

type advanceExperimentAfterGetClient struct {
	client.Client
	next     *v2alpha1.ExperimentStatus
	advanced bool
}

func (c *advanceExperimentAfterGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := c.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	if c.advanced || key != testDDANSN {
		return nil
	}
	if _, ok := obj.(*v2alpha1.DatadogAgent); !ok {
		return nil
	}
	c.advanced = true
	current := &v2alpha1.DatadogAgent{}
	if err := c.Client.Get(ctx, key, current); err != nil {
		return err
	}
	current.Status.Experiment = c.next.DeepCopy()
	return c.Client.Status().Update(ctx, current)
}

func (c *replacePendingBeforeCleanupClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	dda, ok := obj.(*v2alpha1.DatadogAgent)
	if !c.changed && ok && dda.Annotations[v2alpha1.AnnotationPendingTaskID] == "" {
		c.changed = true
		current := &v2alpha1.DatadogAgent{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
			return err
		}
		current.Annotations[v2alpha1.AnnotationPendingTaskID] = c.replacementTaskID
		if err := c.Client.Update(ctx, current); err != nil {
			return err
		}
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

type replaceDDABeforeCleanupClient struct {
	client.Client
	replacement *v2alpha1.DatadogAgent
	changed     bool
}

type transientPendingCleanupPatchClient struct {
	client.Client
	failures int
}

type failPartialManagedAgentInstallationPatchClient struct {
	client.Client
	failures int
}

type failReadyManagedAgentInstallationPatchClient struct {
	client.Client
	failures int
}

type rehydrateOnPartialManagedAgentInstallationPatchClient struct {
	client.Client
	rehydrate    func() error
	rehydrateErr error
	called       bool
}

func (c *rehydrateOnPartialManagedAgentInstallationPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	if err := c.Client.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}
	if !c.called && bytes.Contains(data, []byte(fleetManagedAgentInstallationStateLabel)) && bytes.Contains(data, []byte(fleetManagedAgentInstallationStatePartial)) {
		c.called = true
		c.rehydrateErr = c.rehydrate()
	}
	return nil
}

func (c *failReadyManagedAgentInstallationPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	if c.failures > 0 && bytes.Contains(data, []byte(fleetManagedAgentInstallationStateLabel)) && bytes.Contains(data, []byte(fleetManagedAgentInstallationStateReady)) {
		c.failures--
		return fmt.Errorf("connection reset before ready managed Agent installation patch committed")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *failPartialManagedAgentInstallationPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	if c.failures > 0 && bytes.Contains(data, []byte(fleetManagedAgentInstallationStateLabel)) && bytes.Contains(data, []byte(fleetManagedAgentInstallationStatePartial)) {
		c.failures--
		return fmt.Errorf("connection reset before partial managed Agent installation patch committed")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *transientPendingCleanupPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	if c.failures > 0 && bytes.Contains(data, []byte(v2alpha1.AnnotationPendingTaskID)) {
		c.failures--
		return fmt.Errorf("connection reset before pending cleanup patch committed")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *replaceDDABeforeCleanupClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	dda, ok := obj.(*v2alpha1.DatadogAgent)
	if !c.changed && ok && dda.Annotations[v2alpha1.AnnotationPendingTaskID] == "" {
		c.changed = true
		current := &v2alpha1.DatadogAgent{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
			return err
		}
		if err := c.Client.Delete(ctx, current); err != nil {
			return err
		}
		if err := c.Client.Create(ctx, c.replacement.DeepCopy()); err != nil {
			return err
		}
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *persistThenErrorExperimentPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	if !c.failed && bytes.Contains(data, []byte(v2alpha1.AnnotationExperimentSignal)) {
		if err := c.Client.Patch(ctx, obj, patch, opts...); err != nil {
			return err
		}
		c.failed = true
		return fmt.Errorf("connection reset after experiment patch")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *driftBeforeExperimentPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	if !c.changed && bytes.Contains(data, []byte(v2alpha1.AnnotationExperimentSignal)) {
		c.changed = true
		current := &v2alpha1.DatadogAgent{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
			return err
		}
		site := "datadoghq.eu"
		current.Spec.Global.Site = &site
		if err := c.Client.Update(ctx, current); err != nil {
			return err
		}
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *replaceBeforeExperimentPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	if !c.changed && bytes.Contains(data, []byte(v2alpha1.AnnotationExperimentSignal)) {
		c.changed = true
		current := &v2alpha1.DatadogAgent{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
			return err
		}
		if err := c.Client.Delete(ctx, current); err != nil {
			return err
		}
		if err := c.Client.Create(ctx, c.replacement.DeepCopy()); err != nil {
			return err
		}
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *ownershipChangingDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if _, ok := obj.(*v2alpha1.DatadogAgent); ok && !c.changed {
		c.changed = true
		current := &v2alpha1.DatadogAgent{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
			return err
		}
		delete(current.Labels, fleetManagedByLabel)
		if err := c.Client.Update(ctx, current); err != nil {
			return err
		}
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func (c *concurrentUnmanagedDatadogAgentClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := c.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}
	if _, ok := obj.(*v2alpha1.DatadogAgent); ok && !c.created {
		c.created = true
		return c.Client.Create(ctx, c.unmanaged.DeepCopy())
	}
	return nil
}

func (c *unmanagedDatadogAgentOnReadyPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if err := c.Client.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}
	dda, ok := obj.(*v2alpha1.DatadogAgent)
	if !ok || c.created || dda.Labels[fleetManagedAgentInstallationStateLabel] != fleetManagedAgentInstallationStateReady {
		return nil
	}
	c.created = true
	return c.Client.Create(ctx, c.unmanaged.DeepCopy())
}

func (c *replaceDatadogAgentOnReadyPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if err := c.Client.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}
	dda, ok := obj.(*v2alpha1.DatadogAgent)
	if !ok || c.replaced || dda.Labels[fleetManagedAgentInstallationStateLabel] != fleetManagedAgentInstallationStateReady {
		return nil
	}
	c.replaced = true
	if err := c.Client.Delete(ctx, dda); err != nil {
		return err
	}
	return c.Client.Create(ctx, c.replacement.DeepCopy())
}

func (c *invalidateReadinessOnReadyPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if err := c.Client.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}
	dda, ok := obj.(*v2alpha1.DatadogAgent)
	if !ok || c.changed || dda.Labels[fleetManagedAgentInstallationStateLabel] != fleetManagedAgentInstallationStateReady {
		return nil
	}
	current := &v2alpha1.DatadogAgent{}
	if err := c.Client.Get(ctx, client.ObjectKeyFromObject(dda), current); err != nil {
		return err
	}
	condition := meta.FindStatusCondition(current.Status.Conditions, datadogAgentReconcileErrorCondition)
	if condition == nil {
		return fmt.Errorf("missing reconcile condition")
	}
	condition.Status = metav1.ConditionTrue
	condition.ObservedGeneration = current.Generation
	condition.Reason = "DatadogAgent_reconcile_error"
	condition.Message = "injected reconcile failure"
	c.changed = true
	return c.Client.Status().Update(ctx, current)
}

func (c *replaceDatadogAgentAfterFirstGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := c.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	dda, ok := obj.(*v2alpha1.DatadogAgent)
	if !ok || c.replaced || key != testDDANSN {
		return nil
	}
	c.replaced = true
	if err := c.Client.Delete(ctx, dda.DeepCopy()); err != nil {
		return err
	}
	return c.Client.Create(ctx, c.replacement.DeepCopy())
}

func (c *unmanagedDatadogAgentOnDDADeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if err := c.Client.Delete(ctx, obj, opts...); err != nil {
		return err
	}
	if _, ok := obj.(*v2alpha1.DatadogAgent); !ok || c.created {
		return nil
	}
	c.created = true
	return c.Client.Create(ctx, c.unmanaged.DeepCopy())
}

func (c *failDatadogAgentListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*v2alpha1.DatadogAgentList); ok {
		c.lists++
		if c.lists == c.failOn {
			return fmt.Errorf("DatadogAgent list failed")
		}
	}
	return c.Client.List(ctx, list, opts...)
}

func (c *mutateDatadogAgentAfterConflictListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err := c.Client.List(ctx, list, opts...); err != nil {
		return err
	}
	ddas, ok := list.(*v2alpha1.DatadogAgentList)
	if !ok || c.changed || len(ddas.Items) < 2 {
		return nil
	}
	current := &v2alpha1.DatadogAgent{}
	if err := c.Client.Get(ctx, testDDANSN, current); err != nil {
		return err
	}
	if current.Annotations == nil {
		current.Annotations = make(map[string]string)
	}
	current.Annotations["test.datadoghq.com/concurrent-change"] = "true"
	if err := c.Client.Update(ctx, current); err != nil {
		return err
	}
	c.changed = true
	return nil
}

func (c *datadogAgentInternalVersionChangingDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if _, ok := obj.(*v1alpha1.DatadogAgentInternal); ok && !c.changed {
		c.changed = true
		current := &v1alpha1.DatadogAgentInternal{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
			return err
		}
		if current.Annotations == nil {
			current.Annotations = make(map[string]string)
		}
		current.Annotations["test.datadoghq.com/status-write"] = "1"
		if err := c.Client.Update(ctx, current); err != nil {
			return err
		}
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func (c *lateDDAICreateOnDDADeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := c.Client.Delete(ctx, obj, opts...)
	if err != nil || c.created {
		return err
	}
	if _, ok := obj.(*v2alpha1.DatadogAgent); !ok {
		return nil
	}
	c.created = true
	return c.Client.Create(ctx, c.late.DeepCopy())
}

func (r *transientGetReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if r.failures > 0 {
		r.failures--
		return fmt.Errorf("transient API read failure")
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

func testManagedAgentInstallationDaemon(configs map[string]installerConfig, rcState []*pbgo.PackageState, objects ...client.Object) (*Daemon, client.Client, *mockRCClient) {
	hasFence := false
	for _, object := range objects {
		if client.ObjectKeyFromObject(object) == uninstallFenceKey {
			hasFence = true
			break
		}
	}
	if !hasFence {
		objects = append(objects, testUninstallFenceConfigMap())
	}
	for _, object := range objects {
		ddai, ok := object.(*v1alpha1.DatadogAgentInternal)
		if !ok || ddai.Labels[fleetManagedByLabel] != fleetManagedByValue {
			continue
		}
		if ddai.Labels == nil {
			ddai.Labels = make(map[string]string)
		}
		ddai.Labels[fleetInstallationIDLabel] = testManagedAgentInstallationIdentity.InstallationID
		ddai.Labels[fleetTargetIDLabel] = testManagedAgentInstallationIdentity.TargetID()
	}
	objects = append(objects, testUninstallFenceWebhookConfiguration())
	scheme := testFleetScheme()
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = admissionregistrationv1.AddToScheme(scheme)
	_ = apiregistrationv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v2alpha1.DatadogAgent{}).
		WithObjects(objects...).
		Build()
	c := &autoReadyManagedAgentInstallationClient{Client: baseClient}
	rc := &mockRCClient{state: rcState, clientID: testRCClientID}
	d := &Daemon{
		rcClient:                         rc,
		client:                           c,
		apiReader:                        c,
		revisionsEnabled:                 false,
		managedAgentInstallationIdentity: testManagedAgentInstallationIdentity,
		fenceVerifier:                    func(context.Context, *corev1.ConfigMap) error { return nil },
		fenceModeManager:                 func(context.Context, bool) error { return nil },
		configs:                          configs,
		statusUpdates:                    make(chan ddaStatusSnapshot, 32),
	}
	return d, c, rc
}

func testManagedAgentInstallationInstallerConfig(id string, operation Operation, config string) map[string]installerConfig {
	return map[string]installerConfig{
		id: {
			ID: id,
			Operations: []fleetManagementOperation{
				{Operation: operation, Config: json.RawMessage(config)},
			},
		},
	}
}

func testFleetCredentialSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: fleetCredentialSecretName, Namespace: fleetDatadogAgentNamespace},
		Data:       map[string][]byte{fleetCredentialAPIKey: []byte("api-key-value")},
	}
}

func testFleetOwnedDDA(configID string) *v2alpha1.DatadogAgent {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDDANSN.Name,
			Namespace: testDDANSN.Namespace,
			UID:       types.UID("fleet-dda-uid"),
			Labels: map[string]string{
				fleetManagedByLabel:                     fleetManagedByValue,
				fleetConfigIDLabel:                      configID,
				fleetManagedAgentInstallationStateLabel: fleetManagedAgentInstallationStateReady,
				fleetInstallationIDLabel:                testManagedAgentInstallationIdentity.InstallationID,
				fleetTargetIDLabel:                      testManagedAgentInstallationIdentity.TargetID(),
			},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: fleetCredentialSecretName,
						KeyName:    fleetCredentialAPIKey,
					},
				},
			},
		},
	}
	dda.Status.Conditions = []metav1.Condition{{
		Type:               datadogAgentReconcileErrorCondition,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: dda.Generation,
		Reason:             "DatadogAgent_reconcile_ok",
		LastTransitionTime: metav1.Now(),
	}}
	setTestDatadogAgentReady(dda, false)
	hash, err := fleetDatadogAgentSpecHash(&dda.Spec)
	if err != nil {
		panic(err)
	}
	dda.Annotations = map[string]string{
		fleetConfigHashAnnotation: hash,
	}
	return dda
}

func testManagedAgentInstallationRequest(method, version string) remoteAPIRequest {
	return remoteAPIRequest{
		ID:      "task-123",
		Package: packageDatadogOperator,
		Method:  method,
		Params: operatorTaskParams{
			Version:          version,
			GroupVersionKind: testDDAGVK,
			NamespacedName:   testDDANSN,
			OperationID:      "123e4567-e89b-42d3-a456-426614174001",
			InstallationID:   testManagedAgentInstallationIdentity.InstallationID,
		},
		ExpectedState: expectedState{ClientID: testRCClientID},
	}
}

func testSignedManagedAgentInstallationRequest(d *Daemon, method, version string) remoteAPIRequest {
	req := testManagedAgentInstallationRequest(method, version)
	if _, err := d.getConfig(version); err != nil {
		panic(err)
	}
	return req
}

func TestManagedAgentInstallationRequestEnvelopeValidation(t *testing.T) {
	const configID = "create-config"
	newDaemonAndRequest := func() (*Daemon, *mockRCClient, remoteAPIRequest) {
		d, _, rc := testManagedAgentInstallationDaemon(testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`), nil, testFleetCredentialSecret())
		return d, rc, testSignedManagedAgentInstallationRequest(d, methodInstallDatadogAgent, configID)
	}

	d, _, req := newDaemonAndRequest()
	require.NoError(t, d.validateManagedAgentInstallationRequestEnvelope(req))

	tests := []struct {
		name    string
		mutate  func(*Daemon, *mockRCClient, *remoteAPIRequest)
		wantErr string
	}{
		{name: "missing task ID", mutate: func(_ *Daemon, _ *mockRCClient, req *remoteAPIRequest) { req.ID = "" }, wantErr: "task ID is required"},
		{name: "missing operation ID", mutate: func(_ *Daemon, _ *mockRCClient, req *remoteAPIRequest) { req.Params.OperationID = "" }, wantErr: "operation ID is required"},
		{name: "identity unavailable", mutate: func(d *Daemon, _ *mockRCClient, _ *remoteAPIRequest) {
			d.managedAgentInstallationIdentity = operatorremoteconfig.ManagedAgentInstallationIdentity{}
		}, wantErr: "identity is not configured"},
		{name: "local client unavailable", mutate: func(_ *Daemon, rc *mockRCClient, _ *remoteAPIRequest) { rc.clientID = "" }, wantErr: "local RC client ID is unavailable"},
		{name: "expected client missing", mutate: func(_ *Daemon, _ *mockRCClient, req *remoteAPIRequest) { req.ExpectedState.ClientID = "" }, wantErr: "expected RC client ID is required"},
		{name: "wrong client", mutate: func(_ *Daemon, _ *mockRCClient, req *remoteAPIRequest) { req.ExpectedState.ClientID = "other-client" }, wantErr: "does not match the local client"},
		{name: "wrong installation", mutate: func(_ *Daemon, _ *mockRCClient, req *remoteAPIRequest) {
			req.Params.InstallationID = "223e4567-e89b-42d3-a456-426614174000"
		}, wantErr: "installation ID does not match"},
		{name: "installer config operation changed", mutate: func(d *Daemon, _ *mockRCClient, _ *remoteAPIRequest) {
			d.configs[configID] = testManagedAgentInstallationInstallerConfig(configID, OperationUpdate, `{"spec":{}}`)[configID]
		}, wantErr: "invalid operation: update"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, rc, req := newDaemonAndRequest()
			tt.mutate(d, rc, &req)
			err := d.validateManagedAgentInstallationRequestEnvelope(req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestManagedAgentInstallationTaskRunnerReturnsAfterAcceptance(t *testing.T) {
	const configID = "delete-config"
	d, _, rc := testManagedAgentInstallationDaemon(
		testManagedAgentInstallationInstallerConfig(configID, OperationDelete, ""),
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
	)
	var acceptedTask func()
	d.managedAgentInstallationTaskRunner = func(task func()) {
		acceptedTask = task
	}

	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, configID)
	require.NoError(t, d.handleTask(context.Background(), req))
	require.NotNil(t, acceptedTask)
	require.Len(t, rc.state, 1)
	assert.Equal(t, pbgo.TaskState_RUNNING, rc.state[0].Task.State)

	acceptedTask()
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
}

func setPendingOperationAnnotations(dda *v2alpha1.DatadogAgent, op pendingOperation) {
	if dda.Annotations == nil {
		dda.Annotations = make(map[string]string)
	}
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = op.taskID
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(op.intent)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = op.experimentID
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = op.packageName
	if op.resultVersion != "" {
		dda.Annotations[v2alpha1.AnnotationPendingResultVersion] = op.resultVersion
	}
	if op.targetUID != "" {
		dda.Annotations[fleetPendingTargetUIDAnnotation] = string(op.targetUID)
	}
	if op.intent == pendingIntentStart || op.intent == pendingIntentPromote {
		hash, err := fleetDatadogAgentSpecHash(&dda.Spec)
		if err != nil {
			panic(err)
		}
		dda.Annotations[fleetExperimentHashAnnotation] = hash
	}
}

func TestInstallDatadogAgentCreatesFleetOwnedResource(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{"global":{"site":"ap2.datadoghq.com"}}}`)
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	req := testSignedManagedAgentInstallationRequest(d, methodInstallDatadogAgent, configID)

	require.NoError(t, d.handleTask(context.Background(), req))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, fleetManagedByValue, dda.Labels[fleetManagedByLabel])
	assert.Equal(t, configID, dda.Labels[fleetConfigIDLabel])
	assert.Equal(t, testManagedAgentInstallationIdentity.InstallationID, dda.Labels[fleetInstallationIDLabel])
	require.NotNil(t, dda.Spec.Global)
	require.NotNil(t, dda.Spec.Global.Site)
	assert.Equal(t, "ap2.datadoghq.com", *dda.Spec.Global.Site)
	require.NotNil(t, dda.Spec.Global.Credentials)
	require.NotNil(t, dda.Spec.Global.Credentials.APISecret)
	assert.Equal(t, fleetCredentialSecretName, dda.Spec.Global.Credentials.APISecret.SecretName)
	assert.Equal(t, fleetCredentialAPIKey, dda.Spec.Global.Credentials.APISecret.KeyName)
	assert.Nil(t, dda.Spec.Global.Credentials.AppSecret)

	require.Len(t, rc.state, 1)
	assert.Equal(t, configID, rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
}

func TestInstallDatadogAgentLeavesAmbiguousCreatePartial(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	d.client = &persistThenErrorCreateClient{Client: c}

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remains partial")
	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, fleetManagedAgentInstallationStatePartial, dda.Labels[fleetManagedAgentInstallationStateLabel])
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
}

func TestInstallDatadogAgentPublishesPartialAfterAmbiguousDefaultedCreate(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	d.client = &persistThenErrorCreateClient{Client: c, defaultSpec: true}

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, dda))
	assert.Equal(t, fleetManagedAgentInstallationStatePartial, dda.Labels[fleetManagedAgentInstallationStateLabel])
}

func TestInstallDatadogAgentPreservesAmbiguousCreateOnCoexistenceConflict(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "customer-agent", Namespace: "monitoring", UID: types.UID("customer-uid")}}
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	ambiguous := &persistThenErrorCreateClient{Client: c, unmanaged: unmanaged}
	d.client = ambiguous
	d.apiReader = ambiguous
	previousInterval := managedAgentInstallationDeletePollInterval
	managedAgentInstallationDeletePollInterval = time.Millisecond
	t.Cleanup(func() { managedAgentInstallationDeletePollInterval = previousInterval })

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot safely roll back")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, fleetManagedAgentInstallationStatePartial, current.Labels[fleetManagedAgentInstallationStateLabel])
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(unmanaged), &v2alpha1.DatadogAgent{}))
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
}

func TestInstallDatadogAgentPreservesUnprovenAmbiguousCreateOnCoexistenceConflict(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	recovered := testFleetOwnedDDA(configID)
	delete(recovered.Annotations, fleetCreateTaskIDAnnotation)
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "customer-agent", Namespace: "monitoring", UID: types.UID("customer-uid")}}
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	ambiguous := &recoverForeignCreateClient{Client: c, recovered: recovered, unmanaged: unmanaged}
	d.client = ambiguous
	d.apiReader = ambiguous

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot safely roll back")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, recovered.UID, current.UID)
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
}

func TestCreateRollbackRevalidatesTaskProvenance(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	dda.Annotations[fleetCreateTaskIDAnnotation] = "replacement-task"
	d, c, _ := testManagedAgentInstallationDaemon(nil, nil, dda)
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))

	err := d.rollbackFleetDatadogAgentCreation(context.Background(), testDDANSN, current.UID, current.ResourceVersion, "create-config", "original-task")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no longer has create provenance")
	assert.NoError(t, c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
}

func TestInstallDatadogAgentWaitsForReconcileReadiness(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	d, c, rc := testManagedAgentInstallationDaemon(configs, nil, testFleetCredentialSecret())
	baseClient := c.(*autoReadyManagedAgentInstallationClient).Client
	uidClient := &assignUIDManagedAgentInstallationClient{Client: baseClient}
	d.client = uidClient
	d.apiReader = uidClient
	previousInterval := managedAgentInstallationReadinessPollInterval
	managedAgentInstallationReadinessPollInterval = time.Millisecond
	t.Cleanup(func() { managedAgentInstallationReadinessPollInterval = previousInterval })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := d.installDatadogAgent(ctx, testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has not been reconciled")
	assert.NoError(t, baseClient.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
	require.Len(t, rc.state, 1)
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
}

func TestInstallDatadogAgentIsIdempotentForSameConfig(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	existing := testFleetOwnedDDA(configID)
	d, _, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, existing, testFleetCredentialSecret())

	require.NoError(t, d.handleTask(context.Background(), testSignedManagedAgentInstallationRequest(d, methodInstallDatadogAgent, configID)))
	assert.Equal(t, configID, rc.state[0].StableConfigVersion)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
}

func TestInstallDatadogAgentRejectsDifferentManagedAgentInstallationID(t *testing.T) {
	const configID = "create-config"
	existing := testFleetOwnedDDA(configID)
	existing.Labels[fleetInstallationIDLabel] = "223e4567-e89b-42d3-a456-426614174000"
	d, c, _ := testManagedAgentInstallationDaemon(testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`), nil, existing, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different managed installation")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, existing.Labels[fleetInstallationIDLabel], current.Labels[fleetInstallationIDLabel])
}

func TestInstallDatadogAgentRejectsDifferentManagedAgentInstallationTarget(t *testing.T) {
	const configID = "create-config"
	existing := testFleetOwnedDDA(configID)
	existing.Labels[fleetTargetIDLabel] = strings.Repeat("a", 52)
	d, c, _ := testManagedAgentInstallationDaemon(testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`), nil, existing, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different managed target")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, existing.Labels[fleetTargetIDLabel], current.Labels[fleetTargetIDLabel])
}

func TestManagedAgentInstallationLabeledDatadogAgentRequiresLocalInstallationIdentity(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	d, _, _ := testManagedAgentInstallationDaemon(nil, nil)
	d.managedAgentInstallationIdentity = operatorremoteconfig.ManagedAgentInstallationIdentity{}

	err := d.validateFleetDatadogAgentInstallation(dda)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local managed Agent installation identity is not configured")
}

func TestExperimentUpdateRejectsDifferentManagedAgentInstallationID(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	dda.Labels[fleetInstallationIDLabel] = "223e4567-e89b-42d3-a456-426614174000"
	d, c, _ := testManagedAgentInstallationDaemon(testInstallerConfigWithDDA(), []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: "create-config",
	}}, dda)

	_, err := d.startDatadogAgentExperiment(context.Background(), testStartRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different managed installation")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Empty(t, current.Annotations[v2alpha1.AnnotationExperimentSignal])
}

func TestInstallDatadogAgentReadyReplayRetainsPartialAfterReadinessRegression(t *testing.T) {
	previousInterval := managedAgentInstallationReadinessPollInterval
	previousTimeout := managedAgentInstallationOperationTimeout
	managedAgentInstallationReadinessPollInterval = time.Millisecond
	managedAgentInstallationOperationTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		managedAgentInstallationReadinessPollInterval = previousInterval
		managedAgentInstallationOperationTimeout = previousTimeout
	})

	tests := []struct {
		name             string
		failPartialPatch bool
		wantErr          string
	}{
		{name: "demotion persisted", wantErr: "reconcile condition is True"},
		{name: "demotion persistence failed", failPartialPatch: true, wantErr: "mark managed Agent installation partial before readiness revalidation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const configID = "create-config"
			configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
			existing := testFleetOwnedDDA(configID)
			existing.Status.Conditions[0].Status = metav1.ConditionTrue
			existing.Status.Conditions[0].Message = "reconcile failed after the prior install completed"
			d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{
				Package:             packageDatadogOperator,
				StableConfigVersion: configID,
			}}, existing, testFleetCredentialSecret())
			if tt.failPartialPatch {
				failing := &failPartialManagedAgentInstallationPatchClient{Client: c, failures: 1}
				d.client = failing
				d.apiReader = failing
			}
			req := testSignedManagedAgentInstallationRequest(d, methodInstallDatadogAgent, configID)
			req.ExpectedState.StableConfig = configID

			err := d.handleTask(context.Background(), req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
			require.NotNil(t, rc.state[0].Task)
			assert.Equal(t, pbgo.TaskState_ERROR, rc.state[0].Task.State)

			current := &v2alpha1.DatadogAgent{}
			require.NoError(t, c.Get(context.Background(), testDDANSN, current))
			if tt.failPartialPatch {
				assert.Equal(t, fleetManagedAgentInstallationStateReady, current.Labels[fleetManagedAgentInstallationStateLabel])
				restartedRC := &mockRCClient{state: []*pbgo.PackageState{{
					Package:             packageDatadogOperator,
					StableConfigVersion: configID,
				}}, clientID: testRCClientID}
				restarted := &Daemon{client: c, apiReader: c, rcClient: restartedRC, managedAgentInstallationIdentity: testManagedAgentInstallationIdentity}
				require.NoError(t, restarted.rehydrateInstallerState(context.Background()))
				assert.Equal(t, fleetPartialConfigVersionPrefix+configID, restartedRC.state[0].StableConfigVersion)
				require.NoError(t, c.Get(context.Background(), testDDANSN, current))
				assert.Equal(t, fleetManagedAgentInstallationStatePartial, current.Labels[fleetManagedAgentInstallationStateLabel])
			} else {
				assert.Equal(t, fleetManagedAgentInstallationStatePartial, current.Labels[fleetManagedAgentInstallationStateLabel])
			}
		})
	}
}

func TestInstallDatadogAgentReadyReplayRehydratesAsPartialDuringRevalidation(t *testing.T) {
	previousInterval := managedAgentInstallationReadinessPollInterval
	previousTimeout := managedAgentInstallationOperationTimeout
	managedAgentInstallationReadinessPollInterval = time.Millisecond
	managedAgentInstallationOperationTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		managedAgentInstallationReadinessPollInterval = previousInterval
		managedAgentInstallationOperationTimeout = previousTimeout
	})

	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	existing := testFleetOwnedDDA(configID)
	existing.Status.Conditions[0].Status = metav1.ConditionTrue
	existing.Status.Conditions[0].Message = "reconcile failed after the prior install completed"
	d, c, _ := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: configID,
	}}, existing, testFleetCredentialSecret())
	restartedRC := &mockRCClient{state: []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: configID,
	}}, clientID: testRCClientID}
	restarted := &Daemon{client: c, apiReader: c, rcClient: restartedRC, managedAgentInstallationIdentity: testManagedAgentInstallationIdentity}
	rehydrating := &rehydrateOnPartialManagedAgentInstallationPatchClient{
		Client:    c,
		rehydrate: func() error { return restarted.rehydrateInstallerState(context.Background()) },
	}
	d.client = rehydrating
	d.apiReader = rehydrating
	req := testSignedManagedAgentInstallationRequest(d, methodInstallDatadogAgent, configID)
	req.ExpectedState.StableConfig = configID

	err := d.handleTask(context.Background(), req)
	require.Error(t, err)
	require.True(t, rehydrating.called)
	require.NoError(t, rehydrating.rehydrateErr)
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, restartedRC.state[0].StableConfigVersion)

	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, fleetManagedAgentInstallationStatePartial, current.Labels[fleetManagedAgentInstallationStateLabel])
}

func TestInstallDatadogAgentRejectsDifferentFleetConfig(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, testFleetOwnedDDA("older-config"), testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

func TestInstallDatadogAgentRejectsUnmanagedResource(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: testDDANSN.Name, Namespace: testDDANSN.Namespace}}
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, unmanaged, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

func TestInstallDatadogAgentRejectsCredentialsFromRemoteConfig(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{"global":{"credentials":{"apiKey":"not-allowed"}}}}`)
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials is not allowed")
}

func TestManagedAgentInstallationConfigRejectsCredentialExecutionAndEgressSurfaces(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "component image", config: `{"spec":{"override":{"nodeAgent":{"image":{"name":"attacker.example/agent:latest"}}}}}`},
		{name: "global registry", config: `{"spec":{"global":{"registry":"attacker.example"}}}`},
		{name: "service account", config: `{"spec":{"override":{"nodeAgent":{"serviceAccountName":"attacker"}}}}`},
		{name: "custom config map", config: `{"spec":{"override":{"nodeAgent":{"customConfigurations":{"datadog.yaml":{"configMap":{"name":"attacker"}}}}}}}`},
		{name: "non-Datadog site", config: `{"spec":{"global":{"site":"evil.example"}}}`},
		{name: "external metrics endpoint", config: `{"spec":{"features":{"externalMetricsServer":{"enabled":true,"endpoint":{"url":"https://evil.example"}}}}}`},
		{name: "kubelet ConfigMap selector", config: `{"spec":{"global":{"kubelet":{"host":{"configMapKeyRef":{"name":"customer-config","key":"host"}}}}}}`},
		{name: "kubelet host CA path", config: `{"spec":{"global":{"kubelet":{"hostCAPath":"/customer/ca.pem"}}}}`},
		{name: "Prometheus additional configs", config: `{"spec":{"features":{"prometheusScrape":{"additionalConfigs":[]}}}}`},
		{name: "writable log collection host path", config: `{"spec":{"features":{"logCollection":{"enabled":true,"tempStoragePath":"/etc"}}}}`},
		{name: "container runtime socket host path", config: `{"spec":{"global":{"criSocketPath":"/etc/shadow"}}}`},
		{name: "APM socket host path", config: `{"spec":{"features":{"apm":{"unixDomainSocketConfig":{"enabled":true,"path":"/etc/apm.socket"}}}}}`},
		{name: "DogStatsD socket host path", config: `{"spec":{"features":{"dogstatsd":{"unixDomainSocketConfig":{"enabled":true,"path":"/etc/dsd.socket"}}}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeRemoteDatadogAgentConfig(json.RawMessage(tt.config), false)
			require.Error(t, err)
		})
	}
}

func TestInstallDatadogAgentRequiresCredentialSecret(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil)

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential Secret datadog/datadog-secret is not ready")
}

func TestInstallDatadogAgentRequiresNonEmptyAPIKey(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	secret := testFleetCredentialSecret()
	secret.Data[fleetCredentialAPIKey] = nil
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, secret)

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing non-empty key")
}

func TestInstallDatadogAgentRejectsTerminatingResource(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	existing := testFleetOwnedDDA(configID)
	existing.Finalizers = []string{"test.datadoghq.com/hold-deletion"}
	now := metav1.Now()
	existing.DeletionTimestamp = &now
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, existing, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is terminating")
}

func TestInstallDatadogAgentRejectsCredentialReferenceDrift(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	existing := testFleetOwnedDDA(configID)
	existing.Spec.Global.Credentials.APISecret.SecretName = "other-secret"
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, existing, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

func TestInstallDatadogAgentRejectsSpecDrift(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{"global":{"site":"datadoghq.eu"}}}`)
	existing := testFleetOwnedDDA(configID)
	desired, err := buildFleetDatadogAgentSpec(json.RawMessage(`{"spec":{"global":{"site":"datadoghq.eu"}}}`))
	require.NoError(t, err)
	existing.Spec = *desired
	desiredHash, err := fleetDatadogAgentSpecHash(desired)
	require.NoError(t, err)
	existing.Annotations[fleetConfigHashAnnotation] = desiredHash
	site := "datadoghq.com"
	existing.Spec.Global.Site = &site
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, existing, testFleetCredentialSecret())

	_, err = d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

func TestRecordFleetSpecHashDoesNotBlessConcurrentDrift(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	acceptedHash := dda.Annotations[fleetConfigHashAnnotation]
	site := "datadoghq.eu"
	dda.Spec.Global.Site = &site
	d, _, _ := testManagedAgentInstallationDaemon(nil, nil, dda)

	err := d.recordFleetDatadogAgentSpecHash(context.Background(), testDDANSN, dda.UID, "create-config", acceptedHash)
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

func TestRecordFleetSpecHashRejectsSameNameReplacement(t *testing.T) {
	original := testFleetOwnedDDA("create-config")
	replacement := original.DeepCopy()
	replacement.UID = types.UID("replacement-uid")
	delete(replacement.Annotations, fleetConfigHashAnnotation)
	acceptedHash, err := fleetDatadogAgentSpecHash(&original.Spec)
	require.NoError(t, err)
	d, _, _ := testManagedAgentInstallationDaemon(nil, nil, replacement)

	err = d.recordFleetDatadogAgentSpecHash(context.Background(), testDDANSN, original.UID, "create-config", acceptedHash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was replaced before the accepted Fleet config hash")
}

func TestRecordFleetExperimentHashRejectsSameNameReplacement(t *testing.T) {
	original := testFleetOwnedDDA("create-config")
	replacement := original.DeepCopy()
	replacement.UID = types.UID("replacement-uid")
	acceptedHash, err := fleetDatadogAgentSpecHash(&original.Spec)
	require.NoError(t, err)
	d, _, _ := testManagedAgentInstallationDaemon(nil, nil, replacement)

	err = d.recordFleetExperimentSpecHash(context.Background(), testDDANSN, original.UID, acceptedHash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was replaced before the accepted Fleet experiment hash")
}

func TestMarkFleetExperimentReadyRejectsUnmanagedSameNameReplacement(t *testing.T) {
	original := testFleetOwnedDDA("create-config")
	replacement := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{
		Name:      testDDANSN.Name,
		Namespace: testDDANSN.Namespace,
		UID:       types.UID("replacement-uid"),
	}}
	d, _, _ := testManagedAgentInstallationDaemon(nil, nil, replacement)

	_, err := d.markFleetDatadogAgentExperimentReady(context.Background(), testDDANSN, original.UID, "update-config")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was replaced while finishing the Fleet experiment")
}

func TestMarkFleetPartialRejectsSameNameReplacement(t *testing.T) {
	original := testFleetOwnedDDA("create-config")
	replacement := original.DeepCopy()
	replacement.UID = types.UID("replacement-uid")
	d, c, _ := testManagedAgentInstallationDaemon(nil, nil, replacement)

	_, err := d.markFleetDatadogAgentPartial(context.Background(), testDDANSN, original.UID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was replaced before its managed Agent installation state could be marked partial")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, fleetManagedAgentInstallationStateReady, current.Labels[fleetManagedAgentInstallationStateLabel])
}

func TestInstallDatadogAgentRejectsMissingConfigHash(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	existing := testFleetOwnedDDA(configID)
	delete(existing.Annotations, fleetConfigHashAnnotation)
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, existing, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

func TestInstallDatadogAgentToleratesAPIServerDefaults(t *testing.T) {
	const configID = "create-config"
	raw := json.RawMessage(`{"spec":{"features":{"otelCollector":{"ports":[{"name":"custom","containerPort":4317}]}}}}`)
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, string(raw))
	desired, err := buildFleetDatadogAgentSpec(raw)
	require.NoError(t, err)
	existing := testFleetOwnedDDA(configID)
	existing.Spec = *desired
	existing.Spec.Features.OtelCollector.Ports[0].Protocol = corev1.ProtocolTCP
	hash, err := fleetDatadogAgentSpecHash(&existing.Spec)
	require.NoError(t, err)
	existing.Annotations[fleetConfigHashAnnotation] = hash
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, existing, testFleetCredentialSecret())

	_, err = d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.NoError(t, err)
}

func TestInstallDatadogAgentRejectsExtraLiveSpecFields(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	existing := testFleetOwnedDDA(configID)
	enabled := true
	existing.Spec.Features = &v2alpha1.DatadogFeatures{
		APM: &v2alpha1.APMFeatureConfig{Enabled: &enabled},
	}
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, existing, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
	assert.Contains(t, err.Error(), "spec changed")
}

func TestInstallDatadogAgentRejectsAnotherFleetOwnedResource(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	other := testFleetOwnedDDA("other-config")
	other.Name = "other-datadog-agent"
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, other, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
}

func TestInstallDatadogAgentRejectsDifferentlyNamedUnmanagedResource(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "customer-agent", Namespace: "monitoring"}}
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, unmanaged, testFleetCredentialSecret())

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
	assert.Contains(t, err.Error(), "unmanaged DatadogAgent monitoring/customer-agent")
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(unmanaged), &v2alpha1.DatadogAgent{}))
	assert.True(t, apierrors.IsNotFound(c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{})))
}

func TestInstallDatadogAgentRollsBackWhenUnmanagedResourceAppearsDuringCreate(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "customer-agent", Namespace: "monitoring", UID: types.UID("customer-uid")}}
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, testFleetCredentialSecret())
	concurrentClient := &concurrentUnmanagedDatadogAgentClient{Client: c, unmanaged: unmanaged}
	d.client = concurrentClient
	d.apiReader = concurrentClient
	previousInterval := managedAgentInstallationDeletePollInterval
	managedAgentInstallationDeletePollInterval = time.Millisecond
	t.Cleanup(func() { managedAgentInstallationDeletePollInterval = previousInterval })

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmanaged DatadogAgent monitoring/customer-agent")
	assert.True(t, apierrors.IsNotFound(c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{})))
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(unmanaged), &v2alpha1.DatadogAgent{}))
}

func TestInstallDatadogAgentDoesNotRollBackAfterPostCreateListFailure(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	failing := &failDatadogAgentListClient{Client: c, failOn: 2}
	d.client = failing
	d.apiReader = failing

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DatadogAgent list failed")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, fleetManagedAgentInstallationStatePartial, current.Labels[fleetManagedAgentInstallationStateLabel])
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
}

func TestInstallDatadogAgentDoesNotRollBackAfterTargetChangesSinceConflictObservation(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "customer-agent", Namespace: "monitoring", UID: types.UID("customer-uid")}}
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	concurrent := &concurrentUnmanagedDatadogAgentClient{Client: c, unmanaged: unmanaged}
	mutating := &mutateDatadogAgentAfterConflictListClient{Client: concurrent}
	d.client = mutating
	d.apiReader = mutating

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed after the concurrent-install conflict was observed")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, "true", current.Annotations["test.datadoghq.com/concurrent-change"])
	assert.Equal(t, fleetManagedAgentInstallationStatePartial, current.Labels[fleetManagedAgentInstallationStateLabel])
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(unmanaged), &v2alpha1.DatadogAgent{}))
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
}

func TestInstallDatadogAgentRollsBackWhenUnmanagedResourceAppearsAtReadyBoundary(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "customer-agent", Namespace: "monitoring", UID: types.UID("customer-uid")}}
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	lateClient := &unmanagedDatadogAgentOnReadyPatchClient{Client: c, unmanaged: unmanaged}
	d.client = lateClient
	d.apiReader = lateClient
	previousInterval := managedAgentInstallationDeletePollInterval
	managedAgentInstallationDeletePollInterval = time.Millisecond
	t.Cleanup(func() { managedAgentInstallationDeletePollInterval = previousInterval })

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmanaged DatadogAgent monitoring/customer-agent")
	assert.True(t, apierrors.IsNotFound(c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{})))
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(unmanaged), &v2alpha1.DatadogAgent{}))
	assert.Empty(t, rc.state[0].StableConfigVersion)
}

func TestInstallDatadogAgentRejectsTargetReplacementAtReadyBoundary(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	replacement := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{
		Name:      testDDANSN.Name,
		Namespace: testDDANSN.Namespace,
		UID:       types.UID("customer-replacement-uid"),
	}}
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	replacing := &replaceDatadogAgentOnReadyPatchClient{Client: c, replacement: replacement}
	d.client = replacing
	d.apiReader = replacing

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was replaced before install completion")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, replacement.UID, current.UID)
	assert.Empty(t, current.Labels[fleetManagedByLabel])
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
}

func TestInstallDatadogAgentRejectsReadinessRegressionAtFinalCheck(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, testFleetCredentialSecret())
	invalidating := &invalidateReadinessOnReadyPatchClient{Client: c}
	d.client = invalidating
	d.apiReader = invalidating

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not ready at install completion")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, fleetManagedAgentInstallationStatePartial, current.Labels[fleetManagedAgentInstallationStateLabel])
	condition := meta.FindStatusCondition(current.Status.Conditions, datadogAgentReconcileErrorCondition)
	require.NotNil(t, condition)
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
}

func TestInstallDatadogAgentPreservesReplayedResourceWhenUnmanagedResourceAppearsAtReadyBoundary(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	existing := testFleetOwnedDDA(configID)
	existing.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
	existing.Annotations[fleetCreateTaskIDAnnotation] = "task-123"
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "customer-agent", Namespace: "monitoring", UID: types.UID("customer-uid")}}
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{Package: packageDatadogOperator}}, existing, testFleetCredentialSecret())
	lateClient := &unmanagedDatadogAgentOnReadyPatchClient{Client: c, unmanaged: unmanaged}
	d.client = lateClient
	d.apiReader = lateClient

	_, err := d.installDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot safely roll back")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, existing.UID, current.UID)
	assert.Equal(t, fleetManagedAgentInstallationStatePartial, current.Labels[fleetManagedAgentInstallationStateLabel])
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(unmanaged), &v2alpha1.DatadogAgent{}))
	assert.Equal(t, fleetPartialConfigVersionPrefix+configID, rc.state[0].StableConfigVersion)
}

func TestResolveManagedAgentInstallationOperationValidation(t *testing.T) {
	validConfig := testManagedAgentInstallationInstallerConfig("config", OperationCreate, `{"spec":{}}`)
	tests := []struct {
		name    string
		configs map[string]installerConfig
		mutate  func(*remoteAPIRequest)
	}{
		{
			name:    "missing config",
			configs: map[string]installerConfig{},
		},
		{
			name:    "empty version",
			configs: validConfig,
			mutate:  func(req *remoteAPIRequest) { req.Params.Version = "" },
		},
		{
			name:    "wrong operation",
			configs: testManagedAgentInstallationInstallerConfig("config", OperationUpdate, `{"spec":{}}`),
		},
		{
			name: "multiple operations",
			configs: map[string]installerConfig{"config": {
				ID: "config",
				Operations: []fleetManagementOperation{
					{Operation: OperationCreate},
					{Operation: OperationCreate},
				},
			}},
		},
		{
			name:    "wrong namespace",
			configs: validConfig,
			mutate:  func(req *remoteAPIRequest) { req.Params.NamespacedName.Namespace = "other" },
		},
		{
			name:    "wrong package",
			configs: validConfig,
			mutate:  func(req *remoteAPIRequest) { req.Package = "other-package" },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, _, _ := testManagedAgentInstallationDaemon(tc.configs, nil)
			req := testManagedAgentInstallationRequest(methodInstallDatadogAgent, "config")
			if tc.mutate != nil {
				tc.mutate(&req)
			}
			_, err := d.resolveManagedAgentInstallationOperation(req, OperationCreate)
			assert.Error(t, err)
		})
	}
}

func TestBuildFleetDatadogAgentSpecRejectsUnknownAndTrailingJSON(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"spec":{"unknownField":true}}`),
		json.RawMessage(`{"spec":{}} {}`),
	} {
		_, err := buildFleetDatadogAgentSpec(raw)
		assert.Error(t, err)
	}
}

func TestBuildFleetDatadogAgentSpecRejectsOtherCredentialSources(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"spec":{"global":{"clusterAgentToken":"token"}}}`),
		json.RawMessage(`{"spec":{"global":{"clusterAgentTokenSecret":{"secretName":"other"}}}}`),
		json.RawMessage(`{"spec":{"global":{"endpoint":{"url":"https://example.com"}}}}`),
		json.RawMessage(`{"spec":{"global":{"env":[]}}}`),
		json.RawMessage(`{"spec":{"override":{"nodeAgent":{"disabled":true}}}}`),
	} {
		_, err := buildFleetDatadogAgentSpec(raw)
		assert.Error(t, err)
	}
}

func TestBuildFleetDatadogAgentSpecAllowsCredentialFreeOverrides(t *testing.T) {
	for name, raw := range map[string]string{
		"node selector":       `{"spec":{"override":{"nodeAgent":{"nodeSelector":{"kubernetes.io/os":"linux"}}}}}`,
		"container resources": `{"spec":{"override":{"nodeAgent":{"containers":{"agent":{"resources":{"requests":{"cpu":"100m"}}}}}}}}`,
		"Datadog CSI":         `{"spec":{"global":{"csi":{"enabled":true}}}}`,
		"OTLP bind endpoint":  `{"spec":{"features":{"otlp":{"receiver":{"protocols":{"grpc":{"endpoint":"0.0.0.0:4317"}}}}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := buildFleetDatadogAgentSpec(json.RawMessage(raw))
			require.NoError(t, err)
		})
	}
}

func TestBuildFleetDatadogAgentSpecRejectsNestedSecretSelectors(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"spec":{"global":{"kubelet":{"host":{"secretKeyRef":{"name":"other","key":"token"}}}}}}`),
		json.RawMessage(`{"spec":{"features":{"apm":{"instrumentation":{"targets":[{"name":"default","ddTraceConfigs":[{"name":"DD_API_KEY","valueFrom":{"secretKeyRef":{"name":"other","key":"api-key"}}}] }]}}}}}`),
		json.RawMessage(`{"spec":{"override":{"nodeAgent":{"volumes":[{"name":"secrets-store","csi":{"driver":"secrets-store.csi.k8s.io","volumeAttributes":{"secretProviderClass":"customer-secrets"}}}]}}}}`),
		json.RawMessage(`{"spec":{"override":{"nodeAgent":{"containers":{"agent":{"command":["sh"],"args":["-c","read-secret"]}}}}}}`),
		json.RawMessage(`{"spec":{"override":{"nodeAgent":{"serviceAccountName":"privileged"}}}}`),
		json.RawMessage(`{"spec":{"override":{"nodeAgent":{"serviceAccountAnnotations":{"eks.amazonaws.com/role-arn":"arn:aws:iam::123456789012:role/privileged"}}}}}`),
		json.RawMessage(`{"spec":{"override":{"nodeAgent":{"volumes":[{"name":"token","projected":{"sources":[{"serviceAccountToken":{"audience":"sts.amazonaws.com","path":"token"}}]}}]}}}}`),
	} {
		_, err := buildFleetDatadogAgentSpec(raw)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not allowed")
	}
}

func TestStartExperimentRejectsCredentialAndIdentitySelectors(t *testing.T) {
	tests := map[string]struct {
		config  string
		wantErr string
	}{
		"CSI secret selector": {
			config:  `{"spec":{"override":{"nodeAgent":{"volumes":[{"name":"secrets-store","csi":{"driver":"secrets-store.csi.k8s.io","volumeAttributes":{"secretProviderClass":"customer-secrets"}}}]}}}}`,
			wantErr: "volumes is not allowed",
		},
		"service account": {
			config:  `{"spec":{"override":{"nodeAgent":{"serviceAccountName":"privileged"}}}}`,
			wantErr: "serviceAccountName is not allowed",
		},
		"projected service account token": {
			config:  `{"spec":{"serviceAccountToken":{"audience":"sts.amazonaws.com","path":"token"}}}`,
			wantErr: "serviceAccountToken is not allowed",
		},
		"kubelet ConfigMap selector": {
			config:  `{"spec":{"global":{"kubelet":{"host":{"configMapKeyRef":{"name":"customer-config","key":"host"}}}}}}`,
			wantErr: "configMapKeyRef is not allowed",
		},
		"Prometheus additional configs": {
			config:  `{"spec":{"features":{"prometheusScrape":{"additionalConfigs":[]}}}}`,
			wantErr: "additionalConfigs is not allowed",
		},
		"writable log collection host path": {
			config:  `{"spec":{"features":{"logCollection":{"enabled":true,"tempStoragePath":"/etc"}}}}`,
			wantErr: "log collection host paths",
		},
		"container runtime socket host path": {
			config:  `{"spec":{"global":{"dockerSocketPath":"/etc/shadow"}}}`,
			wantErr: "container runtime socket host paths",
		},
		"APM socket host path": {
			config:  `{"spec":{"features":{"apm":{"unixDomainSocketConfig":{"enabled":true,"path":"/etc/apm.socket"}}}}}`,
			wantErr: "APM Unix domain socket host path",
		},
		"DogStatsD socket host path": {
			config:  `{"spec":{"features":{"dogstatsd":{"unixDomainSocketConfig":{"enabled":true,"path":"/etc/dsd.socket"}}}}}`,
			wantErr: "DogStatsD Unix domain socket host path",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			configs := testInstallerConfigWithDDA()
			config := configs["test-config"]
			config.Operations[0].Config = json.RawMessage(tc.config)
			configs["test-config"] = config
			d, _, _ := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{
				Package:             packageDatadogOperator,
				StableConfigVersion: "create-config",
			}}, testFleetOwnedDDA("create-config"))

			_, err := d.startDatadogAgentExperiment(context.Background(), testStartRequest())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestBuildFleetDatadogAgentSpecRejectsIndirectCredentialSources(t *testing.T) {
	tests := map[string]string{
		"tracer environment": `{"spec":{"features":{"admissionController":{"agentSidecarInjection":{"profiles":[{"env":[{"name":"DD_API_KEY","value":"raw-key"}]}]}}}}}`,
		"inline config":      `{"spec":{"features":{"otelCollector":{"conf":{"configData":"api_key: raw-key"}}}}}`,
		"image pull secret":  `{"spec":{"features":{"admissionController":{"agentSidecarInjection":{"image":{"pullSecrets":[{"name":"private-registry"}]}}}}}}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := buildFleetDatadogAgentSpec(json.RawMessage(raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "is not allowed")
		})
	}
}

func TestUninstallDatadogAgentDeletesOwnedResourceBeforeDone(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	rcState := []*pbgo.PackageState{{Package: packageDatadogOperator, StableConfigVersion: "create-config"}}
	d, c, rc := testManagedAgentInstallationDaemon(configs, rcState, testFleetOwnedDDA("create-config"))
	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	req.ExpectedState.StableConfig = "create-config"

	require.NoError(t, d.handleTask(context.Background(), req))

	err := c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{})
	assert.True(t, apierrors.IsNotFound(err))
	require.Len(t, rc.state, 1)
	assert.Empty(t, rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
	require.NotNil(t, rc.state[0].Task)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
}

func TestUninstallDatadogAgentCleansUpLateDatadogAgentInternal(t *testing.T) {
	const deleteConfigID = "delete-config"
	dda := testFleetOwnedDDA("create-config")
	controller := true
	blockOwnerDeletion := true
	late := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      testDDANSN.Name,
		Namespace: testDDANSN.Namespace,
		UID:       types.UID("late-ddai-uid"),
		Labels: map[string]string{
			fleetManagedByLabel:      fleetManagedByValue,
			fleetInstallationIDLabel: testManagedAgentInstallationIdentity.InstallationID,
			fleetTargetIDLabel:       testManagedAgentInstallationIdentity.TargetID(),
		},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion:         v2alpha1.GroupVersion.String(),
			Kind:               "DatadogAgent",
			Name:               dda.Name,
			UID:                dda.UID,
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		}},
	}}
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, dda)
	lateClient := &lateDDAICreateOnDDADeleteClient{Client: c, late: late}
	d.client = lateClient
	d.apiReader = lateClient
	previousInterval := managedAgentInstallationDeletePollInterval
	managedAgentInstallationDeletePollInterval = time.Millisecond
	t.Cleanup(func() { managedAgentInstallationDeletePollInterval = previousInterval })

	_, err := d.uninstallDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.NoError(t, err)
	assert.True(t, apierrors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(late), &v1alpha1.DatadogAgentInternal{})))
}

func TestUninstallDatadogAgentIsIdempotentWhenMissing(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	rcState := []*pbgo.PackageState{{Package: packageDatadogOperator, StableConfigVersion: "create-config"}}
	d, _, rc := testManagedAgentInstallationDaemon(configs, rcState)
	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	req.ExpectedState.StableConfig = "create-config"

	require.NoError(t, d.handleTask(context.Background(), req))
	assert.Empty(t, rc.state[0].StableConfigVersion)
	assert.Equal(t, pbgo.TaskState_DONE, rc.state[0].Task.State)
}

func TestUninstallDatadogAgentRetriesDatadogAgentInternalVersionConflict(t *testing.T) {
	const deleteConfigID = "delete-config"
	controller := true
	ddai := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      testDDANSN.Name,
		Namespace: testDDANSN.Namespace,
		UID:       types.UID("fleet-ddai-uid"),
		Labels:    map[string]string{fleetManagedByLabel: fleetManagedByValue},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: v2alpha1.GroupVersion.String(),
			Kind:       "DatadogAgent",
			Name:       testDDANSN.Name,
			UID:        types.UID("deleted-fleet-dda-uid"),
			Controller: &controller,
		}},
	}}
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, ddai)
	changingClient := &datadogAgentInternalVersionChangingDeleteClient{Client: c}
	d.client = changingClient
	d.apiReader = changingClient
	previousInterval := managedAgentInstallationDeletePollInterval
	managedAgentInstallationDeletePollInterval = time.Millisecond
	t.Cleanup(func() { managedAgentInstallationDeletePollInterval = previousInterval })

	_, err := d.uninstallDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.NoError(t, err)
	assert.True(t, changingClient.changed)
	assert.True(t, apierrors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(ddai), &v1alpha1.DatadogAgentInternal{})))
}

func TestUninstallDatadogAgentWaitsForOrphanedInternalCleanup(t *testing.T) {
	const deleteConfigID = "delete-config"
	controller := true
	blockOwnerDeletion := true
	ddai := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      testDDANSN.Name,
		Namespace: testDDANSN.Namespace,
		UID:       types.UID("fleet-ddai-uid"),
		Labels:    map[string]string{fleetManagedByLabel: fleetManagedByValue},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion:         v2alpha1.GroupVersion.String(),
			Kind:               "DatadogAgent",
			Name:               testDDANSN.Name,
			UID:                types.UID("deleted-fleet-dda-uid"),
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		}},
	}}
	clusterRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{
		Name: "fleet-ddai-role",
		Labels: map[string]string{
			operatorStoreLabelKey: "true",
			appPartOfLabelKey:     managedAgentInstallationPartOfLabelValue(testDDANSN),
			appManagedByLabelKey:  "datadog-operator",
		},
	}}
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, ddai, clusterRole)
	previousInterval := managedAgentInstallationDeletePollInterval
	managedAgentInstallationDeletePollInterval = time.Millisecond
	t.Cleanup(func() { managedAgentInstallationDeletePollInterval = previousInterval })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := d.uninstallDatadogAgent(ctx, testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "waiting for remaining resource removal")
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(clusterRole), &rbacv1.ClusterRole{}))
}

func TestUninstallDatadogAgentWaitsForOrphanedClusterResourceWithoutInternal(t *testing.T) {
	const deleteConfigID = "delete-config"
	clusterRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{
		Name: "orphaned-fleet-ddai-role",
		Labels: map[string]string{
			operatorStoreLabelKey: "true",
			appPartOfLabelKey:     managedAgentInstallationPartOfLabelValue(testDDANSN),
			appManagedByLabelKey:  "datadog-operator",
		},
	}}
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, clusterRole)
	previousInterval := managedAgentInstallationDeletePollInterval
	managedAgentInstallationDeletePollInterval = time.Millisecond
	t.Cleanup(func() { managedAgentInstallationDeletePollInterval = previousInterval })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := d.uninstallDatadogAgent(ctx, testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "waiting for remaining resource removal")
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(clusterRole), &rbacv1.ClusterRole{}))
}

func TestUninstallDatadogAgentRejectsTargetRecreatedAfterCleanupPoll(t *testing.T) {
	const deleteConfigID = "delete-config"
	dda := testFleetOwnedDDA("create-config")
	replacement := testFleetOwnedDDA("create-config")
	replacement.UID = types.UID("replacement-uid")
	replacement.ResourceVersion = ""
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, dda)
	recreating := &recreateFleetDatadogAgentOnFinalListClient{Client: c, replacement: replacement}
	d.client = recreating
	d.apiReader = recreating
	previousInterval := managedAgentInstallationDeletePollInterval
	managedAgentInstallationDeletePollInterval = time.Millisecond
	t.Cleanup(func() { managedAgentInstallationDeletePollInterval = previousInterval })

	_, err := d.uninstallDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was recreated while uninstalling")
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Equal(t, replacement.UID, current.UID)
}

func TestUninstallDatadogAgentRejectsWrongTargetWhenFleetOwnedResourceExists(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	rcState := []*pbgo.PackageState{{Package: packageDatadogOperator, StableConfigVersion: "create-config"}}
	d, _, rc := testManagedAgentInstallationDaemon(configs, rcState, testFleetOwnedDDA("create-config"))
	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	req.Params.NamespacedName.Name = "wrong-target"
	req.ExpectedState.StableConfig = "create-config"

	err := d.handleTask(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, "create-config", rc.state[0].StableConfigVersion)
	assert.Equal(t, pbgo.TaskState_INVALID_STATE, rc.state[0].Task.State)
}

func TestUninstallDatadogAgentRejectsUnmanagedResource(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: testDDANSN.Name, Namespace: testDDANSN.Namespace}}
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, unmanaged)

	_, err := d.uninstallDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
	assert.NoError(t, c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
}

func TestUninstallDatadogAgentRejectsDifferentManagedAgentInstallationID(t *testing.T) {
	const deleteConfigID = "delete-config"
	existing := testFleetOwnedDDA("create-config")
	existing.Labels[fleetInstallationIDLabel] = "223e4567-e89b-42d3-a456-426614174000"
	d, c, _ := testManagedAgentInstallationDaemon(testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, ""), nil, existing)

	_, err := d.uninstallDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different managed installation")
	assert.NoError(t, c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
}

func TestUninstallDatadogAgentRejectsUnmanagedResourceBesideFleetTarget(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	fleetOwned := testFleetOwnedDDA("create-config")
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "customer-agent", Namespace: "monitoring"}}
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, fleetOwned, unmanaged)

	_, err := d.uninstallDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
	assert.NoError(t, c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(unmanaged), &v2alpha1.DatadogAgent{}))
}

func TestUninstallDatadogAgentRejectsUnmanagedResourceCreatedDuringDeletion(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "customer-agent", Namespace: "monitoring"}}
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: "create-config",
	}}, testFleetOwnedDDA("create-config"))
	lateClient := &unmanagedDatadogAgentOnDDADeleteClient{Client: c, unmanaged: unmanaged}
	d.client = lateClient
	d.apiReader = lateClient
	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	req.ExpectedState.StableConfig = "create-config"

	err := d.handleTask(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmanaged DatadogAgent monitoring/customer-agent")
	assert.True(t, apierrors.IsNotFound(c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{})))
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(unmanaged), &v2alpha1.DatadogAgent{}))
	assert.Equal(t, "create-config", rc.state[0].StableConfigVersion)
	assert.Equal(t, pbgo.TaskState_INVALID_STATE, rc.state[0].Task.State)
}

func TestUninstallDatadogAgentRejectsIncompleteFleetOwnership(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	dda := testFleetOwnedDDA("create-config")
	delete(dda.Annotations, fleetConfigHashAnnotation)
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, dda)

	_, err := d.uninstallDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete or conflicting Fleet ownership metadata")
	assert.NoError(t, c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
}

func TestUninstallDatadogAgentDoesNotDeleteAfterOwnershipChanges(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	dda := testFleetOwnedDDA("create-config")
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, dda)
	changingClient := &ownershipChangingDeleteClient{Client: c}
	d.client = changingClient
	d.apiReader = changingClient

	_, err := d.uninstallDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	assert.Empty(t, current.Labels[fleetManagedByLabel])
}

func TestUninstallDatadogAgentRejectsPendingExperiment(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	dda := testFleetOwnedDDA("create-config")
	dda.Annotations[v2alpha1.AnnotationPendingTaskID] = "experiment-task"
	dda.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	dda.Annotations[v2alpha1.AnnotationPendingExperimentID] = "update-config"
	dda.Annotations[v2alpha1.AnnotationPendingPackage] = packageDatadogOperator
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, dda)

	_, err := d.uninstallDatadogAgent(context.Background(), testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	var stateErr *stateDoesntMatchError
	assert.True(t, errors.As(err, &stateErr))
	assert.NoError(t, c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
}

func TestManagedAgentInstallationTaskRejectsActiveExperimentConfig(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{
		Package:                 packageDatadogOperator,
		StableConfigVersion:     "create-config",
		ExperimentConfigVersion: "update-config",
	}}, testFleetOwnedDDA("create-config"))
	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	req.ExpectedState.StableConfig = "create-config"
	req.ExpectedState.ExperimentConfig = "update-config"

	err := d.handleTask(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, pbgo.TaskState_INVALID_STATE, rc.state[0].Task.State)
	assert.NoError(t, c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
}

func TestPendingExperimentDoesNotOverwriteManagedAgentInstallationReservation(t *testing.T) {
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{
		Package: packageDatadogOperator,
		Task: &pbgo.PackageStateTask{
			Id:    "managed-agent-installation-task",
			State: pbgo.TaskState_RUNNING,
		},
	}})
	d.managedAgentInstallationActive = true
	snapshot := ddaStatusSnapshot{
		nsn: testDDANSN,
		annotations: map[string]string{
			v2alpha1.AnnotationPendingTaskID:       "experiment-task",
			v2alpha1.AnnotationPendingAction:       string(pendingIntentStart),
			v2alpha1.AnnotationPendingExperimentID: "update-config",
			v2alpha1.AnnotationPendingPackage:      packageDatadogOperator,
		},
		experiment: &v2alpha1.ExperimentStatus{ID: "update-config"},
	}

	newOperationTracker(d).onStatusUpdate(context.Background(), snapshot)

	assert.Equal(t, "managed-agent-installation-task", rc.state[0].Task.Id)
	assert.Equal(t, pbgo.TaskState_RUNNING, rc.state[0].Task.State)
}

func TestUninstallDatadogAgentDoesNotClearStateBeforeDeletion(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	dda := testFleetOwnedDDA("create-config")
	dda.Finalizers = []string{"test.datadoghq.com/hold-deletion"}
	rcState := []*pbgo.PackageState{{Package: packageDatadogOperator, StableConfigVersion: "create-config"}}
	d, c, rc := testManagedAgentInstallationDaemon(configs, rcState, dda)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := d.uninstallDatadogAgent(ctx, testManagedAgentInstallationRequest(methodUninstallDatadogAgent, deleteConfigID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "waiting for resource removal")
	assert.Equal(t, "create-config", rc.state[0].StableConfigVersion)

	current := &v2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), testDDANSN, current))
	require.NotNil(t, current.DeletionTimestamp)
}

func TestRehydrateInstallerStateFromFleetOwnedDatadogAgent(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: "empty",
	}}, dda)

	require.NoError(t, d.rehydrateInstallerState(context.Background()))
	require.Len(t, rc.state, 1)
	assert.Equal(t, "create-config", rc.state[0].StableConfigVersion)
	assert.Empty(t, rc.state[0].ExperimentConfigVersion)
}

func TestRehydrateInstallerStatePreservesPartialInstall(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
	site := "datadoghq.eu"
	dda.Spec.Global.Site = &site
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: operatorremoteconfig.InstallerStateUnknownConfigVersion,
	}}, dda)

	require.NoError(t, d.rehydrateInstallerState(context.Background()))
	assert.Equal(t, fleetPartialConfigVersionPrefix+"create-config", rc.state[0].StableConfigVersion)
}

func TestRehydrateInstallerStateTreatsMissingManagedAgentInstallationStateAsPartial(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	delete(dda.Labels, fleetManagedAgentInstallationStateLabel)
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: operatorremoteconfig.InstallerStateUnknownConfigVersion,
	}}, dda)

	require.NoError(t, d.rehydrateInstallerState(context.Background()))
	assert.Equal(t, fleetPartialConfigVersionPrefix+"create-config", rc.state[0].StableConfigVersion)
}

func TestRehydrateInstallerStateTreatsMissingConfigHashAsPartial(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	delete(dda.Annotations, fleetConfigHashAnnotation)
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: operatorremoteconfig.InstallerStateUnknownConfigVersion,
	}}, dda)

	require.NoError(t, d.rehydrateInstallerState(context.Background()))
	assert.Equal(t, fleetPartialConfigVersionPrefix+"create-config", rc.state[0].StableConfigVersion)
}

func TestPublishManagedAgentInstallationStateTreatsMissingLabelAsPartial(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	delete(dda.Labels, fleetManagedAgentInstallationStateLabel)
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{Package: packageDatadogOperator}}, dda)

	d.publishFleetDatadogAgentManagedAgentInstallationState(packageDatadogOperator, dda, "create-config")

	assert.Equal(t, fleetPartialConfigVersionPrefix+"create-config", rc.state[0].StableConfigVersion)
}

func TestRehydrateInstallerStateRejectsMultipleFleetOwnedDatadogAgents(t *testing.T) {
	first := testFleetOwnedDDA("first-config")
	second := testFleetOwnedDDA("second-config")
	second.Name = "second-datadog-agent"
	second.UID = types.UID("second-fleet-dda-uid")
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{
		{Package: packageDatadogOperator, StableConfigVersion: operatorremoteconfig.InstallerStateUnknownConfigVersion},
	}, first, second)

	err := d.rehydrateInstallerState(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple Fleet-managed DatadogAgents")
	assert.Equal(t, operatorremoteconfig.InstallerStateUnknownConfigVersion, rc.state[0].StableConfigVersion)
}

func TestRehydrateInstallerStateRejectsIncompleteFleetOwnership(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	delete(dda.Labels, fleetConfigIDLabel)
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: operatorremoteconfig.InstallerStateUnknownConfigVersion,
	}}, dda)

	err := d.rehydrateInstallerState(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete or conflicting Fleet ownership metadata")
	assert.Equal(t, operatorremoteconfig.InstallerStateUnknownConfigVersion, rc.state[0].StableConfigVersion)
}

func TestRehydrateInstallerStateRejectsResidualFleetMarkers(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	delete(dda.Labels, fleetManagedByLabel)
	delete(dda.Labels, fleetConfigIDLabel)
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: operatorremoteconfig.InstallerStateUnknownConfigVersion,
	}}, dda)

	err := d.rehydrateInstallerState(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete or conflicting Fleet ownership metadata")
	assert.Equal(t, operatorremoteconfig.InstallerStateUnknownConfigVersion, rc.state[0].StableConfigVersion)
}

func TestRehydrateInstallerStateRejectsResidualCreateTaskMarker(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{
		Name:      testDDANSN.Name,
		Namespace: testDDANSN.Namespace,
		Annotations: map[string]string{
			fleetCreateTaskIDAnnotation: "create-task",
		},
	}}
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: operatorremoteconfig.InstallerStateUnknownConfigVersion,
	}}, dda)

	err := d.rehydrateInstallerState(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete or conflicting Fleet ownership metadata")
	assert.Equal(t, operatorremoteconfig.InstallerStateUnknownConfigVersion, rc.state[0].StableConfigVersion)
}

func TestRehydrateInstallerStateRejectsFleetSpecDrift(t *testing.T) {
	dda := testFleetOwnedDDA("create-config")
	site := "datadoghq.eu"
	dda.Spec.Global.Site = &site
	d, _, rc := testManagedAgentInstallationDaemon(nil, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: operatorremoteconfig.InstallerStateUnknownConfigVersion,
	}}, dda)

	err := d.rehydrateInstallerState(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match its accepted Fleet config")
	assert.Equal(t, operatorremoteconfig.InstallerStateUnknownConfigVersion, rc.state[0].StableConfigVersion)
}

func TestManagedAgentInstallationMethodsDoNotRequireControllerRevisions(t *testing.T) {
	const configID = "create-config"
	configs := testManagedAgentInstallationInstallerConfig(configID, OperationCreate, `{"spec":{}}`)
	d, _, _ := testManagedAgentInstallationDaemon(configs, nil, testFleetCredentialSecret())

	_, err := d.handleRemoteAPIRequest(context.Background(), testManagedAgentInstallationRequest(methodInstallDatadogAgent, configID))
	assert.NoError(t, err)
}

func TestExperimentMethodsStillRequireControllerRevisions(t *testing.T) {
	d, _, _ := testManagedAgentInstallationDaemon(testInstallerConfigWithDDA(), nil, testDDAObject(""))

	_, err := d.handleRemoteAPIRequest(context.Background(), testStartRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experiment signals require")
}

func TestManagedAgentInstallationMethodLabels(t *testing.T) {
	assert.Equal(t, "install", methodLabel(methodInstallDatadogAgent))
	assert.Equal(t, "uninstall", methodLabel(methodUninstallDatadogAgent))
}
