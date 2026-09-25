// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestDatadogAnnotations_FiltersTransientSignals(t *testing.T) {
	filtered := DatadogAnnotations(map[string]string{
		"preview.datadoghq.com/feature":                    "on",
		"experiment.datadoghq.com/signal":                  "start",
		"experiment.datadoghq.com/expected-spec-hash":      "abc",
		"fleet.datadoghq.com/pending-task-id":              "task-1",
		"kubectl.kubernetes.io/last-applied-configuration": "{}",
	})

	assert.Equal(t, map[string]string{"preview.datadoghq.com/feature": "on"}, filtered)
}

func TestDatadogAnnotations_NilForEmptyFilteredSet(t *testing.T) {
	// Empty and absent annotation maps must hash identically, so the filter
	// returns nil rather than an empty map for both.
	assert.Nil(t, DatadogAnnotations(nil))
	assert.Nil(t, DatadogAnnotations(map[string]string{"kubectl.kubernetes.io/x": "y"}))

	emptyHash, err := DatadogAnnotationsHash(nil)
	require.NoError(t, err)
	filteredOutHash, err := DatadogAnnotationsHash(map[string]string{"kubectl.kubernetes.io/x": "y"})
	require.NoError(t, err)
	assert.Equal(t, emptyHash, filteredOutHash)
}

// TestComputeSpecHash_MatchesRevisionSnapshotBytes is the load-bearing
// assertion behind the expected-spec-hash pin: Fleet hashes the spec it is
// about to apply and the reconciler hashes the spec it reads back, both
// through this helper, and a ControllerRevision stores exactly these bytes.
// If the hash were ever computed over a different shape than the snapshot
// payload, hash equality and revision equality would stop meaning the same
// thing and the pin would silently mis-detect drift.
func TestComputeSpecHash_MatchesRevisionSnapshotBytes(t *testing.T) {
	spec := DatadogAgentSpec{
		Global: &GlobalConfig{ClusterName: ptr.To("test-cluster")},
	}
	annotations := map[string]string{
		"preview.datadoghq.com/feature":   "on",
		"experiment.datadoghq.com/signal": "start",
	}

	snapBytes, err := BuildRevisionSnapshot(spec, annotations)
	require.NoError(t, err)
	hash, err := ComputeSpecHash(spec, annotations)
	require.NoError(t, err)

	sum := sha256.Sum256(snapBytes)
	assert.Equal(t, fmt.Sprintf("%x", sum), hash)
}

func TestComputeSpecHash_IgnoresTransientAnnotations(t *testing.T) {
	spec := DatadogAgentSpec{
		Global: &GlobalConfig{ClusterName: ptr.To("test-cluster")},
	}

	// The pin annotation itself is written onto the same object it describes,
	// so it must not participate in the hash — otherwise Fleet could never
	// compute a value the reconciler would reproduce.
	withoutPin, err := ComputeSpecHash(spec, map[string]string{"preview.datadoghq.com/feature": "on"})
	require.NoError(t, err)
	withPin, err := ComputeSpecHash(spec, map[string]string{
		"preview.datadoghq.com/feature":            "on",
		AnnotationExperimentExpectedSpecHash:       "stale-value",
		AnnotationExperimentSignal:                 "start",
		AnnotationExperimentRollbackTargetRevision: "dda-abc123",
		"fleet.datadoghq.com/pending-task-id":      "task-1",
	})
	require.NoError(t, err)

	assert.Equal(t, withoutPin, withPin)
}
