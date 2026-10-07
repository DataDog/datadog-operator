// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orderedrollout

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

const lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// IsControlAnnotation reports whether key is a rollout control annotation.
func IsControlAnnotation(key string) bool {
	switch key {
	case HoldAnnotation, SkipAnnotation, BypassAnnotation:
		return true
	}
	return false
}

// isHashAnnotation reports whether key holds a hash computed by the operator
// from the DatadogAgentInternal itself.
func isHashAnnotation(key string) bool {
	return key == TargetHashAnnotation || key == constants.MD5DDAIDeploymentAnnotationKey
}

// IsInertAnnotation reports whether a DatadogAgentInternal annotation does not
// affect DatadogAgentInternal rendering. Every other annotation is part of the
// rollout target hash.
func IsInertAnnotation(key string) bool {
	switch {
	case IsControlAnnotation(key), isHashAnnotation(key):
		return true
	case key == lastAppliedConfigAnnotation,
		key == constants.MD5AgentDeploymentAnnotationKey,
		key == constants.ConfigMapsChecksumAnnotationKey:
		return true
	case strings.HasPrefix(key, "meta.helm.sh/"), strings.HasPrefix(key, "checksum/"):
		return true
	}
	return false
}

// StripControlAnnotations removes rollout control annotations in place.
func StripControlAnnotations(annotations map[string]string) {
	maps.DeleteFunc(annotations, func(k, _ string) bool { return IsControlAnnotation(k) })
}

// TargetHash returns the rollout target hash of a DatadogAgentInternal from
// its spec hash and annotations. Inert annotations are ignored, so the result
// does not depend on hash annotations already present on the object.
func TargetHash(specHash string, annotations map[string]string) string {
	h := sha256.New()
	fmt.Fprintf(h, "spec=%s\n", specHash)
	for _, k := range slices.Sorted(maps.Keys(annotations)) {
		if IsInertAnnotation(k) {
			continue
		}
		fmt.Fprintf(h, "%q=%q\n", k, annotations[k])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// RolloutID identifies an aggregate rollout from step keys and target hashes.
func RolloutID(targets map[string]string) string {
	h := sha256.New()
	for _, k := range slices.Sorted(maps.Keys(targets)) {
		fmt.Fprintf(h, "%s=%s\n", k, targets[k])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// InertAnnotationPatch returns the inert annotation changes needed to bring
// live in line with desired without touching hash or rendering-relevant
// annotations. A nil value in the result removes the annotation.
func InertAnnotationPatch(desired, live map[string]string) map[string]*string {
	patch := map[string]*string{}
	syncable := func(k string) bool { return IsInertAnnotation(k) && !isHashAnnotation(k) }
	for k, v := range desired {
		if syncable(k) && (live == nil || live[k] != v) {
			patch[k] = &v
		}
	}
	for k := range live {
		if _, ok := desired[k]; !ok && syncable(k) {
			patch[k] = nil
		}
	}
	if len(patch) == 0 {
		return nil
	}
	return patch
}
