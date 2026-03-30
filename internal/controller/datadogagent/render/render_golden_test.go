// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-operator/pkg/testutils"
)

// TestRenderManifests_MatchesCluster compares render output against golden files
// collected from a live cluster. The test skips if golden files are not present.
//
// To generate golden files:
//  1. Deploy the operator to a kind cluster
//  2. Apply the DDA: kubectl apply -f testdata/inputs/minimal.yaml
//  3. Wait for reconciliation: kubectl get dda -w
//  4. Collect:
//     go run ./internal/controller/datadogagent/render/cmd/collect/ \
//     -dda-name datadog -namespace datadog \
//     -output-dir internal/controller/datadogagent/render/testdata/golden/minimal
func TestRenderManifests_MatchesCluster(t *testing.T) {
	tests := []struct {
		name      string
		inputFile string
		goldenDir string
	}{
		{"minimal", "testdata/inputs/minimal.yaml", "testdata/golden/minimal"},
		{"all-features", "testdata/inputs/all-features.yaml", "testdata/golden/all-features"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := os.ReadDir(tt.goldenDir)
			if err != nil || len(entries) == 0 {
				t.Skipf("no golden files at %s; run collection script first", tt.goldenDir)
			}

			dda, err := loadDatadogAgent(tt.inputFile, "")
			require.NoError(t, err)

			objects, err := RenderManifests(dda, nil, v3Opts())
			require.NoError(t, err)
			require.NotEmpty(t, objects)

			s := newScheme()

			// Build map: "Kind_Name" → normalized rendered object
			rendered := map[string]client.Object{}
			for _, obj := range objects {
				setTypeMeta(obj, s)
				normalizeForComparison(obj)
				key := objectKey(obj)
				rendered[key] = obj
			}

			// Load golden files and compare
			goldenFiles, _ := filepath.Glob(filepath.Join(tt.goldenDir, "*.yaml"))
			require.NotEmpty(t, goldenFiles, "golden directory exists but contains no YAML files")

			for _, gf := range goldenFiles {
				baseName := strings.TrimSuffix(filepath.Base(gf), ".yaml")
				t.Run(baseName, func(t *testing.T) {
					goldenObj := loadAndNormalizeGolden(t, gf, s)
					renderedObj, ok := rendered[baseName]
					if !ok {
						t.Errorf("render did not produce %s (cluster had it)", baseName)
						return
					}

					diff := testutils.CompareKubeResource(renderedObj, goldenObj)
					assert.Empty(t, diff, "mismatch for %s:\n%s", baseName, diff)
				})
			}

			// Check for resources render produced but cluster didn't have
			for key := range rendered {
				goldenPath := filepath.Join(tt.goldenDir, key+".yaml")
				if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
					t.Errorf("render produced %s but no golden file exists (cluster didn't have it)", key)
				}
			}
		})
	}
}

// objectKey returns a "Kind_Name" key for a rendered object.
func objectKey(obj client.Object) string {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return fmt.Sprintf("%s_%s", gvk.Kind, obj.GetName())
}

// loadAndNormalizeGolden reads a golden YAML file and normalizes it for comparison.
func loadAndNormalizeGolden(t *testing.T, path string, s *runtime.Scheme) client.Object {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read golden file %s", path)

	// Decode to unstructured first to get GVK
	u := &unstructured.Unstructured{}
	require.NoError(t, yaml.Unmarshal(data, &u.Object), "failed to unmarshal golden file %s", path)

	// Try to decode to a typed object using the scheme
	gvk := u.GroupVersionKind()
	typedObj, err := s.New(gvk)
	if err != nil {
		// If the scheme doesn't know the type, return unstructured
		normalizeForComparison(u)
		return u
	}

	// Re-unmarshal into the typed object
	require.NoError(t, yaml.Unmarshal(data, typedObj), "failed to unmarshal golden file %s into typed object", path)

	obj, ok := typedObj.(client.Object)
	require.True(t, ok, "typed object does not implement client.Object")

	normalizeForComparison(obj)
	return obj
}

// normalizeForComparison strips fields that are expected to differ between
// render output and live cluster resources.
func normalizeForComparison(obj client.Object) {
	// Runtime fields (same as cleanObject)
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetManagedFields(nil)
	obj.SetGeneration(0)
	obj.SetOwnerReferences(nil)

	// Annotations that vary between render and cluster
	annotations := obj.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		delete(annotations, "agent.datadoghq.com/agentspechash")
		delete(annotations, "agent.datadoghq.com/ddaispechash")
		// Remove checksum annotations (checksum/<name>-custom-config)
		for k := range annotations {
			if strings.HasPrefix(k, "checksum/") {
				delete(annotations, k)
			}
		}
		if len(annotations) == 0 {
			obj.SetAnnotations(nil)
		} else {
			obj.SetAnnotations(annotations)
		}
	}

	// Strip status for unstructured objects (typed objects don't serialize status from render)
	if u, ok := obj.(*unstructured.Unstructured); ok {
		delete(u.Object, "status")
	}
}

