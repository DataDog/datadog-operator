// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnqueueIfOwnedByDatadogAgentInternal(t *testing.T) {
	unmanaged := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		kubernetes.AppKubernetesPartOfLabelKey: "default-profile--ddai",
	}}}
	assert.Empty(t, enqueueIfOwnedByDatadogAgentInternal(context.Background(), unmanaged))

	managed := unmanaged.DeepCopy()
	managed.Labels[kubernetes.AppKubernetesManageByLabelKey] = "datadog-operator"
	requests := enqueueIfOwnedByDatadogAgentInternal(context.Background(), managed)
	require.Len(t, requests, 1)
	owner := object.PartOfLabelValue{Value: "default-profile--ddai"}
	assert.Equal(t, owner.NamespacedName(), requests[0].NamespacedName)
}
