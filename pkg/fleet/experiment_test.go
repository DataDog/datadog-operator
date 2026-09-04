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

// TestBuildStartPatch_PinsHashOfPostMergeSpec is the load-bearing assertion
// behind the expected-spec-hash pin: the value Fleet writes must equal what the
// reconciler computes from the live object after the patch lands. Hashing the
// raw config fragment or the un-merged spec would produce a different shape and
// silently defeat the pin.
func TestBuildStartPatch_PinsHashOfPostMergeSpec(t *testing.T) {
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

	patch, err := BuildStartPatch(dda, "exp-1", config, "rev-7")
	require.NoError(t, err)

	var got struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(patch, &got))
	assert.Equal(t, "rev-7", got.Metadata.Annotations[v2alpha1.AnnotationExperimentRollbackTargetRevision])

	// What the reconciler will compute once the patch has landed.
	landed := dda.DeepCopy()
	landed.Spec.Global.ClusterName = ptr.To("after")
	for k, v := range got.Metadata.Annotations {
		landed.Annotations[k] = v
	}
	want, err := v2alpha1.ComputeSpecHash(landed.Spec, landed.Annotations)
	require.NoError(t, err)

	assert.Equal(t, want, got.Metadata.Annotations[v2alpha1.AnnotationExperimentExpectedSpecHash])
}
