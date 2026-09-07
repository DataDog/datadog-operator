// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gobwas/glob"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func mergeAnnotationsLabels(logger logr.Logger, previousVal map[string]string, newVal map[string]string, filter string) map[string]string {
	var globFilter glob.Glob
	var err error
	if filter != "" {
		globFilter, err = glob.Compile(filter)
		if err != nil {
			logger.Error(err, "Unable to parse glob filter for metadata/annotations - discarding everything", "filter", filter)
		}
	}

	mergedMap := make(map[string]string, len(newVal))
	maps.Copy(mergedMap, newVal)

	// Copy from previous if not in new match and matches globfilter
	for k, v := range previousVal {
		if _, found := newVal[k]; !found {
			if (globFilter != nil && globFilter.Match(k)) || strings.Contains(k, "datadoghq.com") {
				mergedMap[k] = v
			}
		}
	}

	return mergedMap
}

// newOwnerRef creates an OwnerReference pointing to the given owner.
func newOwnerRef(owner metav1.Object, gvk schema.GroupVersionKind) *metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return &metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

// createOwnerReferencePatch creates a patch from owner references
// We assume there is only one DDAI owner reference
func createOwnerReferencePatch(ownerRef []metav1.OwnerReference, owner metav1.Object, gvk schema.GroupVersionKind) ([]byte, error) {
	patchedRefs := make([]metav1.OwnerReference, len(ownerRef))
	copy(patchedRefs, ownerRef)

	// Replace DDAI owner reference with new owner reference
	for i, ref := range patchedRefs {
		if ref.Kind == "DatadogAgentInternal" {
			patchedRefs[i] = *newOwnerRef(owner, gvk)
		}
	}

	// Create JSON patch for ownerReferences field
	refBytes, err := json.Marshal(patchedRefs)
	if err != nil {
		return nil, err
	}

	return fmt.Appendf(nil, `{"metadata":{"ownerReferences":%s}}`, string(refBytes)), nil
}

// shouldUpdateOwnerReference returns true if the owner reference is a DatadogAgent
func shouldUpdateOwnerReference(currentOwnerRef []metav1.OwnerReference) bool {
	for _, ownerRef := range currentOwnerRef {
		if ownerRef.Kind == "DatadogAgentInternal" {
			return true
		}
	}
	return false
}

// getReplicas returns the desired replicas of a
// deployment based on the current and new replica values.
func getReplicas(currentReplicas, newReplicas *int32) *int32 {
	if newReplicas == nil {
		if currentReplicas != nil {
			// Do not overwrite the current value
			// It's most likely managed by an autoscaler
			return new(*currentReplicas)
		}

		// Both new and current are nil
		return nil
	}

	return new(*newReplicas)
}

// getDDAICRDFromConfig is only used in tests
//
//lint:ignore U1000
func getDDAICRDFromConfig(sch *runtime.Scheme) (*apiextensionsv1.CustomResourceDefinition, error) {
	_, filename, _, ok := goruntime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("unable to get caller")
	}
	path := filepath.Join(filepath.Dir(filename), "..", "..", "..", "config", "crd", "bases", "v1", "datadoghq.com_datadogagentinternals.yaml")

	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	codecs := serializer.NewCodecFactory(sch)
	decoder := codecs.UniversalDeserializer()
	obj, _, err := decoder.Decode(body, nil, &apiextensionsv1.CustomResourceDefinition{})
	if err != nil {
		return nil, err
	}

	if crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition); ok {
		return crd, nil
	}

	return nil, fmt.Errorf("decoded object is not a CustomResourceDefinition")
}

func getDDAIGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "datadoghq.com",
		Version: "v1alpha1",
		Kind:    "DatadogAgentInternal",
	}
}

// Delete specific workload and dependents with background propagation
// Only used for Helm-managed cluster checks runner deployment
func deleteAllWorkloadsAndDependentsBackground(ctx context.Context, logger logr.Logger, c client.Client, obj client.Object, component string) error {
	propagationPolicy := metav1.DeletePropagationBackground
	selector := labels.SelectorFromSet(labels.Set{
		apicommon.AgentDeploymentComponentLabelKey: component,
		kubernetes.AppKubernetesManageByLabelKey:   "Helm",
	})
	logger.Info("deleting all workloads and dependents for matching DDA with background propagation", "labels", selector.String())
	if err := c.DeleteAllOf(ctx, obj, &client.DeleteAllOfOptions{ListOptions: client.ListOptions{LabelSelector: selector, Namespace: obj.GetNamespace()}, DeleteOptions: client.DeleteOptions{PropagationPolicy: &propagationPolicy}}); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("object not found, skipping deletion", "object", obj.GetName(), "namespace", obj.GetNamespace())
			return nil
		}
		return err
	}
	return nil
}

// delete ALL workloads for a given DDA/DDAI and orphan pods
func deleteObjectAndOrphanDependents(ctx context.Context, logger logr.Logger, c client.Client, obj client.Object, component string) error {
	propagationPolicy := metav1.DeletePropagationOrphan
	selector := labels.SelectorFromSet(labels.Set{
		kubernetes.AppKubernetesPartOfLabelKey:     obj.GetLabels()[kubernetes.AppKubernetesPartOfLabelKey],
		apicommon.AgentDeploymentComponentLabelKey: component,
	})
	logger.Info("deleting all workloads for matching DDA", "labels", selector.String())
	if err := c.DeleteAllOf(ctx, obj, &client.DeleteAllOfOptions{ListOptions: client.ListOptions{LabelSelector: selector, Namespace: obj.GetNamespace()}, DeleteOptions: client.DeleteOptions{PropagationPolicy: &propagationPolicy}}); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("object not found, skipping deletion", "object", obj.GetName(), "namespace", obj.GetNamespace())
			return nil
		}
		return err
	}
	return nil
}
