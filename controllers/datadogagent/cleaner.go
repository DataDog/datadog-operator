// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilserrors "k8s.io/apimachinery/pkg/util/errors"
	kversion "k8s.io/apimachinery/pkg/version"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// handleDeprecatedResources this function is used to cleanup old resources that was used by the DatadogAgent
func (r *Reconciler) handleDeprecatedResources(reqLogger logr.Logger, now time.Time, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) error {
	// get cleaner status condition
	cleanerCondition, err := condition.GetDatadogAgentStatusCondition(newStatus, datadoghqv1alpha1.DatadogAgentConditionTypeResourcesCleaner, metav1.NewTime(now), true)
	if err != nil {
		return err
	}
	lastRun := cleanerCondition.LastUpdateTime.Time
	if lastRun.Add(r.cleanUpPeriod).Sub(now) > 0 {
		return nil
	}

	status := corev1.ConditionTrue
	description := ""
	if _, err = cleanupOldClusterRBACs(reqLogger, r.client, r.versionInfo, dda); err != nil {
		status = corev1.ConditionFalse
		description = fmt.Sprintf("err: %v", err)
	}
	condition.UpdateDatadogAgentStatusConditions(newStatus, metav1.NewTime(now), datadoghqv1alpha1.DatadogAgentConditionTypeResourcesCleaner, status, description, true)

	return err
}

// cleanupOldClusterRBACs use to delete resources that should not exist anymore.
// for example if the user update the name of the DCA deployment, the RBAC name is updated, new resources are created, but older wasn't cleanup
func cleanupOldClusterRBACs(reqLogger logr.Logger, k8sClient client.Client, version *kversion.Info, dda *datadoghqv1alpha1.DatadogAgent) ([]client.Object, error) {
	// Get All RBAC attached to the DDA
	labelSet := labels.Set{
		kubernetes.AppKubernetesPartOfLabelKey: NewPartOfLabelValue(dda).String(),
	}
	listOptions := &client.ListOptions{
		LabelSelector: labelSet.AsSelector(),
	}

	// all ClusterRole attached to the DDA
	allClusterRoles := &rbacv1.ClusterRoleList{}
	if err := k8sClient.List(context.TODO(), allClusterRoles, listOptions); err != nil {
		return nil, fmt.Errorf("unabled to list ClusterRole, err: %w", err)
	}

	// all ClusterRole attached to the DDA
	allClusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
	if err := k8sClient.List(context.TODO(), allClusterRoleBindings, listOptions); err != nil {
		return nil, fmt.Errorf("unabled to list ClusterRoleBinding, err: %w", err)
	}

	// Get the list of current RBAC
	currentResourceNameMap := map[string]struct{}{}
	for _, name := range rbacNamesForDda(dda, version) {
		currentResourceNameMap[name] = struct{}{}
	}

	var resourcesToDeleted []client.Object
	// Make the diff
	for id, clusterRole := range allClusterRoles.Items {
		if _, found := currentResourceNameMap[clusterRole.Name]; !found {
			resourcesToDeleted = append(resourcesToDeleted, &allClusterRoles.Items[id])
		}
	}
	for id, clusterRoleBinding := range allClusterRoleBindings.Items {
		if _, found := currentResourceNameMap[clusterRoleBinding.Name]; !found {
			resourcesToDeleted = append(resourcesToDeleted, &allClusterRoleBindings.Items[id])
		}
	}

	// Cleanup (delete)
	var errs []error
	for _, obj := range resourcesToDeleted {
		if err := k8sClient.Delete(context.TODO(), obj); err != nil {
			errs = append(errs, err)
			reqLogger.Error(err, "cleanupOldClusterRBAC: unable to delete object", getObjInfo(obj)...)
		}
		reqLogger.Info("cleanupOldClusterRBAC: object deleted", getObjInfo(obj)...)
	}

	return resourcesToDeleted, utilserrors.NewAggregate(errs)
}

func getObjInfo(obj client.Object) []interface{} {
	return []interface{}{"kind", obj.GetObjectKind(), "namespace", obj.GetNamespace(), "name", obj.GetName()}
}
