package evict

import (
	"context"
	"fmt"
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"
)

// nodeDrainOptions captures the per-call tunables for evicting a node's pods.
type nodeDrainOptions struct {
	DryRun          bool
	EvictionTimeout time.Duration // per pod, bound for retries on 429
	NodeTimeout     time.Duration // total wait for the node to become empty
	PollInterval    time.Duration // interval between empty-checks; default 2s
}

// drainNode evicts every evictable pod from the node and waits for the node to
// become empty. Pods owned by a DaemonSet, mirror pods, terminating pods and
// completed Job pods are skipped — the kubelet handles their cleanup when the
// underlying instance disappears.
//
// Pods that cannot be evicted (PDB-blocked beyond EvictionTimeout, etc.) are
// logged as warnings; drainNode then continues with the remaining pods rather
// than aborting the whole run.
func drainNode(ctx context.Context, clientset kubernetes.Interface, nodeName string, opts nodeDrainOptions) error {
	if opts.DryRun {
		log.Printf("[dry-run] would drain node %s", nodeName)
		return nil
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 2 * time.Second
	}
	pods, err := listPodsOnNode(ctx, clientset, nodeName)
	if err != nil {
		return fmt.Errorf("failed to list pods on node %s: %w", nodeName, err)
	}
	for _, p := range pods {
		if shouldSkipPod(&p) {
			continue
		}
		if err := evictPodWithRetry(ctx, clientset, &p, opts.EvictionTimeout, opts.PollInterval); err != nil {
			log.Printf("Warning: pod %s/%s: %v", p.Namespace, p.Name, err)
		}
	}
	return waitForNodeEmpty(ctx, clientset, nodeName, opts.NodeTimeout, opts.PollInterval)
}

// listPodsOnNode enumerates pods scheduled on the given node, server-side
// filtered via the spec.nodeName field selector. Uses the client-go pager
// defaults so very large nodes (250 pods+) don't trigger oversized list calls.
func listPodsOnNode(ctx context.Context, clientset kubernetes.Interface, nodeName string) ([]corev1.Pod, error) {
	var pods []corev1.Pod
	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, opts)
	})
	err := p.EachListItem(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName).String(),
	}, func(obj runtime.Object) error {
		pods = append(pods, *obj.(*corev1.Pod))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pods, nil
}

// evictPodWithRetry sends a single Eviction request and retries while the
// apiserver returns 429 TooManyRequests (the canonical PDB-blocked signal).
// Aborts after timeout; the caller then logs and moves to the next pod.
//
// 404 (pod already gone) is treated as success — the eviction goal is met.
// Non-429 errors are returned immediately.
func evictPodWithRetry(ctx context.Context, clientset kubernetes.Interface, p *corev1.Pod, timeout, retryInterval time.Duration) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{Name: p.Name, Namespace: p.Namespace},
	}
	deadline := time.Now().Add(timeout)
	for {
		err := clientset.CoreV1().Pods(p.Namespace).EvictV1(ctx, eviction)
		if err == nil {
			log.Printf("Evicted pod %s/%s.", p.Namespace, p.Name)
			return nil
		}
		if apierrors.IsNotFound(err) {
			return nil
		}
		if !apierrors.IsTooManyRequests(err) {
			return fmt.Errorf("eviction failed: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("eviction timed out (likely PDB-blocked): %w", err)
		}
		select {
		case <-time.After(retryInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// waitForNodeEmpty polls the node until no evictable pods remain. Pods that
// shouldSkipPod returns true on (DaemonSet, mirror, terminating, completed)
// don't count against emptiness.
func waitForNodeEmpty(ctx context.Context, clientset kubernetes.Interface, nodeName string, timeout, pollInterval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		pods, err := listPodsOnNode(ctx, clientset, nodeName)
		if err != nil {
			return fmt.Errorf("failed to list pods on node %s: %w", nodeName, err)
		}
		remaining := 0
		for _, p := range pods {
			if !shouldSkipPod(&p) {
				remaining++
			}
		}
		if remaining == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for node %s to drain: %d pod(s) still present", nodeName, remaining)
		}
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func shouldSkipPod(p *corev1.Pod) bool {
	if p.DeletionTimestamp != nil {
		return true
	}
	if isMirrorPod(p) {
		return true
	}
	if isDaemonSetPod(p) {
		return true
	}
	if isCompleted(p) {
		return true
	}
	return false
}

func isMirrorPod(p *corev1.Pod) bool {
	_, ok := p.Annotations[corev1.MirrorPodAnnotationKey]
	return ok
}

func isDaemonSetPod(p *corev1.Pod) bool {
	for _, owner := range p.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

func isCompleted(p *corev1.Pod) bool {
	return p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed
}
