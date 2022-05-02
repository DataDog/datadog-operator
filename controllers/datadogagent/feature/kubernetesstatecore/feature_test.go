// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	mergerfake "github.com/DataDog/datadog-operator/controllers/datadogagent/merger/fake"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func createEmptyFakeManager(t testing.TB) feature.PodTemplateManagers {
	mgr := fake.NewPodTemplateManagers(t)
	return mgr
}

func Test_ksmFeature_Configure(t *testing.T) {
	ddav1KSMDisable := v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			Features: v1alpha1.DatadogFeatures{
				KubeStateMetricsCore: &v1alpha1.KubeStateMetricsCore{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}

	ddav1KSMEnable := ddav1KSMDisable.DeepCopy()
	{
		ddav1KSMEnable.Spec.Features.KubeStateMetricsCore.Enabled = apiutils.NewBoolPointer(true)
	}

	ddav2KSMDisable := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2KSMEnable := ddav2KSMDisable.DeepCopy()
	{
		ddav2KSMEnable.Spec.Features.KubeStateMetricsCore.Enabled = apiutils.NewBoolPointer(true)
	}

	ksmClusterAgentWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]

		want := []*corev1.EnvVar{
			{
				Name:  apicommon.DDKubeStateMetricsCoreEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDKubeStateMetricsCoreConfigMap,
				Value: "-kube-state-metrics-core-config",
			},
		}
		assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
	}

	ksmAgentNodeWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[common.AgentContainerName]

		want := []*corev1.EnvVar{
			{
				Name:  apicommon.DDIgnoreAutoConf,
				Value: "kubernetes_state",
			},
		}
		assert.True(t, apiutils.IsEqualStruct(agentEnvVars, want), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, want))
	}

	tests := test.FeatureTestSuite{
		//////////////////////////
		// v1Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v1alpha1 ksm-core not enable",
			DDAv1:         ddav1KSMDisable.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 ksm-core not enable",
			DDAv1:         ddav1KSMEnable,
			WantConfigure: true,
			ClusterAgent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   ksmClusterAgentWantFunc,
			},
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   ksmAgentNodeWantFunc,
			},
		},
		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v2alpha1 ksm-core not enable",
			DDAv2:         ddav2KSMDisable.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "v2alpha1 ksm-core not enable",
			DDAv2:         ddav2KSMEnable,
			WantConfigure: true,
			ClusterAgent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   ksmClusterAgentWantFunc,
			},
			Agent: &test.ComponentTest{
				CreateFunc: createEmptyFakeManager,
				WantFunc:   ksmAgentNodeWantFunc,
			},
		},
	}

	tests.Run(t, buildKSMfeature)
}
