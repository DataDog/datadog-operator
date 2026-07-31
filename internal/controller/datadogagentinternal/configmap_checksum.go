// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"fmt"
	"hash"
	"hash/fnv"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

// configMapContent excludes metadata (resourceVersion, labels, annotations) so
// those changes don't affect the checksum.
type configMapContent struct {
	Data       map[string]string
	BinaryData map[string][]byte
}

// annotateConfigMapsChecksum hashes the content of ConfigMaps
// referenced by podTmpl's volumes and stores it as an annotation on podTmpl,
// so the change is picked up by the existing pod-template-hash rollout.
//
// Missing ConfigMaps are skipped rather than erroring, since a dangling
// reference already surfaces elsewhere as a pod mount failure.
func (r *Reconciler) annotateConfigMapsChecksum(ctx context.Context, namespace string, podTmpl *corev1.PodTemplateSpec) error {
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

	if podTmpl.Annotations == nil {
		podTmpl.Annotations = map[string]string{}
	}
	podTmpl.Annotations[constants.ConfigMapsChecksumAnnotationKey] = hashConfigMapContents(contents)

	return nil
}

// hashConfigMapContents hashes contents in sorted-name order, so the result
// doesn't depend on map iteration order.
func hashConfigMapContents(contents map[string]configMapContent) string {
	names := make([]string, 0, len(contents))
	for name := range contents {
		names = append(names, name)
	}
	sort.Strings(names)

	h := fnv.New64a()
	for _, name := range names {
		h.Write([]byte(name))
		c := contents[name]
		writeSortedMap(h, c.Data)
		writeSortedMap(h, c.BinaryData)
	}

	return fmt.Sprintf("%016x", h.Sum64())
}

func writeSortedMap[V ~string | ~[]byte](h hash.Hash, m map[string]V) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(m[k]))
	}
}

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
