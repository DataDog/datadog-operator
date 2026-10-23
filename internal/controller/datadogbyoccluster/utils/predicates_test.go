// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestHorizontalPodAutoscalerSpecChangedUpdate(t *testing.T) {
	newHPA := func(labels map[string]string, maxReplicas, currentReplicas int32) *autoscalingv2.HorizontalPodAutoscaler {
		return &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "hpa", Labels: labels},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MaxReplicas: maxReplicas},
			Status:     autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: currentReplicas},
		}
	}

	tests := []struct {
		name      string
		oldObject client.Object
		newObject client.Object
		want      bool
	}{
		{
			name:      "spec change",
			oldObject: newHPA(nil, 3, 1),
			newObject: newHPA(nil, 5, 1),
			want:      true,
		},
		{
			name:      "status-only change",
			oldObject: newHPA(nil, 3, 1),
			newObject: newHPA(nil, 3, 2),
			want:      false,
		},
		{
			name:      "label-only change",
			oldObject: newHPA(nil, 3, 1),
			newObject: newHPA(map[string]string{"foo": "bar"}, 3, 1),
			want:      false,
		},
		{
			name:      "not a HorizontalPodAutoscaler",
			oldObject: &metav1.PartialObjectMetadata{},
			newObject: &metav1.PartialObjectMetadata{},
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HorizontalPodAutoscalerSpecChanged.Update(event.UpdateEvent{ObjectOld: tt.oldObject, ObjectNew: tt.newObject})
			if got != tt.want {
				t.Errorf("Update() = %v, want %v", got, tt.want)
			}
		})
	}
}
