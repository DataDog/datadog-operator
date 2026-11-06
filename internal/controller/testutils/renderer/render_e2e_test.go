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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
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

// TestRender_DAPKeepsExplicitlyDisabledAPMHostPort checks that a profile APM
// host port does not enable host ports on default nodes when the DDA disables
// them explicitly. The profile port only changes its own nodes and the Service.
func TestRender_DAPKeepsExplicitlyDisabledAPMHostPort(t *testing.T) {
	dda, err := LoadDDA("testdata/apm-hostport-disabled-dda.yaml")
	require.NoError(t, err)

	daps, err := LoadDAPs([]string{"testdata/apm-hostport-profile.yaml"})
	require.NoError(t, err)

	baseline, _, err := Render(Options{DDA: dda.DeepCopy(), ProfileEnabled: true})
	require.NoError(t, err)
	var baselineTemplate *corev1.PodTemplateSpec
	for _, obj := range baseline {
		if ds, ok := obj.(*appsv1.DaemonSet); ok && ds.Name == "datadog-agent-agent" {
			baselineTemplate = ds.Spec.Template.DeepCopy()
		}
	}
	require.NotNil(t, baselineTemplate)

	objects, _, err := Render(Options{
		DDA:            dda,
		DAPs:           daps,
		ProfileEnabled: true,
	})
	require.NoError(t, err)

	traceHostPorts := map[string]int32{}
	for _, obj := range objects {
		ds, ok := obj.(*appsv1.DaemonSet)
		if !ok {
			continue
		}
		if ds.Name == "datadog-agent-agent" {
			assert.Equal(t, baselineTemplate, &ds.Spec.Template, "profile must not change the default Agent pod template")
		}
		for _, c := range ds.Spec.Template.Spec.Containers {
			for _, p := range c.Ports {
				if c.Name == "trace-agent" && p.Name == "traceport" {
					traceHostPorts[ds.Name] = p.HostPort
				}
			}
		}
	}

	require.Contains(t, traceHostPorts, "datadog-agent-agent", "default DaemonSet has no trace-agent traceport")
	assert.Zero(t, traceHostPorts["datadog-agent-agent"], "default DaemonSet must keep the DDA's disabled APM host port")
	assert.Equal(t, int32(9126), traceHostPorts["apm-hostport-profile-agent"])
}

// TestRender_DAPAPMHostPortDoesNotReachDefaultNodes checks that a profile APM
// host port leaves the default Agent pod template unchanged when the DDA does
// not explicitly disable host ports.
func TestRender_DAPAPMHostPortDoesNotReachDefaultNodes(t *testing.T) {
	tests := []struct {
		name string
		apm  *v2alpha1.APMFeatureConfig
		dap  string
		// wantProfileDS is the profile DaemonSet that must be rendered, if any.
		wantProfileDS string
	}{
		{
			// Control: an override-only profile never touches default nodes.
			name:          "override-only profile",
			apm:           &v2alpha1.APMFeatureConfig{Enabled: ptr.To(true)},
			dap:           "testdata/linux-profile.yaml",
			wantProfileDS: "linux-profile-agent",
		},
		{
			// Default nodes run no trace-agent, so the profile can own the Service port.
			name:          "DDA APM disabled",
			apm:           &v2alpha1.APMFeatureConfig{Enabled: ptr.To(false)},
			dap:           "testdata/apm-hostport-profile.yaml",
			wantProfileDS: "apm-hostport-profile-agent",
		},
		{
			wantProfileDS: "apm-hostport-profile-agent",
			name:          "DDA APM omitted",
			dap:           "testdata/apm-hostport-profile.yaml",
		},
		{
			wantProfileDS: "apm-hostport-profile-agent",
			name:          "DDA host port config omitted",
			apm:           &v2alpha1.APMFeatureConfig{Enabled: ptr.To(true)},
			dap:           "testdata/apm-hostport-profile.yaml",
		},
		{
			wantProfileDS: "apm-hostport-profile-agent",
			name:          "DDA host port enabled, port omitted",
			apm: &v2alpha1.APMFeatureConfig{
				Enabled:        ptr.To(true),
				HostPortConfig: &v2alpha1.HostPortConfig{Enabled: ptr.To(true)},
			},
			dap: "testdata/apm-hostport-profile.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda, err := LoadDDA("testdata/minimal-dda.yaml")
			require.NoError(t, err)
			if tt.apm != nil {
				dda.Spec.Features = &v2alpha1.DatadogFeatures{APM: tt.apm}
			}
			daps, err := LoadDAPs([]string{tt.dap})
			require.NoError(t, err)

			baseline := renderDaemonSets(t, Options{DDA: dda.DeepCopy(), ProfileEnabled: true})
			got := renderDaemonSets(t, Options{DDA: dda, DAPs: daps, ProfileEnabled: true})
			require.Contains(t, baseline, "datadog-agent-agent")
			require.Contains(t, got, "datadog-agent-agent")
			assert.Equal(t, baseline["datadog-agent-agent"].Spec.Template, got["datadog-agent-agent"].Spec.Template, "profile must not change the default Agent pod template")
			if tt.wantProfileDS != "" {
				assert.Contains(t, got, tt.wantProfileDS, "profile should be applied")
			} else {
				assert.NotContains(t, got, "apm-hostport-profile-agent", "conflicting profile must be rejected")
			}
		})
	}
}

// TestRender_DAPAPMHostPortServiceTargetsEveryAgent checks that, with host
// networking, the local Service traceport targets the port every rendered
// trace-agent listens on.
func TestRender_DAPAPMHostPortServiceTargetsEveryAgent(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)
	dda.Spec.Features = &v2alpha1.DatadogFeatures{APM: &v2alpha1.APMFeatureConfig{Enabled: ptr.To(true)}}
	dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.NodeAgentComponentName: {HostNetwork: ptr.To(true)},
	}
	// One profile sets a host port; the other only overrides resources.
	daps, err := LoadDAPs([]string{"testdata/apm-hostport-profile.yaml", "testdata/gpu-profile.yaml"})
	require.NoError(t, err)

	objects, _, err := Render(Options{DDA: dda, DAPs: daps, ProfileEnabled: true})
	require.NoError(t, err)

	var targetPort *string
	listenPorts := map[string]int32{}
	for _, obj := range objects {
		switch o := obj.(type) {
		case *corev1.Service:
			if o.Name != "datadog-agent-agent" {
				continue
			}
			for _, p := range o.Spec.Ports {
				if p.Name == "traceport" {
					targetPort = ptr.To(p.TargetPort.StrVal)
				}
			}
		case *appsv1.DaemonSet:
			for _, c := range o.Spec.Template.Spec.Containers {
				for _, p := range c.Ports {
					if c.Name == "trace-agent" && p.Name == "traceport" {
						listenPorts[o.Name] = p.ContainerPort
					}
				}
			}
		}
	}

	require.NotNil(t, targetPort, "local Service has no traceport")
	require.Contains(t, listenPorts, "gpu-profile-agent", "override-only profile DaemonSet was not rendered")
	assert.Equal(t, "traceport", *targetPort)
	assert.Equal(t, map[string]int32{"datadog-agent-agent": 8126, "gpu-profile-agent": 8126, "apm-hostport-profile-agent": 9126}, listenPorts)

}

// renderDaemonSets returns the rendered DaemonSets by name.
func renderDaemonSets(t *testing.T, opts Options) map[string]*appsv1.DaemonSet {
	t.Helper()
	objects, _, err := Render(opts)
	require.NoError(t, err)
	daemonSets := map[string]*appsv1.DaemonSet{}
	for _, obj := range objects {
		if ds, ok := obj.(*appsv1.DaemonSet); ok {
			daemonSets[ds.Name] = ds
		}
	}
	return daemonSets
}

func TestRender_DAPLanguageDetectionUsesSharedDCA(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		baseSSI, baseDetection, apmDisabled bool
		profileSSI, profileDetection        *bool
		wantDetection                       bool
	}{
		{name: "resource-only profiles inherit detection", baseSSI: true, baseDetection: true, wantDetection: true},
		{name: "profile enables shared detection", baseSSI: true, profileDetection: ptr.To(true), wantDetection: true},
		{name: "profile disables shared detection", baseSSI: true, baseDetection: true, profileDetection: ptr.To(false)},
		{name: "profile disables SSI", baseSSI: true, baseDetection: true, profileSSI: ptr.To(false)},
		{name: "disabled SSI gates explicit detection", profileDetection: ptr.To(true)},
		{name: "profile enables SSI", baseDetection: true, profileSSI: ptr.To(true), wantDetection: true},
		{name: "APM disabled does not disable explicit detection", baseSSI: true, baseDetection: true, apmDisabled: true, wantDetection: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dda, err := LoadDDA("testdata/minimal-dda.yaml")
			require.NoError(t, err)
			dda.Spec.Features = &v2alpha1.DatadogFeatures{APM: &v2alpha1.APMFeatureConfig{
				Enabled: ptr.To(!tc.apmDisabled),
				SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
					Enabled:           ptr.To(tc.baseSSI),
					LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(tc.baseDetection)},
				},
			}}
			profiles, err := LoadDAPs([]string{"testdata/linux-profile.yaml", "testdata/gpu-profile.yaml"})
			require.NoError(t, err)
			if tc.profileSSI != nil || tc.profileDetection != nil {
				ssi := &v2alpha1.SingleStepInstrumentation{Enabled: tc.profileSSI}
				if tc.profileDetection != nil {
					ssi.LanguageDetection = &v2alpha1.LanguageDetectionConfig{Enabled: tc.profileDetection}
				}
				profiles[0].Spec.Config.Features = &v2alpha1.DatadogFeatures{APM: &v2alpha1.APMFeatureConfig{SingleStepInstrumentation: ssi}}
			}
			objects, _, err := Render(Options{DDA: dda, DAPs: profiles, ProfileEnabled: true})
			require.NoError(t, err)
			var dcaCount, detectionRoles int
			nodes := map[string]bool{}
			for _, obj := range objects {
				switch o := obj.(type) {
				case *appsv1.Deployment:
					dcaCount++
					assert.Equal(t, "datadog-agent-cluster-agent", o.Name)
					for _, c := range o.Spec.Template.Spec.Containers {
						if c.Name == "cluster-agent" {
							enabled := false
							for _, env := range c.Env {
								if env.Name == "DD_LANGUAGE_DETECTION_ENABLED" {
									enabled = env.Value == "true"
								}
							}
							assert.Equal(t, tc.wantDetection, enabled, "DCA detection")
						}
					}
				case *appsv1.DaemonSet:
					for _, c := range o.Spec.Template.Spec.Containers {
						if c.Name != "agent" {
							continue
						}
						envs := map[string]corev1.EnvVar{}
						for _, env := range c.Env {
							envs[env.Name] = env
						}
						nodes[o.Name] = envs["DD_LANGUAGE_DETECTION_REPORTING_ENABLED"].Value == "true"
						assert.Equal(t, tc.wantDetection, envs["DD_LANGUAGE_DETECTION_ENABLED"].Value == "true", o.Name)
						assert.Equal(t, "datadog-agent-cluster-agent", envs["DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME"].Value, o.Name)
						assert.Equal(t, "datadog-agent-token", envs["DD_CLUSTER_AGENT_TOKEN_NAME"].Value, o.Name)
					}
				case *rbacv1.ClusterRole:
					if strings.HasSuffix(o.Name, "-apm-cluster-agent") {
						detectionRoles++
						assert.True(t, strings.HasPrefix(o.Name, "datadog-agent-"), "profiles must not create shared RBAC")
					}
				}
			}
			assert.Equal(t, 1, dcaCount)
			assert.Equal(t, map[string]bool{"datadog-agent-agent": tc.wantDetection, "linux-profile-agent": tc.wantDetection, "gpu-profile-agent": tc.wantDetection}, nodes)
			if tc.wantDetection {
				assert.Equal(t, 1, detectionRoles)
			} else {
				assert.Zero(t, detectionRoles)
			}
		})
	}
}

func TestRender_DAPOmittedAPMPortDoesNotConflict(t *testing.T) {
	for _, explicitFirst := range []bool{false, true} {
		name := "defaulted port first"
		if explicitFirst {
			name = "explicit port first"
		}
		t.Run(name, func(t *testing.T) {
			dda, err := LoadDDA("testdata/minimal-dda.yaml")
			require.NoError(t, err)
			dda.Spec.Features = &v2alpha1.DatadogFeatures{APM: &v2alpha1.APMFeatureConfig{Enabled: ptr.To(false)}}
			dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {HostNetwork: ptr.To(true)},
			}
			daps, err := LoadDAPs([]string{"testdata/apm-hostport-profile.yaml"})
			require.NoError(t, err)
			explicit := daps[0]
			omitted := explicit.DeepCopy()
			omitted.Spec.Config.Features.APM.HostPortConfig = nil
			first, second := omitted, explicit
			wantPort := int32(9126)
			if explicitFirst {
				first, second = explicit, omitted
				wantPort = 9126
			}
			first.Name, second.Name = "a-first", "b-second"
			objects, _, err := Render(Options{DDA: dda, DAPs: []*v1alpha1.DatadogAgentProfile{first, second}, ProfileEnabled: true})
			require.NoError(t, err)
			foundAgents, foundService := map[string]int32{}, false
			for _, obj := range objects {
				switch o := obj.(type) {
				case *appsv1.DaemonSet:
					for _, c := range o.Spec.Template.Spec.Containers {
						if c.Name == "trace-agent" {
							for _, p := range c.Ports {
								if p.Name == "traceport" {
									foundAgents[o.Name] = p.ContainerPort
								}
							}
						}
					}
				case *corev1.Service:
					if o.Name == "datadog-agent-agent" {
						for _, p := range o.Spec.Ports {
							if p.Name == "traceport" {
								foundService = true
								assert.Equal(t, wantPort, p.Port)
								assert.Equal(t, "traceport", p.TargetPort.StrVal)
							}
						}
					}
				}
			}
			assert.Equal(t, int32(9126), foundAgents[explicit.Name+"-agent"])
			assert.Equal(t, int32(8126), foundAgents[omitted.Name+"-agent"])
			assert.True(t, foundService)
		})
	}
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
