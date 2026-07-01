// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func TestDatadogAgentInternalEventFilterAllowsAnnotationUpdates(t *testing.T) {
	oldDDAI := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "datadog",
			Annotations: map[string]string{
				constants.DDAIRenderedByOperatorVersionAnnotationKey: "1.27.1",
			},
		},
	}
	newDDAI := oldDDAI.DeepCopy()
	newDDAI.Annotations[constants.DDAIRenderedByOperatorVersionAnnotationKey] = "1.28.0-rc.3"

	assert.True(t, datadogAgentInternalEventFilter().Update(event.UpdateEvent{
		ObjectOld: oldDDAI,
		ObjectNew: newDDAI,
	}))
}
