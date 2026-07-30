// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
)

// TestPropagateComponentChecksums exercises propagateComponentChecksums against a real
// feature.PodTemplateManagers backed by a real corev1.PodTemplateSpec (not the fake package's
// disconnected annotation map), to prove that a checksum registered via Store.RegisterComponentChecksum
// actually ends up on the pod template's annotations.
func TestPropagateComponentChecksums(t *testing.T) {
	t.Run("registered checksum is written onto the pod template", func(t *testing.T) {
		s := store.NewStore(&metav1.ObjectMeta{Name: "dda", Namespace: "ns"}, &store.StoreOptions{})
		s.RegisterComponentChecksum(datadoghqv2alpha1.ClusterAgentComponentName, "kubernetes-state-core", "hash1")

		podTmpl := &corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}}
		podManagers := feature.NewPodTemplateManagers(podTmpl)

		propagateComponentChecksums(podManagers, s.GetComponentChecksums(datadoghqv2alpha1.ClusterAgentComponentName))

		assert.Equal(t, map[string]string{
			"checksum.datadoghq.com/clusterAgent.kubernetes-state-core": "hash1",
		}, podTmpl.Annotations)
	})

	t.Run("a user-set override annotation wins over the registered checksum", func(t *testing.T) {
		s := store.NewStore(&metav1.ObjectMeta{Name: "dda", Namespace: "ns"}, &store.StoreOptions{})
		s.RegisterComponentChecksum(datadoghqv2alpha1.ClusterAgentComponentName, "kubernetes-state-core", "hash1")

		key := "checksum.datadoghq.com/clusterAgent.kubernetes-state-core"
		podTmpl := &corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{key: "user-override-value"},
			},
		}
		podManagers := feature.NewPodTemplateManagers(podTmpl)

		propagateComponentChecksums(podManagers, s.GetComponentChecksums(datadoghqv2alpha1.ClusterAgentComponentName))

		assert.Equal(t, "user-override-value", podTmpl.Annotations[key])
	})

	t.Run("no checksums registered leaves the pod template annotations untouched", func(t *testing.T) {
		s := store.NewStore(&metav1.ObjectMeta{Name: "dda", Namespace: "ns"}, &store.StoreOptions{})

		podTmpl := &corev1.PodTemplateSpec{}
		podManagers := feature.NewPodTemplateManagers(podTmpl)

		propagateComponentChecksums(podManagers, s.GetComponentChecksums(datadoghqv2alpha1.ClusterAgentComponentName))

		assert.Nil(t, podTmpl.Annotations)
	})
}
