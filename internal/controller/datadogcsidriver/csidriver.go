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
	driverName := csiDriverName
	attachRequired := false
	podInfoOnMount := true

	return &storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: driverName,
			Labels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(instance).String(),
			},
		},
		Spec: storagev1.CSIDriverSpec{
			AttachRequired: &attachRequired,
			PodInfoOnMount: &podInfoOnMount,
			VolumeLifecycleModes: []storagev1.VolumeLifecycleMode{
				storagev1.VolumeLifecyclePersistent,
				storagev1.VolumeLifecycleEphemeral,
			},
		},
	}
}
