// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"slices"

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

const (
	fnv64aOffset uint64 = 14695981039346656037
	fnv64aPrime  uint64 = 1099511628211
)

func fnv64aWriteString(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnv64aPrime
	}
	return h
}

func fnv64aWriteBytes(h uint64, b []byte) uint64 {
	for _, c := range b {
		h ^= uint64(c)
		h *= fnv64aPrime
	}
	return h
}

// hashConfigMapContents hashes contents in sorted-name order, so the result
// doesn't depend on map iteration order.
func hashConfigMapContents(contents map[string]configMapContent) string {
	names := make([]string, 0, len(contents))
	for name := range contents {
		names = append(names, name)
	}
	slices.Sort(names)

	var keys []string
	h := fnv64aOffset
	for _, name := range names {
		h = fnv64aWriteString(h, name)
		c := contents[name]
		if len(c.Data) > 0 {
			keys = keys[:0]
			for k := range c.Data {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				h = fnv64aWriteString(h, k)
				h = fnv64aWriteString(h, c.Data[k])
			}
		}
		if len(c.BinaryData) > 0 {
			keys = keys[:0]
			for k := range c.BinaryData {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				h = fnv64aWriteString(h, k)
				h = fnv64aWriteBytes(h, c.BinaryData[k])
			}
		}
	}

	var sumBytes [8]byte
	binary.BigEndian.PutUint64(sumBytes[:], h)
	var hexBuf [16]byte
	hex.Encode(hexBuf[:], sumBytes[:])
	return string(hexBuf[:])
}

func referencedConfigMapNames(podTmpl *corev1.PodTemplateSpec) []string {
	var seen map[string]struct{}
	var names []string
	for i := range podTmpl.Spec.Volumes {
		cm := podTmpl.Spec.Volumes[i].ConfigMap
		if cm == nil {
			continue
		}
		if seen == nil {
			seen = make(map[string]struct{}, 4)
		} else if _, ok := seen[cm.Name]; ok {
			continue
		}
		seen[cm.Name] = struct{}{}
		names = append(names, cm.Name)
	}
	return names
}
