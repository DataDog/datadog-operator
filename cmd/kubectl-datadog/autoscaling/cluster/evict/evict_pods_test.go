package evict

import (
	"context"
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
)

// installPodEvictionReactor makes a fake clientset accept Eviction
// subresource creates and echo the eviction object back. The Pod itself is
// NOT removed from the tracker — tests that want a pod-removal effect (e.g.
// to make drainNode observe an empty node) must add a "delete pods" reactor
// explicitly, or use a fixture that starts with no pod on the node.
func installPodEvictionReactor(client *fake.Clientset) {
	client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ca, ok := action.(clienttesting.CreateAction)
		if !ok || ca.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		return true, ca.GetObject(), nil
	})
}

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
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ds"}},
		}}, skip: true},
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
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ds"}},
		}}, occupies: false},
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

			err := evictPodWithRetry(context.Background(), client, tc.pod, tc.timeout, 10*time.Millisecond)
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
