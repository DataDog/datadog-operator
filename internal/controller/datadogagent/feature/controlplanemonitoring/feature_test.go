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
			Name: "Control Plane Monitoring enabled with default provider",
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
		// Validate OpenShift configMap - the feature always creates configmaps even if they're not used
		obj, found := store.Get(kubernetes.ConfigMapKind, resourcesNamespace, openshiftConfigMapName)

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
				assert.Contains(t, etcdConfig, "/etc/etcd-certs/tls.crt")
				assert.Contains(t, etcdConfig, "/etc/etcd-certs/tls.key")
			}
			if kubeControllerManagerConfig, exists := cm.Data["kube_controller_manager.yaml"]; exists {
				assert.Contains(t, kubeControllerManagerConfig, "kube-controller-manager")
				assert.Contains(t, kubeControllerManagerConfig, "bearer_token_auth: true")
			}
			if kubeSchedulerConfig, exists := cm.Data["kube_scheduler.yaml"]; exists {
				assert.Contains(t, kubeSchedulerConfig, "openshift-kube-scheduler")
				assert.Contains(t, kubeSchedulerConfig, "bearer_token_auth: true")
			}
		}

		obj2, found2 := store.Get(kubernetes.ConfigMapKind, resourcesNamespace, eksConfigMapName)
		if !found2 {
			t.Error("Should have created an EKS ConfigMap")
		} else {
			cm2 := obj2.(*corev1.ConfigMap)

			// Check for expected EKS configuration files
			expectedKeys := []string{
				"kube_apiserver_metrics.yaml",
				"kube_controller_manager.yaml",
				"kube_scheduler.yaml",
			}

			for _, key := range expectedKeys {
				if _, exists := cm2.Data[key]; !exists {
					t.Errorf("Expected EKS ConfigMap to contain key: %s", key)
				}
			}

			// Validate specific EKS configurations
			if kubeAPIServerConfig, exists := cm2.Data["kube_apiserver_metrics.yaml"]; exists {
				assert.Contains(t, kubeAPIServerConfig, "kubernetes")
				assert.Contains(t, kubeAPIServerConfig, "default")
				assert.Contains(t, kubeAPIServerConfig, "bearer_token_auth: true")
			}
			if kubeControllerManagerConfig, exists := cm2.Data["kube_controller_manager.yaml"]; exists {
				assert.Contains(t, kubeControllerManagerConfig, "default")
				assert.Contains(t, kubeControllerManagerConfig, "bearer_token_auth: true")
			}
			if kubeSchedulerConfig, exists := cm2.Data["kube_scheduler.yaml"]; exists {
				assert.Contains(t, kubeSchedulerConfig, "default")
				assert.Contains(t, kubeSchedulerConfig, "tls_ca_cert")
				assert.Contains(t, kubeSchedulerConfig, "extra_headers")
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
			}

			dcaVolMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ClusterAgentContainerName]

			assert.True(
				t,
				apiutils.IsEqualStruct(dcaVolMounts, expectedVolMounts),
				"DCA VolumeMounts \ndiff = %s", cmp.Diff(dcaVolMounts, expectedVolMounts),
			)
		})
}
