// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// TestBuildStartPatch_EmbedsProvidedHash verifies BuildStartPatch is a pure
// builder: whatever expectedHash the caller supplies is what lands in the
// annotation. The pin's correctness against apiserver-admitted state is the
// responsibility of planExpectedSpecHash (dry-run path), covered separately
// against envtest.
func TestBuildStartPatch_EmbedsProvidedHash(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "dda", Namespace: "ns"},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{ClusterName: ptr.To("before")},
		},
	}
	config := json.RawMessage(`{"spec":{"global":{"clusterName":"after"}}}`)
	const providedHash = "1111111111111111111111111111111111111111111111111111111111111111"

	patch, err := BuildStartPatch(dda, "exp-1", config, "rev-7", providedHash)
	require.NoError(t, err)

	var got struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(patch, &got))
	assert.Equal(t, "rev-7", got.Metadata.Annotations[v2alpha1.AnnotationExperimentRollbackTargetRevision])
	assert.Equal(t, providedHash, got.Metadata.Annotations[v2alpha1.AnnotationExperimentExpectedSpecHash])
}

// TestExpectedSpecHashAfterInMemoryMerge_MatchesReconcilerComputationWithoutAdmission
// documents the boundary of the in-memory hash helper: when no CRD structural
// defaults or admission webhooks would rewrite the spec (i.e. every path that
// runs under a fake client), the in-memory hash and the reconciler's later
// ComputeSpecHash agree. Production callers must use the dry-run path
// instead; this helper stays for test fixtures.
func TestExpectedSpecHashAfterInMemoryMerge_MatchesReconcilerComputationWithoutAdmission(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dda",
			Namespace: "ns",
			Annotations: map[string]string{
				"preview.datadoghq.com/feature":       "on",
				"fleet.datadoghq.com/pending-task-id": "task-1",
			},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{ClusterName: ptr.To("before")},
		},
	}
	config := json.RawMessage(`{"spec":{"global":{"clusterName":"after"}}}`)

	hash, err := ExpectedSpecHashAfterInMemoryMerge(dda, config)
	require.NoError(t, err)

	landed := dda.DeepCopy()
	landed.Spec.Global.ClusterName = ptr.To("after")
	want, err := v2alpha1.ComputeSpecHash(landed.Spec, landed.Annotations)
	require.NoError(t, err)
	assert.Equal(t, want, hash)
}
