package datadogagent

import (
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	componentclc "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clustercheckrunner"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/override"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) reconcileV2ClusterCheckRunner(logger logr.Logger, features []feature.Feature, dda *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var result reconcile.Result
	var err error

	// Start by creating the Default Cluster-Agent deployment
	deployment := componentclc.NewDefaultClusterCheckRunnerDeployment(dda)

	// Set Global setting on the default deployment
	deployment.Spec.Template = *override.ApplyGlobalSettings(&deployment.Spec.Template, dda.Spec.Global)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range features {
		podManager := feature.NewPodTemplateManagers(&deployment.Spec.Template)
		if errFeat := feat.ManageClusterChecksRunner(podManager); errFeat != nil {
			return result, errFeat
		}
	}

	// If Override is define for the cluster-check-runner component, apply the override on the PodTemplateSpec, it will cascade to container.
	if _, ok := dda.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName]; ok {
		_, err = override.PodTemplateSpec(&deployment.Spec.Template, dda.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName])
		if err != nil {
			return result, err
		}
	}

	deploymentLogger := logger.WithValues("component", datadoghqv2alpha1.ClusterCheckRunnerReconcileConditionType)
	return r.createOrUpdateDeployment(deploymentLogger, dda, deployment, newStatus, updateStatusV2WithClusterCheckRunner)
}

func updateStatusV2WithClusterCheckRunner(newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, message string) {
	// TODO(operator-ga): update status with DCA deployment information
	datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, updateTime, datadoghqv2alpha1.ClusterCheckRunnerReconcileConditionType, status, message, true)
}
