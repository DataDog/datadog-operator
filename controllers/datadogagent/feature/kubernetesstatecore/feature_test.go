// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"fmt"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	mergerfake "github.com/DataDog/datadog-operator/controllers/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

const (
	customData = `cluster_check: true
init_config:
instances:
    collectors:
    - pods`
)

func Test_ksmFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "ksm-core not enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithKSMEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "ksm-core not enabled with single agent container",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithKSMEnabled(false).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "ksm-core enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled with single agent container",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentSingleAgentWantFunc),
		},
		{
			Name: "ksm-core enabled, custom config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMCustomConf(customData).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(true),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled, custom config with single agent container",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMCustomConf(customData).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(true),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentSingleAgentWantFunc),
		},
	}

	tests.Run(t, buildKSMFeature)
}

func ksmClusterAgentWantFunc(hasCustomConfig bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
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

			if hasCustomConfig {
				cm, err := configmap.BuildConfigMapConfigData("", apiutils.NewStringPointer(customData), "", ksmCoreCheckName)
				assert.NoError(t, err)
				hash, err := comparison.GenerateMD5ForSpec(cm.Data)
				assert.NoError(t, err)
				wantAnnotations := map[string]string{
					fmt.Sprintf(apicommon.MD5ChecksumAnnotationKey, feature.KubernetesStateCoreIDType): hash,
				}
				annotations := mgr.AnnotationMgr.Annotations
				assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))
				t.Logf("want annotations: %#v\n", wantAnnotations)
				t.Logf("get annotations: %#v\n", wantAnnotations)
			}
		},
	)
}

func ksmAgentNodeWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	ksmAgentWantFunc(t, mgrInterface, apicommonv1.CoreAgentContainerName)
}

func ksmAgentSingleAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	ksmAgentWantFunc(t, mgrInterface, apicommonv1.UnprivilegedSingleAgentContainerName)
}

func ksmAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers, agentContainerName apicommonv1.AgentContainerName) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]

	want := []*corev1.EnvVar{
		{
			Name:  apicommon.DDIgnoreAutoConf,
			Value: "kubernetes_state",
		},
	}
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, want), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, want))
}
