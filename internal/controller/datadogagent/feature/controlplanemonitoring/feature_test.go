// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplanemonitoring

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

const resourcesName = "foo"
const resourcesNamespace = "bar"

func Test_controlPlaneMonitoringFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "Control Plane Monitoring disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithControlPlaneMonitoring(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Control Plane Monitoring enabled",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithControlPlaneMonitoring(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: controlPlaneWantDepsFunc(),
			ClusterAgent:         controlPlaneWantResourcesFunc(),
		},
	}

	tests.Run(t, buildControlPlaneMonitoringFeature)
}

func controlPlaneWantDepsFunc() func(t testing.TB, store store.StoreClient) {
	return func(t testing.TB, store store.StoreClient) {
		// Validate default configMap - the feature always creates both configmaps
		obj, found := store.Get(kubernetes.ConfigMapKind, resourcesNamespace, defaultConfigMapName)

		if !found {
			t.Error("Should have created a default ConfigMap")
		} else {
			cm := obj.(*corev1.ConfigMap)
			expectedData := map[string]string{
				"foo.yaml": "bar",
			}
			assert.True(
				t,
				apiutils.IsEqualStruct(cm.Data, expectedData),
				"Default ConfigMap data \ndiff = %s", cmp.Diff(cm.Data, expectedData),
			)
		}

		// Validate OpenShift configMap - the feature always creates both configmaps
		obj, found = store.Get(kubernetes.ConfigMapKind, resourcesNamespace, openshiftConfigMapName)

		if !found {
			t.Error("Should have created an OpenShift ConfigMap")
		} else {
			cm := obj.(*corev1.ConfigMap)

			// Check for expected OpenShift configuration files
			expectedKeys := []string{
				"kube_apiserver_metrics.yaml",
				"kube_controller_manager.yaml",
				"kube_scheduler.yaml",
				"etcd.yaml",
			}

			for _, key := range expectedKeys {
				if _, exists := cm.Data[key]; !exists {
					t.Errorf("Expected OpenShift ConfigMap to contain key: %s", key)
				}
			}

			// Validate specific OpenShift configurations
			if kubeAPIServerConfig, exists := cm.Data["kube_apiserver_metrics.yaml"]; exists {
				assert.Contains(t, kubeAPIServerConfig, "kubernetes")
				assert.Contains(t, kubeAPIServerConfig, "default")
				assert.Contains(t, kubeAPIServerConfig, "bearer_token_auth: true")
			}

			if etcdConfig, exists := cm.Data["etcd.yaml"]; exists {
				assert.Contains(t, etcdConfig, "openshift-etcd")
				assert.Contains(t, etcdConfig, "/etc/etcd-certs/etcd-client-ca.crt")
				assert.Contains(t, etcdConfig, "/etc/etcd-certs/etcd-client.crt")
				assert.Contains(t, etcdConfig, "/etc/etcd-certs/etcd-client.key")
			}
		}
	}
}

func controlPlaneWantResourcesFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// Validate volumes
			expectedVols := []*corev1.Volume{
				{
					Name: emptyDirVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: controlPlaneMonitoringVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: defaultConfigMapName,
								// Default provider is used in tests since provider detection is done at runtime
							},
						},
					},
				},
			}

			dcaVols := mgr.VolumeMgr.Volumes

			assert.True(
				t,
				apiutils.IsEqualStruct(dcaVols, expectedVols),
				"DCA Volumes \ndiff = %s", cmp.Diff(dcaVols, expectedVols),
			)

			// Validate volumeMounts
			expectedVolMounts := []*corev1.VolumeMount{
				{
					Name:      emptyDirVolumeName,
					MountPath: controlPlaneMonitoringVolumeMountPath,
					ReadOnly:  false,
				},
				{
					Name:      controlPlaneMonitoringVolumeName,
					MountPath: controlPlaneMonitoringVolumeMountPath,
					ReadOnly:  true,
				},
			}

			dcaVolMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ClusterAgentContainerName]

			assert.True(
				t,
				apiutils.IsEqualStruct(dcaVolMounts, expectedVolMounts),
				"DCA VolumeMounts \ndiff = %s", cmp.Diff(dcaVolMounts, expectedVolMounts),
			)
		})
}
