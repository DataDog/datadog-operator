// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestRender_AutopilotNPMProtocolClassification(t *testing.T) {
	for _, tt := range []struct {
		name            string
		annotationKey   string
		annotationValue string
		npmEnabled      bool
		wantDisabled    bool
	}{
		{"autopilot with NPM", kubernetes.ProviderAnnotationKey, kubernetes.GKEAutopilotProvider, true, true},
		{"experimental autopilot with NPM", experimental.ExperimentalAnnotationPrefix + "/" + experimental.ExperimentalAutopilotSubkey, "true", true, true},
		{"autopilot without NPM", kubernetes.ProviderAnnotationKey, kubernetes.GKEAutopilotProvider, false, false},
		{"default provider with NPM", "", "", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dda, err := LoadDDA("testdata/minimal-dda.yaml")
			require.NoError(t, err)
			if tt.annotationKey != "" {
				dda.Annotations = map[string]string{tt.annotationKey: tt.annotationValue}
			}
			dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {Image: &v2alpha1.AgentImageConfig{Tag: "7-rc"}},
			}
			dda.Spec.Features = &v2alpha1.DatadogFeatures{
				NPM: &v2alpha1.NPMFeatureConfig{Enabled: ptr.To(tt.npmEnabled)},
			}
			objects, _, err := Render(Options{DDA: dda})
			require.NoError(t, err)
			var daemonSetFound bool
			for _, object := range objects {
				ds, ok := object.(*appsv1.DaemonSet)
				if !ok {
					continue
				}
				daemonSetFound = true
				var workaroundFound bool
				for _, c := range ds.Spec.Template.Spec.Containers {
					for _, env := range c.Env {
						if env.Name != "DD_ENABLE_PROTOCOL_CLASSIFICATION" {
							continue
						}
						workaroundFound = true
						assert.Equal(t, string(apicommon.SystemProbeContainerName), c.Name)
						assert.Equal(t, corev1.EnvVar{Name: "DD_ENABLE_PROTOCOL_CLASSIFICATION", Value: "false"}, env)
					}
				}
				assert.Equal(t, tt.wantDisabled, workaroundFound)
			}
			require.True(t, daemonSetFound)
		})
	}
}
