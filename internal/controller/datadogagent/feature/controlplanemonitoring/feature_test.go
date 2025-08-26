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
		// for a default provider, no configmap is created.
		_, found := store.Get(kubernetes.ConfigMapKind, resourcesNamespace, openshiftConfigMapName)
		assert.False(t, found, "Should not have created an OpenShift ConfigMap")

		_, found2 := store.Get(kubernetes.ConfigMapKind, resourcesNamespace, eksConfigMapName)
		assert.False(t, found2, "Should not have created an EKS ConfigMap")
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
