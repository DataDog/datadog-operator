package feature

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestApplyProfileSharedConfigOverlaysNilTarget(t *testing.T) {
	err := ApplyProfileSharedConfigOverlays(nil, nil, nil, nil)

	assert.ErrorContains(t, err, "profile shared config overlay target spec is nil")
}

// Stub features exercise raw accumulation across separate shared fields.
func TestProfileSharedConfigAcrossFeatures(t *testing.T) {
	original := profileSharedConfigOverlays
	profileSharedConfigOverlays = map[IDType]ProfileSharedConfigOverlayFunc{}
	t.Cleanup(func() { profileSharedConfigOverlays = original })

	field := func(spec *v2alpha1.DatadogAgentSpec, id IDType) **string {
		if spec.Global == nil {
			spec.Global = &v2alpha1.GlobalConfig{}
		}
		if id == "first" {
			return &spec.Global.ClusterName
		}
		return &spec.Global.Site
	}
	var calls []IDType
	for _, id := range []IDType{"second", "first"} {
		err := RegisterProfileSharedConfigOverlay(id, func(raw, dst, base, profile *v2alpha1.DatadogAgentSpec) error {
			calls = append(calls, id)
			value := *field(profile, id)
			if value == nil {
				return nil
			}
			existing := *field(raw, id)
			if existing != nil && *existing != *value {
				return fmt.Errorf("conflicting explicit value")
			}
			*field(raw, id) = ptr.To(*value)
			*field(dst, id) = ptr.To(*value)
			return nil
		})
		require.NoError(t, err)
	}
	raw := &v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{ClusterName: ptr.To("explicit-cluster")}}
	base := raw.DeepCopy()
	base.Global.Site = ptr.To("default-site")
	shared := raw.DeepCopy()
	assert.Nil(t, shared.Global.Site)
	profile := &v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{
		ClusterName: ptr.To("explicit-cluster"), Site: ptr.To("profile-site"),
	}}
	require.NoError(t, ApplyProfileSharedConfigOverlays(shared, base, base.DeepCopy(), profile))
	assert.Equal(t, []IDType{"first", "second"}, calls)
	assert.Equal(t, "explicit-cluster", *base.Global.ClusterName)
	assert.Equal(t, "profile-site", *base.Global.Site)
	assert.Nil(t, raw.Global.Site)

	candidate := shared.DeepCopy()
	profile.Global.Site = ptr.To("conflicting-site")
	require.ErrorContains(t, ApplyProfileSharedConfigOverlays(candidate, base.DeepCopy(), base, profile), "second profile shared config overlay failed")
	assert.Equal(t, "profile-site", *shared.Global.Site)
}

func TestProfileSharedConfigRejectsBothCandidates(t *testing.T) {
	original := profileSharedConfigOverlays
	profileSharedConfigOverlays = map[IDType]ProfileSharedConfigOverlayFunc{}
	t.Cleanup(func() { profileSharedConfigOverlays = original })
	require.NoError(t, RegisterProfileSharedConfigOverlay("first", func(raw, dst, base, profile *v2alpha1.DatadogAgentSpec) error {
		raw.Global.ClusterName = ptr.To("candidate-cluster")
		dst.Global.ClusterName = ptr.To("candidate-cluster")
		return nil
	}))
	require.NoError(t, RegisterProfileSharedConfigOverlay("second", func(raw, dst, base, profile *v2alpha1.DatadogAgentSpec) error {
		return fmt.Errorf("profile conflict")
	}))
	raw := &v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
	dst := &v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{ClusterName: ptr.To("generated-cluster")}}
	rawCandidate, ddaiCandidate := raw.DeepCopy(), dst.DeepCopy()
	err := ApplyProfileSharedConfigOverlays(rawCandidate, ddaiCandidate, dst, &v2alpha1.DatadogAgentSpec{})
	require.ErrorContains(t, err, "second profile shared config overlay failed")
	assert.Equal(t, "candidate-cluster", *rawCandidate.Global.ClusterName)
	assert.Equal(t, "candidate-cluster", *ddaiCandidate.Global.ClusterName)
	assert.Nil(t, raw.Global.ClusterName)
	assert.Equal(t, "generated-cluster", *dst.Global.ClusterName)
}
