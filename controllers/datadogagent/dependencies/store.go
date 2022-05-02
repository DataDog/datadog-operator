// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dependencies

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-operator/pkg/equality"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// operatorStoreLabelKey used to identified which resource is managed by the store.
	operatorStoreLabelKey = "operator.datadoghq.com/managed-by-store"
)

// StoreClient dependencies store client interface
type StoreClient interface {
	AddOrUpdate(kind kubernetes.ObjectKind, obj client.Object)
	Get(kind kubernetes.ObjectKind, namespace, name string) (client.Object, bool)
	GetOrCreate(kind kubernetes.ObjectKind, namespace, name string) (client.Object, bool)
}

// NewStore returns a new Store instance
func NewStore(options *StoreOptions) *Store {
	store := &Store{
		deps: make(map[kubernetes.ObjectKind]map[string]client.Object),
	}
	if options != nil {
		store.supportCilium = options.SupportCilium
	}

	return store
}

// Store Kubernetes resource dependencies store
// this store helps to keep track of every resources that the different agent deployments depend on.
type Store struct {
	deps  map[kubernetes.ObjectKind]map[string]client.Object
	mutex sync.RWMutex

	supportCilium bool
}

// StoreOptions use to provide to NewStore() function some Store creation options.
type StoreOptions struct {
	SupportCilium bool
}

// AddOrUpdate used to add or update an object in the Store
// kind correspond to the object kind, and id can be `namespace/name` identifier of just
// `name` if we are talking about a cluster scope object like `ClusterRole`.
func (ds *Store) AddOrUpdate(kind kubernetes.ObjectKind, obj client.Object) {
	ds.mutex.Lock()
	defer ds.mutex.Unlock()

	if _, found := ds.deps[kind]; !found {
		ds.deps[kind] = map[string]client.Object{}
	}

	id := buildID(obj.GetNamespace(), obj.GetName())
	if obj.GetLabels() == nil {
		obj.SetLabels(map[string]string{})
	}
	obj.GetLabels()[operatorStoreLabelKey] = "true"

	ds.deps[kind][id] = obj
}

// AddOrUpdateStore used to add or update an object in the Store
// kind correspond to the object kind, and id can be `namespace/name` identifier of just
// `name` if we are talking about a cluster scope object like `ClusterRole`.
func (ds *Store) AddOrUpdateStore(kind kubernetes.ObjectKind, obj client.Object) *Store {
	ds.AddOrUpdate(kind, obj)
	return ds
}

// Get returns the client.Object instance if it was previously added in the Store.
// kind correspond to the object kind, and id can be `namespace/name` identifier of just
// `name` if we are talking about a cluster scope object like `ClusterRole`.
// It also return a boolean to know if the Object was found in the Store.
func (ds *Store) Get(kind kubernetes.ObjectKind, namespace string, name string) (client.Object, bool) {
	ds.mutex.RLock()
	defer ds.mutex.RUnlock()

	if _, found := ds.deps[kind]; !found {
		return nil, false
	}
	id := buildID(namespace, name)
	if obj, found := ds.deps[kind][id]; found {
		return obj, true
	}
	return nil, false
}

// GetOrCreate returns the client.Object instance.
// * if it was previously added in the Store, it returns the corresponding object
// * if it wasn't previously added in the Store, it returns a new instance of the object Kind with
//   the corresponding name and namespace.
// `kind`` correspond to the object kind, and id can be `namespace/name` identifier of just
// `name` if we are talking about a cluster scope object like `ClusterRole`.
// It also return a boolean to know if the Object was found in the Store.
func (ds *Store) GetOrCreate(kind kubernetes.ObjectKind, namespace, name string) (client.Object, bool) {
	obj, found := ds.Get(kind, namespace, name)
	if found {
		return obj, found
	}
	obj = kubernetes.ObjectFromKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj, found
}

// Apply use to create/update resources in the api-server
func (ds *Store) Apply(ctx context.Context, k8sClient client.Client) []error {
	ds.mutex.RLock()
	defer ds.mutex.RUnlock()

	var errs []error
	var objsToCreate []client.Object
	var objsToUpdate []client.Object
	for kind := range ds.deps {
		for objID, objStore := range ds.deps[kind] {
			objNSName := buildObjectKey(objID)
			objAPIServer := kubernetes.ObjectFromKind(kind)
			err := k8sClient.Get(ctx, objNSName, objAPIServer)
			if err != nil && apierrors.IsNotFound(err) {
				objsToCreate = append(objsToCreate, objStore)
				continue
			} else if err != nil {
				errs = append(errs, err)
				continue
			}

			if !equality.IsEqualObject(kind, objStore, objAPIServer) {
				objsToUpdate = append(objsToUpdate, objStore)
			}
		}
	}

	for _, obj := range objsToCreate {
		if err := k8sClient.Create(ctx, obj); err != nil {
			errs = append(errs, err)
		}
	}

	for _, obj := range objsToUpdate {
		if err := k8sClient.Update(ctx, obj); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

// Cleanup use to cleanup resources that are not needed anymore
func (ds *Store) Cleanup(ctx context.Context, k8sClient client.Client, ddaNs, ddaName string) []error {
	ds.mutex.RLock()
	defer ds.mutex.RUnlock()

	var errs []error

	requirementLabel, _ := labels.NewRequirement(operatorStoreLabelKey, selection.Exists, nil)
	listOptions := &client.ListOptions{
		LabelSelector: labels.NewSelector().Add(*requirementLabel),
	}
	for _, kind := range kubernetes.GetResourcesKind(ds.supportCilium) {
		objList := kubernetes.ObjectListFromKind(kind)
		if err := k8sClient.List(ctx, objList, listOptions); err != nil {
			errs = append(errs, err)
			continue
		}

		objsToDelete, err := listObjectToDelete(objList, ds.deps[kind])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, deleteObjects(ctx, k8sClient, objsToDelete)...)
	}

	return errs
}

func listObjectToDelete(objList client.ObjectList, cacheObjects map[string]client.Object) ([]client.Object, error) {
	items, err := apimeta.ExtractList(objList)
	if err != nil {
		return nil, err
	}

	var objsToDelete []client.Object
	for _, objAPIServer := range items {
		objMeta, _ := apimeta.Accessor(objAPIServer)

		idObj := buildID(objMeta.GetNamespace(), objMeta.GetName())
		if _, found := cacheObjects[idObj]; !found {
			partialObj := &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:      objMeta.GetName(),
					Namespace: objMeta.GetNamespace(),
				},
			}
			partialObj.TypeMeta.SetGroupVersionKind(objAPIServer.GetObjectKind().GroupVersionKind())
			objsToDelete = append(objsToDelete, partialObj)
		}
	}
	return objsToDelete, nil
}

func deleteObjects(ctx context.Context, k8sClient client.Client, objsToDelete []client.Object) []error {
	var errs []error
	for _, partialObj := range objsToDelete {
		err := k8sClient.Delete(ctx, partialObj)
		if err != nil {
			if apierrors.IsNotFound(err) || apierrors.IsGone(err) {
				continue
			}
			errs = append(errs, err)
		}
	}
	return errs
}

func buildID(ns, name string) string {
	if ns == "" {
		return name
	}
	return fmt.Sprintf("%s/%s", ns, name)
}

func buildObjectKey(key string) types.NamespacedName {
	keySplit := strings.Split(key, string(types.Separator))
	var ns, name string
	if len(keySplit) > 1 {
		ns = keySplit[0]
		name = keySplit[1]
	} else {
		name = key
	}
	return types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}
}
