// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"time"

	jsonpatch "gomodules.xyz/jsonpatch/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// validateParams checks that experimentParams has the fields required to locate
// and act on a DatadogAgent resource.
func validateParams(p experimentParams) error {
	if p.NamespacedName.Name == "" {
		return fmt.Errorf("params namespaced_name must have a non-empty name")
	}
	if p.NamespacedName.Namespace == "" {
		return fmt.Errorf("params namespaced_name must have a non-empty namespace")
	}
	if p.GroupVersionKind.Kind != "DatadogAgent" {
		return fmt.Errorf("params kind must be DatadogAgent, got %q", p.GroupVersionKind.Kind)
	}
	return nil
}

// experimentSignal is the signal value passed to resolveOperation, used as an error prefix.
type experimentSignal string

const (
	signalStartDatadogAgentExperiment   experimentSignal = "start DatadogAgent experiment"
	signalStopDatadogAgentExperiment    experimentSignal = "stop DatadogAgent experiment"
	signalPromoteDatadogAgentExperiment experimentSignal = "promote DatadogAgent experiment"
)

// resolvedOperation holds the data needed to execute an experiment operation.
// For OperationUpdate, Config is the full config, merge-patched as-is. For
// OperationReplace, Config is just the "spec" value (see extractReplaceSpec),
// which replaces the DatadogAgent's spec wholesale.
type resolvedOperation struct {
	NamespacedName types.NamespacedName
	Operation      Operation
	Config         json.RawMessage
}

// signalPatch is a patch plus the type to apply it as: a JSON merge patch
// (RFC 7386, used by update) or a JSON Patch (RFC 6902, used by replace).
// Data holds the merge patch body; Ops holds the JSON Patch operations.
//
// Use mergePatch(nil) for "nothing to patch", not the zero value: an empty
// Type isn't valid and gets rejected by injectPendingAnnotations.
type signalPatch struct {
	Type types.PatchType
	Data []byte
	Ops  []jsonpatch.Operation
}

// mergePatch wraps data as a JSON merge patch (RFC 7386).
func mergePatch(data []byte) signalPatch {
	return signalPatch{Type: types.MergePatchType, Data: data}
}

// jsonPatch wraps ops as a JSON Patch (RFC 6902).
func jsonPatch(ops []jsonpatch.Operation) signalPatch {
	return signalPatch{Type: types.JSONPatchType, Ops: ops}
}

// experimentBackoff retries k8s operations starting at 1s, doubling up to 10s,
// for up to 3 minutes total.
var experimentBackoff = wait.Backoff{
	Duration: 1 * time.Second,
	Factor:   2.0,
	Jitter:   0.1,
	Cap:      10 * time.Second,
	Steps:    math.MaxInt32,
}

// isRetryable returns true for errors that are worth retrying (i.e. not permanent).
func isRetryable(err error) bool {
	return !apierrors.IsNotFound(err) &&
		!apierrors.IsForbidden(err) &&
		!apierrors.IsInvalid(err) &&
		!apierrors.IsMethodNotSupported(err)
}

// retryWithBackoff retries fn on transient errors, bounded by a 3-minute timeout.
// Permanent errors (not-found, forbidden, invalid, method-not-supported) are not retried.
func retryWithBackoff(ctx context.Context, fn func() error) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return retry.OnError(experimentBackoff, func(err error) bool {
		return ctx.Err() == nil && isRetryable(err)
	}, fn)
}

// buildSignalPatch creates a JSON merge patch that sets the experiment signal
// and ID annotations, merging in config's spec fields if given so the spec and
// annotations are written atomically.
func buildSignalPatch(signal, id string, config ...json.RawMessage) ([]byte, error) {
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{
				v2alpha1.AnnotationExperimentSignal: signal,
				v2alpha1.AnnotationExperimentID:     id,
			},
		},
	}

	if len(config) > 0 && config[0] != nil {
		var specPatch map[string]any
		if err := json.Unmarshal(config[0], &specPatch); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
		// Safe as a shallow copy only because config never contains "metadata".
		maps.Copy(patch, specPatch)
	}

	return json.Marshal(patch)
}

// extractReplaceSpec returns config's "spec" value. It doesn't type-check the
// spec — the API server does that — but it does reject an empty spec ({}),
// since the API server would accept that and wipe the resource instead.
func extractReplaceSpec(config json.RawMessage) (json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(config, &raw); err != nil {
		return nil, fmt.Errorf("config must be a JSON object: %w", err)
	}
	specRaw, ok := raw["spec"]
	if !ok {
		return nil, fmt.Errorf(`config for replace operation must contain a "spec" key`)
	}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(specRaw, &spec); err == nil && len(spec) == 0 {
		return nil, fmt.Errorf(`config "spec" must not be empty`)
	}
	return specRaw, nil
}

// buildReplaceSignalPatch creates a JSON Patch that replaces the DatadogAgent's
// spec wholesale and writes the experiment signal and ID annotations.
// bootstrapAnnotations should be true if the DatadogAgent has no annotations
// map yet, so the patch creates one first. Pending-task annotations are added
// later, by injectPendingAnnotations.
func buildReplaceSignalPatch(signal, id string, spec json.RawMessage, bootstrapAnnotations bool) []jsonpatch.Operation {
	ops := make([]jsonpatch.Operation, 0, 5)

	if bootstrapAnnotations {
		// "test" fails the patch if annotations already exist, instead of "add" overwriting them.
		ops = append(ops,
			jsonpatch.Operation{Operation: "test", Path: "/metadata/annotations", Value: nil},
			jsonpatch.Operation{Operation: "add", Path: "/metadata/annotations", Value: map[string]string{}},
		)
	}

	ops = append(ops,
		jsonpatch.Operation{Operation: "add", Path: "/spec", Value: spec},
		jsonpatch.Operation{Operation: "add", Path: kubernetes.AnnotationJSONPatchPath(v2alpha1.AnnotationExperimentSignal), Value: signal},
		jsonpatch.Operation{Operation: "add", Path: kubernetes.AnnotationJSONPatchPath(v2alpha1.AnnotationExperimentID), Value: id},
	)

	return ops
}

// injectPendingAnnotations adds pending's task-tracking annotations to sp's
// patch, merging them in for a merge patch or appending JSON Patch ops for a
// JSON Patch. This is the only place pending annotations are written.
func injectPendingAnnotations(sp signalPatch, pending *pendingOperation) ([]byte, error) {
	// resultVersion is handled separately below: merge and JSON Patch clear a
	// stale value differently.
	entries := []struct{ key, value string }{
		{v2alpha1.AnnotationPendingTaskID, pending.taskID},
		{v2alpha1.AnnotationPendingAction, string(pending.intent)},
		{v2alpha1.AnnotationPendingExperimentID, pending.experimentID},
		{v2alpha1.AnnotationPendingPackage, pending.packageName},
	}

	switch sp.Type {
	case types.JSONPatchType:
		ops := sp.Ops
		for _, e := range entries {
			ops = append(ops, jsonpatch.Operation{Operation: "add", Path: kubernetes.AnnotationJSONPatchPath(e.key), Value: e.value})
		}
		// Always written, even as "", to clear a stale value from an earlier
		// promote — pendingOperationFromAnnotations treats "" as absent.
		ops = append(ops, jsonpatch.Operation{Operation: "add", Path: kubernetes.AnnotationJSONPatchPath(v2alpha1.AnnotationPendingResultVersion), Value: pending.resultVersion})
		return json.Marshal(ops)

	case types.MergePatchType:
		var patchMap map[string]any
		if len(sp.Data) != 0 {
			if err := json.Unmarshal(sp.Data, &patchMap); err != nil {
				return nil, fmt.Errorf("failed to unmarshal base patch: %w", err)
			}
		} else {
			patchMap = make(map[string]any)
		}

		metadata, ok := patchMap["metadata"].(map[string]any)
		if !ok {
			metadata = make(map[string]any)
			patchMap["metadata"] = metadata
		}
		annotations, ok := metadata["annotations"].(map[string]any)
		if !ok {
			annotations = make(map[string]any)
			metadata["annotations"] = annotations
		}
		for _, e := range entries {
			annotations[e.key] = e.value
		}
		if pending.resultVersion != "" {
			annotations[v2alpha1.AnnotationPendingResultVersion] = pending.resultVersion
		} else {
			// Clear any old promote result version. Merge patch leaves keys alone
			// when they are omitted.
			annotations[v2alpha1.AnnotationPendingResultVersion] = nil
		}

		return json.Marshal(patchMap)

	default:
		return nil, fmt.Errorf("unsupported patch type %q", sp.Type)
	}
}

// isTerminalPhase returns true for terminal experiment phases.
func isTerminalPhase(phase v2alpha1.ExperimentPhase) bool {
	switch phase {
	case v2alpha1.ExperimentPhaseTerminated, v2alpha1.ExperimentPhasePromoted, v2alpha1.ExperimentPhaseAborted:
		return true
	default:
		return false
	}
}
