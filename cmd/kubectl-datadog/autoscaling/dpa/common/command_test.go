// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

var pauseSpec = AnnotationSpec{
	Key:            PauseAnnotationKey,
	Value:          SetValue("true"),
	PastTense:      "paused",
	UntouchedState: "paused",
	Confirmation:   PauseConfirmation,
}

var unpauseSpec = AnnotationSpec{
	Key:            PauseAnnotationKey,
	Value:          RemoveKey,
	PastTense:      "unpaused",
	UntouchedState: "not paused",
}

type commandTest struct {
	opts   *annotationOptions
	out    *bytes.Buffer
	errOut *bytes.Buffer
}

func newCommandTest(t *testing.T, c client.Client, spec AnnotationSpec, sel selection, stdin string) *commandTest {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	o := &annotationOptions{
		IOStreams: genericclioptions.IOStreams{In: bytes.NewBufferString(stdin), Out: out, ErrOut: errOut},
		selection: sel,
		spec:      spec,
		yes:       true,
		wait:      false,
	}
	o.Client = c
	o.UserNamespace = "ns"
	return &commandTest{opts: o, out: out, errOut: errOut}
}

func annotationsOf(t *testing.T, c client.Client, name string) map[string]string {
	t.Helper()
	return getDPA(t, c, "ns", name).Annotations
}

func setTerminal(t *testing.T, terminal bool) {
	t.Helper()
	orig := isTerminal
	isTerminal = func(io.Reader) bool { return terminal }
	t.Cleanup(func() { isTerminal = orig })
}

// unreachableKubeconfig writes a kubeconfig for a server that refuses connections.
func unreachableKubeconfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(path, []byte(`apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:1
contexts:
- name: c
  context:
    cluster: c
    namespace: ns
current-context: c
`), 0o600))
	return path
}

func TestNewAnnotationCommand(t *testing.T) {
	validateErr := errors.New("invalid flags")
	for _, tc := range []struct {
		name    string
		spec    AnnotationSpec
		args    []string
		wantErr string
	}{
		{name: "selection is required", spec: pauseSpec, args: nil, wantErr: "select DatadogPodAutoscalers"},
		{name: "spec validation runs before connecting", spec: AnnotationSpec{Key: PauseAnnotationKey, Value: SetValue("true"), Validate: func() error { return validateErr }}, args: []string{"a"}, wantErr: "invalid flags"},
		{name: "unreachable cluster", spec: pauseSpec, args: []string{"a"}, wantErr: "unable to get DatadogPodAutoscaler ns/a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			streams := genericclioptions.NewTestIOStreamsDiscard()
			cmd := NewAnnotationCommand(streams, tc.spec, &cobra.Command{Use: "test"})
			cmd.SetArgs(append(tc.args, "--kubeconfig", unreachableKubeconfig(t)))
			cmd.SetOut(streams.Out)
			cmd.SetErr(streams.ErrOut)
			assert.ErrorContains(t, cmd.Execute(), tc.wantErr)
		})
	}
}

func TestNewAnnotationCommandFlags(t *testing.T) {
	streams := genericclioptions.NewTestIOStreamsDiscard()
	withWait := NewAnnotationCommand(streams, pauseSpec, &cobra.Command{Use: "test"})
	assert.Equal(t, "true", withWait.Flags().Lookup("wait").DefValue)
	assert.Equal(t, "30s", withWait.Flags().Lookup("timeout").DefValue)

	withoutWait := NewAnnotationCommand(streams, unpauseSpec, &cobra.Command{Use: "test"})
	assert.Nil(t, withoutWait.Flags().Lookup("wait"), "no Confirmation, nothing to wait for")
}

func TestCommandPauseBySelector(t *testing.T) {
	c := newFakeClient(t,
		newDPA("ns", "fresh", nil, map[string]string{"app": "web"}),
		newDPA("ns", "already", map[string]string{PauseAnnotationKey: "true"}, map[string]string{"app": "web"}),
		newDPA("ns", "invalid", map[string]string{PauseAnnotationKey: "yes"}, map[string]string{"app": "web"}),
		newDPA("ns", "other-app", nil, map[string]string{"app": "api"}),
	)
	tt := newCommandTest(t, c, pauseSpec, selection{labelSelector: "app=web"}, "")

	require.NoError(t, tt.opts.run(context.Background()))

	assert.Equal(t, "true", annotationsOf(t, c, "fresh")[PauseAnnotationKey])
	assert.Equal(t, "true", annotationsOf(t, c, "already")[PauseAnnotationKey])
	assert.Equal(t, "true", annotationsOf(t, c, "invalid")[PauseAnnotationKey], "an invalid value is ignored by the Cluster Agent, so it counts as not paused")
	assert.NotContains(t, annotationsOf(t, c, "other-app"), PauseAnnotationKey)

	assert.Contains(t, tt.errOut.String(), "1 DatadogPodAutoscaler(s) already paused")
	assert.Contains(t, tt.out.String(), "ns/fresh paused")
	assert.Contains(t, tt.out.String(), "ns/invalid paused")
	assert.NotContains(t, tt.out.String(), "ns/already paused")
}

func TestCommandRemoveKey(t *testing.T) {
	c := newFakeClient(t,
		newDPA("ns", "paused", map[string]string{PauseAnnotationKey: "true", "other": "x"}, nil),
		newDPA("ns", "false", map[string]string{PauseAnnotationKey: "false"}, nil),
		newDPA("ns", "absent", nil, nil),
	)
	tt := newCommandTest(t, c, unpauseSpec, selection{all: true}, "")

	require.NoError(t, tt.opts.run(context.Background()))

	assert.Equal(t, map[string]string{"other": "x"}, annotationsOf(t, c, "paused"), "unpause removes the key, it never writes false")
	assert.NotContains(t, annotationsOf(t, c, "false"), PauseAnnotationKey, "any leftover value is removed")
	assert.Contains(t, tt.errOut.String(), "1 DatadogPodAutoscaler(s) already not paused")
}

func TestCommandDryRun(t *testing.T) {
	c := newFakeClient(t, newDPA("ns", "b", nil, nil), newDPA("ns", "a", nil, nil))
	tt := newCommandTest(t, c, pauseSpec, selection{all: true}, "")
	tt.opts.dryRun = true

	require.NoError(t, tt.opts.run(context.Background()))

	assert.Equal(t, "ns/a: <none> → true\nns/b: <none> → true\n", tt.out.String(), "dry-run prints one line per object and nothing else on stdout")
	assert.NotContains(t, annotationsOf(t, c, "a"), PauseAnnotationKey)
}

func TestCommandNothingToDo(t *testing.T) {
	c := newFakeClient(t, newDPA("ns", "a", map[string]string{PauseAnnotationKey: "true"}, nil))
	tt := newCommandTest(t, c, pauseSpec, selection{all: true}, "")

	require.NoError(t, tt.opts.run(context.Background()))
	assert.Empty(t, tt.out.String())
}

func TestCommandConfirmation(t *testing.T) {
	newClient := func() client.Client {
		return newFakeClient(t, newDPA("ns", "a", nil, nil), newDPA("ns", "b", nil, nil))
	}

	t.Run("no terminal and no --yes fails", func(t *testing.T) {
		setTerminal(t, false)
		c := newClient()
		tt := newCommandTest(t, c, pauseSpec, selection{all: true}, "")
		tt.opts.yes = false

		assert.ErrorIs(t, tt.opts.run(context.Background()), errNoTerminal)
		assert.NotContains(t, annotationsOf(t, c, "a"), PauseAnnotationKey)
	})

	t.Run("declined", func(t *testing.T) {
		setTerminal(t, true)
		c := newClient()
		tt := newCommandTest(t, c, pauseSpec, selection{all: true}, "n\n")
		tt.opts.yes = false

		require.NoError(t, tt.opts.run(context.Background()))
		assert.Contains(t, tt.out.String(), "Cancelled.")
		assert.NotContains(t, annotationsOf(t, c, "a"), PauseAnnotationKey)
	})

	t.Run("accepted", func(t *testing.T) {
		setTerminal(t, true)
		c := newClient()
		tt := newCommandTest(t, c, pauseSpec, selection{all: true}, "y\n")
		tt.opts.yes = false

		require.NoError(t, tt.opts.run(context.Background()))
		assert.Contains(t, tt.out.String(), "2 DatadogPodAutoscaler(s) selected; the following 2 will be paused")
		assert.Equal(t, "true", annotationsOf(t, c, "a")[PauseAnnotationKey])
	})

	t.Run("objects already in the requested state count", func(t *testing.T) {
		setTerminal(t, false)
		c := newFakeClient(t, newDPA("ns", "a", map[string]string{PauseAnnotationKey: "true"}, nil), newDPA("ns", "b", nil, nil))
		tt := newCommandTest(t, c, pauseSpec, selection{all: true}, "")
		tt.opts.yes = false

		assert.ErrorIs(t, tt.opts.run(context.Background()), errNoTerminal)
		assert.NotContains(t, annotationsOf(t, c, "b"), PauseAnnotationKey)
	})

	t.Run("a single object needs no confirmation", func(t *testing.T) {
		setTerminal(t, false)
		c := newClient()
		tt := newCommandTest(t, c, pauseSpec, selection{names: []string{"a"}}, "")
		tt.opts.yes = false

		require.NoError(t, tt.opts.run(context.Background()))
		assert.Equal(t, "true", annotationsOf(t, c, "a")[PauseAnnotationKey])
	})
}

// agentClient is a fake client where every patch is followed by agent(dpa),
// standing in for the Cluster Agent updating the status.
func agentClient(t *testing.T, dpa *v1alpha2.DatadogPodAutoscaler, agent func(*v1alpha2.DatadogPodAutoscaler)) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(s))
	return fake.NewClientBuilder().WithScheme(s).WithObjects(dpa).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if err := c.Patch(ctx, obj, patch, opts...); err != nil {
					return err
				}
				current := &v1alpha2.DatadogPodAutoscaler{}
				require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(obj), current))
				agent(current)
				return c.Update(ctx, current)
			},
		}).Build()
}

func setCondition(conditionType datadoghqcommon.DatadogPodAutoscalerConditionType, status corev1.ConditionStatus, reason, message string) func(*v1alpha2.DatadogPodAutoscaler) {
	return func(dpa *v1alpha2.DatadogPodAutoscaler) {
		dpa.Status.Conditions = nil
		withCondition(dpa, conditionType, status, reason, message)
	}
}

func TestCommandWait(t *testing.T) {
	origInterval := waitInterval
	waitInterval = 10 * time.Millisecond
	t.Cleanup(func() { waitInterval = origInterval })

	forceReplicas := AnnotationSpec{Key: ForceReplicasAnnotationKey, Value: SetValue("28"), PastTense: "forced", UntouchedState: "forced", Confirmation: ForceReplicasConfirmation}
	forceResources := AnnotationSpec{Key: ForceResourcesAnnotationKey, Value: SetValue(`[{"name":"app","requests":{"cpu":"2"}}]`), PastTense: "forced", UntouchedState: "forced", Confirmation: ForceResourcesConfirmation}
	unpause := unpauseSpec
	unpause.Confirmation = PauseConfirmation
	paused := func(dpa *v1alpha2.DatadogPodAutoscaler) *v1alpha2.DatadogPodAutoscaler {
		return withCondition(dpa, active, corev1.ConditionFalse, LocallyPausedReason, pausedMsg)
	}

	forceFallback := AnnotationSpec{Key: ForceFallbackAnnotationKey, Value: SetValue("true"), PastTense: "forced", UntouchedState: "forced", Confirmation: ForceFallbackConfirmation}

	for _, tc := range []struct {
		name    string
		before  *v1alpha2.DatadogPodAutoscaler
		agent   func(*v1alpha2.DatadogPodAutoscaler)
		spec    AnnotationSpec
		wantErr string
		wantOut string
	}{
		{
			name:    "advisory, never reported, warns without failing",
			before:  newDPA("ns", "a", nil, nil),
			agent:   func(*v1alpha2.DatadogPodAutoscaler) {},
			spec:    forceFallback,
			wantOut: "Warning: Not confirmed by the Cluster Agent",
		},
		{
			name:    "set, confirmed",
			before:  withCondition(newDPA("ns", "a", nil, nil), active, corev1.ConditionTrue, "", ""),
			agent:   setCondition(active, corev1.ConditionFalse, LocallyPausedReason, pausedMsg),
			spec:    pauseSpec,
			wantOut: "Confirmed by the Cluster Agent",
		},
		{
			name:    "set, never reported",
			before:  newDPA("ns", "a", nil, nil),
			agent:   func(*v1alpha2.DatadogPodAutoscaler) {},
			spec:    pauseSpec,
			wantErr: "could not be paused",
			wantOut: "Active=False with reason LocallyPaused is not reported: the Cluster Agent is older than 7.85.0",
		},
		{
			name:    "set, the count is in the status",
			before:  newDPA("ns", "a", nil, nil),
			agent:   setCondition(hLimited, corev1.ConditionTrue, "", pinned28),
			spec:    forceReplicas,
			wantOut: "Confirmed by the Cluster Agent",
		},
		{
			name:    "set, a previous count is not a confirmation",
			before:  withCondition(newDPA("ns", "a", map[string]string{ForceReplicasAnnotationKey: "3"}, nil), hLimited, corev1.ConditionTrue, "", "replica count pinned to 3 by the autoscaling.datadoghq.com/force-replicas annotation"),
			agent:   func(*v1alpha2.DatadogPodAutoscaler) {},
			spec:    forceReplicas,
			wantErr: "could not be forced",
			wantOut: "not while paused, in Preview mode",
		},
		{
			name:    "set, reported from before cannot confirm the new value",
			before:  withCondition(newDPA("ns", "a", map[string]string{ForceResourcesAnnotationKey: `[{"name":"app","requests":{"cpu":"1"}}]`}, nil), vLimited, corev1.ConditionTrue, ForcedByAnnotationReason, forcedApp),
			agent:   func(*v1alpha2.DatadogPodAutoscaler) {},
			spec:    forceResources,
			wantOut: "already reported VerticalScalingLimited with reason ForcedByAnnotation before this change",
		},
		{
			name:    "set, resources need a vertical recommendation",
			before:  newDPA("ns", "a", nil, nil),
			agent:   func(*v1alpha2.DatadogPodAutoscaler) {},
			spec:    forceResources,
			wantErr: "could not be forced",
			wantOut: "only reported while the DatadogPodAutoscaler has a vertical recommendation",
		},
		{
			name:    "removal, confirmed once no longer paused",
			before:  paused(newDPA("ns", "a", map[string]string{PauseAnnotationKey: "true"}, nil)),
			agent:   setCondition(active, corev1.ConditionTrue, "", ""),
			spec:    unpause,
			wantOut: "Confirmed by the Cluster Agent",
		},
		{
			name:    "removal, still paused",
			before:  paused(newDPA("ns", "a", map[string]string{PauseAnnotationKey: "true"}, nil)),
			agent:   func(*v1alpha2.DatadogPodAutoscaler) {},
			spec:    unpause,
			wantErr: "could not be unpaused",
			wantOut: "Active=False with reason LocallyPaused is still reported",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tt := newCommandTest(t, agentClient(t, tc.before, tc.agent), tc.spec, selection{all: true}, "")
			tt.opts.wait = true
			tt.opts.timeout = 200 * time.Millisecond

			err := tt.opts.run(context.Background())
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
			assert.Contains(t, tt.out.String()+tt.errOut.String(), tc.wantOut)
		})
	}
}

func TestWaitForConfirmationIgnoresDeletedObjects(t *testing.T) {
	tt := newCommandTest(t, newFakeClient(t), pauseSpec, selection{}, "")
	tt.opts.wait = true
	tt.opts.timeout = time.Second

	unconfirmed, err := tt.opts.waitForConfirmation(context.Background(), []change{{dpa: *newDPA("ns", "gone", nil, nil), value: Set("true")}})
	require.NoError(t, err)
	assert.Zero(t, unconfirmed)
}

// listErrClient is a fake client whose n-th List (from 1) fails with
// listErr(n), or succeeds when it returns nil.
func listErrClient(t *testing.T, listErr func(n int32) error, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(s))
	var calls atomic.Int32
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if err := listErr(calls.Add(1)); err != nil {
					return err
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()
}

func TestWaitForConfirmationRetriesListErrors(t *testing.T) {
	origInterval := waitInterval
	waitInterval = 10 * time.Millisecond
	t.Cleanup(func() { waitInterval = origInterval })
	rateLimited := errors.New("client rate limiter Wait returned an error: rate: Wait(n=1) would exceed context deadline")
	forbidden := apierrors.NewForbidden(schema.GroupResource{Group: "datadoghq.com", Resource: "datadogpodautoscalers"}, "", errors.New(`User "u" cannot list resource "datadogpodautoscalers"`))

	for _, tc := range []struct {
		name       string
		listErr    func(n int32) error
		wantOut    []string
		notWantOut []string
	}{
		{
			name: "rate limiter at the deadline is a timeout",
			listErr: func(n int32) error {
				if n > 1 {
					return rateLimited
				}
				return nil
			},
			wantOut:    []string{"is not reported: the Cluster Agent is older than"},
			notWantOut: []string{"Unable to read"},
		},
		{
			name: "the real cause is kept over a later rate-limiter error",
			listErr: func(n int32) error {
				if n > 2 {
					return rateLimited
				}
				return forbidden
			},
			wantOut:    []string{"Unable to read the DatadogPodAutoscaler status: " + forbidden.Error()},
			notWantOut: []string{"rate limiter", "older than", "Check with"},
		},
		{
			name: "a failed first tick then a successful List",
			listErr: func(n int32) error {
				if n == 1 {
					return forbidden
				}
				return nil
			},
			wantOut:    []string{"is not reported: the Cluster Agent is older than"},
			notWantOut: []string{"Unable to read"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dpa := newDPA("ns", "a", nil, nil)
			tt := newCommandTest(t, listErrClient(t, tc.listErr, dpa), pauseSpec, selection{}, "")
			tt.opts.timeout = 60 * time.Millisecond

			unconfirmed, err := tt.opts.waitForConfirmation(context.Background(), []change{{dpa: *dpa, value: Set("true")}})
			require.NoError(t, err, "a List error is not a status check failure")
			assert.Equal(t, 1, unconfirmed)
			out := tt.errOut.String()
			assert.Contains(t, out, "Not confirmed by the Cluster Agent after 60ms:\n  • ns/a")
			for _, want := range tc.wantOut {
				assert.Contains(t, out, want)
			}
			for _, notWant := range tc.notWantOut {
				assert.NotContains(t, out, notWant)
			}
		})
	}
}

func TestWaitForConfirmationInterrupted(t *testing.T) {
	tt := newCommandTest(t, newFakeClient(t, newDPA("ns", "a", nil, nil)), pauseSpec, selection{}, "")
	tt.opts.timeout = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tt.opts.waitForConfirmation(ctx, []change{{dpa: *newDPA("ns", "a", nil, nil), value: Set("true")}})
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, tt.errOut.String(), "Not confirmed", "an interruption is not a Cluster Agent timeout")
}

func TestWaitForConfirmationMixedResults(t *testing.T) {
	origInterval := waitInterval
	waitInterval = 10 * time.Millisecond
	t.Cleanup(func() { waitInterval = origInterval })

	notForced := newDPA("ns", "set", nil, nil)
	stillForced := withCondition(newDPA("ns", "removed", nil, nil), vLimited, corev1.ConditionTrue, ForcedByAnnotationReason, forcedApp)
	spec := AnnotationSpec{Key: ForceResourcesAnnotationKey, PastTense: "forced", UntouchedState: "forced", Confirmation: ForceResourcesConfirmation}
	tt := newCommandTest(t, newFakeClient(t, notForced, stillForced), spec, selection{}, "")
	tt.opts.timeout = 50 * time.Millisecond

	unconfirmed, err := tt.opts.waitForConfirmation(context.Background(), []change{
		{dpa: *notForced, value: Set(`[{"name":"app","requests":{"cpu":"2"}}]`)},
		{dpa: *stillForced, value: Removed},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, unconfirmed)
	out := tt.errOut.String()
	assert.Regexp(t, `ns/set\n.*is not reported`, out)
	assert.Regexp(t, `ns/removed\n.*is still reported`, out)
}

func TestAnnotationValueAppliedTo(t *testing.T) {
	const key = PauseAnnotationKey
	for _, tc := range []struct {
		name        string
		annotations map[string]string
		value       AnnotationValue
		want        bool
	}{
		{"set, absent", nil, Set("true"), false},
		{"set, same", map[string]string{key: "true"}, Set("true"), true},
		{"set, different", map[string]string{key: "false"}, Set("true"), false},
		{"set empty, present and empty", map[string]string{key: ""}, Set(""), true},
		{"set empty, absent", nil, Set(""), false},
		{"remove, absent", map[string]string{"other": "x"}, Removed, true},
		{"remove, present and empty", map[string]string{key: ""}, Removed, false},
		{"remove, present", map[string]string{key: "true"}, Removed, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.value.appliedTo(tc.annotations, key))
		})
	}
}

func TestCommandDescribeEmptyAnnotation(t *testing.T) {
	c := newFakeClient(t, newDPA("ns", "a", map[string]string{PauseAnnotationKey: ""}, nil))
	tt := newCommandTest(t, c, unpauseSpec, selection{all: true}, "")
	tt.opts.dryRun = true

	require.NoError(t, tt.opts.run(context.Background()))
	assert.Contains(t, tt.out.String(), `ns/a: "" → <none>`, "an empty annotation is not shown as absent")
}

func TestCommandInterruptedAtPrompt(t *testing.T) {
	setTerminal(t, true)
	c := newFakeClient(t, newDPA("ns", "a", nil, nil), newDPA("ns", "b", nil, nil))
	stdin, w := io.Pipe() // never answered
	// Closing the writer unblocks the confirm reader goroutine with EOF.
	t.Cleanup(func() { _ = w.Close() })
	tt := newCommandTest(t, c, pauseSpec, selection{all: true}, "")
	tt.opts.In = stdin
	tt.opts.yes = false
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	assert.ErrorIs(t, tt.opts.run(ctx), context.Canceled)
	assert.NotContains(t, annotationsOf(t, c, "a"), PauseAnnotationKey)
	assert.NotContains(t, annotationsOf(t, c, "b"), PauseAnnotationKey)
}

func TestCommandInterruptedWhilePatching(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(s))
	ctx, cancel := context.WithCancel(context.Background())
	c := fake.NewClientBuilder().WithScheme(s).
		WithObjects(newDPA("ns", "a", nil, nil), newDPA("ns", "b", nil, nil), newDPA("ns", "c", nil, nil)).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				defer cancel() // Ctrl-C right after the first patch
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	tt := newCommandTest(t, c, pauseSpec, selection{all: true}, "")

	assert.ErrorIs(t, tt.opts.run(ctx), context.Canceled)
	assert.Equal(t, "true", annotationsOf(t, c, "a")[PauseAnnotationKey])
	assert.NotContains(t, annotationsOf(t, c, "b"), PauseAnnotationKey)
	assert.Contains(t, tt.errOut.String(), "Interrupted: 1 DatadogPodAutoscaler(s) paused, 0 failed, 2 left untouched.")
	assert.NotContains(t, tt.errOut.String(), "ns/b: not paused", "no error line per remaining object")
}

func TestCommandReportsFailedPatches(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(s))
	c := fake.NewClientBuilder().WithScheme(s).
		WithObjects(newDPA("ns", "a", nil, nil), newDPA("ns", "b", nil, nil)).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				if obj.GetName() == "b" {
					return errors.New("injected failure")
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	tt := newCommandTest(t, c, pauseSpec, selection{all: true}, "")

	err := tt.opts.run(context.Background())
	assert.ErrorContains(t, err, "1 DatadogPodAutoscaler(s) could not be paused")
	assert.Equal(t, "true", annotationsOf(t, c, "a")[PauseAnnotationKey], "a failure does not stop the other objects")
	assert.Contains(t, tt.errOut.String(), "ns/b: not paused")
}

func TestCommandSetsArbitraryValue(t *testing.T) {
	c := newFakeClient(t,
		newDPA("ns", "absent", nil, nil),
		newDPA("ns", "same", map[string]string{ForceReplicasAnnotationKey: "5"}, nil),
		newDPA("ns", "different", map[string]string{ForceReplicasAnnotationKey: "3"}, nil),
	)
	spec := AnnotationSpec{Key: ForceReplicasAnnotationKey, Value: SetValue("5"), PastTense: "forced", UntouchedState: "forced"}
	tt := newCommandTest(t, c, spec, selection{all: true}, "")

	require.NoError(t, tt.opts.run(context.Background()))

	for _, name := range []string{"absent", "same", "different"} {
		assert.Equal(t, "5", annotationsOf(t, c, name)[ForceReplicasAnnotationKey], name)
	}
	assert.Contains(t, tt.errOut.String(), "1 DatadogPodAutoscaler(s) already forced")
	assert.NotContains(t, tt.out.String(), "ns/same forced")
}

func TestCommandValueErrorFailsOnlyThatObject(t *testing.T) {
	c := newFakeClient(t, newDPA("ns", "bad", nil, nil), newDPA("ns", "good", nil, nil))
	spec := AnnotationSpec{
		Key: ForceReplicasAnnotationKey,
		Value: func(dpa *v1alpha2.DatadogPodAutoscaler) (AnnotationValue, error) {
			if dpa.Name == "bad" {
				return AnnotationValue{}, errors.New("cannot compute")
			}
			return Set("3"), nil
		},
		PastTense:      "forced",
		UntouchedState: "forced",
	}
	tt := newCommandTest(t, c, spec, selection{all: true}, "")

	assert.ErrorContains(t, tt.opts.run(context.Background()), "1 DatadogPodAutoscaler(s) could not be forced")
	assert.Contains(t, tt.errOut.String(), "ns/bad: not forced: cannot compute")
	assert.Equal(t, "3", annotationsOf(t, c, "good")[ForceReplicasAnnotationKey])
	assert.NotContains(t, annotationsOf(t, c, "bad"), ForceReplicasAnnotationKey)
}
