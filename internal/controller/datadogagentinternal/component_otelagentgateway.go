// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentotelagentgateway "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/otelagentgateway"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// OtelAgentGatewayComponent implements ComponentReconciler for the OTel Agent Gateway deployment
type OtelAgentGatewayComponent struct {
	reconciler *Reconciler
}

// NewOtelAgentGatewayComponent creates a new OtelAgentGateway component
func NewOtelAgentGatewayComponent(reconciler *Reconciler) *OtelAgentGatewayComponent {
	return &OtelAgentGatewayComponent{
		reconciler: reconciler,
	}
}

// Name returns the component name
func (c *OtelAgentGatewayComponent) Name() datadoghqv2alpha1.ComponentName {
	return datadoghqv2alpha1.OtelAgentGatewayComponentName
}

// IsEnabled checks if the OtelAgentGateway component should be reconciled
func (c *OtelAgentGatewayComponent) IsEnabled(requiredComponents feature.RequiredComponents) bool {
	return requiredComponents.OtelAgentGateway.IsEnabled()
}

// GetConditionType returns the condition type for status updates
func (c *OtelAgentGatewayComponent) GetConditionType() string {
	return common.OtelAgentGatewayReconcileConditionType
}

// Reconcile reconciles the OtelAgentGateway component
func (c *OtelAgentGatewayComponent) Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	var result reconcile.Result

	// Start by creating the Default OtelAgentGateway deployment
	deployment := componentotelagentgateway.NewDefaultOtelAgentGatewayDeployment(params.DDAI, &params.DDAI.Spec)
	objLogger := ctrl.LoggerFrom(ctx).WithValues("object.kind", "Deployment", "object.namespace", deployment.Namespace, "object.name", deployment.Name)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	global.ApplyGlobalSettingsOtelAgentGateway(objLogger, podManagers, params.DDAI.GetObjectMeta(), &params.DDAI.Spec, params.ResourceManagers, params.RequiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range params.Features {
		if errFeat := feat.ManageOtelAgentGateway(podManagers, ""); errFeat != nil {
			return result, errFeat
		}
	}

	// If Override is defined for the OtelAgentGateway component, apply the override on the PodTemplateSpec
	if componentOverride, ok := params.DDAI.Spec.Override[c.Name()]; ok {
		if apiutils.BoolValue(componentOverride.Disabled) {
			// This case is handled by the registry, but we double-check here
			return c.Cleanup(ctx, params)
		}
		override.PodTemplateSpec(objLogger, podManagers, componentOverride, c.Name(), params.DDAI.Name)
		override.Deployment(deployment, componentOverride)
	}

	return c.reconciler.createOrUpdateDeployment(objLogger, params.DDAI, deployment, params.Status, updateStatusV2WithOtelAgentGateway)
}

// Cleanup removes the OtelAgentGateway deployment
func (c *OtelAgentGatewayComponent) Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	deployment := componentotelagentgateway.NewDefaultOtelAgentGatewayDeployment(params.DDAI, &params.DDAI.Spec)
	objLogger := ctrl.LoggerFrom(ctx).WithValues("object.kind", "Deployment", "object.namespace", deployment.Namespace, "object.name", deployment.Name)
	return c.reconciler.cleanupV2OtelAgentGateway(objLogger, params.DDAI, deployment, params.Status)
}

func (r *Reconciler) cleanupV2OtelAgentGateway(logger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, deployment *appsv1.Deployment, newStatus *v1alpha1.DatadogAgentInternalStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	// OtelAgentGateway deployment attached to this instance
	otelAgentGatewayDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, otelAgentGatewayDeployment); err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	} else {
		logger.Info("Deleting OTel Agent Gateway Deployment")
		event := buildEventInfo(otelAgentGatewayDeployment.Name, otelAgentGatewayDeployment.Namespace, kubernetes.DeploymentKind, datadog.DeletionEvent)
		r.recordEvent(ddai, event)
		if err := r.client.Delete(context.TODO(), otelAgentGatewayDeployment); err != nil {
			return reconcile.Result{}, err
		}
	}

	deleteStatusWithOtelAgentGateway(newStatus)

	return reconcile.Result{}, nil
}

func updateStatusV2WithOtelAgentGateway(deployment *appsv1.Deployment, newStatus *v1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.OtelAgentGateway = condition.UpdateDeploymentStatus(deployment, newStatus.OtelAgentGateway, &updateTime)
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, updateTime, common.OtelAgentGatewayReconcileConditionType, status, reason, message, true)
}

func deleteStatusWithOtelAgentGateway(newStatus *v1alpha1.DatadogAgentInternalStatus) {
	newStatus.OtelAgentGateway = nil
	condition.DeleteDatadogAgentInternalStatusCondition(newStatus, common.OtelAgentGatewayReconcileConditionType)
}
