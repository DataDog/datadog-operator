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
		{name: "newer major", image: "agent:8.0.0", expectedPath: host},
		{name: "prefixed version", image: "agent:v7.86.0", expectedPath: host},
		{name: "old prefixed version", image: "agent:v7.85.99", expectedPath: legacy},
		{name: "JMX", image: "agent:7.86.0-jmx", expectedPath: host},
		{name: "prefixed JMX", image: "agent:v7.86.0-jmx", expectedPath: host},
		{name: "full", image: "agent:7.86.0-full", expectedPath: host},
		{name: "FIPS", image: "agent:7.86.0-fips", expectedPath: host},
		{name: "FIPS JMX", image: "agent:7.86.0-fips-jmx", expectedPath: host},
		{name: "FIPS full", image: "agent:7.86.0-fips-full", expectedPath: host},
		{name: "devel predates resolver", image: "agent:7.86.0-devel", expectedPath: legacy},
		{name: "RC", image: "agent:7.86.0-rc.1", expectedPath: legacy},
		{name: "future RC", image: "agent:7.87.0-rc.1", expectedPath: legacy},
		{name: "devel full variant", image: "agent:7.86.0-devel-full", expectedPath: legacy},
		{name: "future RC JMX variant", image: "agent:7.87.0-rc.1-jmx", expectedPath: legacy},
		{name: "major alias", image: "agent:7", expectedPath: legacy},
		{name: "minor alias", image: "agent:7.86", expectedPath: host},
		{name: "old minor alias", image: "agent:7.85", expectedPath: legacy},
		{name: "minor alias FIPS full variant", image: "agent:7.86-fips-full", expectedPath: host},
		{name: "latest alias", image: "agent:latest", expectedPath: legacy},
		{name: "custom tag", image: "agent:local-build", expectedPath: legacy},
		{name: "version-looking custom tag", image: "agent:branch-7.86.0", expectedPath: legacy},
		{name: "custom suffix", image: "agent:7.86.0-custom", expectedPath: legacy},
		{name: "custom suffix full variant", image: "agent:7.86.0-custom-full", expectedPath: legacy},
		{name: "embedded version JMX variant", image: "agent:branch-7.86.0-jmx", expectedPath: legacy},
		{name: "dash separated custom tag", image: "agent:7-86-0", expectedPath: legacy},
		{name: "extra version component", image: "agent:7.86.0.1", expectedPath: legacy},
		{name: "empty tag", image: "agent:", expectedPath: legacy},
		{name: "untagged mirror with port", image: "registry.example:5000/mirror/agent", expectedPath: legacy},
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
		{name: "autopilot provider", image: "agent:7.86.0", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.GKEAutopilotProvider}, expectedPath: host},
		{name: "autopilot annotation", image: "agent:7.86.0", annotations: map[string]string{experimental.ExperimentalAnnotationPrefix + "/autopilot": "true"}, expectedPath: host},
		{name: "autopilot old stable", image: "agent:7.85.0", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.GKEAutopilotProvider}, expectedPath: legacy},
		{name: "autopilot future RC", image: "agent:7.87.0-rc.1", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.GKEAutopilotProvider}, expectedPath: legacy},
		{name: "autopilot digest", image: "agent:7.86.0@sha256:aaaaaaaa", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.GKEAutopilotProvider}, expectedPath: legacy},
		{name: "autopilot per container downgrade", image: "agent:7.86.0", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.GKEAutopilotProvider, imageOverride: `{"agent":{"tag":"7.85.0"}}`}, expectedPath: legacy},
		{name: "autopilot per container upgrade", image: "agent:7.85.0", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.GKEAutopilotProvider, imageOverride: `{"agent":{"tag":"7.86.0"}}`}, expectedPath: host},
		{name: "windows", image: "agent:7.86.0", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider}, expectedPath: legacy},
		{name: "windows with autopilot annotation", image: "agent:7.86.0", annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider, experimental.ExperimentalAnnotationPrefix + "/autopilot": "true"}, expectedPath: legacy},
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

func TestProcessPasswdMountPathAutopilotAllowlist(t *testing.T) {
	const host = "/host/etc/passwd"
	const legacy = common.PasswdMountPath
	const allowlistAnnotation = experimental.ExperimentalAnnotationPrefix + "/" + experimental.ExperimentalAutopilotAllowlistVersionSubkey
	tests := []struct {
		name         string
		version      string
		expectedPath string
	}{
		{name: "default", expectedPath: host},
		{name: "first compatible allowlist", version: "v1.0.8", expectedPath: host},
		{name: "newer patch", version: "v1.0.9", expectedPath: host},
		{name: "newer minor", version: "v1.1.0", expectedPath: host},
		{name: "newer major", version: "v2.0.0", expectedPath: host},
		{name: "previous allowlist", version: "v1.0.7", expectedPath: legacy},
		{name: "older allowlist", version: "v1.0.0", expectedPath: legacy},
		{name: "leading zero old version", version: "v01.00.007", expectedPath: legacy},
		{name: "missing prefix falls back to default", version: "1.0.7", expectedPath: host},
		{name: "prerelease falls back to default", version: "v1.0.7-rc.1", expectedPath: host},
		{name: "metadata falls back to default", version: "v1.0.7+build", expectedPath: host},
		{name: "abbreviated version falls back to default", version: "v1.0", expectedPath: host},
		{name: "whitespace falls back to default", version: " v1.0.7 ", expectedPath: host},
		{name: "malformed falls back to default", version: "invalid", expectedPath: host},
		{name: "unparseable accepted format is conservative", version: "v18446744073709551616.0.0", expectedPath: legacy},
	}
	for _, provider := range []struct{ name, key, value string }{
		{"provider", kubernetes.ProviderAnnotationKey, kubernetes.GKEAutopilotProvider},
		{"experimental", experimental.ExperimentalAnnotationPrefix + "/" + experimental.ExperimentalAutopilotSubkey, "TrUe"},
	} {
		for _, containerName := range []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.ProcessAgentContainerName} {
			for _, tt := range tests {
				t.Run(provider.name+"/"+string(containerName)+"/"+tt.name, func(t *testing.T) {
					owner := &metav1.ObjectMeta{Annotations: map[string]string{
						provider.key:        provider.value,
						allowlistAnnotation: tt.version,
					}}
					pod := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: string(containerName), Image: "agent:7.86.0"}}}}
					expectedPod := pod.DeepCopy()
					assert.Equal(t, tt.expectedPath, ProcessPasswdMountPath(owner, nil, containerName, pod))
					assert.Equal(t, expectedPod, pod, "selection must not mutate the pod")
				})
			}
		}
	}
}
