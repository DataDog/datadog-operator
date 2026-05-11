package datadogagent

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// useDefaultDaemonset determines if we should use a legacy provider specific Daemonset for EKS and Openshift providers
func (r *Reconciler) useDefaultDaemonset(providerList map[string]struct{}) bool {
	if len(providerList) == 0 {
		return false
	}
	return kubernetes.ShouldUseDefaultDaemonset(providerList)
}

// generateNewStatusFromDDA generates a new status from a DDA status.
// If an existing DCA token is present, it is copied to the new status.
func generateNewStatusFromDDA(ddaStatus *datadoghqv2alpha1.DatadogAgentStatus) *datadoghqv2alpha1.DatadogAgentStatus {
	status := &datadoghqv2alpha1.DatadogAgentStatus{}
	if ddaStatus != nil {
		if ddaStatus.ClusterAgent != nil && ddaStatus.ClusterAgent.GeneratedToken != "" {
			status.ClusterAgent = &datadoghqv2alpha1.DeploymentStatus{
				GeneratedToken: ddaStatus.ClusterAgent.GeneratedToken,
			}
		}
		if ddaStatus.RemoteConfigConfiguration != nil {
			status.RemoteConfigConfiguration = ddaStatus.RemoteConfigConfiguration
		}
		status.Experiment = ddaStatus.Experiment.DeepCopy()
	}
	return status
}

// deleteDeploymentWithEvent deletes a deployment and records DDA event only if deletion was successful
func (r *Reconciler) deleteDeploymentWithEvent(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, deployment *appsv1.Deployment) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	// Existing deployment attached to this instance
	existingDeployment := &appsv1.Deployment{}
	if err := r.client.Get(ctx, nsName, existingDeployment); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("Deleting Deployment", "deployment.Namespace", existingDeployment.Namespace, "deployment.Name", existingDeployment.Name)
	if err := r.client.Delete(ctx, existingDeployment); err != nil {
		return reconcile.Result{}, err
	}
	// Record event only if deletion was successful
	event := buildEventInfo(existingDeployment.Name, existingDeployment.Namespace, kubernetes.DeploymentKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{}, nil
}
