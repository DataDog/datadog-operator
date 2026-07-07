// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// update regenerates the golden files instead of comparing against them.
// Run: go test ./internal/controller/testutils/renderer/ -run TestRender_Golden -update
var update = flag.Bool("update", false, "update golden files")

// goldenImageTag pins the node Agent and Cluster Agent image tags used by
// TestRender_Golden. Without this, the golden files embed whatever
// pkg/images/images.go currently defaults to, so every default-version bump
// would force a golden regen unrelated to the change being tested. Kept a
// plausible current semver since some rendering paths gate behavior on it.
const goldenImageTag = "7.80.0"

// pinImageTags overrides image tags on the in-memory DDA before rendering,
// rather than in the testdata fixtures, so the fixtures stay representative
// of real user input. It merges into any existing per-component override
// (e.g. testdata/override-dda.yaml already overrides nodeAgent.volumes)
// instead of replacing it.
func pinImageTags(dda *datadoghqv2alpha1.DatadogAgent) {
	if dda.Spec.Override == nil {
		dda.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{}
	}
	setImageTag(dda, datadoghqv2alpha1.NodeAgentComponentName)
	setImageTag(dda, datadoghqv2alpha1.ClusterAgentComponentName)
}

func setImageTag(dda *datadoghqv2alpha1.DatadogAgent, component datadoghqv2alpha1.ComponentName) {
	override, ok := dda.Spec.Override[component]
	if !ok || override == nil {
		override = &datadoghqv2alpha1.DatadogAgentComponentOverride{}
		dda.Spec.Override[component] = override
	}
	override.Image = &datadoghqv2alpha1.AgentImageConfig{Tag: goldenImageTag}
}

// TestRender_Golden is the regression safety net for the GKE Autopilot refactor
// (experimental imperative overrides → providercaps declarative framework).
//
// Each case renders a fixture with a trigger injected here (not in the fixture
// file) and byte-compares the full rendered manifest against a committed golden.
// GKE Autopilot is triggered via the experimental opt-in annotation, which works
// both before the refactor (original autopilot.go fires on it) and after (the DDA
// controller maps it to the gke-autopilot provider) — so the golden diff between
// the safety-net commit and the refactor commit is exactly the refactor's delta.
// EKS uses its provider annotation directly.
func TestRender_Golden(t *testing.T) {
	tests := []struct {
		name      string
		ddaFile   string
		autopilot bool   // inject the experimental GKE Autopilot opt-in annotation
		provider  string // inject the provider annotation (e.g. EKS); "" = none
		golden    string
	}{
		{
			name:      "comprehensive dda, autopilot",
			ddaFile:   "testdata/comprehensive-dda.yaml",
			autopilot: true,
			golden:    "testdata/golden/comprehensive-autopilot.golden.yaml",
		},
		{
			name:    "comprehensive dda, baseline (no provider)",
			ddaFile: "testdata/comprehensive-dda.yaml",
			golden:  "testdata/golden/comprehensive-baseline.golden.yaml",
		},
		{
			name:      "minimal dda, autopilot",
			ddaFile:   "testdata/minimal-dda.yaml",
			autopilot: true,
			golden:    "testdata/golden/minimal-autopilot.golden.yaml",
		},
		{
			name:    "minimal dda, baseline (no provider)",
			ddaFile: "testdata/minimal-dda.yaml",
			golden:  "testdata/golden/minimal-baseline.golden.yaml",
		},
		{
			// A user override re-adds volumes GKE Autopilot strips. Pre-refactor the
			// overrides applied last and were stripped; post-refactor removals run
			// before overrides, so override content survives — a reviewed diff.
			name:      "override dda, autopilot",
			ddaFile:   "testdata/override-dda.yaml",
			autopilot: true,
			golden:    "testdata/golden/override-autopilot.golden.yaml",
		},
		{
			name:    "override dda, baseline (no provider)",
			ddaFile: "testdata/override-dda.yaml",
			golden:  "testdata/golden/override-baseline.golden.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda, err := LoadDDA(tt.ddaFile)
			require.NoError(t, err)

			if dda.Annotations == nil {
				dda.Annotations = map[string]string{}
			}
			if tt.autopilot {
				dda.Annotations[experimental.ExperimentalAnnotationPrefix+"/"+experimental.ExperimentalAutopilotSubkey] = "true"
			}
			if tt.provider != "" {
				dda.Annotations[kubernetes.ProviderAnnotationKey] = tt.provider
			}
			pinImageTags(dda)

			objects, scheme, err := Render(Options{DDA: dda})
			require.NoError(t, err)

			out, err := Serialize(objects, scheme, "yaml", false)
			require.NoError(t, err)

			assertGolden(t, tt.golden, out)
		})
	}
}

// assertGolden compares got against the golden file at path. With -update it
// writes got to the file instead (creating the directory if needed).
func assertGolden(t *testing.T, path string, got []byte) {
	t.Helper()

	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}

	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden %s; regenerate with -update", path)
	require.Equal(t, string(want), string(got),
		"rendered output differs from golden %s; if intentional, regenerate with -update", path)
}
