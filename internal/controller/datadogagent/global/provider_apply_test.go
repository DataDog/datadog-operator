// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// autopilotTestTemplate builds a node-agent pod template carrying the
// default-builder volumes/mounts/env that GKE Autopilot mutates.
func autopilotTestTemplate() v1.PodTemplateSpec {
	authEnv := []v1.EnvVar{
		{Name: common.DDAuthTokenFilePath, Value: "/etc/datadog-agent/auth/token"},
		{Name: common.DDClusterAgentEnabled, Value: "true"},
	}
	return v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{Name: common.AuthVolumeName},
				{Name: common.CriSocketVolumeName},
				{Name: common.DogstatsdSocketVolumeName},
				{Name: common.ProcdirVolumeName},
				{Name: common.CgroupsVolumeName},
				{
					Name: common.RunPathVolumeName,
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{Path: "/var/lib/datadog-agent/logs"},
					},
				},
			},
			InitContainers: []v1.Container{
				{Name: autopilotInitVolumeName, Args: []string{"cp", "-vn", "/etc/datadog-agent", "/opt"}},
				{
					Name:         string(apicommon.SeccompSetupContainerName),
					VolumeMounts: []v1.VolumeMount{{Name: common.SeccompSecurityVolumeName, ReadOnly: false}},
				},
				{
					Name:         string(apicommon.InitConfigContainerName),
					Env:          authEnv,
					VolumeMounts: []v1.VolumeMount{{Name: common.AuthVolumeName}, {Name: common.CriSocketVolumeName}},
				},
			},
			Containers: []v1.Container{
				{
					Name: string(apicommon.CoreAgentContainerName),
					Env:  authEnv,
					VolumeMounts: []v1.VolumeMount{
						{Name: common.AuthVolumeName}, {Name: common.CriSocketVolumeName},
						{Name: common.DogstatsdSocketVolumeName}, {Name: common.ProcdirVolumeName}, {Name: common.CgroupsVolumeName},
					},
				},
				{
					Name:    string(apicommon.TraceAgentContainerName),
					Command: []string{"trace-agent", "-config=/etc/datadog-agent/datadog-agent.yaml"},
					VolumeMounts: []v1.VolumeMount{
						{Name: common.AuthVolumeName}, {Name: common.ProcdirVolumeName}, {Name: common.CgroupsVolumeName},
					},
				},
				{
					Name:    string(apicommon.ProcessAgentContainerName),
					Command: []string{"process-agent", "-config=/etc/datadog-agent/datadog-agent.yaml"},
				},
				{
					Name:         string(apicommon.SystemProbeContainerName),
					VolumeMounts: []v1.VolumeMount{{Name: common.ProcdirVolumeName}, {Name: common.CgroupsVolumeName}},
				},
			},
		},
	}
}

func TestApplyGlobalNodeAgentSpec_GKEAutopilot(t *testing.T) {
	manager := fake.NewPodTemplateManagers(t, autopilotTestTemplate())
	ApplyGlobalNodeAgentSpec(manager, kubernetes.GKEAutopilotProvider)
	tmpl := manager.PodTemplateSpec()

	// Forbidden volumes stripped; proc/cgroups/run-path kept.
	vols := map[string]bool{}
	for _, v := range tmpl.Spec.Volumes {
		vols[v.Name] = true
	}
	assert.False(t, vols[common.AuthVolumeName], "auth volume stripped")
	assert.False(t, vols[common.CriSocketVolumeName], "CRI socket volume stripped")
	// dogstatsd socket removal is colocated in the dogstatsd feature, not global.
	assert.True(t, vols[common.DogstatsdSocketVolumeName], "dogstatsd socket not stripped by global")
	assert.True(t, vols[common.ProcdirVolumeName], "proc volume kept")
	assert.True(t, vols[common.CgroupsVolumeName], "cgroups volume kept")

	// Run-path is NOT remapped here — that is the logCollection feature's capability.
	for _, v := range tmpl.Spec.Volumes {
		if v.Name == common.RunPathVolumeName {
			assert.Equal(t, "/var/lib/datadog-agent/logs", v.HostPath.Path, "global must not remap run-path")
		}
	}

	// DD_AUTH_TOKEN_FILE_PATH stripped from every container and init container.
	for _, c := range append(tmpl.Spec.Containers, tmpl.Spec.InitContainers...) {
		for _, e := range c.Env {
			assert.NotEqual(t, common.DDAuthTokenFilePath, e.Name, "DD_AUTH_TOKEN_FILE_PATH stripped from %s", c.Name)
		}
	}

	// proc/cgroups mounts removed from trace-agent only.
	mounts := containerMounts(tmpl)
	assert.NotContains(t, mounts[string(apicommon.TraceAgentContainerName)], common.ProcdirVolumeName, "trace-agent proc mount removed")
	assert.NotContains(t, mounts[string(apicommon.TraceAgentContainerName)], common.CgroupsVolumeName, "trace-agent cgroups mount removed")
	assert.Contains(t, mounts[string(apicommon.SystemProbeContainerName)], common.ProcdirVolumeName, "system-probe proc mount kept")
	assert.Contains(t, mounts[string(apicommon.CoreAgentContainerName)], common.ProcdirVolumeName, "core-agent proc mount kept")

	// Auth/CRI mounts gone everywhere (stripped with their volumes).
	for name, ms := range mounts {
		assert.NotContains(t, ms, common.AuthVolumeName, "auth mount gone from %s", name)
		assert.NotContains(t, ms, common.CriSocketVolumeName, "CRI mount gone from %s", name)
	}

	// Pod label set.
	assert.Equal(t, "false", tmpl.Labels["admission.datadoghq.com/enabled"])

	// init-volume args rewritten; seccomp mount made read-only.
	for _, c := range tmpl.Spec.InitContainers {
		switch c.Name {
		case autopilotInitVolumeName:
			assert.Equal(t, []string{"cp -r /etc/datadog-agent /opt"}, c.Args)
		case string(apicommon.SeccompSetupContainerName):
			for _, m := range c.VolumeMounts {
				if m.Name == common.SeccompSecurityVolumeName {
					assert.True(t, m.ReadOnly, "seccomp mount read-only")
				}
			}
		}
	}

	// trace/process commands point at the in-pod config.
	for _, c := range tmpl.Spec.Containers {
		switch c.Name {
		case string(apicommon.TraceAgentContainerName):
			assert.Equal(t, []string{"trace-agent", "-config=/etc/datadog-agent/datadog.yaml"}, c.Command)
		case string(apicommon.ProcessAgentContainerName):
			assert.Equal(t, []string{"process-agent", "-config=/etc/datadog-agent/datadog.yaml"}, c.Command)
		}
	}

	// Autopilot env vars added to all containers.
	all := manager.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]
	assert.Equal(t, "true", findEnv(all, ddKubeletUseAPIServer).Value)
	assert.Equal(t, `["gcp"]`, findEnv(all, ddCloudProviderMetadata).Value)
}

func TestApplyGlobalNodeAgentSpec_NoProviderIsNoop(t *testing.T) {
	manager := fake.NewPodTemplateManagers(t, autopilotTestTemplate())
	ApplyGlobalNodeAgentSpec(manager, "")
	tmpl := manager.PodTemplateSpec()

	assert.Len(t, tmpl.Spec.Volumes, 6, "no volumes stripped for empty provider")
	assert.Empty(t, tmpl.Labels["admission.datadoghq.com/enabled"], "no autopilot label for empty provider")
	assert.Empty(t, manager.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers], "no env vars added for empty provider")
}

func containerMounts(tmpl *v1.PodTemplateSpec) map[string][]string {
	out := map[string][]string{}
	for _, c := range append(tmpl.Spec.Containers, tmpl.Spec.InitContainers...) {
		for _, m := range c.VolumeMounts {
			out[c.Name] = append(out[c.Name], m.Name)
		}
	}
	return out
}

func findEnv(envs []*v1.EnvVar, name string) *v1.EnvVar {
	for _, e := range envs {
		if e.Name == name {
			return e
		}
	}
	return &v1.EnvVar{}
}
