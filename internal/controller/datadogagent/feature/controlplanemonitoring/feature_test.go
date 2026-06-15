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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

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
		{
			Name: "Control Plane Monitoring enabled with OpenShift provider copies etcd metric client secret",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithAnnotations(map[string]string{kubernetes.ProviderAnnotationKey: "openshift-rhcos"}).
				WithControlPlaneMonitoring(true).
				Build(),
			FeatureOptions: &feature.Options{
				Client: fakeClientWithSecrets(t, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdCertsSecretName,
						Namespace: etcdCertsSourceNamespace,
					},
					Type: corev1.SecretTypeTLS,
					Data: map[string][]byte{
						"tls.crt": []byte("cert"),
						"tls.key": []byte("key"),
					},
				}),
			},
			WantConfigure:        true,
			WantDependenciesFunc: openShiftControlPlaneWantDepsFunc(),
		},
		{
			Name: "Control Plane Monitoring enabled with OpenShift provider keeps existing target secret when source read fails",
			DDA: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithAnnotations(map[string]string{kubernetes.ProviderAnnotationKey: "openshift-rhcos"}).
				WithControlPlaneMonitoring(true).
				Build(),
			FeatureOptions: &feature.Options{
				Client: fakeClientWithSecrets(t, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdCertsSecretName,
						Namespace: resourcesNamespace,
					},
					Type: corev1.SecretTypeTLS,
					Data: map[string][]byte{
						"tls.crt": []byte("existing-cert"),
						"tls.key": []byte("existing-key"),
					},
				}),
			},
			WantConfigure:        true,
			WantDependenciesFunc: openShiftControlPlaneWantExistingSecretDepsFunc(),
		},
	}

	tests.Run(t, buildControlPlaneMonitoringFeature)
}

func fakeClientWithSecrets(t testing.TB, secrets ...*corev1.Secret) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, corev1.AddToScheme(scheme))
	objs := make([]client.Object, 0, len(secrets))
	for _, secret := range secrets {
		objs = append(objs, secret)
	}
	return ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func controlPlaneWantDepsFunc() func(t testing.TB, store store.StoreClient) {
	return func(t testing.TB, store store.StoreClient) {
		// for a default provider, no configmap is created.
		_, found := store.Get(kubernetes.ConfigMapKind, resourcesNamespace, openshiftConfigMapName)
		assert.False(t, found, "Should not have created an OpenShift ConfigMap")

		_, found2 := store.Get(kubernetes.ConfigMapKind, resourcesNamespace, eksConfigMapName)
		assert.False(t, found2, "Should not have created an EKS ConfigMap")
	}
}

func openShiftControlPlaneWantDepsFunc() func(t testing.TB, store store.StoreClient) {
	return func(t testing.TB, store store.StoreClient) {
		obj, found := store.Get(kubernetes.SecretsKind, resourcesNamespace, etcdCertsSecretName)
		assert.True(t, found, "Should have copied the OpenShift etcd metric client Secret")

		secret, ok := obj.(*corev1.Secret)
		assert.True(t, ok)
		assert.Equal(t, corev1.SecretTypeTLS, secret.Type)
		assert.Equal(t, []byte("cert"), secret.Data["tls.crt"])
		assert.Equal(t, []byte("key"), secret.Data["tls.key"])
	}
}

func openShiftControlPlaneWantExistingSecretDepsFunc() func(t testing.TB, store store.StoreClient) {
	return func(t testing.TB, store store.StoreClient) {
		obj, found := store.Get(kubernetes.SecretsKind, resourcesNamespace, etcdCertsSecretName)
		assert.True(t, found, "Should have kept the existing OpenShift etcd metric client Secret")

		secret, ok := obj.(*corev1.Secret)
		assert.True(t, ok)
		assert.Equal(t, corev1.SecretTypeTLS, secret.Type)
		assert.Equal(t, []byte("existing-cert"), secret.Data["tls.crt"])
		assert.Equal(t, []byte("existing-key"), secret.Data["tls.key"])
	}
}

func controlPlaneWantResourcesFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// For default provider, no volumes should be created
			dcaVols := mgr.VolumeMgr.Volumes
			dcaVolMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ClusterAgentContainerName]

			// No volumes should be created for the default provider
			expectedVols := []*corev1.Volume{}

			assert.True(
				t,
				apiutils.IsEqualStruct(dcaVols, expectedVols),
				"DCA Volumes \ndiff = %s", cmp.Diff(dcaVols, expectedVols),
			)

			// No volume mounts should be created for the default provider
			var expectedVolMounts []*corev1.VolumeMount

			assert.True(
				t,
				apiutils.IsEqualStruct(dcaVolMounts, expectedVolMounts),
				"DCA VolumeMounts \ndiff = %s", cmp.Diff(dcaVolMounts, expectedVolMounts),
			)
		})
}
