// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func buildCSIDriverObject(instance *datadoghqv1alpha1.DatadogCSIDriver) *storagev1.CSIDriver {
	labels := map[string]string{
		kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(instance).String(),
	}
	// Merge extraLabels propagated from spec.global.extraLabels on the parent
	// DatadogAgent. Operator-owned keys already present in labels win.
	for k, v := range instance.Spec.ExtraLabels {
		if _, exists := labels[k]; !exists {
			labels[k] = v
		}
	}
	return &storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: csiDriverName,
			Annotations: map[string]string{
				apmEnabledAnnotationKey: getAPMEnabledString(instance),
			},
			Labels: labels,
		},
		Spec: storagev1.CSIDriverSpec{
			VolumeLifecycleModes: []storagev1.VolumeLifecycleMode{
				storagev1.VolumeLifecyclePersistent,
				storagev1.VolumeLifecycleEphemeral,
			},
		},
	}
}
