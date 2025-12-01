// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

// ApplyManifestFile creates or updates the Kubernetes resources defined in the manifest at manifestPath.
func ApplyManifestFile(ctx context.Context, kubeClient *client.KubernetesClient, manifestPath, defaultNamespace string) error {
	return visitManifest(ctx, kubeClient, manifestPath, defaultNamespace, applyUnstructured)
}

// DeleteManifestFile deletes the Kubernetes resources defined in the manifest at manifestPath.
func DeleteManifestFile(ctx context.Context, kubeClient *client.KubernetesClient, manifestPath, defaultNamespace string) error {
	return visitManifest(ctx, kubeClient, manifestPath, defaultNamespace, deleteUnstructured)
}

type manifestVisitor func(ctx context.Context, dynClient dynamic.Interface, mapper *restmapper.DeferredDiscoveryRESTMapper, obj *unstructured.Unstructured) error

func visitManifest(ctx context.Context, kubeClient *client.KubernetesClient, manifestPath, defaultNamespace string, visitor manifestVisitor) error {
	objects, err := parseManifest(manifestPath, defaultNamespace)
	if err != nil {
		return err
	}

	dynClient, err := dynamic.NewForConfig(kubeClient.K8sConfig)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeClient.K8sConfig)
	if err != nil {
		return err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))

	for _, obj := range objects {
		if err := visitor(ctx, dynClient, mapper, obj); err != nil {
			return fmt.Errorf("processing %s %s/%s: %w", obj.GroupVersionKind().String(), obj.GetNamespace(), obj.GetName(), err)
		}
	}

	return nil
}

func parseManifest(manifestPath, defaultNamespace string) ([]*unstructured.Unstructured, error) {
	file, err := os.Open(manifestPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := k8syaml.NewYAMLOrJSONDecoder(file, 4096)
	var objects []*unstructured.Unstructured
	for {
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}

		if obj.Object == nil || obj.GetKind() == "" {
			continue
		}

		if obj.GetNamespace() == "" {
			obj.SetNamespace(defaultNamespace)
		}

		objects = append(objects, obj)
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("manifest %s does not define any Kubernetes objects", manifestPath)
	}

	return objects, nil
}

func applyUnstructured(ctx context.Context, dynClient dynamic.Interface, mapper *restmapper.DeferredDiscoveryRESTMapper, obj *unstructured.Unstructured) error {
	resourceClient, err := resourceInterfaceFor(mapper, dynClient, obj.GroupVersionKind(), obj.GetNamespace())
	if err != nil {
		return err
	}

	_, err = resourceClient.Create(ctx, obj, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return err
	}

	current, err := resourceClient.Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	obj.SetResourceVersion(current.GetResourceVersion())
	_, err = resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func deleteUnstructured(ctx context.Context, dynClient dynamic.Interface, mapper *restmapper.DeferredDiscoveryRESTMapper, obj *unstructured.Unstructured) error {
	resourceClient, err := resourceInterfaceFor(mapper, dynClient, obj.GroupVersionKind(), obj.GetNamespace())
	if err != nil {
		return err
	}
	err = resourceClient.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func resourceInterfaceFor(mapper *restmapper.DeferredDiscoveryRESTMapper, dynClient dynamic.Interface, gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error) {
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return dynClient.Resource(mapping.Resource).Namespace(namespace), nil
	}

	return dynClient.Resource(mapping.Resource), nil
}
