// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// baseKindCounts is the expected resource inventory for a minimal DatadogAgent
// with no explicitly enabled features. Instrumentation CRD is enabled by
// default because the default Agent versions meet its minimum version. The
// three Services are the Cluster Agent, the admission controller, and the node
// Agent local service (rendered because the simulated Kubernetes version is >=
// 1.22).
var baseKindCounts = map[string]int{
	"ServiceAccount":       2,
	"ClusterRole":          6,
	"ClusterRoleBinding":   6,
	"Role":                 1,
	"RoleBinding":          1,
	"Secret":               1,
	"ConfigMap":            5,
	"Service":              3,
	"DatadogAgentInternal": 1,
	"DaemonSet":            1,
	"Deployment":           1,
}

// baseKindSeq is the expected serialized kind order for the same input,
// reflecting the dependency-aware sort in kindOrder.
var baseKindSeq = []string{
	"ServiceAccount", "ServiceAccount",
	"ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole",
	"ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding",
	"Role",
	"RoleBinding",
	"Secret",
	"ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap",
	"Service", "Service", "Service",
	"DatadogAgentInternal",
	"DaemonSet",
	"Deployment",
}

// TestRender_MinimalDDA is the primary e2e test. It loads a real DatadogAgent
// manifest, runs both reconciliation passes, and asserts:
//   - exact kind inventory
//   - full serialized kind order (dependency-aware + alphabetical within kind)
//   - dynamic metadata fields are stripped from serialized output
func TestRender_MinimalDDA(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)

	objects, scheme, err := Render(Options{DDA: dda})
	require.NoError(t, err)

	assert.Equal(t, baseKindCounts, countKinds(objects, scheme))

	out, err := Serialize(objects, scheme, "yaml", false)
	require.NoError(t, err)
	s := string(out)

	assert.Equal(t, baseKindSeq, kindSequence(s))

	for _, banned := range []string{"resourceVersion:", "generation:", "creationTimestamp:", "managedFields:"} {
		assert.NotContains(t, s, banned, "field %q must be stripped", banned)
	}
}

// TestRender_WithDAP exercises the ProfileEnabled code path with two valid profiles.
// Each profile produces its own DDAI and DaemonSet, so the output has 3 DDAIs
// (default + linux-profile + gpu-profile) and 3 DaemonSets.
func TestRender_WithDAP(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)

	daps, err := LoadDAPs([]string{"testdata/linux-profile.yaml", "testdata/gpu-profile.yaml"})
	require.NoError(t, err)

	objects, scheme, err := Render(Options{
		DDA:            dda,
		DAPs:           daps,
		ProfileEnabled: true,
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]int{
		"ServiceAccount":       2,
		"ClusterRole":          6,
		"ClusterRoleBinding":   6,
		"Role":                 1,
		"RoleBinding":          1,
		"Secret":               1,
		"ConfigMap":            5,
		"Service":              3,
		"DatadogAgentInternal": 3,
		"DaemonSet":            3,
		"Deployment":           1,
	}, countKinds(objects, scheme))

	out, err := Serialize(objects, scheme, "yaml", false)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"ServiceAccount", "ServiceAccount",
		"ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole",
		"ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding",
		"Role",
		"RoleBinding",
		"Secret",
		"ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap",
		"Service", "Service", "Service",
		"DatadogAgentInternal", "DatadogAgentInternal", "DatadogAgentInternal",
		"DaemonSet", "DaemonSet", "DaemonSet",
		"Deployment",
	}, kindSequence(string(out)))
}

func TestRender_AppArmorProfileVersionGate(t *testing.T) {
	tests := []struct {
		name              string
		kubernetesVersion string
		wantAnnotation    bool
		wantProfileField  bool
	}{
		{
			name:              "Kubernetes 1.29 uses the annotation",
			kubernetesVersion: "v1.29.9",
			wantAnnotation:    true,
		},
		{
			name:              "Kubernetes 1.30 uses the field",
			kubernetesVersion: "v1.30.0",
			wantProfileField:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda, err := LoadDDA("testdata/comprehensive-dda.yaml")
			require.NoError(t, err)

			objects, scheme, err := Render(Options{DDA: dda, KubernetesVersion: tt.kubernetesVersion})
			require.NoError(t, err)

			out, err := Serialize(objects, scheme, "yaml", false)
			require.NoError(t, err)
			rendered := string(out)

			assert.Equal(t, tt.wantAnnotation, strings.Contains(rendered, "container.apparmor.security.beta.kubernetes.io/system-probe: unconfined"))
			assert.Equal(t, tt.wantProfileField, strings.Contains(rendered, "appArmorProfile:\n            type: Unconfined"))
		})
	}
}

// kindSequence extracts the ordered list of "kind: X" values from serialized YAML.
func kindSequence(yaml string) []string {
	var kinds []string
	for line := range strings.SplitSeq(yaml, "\n") {
		if kind, ok := strings.CutPrefix(line, "kind: "); ok {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// countKinds tallies each GVK Kind in the object slice.
func countKinds(objects []client.Object, scheme *runtime.Scheme) map[string]int {
	counts := map[string]int{}
	for _, obj := range objects {
		counts[resolveKind(obj, scheme)]++
	}
	return counts
}

// TestRender_OpenShiftSCCDenied covers the safety property of the OpenShift
// ServiceAccount wiring: when authorization cannot be confirmed, nothing changes.
//
// The golden cases all run authorized, and the decision itself is unit tested in the
// openshift package. What neither covers is the RBAC half — the operator binds its
// agent ClusterRole to whichever name is resolved, so a regression could leave the
// pod on one ServiceAccount and the binding on another, which is worse than either
// outcome alone. That needs a real render, but not a 2k-line golden.
func TestRender_OpenShiftSCCDenied(t *testing.T) {
	const (
		defaultSA = "datadog-agent-agent"
		bundleSA  = "datadog-agent-scc"
	)

	tests := []struct {
		name       string
		sccAllowed bool
		wantSA     string
	}{
		{
			name:       "denied: default ServiceAccount is kept",
			sccAllowed: false,
			wantSA:     defaultSA,
		},
		{
			name:       "allowed: the bundle ServiceAccount is adopted",
			sccAllowed: true,
			wantSA:     bundleSA,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda, err := LoadDDA("testdata/openshift-dda.yaml")
			require.NoError(t, err)
			if dda.Annotations == nil {
				dda.Annotations = map[string]string{}
			}
			dda.Annotations[kubernetes.ProviderAnnotationKey] = "openshift-rhcos"

			objects, scheme, err := Render(Options{DDA: dda, SCCAllowed: tt.sccAllowed})
			require.NoError(t, err)

			var (
				podSA       string
				bindingSubj []string
				sawAgentDS  bool
			)
			for _, obj := range objects {
				switch o := obj.(type) {
				case *appsv1.DaemonSet:
					sawAgentDS = true
					podSA = o.Spec.Template.Spec.ServiceAccountName
				case *rbacv1.ClusterRoleBinding:
					if o.Name == defaultSA { // the node agent's ClusterRoleBinding
						for _, s := range o.Subjects {
							bindingSubj = append(bindingSubj, s.Name)
						}
					}
				}
			}

			require.True(t, sawAgentDS, "no node agent DaemonSet was rendered")
			assert.Equal(t, tt.wantSA, podSA, "DaemonSet ServiceAccount")
			assert.Equal(t, []string{tt.wantSA}, bindingSubj,
				"the agent ClusterRoleBinding must follow the same name as the pod")

			_ = scheme
		})
	}
}
