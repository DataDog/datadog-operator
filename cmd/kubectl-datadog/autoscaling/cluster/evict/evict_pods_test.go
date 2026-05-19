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

func TestEvictPodWithRetry_Success(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}}
	client := fake.NewClientset(pod)
	installPodEvictionReactor(client)

	err := evictPodWithRetry(context.Background(), client, pod, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, err)
}

func TestEvictPodWithRetry_RetriesOn429AndSucceeds(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}}
	client := fake.NewClientset(pod)
	var calls int
	client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ca, ok := action.(clienttesting.CreateAction)
		if !ok || ca.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		calls++
		if calls == 1 {
			return true, nil, apierrors.NewTooManyRequests("PDB blocked", 1)
		}
		return true, ca.GetObject(), nil
	})

	require.NoError(t, evictPodWithRetry(context.Background(), client, pod, 10*time.Second, 10*time.Millisecond))
	assert.GreaterOrEqual(t, calls, 2)
}

func TestEvictPodWithRetry_NotFoundIsSuccess(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}}
	client := fake.NewClientset()
	client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ca, ok := action.(clienttesting.CreateAction)
		if !ok || ca.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		eviction := ca.GetObject().(*policyv1.Eviction)
		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Resource: "pods"}, eviction.Name,
		)
	})
	require.NoError(t, evictPodWithRetry(context.Background(), client, pod, time.Second, 10*time.Millisecond))
}

func TestEvictPodWithRetry_NonRetryableError(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}}
	client := fake.NewClientset(pod)
	client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ca, ok := action.(clienttesting.CreateAction)
		if !ok || ca.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		return true, nil, errors.New("non-retryable")
	})
	err := evictPodWithRetry(context.Background(), client, pod, 5*time.Second, 10*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "eviction failed")
}

// Smoke test that the eviction object's name/namespace are set correctly.
func TestEvictPodWithRetry_EvictionObjectShape(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"}}
	client := fake.NewClientset(pod)
	var captured *policyv1.Eviction
	client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ca, ok := action.(clienttesting.CreateAction)
		if !ok || ca.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		captured = ca.GetObject().(*policyv1.Eviction)
		return true, captured, nil
	})
	require.NoError(t, evictPodWithRetry(context.Background(), client, pod, time.Second, 10*time.Millisecond))
	require.NotNil(t, captured)
	assert.Equal(t, "p1", captured.Name)
	assert.Equal(t, "ns1", captured.Namespace)
}
