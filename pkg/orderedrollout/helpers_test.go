// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orderedrollout

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func TestResolveConfig(t *testing.T) {
	ddaCfg := ResolveDDAConfig(nil)
	assert.Equal(t, DefaultConfig(), ddaCfg)
	assert.Equal(t, v2alpha1.RolloutTimeoutActionContinue, ddaCfg.OnStepTimeout)

	halt := v2alpha1.RolloutTimeoutActionHalt
	ddaCfg = ResolveDDAConfig(&v2alpha1.RolloutStrategy{
		Priority:       ptr.To[int32](5),
		StepTimeout:    &metav1.Duration{Duration: time.Hour},
		StepSoak:       &metav1.Duration{Duration: time.Minute},
		MaxUnavailable: ptr.To(intstr.FromString("10%")),
		OnStepTimeout:  &halt,
	})
	assert.Equal(t, int32(5), ddaCfg.Priority)
	assert.Equal(t, time.Hour, ddaCfg.StepTimeout)

	// DAP priority defaults to 0; other fields inherit.
	dapCfg := ResolveDAPConfig(ddaCfg, nil)
	assert.Equal(t, int32(0), dapCfg.Priority)
	assert.Equal(t, time.Hour, dapCfg.StepTimeout)
	assert.Equal(t, time.Minute, dapCfg.StepSoak)
	assert.Equal(t, intstr.FromString("10%"), dapCfg.MaxUnavailable)
	assert.Equal(t, halt, dapCfg.OnStepTimeout)

	dapCfg = ResolveDAPConfig(ddaCfg, &v2alpha1.RolloutStrategy{
		Priority:    ptr.To[int32](10),
		StepTimeout: &metav1.Duration{Duration: 20 * time.Minute},
	})
	assert.Equal(t, int32(10), dapCfg.Priority)
	assert.Equal(t, 20*time.Minute, dapCfg.StepTimeout)
	assert.Equal(t, time.Minute, dapCfg.StepSoak)
}

func TestResolveMaxUnavailable(t *testing.T) {
	tests := []struct {
		value   intstr.IntOrString
		desired int32
		want    int32
		wantErr bool
	}{
		{intstr.FromInt32(0), 50, 0, false},
		{intstr.FromInt32(3), 50, 3, false},
		{intstr.FromString("1%"), 50, 1, false},
		{intstr.FromString("1%"), 250, 3, false},
		{intstr.FromString("0%"), 50, 0, false},
		{intstr.FromString("10%"), 0, 0, false},
		{intstr.FromInt32(-1), 50, 0, true},
		{intstr.FromString("abc"), 50, 0, true},
	}
	for _, tt := range tests {
		got, err := ResolveMaxUnavailable(tt.value, tt.desired)
		if tt.wantErr {
			assert.Error(t, err, tt.value.String())
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, tt.want, got, tt.value.String())
	}
}

func TestIsInertAnnotation(t *testing.T) {
	inert := []string{
		"meta.helm.sh/release-name",
		"kubectl.kubernetes.io/last-applied-configuration",
		HoldAnnotation, SkipAnnotation, BypassAnnotation,
		TargetHashAnnotation,
		constants.MD5DDAIDeploymentAnnotationKey,
		constants.MD5AgentDeploymentAnnotationKey,
		constants.ConfigMapsChecksumAnnotationKey,
		"checksum/foo-custom-config",
	}
	for _, k := range inert {
		assert.True(t, IsInertAnnotation(k), k)
	}
	relevant := []string{
		"agent.datadoghq.com/cluster-provider",
		"experimental.agent.datadoghq.com/image-override-config",
		"agent.datadoghq.com/host-profiler-enabled",
		"example.com/anything",
	}
	for _, k := range relevant {
		assert.False(t, IsInertAnnotation(k), k)
	}
}

func TestTargetHash(t *testing.T) {
	base := map[string]string{"agent.datadoghq.com/cluster-provider": "gke-cos"}
	h := TargetHash("spec1", base)

	t.Run("ignores inert, control and hash annotations", func(t *testing.T) {
		withInert := map[string]string{
			"agent.datadoghq.com/cluster-provider":             "gke-cos",
			"meta.helm.sh/release-name":                        "x",
			"kubectl.kubernetes.io/last-applied-configuration": "{}",
			HoldAnnotation:                           "true",
			BypassAnnotation:                         "abc",
			TargetHashAnnotation:                     "previous",
			constants.MD5DDAIDeploymentAnnotationKey: "previous",
		}
		assert.Equal(t, h, TargetHash("spec1", withInert))
	})
	t.Run("stable when the object carries its own hash", func(t *testing.T) {
		annotations := map[string]string{"agent.datadoghq.com/cluster-provider": "gke-cos"}
		first := TargetHash("spec1", annotations)
		annotations[TargetHashAnnotation] = first
		annotations[constants.MD5DDAIDeploymentAnnotationKey] = "spec1"
		assert.Equal(t, first, TargetHash("spec1", annotations))
	})
	t.Run("includes rendering annotations", func(t *testing.T) {
		assert.NotEqual(t, h, TargetHash("spec1", map[string]string{"agent.datadoghq.com/cluster-provider": "eks"}))
		assert.NotEqual(t, h, TargetHash("spec1", nil))
	})
	t.Run("includes the spec", func(t *testing.T) {
		assert.NotEqual(t, h, TargetHash("spec2", base))
	})
}

func TestRolloutID(t *testing.T) {
	a := RolloutID(map[string]string{"default": "h1", "ns/a": "h2"})
	assert.Equal(t, a, RolloutID(map[string]string{"ns/a": "h2", "default": "h1"}))
	assert.NotEqual(t, a, RolloutID(map[string]string{"default": "h1", "ns/a": "h3"}))
}

func TestInertAnnotationPatch(t *testing.T) {
	desired := map[string]string{
		"meta.helm.sh/release-name":              "new",
		"agent.datadoghq.com/cluster-provider":   "eks",
		TargetHashAnnotation:                     "target",
		constants.MD5DDAIDeploymentAnnotationKey: "spec-new",
	}
	live := map[string]string{
		"meta.helm.sh/release-name":              "old",
		"meta.helm.sh/release-namespace":         "gone",
		"agent.datadoghq.com/cluster-provider":   "gke",
		TargetHashAnnotation:                     "live",
		constants.MD5DDAIDeploymentAnnotationKey: "spec-old",
	}
	patch := InertAnnotationPatch(desired, live)
	assert.Equal(t, map[string]*string{
		"meta.helm.sh/release-name":      ptr.To("new"),
		"meta.helm.sh/release-namespace": nil,
	}, patch)
	assert.Nil(t, InertAnnotationPatch(live, live))
}

func TestEvaluateDaemonSet(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	ds := func(gen, observed int64, desired, updated, unavailable int32) *DaemonSetFacts {
		return &DaemonSetFacts{Generation: gen, ObservedGeneration: observed, DesiredNumberScheduled: desired, UpdatedNumberScheduled: updated, NumberUnavailable: unavailable}
	}

	tests := []struct {
		name       string
		ds         *DaemonSetFacts
		cfg        Config
		soak       *metav1.Time
		wantDone   bool
		wantReason string
	}{
		{"missing", nil, cfg, nil, false, ReasonDaemonSetMissing},
		{"stale observed generation", ds(2, 1, 3, 3, 0), cfg, nil, false, ReasonObservedGenerationStale},
		{"updating", ds(2, 2, 3, 2, 0), cfg, nil, false, ReasonUpdating},
		{"unavailable above strict threshold", ds(2, 2, 3, 3, 1), cfg, nil, false, ReasonUnavailableAboveThreshold},
		{"unavailable within percentage", ds(2, 2, 100, 100, 1), Config{MaxUnavailable: intstr.FromString("1%")}, nil, true, ReasonComplete},
		{"complete", ds(2, 2, 3, 3, 0), cfg, nil, true, ReasonComplete},
		{"zero nodes after observed generation", ds(2, 2, 0, 0, 0), cfg, nil, true, ReasonComplete},
		{"zero nodes before observed generation", ds(2, 1, 0, 0, 0), cfg, nil, false, ReasonObservedGenerationStale},
		{"soak starts", ds(2, 2, 3, 3, 0), Config{StepSoak: time.Minute}, nil, false, ReasonSoaking},
		{"soak not elapsed", ds(2, 2, 3, 3, 0), Config{StepSoak: time.Minute}, &metav1.Time{Time: now.Add(-30 * time.Second)}, false, ReasonSoaking},
		{"soak elapsed", ds(2, 2, 3, 3, 0), Config{StepSoak: time.Minute}, &metav1.Time{Time: now.Add(-2 * time.Minute)}, true, ReasonComplete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := EvaluateDaemonSet(tt.ds, tt.cfg, tt.soak, now)
			assert.Equal(t, tt.wantDone, c.Complete)
			assert.Equal(t, tt.wantReason, c.Reason)
		})
	}

	t.Run("soak start is recorded and reset outside threshold", func(t *testing.T) {
		c := EvaluateDaemonSet(ds(2, 2, 3, 3, 0), Config{StepSoak: time.Minute}, nil, now)
		require.NotNil(t, c.SoakStartedAt)
		assert.True(t, c.SoakStartedAt.Time.Equal(now))
		c = EvaluateDaemonSet(ds(2, 2, 3, 3, 1), Config{StepSoak: time.Minute}, c.SoakStartedAt, now)
		assert.Nil(t, c.SoakStartedAt)
	})
}
