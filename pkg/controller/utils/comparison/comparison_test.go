// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package comparison

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

func TestGenerateMD5ForSpec(t *testing.T) {
	t.Run("returns the same hash for the same spec", func(t *testing.T) {
		spec := map[string]any{
			"agent": map[string]any{
				"enabled": true,
			},
		}

		firstHash, err := GenerateMD5ForSpec(spec)
		require.NoError(t, err)
		require.NotEmpty(t, firstHash)

		secondHash, err := GenerateMD5ForSpec(spec)
		require.NoError(t, err)
		require.Equal(t, firstHash, secondHash)
	})

	t.Run("returns a marshal error for unsupported values", func(t *testing.T) {
		_, err := GenerateMD5ForSpec(map[string]any{
			"unsupported": make(chan struct{}),
		})
		require.Error(t, err)
	})
}

func TestSetMD5GenerationAnnotation(t *testing.T) {
	t.Run("initializes annotations and writes the generated hash", func(t *testing.T) {
		obj := &metav1.ObjectMeta{}
		spec := map[string]bool{"enabled": true}

		hash, err := SetMD5GenerationAnnotation(obj, spec, "custom-hash")
		require.NoError(t, err)

		require.Equal(t, hash, obj.Annotations["custom-hash"])
		require.True(t, IsSameMD5Hash(hash, obj.Annotations, "custom-hash"))
		require.False(t, IsSameMD5Hash("different-hash", obj.Annotations, "custom-hash"))
	})

	t.Run("uses the DatadogAgent annotation helper", func(t *testing.T) {
		obj := &metav1.ObjectMeta{Annotations: map[string]string{}}

		hash, err := SetMD5DatadogAgentGenerationAnnotation(obj, map[string]string{"image": "agent"})
		require.NoError(t, err)

		require.Equal(t, hash, obj.Annotations[constants.MD5AgentDeploymentAnnotationKey])
		require.True(t, IsSameSpecMD5Hash(hash, obj.Annotations))
	})
}
