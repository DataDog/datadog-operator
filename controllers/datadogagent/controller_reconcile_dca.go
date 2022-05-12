package datadogagent

import (
	"context"
	"time"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	componentdca "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) reconcileV2ClusterAgent(logger logr.Logger, features []feature.Feature, dda *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var result reconcile.Result
	var err error

	// Start by creating the Default Cluster-Agent deployment
	deployment := componentdca.NewDefaultClusterAgentDeployment(dda)

	// Set Global setting on the default deployment
	deployment.Spec.Template = *override.ApplyGlobalSettings(&deployment.Spec.Template, dda.Spec.Global)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range features {
		podManager := feature.NewPodTemplateManagers(&deployment.Spec.Template)
		if errFeat := feat.ManageClusterAgent(podManager); errFeat != nil {
			return result, errFeat
		}
	}

	// If Override is define for the cluster-agent component, apply the override on the PodTemplateSpec, it will cascade to container.
	if _, ok := dda.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName]; ok {
		_, err = override.PodTemplateSpec(&deployment.Spec.Template, dda.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName])
		if err != nil {
			return result, err
		}
	}

	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, deployment, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// From here the PodTemplateSpec should be ready, we can generate the hash that will be use to compare this deployment with the current (if exist).
	var hash string
	hash, err = comparison.SetMD5DatadogAgentGenerationAnnotation(&deployment.ObjectMeta, deployment.Spec)
	if err != nil {
		return result, err
	}

	// Get the current deployment and compare
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	currentDeployment := &appsv1.Deployment{}
	alreadyExist := true
	err = r.client.Get(context.TODO(), nsName, currentDeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("the ClusterAgent deployment is not found", "name", nsName.Name, "namespace", nsName.Namespace)
			alreadyExist = false
		} else {
			logger.Error(err, "unexpected error during ClusterAgent deployment get", "name", nsName.Name, "namespace", nsName.Namespace)
			return reconcile.Result{}, err
		}
	}

	if alreadyExist {
		// check if same hash
		needUpdate := !comparison.IsSameSpecMD5Hash(hash, currentDeployment.GetAnnotations())
		if !needUpdate {
			// no need to update to stop here the process
			return reconcile.Result{}, nil
		}

		logger.Info("Updating an existing Cluster Agent Deployment", "deployment.Namespace", deployment.Namespace, "deployment.Name", deployment.Name, "currentHash", hash)

		// TODO: these parameter can be added to the override.PodTemplateSpec. (it exist in v1alpha)
		keepAnnotationsFilter := ""
		keepLabelsFilter := ""

		// Copy possibly changed fields
		updateDca := deployment.DeepCopy()
		updateDca.Spec = *deployment.Spec.DeepCopy()
		updateDca.Spec.Replicas = getReplicas(currentDeployment.Spec.Replicas, updateDca.Spec.Replicas)
		updateDca.Annotations = mergeAnnotationsLabels(logger, currentDeployment.GetAnnotations(), deployment.GetAnnotations(), keepAnnotationsFilter)
		updateDca.Labels = mergeAnnotationsLabels(logger, currentDeployment.GetLabels(), deployment.GetLabels(), keepLabelsFilter)

		now := metav1.NewTime(time.Now())
		err = kubernetes.UpdateFromObject(context.TODO(), r.client, updateDca, currentDeployment.ObjectMeta)
		if err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updateDca.Name, updateDca.Namespace, deploymentKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		updateStatusV2WithClusterAgent(updateDca, newStatus, now, metav1.ConditionTrue, "Cluster Agent Deployment updated")
	} else {
		now := metav1.NewTime(time.Now())

		// TODO: Create the Deployment here

		updateStatusV2WithClusterAgent(deployment, newStatus, now, metav1.ConditionTrue, "Cluster Agent Deployment created")
	}

	logger.Info("Creating a new Cluster Agent Deployment", "deployment.Namespace", deployment.Namespace, "deployment.Name", deployment.Name, "ClusterAgent.CurrentHash", hash)

	return result, err
}

func updateStatusV2WithClusterAgent(dda *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, message string) {
	// TODO(operator-ga): update status with DCA deployment information
	_ = dda // for linter purpose
	datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, updateTime, datadoghqv2alpha1.ClusterAgentReconcileConditionType, status, message, true)
}
