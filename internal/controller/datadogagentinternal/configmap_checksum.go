// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

// configMapContent is the subset of a ConfigMap that participates in the
// referenced-configmaps checksum. Only content fields are hashed so that
// metadata-only changes (resourceVersion, labels, annotations) on the
// ConfigMap don't cause spurious rollouts.
type configMapContent struct {
	Data       map[string]string `json:"data,omitempty"`
	BinaryData map[string][]byte `json:"binaryData,omitempty"`
}

// annotateWithReferencedConfigMapsChecksum walks podTmpl's volumes for
// ConfigMap references (operator-owned or user-supplied alike), hashes their
// current content, and stamps the result onto podTmpl's annotations. Because
// this annotation becomes part of the pod template that is later hashed as a
// whole by createOrUpdateDaemonset/createOrUpdateDeployment, a ConfigMap
// content change automatically participates in the existing rollout decision
// with no further wiring.
//
// Missing ConfigMaps are skipped rather than treated as a reconcile error,
// since a dangling reference is already surfaced elsewhere as a pod mount
// failure.
func (r *Reconciler) annotateWithReferencedConfigMapsChecksum(ctx context.Context, namespace string, podTmpl *corev1.PodTemplateSpec) error {
	names := referencedConfigMapNames(podTmpl)
	if len(names) == 0 {
		return nil
	}

	contents := make(map[string]configMapContent, len(names))
	for _, name := range names {
		cm := &corev1.ConfigMap{}
		err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm)
		if apierrors.IsNotFound(err) {
			ctrl.LoggerFrom(ctx).Info("referenced ConfigMap not found, skipping checksum contribution", "configmap", name, "namespace", namespace)
			continue
		}
		if err != nil {
			return err
		}
		contents[name] = configMapContent{Data: cm.Data, BinaryData: cm.BinaryData}
	}

	if len(contents) == 0 {
		return nil
	}

	orderedNames := make([]string, 0, len(contents))
	for name := range contents {
		orderedNames = append(orderedNames, name)
	}
	sort.Strings(orderedNames)

	ordered := make([]configMapContent, 0, len(orderedNames))
	for _, name := range orderedNames {
		ordered = append(ordered, contents[name])
	}

	hash, err := comparison.GenerateMD5ForSpec(ordered)
	if err != nil {
		return err
	}

	if podTmpl.Annotations == nil {
		podTmpl.Annotations = map[string]string{}
	}
	podTmpl.Annotations[constants.MD5ReferencedConfigMapsAnnotationKey] = hash

	return nil
}

// referencedConfigMapNames returns the distinct ConfigMap names referenced
// by podTmpl's volumes.
func referencedConfigMapNames(podTmpl *corev1.PodTemplateSpec) []string {
	seen := map[string]struct{}{}
	var names []string
	for _, vol := range podTmpl.Spec.Volumes {
		if vol.ConfigMap == nil {
			continue
		}
		if _, ok := seen[vol.ConfigMap.Name]; ok {
			continue
		}
		seen[vol.ConfigMap.Name] = struct{}{}
		names = append(names, vol.ConfigMap.Name)
	}
	return names
}
