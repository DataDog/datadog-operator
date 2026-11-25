// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// legacyListPageSize bounds the size of each page when listing legacy CRs.
const legacyListPageSize = 500

// legacyKind describes a legacy CRD that manages the same Datadog objects as
// a DatadogGenericResource type.
type legacyKind struct {
	kind     string
	resource string
	newObj   func() client.Object
	newList  func() client.ObjectList
	// statusIDs returns the Datadog ID managed by each item of list, keyed by
	// the item's namespace and name. Items without an ID are left out.
	statusIDs func(list client.ObjectList) map[types.NamespacedName]string
	statusID  func(obj client.Object) string
}

var legacyKinds = map[v1alpha1.SupportedResourcesType]legacyKind{
	v1alpha1.Monitor: {
		kind:     "DatadogMonitor",
		resource: "datadogmonitors",
		newObj:   func() client.Object { return &v1alpha1.DatadogMonitor{} },
		newList:  func() client.ObjectList { return &v1alpha1.DatadogMonitorList{} },
		statusIDs: func(list client.ObjectList) map[types.NamespacedName]string {
			ids := map[types.NamespacedName]string{}
			for _, item := range list.(*v1alpha1.DatadogMonitorList).Items {
				if item.Status.ID != 0 {
					ids[types.NamespacedName{Namespace: item.Namespace, Name: item.Name}] = strconv.Itoa(item.Status.ID)
				}
			}
			return ids
		},
		statusID: func(obj client.Object) string {
			if id := obj.(*v1alpha1.DatadogMonitor).Status.ID; id != 0 {
				return strconv.Itoa(id)
			}
			return ""
		},
	},
	v1alpha1.Dashboard: {
		kind:     "DatadogDashboard",
		resource: "datadogdashboards",
		newObj:   func() client.Object { return &v1alpha1.DatadogDashboard{} },
		newList:  func() client.ObjectList { return &v1alpha1.DatadogDashboardList{} },
		statusIDs: func(list client.ObjectList) map[types.NamespacedName]string {
			ids := map[types.NamespacedName]string{}
			for _, item := range list.(*v1alpha1.DatadogDashboardList).Items {
				if item.Status.ID != "" {
					ids[types.NamespacedName{Namespace: item.Namespace, Name: item.Name}] = item.Status.ID
				}
			}
			return ids
		},
		statusID: func(obj client.Object) string { return obj.(*v1alpha1.DatadogDashboard).Status.ID },
	},
	v1alpha1.SLO: {
		kind:     "DatadogSLO",
		resource: "datadogslos",
		newObj:   func() client.Object { return &v1alpha1.DatadogSLO{} },
		newList:  func() client.ObjectList { return &v1alpha1.DatadogSLOList{} },
		statusIDs: func(list client.ObjectList) map[types.NamespacedName]string {
			ids := map[types.NamespacedName]string{}
			for _, item := range list.(*v1alpha1.DatadogSLOList).Items {
				if item.Status.ID != "" {
					ids[types.NamespacedName{Namespace: item.Namespace, Name: item.Name}] = item.Status.ID
				}
			}
			return ids
		},
		statusID: func(obj client.Object) string { return obj.(*v1alpha1.DatadogSLO).Status.ID },
	},
}

// legacyOwnerIndex finds the DatadogMonitor, DatadogDashboard or DatadogSLO
// that manages a Datadog object, so that a DatadogGenericResource never adopts
// an object that another controller manages.
//
// The API server can't filter custom resources on status.id, and the shared
// cache only has legacy kinds whose controller is enabled. The index is
// therefore built from one paginated, uncached list per kind, the first time
// that kind is needed, and only keeps the ID, namespace and name of each CR.
// A hit is confirmed with a GET, which also drops stale entries. A legacy CR
// that gets its status.id after the list isn't in the index: to collide, a
// DatadogGenericResource would have to adopt an ID that didn't exist yet. A
// restart refreshes the index.
type legacyOwnerIndex struct {
	reader client.Reader

	mu    sync.Mutex
	kinds map[v1alpha1.SupportedResourcesType]*legacyKindIndex
}

type legacyKindIndex struct {
	// notInstalled means the CRD doesn't exist, so it can't own anything.
	notInstalled bool
	owners       map[string]types.NamespacedName
}

func newLegacyOwnerIndex(reader client.Reader) *legacyOwnerIndex {
	return &legacyOwnerIndex{reader: reader, kinds: map[v1alpha1.SupportedResourcesType]*legacyKindIndex{}}
}

// owner returns a description of the legacy CR that manages the Datadog
// object of the given type and ID, or an empty string if there is none. It
// fails closed: an error means the check couldn't be done.
func (x *legacyOwnerIndex) owner(ctx context.Context, resourceType v1alpha1.SupportedResourcesType, id string) (string, error) {
	lk, ok := legacyKinds[resourceType]
	if !ok {
		return "", nil
	}

	// Holding the lock while listing makes concurrent adoptions of the same
	// kind wait for a single list instead of each running their own.
	x.mu.Lock()
	defer x.mu.Unlock()

	idx, err := x.build(ctx, resourceType, lk)
	if err != nil {
		return "", err
	}
	if idx.notInstalled {
		return "", nil
	}
	key, ok := idx.owners[id]
	if !ok {
		return "", nil
	}

	// Confirm the hit: the CR may be gone or manage another object by now.
	obj := lk.newObj()
	if err := x.reader.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			delete(idx.owners, id)
			return "", nil
		}
		return "", fmt.Errorf("checking whether %s %s manages %s %s: %w", lk.kind, key, resourceType, id, err)
	}
	if lk.statusID(obj) != id {
		delete(idx.owners, id)
		return "", nil
	}
	state := ""
	if !obj.GetDeletionTimestamp().IsZero() {
		// Its finalizer may still delete the object.
		state = " and is being deleted"
	}
	hint := ""
	if resourceType == v1alpha1.Monitor {
		hint = fmt.Sprintf(", or run \"kubectl datadog monitor migrate %s -n %s\"", key.Name, key.Namespace)
	}
	return fmt.Sprintf("%s %s manages it%s. Annotate it with %s=%s and delete it so that it releases the object without deleting it%s; the adoption then goes ahead", lk.kind, key, state, v1alpha1.ManagementPolicyAnnotationKey, v1alpha1.ManagementPolicyOrphan, hint), nil
}

// build lists the legacy kind once. On error nothing is cached, so the next
// check lists again. Must be called with x.mu held.
func (x *legacyOwnerIndex) build(ctx context.Context, resourceType v1alpha1.SupportedResourcesType, lk legacyKind) (*legacyKindIndex, error) {
	if idx, ok := x.kinds[resourceType]; ok {
		return idx, nil
	}

	idx := &legacyKindIndex{owners: map[string]types.NamespacedName{}}
	continueToken := ""
	for {
		list := lk.newList()
		err := x.reader.List(ctx, list, client.Limit(legacyListPageSize), client.Continue(continueToken))
		if err != nil {
			if meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) {
				idx = &legacyKindIndex{notInstalled: true}
				break
			}
			if apierrors.IsForbidden(err) {
				return nil, fmt.Errorf("the Operator needs the list permission on %s.datadoghq.com to check that no %s manages the object: %w", lk.resource, lk.kind, err)
			}
			return nil, fmt.Errorf("listing %s resources: %w", lk.kind, err)
		}
		for key, id := range lk.statusIDs(list) {
			idx.owners[id] = key
		}
		listMeta, err := meta.ListAccessor(list)
		if err != nil {
			return nil, err
		}
		if continueToken = listMeta.GetContinue(); continueToken == "" {
			break
		}
	}

	ctrl.LoggerFrom(ctx).Info("Indexed legacy resources for adoption checks", "kind", lk.kind, "installed", !idx.notInstalled, "count", len(idx.owners))
	x.kinds[resourceType] = idx
	return idx, nil
}
