// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func Test_sbomFeature_NodeAgentProviderCapabilities(t *testing.T) {
	newPodTemplate := func() *corev1.PodTemplateSpec {
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

	volumeNames := func(tmpl *corev1.PodTemplateSpec) []string {
		names := make([]string, 0, len(tmpl.Spec.Volumes))
		for _, v := range tmpl.Spec.Volumes {
			names = append(names, v.Name)
		}
		return names
	}

	t.Run("talos adds the tracefs volume when enrichment is enabled", func(t *testing.T) {
		f := &sbomFeature{enabled: true, enrichmentUsageEnabled: true}
		tmpl := newPodTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		providercaps.ApplyProviderCapabilities(mgr, kubernetes.TalosProvider, f.NodeAgentProviderCapabilities())

		assert.Contains(t, volumeNames(tmpl), common.TracefsVolumeName)
		// debugfs must remain: tracefs is an addition, not a replacement.
		assert.Contains(t, volumeNames(tmpl), common.DebugfsVolumeName)
	})

	t.Run("talos adds no tracefs volume without enrichment", func(t *testing.T) {
		f := &sbomFeature{enabled: true}
		tmpl := newPodTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		providercaps.ApplyProviderCapabilities(mgr, kubernetes.TalosProvider, f.NodeAgentProviderCapabilities())

		assert.NotContains(t, volumeNames(tmpl), common.TracefsVolumeName)
	})

	t.Run("default provider adds no tracefs volume", func(t *testing.T) {
		f := &sbomFeature{enabled: true, enrichmentUsageEnabled: true}
		tmpl := newPodTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		providercaps.ApplyProviderCapabilities(mgr, kubernetes.DefaultProvider, f.NodeAgentProviderCapabilities())

		assert.NotContains(t, volumeNames(tmpl), common.TracefsVolumeName)
		assert.Contains(t, volumeNames(tmpl), common.DebugfsVolumeName)
	})
}
