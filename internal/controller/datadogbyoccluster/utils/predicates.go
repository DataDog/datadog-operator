// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// HorizontalPodAutoscalerSpecChanged passes HorizontalPodAutoscaler updates that change the spec, filtering out
// status-only updates. GenerationChangedPredicate does not work here: before Kubernetes 1.37, the API server does not
// bump the generation of HorizontalPodAutoscalers on spec changes. Kubernetes 1.37 enables it by default through the
// HPAGeneration feature gate, which is GA in 1.38. Once the minimum supported Kubernetes version is 1.38, replace this
// predicate with GenerationChangedPredicate.
var HorizontalPodAutoscalerSpecChanged = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldHPA, oldOK := e.ObjectOld.(*autoscalingv2.HorizontalPodAutoscaler)
		newHPA, newOK := e.ObjectNew.(*autoscalingv2.HorizontalPodAutoscaler)
		return !oldOK || !newOK || !apiequality.Semantic.DeepEqual(oldHPA.Spec, newHPA.Spec)
	},
}
