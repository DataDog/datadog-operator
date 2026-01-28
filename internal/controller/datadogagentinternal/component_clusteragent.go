// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// ClusterAgentComponent implements ComponentReconciler for the Cluster Agent deployment
type ClusterAgentComponent struct {
	reconciler *Reconciler
}

// NewClusterAgentComponent creates a new ClusterAgent component
func NewClusterAgentComponent(reconciler *Reconciler) *ClusterAgentComponent {
	return &ClusterAgentComponent{
		reconciler: reconciler,
	}
}

// Name returns the component name
func (c *ClusterAgentComponent) Name() datadoghqv2alpha1.ComponentName {
	return datadoghqv2alpha1.ClusterAgentComponentName
}

// IsEnabled checks if the Cluster Agent component should be reconciled
func (c *ClusterAgentComponent) IsEnabled(requiredComponents feature.RequiredComponents) bool {
	return requiredComponents.ClusterAgent.IsEnabled()
}

// GetConditionType returns the condition type for status updates
func (c *ClusterAgentComponent) GetConditionType() string {
	return common.ClusterAgentReconcileConditionType
}

// Reconcile reconciles the Cluster Agent component
func (c *ClusterAgentComponent) Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	var result reconcile.Result
	now := metav1.NewTime(time.Now())

	// Start by creating the Default Cluster-Agent deployment
	deployment := componentdca.NewDefaultClusterAgentDeployment(params.DDAI.GetObjectMeta(), &params.DDAI.Spec)
	objLogger := ctrl.LoggerFrom(ctx).WithValues("object.kind", "Deployment", "object.namespace", deployment.Namespace, "object.name", deployment.Name)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	global.ApplyGlobalSettingsClusterAgent(objLogger, podManagers, params.DDAI.GetObjectMeta(), &params.DDAI.Spec, params.ResourceManagers, params.RequiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	var featErrors []error
	for _, feat := range params.Features {
		if errFeat := feat.ManageClusterAgent(podManagers, params.Provider); errFeat != nil {
			featErrors = append(featErrors, errFeat)
		}
	}
	if len(featErrors) > 0 {
		err := utilerrors.NewAggregate(featErrors)
		updateStatusV2WithClusterAgent(deployment, params.Status, now, metav1.ConditionFalse, "ClusterAgent feature error", err.Error())
		return result, err
	}

	// If Override is defined for the clusterAgent component, apply the override on the PodTemplateSpec
	if componentOverride, ok := params.DDAI.Spec.Override[c.Name()]; ok {
		if apiutils.BoolValue(componentOverride.Disabled) {
			// This case is handled by the registry, but we double-check here
			return c.Cleanup(ctx, params)
		}
		override.PodTemplateSpec(objLogger, podManagers, componentOverride, c.Name(), params.DDAI.Name)
		override.Deployment(deployment, componentOverride)
	}

	return c.reconciler.createOrUpdateDeployment(objLogger, params.DDAI, deployment, params.Status, updateStatusV2WithClusterAgent)
}

// Cleanup removes the Cluster Agent deployment and associated resources
func (c *ClusterAgentComponent) Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	deployment := componentdca.NewDefaultClusterAgentDeployment(params.DDAI.GetObjectMeta(), &params.DDAI.Spec)
	objLogger := ctrl.LoggerFrom(ctx).WithValues("object.kind", "Deployment", "object.namespace", deployment.Namespace, "object.name", deployment.Name)
	return c.reconciler.cleanupV2ClusterAgent(objLogger, params.DDAI, deployment, params.ResourceManagers, params.Status)
}

// The following functions are kept for backward compatibility with existing code

// cleanupOldDCADeployments deletes DCA deployments when a DCA Deployment's name is changed using clusterAgent name override
func (r *Reconciler) cleanupOldDCADeployments(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, resourcesManager feature.ResourceManagers, newStatus *v1alpha1.DatadogAgentInternalStatus) error {
	matchLabels := client.MatchingLabels{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
	}
	deploymentName := component.GetDeploymentNameFromDatadogAgent(ddai.GetObjectMeta(), &ddai.Spec)
	deploymentList := appsv1.DeploymentList{}
	if err := r.client.List(ctx, &deploymentList, matchLabels); err != nil {
		return err
	}
	for _, deployment := range deploymentList.Items {
		if deploymentName != deployment.Name {
			objLogger := ctrl.LoggerFrom(ctx).WithValues("object.kind", "Deployment", "object.namespace", deployment.Namespace, "object.name", deployment.Name)
			if _, err := r.cleanupV2ClusterAgent(objLogger, ddai, &deployment, resourcesManager, newStatus); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Reconciler) cleanupV2ClusterAgent(logger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, deployment *appsv1.Deployment, resourcesManager feature.ResourceManagers, newStatus *v1alpha1.DatadogAgentInternalStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	// ClusterAgentDeployment attached to this instance
	clusterAgentDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, clusterAgentDeployment); err != nil {
		if errors.IsNotFound(err) {
			deleteStatusV2WithClusterAgent(newStatus)

			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("Deleting Cluster Agent Deployment")
	event := buildEventInfo(clusterAgentDeployment.Name, clusterAgentDeployment.Namespace, kubernetes.ClusterRoleBindingKind, datadog.DeletionEvent)
	r.recordEvent(ddai, event)
	if err := r.client.Delete(context.TODO(), clusterAgentDeployment); err != nil {
		return reconcile.Result{}, err
	}

	// Delete associated RBACs as well
	rbacManager := resourcesManager.RBACManager()
	logger.Info("Deleting Cluster Agent RBACs")
	if err := rbacManager.DeleteServiceAccountByComponent(string(datadoghqv2alpha1.ClusterAgentComponentName), ddai.Namespace); err != nil {
		return reconcile.Result{}, err
	}
	if err := rbacManager.DeleteRoleByComponent(string(datadoghqv2alpha1.ClusterAgentComponentName), ddai.Namespace); err != nil {
		return reconcile.Result{}, err
	}
	if err := rbacManager.DeleteClusterRoleByComponent(string(datadoghqv2alpha1.ClusterAgentComponentName)); err != nil {
		return reconcile.Result{}, err
	}

	deleteStatusV2WithClusterAgent(newStatus)

	return reconcile.Result{}, nil
}

func updateStatusV2WithClusterAgent(dca *appsv1.Deployment, newStatus *v1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.ClusterAgent = condition.UpdateDeploymentStatus(dca, newStatus.ClusterAgent, &updateTime)
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, updateTime, common.ClusterAgentReconcileConditionType, status, reason, message, true)
}

func deleteStatusV2WithClusterAgent(newStatus *v1alpha1.DatadogAgentInternalStatus) {
	newStatus.ClusterAgent = nil
	condition.DeleteDatadogAgentInternalStatusCondition(newStatus, common.ClusterAgentReconcileConditionType)
}
