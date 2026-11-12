// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

// DatadogAnnotations returns a copy of annotations filtered to only those
// with `.datadoghq.com/` in the key, which are used for preview features.
// Experiment signal annotations (experiment.datadoghq.com/) and Fleet
// pending-operation annotations (fleet.datadoghq.com/) are excluded because
// they are transient signals, not part of the spec snapshot.
//
// Returns nil for an empty filtered set so empty and absent annotation maps
// hash consistently.
func DatadogAnnotations(all map[string]string) map[string]string {
	filtered := make(map[string]string)
	for k, v := range all {
		if strings.Contains(k, ".datadoghq.com/") &&
			!strings.HasPrefix(k, "experiment.datadoghq.com/") &&
			!strings.HasPrefix(k, "fleet.datadoghq.com/") {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// DatadogAnnotationsHash returns the sha256 hex digest of the filtered
// annotation map returned by DatadogAnnotations. Used to detect
// annotation-only changes for revision pointer freshness, since
// metadata.generation does not advance on metadata-only writes.
func DatadogAnnotationsHash(all map[string]string) (string, error) {
	filtered := DatadogAnnotations(all)
	data, err := json.Marshal(filtered)
	if err != nil {
		return "", fmt.Errorf("failed to marshal annotations for hashing: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

// RevisionSnapshot is the payload stored in a ControllerRevision.
// Annotations are included for preview features.
type RevisionSnapshot struct {
	Spec        DatadogAgentSpec  `json:"spec"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// BuildRevisionSnapshot marshals a RevisionSnapshot from spec and annotations.
// Spec must be the raw, user-submitted spec (not the in-memory defaulted copy)
// so that snapshot comparisons are unaffected by defaulting.
//
// This lives in the API package rather than the controller because the Fleet
// daemon computes the same hash at plan time that the reconciler validates
// against; a second implementation would be free to drift and defeat the pin.
func BuildRevisionSnapshot(spec DatadogAgentSpec, allAnnotations map[string]string) ([]byte, error) {
	snap := RevisionSnapshot{Spec: spec, Annotations: DatadogAnnotations(allAnnotations)}
	return json.Marshal(snap)
}

// ComputeSpecHash returns the sha256 hex digest of the canonical revision
// snapshot (raw spec + filtered Datadog annotations) — the same bytes stored
// in a ControllerRevision — so hash equality and revision payload equality
// mean the same thing.
func ComputeSpecHash(spec DatadogAgentSpec, allAnnotations map[string]string) (string, error) {
	snapBytes, err := BuildRevisionSnapshot(spec, allAnnotations)
	if err != nil {
		return "", fmt.Errorf("failed to build revision snapshot: %w", err)
	}
	sum := sha256.Sum256(snapBytes)
	return fmt.Sprintf("%x", sum), nil
}
