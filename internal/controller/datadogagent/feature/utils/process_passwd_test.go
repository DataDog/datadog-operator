// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestProcessPasswdMountPath(t *testing.T) {
	const host = common.HostPasswdMountPath
	const legacy = common.PasswdMountPath
	const imageOverride = experimental.ExperimentalAnnotationPrefix + "/image-override-config"
	tests := []struct {
		name         string
		image        *v2alpha1.AgentImageConfig
		annotations  map[string]string
		expectedPath string
	}{
		{
			name:         "no override uses the default Agent version",
			expectedPath: legacy,
		},
		{
			name: "old stable",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.85.99",
			},
			expectedPath: legacy,
		},
		{
			name: "first supported release",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.0",
			},
			expectedPath: host,
		},
		{
			name: "newer patch",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.1",
			},
			expectedPath: host,
		},
		{
			name: "newer minor",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.87.0",
			},
			expectedPath: host,
		},
		{
			name: "newer major",
			image: &v2alpha1.AgentImageConfig{
				Tag: "8.0.0",
			},
			expectedPath: host,
		},
		{
			name: "prefixed version",
			image: &v2alpha1.AgentImageConfig{
				Tag: "v7.86.0",
			},
			expectedPath: host,
		},
		{
			name: "JMX variant",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.0-jmx",
			},
			expectedPath: host,
		},
		{
			name: "full variant",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.0-full",
			},
			expectedPath: host,
		},
		{
			name: "FIPS full variant",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.0-fips-full",
			},
			expectedPath: host,
		},
		{
			name: "release candidate",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.0-rc.1",
			},
			expectedPath: host,
		},
		{
			name: "devel prerelease",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.0-devel",
			},
			expectedPath: host,
		},
		{
			name: "future release candidate",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.87.0-rc.1",
			},
			expectedPath: host,
		},
		{
			name: "old release candidate",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.85.0-rc.1",
			},
			expectedPath: legacy,
		},
		{
			name: "major alias",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7",
			},
			expectedPath: legacy,
		},
		{
			name: "latest alias",
			image: &v2alpha1.AgentImageConfig{
				Tag: "latest",
			},
			expectedPath: legacy,
		},
		{
			name: "minor alias",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86",
			},
			expectedPath: legacy,
		},
		{
			name: "custom tag",
			image: &v2alpha1.AgentImageConfig{
				Tag: "local-build",
			},
			expectedPath: legacy,
		},
		{
			name: "tag in name takes precedence over tag",
			image: &v2alpha1.AgentImageConfig{
				Name: "gcr.io/datadoghq/agent:7.85.0",
				Tag:  "7.86.0",
			},
			expectedPath: legacy,
		},
		{
			name: "registry port is not a tag",
			image: &v2alpha1.AgentImageConfig{
				Name: "registry.example:5000/datadog/agent",
				Tag:  "7.86.0",
			},
			expectedPath: host,
		},
		{
			name: "digest is not version evidence",
			image: &v2alpha1.AgentImageConfig{
				Name: "gcr.io/datadoghq/agent@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			expectedPath: legacy,
		},
		{
			name: "Windows keeps the legacy path",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.0",
			},
			annotations: map[string]string{
				kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider,
			},
			expectedPath: legacy,
		},
		// Known gap shared with other version-gated features: the experimental
		// per-container image annotation is not considered.
		{
			name: "experimental image upgrade is not considered",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.85.0",
			},
			annotations: map[string]string{
				imageOverride: `{"agent":{"tag":"7.86.0"}}`,
			},
			expectedPath: legacy,
		},
		{
			name: "experimental image downgrade is not considered",
			image: &v2alpha1.AgentImageConfig{
				Tag: "7.86.0",
			},
			annotations: map[string]string{
				imageOverride: `{"agent":{"tag":"7.85.0"}}`,
			},
			expectedPath: host,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedPath, ProcessPasswdMountPath(newPasswdTestDDA(tt.image, tt.annotations)))
		})
	}
}

// newPasswdTestDDA returns a DatadogAgent with the given node Agent image override and annotations.
func newPasswdTestDDA(image *v2alpha1.AgentImageConfig, annotations map[string]string) *v2alpha1.DatadogAgent {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
		},
	}
	if image != nil {
		dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {
				Image: image,
			},
		}
	}
	return dda
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
		{
			name:         "default",
			expectedPath: host,
		},
		{
			name:         "first compatible allowlist",
			version:      "v1.0.8",
			expectedPath: host,
		},
		{
			name:         "newer patch",
			version:      "v1.0.9",
			expectedPath: host,
		},
		{
			name:         "newer minor",
			version:      "v1.1.0",
			expectedPath: host,
		},
		{
			name:         "newer major",
			version:      "v2.0.0",
			expectedPath: host,
		},
		{
			name:         "previous allowlist",
			version:      "v1.0.7",
			expectedPath: legacy,
		},
		{
			name:         "older allowlist",
			version:      "v1.0.0",
			expectedPath: legacy,
		},
		{
			name:         "leading zero old version",
			version:      "v01.00.007",
			expectedPath: legacy,
		},
		{
			name:         "missing prefix falls back to default",
			version:      "1.0.7",
			expectedPath: host,
		},
		{
			name:         "prerelease falls back to default",
			version:      "v1.0.7-rc.1",
			expectedPath: host,
		},
		{
			name:         "metadata falls back to default",
			version:      "v1.0.7+build",
			expectedPath: host,
		},
		{
			name:         "abbreviated version falls back to default",
			version:      "v1.0",
			expectedPath: host,
		},
		{
			name:         "whitespace falls back to default",
			version:      " v1.0.7 ",
			expectedPath: host,
		},
		{
			name:         "malformed falls back to default",
			version:      "invalid",
			expectedPath: host,
		},
		{
			name:         "unparseable accepted format is conservative",
			version:      "v18446744073709551616.0.0",
			expectedPath: legacy,
		},
	}
	for _, provider := range []struct{ name, key, value string }{
		{"provider", kubernetes.ProviderAnnotationKey, kubernetes.GKEAutopilotProvider},
		{"experimental", experimental.ExperimentalAnnotationPrefix + "/" + experimental.ExperimentalAutopilotSubkey, "TrUe"},
	} {
		for _, tt := range tests {
			t.Run(provider.name+"/"+tt.name, func(t *testing.T) {
				dda := newPasswdTestDDA(&v2alpha1.AgentImageConfig{Tag: "7.86.0"}, map[string]string{
					provider.key:        provider.value,
					allowlistAnnotation: tt.version,
				})
				assert.Equal(t, tt.expectedPath, ProcessPasswdMountPath(dda))
			})
		}
	}
}

func TestProcessPasswdVolumeName(t *testing.T) {
	assert.Equal(t, common.PasswdVolumeName, ProcessPasswdVolumeName(common.PasswdMountPath))
	assert.Equal(t, common.HostPasswdVolumeName, ProcessPasswdVolumeName(common.HostPasswdMountPath))
}

func TestProcessPasswdVolume(t *testing.T) {
	_, mount := ProcessPasswdVolume(common.PasswdMountPath)
	assert.Equal(t, common.PasswdVolumeName, mount.Name)
	assert.Equal(t, common.PasswdMountPath, mount.MountPath)

	volume, mount := ProcessPasswdVolume(common.HostPasswdMountPath)
	assert.Equal(t, common.HostPasswdVolumeName, mount.Name)
	assert.Equal(t, common.HostPasswdMountPath, mount.MountPath)
	assert.True(t, mount.ReadOnly)
	assert.Equal(t, common.PasswdHostPath, volume.HostPath.Path)
}
