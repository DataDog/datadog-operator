// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kueue

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

const (
	resourcesName      = "foo"
	resourcesNamespace = "bar"
)

var (
	defaultConfigMapName = fmt.Sprintf("%s-%s", resourcesName, defaultKueueConf)
	dcaRBACName          = fmt.Sprintf("%s-%s-%s-dca", resourcesNamespace, resourcesName, kueueRBACPrefix)
	nodeRBACName         = fmt.Sprintf("%s-%s-%s-node", resourcesNamespace, resourcesName, kueueRBACPrefix)
)

func Test_kueueFeature_Configure(t *testing.T) {
	customConfigData := "init_config:\ninstances:\n  - openmetrics_endpoint: https://kueue:8443/metrics\n"

	tests := test.FeatureTestSuite{
		{
			Name: "Kueue disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKueueEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Kueue enabled with Cluster Agent too old",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKueueEnabled(true).
				WithClusterChecksEnabled(true).
				WithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.81.0").
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Kueue enabled with node Agent too old",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKueueEnabled(true).
				WithClusterChecksEnabled(true).
				WithNodeAgentImage("gcr.io/datadoghq/agent:7.81.0").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: metadataOnlyDepsWantFunc,
			ClusterAgent:         clusterAgentWantFunc(nil),
		},
		{
			Name: "Kueue enabled with prerelease images",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKueueEnabled(true).
				WithClusterChecksEnabled(true).
				WithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.82.0-rc.1").
				WithNodeAgentImage("gcr.io/datadoghq/agent:7.82.0-rc.1").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: checkDepsWantFunc(defaultConfigMapName, kueueCheckConfig(defaultMetricsServiceName, defaultMetricsServiceNamespace, true)),
			ClusterAgent:         clusterAgentWantFunc(basicVolume(defaultConfigMapName)),
		},
		{
			Name: "Kueue enabled without cluster checks",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKueueEnabled(true).
				WithClusterChecksEnabled(false).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: metadataOnlyDepsWantFunc,
			ClusterAgent:         clusterAgentWantFunc(nil),
		},
		{
			Name: "Kueue enabled",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKueueEnabled(true).
				WithClusterChecksEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: checkDepsWantFunc(defaultConfigMapName, kueueCheckConfig(defaultMetricsServiceName, defaultMetricsServiceNamespace, true)),
			ClusterAgent:         clusterAgentWantFunc(basicVolume(defaultConfigMapName)),
		},
		{
			Name: "Kueue enabled with cluster checks runners",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKueueEnabled(true).
				WithClusterChecks(true, true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: checkDepsWantFunc(defaultConfigMapName, kueueCheckConfig(defaultMetricsServiceName, defaultMetricsServiceNamespace, true)),
			ClusterAgent:         clusterAgentWantFunc(basicVolume(defaultConfigMapName)),
		},
		{
			Name: "Kueue enabled with metrics service override and workload events disabled",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKueueEnabled(true).
				WithKueueCollectWorkloadEvents(false).
				WithKueueMetricsService("my-kueue-metrics", "my-kueue").
				WithClusterChecksEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: checkDepsWantFunc(defaultConfigMapName, kueueCheckConfig("my-kueue-metrics", "my-kueue", false)),
			ClusterAgent:         clusterAgentWantFunc(basicVolume(defaultConfigMapName)),
		},
		{
			Name: "Kueue enabled with custom config data",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKueueEnabled(true).
				WithKueueCustomConf(&v2alpha1.CustomConfig{ConfigData: &customConfigData}).
				WithClusterChecksEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: checkDepsWantFunc(defaultConfigMapName, customConfigData),
			ClusterAgent:         clusterAgentWantFunc(basicVolume(defaultConfigMapName)),
		},
		{
			Name: "Kueue enabled with custom ConfigMap",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithKueueEnabled(true).
				WithKueueCustomConf(&v2alpha1.CustomConfig{ConfigMap: &v2alpha1.ConfigMapConfig{Name: "user-kueue"}}).
				WithClusterChecksEnabled(true).
				Build(),
			WantConfigure: true,
			WantDependenciesFunc: func(t testing.TB, s store.StoreClient) {
				assertClusterRole(t, s, dcaRBACName, kueueClusterAgentRBACPolicyRules)
				assertClusterRole(t, s, nodeRBACName, kueueNodeAgentRBACPolicyRules)
				_, found := s.Get(kubernetes.ConfigMapKind, resourcesNamespace, defaultConfigMapName)
				assert.False(t, found, "operator ConfigMap should not be created")
				_, found = s.Get(kubernetes.ConfigMapKind, resourcesNamespace, "user-kueue")
				assert.False(t, found, "user ConfigMap should not be managed")
			},
			ClusterAgent: clusterAgentWantFunc(basicVolume("user-kueue")),
		},
	}

	tests.Run(t, buildKueueFeature)
}

func basicVolume(cmName string) *corev1.Volume {
	return &corev1.Volume{
		Name: kueueConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
			},
		},
	}
}

func metadataOnlyDepsWantFunc(t testing.TB, s store.StoreClient) {
	assertClusterRole(t, s, dcaRBACName, kueueClusterAgentRBACPolicyRules)
	_, found := s.Get(kubernetes.ClusterRolesKind, "", nodeRBACName)
	assert.False(t, found, "node Agent ClusterRole should not be created")
	_, found = s.Get(kubernetes.ConfigMapKind, resourcesNamespace, defaultConfigMapName)
	assert.False(t, found, "check ConfigMap should not be created")
}

func checkDepsWantFunc(cmName, wantConfig string) func(testing.TB, store.StoreClient) {
	return func(t testing.TB, s store.StoreClient) {
		obj, found := s.Get(kubernetes.ConfigMapKind, resourcesNamespace, cmName)
		require.True(t, found, "should have created the check ConfigMap")
		assert.Equal(t, map[string]string{kueueConfFileName: wantConfig}, obj.(*corev1.ConfigMap).Data)

		assertClusterRole(t, s, dcaRBACName, kueueClusterAgentRBACPolicyRules)
		assertClusterRole(t, s, nodeRBACName, kueueNodeAgentRBACPolicyRules)
	}
}

func assertClusterRole(t testing.TB, s store.StoreClient, name string, wantRules []rbacv1.PolicyRule) {
	crObj, found := s.Get(kubernetes.ClusterRolesKind, "", name)
	require.True(t, found, "should have created ClusterRole %s", name)
	assert.Equal(t, wantRules, crObj.(*rbacv1.ClusterRole).Rules)

	crbObj, found := s.Get(kubernetes.ClusterRoleBindingKind, "", name)
	require.True(t, found, "should have created ClusterRoleBinding %s", name)
	assert.Equal(t, rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: name}, crbObj.(*rbacv1.ClusterRoleBinding).RoleRef)
}

// clusterAgentWantFunc checks the Cluster Agent env var, and the check volume when wantVolume is set.
func clusterAgentWantFunc(wantVolume *corev1.Volume) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			assert.Equal(t,
				[]*corev1.EnvVar{{Name: DDClusterAgentKueueEnabled, Value: "true"}},
				mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName],
			)

			mounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ClusterAgentContainerName]
			if wantVolume == nil {
				assert.Empty(t, mgr.VolumeMgr.Volumes)
				assert.Empty(t, mounts)
				return
			}

			assert.Equal(t, []*corev1.Volume{wantVolume}, mgr.VolumeMgr.Volumes)
			assert.Equal(t, []*corev1.VolumeMount{{
				Name:      kueueConfigVolumeName,
				MountPath: "/etc/datadog-agent/conf.d/kueue.d",
				ReadOnly:  true,
			}}, mounts)
		})
}
