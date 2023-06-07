// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"

	// register features
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/oomkill"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/tcpqueuelength"
)

func TestGCPCosOverride(t *testing.T) {
	testDDA := &v2alpha1.DatadogAgent{}

	srcVolume := corev1.Volume{
		Name: apicommon.SrcVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: apicommon.SrcVolumePath,
			},
		},
	}

	srcVolumeMount := corev1.VolumeMount{
		Name:      apicommon.SrcVolumeName,
		MountPath: apicommon.SrcVolumePath,
		ReadOnly:  true,
	}

	tests := []struct {
		name            string
		features        v2alpha1.DatadogFeatures
		validateManager func(t *testing.T, manager feature.PodTemplateManagers)
	}{
		{
			name: "one feature enabled",
			features: v2alpha1.DatadogFeatures{
				OOMKill: &v2alpha1.OOMKillFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
			validateManager: func(t *testing.T, manager feature.PodTemplateManagers) {
				assert.NotContains(t, manager.PodTemplateSpec().Spec.Volumes, srcVolume)
				assert.NotContains(t, manager.PodTemplateSpec().Spec.Containers[0].VolumeMounts, srcVolumeMount)
			},
		},
		{
			name: "both features enabled",
			features: v2alpha1.DatadogFeatures{
				OOMKill: &v2alpha1.OOMKillFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
				TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
			validateManager: func(t *testing.T, manager feature.PodTemplateManagers) {
				assert.NotContains(t, manager.PodTemplateSpec().Spec.Volumes, srcVolume)
				assert.NotContains(t, manager.PodTemplateSpec().Spec.Containers[0].VolumeMounts, srcVolumeMount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			features, rc := buildFeatureListFromDDA(*testDDA, tt.features)
			daemonset := agent.NewDefaultAgentDaemonset(testDDA, rc.Agent.Containers)
			manager := feature.NewPodTemplateManagers(&daemonset.Spec.Template)

			for _, feat := range features {
				errFeat := feat.ManageNodeAgent(manager)
				assert.NoError(t, errFeat)
			}

			gcpCosOverride(manager.PodTemplateSpec(), features)

			tt.validateManager(t, manager)
		})
	}
}

func buildFeatureListFromDDA(dda v2alpha1.DatadogAgent, features v2alpha1.DatadogFeatures) ([]feature.Feature, feature.RequiredComponents) {
	dda.Spec = v2alpha1.DatadogAgentSpec{
		Features: &features,
	}
	v2alpha1.DefaultDatadogAgent(&dda)
	f, rc := feature.BuildFeatures(&dda, nil)
	return f, rc
}
