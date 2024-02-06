// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const resourcesName = "foo"
const resourcesNamespace = "bar"

func Test_helmCheckFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "Helm check disabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithHelmCheckEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Helm check enabled",
			DDAv2: v2alpha1test.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithHelmCheckEnabled(true).
				WithHelmCheckCollectEvents(true).
				WithHelmCheckValuesAsTags(map[string]string{"foo": "bar", "zip": "zap"}).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: helmCheckWantDepsFunc(),
			ClusterAgent:         helmCheckWantResourcesFunc(),
		},
		{
			Name: "Helm check enabled and runs on cluster checks runner",
			DDAv2: v2alpha1test.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithHelmCheckEnabled(true).
				WithHelmCheckCollectEvents(true).
				WithHelmCheckValuesAsTags(map[string]string{"foo": "bar", "zip": "zap"}).
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: helmCheckWantDepsFunc(),
			ClusterAgent:         helmCheckWantResourcesFunc(),
		},
	}

	tests.Run(t, buildHelmCheckFeature)
}

func helmCheckWantDepsFunc() func(t testing.TB, store dependencies.StoreClient) {
	return func(t testing.TB, store dependencies.StoreClient) {
		configMapName := fmt.Sprintf("%s-%s", resourcesName, apicommon.DefaultHelmCheckConf)

		if _, found := store.Get(kubernetes.ConfigMapKind, resourcesNamespace, configMapName); !found {
			t.Error("Should have created a ConfigMap")
		}
	}
}

func helmCheckWantResourcesFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			expectedVols := []*corev1.Volume{
				{
					Name: apicommon.DefaultHelmCheckConf,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "foo-helm-check-config",
							},
						},
					},
				},
			}

			dcaVols := mgr.VolumeMgr.Volumes

			assert.True(
				t,
				apiutils.IsEqualStruct(dcaVols, expectedVols),
				"DCA VolumeMounts \ndiff = %s", cmp.Diff(dcaVols, expectedVols),
			)

			expectedVolMounts := []*corev1.VolumeMount{
				{
					Name:      apicommon.DefaultHelmCheckConf,
					MountPath: "/etc/datadog-agent/conf.d/helm.d",
					ReadOnly:  true,
				},
			}

			dcaVolMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.ClusterAgentContainerName]

			assert.True(
				t,
				apiutils.IsEqualStruct(dcaVolMounts, expectedVolMounts),
				"DCA VolumeMounts \ndiff = %s", cmp.Diff(dcaVolMounts, expectedVolMounts),
			)
		})
}
