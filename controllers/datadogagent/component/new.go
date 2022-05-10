package component

import (
	appsv1 "k8s.io/api/apps/v1"

	// corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
)

// NewDeployment use to generate the skeleton of a new deployment based on few information
func NewDeployment(owner metav1.Object, componentKind, componentName, version string, selector *metav1.LabelSelector) *appsv1.Deployment {
	labels := object.GetDefaultLabels(owner, componentName, version)
	labels[apicommon.AgentDeploymentNameLabelKey] = owner.GetName()
	labels[apicommon.AgentDeploymentComponentLabelKey] = componentKind

	if selector != nil {
		for key, val := range selector.MatchLabels {
			labels[key] = val
		}
	} else {
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				apicommon.AgentDeploymentNameLabelKey:      owner.GetName(),
				apicommon.AgentDeploymentComponentLabelKey: componentKind,
			},
		}
	}

	annotations := object.GetDefaultAnnotations(owner)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentName,
			Namespace:   owner.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: selector,
		},
	}

	return deployment
}
