// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"fmt"
	"testing"

	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/testutils"

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
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "ksm-core not enabled with single agent container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(false).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "ksm-core enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled with single agent container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentSingleAgentWantFunc),
		},
		{
			Name: "ksm-core enabled, custom config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMCustomConf(customData).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(true),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled, custom config with single agent container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMCustomConf(customData).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(true),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentSingleAgentWantFunc),
		},
		{
			Name: "ksm-core enabled, cluster agent with image >= 7.72.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithClusterAgentImage("gcr.io/datadoghq/agent:7.72.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled, cluster agent with image < 7.72.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithClusterAgentImage("gcr.io/datadoghq/agent:7.71.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled, cluster checks runner with image >= 7.72.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithClusterChecks(true, true).
				WithClusterChecksRunnerImage("gcr.io/datadoghq/agent:7.72.0").
				Build(),
			WantConfigure:       true,
			Agent:               test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
			ClusterChecksRunner: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
		},
		{
			Name: "ksm-core enabled, cluster checks runner with image < 7.72.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithClusterChecks(true, true).
				WithClusterChecksRunnerImage("gcr.io/datadoghq/agent:7.71.0").
				Build(),
			WantConfigure:       true,
			Agent:               test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
			ClusterAgent:        test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
			ClusterChecksRunner: test.NewDefaultComponentTest().WithWantFunc(func(t testing.TB, mgrInterface feature.PodTemplateManagers) {}),
		},
		{
			Name: "ksm-core enabled, podCollectionMode=default (explicit) preserves existing behavior",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMPodCollectionMode(v2alpha1.KSMPodCollectionModeDefault).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled, podCollectionMode=node_kubelet, default conf",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMPodCollectionMode(v2alpha1.KSMPodCollectionModeNodeKubelet).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false, withPodCollectionOnNode()),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFuncWithPodsOnNode),
		},
		{
			Name: "ksm-core enabled, podCollectionMode=node_kubelet, single agent container",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMPodCollectionMode(v2alpha1.KSMPodCollectionModeNodeKubelet).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false, withPodCollectionOnNode()),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentSingleAgentWantFuncWithPodsOnNode),
		},
		{
			Name: "ksm-core enabled, podCollectionMode=node_kubelet + user-supplied conf still mounts node-side check",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMPodCollectionMode(v2alpha1.KSMPodCollectionModeNodeKubelet).
				WithKSMCustomConf(customData).
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(true),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFuncWithPodsOnNode),
		},
		{
			Name: "ksm-core enabled, podCollectionMode=node_kubelet but cluster-agent image < 7.60 -> fall back",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMPodCollectionMode(v2alpha1.KSMPodCollectionModeNodeKubelet).
				WithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.59.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
		{
			Name: "ksm-core enabled, podCollectionMode=node_kubelet but node-agent image < 7.60 -> fall back",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKSMEnabled(true).
				WithKSMPodCollectionMode(v2alpha1.KSMPodCollectionModeNodeKubelet).
				WithNodeAgentImage("gcr.io/datadoghq/agent:7.59.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  ksmClusterAgentWantFunc(false),
			Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
		},
	}

	tests.Run(t, buildKSMFeature)
}

type ksmClusterAgentWantConfig struct {
	podCollectionOnNode bool
}

type ksmClusterAgentOption func(*ksmClusterAgentWantConfig)

func withPodCollectionOnNode() ksmClusterAgentOption {
	return func(c *ksmClusterAgentWantConfig) { c.podCollectionOnNode = true }
}

func ksmClusterAgentWantFunc(hasCustomConfig bool, opts ...ksmClusterAgentOption) *test.ComponentTest {
	cfg := ksmClusterAgentWantConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]

			want := []*corev1.EnvVar{
				{
					Name:  DDKubeStateMetricsCoreEnabled,
					Value: "true",
				},
				{
					Name:  DDKubeStateMetricsCoreConfigMap,
					Value: "-kube-state-metrics-core-config",
				},
			}
			assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))

			if hasCustomConfig {
				customConfig := v2alpha1.CustomConfig{
					ConfigData: ptr.To(customData),
				}
				hash, err := comparison.GenerateMD5ForSpec(&customConfig)
				assert.NoError(t, err)
				wantAnnotations := map[string]string{
					fmt.Sprintf(constants.MD5ChecksumAnnotationKey, feature.KubernetesStateCoreIDType): hash,
				}
				annotations := mgr.AnnotationMgr.Annotations
				assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))
			} else {
				// Verify default config annotation - CRDs and APIServices collected, no custom resource metrics
				defaultConfigData := map[string]any{
					"collect_crds":           true,
					"collect_apiservices":    true,
					"collect_cr_metrics":     nil,
					"pod_collection_on_node": cfg.podCollectionOnNode,
				}
				hash, err := comparison.GenerateMD5ForSpec(defaultConfigData)
				assert.NoError(t, err)
				wantAnnotations := map[string]string{
					fmt.Sprintf(constants.MD5ChecksumAnnotationKey, feature.KubernetesStateCoreIDType): hash,
				}
				annotations := mgr.AnnotationMgr.Annotations
				assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Default config annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))
			}
		},
	)
}

func ksmAgentNodeWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	ksmAgentWantFunc(t, mgrInterface, apicommon.CoreAgentContainerName, false)
}

func ksmAgentSingleAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	ksmAgentWantFunc(t, mgrInterface, apicommon.UnprivilegedSingleAgentContainerName, false)
}

func ksmAgentNodeWantFuncWithPodsOnNode(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	ksmAgentWantFunc(t, mgrInterface, apicommon.CoreAgentContainerName, true)
}

func ksmAgentSingleAgentWantFuncWithPodsOnNode(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	ksmAgentWantFunc(t, mgrInterface, apicommon.UnprivilegedSingleAgentContainerName, true)
}

func ksmAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers, agentContainerName apicommon.AgentContainerName, wantPodsOnNodeMount bool) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]

	want := []*corev1.EnvVar{
		{
			Name:  DDIgnoreAutoConf,
			Value: "kubernetes_state",
		},
	}
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, want), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, want))

	// When PodCollectionMode=node_kubelet is active the operator must mount the
	// node-side pods-only ConfigMap into this container. When it isn't active
	// the container must NOT have that volume/mount.
	gotMount := findVolumeMount(mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName], ksmCorePodsOnNodeVolumeName)
	gotVolume := findVolume(mgr.VolumeMgr.Volumes, ksmCorePodsOnNodeVolumeName)
	if wantPodsOnNodeMount {
		assert.NotNil(t, gotMount, "expected node-side KSM pods-on-node volume mount on container %s", agentContainerName)
		assert.NotNil(t, gotVolume, "expected node-side KSM pods-on-node volume in pod spec")
		if gotMount != nil {
			assert.Equal(t, "/etc/datadog-agent/conf.d/kubernetes_state_core.d", gotMount.MountPath)
			assert.True(t, gotMount.ReadOnly, "node-side KSM mount should be read-only")
		}
	} else {
		assert.Nil(t, gotMount, "node-side KSM volume mount should NOT be present when podCollectionMode is unset/default")
		assert.Nil(t, gotVolume, "node-side KSM volume should NOT be present when podCollectionMode is unset/default")
	}
}

func findVolumeMount(mounts []*corev1.VolumeMount, name string) *corev1.VolumeMount {
	for _, m := range mounts {
		if m.Name == name {
			return m
		}
	}
	return nil
}

func findVolume(volumes []*corev1.Volume, name string) *corev1.Volume {
	for _, v := range volumes {
		if v.Name == name {
			return v
		}
	}
	return nil
}
