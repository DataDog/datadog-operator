// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/equality"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	// OperatorStoreLabelKey used to identified which resource is managed by the store.
	OperatorStoreLabelKey = "operator.datadoghq.com/managed-by-store"
	// ManagedByDDAControllerLabelKey used to identify resources managed by the DDA controller
	// when DatadogAgentInternalEnabled is true. These resources should not be cleaned up
	// by the DDAI controller to avoid competition between the two controllers.
	ManagedByDDAControllerLabelKey = "operator.datadoghq.com/managed-by-dda-controller"
)

// StoreClient dependencies store client interface
type StoreClient interface {
	AddOrUpdate(kind kubernetes.ObjectKind, obj client.Object) error
	Get(kind kubernetes.ObjectKind, namespace, name string) (client.Object, bool)
	GetOrCreate(kind kubernetes.ObjectKind, namespace, name string) (client.Object, bool)
	GetPlatformInfo() kubernetes.PlatformInfo
	Delete(kind kubernetes.ObjectKind, namespace string, name string) bool
	DeleteAll(ctx context.Context, k8sClient client.Client) []error
	Logger() logr.Logger
}

// NewStore returns a new Store instance
func NewStore(owner metav1.Object, options *StoreOptions) *Store {
	store := &Store{
		deps:  make(map[kubernetes.ObjectKind]map[string]client.Object),
		owner: owner,
	}
	if options != nil {
		store.supportCilium = options.SupportCilium
		store.platformInfo = options.PlatformInfo
		store.logger = options.Logger
		store.scheme = options.Scheme
		store.isDDAControllerStore = options.IsDDAControllerStore
	}

	return store
}

// Store Kubernetes resource dependencies store
// this store helps to keep track of every resources that the different agent deployments depend on.
type Store struct {
	deps  map[kubernetes.ObjectKind]map[string]client.Object
	mutex sync.RWMutex

	supportCilium        bool
	platformInfo         kubernetes.PlatformInfo
	isDDAControllerStore bool

	scheme *runtime.Scheme
	logger logr.Logger
	owner  metav1.Object
}

// StoreOptions use to provide to NewStore() function some Store creation options.
type StoreOptions struct {
	SupportCilium bool
	PlatformInfo  kubernetes.PlatformInfo

	Scheme *runtime.Scheme
	Logger logr.Logger

	// IsDDAControllerStore indicates that this store is used by the DDA controller
	// to manage dependencies when DatadogAgentInternalEnabled is true.
	// Resources created by this store will be labeled with ManagedByDDAControllerLabelKey
	// so they won't be cleaned up by the DDAI controller.
	IsDDAControllerStore bool
}

// AddOrUpdate used to add or update an object in the Store
// kind correspond to the object kind, and id can be `namespace/name` identifier of just
// `name` if we are talking about a cluster scope object like `ClusterRole`.
func (ds *Store) AddOrUpdate(kind kubernetes.ObjectKind, obj client.Object) error {
	ds.mutex.Lock()
	defer ds.mutex.Unlock()

	if _, found := ds.deps[kind]; !found {
		ds.deps[kind] = map[string]client.Object{}
	}

	id := buildID(obj.GetNamespace(), obj.GetName())
	if obj.GetLabels() == nil {
		obj.SetLabels(map[string]string{})
	}
	obj.GetLabels()[OperatorStoreLabelKey] = "true"

	// Add the DDA controller label when this store is used by the DDA controller
	// with DatadogAgentInternalEnabled. This prevents DDAI controller from cleaning
	// up these resources.
	if ds.isDDAControllerStore {
		obj.GetLabels()[ManagedByDDAControllerLabelKey] = "true"
	}

	if ds.owner != nil {
		defaultLabels := object.GetDefaultLabels(ds.owner, ds.owner.GetName(), common.GetAgentVersion(ds.owner))
		if len(defaultLabels) > 0 {
			maps.Copy(obj.GetLabels(), defaultLabels)
		}

		defaultAnnotations := object.GetDefaultAnnotations(ds.owner)
		if len(defaultAnnotations) > 0 {
			if obj.GetAnnotations() == nil {
				obj.SetAnnotations(map[string]string{})
			}
			maps.Copy(obj.GetAnnotations(), defaultAnnotations)
		}

		// Owner-reference should not be added to cluster level objects
		if shouldSetOwnerReference(kind, obj.GetNamespace(), ds.owner.GetNamespace()) {
			if err := object.SetOwnerReference(ds.owner, obj, ds.scheme); err != nil {
				return fmt.Errorf("store.AddOrUpdate, %w", err)
			}
		}
	}

	ds.deps[kind][id] = obj
	return nil
}

// AddOrUpdateStore used to add or update an object in the Store
// kind correspond to the object kind, and id can be `namespace/name` identifier of just
// `name` if we are talking about a cluster scope object like `ClusterRole`.
func (ds *Store) AddOrUpdateStore(kind kubernetes.ObjectKind, obj client.Object) *Store {
	_ = ds.AddOrUpdate(kind, obj)
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
//   - if it was previously added in the Store, it returns the corresponding object
//   - if it wasn't previously added in the Store, it returns a new instance of the object Kind with
//     the corresponding name and namespace.
//
// `kind“ correspond to the object kind, and id can be `namespace/name` identifier of just
// `name` if we are talking about a cluster scope object like `ClusterRole`.
// It also return a boolean to know if the Object was found in the Store.
func (ds *Store) GetOrCreate(kind kubernetes.ObjectKind, namespace, name string) (client.Object, bool) {
	obj, found := ds.Get(kind, namespace, name)
	if found {
		return obj, found
	}
	obj = kubernetes.ObjectFromKind(kind, ds.platformInfo)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj, found
}

// Delete deletes an item from the store by kind, namespace and name.
func (ds *Store) Delete(kind kubernetes.ObjectKind, namespace string, name string) bool {
	ds.mutex.RLock()
	defer ds.mutex.RUnlock()

	if _, found := ds.deps[kind]; !found {
		return false
	}
	id := buildID(namespace, name)
	if _, found := ds.deps[kind][id]; found {
		delete(ds.deps[kind], id)
		return true
	}
	return false
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
			objAPIServer := kubernetes.ObjectFromKind(kind, ds.platformInfo)
			err := k8sClient.Get(ctx, objNSName, objAPIServer)
			if err != nil && apierrors.IsNotFound(err) {
				objAPIServer = nil // Object doesn't exist and needs to be created
			} else if err != nil {
				errs = append(errs, err)
				continue
			}

			// Apply preprocessing for each object kind
			objStore, err = ds.applyPreprocessing(kind, objStore, objAPIServer)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if objAPIServer == nil {
				ds.logger.V(2).Info("store.store Add object to create", "obj.namespace", objStore.GetNamespace(), "obj.name", objStore.GetName(), "obj.kind", kind)
				objsToCreate = append(objsToCreate, objStore)
				continue
			}

			if !equality.IsEqualObject(kind, objStore, objAPIServer) {
				ds.logger.V(2).Info("store.store Add object to update", "obj.namespace", objStore.GetNamespace(), "obj.name", objStore.GetName(), "obj.kind", kind)
				objsToUpdate = append(objsToUpdate, objStore)
				continue
			}
		}
	}

	ds.logger.V(2).Info("store.store objsToCreate", "nb", len(objsToCreate))
	for _, obj := range objsToCreate {
		if err := k8sClient.Create(ctx, obj); err != nil {
			ds.logger.Error(err, "store.store Create", "obj.namespace", obj.GetNamespace(), "obj.name", obj.GetName())
			errs = append(errs, err)
		}
	}

	ds.logger.V(2).Info("store.store objsToUpdate", "nb", len(objsToUpdate))
	for _, obj := range objsToUpdate {
		if err := k8sClient.Update(ctx, obj); err != nil {
			ds.logger.Error(err, "store.store Update", "obj.namespace", obj.GetNamespace(), "obj.name", obj.GetName())
			errs = append(errs, err)
		}
	}
	return errs
}

// Cleanup use to cleanup resources that are not needed anymore.
// If excludeDDAManagedResources is true, resources managed by the DDA controller
// (marked with ManagedByDDAControllerLabelKey) will be excluded from cleanup.
// This is used when the DDAI controller performs cleanup to avoid deleting
// resources managed by the DDA controller.
//
// If the store is a DDA controller store (isDDAControllerStore is true), cleanup
// will ONLY delete resources that have the ManagedByDDAControllerLabelKey label.
// This prevents the DDA controller from accidentally deleting DDAI-managed resources.
func (ds *Store) Cleanup(ctx context.Context, k8sClient client.Client, excludeDDAManagedResources bool) []error {
	ds.mutex.RLock()
	defer ds.mutex.RUnlock()

	var errs []error

	selector := labels.NewSelector()
	requirementStore, _ := labels.NewRequirement(OperatorStoreLabelKey, selection.Exists, nil)
	selector = selector.Add(*requirementStore)

	// If this is a DDA controller store, only list resources that have the DDA controller label.
	// This ensures the DDA controller only cleans up its own resources, not DDAI resources.
	if ds.isDDAControllerStore {
		requirementDDA, _ := labels.NewRequirement(ManagedByDDAControllerLabelKey, selection.Exists, nil)
		selector = selector.Add(*requirementDDA)
	}

	listOptions := &client.ListOptions{
		LabelSelector: selector,
	}

	for _, kind := range ds.platformInfo.GetAgentResourcesKind(ds.supportCilium) {
		objList := kubernetes.ObjectListFromKind(kind, ds.platformInfo)
		if err := k8sClient.List(ctx, objList, listOptions); err != nil {
			errs = append(errs, err)
			continue
		}

		objsToDelete, err := ds.listObjectToDelete(kind, objList, ds.deps[kind], excludeDDAManagedResources)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, deleteObjects(ctx, k8sClient, objsToDelete)...)
	}

	return errs
}

// GetPlatformInfo returns api-resources info
func (ds *Store) GetPlatformInfo() kubernetes.PlatformInfo {
	return ds.platformInfo
}

// Logger returns the log client
func (ds *Store) Logger() logr.Logger {
	return ds.logger
}

// DeleteAll deletes all the resources that are in the Store
func (ds *Store) DeleteAll(ctx context.Context, k8sClient client.Client) []error {
	ds.mutex.RLock()
	defer ds.mutex.RUnlock()

	var objsToDelete []client.Object

	for _, kind := range ds.platformInfo.GetAgentResourcesKind(ds.supportCilium) {
		requirementLabel, _ := labels.NewRequirement(OperatorStoreLabelKey, selection.Exists, nil)
		listOptions := &client.ListOptions{
			LabelSelector: labels.NewSelector().Add(*requirementLabel),
		}
		objList := kubernetes.ObjectListFromKind(kind, ds.platformInfo)
		if err := k8sClient.List(ctx, objList, listOptions); err != nil {
			return []error{err}
		}

		items, err := apimeta.ExtractList(objList)
		if err != nil {
			return []error{err}
		}

		for _, objAPIServer := range items {
			objMeta, _ := apimeta.Accessor(objAPIServer)

			idObj := buildID(objMeta.GetNamespace(), objMeta.GetName())
			if _, found := ds.deps[kind][idObj]; found {
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
	}

	return deleteObjects(ctx, k8sClient, objsToDelete)
}

func (ds *Store) listObjectToDelete(kind kubernetes.ObjectKind, objList client.ObjectList, cacheObjects map[string]client.Object, excludeDDAManagedResources bool) ([]client.Object, error) {
	items, err := apimeta.ExtractList(objList)
	if err != nil {
		return nil, err
	}

	var objsToDelete []client.Object
	for _, objAPIServer := range items {
		objMeta, _ := apimeta.Accessor(objAPIServer)

		idObj := buildID(objMeta.GetNamespace(), objMeta.GetName())
		if _, found := cacheObjects[idObj]; !found {
			objLabels := objMeta.GetLabels()

			// Skip resources managed by the DDA controller when excludeDDAManagedResources is true.
			// This prevents the DDAI controller from deleting resources that are managed
			// by the DDA controller (manageDDADependenciesWithDDAI).
			if excludeDDAManagedResources {
				if _, isDDAManaged := objLabels[ManagedByDDAControllerLabelKey]; isDDAManaged {
					continue
				}
			}

			// only delete dependencies associated with the currently reconciled dda
			if partOfValue, found := objLabels[kubernetes.AppKubernetesPartOfLabelKey]; found {
				partialDDA := &metav1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ds.owner.GetName(),
						Namespace: ds.owner.GetNamespace(),
					},
				}
				if partOfValue == object.NewPartOfLabelValue(partialDDA).String() {
					partialObj := &metav1.PartialObjectMetadata{
						ObjectMeta: metav1.ObjectMeta{
							Name:      objMeta.GetName(),
							Namespace: objMeta.GetNamespace(),
						},
					}

					// Try to get GVK from the listed object first (works in production)
					gvk := objAPIServer.GetObjectKind().GroupVersionKind()

					// If GVK is empty (e.g., in fake client tests), get it from the scheme
					if gvk.Empty() && ds.scheme != nil {
						refObj := kubernetes.ObjectFromKind(kind, ds.platformInfo)
						if gvks, _, err := ds.scheme.ObjectKinds(refObj); err == nil && len(gvks) > 0 {
							gvk = gvks[0]
						}
					}

					partialObj.TypeMeta.SetGroupVersionKind(gvk)
					objsToDelete = append(objsToDelete, partialObj)
				}
			}
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

func shouldSetOwnerReference(kind kubernetes.ObjectKind, objNamespace, ownerNamespace string) bool {
	// Owner-reference should not be added to cluster level objects
	switch kind {
	case kubernetes.ClusterRoleBindingKind:
		return false
	case kubernetes.ClusterRolesKind:
		return false
	case kubernetes.APIServiceKind:
		return false
	}

	// Owner-reference should not be added to namespaced resources in a different namespace than the owner
	if objNamespace != "" && ownerNamespace != "" && objNamespace != ownerNamespace {
		return false
	}

	return true
}
