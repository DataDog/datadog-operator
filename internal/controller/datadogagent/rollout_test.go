// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/orderedrollout"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

const (
	rolloutTestNS  = "bar"
	rolloutTestDDA = "foo"
)

type rolloutEnv struct {
	t        *testing.T
	c        client.Client
	r        *Reconciler
	recorder *record.FakeRecorder
}

func newRolloutProfile(name string, priority *int32, annotations map[string]string) *v1alpha1.DatadogAgentProfile {
	p := &v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: rolloutTestNS, Annotations: annotations},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []corev1.NodeSelectorRequirement{{Key: "pool", Operator: corev1.NodeSelectorOpIn, Values: []string{name}}},
			},
			Config: &v2alpha1.DatadogAgentSpec{
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {Labels: map[string]string{"pool": name}},
				},
			},
		},
	}
	if priority != nil {
		p.Spec.Rollout = &v2alpha1.RolloutStrategy{Priority: priority}
	}
	return p
}

func newRolloutEnv(t *testing.T, dda *v2alpha1.DatadogAgent, objs ...client.Object) *rolloutEnv {
	return newRolloutEnvWithInterceptor(t, interceptor.Funcs{}, dda, objs...)
}

func newRolloutEnvWithInterceptor(t *testing.T, funcs interceptor.Funcs, dda *v2alpha1.DatadogAgent, objs ...client.Object) *rolloutEnv {
	t.Helper()
	s := agenttestutils.TestScheme()
	crd, err := getDDAICRDFromConfig(s)
	require.NoError(t, err)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithInterceptorFuncs(funcs).
		WithStatusSubresource(&v2alpha1.DatadogAgent{}, &v1alpha1.DatadogAgentProfile{}, &v1alpha1.DatadogAgentInternal{}, &appsv1.DaemonSet{}).
		WithObjects(append(objs, crd, dda)...).
		Build()
	recorder := record.NewFakeRecorder(1000)
	r := &Reconciler{
		client:     c,
		scheme:     s,
		recorder:   recorder,
		log:        logf.Log.WithName(t.Name()),
		forwarders: dummyManager{},
		options:    ReconcilerOptions{DatadogAgentProfileEnabled: true},
	}
	r.initializeComponentRegistry()
	return &rolloutEnv{t: t, c: c, r: r, recorder: recorder}
}

func (e *rolloutEnv) dda() *v2alpha1.DatadogAgent {
	dda := &v2alpha1.DatadogAgent{}
	require.NoError(e.t, e.c.Get(context.TODO(), types.NamespacedName{Namespace: rolloutTestNS, Name: rolloutTestDDA}, dda))
	return dda
}

func (e *rolloutEnv) reconcile() {
	e.t.Helper()
	for range 3 {
		res, err := e.r.Reconcile(context.TODO(), e.dda())
		require.NoError(e.t, err)
		if !res.Requeue {
			return
		}
	}
}

func (e *rolloutEnv) updateDDA(mutate func(*v2alpha1.DatadogAgent)) {
	e.t.Helper()
	dda := e.dda()
	mutate(dda)
	require.NoError(e.t, e.c.Update(context.TODO(), dda))
}

func (e *rolloutEnv) ddai(name string) *v1alpha1.DatadogAgentInternal {
	e.t.Helper()
	ddai := &v1alpha1.DatadogAgentInternal{}
	require.NoError(e.t, e.c.Get(context.TODO(), types.NamespacedName{Namespace: rolloutTestNS, Name: name}, ddai))
	return ddai
}

func (e *rolloutEnv) profile(name string) *v1alpha1.DatadogAgentProfile {
	e.t.Helper()
	p := &v1alpha1.DatadogAgentProfile{}
	require.NoError(e.t, e.c.Get(context.TODO(), types.NamespacedName{Namespace: rolloutTestNS, Name: name}, p))
	return p
}

// setDaemonSet simulates the DDAI and DaemonSet controllers for a DDAI.
func (e *rolloutEnv) setDaemonSet(ddaiName string, updated int32) {
	e.t.Helper()
	dsName := "ds-" + ddaiName
	ddai := e.ddai(ddaiName)
	ddai.Status.ObservedGeneration = ddai.Generation
	ddai.Status.Agent = &v2alpha1.DaemonSetStatus{DaemonsetName: dsName}
	require.NoError(e.t, e.c.Status().Update(context.TODO(), ddai))

	ds := &appsv1.DaemonSet{}
	err := e.c.Get(context.TODO(), types.NamespacedName{Namespace: rolloutTestNS, Name: dsName}, ds)
	if err != nil {
		ds = &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: rolloutTestNS, Name: dsName}}
		require.NoError(e.t, e.c.Create(context.TODO(), ds))
	}
	ds.Status = appsv1.DaemonSetStatus{DesiredNumberScheduled: 2, UpdatedNumberScheduled: updated, NumberAvailable: updated}
	require.NoError(e.t, e.c.Status().Update(context.TODO(), ds))
}

func (e *rolloutEnv) specHash(name string) string {
	return e.ddai(name).Annotations[constants.MD5DDAIDeploymentAnnotationKey]
}

func (e *rolloutEnv) events() string {
	var out []string
	for {
		select {
		case ev := <-e.recorder.Events:
			out = append(out, ev)
		default:
			return strings.Join(out, "\n")
		}
	}
}

func rolloutTestDDAObject() *v2alpha1.DatadogAgent {
	dda := testutils.NewInitializedDatadogAgentBuilder(rolloutTestNS, rolloutTestDDA).Build()
	dda.Spec.Global.Tags = []string{"v1"}
	return dda
}

func setTags(tag string) func(*v2alpha1.DatadogAgent) {
	return func(dda *v2alpha1.DatadogAgent) { dda.Spec.Global.Tags = []string{tag} }
}

func stepByName(status *v2alpha1.RolloutStatus, name string) *v2alpha1.RolloutStepStatus {
	for i := range status.Steps {
		if status.Steps[i].Name == name {
			return &status.Steps[i]
		}
	}
	return nil
}

func TestRollout_DefaultSettingsRollInParallel(t *testing.T) {
	e := newRolloutEnv(t, rolloutTestDDAObject(), newRolloutProfile("a", nil, nil))
	e.reconcile()
	before := map[string]string{rolloutTestDDA: e.specHash(rolloutTestDDA), "a": e.specHash("a")}

	e.updateDDA(setTags("v2"))
	e.reconcile()

	for name, old := range before {
		assert.NotEqual(t, old, e.specHash(name), "%s not updated", name)
	}
	assert.Nil(t, e.dda().Status.Rollout)
	assert.Nil(t, e.profile("a").Status.Rollout)
	assert.NotContains(t, e.events(), "Rollout")
}

func TestRollout_PriorityWaves(t *testing.T) {
	e := newRolloutEnv(t, rolloutTestDDAObject(),
		newRolloutProfile("a", nil, nil),
		newRolloutProfile("b", ptr.To[int32](10), nil))
	e.reconcile()
	for _, n := range []string{rolloutTestDDA, "a", "b"} {
		e.setDaemonSet(n, 2)
	}
	e.reconcile()
	status := e.dda().Status.Rollout
	require.NotNil(t, status)
	assert.Equal(t, v2alpha1.RolloutPhaseCompleted, status.Phase)
	oldB := e.specHash("b")
	e.events()

	e.updateDDA(setTags("v2"))
	e.reconcile()
	status = e.dda().Status.Rollout
	assert.Equal(t, v2alpha1.RolloutPhaseInProgress, status.Phase)
	assert.Equal(t, int32(0), *status.CurrentPriority)
	assert.Equal(t, oldB, e.specHash("b"), "priority 10 must wait")
	assert.Equal(t, v2alpha1.RolloutPhasePending, stepByName(status, "b").Phase)
	assert.Equal(t, v2alpha1.RolloutPhasePending, e.profile("b").Status.Rollout.Phase)
	assert.Contains(t, status.Warnings, orderedrollout.WarningNoStepTimeout)
	startedAt := stepByName(status, "default").StartedAt
	require.NotNil(t, startedAt)

	// The default DaemonSet is still rolling: b keeps waiting.
	e.setDaemonSet(rolloutTestDDA, 1)
	e.setDaemonSet("a", 2)
	e.reconcile()
	status = e.dda().Status.Rollout
	assert.Equal(t, oldB, e.specHash("b"))
	assert.Equal(t, startedAt, stepByName(status, "default").StartedAt, "startedAt is persisted")
	assert.Equal(t, v2alpha1.RolloutPhaseCompleted, stepByName(status, "a").Phase)

	// Priority 0 completes: b is written.
	e.setDaemonSet(rolloutTestDDA, 2)
	e.reconcile()
	status = e.dda().Status.Rollout
	assert.NotEqual(t, oldB, e.specHash("b"))
	assert.Equal(t, int32(10), *status.CurrentPriority)
	assert.Equal(t, v2alpha1.RolloutPhaseInProgress, e.profile("b").Status.Rollout.Phase)

	e.setDaemonSet("b", 2)
	e.reconcile()
	assert.Equal(t, v2alpha1.RolloutPhaseCompleted, e.dda().Status.Rollout.Phase)
	events := e.events()
	assert.Contains(t, events, "RolloutStepStarted")
	assert.Contains(t, events, "RolloutCompleted")
}

func TestRollout_BypassConsumedOnce(t *testing.T) {
	e := newRolloutEnv(t, rolloutTestDDAObject(),
		newRolloutProfile("a", nil, nil),
		newRolloutProfile("b", ptr.To[int32](10), nil))
	e.reconcile()

	oldB := e.specHash("b")
	e.updateDDA(func(dda *v2alpha1.DatadogAgent) {
		setTags("v2")(dda)
		dda.Annotations = map[string]string{orderedrollout.BypassAnnotation: "1"}
	})
	e.reconcile()
	assert.NotEqual(t, oldB, e.specHash("b"), "bypass rolls every step")
	status := e.dda().Status.Rollout
	assert.Equal(t, "1", status.LastBypass)
	assert.Equal(t, "1", e.dda().Annotations[orderedrollout.BypassAnnotation], "annotation is not mutated")
	assert.Contains(t, e.events(), "RolloutBypassed")
	assert.NotContains(t, e.ddai("b").Annotations, orderedrollout.BypassAnnotation)

	for _, n := range []string{rolloutTestDDA, "a", "b"} {
		e.setDaemonSet(n, 2)
	}
	e.reconcile()
	assert.Empty(t, e.dda().Status.Rollout.BypassRolloutID)

	// The same value does not bypass the next rollout.
	oldB = e.specHash("b")
	e.updateDDA(setTags("v3"))
	e.reconcile()
	assert.Equal(t, oldB, e.specHash("b"))
	assert.Equal(t, v2alpha1.RolloutPhaseInProgress, e.dda().Status.Rollout.Phase)
}

func TestRollout_GatedStepGetsInertAnnotationsOnly(t *testing.T) {
	e := newRolloutEnv(t, rolloutTestDDAObject(),
		newRolloutProfile("a", nil, map[string]string{orderedrollout.HoldAnnotation: "true"}),
		newRolloutProfile("b", ptr.To[int32](10), nil))
	e.reconcile()
	oldA, oldB := e.ddai("a"), e.ddai("b")

	e.updateDDA(func(dda *v2alpha1.DatadogAgent) {
		setTags("v2")(dda)
		dda.Annotations = map[string]string{"meta.helm.sh/release-name": "datadog"}
	})
	e.reconcile()

	status := e.dda().Status.Rollout
	assert.Equal(t, orderedrollout.ReasonDAPHold, stepByName(status, "a").Reason)
	assert.Equal(t, v2alpha1.RolloutPhasePending, stepByName(status, "b").Phase)
	assert.Equal(t, v2alpha1.RolloutPhaseHeld, e.profile("a").Status.Rollout.Phase)
	for _, old := range []*v1alpha1.DatadogAgentInternal{oldA, oldB} {
		live := e.ddai(old.Name)
		assert.Equal(t, old.Spec, live.Spec, "%s spec leaked", old.Name)
		assert.Equal(t, old.Annotations[constants.MD5DDAIDeploymentAnnotationKey], live.Annotations[constants.MD5DDAIDeploymentAnnotationKey])
		assert.Equal(t, old.Annotations[orderedrollout.TargetHashAnnotation], live.Annotations[orderedrollout.TargetHashAnnotation])
		assert.Equal(t, "datadog", live.Annotations["meta.helm.sh/release-name"], "%s inert annotation not synced", old.Name)
		assert.NotContains(t, live.Annotations, orderedrollout.HoldAnnotation)
	}
	assert.Equal(t, "datadog", e.ddai(rolloutTestDDA).Annotations["meta.helm.sh/release-name"])
	assert.Contains(t, e.events(), "RolloutStepHeld")
}

func TestRollout_PartialWriteFailure(t *testing.T) {
	failA := false
	funcs := interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*v1alpha1.DatadogAgentInternal); ok && failA && obj.GetName() == "a" {
				return errors.New("injected")
			}
			return c.Update(ctx, obj, opts...)
		},
	}
	e := newRolloutEnvWithInterceptor(t, funcs, rolloutTestDDAObject(),
		newRolloutProfile("a", nil, nil),
		newRolloutProfile("b", ptr.To[int32](10), nil))
	e.reconcile()
	oldA := e.specHash("a")

	failA = true
	e.updateDDA(setTags("v2"))
	_, err := e.r.Reconcile(context.TODO(), e.dda())
	require.Error(t, err)
	status := e.dda().Status.Rollout
	assert.Equal(t, oldA, e.specHash("a"))
	assert.Equal(t, v2alpha1.RolloutPhaseBlocked, stepByName(status, "a").Phase)
	assert.Nil(t, stepByName(status, "a").StartedAt)
	assert.NotNil(t, stepByName(status, "default").StartedAt, "the written step stays started")
	assert.Equal(t, v2alpha1.RolloutPhasePending, stepByName(status, "b").Phase)

	failA = false
	e.reconcile()
	assert.NotEqual(t, oldA, e.specHash("a"))
	assert.Equal(t, v2alpha1.RolloutPhaseInProgress, stepByName(e.dda().Status.Rollout, "a").Phase)
}

func TestRollout_ControlAnnotationsDoNotRewriteDDAIs(t *testing.T) {
	e := newRolloutEnv(t, rolloutTestDDAObject(), newRolloutProfile("b", ptr.To[int32](10), nil))
	e.reconcile()
	rv := map[string]string{rolloutTestDDA: e.ddai(rolloutTestDDA).ResourceVersion, "b": e.ddai("b").ResourceVersion}

	e.updateDDA(func(dda *v2alpha1.DatadogAgent) {
		dda.Annotations = map[string]string{orderedrollout.HoldAnnotation: "true", orderedrollout.BypassAnnotation: "x"}
		dda.Spec.RolloutStrategy = &v2alpha1.RolloutStrategy{StepTimeout: &metav1.Duration{Duration: time.Hour}}
	})
	e.reconcile()
	for name, v := range rv {
		assert.Equal(t, v, e.ddai(name).ResourceVersion, "%s rewritten", name)
	}
}

func TestRolloutNormalization(t *testing.T) {
	r := &Reconciler{scheme: agenttestutils.TestScheme()}
	dda := rolloutTestDDAObject()
	plain, err := r.generateDDAIFromDDA(dda, "")
	require.NoError(t, err)

	dda.Annotations = map[string]string{
		orderedrollout.HoldAnnotation:   "true",
		orderedrollout.BypassAnnotation: "x",
	}
	dda.Spec.RolloutStrategy = &v2alpha1.RolloutStrategy{Priority: ptr.To[int32](3), StepTimeout: &metav1.Duration{Duration: time.Minute}}
	withRollout, err := r.generateDDAIFromDDA(dda, "")
	require.NoError(t, err)

	assert.Nil(t, withRollout.Spec.RolloutStrategy)
	assert.NotContains(t, withRollout.Annotations, orderedrollout.HoldAnnotation)
	assert.NotContains(t, withRollout.Annotations, orderedrollout.BypassAnnotation)
	plainHash, err := targetHashForDDAI(plain)
	require.NoError(t, err)
	rolloutHash, err := targetHashForDDAI(withRollout)
	require.NoError(t, err)
	assert.Equal(t, plainHash, rolloutHash, "rollout settings do not change the target hash")

	// A rendering annotation changes the target hash.
	dda.Annotations[kubernetes.ProviderAnnotationKey] = "gke-cos"
	withProvider, err := r.generateDDAIFromDDA(dda, "")
	require.NoError(t, err)
	providerHash, err := targetHashForDDAI(withProvider)
	require.NoError(t, err)
	assert.NotEqual(t, plainHash, providerHash)
}

func TestRolloutPlan_LiveHashFallback(t *testing.T) {
	dda := rolloutTestDDAObject()
	e := newRolloutEnv(t, dda)
	rendered, err := e.r.generateDDAIFromDDA(dda, "")
	require.NoError(t, err)

	// A live DDAI written before the target hash annotation existed.
	live := rendered.DeepCopy()
	require.NoError(t, e.c.Create(context.TODO(), live))

	plan, err := e.r.buildRolloutPlan(context.TODO(), dda, nil, []*v1alpha1.DatadogAgentInternal{rendered}, time.Now())
	require.NoError(t, err)
	step := plan.inputs.Steps[0]
	require.NotNil(t, step.Live)
	assert.Equal(t, step.TargetHash, step.Live.TargetHash)
	assert.True(t, step.Live.AnnotationsDiffer, "the target hash annotation is still missing")
	assert.False(t, step.Live.InertAnnotationsDiffer)
	assert.Equal(t, step.TargetHash, rendered.Annotations[orderedrollout.TargetHashAnnotation])
}

func TestRolloutStatusCarryOverAndEquality(t *testing.T) {
	rollout := &v2alpha1.RolloutStatus{Phase: v2alpha1.RolloutPhaseInProgress, LastBypass: "x"}
	status := generateNewStatusFromDDA(&v2alpha1.DatadogAgentStatus{Rollout: rollout})
	assert.Equal(t, rollout, status.Rollout)
	assert.NotSame(t, rollout, status.Rollout)

	other := status.DeepCopy()
	assert.True(t, IsEqualStatus(status, other))
	other.Rollout.Phase = v2alpha1.RolloutPhaseCompleted
	assert.False(t, IsEqualStatus(status, other))
}

func TestSetRolloutConditions(t *testing.T) {
	now := metav1.Now()
	status := &v2alpha1.DatadogAgentStatus{Rollout: &v2alpha1.RolloutStatus{
		Phase: v2alpha1.RolloutPhaseInProgress,
		Steps: []v2alpha1.RolloutStepStatus{{Name: "a", Phase: v2alpha1.RolloutPhaseHeld}},
	}}
	setRolloutConditions(status, now)
	get := func(t string) metav1.ConditionStatus {
		for _, c := range status.Conditions {
			if c.Type == t {
				return c.Status
			}
		}
		return ""
	}
	assert.Equal(t, metav1.ConditionTrue, get("RolloutProgressing"))
	assert.Equal(t, metav1.ConditionFalse, get("RolloutComplete"))
	assert.Equal(t, metav1.ConditionTrue, get("RolloutHeld"))
	assert.Equal(t, metav1.ConditionFalse, get("RolloutTimedOut"))

	empty := &v2alpha1.DatadogAgentStatus{}
	setRolloutConditions(empty, now)
	assert.Empty(t, empty.Conditions)
}
