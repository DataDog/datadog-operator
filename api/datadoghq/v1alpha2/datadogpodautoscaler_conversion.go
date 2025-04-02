// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func NewDatadogPodAutoscalerFromV1Alpha1(in *v1alpha1.DatadogPodAutoscaler) *DatadogPodAutoscaler {
	if in == nil {
		return nil
	}

	// As many types are shared, we'll assign the deep copied value to the new object
	in = in.DeepCopy()
	out := &DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: in.ObjectMeta,
		Spec:       ConvertDatadogPodAutoscalerSpecFromV1Alpha1(in.Spec),
		Status:     in.Status,
	}
	return out
}

func ConvertDatadogPodAutoscalerSpecFromV1Alpha1(in v1alpha1.DatadogPodAutoscalerSpec) DatadogPodAutoscalerSpec {
	// Same fields
	out := DatadogPodAutoscalerSpec{
		TargetRef:     in.TargetRef,
		Owner:         in.Owner,
		RemoteVersion: in.RemoteVersion,
		Objectives:    in.Targets,
		Constraints:   in.Constraints,
	}

	// Other fields
	if in.Policy != nil {
		out.ApplyPolicy = &DatadogPodAutoscalerApplyPolicy{
			Update:    in.Policy.Update,
			ScaleUp:   in.Policy.Upscale,
			ScaleDown: in.Policy.Downscale,
		}

		switch in.Policy.ApplyMode {
		case v1alpha1.DatadogPodAutoscalerAllApplyMode:
			out.ApplyPolicy.Mode = DatadogPodAutoscalerApplyModeApply
		case v1alpha1.DatadogPodAutoscalerNoneApplyMode, v1alpha1.DatadogPodAutoscalerManualApplyMode:
			out.ApplyPolicy.Mode = DatadogPodAutoscalerApplyModePreview
		}
	}
	return out
}

func NewDatadogPodAutoscalerToV1Alpha1(in *DatadogPodAutoscaler) *v1alpha1.DatadogPodAutoscaler {
	if in == nil {
		return nil
	}

	// As many types are shared, we'll assign the deep copied value to the new object
	in = in.DeepCopy()
	out := &v1alpha1.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha1",
		},
		ObjectMeta: in.ObjectMeta,
		Spec:       ConvertDatadogPodAutoscalerSpecToV1Alpha1(in.Spec),
		Status:     in.Status,
	}
	return out
}

func ConvertDatadogPodAutoscalerSpecToV1Alpha1(in DatadogPodAutoscalerSpec) v1alpha1.DatadogPodAutoscalerSpec {
	out := v1alpha1.DatadogPodAutoscalerSpec{
		TargetRef:     in.TargetRef,
		Owner:         in.Owner,
		RemoteVersion: in.RemoteVersion,
		Targets:       in.Objectives,
		Constraints:   in.Constraints,
	}

	if in.ApplyPolicy != nil {
		out.Policy = &v1alpha1.DatadogPodAutoscalerPolicy{
			Update:    in.ApplyPolicy.Update,
			Upscale:   in.ApplyPolicy.ScaleUp,
			Downscale: in.ApplyPolicy.ScaleDown,
		}

		switch in.ApplyPolicy.Mode {
		case DatadogPodAutoscalerApplyModeApply:
			out.Policy.ApplyMode = v1alpha1.DatadogPodAutoscalerAllApplyMode
		case DatadogPodAutoscalerApplyModePreview:
			out.Policy.ApplyMode = v1alpha1.DatadogPodAutoscalerManualApplyMode
		}
	}

	return out
}
