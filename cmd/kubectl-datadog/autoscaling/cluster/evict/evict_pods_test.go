package evict

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestShouldSkipEviction(t *testing.T) {
	now := metav1.Now()
	for _, tc := range []struct {
		name string
		pod  *corev1.Pod
		skip bool
	}{
		{name: "regular pod", pod: &corev1.Pod{}, skip: false},
		{name: "terminating", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}}, skip: true},
		{name: "mirror", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{corev1.MirrorPodAnnotationKey: "x"},
		}}, skip: true},
		{name: "daemonset", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: "ds", Controller: ptr.To(true)}},
		}}, skip: true},
		{name: "non-controller DaemonSet owner is not skipped", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: "ds"}},
		}}, skip: false},
		{name: "succeeded job", pod: &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}, skip: true},
		{name: "failed job", pod: &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}, skip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.skip, shouldSkipEviction(tc.pod))
		})
	}
}

// TestPodOccupiesNode locks in the asymmetry between "skip eviction" and
// "keeps the node busy": terminating pods are skipped from eviction (the
// kubelet is already deleting them) BUT still occupy the node for drain
// purposes (otherwise we'd terminate the instance mid-grace-period and kill
// the container before its preStop hook finishes).
func TestPodOccupiesNode(t *testing.T) {
	now := metav1.Now()
	for _, tc := range []struct {
		name     string
		pod      *corev1.Pod
		occupies bool
	}{
		{name: "regular pod", pod: &corev1.Pod{}, occupies: true},
		{name: "terminating pod still occupies", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}}, occupies: true},
		{name: "mirror pod", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{corev1.MirrorPodAnnotationKey: "x"},
		}}, occupies: false},
		{name: "daemonset pod", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: "ds", Controller: ptr.To(true)}},
		}}, occupies: false},
		{name: "non-controller DaemonSet owner still occupies", pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: "ds"}},
		}}, occupies: true},
		{name: "succeeded job", pod: &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}, occupies: false},
		{name: "failed job", pod: &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}, occupies: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.occupies, podOccupiesNode(tc.pod))
		})
	}
}

func TestEvictPodWithRetry(t *testing.T) {
	// evictionResponder shapes the test-side of the Eviction subresource
	// reactor. `call` is the 1-based call counter.
	type evictionResponder func(call int, eviction *policyv1.Eviction) (resp runtime.Object, err error)

	echoSuccess := evictionResponder(func(_ int, e *policyv1.Eviction) (runtime.Object, error) {
		return e, nil
	})
	tooManyOnceThenSuccess := evictionResponder(func(call int, e *policyv1.Eviction) (runtime.Object, error) {
		if call == 1 {
			return nil, apierrors.NewTooManyRequests("PDB blocked", 1)
		}
		return e, nil
	})
	notFound := evictionResponder(func(_ int, e *policyv1.Eviction) (runtime.Object, error) {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, e.Name)
	})
	nonRetryable := evictionResponder(func(_ int, _ *policyv1.Eviction) (runtime.Object, error) {
		return nil, errors.New("non-retryable")
	})
	alwaysTooMany := evictionResponder(func(_ int, _ *policyv1.Eviction) (runtime.Object, error) {
		return nil, apierrors.NewTooManyRequests("PDB blocked", 1)
	})

	for _, tc := range []struct {
		name string
		// pod is the object passed to evictPodWithRetry (and pre-loaded
		// into the fake when seedPod is true).
		pod     *corev1.Pod
		seedPod bool
		// responder controls what the Eviction reactor returns each call.
		responder evictionResponder
		timeout   time.Duration

		wantErr          bool
		wantErrContains  string
		wantMinCalls     int
		wantCapturedName string
		wantCapturedNs   string
	}{
		{
			name:         "success on first call",
			pod:          &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}},
			seedPod:      true,
			responder:    echoSuccess,
			timeout:      5 * time.Second,
			wantMinCalls: 1,
		},
		{
			name:         "retries on 429 then succeeds",
			pod:          &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}},
			seedPod:      true,
			responder:    tooManyOnceThenSuccess,
			timeout:      10 * time.Second,
			wantMinCalls: 2,
		},
		{
			// Pod already gone: 404 from the apiserver is treated as
			// success — the eviction goal is met regardless.
			name:         "404 from apiserver is success",
			pod:          &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}},
			seedPod:      false,
			responder:    notFound,
			timeout:      time.Second,
			wantMinCalls: 1,
		},
		{
			name:            "non-retryable error returned",
			pod:             &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}},
			seedPod:         true,
			responder:       nonRetryable,
			timeout:         5 * time.Second,
			wantErr:         true,
			wantErrContains: "eviction failed",
			wantMinCalls:    1,
		},
		{
			// Always 429: OnError exhausts its retries while still
			// PDB-blocked, and the last 429 is surfaced as a timeout.
			name:            "exhausts retries while 429-blocked",
			pod:             &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}},
			seedPod:         true,
			responder:       alwaysTooMany,
			timeout:         50 * time.Millisecond,
			wantErr:         true,
			wantErrContains: "eviction timed out",
			wantMinCalls:    2,
		},
		{
			// Smoke check that the Eviction object carries the pod's
			// Name/Namespace through to the apiserver.
			name:             "eviction object name/namespace",
			pod:              &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"}},
			seedPod:          true,
			responder:        echoSuccess,
			timeout:          time.Second,
			wantMinCalls:     1,
			wantCapturedName: "p1",
			wantCapturedNs:   "ns1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seed []runtime.Object
			if tc.seedPod {
				seed = append(seed, tc.pod)
			}
			client := fake.NewClientset(seed...)

			var (
				calls    int
				captured *policyv1.Eviction
			)
			client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
				ca, ok := action.(clienttesting.CreateAction)
				if !ok || ca.GetSubresource() != "eviction" {
					return false, nil, nil
				}
				calls++
				eviction := ca.GetObject().(*policyv1.Eviction)
				captured = eviction
				resp, err := tc.responder(calls, eviction)
				return true, resp, err
			})

			err := evictPodWithRetry(t.Context(), client, tc.pod, tc.timeout, 10*time.Millisecond)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrContains != "" {
					assert.Contains(t, err.Error(), tc.wantErrContains)
				}
			} else {
				require.NoError(t, err)
			}
			assert.GreaterOrEqual(t, calls, tc.wantMinCalls)
			if tc.wantCapturedName != "" {
				require.NotNil(t, captured)
				assert.Equal(t, tc.wantCapturedName, captured.Name)
				assert.Equal(t, tc.wantCapturedNs, captured.Namespace)
			}
		})
	}
}

// TestEvictPodWithRetryZeroInterval pins the guard against a non-positive
// retryInterval: it must not panic (division by zero) and degrades to a single
// eviction attempt.
func TestEvictPodWithRetryZeroInterval(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}}
	client := fake.NewClientset(pod)
	var calls int
	client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ca, ok := action.(clienttesting.CreateAction)
		if !ok || ca.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		calls++
		return true, ca.GetObject(), nil
	})

	err := evictPodWithRetry(t.Context(), client, pod, time.Second, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

// TestListPodsOnNode locks in the server-side node scoping: the fake client
// does not enforce field selectors, so we assert explicitly that the request
// carries spec.nodeName=<node>. A regression here would silently drain pods
// from every node, not just the target.
func TestListPodsOnNode(t *testing.T) {
	client := fake.NewClientset()
	var gotFieldSelector string
	client.PrependReactor("list", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		gotFieldSelector = action.(clienttesting.ListAction).GetListRestrictions().Fields.String()
		return true, &corev1.PodList{Items: []corev1.Pod{occupyingPod("a"), occupyingPod("b")}}, nil
	})

	pods, err := listPodsOnNode(t.Context(), client, "ip-7")
	require.NoError(t, err)
	assert.Len(t, pods, 2)
	assert.Equal(t, "spec.nodeName=ip-7", gotFieldSelector)
}

// occupyingPod and dsPod are the two fixtures the drain/wait tests reuse: a
// plain pod that keeps the node busy, and a DaemonSet pod that never does.
func occupyingPod(name string) corev1.Pod {
	return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
}

func dsPod(name string) corev1.Pod {
	return corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:            name,
		Namespace:       "default",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: "ds", Controller: ptr.To(true)}},
	}}
}

func TestWaitForNodeEmpty(t *testing.T) {
	occupying := occupyingPod("app")
	ds := dsPod("agent")

	for _, tc := range []struct {
		name string
		// listResponder returns the pod list for each 1-based List call,
		// letting a case spread a transition across successive polls.
		listResponder   func(call int) (runtime.Object, error)
		timeout         time.Duration
		wantErr         bool
		wantErrContains string
	}{
		{
			name:          "empty on first poll",
			listResponder: func(int) (runtime.Object, error) { return &corev1.PodList{}, nil },
			timeout:       time.Second,
		},
		{
			// Only DaemonSet/mirror/completed pods linger: the node does not
			// "occupy" for drain purposes, so the wait returns immediately.
			name: "only non-occupying pods counts as empty",
			listResponder: func(int) (runtime.Object, error) {
				return &corev1.PodList{Items: []corev1.Pod{ds}}, nil
			},
			timeout: time.Second,
		},
		{
			name: "becomes empty after the first poll",
			listResponder: func(call int) (runtime.Object, error) {
				if call == 1 {
					return &corev1.PodList{Items: []corev1.Pod{occupying}}, nil
				}
				return &corev1.PodList{}, nil
			},
			timeout: time.Second,
		},
		{
			name: "times out while an occupying pod remains",
			listResponder: func(int) (runtime.Object, error) {
				return &corev1.PodList{Items: []corev1.Pod{occupying}}, nil
			},
			timeout: 60 * time.Millisecond,
			wantErr: true,
		},
		{
			name: "list error is wrapped",
			listResponder: func(int) (runtime.Object, error) {
				return nil, errors.New("apiserver unreachable")
			},
			timeout:         time.Second,
			wantErr:         true,
			wantErrContains: "failed to list pods on node",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientset()
			var calls int
			client.PrependReactor("list", "pods", func(_ clienttesting.Action) (bool, runtime.Object, error) {
				calls++
				obj, err := tc.listResponder(calls)
				return true, obj, err
			})
			err := waitForNodeEmpty(t.Context(), client, "ip-1", tc.timeout, 10*time.Millisecond)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrContains != "" {
					assert.Contains(t, err.Error(), tc.wantErrContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDrainNode(t *testing.T) {
	occupying := occupyingPod("app")
	ds := dsPod("agent")

	// countingEvictionReactor installs an Eviction reactor that echoes success
	// and increments *n on every eviction create.
	countingEvictionReactor := func(client *fake.Clientset, n *int) {
		client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
			ca, ok := action.(clienttesting.CreateAction)
			if !ok || ca.GetSubresource() != "eviction" {
				return false, nil, nil
			}
			*n++
			return true, ca.GetObject(), nil
		})
	}

	t.Run("dry-run evicts nothing", func(t *testing.T) {
		client := fake.NewClientset(&occupying)
		var evictions int
		countingEvictionReactor(client, &evictions)
		err := drainNode(t.Context(), client, "ip-1", nodeDrainOptions{DryRun: true})
		require.NoError(t, err)
		assert.Zero(t, evictions)
	})

	t.Run("evicts the occupying pod then waits for the node to empty", func(t *testing.T) {
		client := fake.NewClientset(&occupying)
		var evictions, lists int
		countingEvictionReactor(client, &evictions)
		// First List (drainNode's own enumeration) still sees the pod; the
		// subsequent List from waitForNodeEmpty sees the node drained.
		client.PrependReactor("list", "pods", func(_ clienttesting.Action) (bool, runtime.Object, error) {
			lists++
			if lists == 1 {
				return true, &corev1.PodList{Items: []corev1.Pod{occupying}}, nil
			}
			return true, &corev1.PodList{}, nil
		})
		err := drainNode(t.Context(), client, "ip-1", nodeDrainOptions{
			EvictionTimeout: time.Second,
			NodeTimeout:     time.Second,
			PollInterval:    10 * time.Millisecond,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, evictions)
	})

	t.Run("skips non-evictable pods and treats the node as empty", func(t *testing.T) {
		client := fake.NewClientset(&ds)
		var evictions int
		countingEvictionReactor(client, &evictions)
		client.PrependReactor("list", "pods", func(_ clienttesting.Action) (bool, runtime.Object, error) {
			return true, &corev1.PodList{Items: []corev1.Pod{ds}}, nil
		})
		err := drainNode(t.Context(), client, "ip-1", nodeDrainOptions{
			EvictionTimeout: time.Second,
			NodeTimeout:     time.Second,
			PollInterval:    10 * time.Millisecond,
		})
		require.NoError(t, err)
		assert.Zero(t, evictions)
	})
}
