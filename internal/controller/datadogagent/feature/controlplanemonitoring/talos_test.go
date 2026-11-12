// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplanemonitoring

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// Toleration that lets the node agent run on control-plane nodes.
func controlPlaneToleration() corev1.Toleration {
	return corev1.Toleration{
		Key:      controlPlaneTaintKey,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}
}

func talosSpec(tolerations ...corev1.Toleration) *v2alpha1.DatadogAgentSpec {
	spec := &v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
				Enabled: new(true),
			},
		},
	}
	if len(tolerations) > 0 {
		spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {Tolerations: tolerations},
		}
	}
	return spec
}

func talosOwner() metav1.Object {
	return &metav1.ObjectMeta{
		Name:        resourcesName,
		Namespace:   resourcesNamespace,
		Annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.TalosProvider},
	}
}

func newTalosFeature() *controlPlaneMonitoringFeature {
	return &controlPlaneMonitoringFeature{logger: logr.Discard()}
}

func nodeAgentTemplate() *corev1.PodTemplateSpec {
	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: string(apicommon.CoreAgentContainerName)},
				{Name: string(apicommon.SystemProbeContainerName)},
			},
		},
	}
}

func volumeNames(tmpl *corev1.PodTemplateSpec) []string {
	names := make([]string, 0, len(tmpl.Spec.Volumes))
	for _, v := range tmpl.Spec.Volumes {
		names = append(names, v.Name)
	}
	return names
}

func mountPaths(tmpl *corev1.PodTemplateSpec, container apicommon.AgentContainerName) map[string]corev1.VolumeMount {
	out := map[string]corev1.VolumeMount{}
	for _, c := range tmpl.Spec.Containers {
		if c.Name != string(container) {
			continue
		}
		for _, m := range c.VolumeMounts {
			out[m.Name] = m
		}
	}
	return out
}

// Pins the parts of the Talos check configs that silently break collection.
func Test_talosControlPlaneMonitoringConfigMap(t *testing.T) {
	f := newTalosFeature()
	f.owner = talosOwner()

	cm, err := f.buildControlPlaneMonitoringConfigMap(kubernetes.TalosProvider, talosConfigMapName)
	require.NoError(t, err)
	require.NotNil(t, cm)

	assert.Equal(t, talosConfigMapName, cm.Name)
	assert.Equal(t, resourcesNamespace, cm.Namespace)

	keys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{
		"kube_apiserver_metrics.yaml",
		"kube_controller_manager.yaml",
		"kube_scheduler.yaml",
		"etcd.yaml",
	}, keys)

	t.Run("tier 1 checks are cluster checks off the kubernetes endpoint", func(t *testing.T) {
		for _, key := range []string{"kube_apiserver_metrics.yaml", "kube_controller_manager.yaml", "kube_scheduler.yaml"} {
			body := cm.Data[key]
			assert.Contains(t, body, "cluster_check: true", key)
			// This endpoint resolves to a control-plane address.
			assert.Contains(t, body, `name: "kubernetes"`, key)
			assert.Contains(t, body, `namespace: "default"`, key)
			// Without this the Cluster Agent dispatches none of the three checks.
			assert.Contains(t, body, `resolve: "ip"`, key)
		}
	})

	t.Run("scheduler and controller-manager use literal secure ports", func(t *testing.T) {
		// %%port%% would be the apiserver's 6443.
		assert.Contains(t, cm.Data["kube_scheduler.yaml"], "https://%%host%%:10259/metrics")
		assert.Contains(t, cm.Data["kube_controller_manager.yaml"], "https://%%host%%:10257/metrics")
		for _, key := range []string{"kube_scheduler.yaml", "kube_controller_manager.yaml"} {
			assert.Contains(t, cm.Data[key], "bearer_token_auth: true", key)
			assert.Contains(t, cm.Data[key], "ssl_verify: false", key)
		}
	})

	t.Run("apiserver takes the port from the endpoint", func(t *testing.T) {
		assert.Contains(t, cm.Data["kube_apiserver_metrics.yaml"], "https://%%host%%:%%port%%/metrics")
	})

	t.Run("etcd is node-local, not a cluster check", func(t *testing.T) {
		etcd := cm.Data["etcd.yaml"]
		// cluster_check would move it to the Cluster Agent, which has no certs.
		assert.NotContains(t, etcd, "cluster_check")
		assert.NotContains(t, etcd, "advanced_ad_identifiers")
		assert.Contains(t, etcd, "ad_identifiers:")
		// etcd has no pod of its own; this only pins the check to a node.
		assert.Contains(t, etcd, "kube-scheduler")
		assert.Contains(t, etcd, "https://%%host%%:2379/metrics")
		for _, f := range []string{"ca.crt", "server.crt", "server.key"} {
			assert.Contains(t, etcd, talosEtcdCertsMountPath+"/"+f)
		}
	})
}

func Test_nodeAgentToleratesControlPlane(t *testing.T) {
	tests := []struct {
		name        string
		tolerations []corev1.Toleration
		want        bool
	}{
		{
			name: "no override at all",
			want: false,
		},
		{
			name:        "exists toleration on the control-plane taint",
			tolerations: []corev1.Toleration{controlPlaneToleration()},
			want:        true,
		},
		{
			// Not just Operator == Exists.
			name: "equal toleration on the control-plane taint",
			tolerations: []corev1.Toleration{{
				Key:      controlPlaneTaintKey,
				Operator: corev1.TolerationOpEqual,
				Effect:   corev1.TaintEffectNoSchedule,
			}},
			want: true,
		},
		{
			// An empty key with Exists tolerates every taint.
			name:        "empty-key exists tolerates everything",
			tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			want:        true,
		},
		{
			// No effect means it applies to all effects of that key.
			name:        "control-plane key with no effect",
			tolerations: []corev1.Toleration{{Key: controlPlaneTaintKey, Operator: corev1.TolerationOpExists}},
			want:        true,
		},
		{
			name: "toleration for an unrelated taint",
			tolerations: []corev1.Toleration{{
				Key:      "example.com/other",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			}},
			want: false,
		},
		{
			name: "right key but wrong effect",
			tolerations: []corev1.Toleration{{
				Key:      controlPlaneTaintKey,
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoExecute,
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nodeAgentToleratesControlPlane(logr.Discard(), talosSpec(tt.tolerations...))
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_talosConfigure_etcdTierGating(t *testing.T) {
	t.Run("without toleration the node agent is not required", func(t *testing.T) {
		f := newTalosFeature()
		reqComp := f.Configure(talosOwner(), talosSpec(), nil)

		assert.True(t, f.enabled)
		assert.False(t, f.talosEtcdEnabled)
		// Tier 1 runs on the cluster agent, so only that is required.
		require.NotNil(t, reqComp.ClusterAgent.IsRequired)
		assert.True(t, *reqComp.ClusterAgent.IsRequired)
		assert.Nil(t, reqComp.Agent.IsRequired)
	})

	t.Run("with toleration the node agent is required for etcd", func(t *testing.T) {
		f := newTalosFeature()
		reqComp := f.Configure(talosOwner(), talosSpec(controlPlaneToleration()), nil)

		assert.True(t, f.talosEtcdEnabled)
		require.NotNil(t, reqComp.Agent.IsRequired)
		assert.True(t, *reqComp.Agent.IsRequired)
		assert.Contains(t, reqComp.Agent.Containers, apicommon.CoreAgentContainerName)
	})

	t.Run("non-talos providers are unaffected", func(t *testing.T) {
		for _, provider := range []string{kubernetes.DefaultProvider, kubernetes.EKSCloudProvider} {
			f := newTalosFeature()
			owner := &metav1.ObjectMeta{
				Name:        resourcesName,
				Namespace:   resourcesNamespace,
				Annotations: map[string]string{kubernetes.ProviderAnnotationKey: provider},
			}
			reqComp := f.Configure(owner, talosSpec(controlPlaneToleration()), nil)
			assert.False(t, f.talosEtcdEnabled, provider)
			assert.Nil(t, reqComp.Agent.IsRequired, provider)
		}
	})
}

func Test_talosManageNodeAgent(t *testing.T) {
	t.Run("etcd config and host certs mount when tolerated", func(t *testing.T) {
		f := newTalosFeature()
		f.Configure(talosOwner(), talosSpec(controlPlaneToleration()), nil)

		tmpl := nodeAgentTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		assert.Contains(t, volumeNames(tmpl), etcdVolumeName)
		assert.Contains(t, volumeNames(tmpl), talosEtcdCertsVolumeName)

		mounts := mountPaths(tmpl, apicommon.CoreAgentContainerName)

		etcdConf, ok := mounts[etcdVolumeName]
		require.True(t, ok, "etcd config must mount on the core agent")
		assert.Equal(t, etcdMountPath, etcdConf.MountPath)
		assert.True(t, etcdConf.ReadOnly)

		certs, ok := mounts[talosEtcdCertsVolumeName]
		require.True(t, ok, "etcd certs must mount on the core agent")
		assert.Equal(t, talosEtcdCertsMountPath, certs.MountPath)
		assert.True(t, certs.ReadOnly, "etcd certs must be read-only")

		// Certs come from the host, not a Secret.
		for _, v := range tmpl.Spec.Volumes {
			if v.Name == talosEtcdCertsVolumeName {
				require.NotNil(t, v.HostPath)
				assert.Equal(t, talosEtcdCertsHostPath, v.HostPath.Path)
			}
		}
	})

	// Masks the Agent image's bundled autoconf so the cluster checks are not
	// also collected node-locally on a control-plane node.
	t.Run("bundled tier-1 autoconf is masked on the node agent", func(t *testing.T) {
		for _, tolerations := range [][]corev1.Toleration{nil, {controlPlaneToleration()}} {
			f := newTalosFeature()
			f.Configure(talosOwner(), talosSpec(tolerations...), nil)

			tmpl := nodeAgentTemplate()
			mgr := feature.NewPodTemplateManagers(tmpl)
			require.NoError(t, f.ManageNodeAgent(mgr))

			mounts := mountPaths(tmpl, apicommon.CoreAgentContainerName)
			for name, path := range map[string]string{
				disableKubeApiserverMetricsAutoconfVolumeName:  kubeApiserverMetricsMountPath,
				disableKubeControllerManagerAutoconfVolumeName: kubeControllerManagerMountPath,
				disableKubeSchedulerAutoconfVolumeName:         kubeSchedulerMountPath,
			} {
				m, ok := mounts[name]
				require.True(t, ok, "%s must be masked", name)
				assert.Equal(t, path, m.MountPath)
			}
		}
	})

	t.Run("nothing mounts without the toleration", func(t *testing.T) {
		f := newTalosFeature()
		f.Configure(talosOwner(), talosSpec(), nil)

		tmpl := nodeAgentTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		assert.NotContains(t, volumeNames(tmpl), etcdVolumeName)
		assert.NotContains(t, volumeNames(tmpl), talosEtcdCertsVolumeName)
	})

	// Control plane monitoring is on by default, so adding a toleration here
	// would reschedule the node agent for every Talos user.
	t.Run("operator never adds a toleration itself", func(t *testing.T) {
		for _, spec := range []*v2alpha1.DatadogAgentSpec{talosSpec(), talosSpec(controlPlaneToleration())} {
			f := newTalosFeature()
			f.Configure(talosOwner(), spec, nil)

			tmpl := nodeAgentTemplate()
			before := len(tmpl.Spec.Tolerations)
			mgr := feature.NewPodTemplateManagers(tmpl)
			require.NoError(t, f.ManageNodeAgent(mgr))

			assert.Len(t, tmpl.Spec.Tolerations, before,
				"ManageNodeAgent must not change tolerations")
		}
	})
}

func Test_talosManageClusterAgent_tier1Only(t *testing.T) {
	f := newTalosFeature()
	f.Configure(talosOwner(), talosSpec(controlPlaneToleration()), nil)

	tmpl := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: string(apicommon.ClusterAgentContainerName)}},
		},
	}
	mgr := feature.NewPodTemplateManagers(tmpl)
	require.NoError(t, f.ManageClusterAgent(mgr))

	mounts := mountPaths(tmpl, apicommon.ClusterAgentContainerName)
	assert.Contains(t, mounts, kubeApiserverMetricsVolumeName)
	assert.Contains(t, mounts, kubeControllerManagerVolumeName)
	assert.Contains(t, mounts, kubeSchedulerVolumeName)

	// Not mounted here even when the etcd tier is on.
	assert.NotContains(t, mounts, etcdVolumeName)
	assert.NotContains(t, mounts, talosEtcdCertsVolumeName)
	for _, name := range volumeNames(tmpl) {
		assert.False(t, strings.Contains(name, "etcd"),
			"unexpected etcd volume %q on the cluster agent", name)
	}
}
