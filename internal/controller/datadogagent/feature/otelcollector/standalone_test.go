// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelcollector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/composition"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func envByName(envs []*corev1.EnvVar) map[string]*corev1.EnvVar {
	byName := make(map[string]*corev1.EnvVar, len(envs))
	for _, env := range envs {
		byName[env.Name] = env
	}
	return byName
}

func standaloneOtelAgentTest(wantConverterFeatures string) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithCreateFunc(
		func(t testing.TB) (feature.PodTemplateManagers, string) {
			return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: string(apicommon.OtelAgent), Command: []string{"otel-agent"}}}},
			}), kubernetes.DefaultProvider
		},
	).WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			env := envByName(mgr.EnvVarMgr.EnvVarsByC[apicommon.OtelAgent])

			for name, value := range map[string]string{
				DDOtelCollectorCoreConfigEnabled:       "true",
				DDOtelCollectorConverterFeatures:       wantConverterFeatures,
				DDCmdPort:                              "0",
				common.DDAgentIpcPort:                  "0",
				common.DDAgentIpcConfigRefreshInterval: "0",
				DDRemoteConfigurationEnabled:           "false",
				DDEnableMetadataCollection:             "false",
				DDInventoriesEnabled:                   "false",
				DDProcessAgentEnabled:                  "false",
			} {
				if assert.Contains(t, env, name) {
					assert.Equal(t, value, env[name].Value, "env %s", name)
				}
			}
			for _, name := range []string{constants.DDHostName, K8sNodeName} {
				if assert.Contains(t, env, name) {
					assert.Equal(t, common.FieldPathSpecNodeName, env[name].ValueFrom.FieldRef.FieldPath, "env %s", name)
				}
			}
			assert.NotContains(t, mgr.EnvVarMgr.EnvVarsByC, apicommon.CoreAgentContainerName)
		},
	)
}

func testStandaloneDependencies(t testing.TB, store store.StoreClient) {
	obj, found := store.Get(kubernetes.ConfigMapKind, "", "-otel-config")
	if assert.True(t, found) {
		assert.Equal(t, defaultconfig.WithoutCoreAgent(defaultconfig.DefaultOtelCollectorConfig), obj.(*corev1.ConfigMap).Data[otelConfigFileName])
	}

	obj, found = store.Get(kubernetes.ClusterRolesKind, "", "--otel-agent-standalone")
	if assert.True(t, found) {
		assert.Equal(t, standaloneRBACPolicyRules, obj.(*rbacv1.ClusterRole).Rules)
	}
}

func Test_otelCollectorFeature_Standalone(t *testing.T) {
	standalone := map[string]string{composition.ModeAnnotationKey: string(composition.ModeStandalone)}
	tests := test.FeatureTestSuite{
		{
			Name: "standalone uses the standalone runtime and default config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(standalone).
				WithOTelCollectorEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testStandaloneDependencies,
			Agent:                standaloneOtelAgentTest("health_check,zpages,pprof,datadog"),
		},
		{
			Name: "standalone with gateway forwards without the datadog converter feature",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(standalone).
				WithOTelCollectorEnabled(true).
				WithOTelAgentGatewayEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         standaloneOtelAgentTest("health_check,zpages,pprof"),
		},
	}
	tests.Run(t, buildOtelCollectorFeature)
}
