// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func Test_eventCollectionFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "Event Collection not enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithEventCollectionKubernetesEvents(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Event Collection enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithEventCollectionKubernetesEvents(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(eventCollectionClusterAgentWantFunc),
		},
		{
			Name: "Unbundle event enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithEventCollectionKubernetesEvents(true).
				WithEventCollectionUnbundleEvents(true, []v2alpha1.EventTypes{
					{
						Kind:    "Pod",
						Reasons: []string{"Killing", "Created", "Deleted"},
					},
					{
						Kind:    "Node",
						Reasons: []string{"NodeNotReady"},
					},
				}).
				Build(),
			WantConfigure:        true,
			ClusterAgent:         test.NewDefaultComponentTest().WithWantFunc(unbundledEventsClusterAgentWantFunc),
			WantDependenciesFunc: unbundledEventsDependencies,
		},
	}

	tests.Run(t, buildEventCollectionFeature)
}

func eventCollectionClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]

	want := []*corev1.EnvVar{
		{
			Name:  apicommon.DDCollectKubernetesEvents,
			Value: "true",
		},
		{
			Name:  apicommon.DDLeaderElection,
			Value: "true",
		},
		{
			Name:  apicommon.DDLeaderLeaseName,
			Value: "ddaDCA-leader-election",
		},
		{
			Name:  apicommon.DDClusterAgentTokenName,
			Value: "ddaDCA-token",
		},
	}
	assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
}

func unbundledEventsDependencies(t testing.TB, store dependencies.StoreClient) {
	// validate clusterRole policy rules
	crObj, found := store.Get(kubernetes.ConfigMapKind, "", "ddaDCA-kube-apiserver-config")
	if !found {
		t.Error("Should have created check ConfigMap")
	} else {
		cr := crObj.(*corev1.ConfigMap)
		expectedCM := map[string]string{
			"kubernetes_apiserver.yaml": `init_config: null
instances:
- collected_event_types:
  - kind: Pod
    reasons:
    - Killing
    - Created
    - Deleted
  - kind: Node
    reasons:
    - NodeNotReady
  unbundle_events: true
`,
		}

		assert.True(
			t,
			apiutils.IsEqualStruct(cr.Data, expectedCM),
			"ConfigMap \ndiff = %s", cmp.Diff(cr.Data, expectedCM),
		)
	}
}

func unbundledEventsClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	expectedVolumes := []*corev1.Volume{
		{
			Name: "kubernetes-apiserver-check-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "ddaDCA-kube-apiserver-config",
					},
				},
			},
		},
	}
	assert.True(t, apiutils.IsEqualStruct(mgr.VolumeMgr.Volumes, expectedVolumes), "DCA volumes \ndiff = %s", cmp.Diff(mgr.VolumeMgr.Volumes, expectedVolumes))

	volumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.ClusterAgentContainerName]
	expectedVolumeMounts := []*corev1.VolumeMount{
		{
			Name:      "kubernetes-apiserver-check-config",
			MountPath: "/etc/datadog-agent/conf.d/kubernetes_apiserver.d",
			ReadOnly:  true,
		},
	}
	assert.True(t, apiutils.IsEqualStruct(volumeMounts, expectedVolumeMounts), "DCA volume mounts \ndiff = %s", cmp.Diff(volumeMounts, expectedVolumeMounts))
}
