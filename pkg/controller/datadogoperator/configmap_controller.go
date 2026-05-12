package datadogoperator

import (
	"context"
	"fmt"
	"log"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ConfigMapReconciler reconciles a ConfigMap object.
type ConfigMapReconciler struct {
	Client client.Client
}

//+kubebuilder:rbac:groups=",",resources=configmaps,verbs=get;list;watch
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Printf("Reconcile ConfigMap %s/%s\n", req.Namespace, req.Name)
	// Use the uncached APIReader for one-off ConfigMap reads
	apiReader, err := r.Client.GetAPIReader()
	if err != nil {
		return ctrl.Result{}, err
	}
	// Try to get the ConfigMap, and log a warning if it's not accessible
	configMap := &corev1.ConfigMap{}
	err = apiReader.Get(ctx, req.NamespacedName, configMap)
	if err != nil {
		log.Printf(" Warning: unable to get ConfigMap %s/%s: %v\n", req.Namespace, req.Name, err)
		return ctrl.Result{}, nil
	}
	// Process the ConfigMap
	// ...
	return ctrl.Result{}, nil
}
