// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

func (r *ReconcileDatadogAgent) manageClusterAgentService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if dda.Spec.ClusterAgent == nil {
		return r.cleanupClusterAgentService(dda)
	}

	serviceName := getClusterAgentServiceName(dda)
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: serviceName}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterAgentService(logger, dda)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededClusterAgentService(logger, dda, service)
}

func (r *ReconcileDatadogAgent) createClusterAgentService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	newService, _ := newClusterAgentService(dda)
	return r.createService(logger, dda, newService)
}

func (r *ReconcileDatadogAgent) updateIfNeededClusterAgentService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentService *corev1.Service) (reconcile.Result, error) {
	newService, _ := newClusterAgentService(dda)
	return r.updateIfNeededService(logger, dda, currentService, newService)
}

func (r *ReconcileDatadogAgent) cleanupClusterAgentService(dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	serviceName := getClusterAgentServiceName(dda)
	return cleanupService(r.client, serviceName, dda.Namespace)
}

func newClusterAgentService(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.Service, string) {
	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := getDefaultAnnotations(dda)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getClusterAgentServiceName(dda),
			Namespace:   dda.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				datadoghqv1alpha1.AgentDeploymentNameLabelKey:      dda.Name,
				datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterAgentResourceSuffix,
			},
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultClusterAgentServicePort),
					Port:       datadoghqv1alpha1.DefaultClusterAgentServicePort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
	hash, _ := comparison.SetMD5GenerationAnnotation(&service.ObjectMeta, &service.Spec)

	return service, hash
}

func (r *ReconcileDatadogAgent) manageMetricsServerService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if dda.Spec.ClusterAgent == nil {
		return r.cleanupMetricsServerService(dda)
	}

	if !isMetricsProviderEnabled(dda.Spec.ClusterAgent) {
		return reconcile.Result{}, nil
	}

	serviceName := getMetricsServerServiceName(dda)
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: serviceName}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createMetricsServerService(logger, dda)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededMetricsServerService(logger, dda, service)
}

func (r *ReconcileDatadogAgent) createMetricsServerService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	newService, _ := newMetricsServerService(dda)
	return r.createService(logger, dda, newService)
}

func (r *ReconcileDatadogAgent) cleanupMetricsServerService(dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	serviceName := getMetricsServerServiceName(dda)
	return cleanupService(r.client, serviceName, dda.Namespace)
}

func (r *ReconcileDatadogAgent) updateIfNeededMetricsServerService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentService *corev1.Service) (reconcile.Result, error) {
	newService, _ := newMetricsServerService(dda)
	return r.updateIfNeededService(logger, dda, currentService, newService)
}

func (r *ReconcileDatadogAgent) createService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newService *corev1.Service) (reconcile.Result, error) {
	if err := controllerutil.SetControllerReference(dda, newService, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newService); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Create Service", "name", newService.Name)
	event := buildEventInfo(newService.Name, newService.Namespace, serviceKind, datadog.CreationEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{Requeue: true}, nil
}

func cleanupService(client client.Client, name, namespace string) (reconcile.Result, error) {
	service := &corev1.Service{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	err = client.Delete(context.TODO(), service)
	return reconcile.Result{}, err
}

func (r *ReconcileDatadogAgent) updateIfNeededService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentService, newService *corev1.Service) (reconcile.Result, error) {
	result := reconcile.Result{}
	hash := newService.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
	if !comparison.IsSameSpecMD5Hash(hash, currentService.GetAnnotations()) {

		updatedService := currentService.DeepCopy()
		updatedService.Labels = newService.Labels
		updatedService.Annotations = newService.Annotations
		updatedService.Spec = newService.Spec
		// ClusterIP is an immutable field
		updatedService.Spec.ClusterIP = currentService.Spec.ClusterIP

		if err := r.client.Update(context.TODO(), updatedService); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updatedService.Name, updatedService.Namespace, serviceKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		logger.Info("Update Service", "name", newService.Name)

		result.Requeue = true
	}

	return result, nil
}

func newMetricsServerService(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.Service, string) {
	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := getDefaultAnnotations(dda)

	port := getClusterAgentMetricsProviderPort(dda.Spec.ClusterAgent.Config)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getMetricsServerServiceName(dda),
			Namespace:   dda.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				datadoghqv1alpha1.AgentDeploymentNameLabelKey:      dda.Name,
				datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterAgentResourceSuffix,
			},
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(int(port)),
					Port:       port,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
	hash, _ := comparison.SetMD5GenerationAnnotation(&service.ObjectMeta, &service.Spec)

	return service, hash
}
