// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentotelcollectorgateway "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/otelcollectorgateway"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
)

// OtelCollectorGatewayComponent implements ComponentReconciler for the OTel Collector Gateway deployment
type OtelCollectorGatewayComponent struct {
	reconciler *Reconciler
}

// NewOtelCollectorGatewayComponent creates a new OtelCollectorGateway component
func NewOtelCollectorGatewayComponent(reconciler *Reconciler) *OtelCollectorGatewayComponent {
	return &OtelCollectorGatewayComponent{
		reconciler: reconciler,
	}
}

// Name returns the component name
func (c *OtelCollectorGatewayComponent) Name() datadoghqv2alpha1.ComponentName {
	return datadoghqv2alpha1.OtelCollectorGatewayComponentName
}

// IsEnabled checks if the OtelCollectorGateway component should be reconciled
func (c *OtelCollectorGatewayComponent) IsEnabled(requiredComponents feature.RequiredComponents) bool {
	return requiredComponents.OtelCollectorGateway.IsEnabled()
}

// GetConditionType returns the condition type for status updates
func (c *OtelCollectorGatewayComponent) GetConditionType() string {
	return common.OtelCollectorGatewayReconcileConditionType
}

// Reconcile reconciles the OtelCollectorGateway component
func (c *OtelCollectorGatewayComponent) Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	var result reconcile.Result

	// Start by creating the Default OtelCollectorGateway deployment
	deployment := componentotelcollectorgateway.NewDefaultOtelCollectorGatewayDeployment(params.DDA)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	global.ApplyGlobalSettingsOtelCollectorGateway(params.Logger, podManagers, params.DDA.GetObjectMeta(), &params.DDA.Spec, params.ResourceManagers, params.RequiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range params.Features {
		if errFeat := feat.ManageOtelCollectorGateway(podManagers, ""); errFeat != nil {
			return result, errFeat
		}
	}

	// If Override is defined for the OtelCollectorGateway component, apply the override on the PodTemplateSpec
	if componentOverride, ok := params.DDA.Spec.Override[c.Name()]; ok {
		if apiutils.BoolValue(componentOverride.Disabled) {
			// This case is handled by the registry, but we double-check here
			return c.Cleanup(ctx, params)
		}
		override.PodTemplateSpec(params.Logger, podManagers, componentOverride, c.Name(), params.DDA.Name)
		override.Deployment(deployment, componentOverride)
	}

	return c.reconciler.createOrUpdateDeployment(params.Logger, params.DDA, deployment, params.Status, updateStatusV2WithOtelCollectorGateway)
}

// Cleanup removes the OtelCollectorGateway deployment
func (c *OtelCollectorGatewayComponent) Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	deployment := componentotelcollectorgateway.NewDefaultOtelCollectorGatewayDeployment(params.DDA)
	return c.reconciler.cleanupV2OtelCollectorGateway(params.Logger, params.DDA, deployment, params.Status)
}

func (r *Reconciler) cleanupV2OtelCollectorGateway(logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	// OtelCollectorGateway deployment attached to this instance
	otelCollectorGatewayDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, otelCollectorGatewayDeployment); err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	} else {
		logger.Info("Deleting OTel Collector Gateway Deployment", "deployment.Namespace", otelCollectorGatewayDeployment.Namespace, "deployment.Name", otelCollectorGatewayDeployment.Name)
		event := buildEventInfo(otelCollectorGatewayDeployment.Name, otelCollectorGatewayDeployment.Namespace, kubernetes.DeploymentKind, datadog.DeletionEvent)
		r.recordEvent(dda, event)
		if err := r.client.Delete(context.TODO(), otelCollectorGatewayDeployment); err != nil {
			return reconcile.Result{}, err
		}
	}

	deleteStatusWithOtelCollectorGateway(newStatus)

	return reconcile.Result{}, nil
}

func updateStatusV2WithOtelCollectorGateway(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.OtelCollectorGateway = condition.UpdateDeploymentStatus(deployment, newStatus.OtelCollectorGateway, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.OtelCollectorGatewayReconcileConditionType, status, reason, message, true)
}

func deleteStatusWithOtelCollectorGateway(newStatus *datadoghqv2alpha1.DatadogAgentStatus) {
	newStatus.OtelCollectorGateway = nil
	condition.DeleteDatadogAgentStatusCondition(newStatus, common.OtelCollectorGatewayReconcileConditionType)
}
