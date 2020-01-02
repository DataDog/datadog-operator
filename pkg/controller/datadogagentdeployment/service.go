// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

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

func (r *ReconcileDatadogAgentDeployment) manageClusterAgentService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	if dad.Spec.ClusterAgent == nil {
		return r.cleanupClusterAgentService(logger, dad, newStatus)
	}

	serviceName := getClusterAgentServiceName(dad)
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dad.Namespace, Name: serviceName}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterAgentService(logger, dad, newStatus)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededClusterAgentService(logger, dad, service, newStatus)
}

func (r *ReconcileDatadogAgentDeployment) createClusterAgentService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newService, _ := newClusterAgentService(dad)
	return r.createService(logger, dad, newService, newStatus)
}

func (r *ReconcileDatadogAgentDeployment) updateIfNeededClusterAgentService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, currentService *corev1.Service, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newService, _ := newClusterAgentService(dad)
	return r.updateIfNeededService(logger, dad, currentService, newService, newStatus)
}

func (r *ReconcileDatadogAgentDeployment) cleanupClusterAgentService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	serviceName := getClusterAgentServiceName(dad)
	return cleanupService(r.client, serviceName, dad.Namespace)
}

func newClusterAgentService(dad *datadoghqv1alpha1.DatadogAgentDeployment) (*corev1.Service, string) {
	labels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
	annotations := getDefaultAnnotations(dad)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getClusterAgentServiceName(dad),
			Namespace:   dad.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				datadoghqv1alpha1.AgentDeploymentNameLabelKey:      dad.Name,
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

func (r *ReconcileDatadogAgentDeployment) manageMetricsServerService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	if dad.Spec.ClusterAgent == nil {
		return r.cleanupMetricsServerService(logger, dad, newStatus)
	}

	if !isMetricsProviderEnabled(dad.Spec.ClusterAgent) {
		return reconcile.Result{}, nil
	}

	serviceName := getMetricsServerServiceName(dad)
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dad.Namespace, Name: serviceName}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createMetricsServerService(logger, dad, newStatus)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededMetricsServerService(logger, dad, service, newStatus)
}

func (r *ReconcileDatadogAgentDeployment) createMetricsServerService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newService, _ := newMetricsServerService(dad)
	return r.createService(logger, dad, newService, newStatus)
}

func (r *ReconcileDatadogAgentDeployment) cleanupMetricsServerService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	serviceName := getMetricsServerServiceName(dad)
	return cleanupService(r.client, serviceName, dad.Namespace)
}

func (r *ReconcileDatadogAgentDeployment) updateIfNeededMetricsServerService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, currentService *corev1.Service, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newService, _ := newMetricsServerService(dad)
	return r.updateIfNeededService(logger, dad, currentService, newService, newStatus)
}

func (r *ReconcileDatadogAgentDeployment) createService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newService *corev1.Service, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	if err := controllerutil.SetControllerReference(dad, newService, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newService); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Create Service", "name", newService.Name)
	eventInfo := buildEventInfo(newService.Name, newService.Namespace, serviceKind, datadog.CreationEvent)
	r.recordEvent(dad, eventInfo)

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

func (r *ReconcileDatadogAgentDeployment) updateIfNeededService(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, currentService, newService *corev1.Service, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	result := reconcile.Result{}
	hash := newService.Annotations[string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey)]
	if !comparison.CompareSpecMD5Hash(hash, currentService.GetAnnotations()) {

		updatedService := currentService.DeepCopy()
		updatedService.Labels = newService.Labels
		updatedService.Annotations = newService.Annotations
		updatedService.Spec = newService.Spec

		if err := r.client.Update(context.TODO(), updatedService); err != nil {
			return reconcile.Result{}, err
		}
		eventInfo := buildEventInfo(updatedService.Name, updatedService.Namespace, serviceKind, datadog.UpdateEvent)
		r.recordEvent(dad, eventInfo)
		logger.Info("Update Service", "name", newService.Name)

		result.Requeue = true
	}

	return result, nil
}

func newMetricsServerService(dad *datadoghqv1alpha1.DatadogAgentDeployment) (*corev1.Service, string) {
	labels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
	annotations := getDefaultAnnotations(dad)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getMetricsServerServiceName(dad),
			Namespace:   dad.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				datadoghqv1alpha1.AgentDeploymentNameLabelKey:      dad.Name,
				datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterAgentResourceSuffix,
			},
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultMetricsServerServicePort),
					Port:       datadoghqv1alpha1.DefaultMetricsServerServicePort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
	hash, _ := comparison.SetMD5GenerationAnnotation(&service.ObjectMeta, &service.Spec)

	return service, hash
}
