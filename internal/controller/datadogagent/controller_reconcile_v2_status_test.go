// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

func TestUpdateStatusRecordsSuccessfulObservedGeneration(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, datadoghqv2alpha1.AddToScheme(scheme))
	dda := &datadoghqv2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "datadog-agent",
			Namespace:  "datadog",
			Generation: 7,
		},
		Status: datadoghqv2alpha1.DatadogAgentStatus{
			Conditions: []metav1.Condition{{
				Type:               common.DatadogAgentReconcileErrorConditionType,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: 6,
				LastTransitionTime: metav1.Now(),
				Reason:             "DatadogAgent_reconcile_ok",
				Message:            "DatadogAgent reconcile ok",
			}},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&datadoghqv2alpha1.DatadogAgent{}).
		WithObjects(dda).
		Build()
	current := &datadoghqv2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dda), current))
	r := &Reconciler{client: c}

	newStatus := current.Status.DeepCopy()
	_, err := r.updateStatusIfNeededV2(logr.Discard(), current, newStatus, reconcile.Result{}, nil, metav1.Now())
	require.NoError(t, err)

	updated := &datadoghqv2alpha1.DatadogAgent{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dda), updated))
	condition := meta.FindStatusCondition(updated.Status.Conditions, common.DatadogAgentReconcileErrorConditionType)
	require.NotNil(t, condition)
	require.Equal(t, metav1.ConditionFalse, condition.Status)
	require.Equal(t, int64(7), condition.ObservedGeneration)
}
