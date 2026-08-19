// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
)

func xidKernelLogsDDA(gpuEnabled, collectXidKernelLogs bool, logCollection *v2alpha1.LogCollectionFeatureConfig) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "dda-gpu", Namespace: "system"},
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				GPU: &v2alpha1.GPUFeatureConfig{
					Enabled:              ptr.To(gpuEnabled),
					CollectXidKernelLogs: ptr.To(collectXidKernelLogs),
				},
				LogCollection: logCollection,
			},
			Global: &v2alpha1.GlobalConfig{Kubelet: &v2alpha1.KubeletConfig{}},
		},
	}
}

// Kernel log collection is inert without the logs Agent, so it must stay off unless log
// collection is explicitly enabled.
func Test_XidKernelLogs_GatedOnLogCollection(t *testing.T) {
	tests := []struct {
		name          string
		gpuEnabled    bool
		collectXid    bool
		logCollection *v2alpha1.LogCollectionFeatureConfig
		want          bool
	}{
		{
			name:          "enabled with log collection on",
			gpuEnabled:    true,
			collectXid:    true,
			logCollection: &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(true)},
			want:          true,
		},
		{
			name:          "no-op when log collection is off",
			gpuEnabled:    true,
			collectXid:    true,
			logCollection: &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(false)},
			want:          false,
		},
		{
			name:          "no-op when log collection is unset",
			gpuEnabled:    true,
			collectXid:    true,
			logCollection: nil,
			want:          false,
		},
		{
			name:          "off when not requested",
			gpuEnabled:    true,
			collectXid:    false,
			logCollection: &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(true)},
			want:          false,
		},
		{
			name:          "off when gpu monitoring itself is disabled",
			gpuEnabled:    false,
			collectXid:    true,
			logCollection: &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(true)},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := xidKernelLogsDDA(tt.gpuEnabled, tt.collectXid, tt.logCollection)
			f := &gpuFeature{}
			f.Configure(dda, &dda.Spec, nil)

			assert.Equal(t, tt.want, f.collectXidKernelLogs)
		})
	}
}

func Test_XidKernelLogs_ManageNodeAgent(t *testing.T) {
	dda := xidKernelLogsDDA(true, true, &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(true)})
	f := &gpuFeature{}
	f.Configure(dda, &dda.Spec, nil)

	mgr := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	assert.NoError(t, f.ManageNodeAgent(mgr))

	coreAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]

	wantConfdMount := &corev1.VolumeMount{
		Name:      xidKernelLogsVolumeName,
		MountPath: fmt.Sprintf("%s%s/%s", common.ConfigVolumePath, common.ConfdVolumePath, xidKernelLogsConfigName),
		ReadOnly:  true,
	}
	wantJournalMount := &corev1.VolumeMount{
		Name:      journalVolumeName,
		MountPath: journalMountPath,
		ReadOnly:  true,
	}
	wantMachineIDMount := &corev1.VolumeMount{
		Name:      machineIDVolumeName,
		MountPath: machineIDMountPath,
		ReadOnly:  true,
	}

	assert.Contains(t, coreAgentMounts, wantConfdMount)
	assert.Contains(t, coreAgentMounts, wantJournalMount)
	assert.Contains(t, coreAgentMounts, wantMachineIDMount)
}

// The host journal must not be mounted when the feature is not requested.
func Test_XidKernelLogs_NoMountsWhenDisabled(t *testing.T) {
	dda := xidKernelLogsDDA(true, false, &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(true)})
	f := &gpuFeature{}
	f.Configure(dda, &dda.Spec, nil)

	mgr := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	assert.NoError(t, f.ManageNodeAgent(mgr))

	for _, m := range mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName] {
		assert.NotEqual(t, journalVolumeName, m.Name)
		assert.NotEqual(t, xidKernelLogsVolumeName, m.Name)
		assert.NotEqual(t, machineIDVolumeName, m.Name)
	}
}

// The Agent's journald launcher keys tailers on `journald:<config_id>`. If this value ever
// diverges from the one shipped by the Helm chart's journald config, the same kernel messages
// are tailed twice and ingested twice.
func Test_XidKernelLogs_ConfigContract(t *testing.T) {
	cm := buildXidKernelLogsConfigMap("system", "dda-gpu")

	assert.Equal(t, "dda-gpu-gpu-xid-kernel-logs", cm.Name)
	assert.Equal(t, "system", cm.Namespace)

	conf, ok := cm.Data["conf.yaml"]
	assert.True(t, ok, "conf.yaml key must exist for the journald.d mount")

	assert.Contains(t, conf, "config_id: kernel")
	assert.Contains(t, conf, "_TRANSPORT=kernel")
	assert.Contains(t, conf, "source: kernel")
	assert.Contains(t, conf, "service: kernel")
	// Guard against the config silently becoming a second catch-all journal tailer.
	assert.True(t, strings.Contains(conf, "include_matches"), "must filter to the kernel transport")
}

func Test_XidKernelLogs_ManageDependencies(t *testing.T) {
	dda := xidKernelLogsDDA(true, false, &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(true)})
	f := &gpuFeature{}
	f.Configure(dda, &dda.Spec, nil)

	// Disabled: no ConfigMap dependency is created.
	assert.NoError(t, f.ManageDependencies(feature.NewResourceManagers(nil)))
}
