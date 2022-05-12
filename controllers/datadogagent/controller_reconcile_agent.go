package datadogagent

import (
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) reconcileV2Agent(logger logr.Logger, features []feature.Feature, dda *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var result reconcile.Result
	var err error

	// TODO(operator-ga): implement reconcileV2Agent

	return result, err
}
