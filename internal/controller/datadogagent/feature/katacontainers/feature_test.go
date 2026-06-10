// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package katacontainers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func Test_kataContainersFeature(t *testing.T) {
	expectedVcSbsVolume := corev1.Volume{
		Name: kataVcSbsVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: kataVcSbsHostPath,
			},
		},
	}
	expectedVcSbsMount := corev1.VolumeMount{
		Name:      kataVcSbsVolumeName,
		MountPath: kataVcSbsMountPath,
		ReadOnly:  true,
	}
	expectedKataRunVolume := corev1.Volume{
		Name: kataRunVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: kataRunHostPath,
			},
		},
	}
	expectedKataRunMount := corev1.VolumeMount{
		Name:      kataRunVolumeName,
		MountPath: kataRunMountPath,
		ReadOnly:  true,
	}

	tests := test.FeatureTestSuite{
		{
			Name: "kata containers disabled (default)",
			DDA: testutils.NewDatadogAgentBuilder().
				BuildWithDefaults(),
			WantConfigure: false,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					assert.Empty(t, mgr.VolumeMgr.Volumes, "no volumes should be added when kata containers is disabled")
					assert.Empty(t, mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName], "no volume mounts should be added when kata containers is disabled")
				},
			),
		},
		{
			Name: "kata containers enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKataContainersEnabled(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)

					assert.Contains(t, mgr.VolumeMgr.Volumes, &expectedVcSbsVolume, "/run/vc/sbs volume should be added")
					assert.Contains(t, mgr.VolumeMgr.Volumes, &expectedKataRunVolume, "/run/kata volume should be added")

					coreAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, coreAgentMounts, &expectedVcSbsMount, "/run/vc/sbs mount should be added to core agent")
					assert.Contains(t, coreAgentMounts, &expectedKataRunMount, "/run/kata mount should be added to core agent")
				},
			),
		},
		{
			Name: "kata containers explicitly disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKataContainersEnabled(false).
				BuildWithDefaults(),
			WantConfigure: false,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					assert.Empty(t, mgr.VolumeMgr.Volumes, "no volumes should be added when kata containers is explicitly disabled")
					assert.Empty(t, mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName], "no volume mounts should be added when kata containers is explicitly disabled")
				},
			),
		},
	}

	tests.Run(t, buildFeature)
}
