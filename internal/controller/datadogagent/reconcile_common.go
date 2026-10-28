// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	controllercommon "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/trace"
)

func (r *Reconciler) createOrUpdateDDAI(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal) (err error) {
	span, ctx := trace.StartSpan(ctx, trace.Tag("object.name", ddai.Name))
	defer trace.FinishSpan(span, &err)

	logger := ctrl.LoggerFrom(ctx)

	currentDDAI := &v1alpha1.DatadogAgentInternal{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: ddai.Name, Namespace: ddai.Namespace}, currentDDAI); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "unexpected error during DDAI get")
			return err
		}
		// Create the DDAI object if it doesn't exist
		logger.Info("creating DatadogAgentInternal", "object.kind", "DatadogAgentInternal", "object.namespace", ddai.Namespace, "object.name", ddai.Name)
		if err := r.client.Create(ctx, ddai); err != nil {
			return err
		}
		return nil
	}

	// By comparing annotations, we reconcile either instantly if the spec annotation changed or after the reconcile period
	// if only the annotations changed.
	if !maps.Equal(currentDDAI.Annotations, ddai.Annotations) {
		logger.Info("updating DatadogAgentInternal", "object.kind", "DatadogAgentInternal", "object.namespace", ddai.Namespace, "object.name", ddai.Name)
		if err := kubernetes.UpdateFromObject(ctx, r.client, ddai, currentDDAI.ObjectMeta); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) addDDAIStatusToDDAStatus(ctx context.Context, status *v2alpha1.DatadogAgentStatus, ddai metav1.ObjectMeta, now metav1.Time) error {
	logger := ctrl.LoggerFrom(ctx)
	currentDDAI := &v1alpha1.DatadogAgentInternal{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: ddai.Name, Namespace: ddai.Namespace}, currentDDAI); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "unexpected error during DDAI get")
			return err
		}
		// DDAI not yet created
		return nil
	}

	status.Agent = condition.CombineDaemonSetStatus(status.Agent, currentDDAI.Status.Agent)
	status.ClusterAgent = condition.CombineDeploymentStatus(status.ClusterAgent, currentDDAI.Status.ClusterAgent)
	status.ClusterChecksRunner = condition.CombineDeploymentStatus(status.ClusterChecksRunner, currentDDAI.Status.ClusterChecksRunner)

	// Only the default DDAI runs dependency management (e.g. RBAC-gated
	// resources), so it's the only DDAI whose reconcile error is surfaced on
	// the DDA. Profile DDAI reconcile errors are surfaced on their own
	// DatadogAgentProfile status instead (see addDDAIStatusToProfileStatus).
	if currentDDAI.Labels[constants.ProfileLabelKey] == "" {
		if ddaiErrCond := condition.GetDDAICondition(&currentDDAI.Status, controllercommon.DatadogAgentReconcileErrorConditionType); ddaiErrCond != nil && ddaiErrCond.Status == metav1.ConditionTrue {
			condition.UpdateDatadogAgentStatusConditions(status, now, controllercommon.DatadogAgentInternalReconcileErrorConditionType, metav1.ConditionTrue, "DatadogAgentInternal_reconcile_error", fmt.Sprintf("%s: %s", currentDDAI.Name, ddaiErrCond.Message), false)
		}
	}

	return nil
}

func IsEqualStatus(current *v2alpha1.DatadogAgentStatus, newStatus *v2alpha1.DatadogAgentStatus) bool {
	if current == nil && newStatus == nil {
		return true
	}
	if current == nil || newStatus == nil {
		return false
	}

	if !condition.IsEqualDaemonSetStatus(current.Agent, newStatus.Agent) ||
		!apiequality.Semantic.DeepEqual(current.RemoteConfigConfiguration, newStatus.RemoteConfigConfiguration) {
		return false
	}

	// Compare AgentList order-insensitively to avoid spurious diffs and prevent panics
	if len(current.AgentList) != len(newStatus.AgentList) {
		return false
	}
	if len(current.AgentList) > 0 {
		// Clone to avoid mutating the original slices
		ac := slices.Clone(current.AgentList)
		bc := slices.Clone(newStatus.AgentList)

		sortByNameFunc := func(a, b *v2alpha1.DaemonSetStatus) int {
			var an, bn string
			if a != nil {
				an = a.DaemonsetName
			}
			if b != nil {
				bn = b.DaemonsetName
			}
			return strings.Compare(an, bn)
		}

		slices.SortFunc(ac, sortByNameFunc)
		slices.SortFunc(bc, sortByNameFunc)
		for i := range ac {
			if !condition.IsEqualDaemonSetStatus(ac[i], bc[i]) {
				return false
			}
		}
	}

	if !condition.IsEqualDeploymentStatus(current.ClusterAgent, newStatus.ClusterAgent) ||
		!condition.IsEqualDeploymentStatus(current.ClusterChecksRunner, newStatus.ClusterChecksRunner) {
		return false
	}

	if !apiequality.Semantic.DeepEqual(current.Experiment, newStatus.Experiment) {
		return false
	}

	return condition.IsEqualConditions(current.Conditions, newStatus.Conditions)
}
