// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestProcessPasswdMountPath(t *testing.T) {
	const host = "/host/etc/passwd"
	const legacy = common.PasswdMountPath
	const imageOverride = experimental.ExperimentalAnnotationPrefix + "/image-override-config"
	tests := []struct {
		name         string
		image        string
		override     *v2alpha1.AgentImageConfig
		nameOverride *string
		annotations  map[string]string
		expectedPath string
	}{
		{name: "old stable", image: "agent:7.85.99", expectedPath: legacy},
		{name: "first supported release", image: "agent:7.86.0", expectedPath: host},
		{name: "newer patch", image: "agent:7.86.1", expectedPath: host},
		{name: "newer minor", image: "agent:7.87.0", expectedPath: host},
		{name: "JMX", image: "agent:7.86.0-jmx", expectedPath: host},
		{name: "full", image: "agent:7.86.0-full", expectedPath: host},
		{name: "FIPS", image: "agent:7.86.0-fips", expectedPath: host},
		{name: "FIPS JMX", image: "agent:7.86.0-fips-jmx", expectedPath: host},
		{name: "FIPS full", image: "agent:7.86.0-fips-full", expectedPath: host},
		{name: "devel predates resolver", image: "agent:7.86.0-devel", expectedPath: legacy},
		{name: "RC", image: "agent:7.86.0-rc.1", expectedPath: legacy},
		{name: "future RC", image: "agent:7.87.0-rc.1", expectedPath: legacy},
		{name: "major alias", image: "agent:7", expectedPath: legacy},
		{name: "minor alias", image: "agent:7.86", expectedPath: legacy},
		{name: "latest alias", image: "agent:latest", expectedPath: legacy},
		{name: "custom tag", image: "agent:local-build", expectedPath: legacy},
		{name: "version-looking custom tag", image: "agent:branch-7.86.0", expectedPath: legacy},
		{name: "custom suffix", image: "agent:7.86.0-custom", expectedPath: legacy},
		{name: "digest", image: "agent@sha256:aaaaaaaa", expectedPath: legacy},
		{name: "tag and digest", image: "agent:7.86.0@sha256:aaaaaaaa", expectedPath: legacy},
		{name: "mirror with port", image: "registry.example:5000/mirror/agent:7.86.0", expectedPath: host},
		{name: "tag override", image: "agent:7.85.0", override: &v2alpha1.AgentImageConfig{Tag: "7.86.0"}, expectedPath: host},
		{name: "name version wins over tag", image: "agent:7.85.0", override: &v2alpha1.AgentImageConfig{Name: "agent:7.85.0", Tag: "7.86.0"}, expectedPath: legacy},
		{name: "full name version wins over tag", image: "agent:7.85.0", override: &v2alpha1.AgentImageConfig{Name: "registry.example:5000/agent:7.86.0", Tag: "7.85.0"}, expectedPath: host},
		{name: "JMX option", image: "agent:7.85.0", override: &v2alpha1.AgentImageConfig{Tag: "7.86.0", JMXEnabled: true}, expectedPath: host},
		{name: "digest override", image: "agent:7.86.0", override: &v2alpha1.AgentImageConfig{Name: "agent@sha256:aaaaaaaa", Tag: "7.86.0"}, expectedPath: legacy},
		{name: "per container downgrade", image: "agent:7.86.0", annotations: map[string]string{imageOverride: `{"agent":{"tag":"7.85.0"}}`}, expectedPath: legacy},
		{name: "per container upgrade", image: "agent:7.85.0", annotations: map[string]string{imageOverride: `{"agent":{"name":"agent:7.86.0","tag":"7.85.0"}}`}, expectedPath: host},
		{name: "renamed container downgrade", image: "agent:7.86.0", nameOverride: new("custom-agent"), annotations: map[string]string{imageOverride: `{"custom-agent":{"tag":"7.85.0"}}`}, expectedPath: legacy},
		{name: "original container name ignored after rename", image: "agent:7.86.0", nameOverride: new("custom-agent"), annotations: map[string]string{imageOverride: `{"agent":{"tag":"7.85.0"}}`}, expectedPath: host},
		{name: "other container ignored", image: "agent:7.86.0", annotations: map[string]string{imageOverride: `{"trace-agent":{"tag":"7.85.0"}}`}, expectedPath: host},
		{name: "malformed annotation", image: "agent:7.86.0", annotations: map[string]string{imageOverride: `{`}, expectedPath: legacy},
		{name: "autopilot provider", image: "agent:7.86.0", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.GKEAutopilotProvider}, expectedPath: legacy},
		{name: "autopilot annotation", image: "agent:7.86.0", annotations: map[string]string{experimental.ExperimentalAnnotationPrefix + "/autopilot": "true"}, expectedPath: legacy},
		{name: "windows", image: "agent:7.86.0", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider}, expectedPath: legacy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := &metav1.ObjectMeta{Annotations: tt.annotations}
			var nodeOverride *v2alpha1.DatadogAgentComponentOverride
			if tt.override != nil || tt.nameOverride != nil {
				nodeOverride = &v2alpha1.DatadogAgentComponentOverride{Image: tt.override, Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					apicommon.CoreAgentContainerName: {Name: tt.nameOverride},
				}}
			}
			pod := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: tt.image}}}}
			expectedPod := pod.DeepCopy()
			assert.Equal(t, tt.expectedPath, ProcessPasswdMountPath(owner, nodeOverride, apicommon.CoreAgentContainerName, pod))
			assert.Equal(t, expectedPod, pod, "selection must not mutate image or environment")
		})
	}
}
