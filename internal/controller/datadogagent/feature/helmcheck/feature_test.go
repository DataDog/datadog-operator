// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

const resourcesName = "foo"
const resourcesNamespace = "bar"

var valuesAsTags = map[string]string{"foo": "bar", "zip": "zap"}

func Test_helmCheckFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "Helm check disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithHelmCheckEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Helm check enabled",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithHelmCheckEnabled(true).
				WithHelmCheckCollectEvents(true).
				WithHelmCheckValuesAsTags(valuesAsTags).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: helmCheckWantDepsFunc(false, true, valuesAsTags, "dca"),
			ClusterAgent:         helmCheckWantResourcesFunc(false, true),
		},
		{
			Name: "Helm check enabled and runs on cluster checks runner",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithHelmCheckEnabled(true).
				WithHelmCheckCollectEvents(true).
				WithHelmCheckValuesAsTags(valuesAsTags).
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: helmCheckWantDepsFunc(true, true, valuesAsTags, "ccr"),
			ClusterAgent:         helmCheckWantResourcesFunc(true, true),
		},
	}

	tests.Run(t, buildHelmCheckFeature)
}

func helmCheckWantDepsFunc(ccr bool, collectEvents bool, valuesAsTags map[string]string, rbacSuffix string) func(t testing.TB, store store.StoreClient) {
	return func(t testing.TB, store store.StoreClient) {
		// validate configMap
		configMapName := fmt.Sprintf("%s-%s", resourcesName, v2alpha1.DefaultHelmCheckConf)

		obj, found := store.Get(kubernetes.ConfigMapKind, resourcesNamespace, configMapName)

		if !found {
			t.Error("Should have created a ConfigMap")
		} else {
			cm := obj.(*corev1.ConfigMap)

			wantData := helmCheckConfig(ccr, collectEvents, valuesAsTags)
			wantCm, err := configmap.BuildConfigMapConfigData(resourcesNamespace, &wantData, configMapName, helmCheckConfFileName)
			require.NoError(t, err)

			assert.True(
				t,
				apiutils.IsEqualStruct(cm.Data, wantCm.Data),
				"ConfigMap data \ndiff = %s", cmp.Diff(cm.Data, wantCm.Data),
			)
		}

		// RBAC
		rbacName := fmt.Sprintf("%s-%s-%s-%s", resourcesNamespace, resourcesName, helmCheckRBACPrefix, rbacSuffix)

		// validate clusterRole policy rules
		crObj, found := store.Get(kubernetes.ClusterRolesKind, "", rbacName)

		if !found {
			t.Error("Should have created ClusterRole")
		} else {
			cr := crObj.(*rbacv1.ClusterRole)
			assert.True(
				t,
				apiutils.IsEqualStruct(cr.Rules, helmCheckRBACPolicyRules),
				"ClusterRole Policy Rules \ndiff = %s", cmp.Diff(cr.Rules, helmCheckRBACPolicyRules),
			)
		}

		// validate clusterRoleBinding roleRef name
		expectedRoleRef := rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     rbacName,
		}

		crbObj, found := store.Get(kubernetes.ClusterRoleBindingKind, "", rbacName)

		if !found {
			t.Error("Should have created ClusterRoleBinding")
		} else {
			crb := crbObj.(*rbacv1.ClusterRoleBinding)
			assert.True(
				t,
				apiutils.IsEqualStruct(crb.RoleRef, expectedRoleRef),
				"ClusterRoleBinding Role Ref \ndiff = %s", cmp.Diff(crb.RoleRef, expectedRoleRef),
			)
		}
	}
}

func helmCheckWantResourcesFunc(ccr bool, collectEvents bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// validate volumes
			expectedVols := []*corev1.Volume{
				{
					Name: v2alpha1.DefaultHelmCheckConf,
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

			// validate volumeMounts
			expectedVolMounts := []*corev1.VolumeMount{
				{
					Name:      v2alpha1.DefaultHelmCheckConf,
					MountPath: "/etc/datadog-agent/conf.d/helm.d",
					ReadOnly:  true,
				},
			}

			dcaVolMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ClusterAgentContainerName]

			assert.True(
				t,
				apiutils.IsEqualStruct(dcaVolMounts, expectedVolMounts),
				"DCA VolumeMounts \ndiff = %s", cmp.Diff(dcaVolMounts, expectedVolMounts),
			)

			// Validate configMap annotations
			config := map[string]string{
				"helm.yaml": fmt.Sprintf(`---
cluster_check: %s
init_config:
instances:
  - collect_events: %s
    helm_values_as_tags:
      foo: bar
      zip: zap
`, strconv.FormatBool(ccr), strconv.FormatBool(collectEvents)),
			}

			hash, err := comparison.GenerateMD5ForSpec(config)
			assert.NoError(t, err)

			wantAnnotations := map[string]string{
				fmt.Sprintf(constants.MD5ChecksumAnnotationKey, feature.HelmCheckIDType): hash,
			}

			annotations := mgr.AnnotationMgr.Annotations
			assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))
		})
}
