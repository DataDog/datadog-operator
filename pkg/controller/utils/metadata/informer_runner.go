// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metadata

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	authorizationv1 "k8s.io/api/authorization/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// deletePrefix is prepended to queue keys to signal a deletion event.
	deletePrefix = "delete:"
	// keySeparator separates kind, namespace, and name in queue keys.
	keySeparator = "/"
)

// ProcessFunc is invoked for add/update events. The runner has already split
// the queue key into (kind, namespace, name) for the caller.
type ProcessFunc func(ctx context.Context, kind, namespace, name string) error

// DeleteFunc is invoked for delete events.
type DeleteFunc func(kind, namespace, name string)

// HeartbeatFunc is invoked on every heartbeat tick. nil disables the ticker.
type HeartbeatFunc func(ctx context.Context)

// InformerWorkQueue is the shared engine for event-driven metadata forwarders.
// It owns a rate-limited workqueue, a worker pool that drains it, and an
// optional heartbeat ticker. Callers register informers via AddWatch and
// supply ProcessFunc / DeleteFunc / HeartbeatFunc callbacks.
type InformerWorkQueue struct {
	logger            logr.Logger
	mgr               manager.Manager
	queue             workqueue.TypedRateLimitingInterface[string]
	numWorkers        int
	heartbeatInterval time.Duration

	processFn   ProcessFunc
	deleteFn    DeleteFunc
	heartbeatFn HeartbeatFunc
}

// NewInformerWorkQueue constructs a runner. heartbeatInterval = 0 disables the ticker.
func NewInformerWorkQueue(
	logger logr.Logger,
	mgr manager.Manager,
	numWorkers int,
	heartbeatInterval time.Duration,
	processFn ProcessFunc,
	deleteFn DeleteFunc,
	heartbeatFn HeartbeatFunc,
) *InformerWorkQueue {
	return &InformerWorkQueue{
		logger:            logger,
		mgr:               mgr,
		queue:             workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]()),
		numWorkers:        numWorkers,
		heartbeatInterval: heartbeatInterval,
		processFn:         processFn,
		deleteFn:          deleteFn,
		heartbeatFn:       heartbeatFn,
	}
}

// EncodeKey returns "<kind>/<namespace>/<name>".
func EncodeKey(kind, namespace, name string) string {
	return kind + keySeparator + namespace + keySeparator + name
}

// decodeKey splits "<kind>/<namespace>/<name>" into its parts.
// Returns ok=false if the key is malformed.
func decodeKey(key string) (kind, namespace, name string, ok bool) {
	parts := strings.SplitN(key, keySeparator, 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// Run blocks until ctx is done. It spawns numWorkers goroutines that drain the
// queue, plus a heartbeat ticker if heartbeatInterval > 0 and heartbeatFn != nil.
func (r *InformerWorkQueue) Run(ctx context.Context) {
	go func() {
		<-ctx.Done()
		r.logger.Info("Context cancelled, shutting down InformerWorkQueue")
		r.queue.ShutDown()
	}()

	for i := 0; i < r.numWorkers; i++ {
		go r.runWorker()
	}

	if r.heartbeatFn != nil && r.heartbeatInterval > 0 {
		go r.runHeartbeat(ctx)
	}

	<-ctx.Done()
}

func (r *InformerWorkQueue) runWorker() {
	defer utilruntime.HandleCrash()
	for {
		key, shutdown := r.queue.Get()
		if shutdown {
			return
		}
		r.dispatch(key)
	}
}

func (r *InformerWorkQueue) dispatch(key string) {
	defer r.queue.Done(key)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultOperationTimeout)
	defer cancel()

	if rest, ok := strings.CutPrefix(key, deletePrefix); ok {
		kind, ns, name, decoded := decodeKey(rest)
		if !decoded {
			r.logger.V(1).Info("Dropping malformed delete key", "key", key)
			r.queue.Forget(key)
			return
		}
		if r.deleteFn != nil {
			r.deleteFn(kind, ns, name)
		}
		r.queue.Forget(key)
		return
	}

	kind, ns, name, decoded := decodeKey(key)
	if !decoded {
		r.logger.V(1).Info("Dropping malformed key", "key", key)
		r.queue.Forget(key)
		return
	}

	if r.processFn == nil {
		r.queue.Forget(key)
		return
	}

	if err := r.processFn(ctx, kind, ns, name); err != nil {
		r.logger.V(1).Info("Error processing key, requeuing", "key", key, "error", err)
		r.queue.AddRateLimited(key)
		return
	}
	r.queue.Forget(key)
}

func (r *InformerWorkQueue) runHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(r.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickCtx, cancel := context.WithTimeout(context.Background(), DefaultOperationTimeout)
			r.heartbeatFn(tickCtx)
			cancel()
		}
	}
}

// Enqueue adds a key for an add/update event.
func (r *InformerWorkQueue) Enqueue(kind, namespace, name string) {
	r.queue.Add(EncodeKey(kind, namespace, name))
}

// EnqueueDelete adds a key for a delete event.
func (r *InformerWorkQueue) EnqueueDelete(kind, namespace, name string) {
	r.queue.Add(deletePrefix + EncodeKey(kind, namespace, name))
}

// WatchTarget describes one informer registration.
type WatchTarget struct {
	// Object is a typed example of the resource to watch (e.g. &corev1.ConfigMap{}).
	Object client.Object
	// Group is the API group for RBAC checks (e.g. "datadoghq.com"). Empty string
	// means the core/legacy API group ("" matches v1 core resources like ConfigMap).
	Group string
	// Resource is the lowercase plural resource name used for RBAC checks (e.g. "configmaps").
	Resource string
	// Kind is the prefix used for queue keys produced by this watch (e.g. "ConfigMap").
	Kind string
	// Filter, if non-nil, gates whether an event is enqueued.
	Filter func(obj any) bool
	// MetaKey extracts (namespace, name) from the object. If nil, controller-runtime's
	// MetaNamespaceKeyFunc is used and the result split on "/".
	MetaKey func(obj any) (namespace, name string, ok bool)
}

// canListWatch verifies the operator has list+watch permission for the given group/resource.
// Empty group means the core API group.
func (r *InformerWorkQueue) canListWatch(ctx context.Context, group, resource string) bool {
	for _, verb := range []string{"list", "watch"} {
		sar := &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb:     verb,
					Group:    group,
					Resource: resource,
				},
			},
		}
		if err := r.mgr.GetClient().Create(ctx, sar); err != nil {
			r.logger.V(1).Info("Failed to check RBAC permission",
				"group", group, "resource", resource, "verb", verb, "error", err)
			return false
		}
		if !sar.Status.Allowed {
			return false
		}
	}
	return true
}

// AddWatch registers an informer for the target. Returns true if RBAC and informer
// setup both succeeded; false (with a logged warning) otherwise. Failure is non-fatal:
// the caller may continue running with a subset of watches enabled.
func (r *InformerWorkQueue) AddWatch(ctx context.Context, target WatchTarget) bool {
	if !r.canListWatch(ctx, target.Group, target.Resource) {
		r.logger.Info("No permission to list/watch resource; informer will not be registered",
			"group", target.Group, "resource", target.Resource, "kind", target.Kind)
		return false
	}

	informer, err := r.mgr.GetCache().GetInformer(ctx, target.Object)
	if err != nil {
		r.logger.Info("Unable to get informer; informer will not be registered",
			"resource", target.Resource, "kind", target.Kind, "error", err)
		return false
	}

	keyOf := func(obj any) (string, string, bool) {
		if target.MetaKey != nil {
			return target.MetaKey(obj)
		}
		key, err := toolscache.MetaNamespaceKeyFunc(obj)
		if err != nil {
			return "", "", false
		}
		ns, name, ok := strings.Cut(key, "/")
		if !ok {
			// Cluster-scoped: namespace is empty.
			return "", key, true
		}
		return ns, name, true
	}

	deleteKeyOf := func(obj any) (string, string, bool) {
		if target.MetaKey != nil {
			return target.MetaKey(obj)
		}
		key, err := toolscache.DeletionHandlingMetaNamespaceKeyFunc(obj)
		if err != nil {
			return "", "", false
		}
		ns, name, ok := strings.Cut(key, "/")
		if !ok {
			return "", key, true
		}
		return ns, name, true
	}

	handler := toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if ns, name, ok := keyOf(obj); ok {
				r.Enqueue(target.Kind, ns, name)
			}
		},
		UpdateFunc: func(_, obj any) {
			if ns, name, ok := keyOf(obj); ok {
				r.Enqueue(target.Kind, ns, name)
			}
		},
		DeleteFunc: func(obj any) {
			if ns, name, ok := deleteKeyOf(obj); ok {
				r.EnqueueDelete(target.Kind, ns, name)
			}
		},
	}

	var err2 error
	if target.Filter != nil {
		_, err2 = informer.AddEventHandler(toolscache.FilteringResourceEventHandler{
			FilterFunc: target.Filter,
			Handler:    handler,
		})
	} else {
		_, err2 = informer.AddEventHandler(handler)
	}
	if err2 != nil {
		r.logger.Info("Unable to add event handler; informer will not be registered",
			"resource", target.Resource, "kind", target.Kind, "error", err2)
		return false
	}

	r.logger.V(1).Info("Registered informer", "kind", target.Kind, "group", target.Group, "resource", target.Resource)
	return true
}

// QueueLen exposes the queue length for tests and metrics.
func (r *InformerWorkQueue) QueueLen() int {
	return r.queue.Len()
}
