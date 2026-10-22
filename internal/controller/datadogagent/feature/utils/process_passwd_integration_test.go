// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils_test

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/cspm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/cws"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/liveprocess"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/processdiscovery"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

// This test only assembles pod templates in memory. It does not use a cluster.
func TestBothProcessFeaturesPasswdMount(t *testing.T) {
	for _, tt := range []struct {
		name         string
		tag          string
		single       bool
		security     bool
		systemProbe  bool
		talos        bool
		expectedPath string
	}{
		{name: "core", tag: "7.86.0", expectedPath: "/host/etc/passwd"},
		{name: "old core", tag: "7.85.0", expectedPath: "/etc/passwd"},
		{name: "unknown image", tag: "local-build", expectedPath: "/etc/passwd"},
		{name: "supported separate process agent", tag: "7.59.0", expectedPath: "/etc/passwd"},
		{name: "unprivileged single container", tag: "7.86.0", single: true, expectedPath: "/host/etc/passwd"},
		{name: "security agent unchanged", tag: "7.86.0", security: true, expectedPath: "/host/etc/passwd"},
		{name: "system probe unchanged", tag: "7.86.0", security: true, systemProbe: true, expectedPath: "/host/etc/passwd"},
		{name: "Talos exclusion", tag: "7.86.0", talos: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nodeOverride := v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{Tag: tt.tag},
				Env:   []corev1.EnvVar{{Name: "HOST_ETC", Value: "/custom/etc"}, {Name: "DD_IGNORE_HOST_ETC", Value: "true"}},
			}
			dda := testutils.NewDatadogAgentBuilder().WithLiveProcessEnabled(true).WithProcessDiscoveryEnabled(true).
				WithSingleContainerStrategy(tt.single).WithCSPMEnabled(tt.security).WithCWSEnabled(tt.security).
				WithCSPMRunInSystemProbe(tt.systemProbe).WithCWSDirectSendFromSystemProbe(tt.systemProbe).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, nodeOverride).Build()
			if tt.talos {
				dda.Annotations = map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.TalosProvider}
			}
			_, features, required, _ := feature.BuildFeatures(dda, &dda.Spec, nil, &feature.Options{Logger: logr.Discard()})
			pod := &corev1.PodTemplateSpec{}
			pod.Annotations = map[string]string{}
			for _, name := range required.Agent.Containers {
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: string(name), Image: "gcr.io/datadoghq/agent:7.85.0"})
			}
			manager := feature.NewPodTemplateManagers(pod)
			processFeatures := 0
			for _, f := range features {
				if f.ID() == feature.LiveProcessIDType || f.ID() == feature.ProcessDiscoveryIDType {
					processFeatures++
				}
				if tt.single {
					require.NoError(t, f.ManageSingleContainerNodeAgent(manager))
				} else {
					require.NoError(t, f.ManageNodeAgent(manager))
				}
				if contributor, ok := f.(feature.ProviderAwareFeature); ok {
					providercaps.ApplyProviderCapabilities(manager, dda.Annotations[kubernetes.ProviderAnnotationKey], contributor.NodeAgentProviderCapabilities())
				}
			}
			require.Equal(t, 2, processFeatures)
			override.PodTemplateSpec(logr.Discard(), manager, &nodeOverride, v2alpha1.NodeAgentComponentName, dda.Name)
			processMounts, securityMounts := 0, 0
			for _, container := range pod.Spec.Containers {
				count := 0
				for _, mount := range container.VolumeMounts {
					switch mount.Name {
					case common.PasswdVolumeName:
						count++
						assert.True(t, mount.ReadOnly)
						if container.Name == string(apicommon.SecurityAgentContainerName) || container.Name == string(apicommon.SystemProbeContainerName) {
							securityMounts++
							assert.Equal(t, "/etc/passwd", mount.MountPath)
						} else {
							processMounts++
							assert.Equal(t, tt.expectedPath, mount.MountPath)
						}
					case common.GroupVolumeName:
						assert.Equal(t, "/etc/group", mount.MountPath)
					}
				}
				assert.LessOrEqual(t, count, 1, "never dual-mount passwd")
				var hostEtcEnv []corev1.EnvVar
				for _, env := range container.Env {
					if env.Name == "HOST_ETC" || env.Name == "DD_IGNORE_HOST_ETC" {
						hostEtcEnv = append(hostEtcEnv, env)
					}
				}
				assert.ElementsMatch(t, nodeOverride.Env, hostEtcEnv, "preserve explicit configuration without duplicates")
			}
			if tt.talos {
				assert.Zero(t, processMounts)
			} else {
				assert.Equal(t, 1, processMounts, "both features must share one passwd mount")
			}
			if tt.security {
				assert.Positive(t, securityMounts)
			}
		})
	}
}
