// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	common "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func Test_eventCollectionFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "Event Collection not enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithEventCollectionKubernetesEvents(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Event Collection enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithName("ddaDCA").
				WithEventCollectionKubernetesEvents(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(eventCollectionClusterAgentWantFunc),
		},
		{
			Name: "Unbundle event enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
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
	dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

	want := []*corev1.EnvVar{
		{
			Name:  DDCollectKubernetesEvents,
			Value: "true",
		},
		{
			Name:  common.DDLeaderElection,
			Value: "true",
		},
		{
			Name:  DDLeaderLeaseName,
			Value: "ddaDCA-leader-election",
		},
		{
			Name:  common.DDClusterAgentTokenName,
			Value: "ddaDCA-token",
		},
	}
	assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
}

func unbundledEventsDependencies(t testing.TB, store store.StoreClient) {
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

	volumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ClusterAgentContainerName]
	expectedVolumeMounts := []*corev1.VolumeMount{
		{
			Name:      "kubernetes-apiserver-check-config",
			MountPath: "/etc/datadog-agent/conf.d/kubernetes_apiserver.d",
			ReadOnly:  true,
		},
	}
	assert.True(t, apiutils.IsEqualStruct(volumeMounts, expectedVolumeMounts), "DCA volume mounts \ndiff = %s", cmp.Diff(volumeMounts, expectedVolumeMounts))
}
