// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

const statusFieldOwner = "datadogsyntheticsprivatelocation-controller"

// Condition reasons surfaced on the Error condition.
const (
	reasonPrivateLocationDeleted = "PrivateLocationDeleted"
	reasonConfigMissing          = "ConfigMissing"
)

// applyStatusPatch server-side-applies the instance status. Callers run it in
// a defer so the status is persisted even when a reconcile step fails; the
// returned error joins any reconcile error.
func applyStatusPatch(ctx context.Context, kubeClient client.Client, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) error {
	statusApply := datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: datadoghqv1alpha1.GroupVersion.String(),
			Kind:       "DatadogSyntheticsPrivateLocation",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Status: instance.Status,
	}
	if err := kubeClient.Status().Patch(ctx, &statusApply,
		client.Apply, client.FieldOwner(statusFieldOwner), client.ForceOwnership); err != nil {
		return fmt.Errorf("applying status patch: %w", err)
	}
	return nil
}

func setErrorCondition(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time, reason string, err error) {
	ctrutils.UpdateFailureStatusConditions(&instance.Status.Conditions, now, ctrutils.DatadogConditionTypeError, reason, err)
}

func setSuccessConditions(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time) {
	ctrutils.UpdateStatusConditions(&instance.Status.Conditions, now, ctrutils.DatadogConditionTypeError, metav1.ConditionFalse, "", "")
	ctrutils.UpdateStatusConditions(&instance.Status.Conditions, now, ctrutils.DatadogConditionTypeActive, metav1.ConditionTrue, "", "DatadogSyntheticsPrivateLocation ready")
}

// setSyncedConditions records a successful remote operation of the given kind
// (Created or Updated) and clears the Error condition.
func setSyncedConditions(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time, conditionType ctrutils.Type, msg string) {
	ctrutils.UpdateStatusConditions(&instance.Status.Conditions, now, ctrutils.DatadogConditionTypeError, metav1.ConditionFalse, "", "")
	ctrutils.UpdateStatusConditions(&instance.Status.Conditions, now, ctrutils.DatadogConditionTypeActive, metav1.ConditionTrue, "", msg)
	ctrutils.UpdateStatusConditions(&instance.Status.Conditions, now, conditionType, metav1.ConditionTrue, "", msg)
}

// setReadyCondition summarizes remote and worker Deployment health. A worker
// is ready once at least one replica is available and fully up to date.
func setReadyCondition(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time) {
	ready := instance.Status.ID != "" && instance.Status.Deployment != nil &&
		instance.Status.Deployment.AvailableReplicas > 0 &&
		instance.Status.Deployment.UnavailableReplicas == 0

	if ready {
		ctrutils.UpdateStatusConditions(&instance.Status.Conditions, now, ctrutils.DatadogConditionTypeActive, metav1.ConditionTrue, "", "private location is synced and the worker is available")
	} else {
		ctrutils.UpdateStatusConditions(&instance.Status.Conditions, now, ctrutils.DatadogConditionTypeActive, metav1.ConditionFalse, "", "waiting for the worker Deployment to become available")
	}
}
