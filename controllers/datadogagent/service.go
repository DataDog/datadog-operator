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

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

func (r *Reconciler) manageClusterAgentService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
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

func (r *Reconciler) createClusterAgentService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	newService, _ := newClusterAgentService(dda)
	return r.createService(logger, dda, newService)
}

func (r *Reconciler) updateIfNeededClusterAgentService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentService *corev1.Service) (reconcile.Result, error) {
	newService, _ := newClusterAgentService(dda)
	return r.updateIfNeededService(logger, dda, currentService, newService)
}

func (r *Reconciler) cleanupClusterAgentService(dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
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
	hash, _ := comparison.SetMD5DatadogAgentGenerationAnnotation(&service.ObjectMeta, &service.Spec)

	return service, hash
}

func (r *Reconciler) manageMetricsServerService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if !isMetricsProviderEnabled(dda.Spec.ClusterAgent) {
		return r.cleanupMetricsServerService(dda)
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

func (r *Reconciler) manageMetricsServerAPIService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if !isMetricsProviderEnabled(dda.Spec.ClusterAgent) {
		return r.cleanupMetricsServerAPIService(logger)
	}

	apiServiceName := getMetricsServerAPIServiceName()
	apiService := &apiregistrationv1.APIService{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: apiServiceName}, apiService)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createMetricsServerAPIService(logger, dda)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededMetricsServerAPIService(logger, dda, apiService)
}

func (r *Reconciler) manageAdmissionControllerService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if !isAdmissionControllerEnabled(dda.Spec.ClusterAgent) {
		return r.cleanupAdmissionControllerService(dda)
	}

	serviceName := getAdmissionControllerServiceName(dda)
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: serviceName}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createAdmissionControllerService(logger, dda)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededAdmissionControllerService(logger, dda, service)
}

func (r *Reconciler) createMetricsServerService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	newService, _ := newMetricsServerService(dda)
	return r.createService(logger, dda, newService)
}

func (r *Reconciler) createMetricsServerAPIService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	newAPIService, _ := newMetricsServerAPIService(dda)
	return r.createAPIService(logger, dda, newAPIService)
}

func (r *Reconciler) createAdmissionControllerService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	newService, _ := newAdmissionControllerService(dda)
	return r.createService(logger, dda, newService)
}

func (r *Reconciler) cleanupMetricsServerService(dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	serviceName := getMetricsServerServiceName(dda)
	return cleanupService(r.client, serviceName, dda.Namespace)
}

func (r *Reconciler) cleanupMetricsServerAPIService(logger logr.Logger) (reconcile.Result, error) {
	apiServiceName := getMetricsServerAPIServiceName()
	return r.cleanupAPIService(logger, apiServiceName)
}

func (r *Reconciler) cleanupAdmissionControllerService(dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	serviceName := getAdmissionControllerServiceName(dda)
	return cleanupService(r.client, serviceName, dda.Namespace)
}

func (r *Reconciler) updateIfNeededMetricsServerService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentService *corev1.Service) (reconcile.Result, error) {
	newService, _ := newMetricsServerService(dda)
	return r.updateIfNeededService(logger, dda, currentService, newService)
}

func (r *Reconciler) updateIfNeededMetricsServerAPIService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentAPIService *apiregistrationv1.APIService) (reconcile.Result, error) {
	newAPIService, _ := newMetricsServerAPIService(dda)
	return r.updateIfNeededAPIService(logger, dda, currentAPIService, newAPIService)
}

func (r *Reconciler) updateIfNeededAdmissionControllerService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentService *corev1.Service) (reconcile.Result, error) {
	newService, _ := newAdmissionControllerService(dda)
	return r.updateIfNeededService(logger, dda, currentService, newService)
}

func (r *Reconciler) createService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newService *corev1.Service) (reconcile.Result, error) {
	if err := controllerutil.SetControllerReference(dda, newService, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newService); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Created Service", "name", newService.Name)
	event := buildEventInfo(newService.Name, newService.Namespace, serviceKind, datadog.CreationEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{Requeue: true}, nil
}

func (r *Reconciler) createAPIService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newAPIService *apiregistrationv1.APIService) (reconcile.Result, error) {
	if err := SetOwnerReference(dda, newAPIService, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newAPIService); err != nil {
		logger.Error(err, "failed to create APIService", "name", newAPIService.Name)
		return reconcile.Result{}, err
	}
	logger.Info("Created APIService", "name", newAPIService.Name)
	event := buildEventInfo(newAPIService.Name, newAPIService.Namespace, apiServiceKind, datadog.CreationEvent)
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

func (r *Reconciler) cleanupAPIService(logger logr.Logger, name string) (reconcile.Result, error) {
	apiService := &apiregistrationv1.APIService{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: name}, apiService)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	err = r.client.Delete(context.TODO(), apiService)
	if err != nil {
		logger.Error(err, "failed to delete APIService", "name", name)
	}
	logger.Info("Deleted APIService", "name", name)
	return reconcile.Result{}, err
}

func (r *Reconciler) updateIfNeededService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentService, newService *corev1.Service) (reconcile.Result, error) {
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

func (r *Reconciler) updateIfNeededAPIService(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, currentAPIService, newAPIService *apiregistrationv1.APIService) (reconcile.Result, error) {
	result := reconcile.Result{}
	hash := newAPIService.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
	if !comparison.IsSameSpecMD5Hash(hash, currentAPIService.GetAnnotations()) {

		updatedAPIService := currentAPIService.DeepCopy()
		updatedAPIService.Labels = newAPIService.Labels
		updatedAPIService.Annotations = newAPIService.Annotations
		updatedAPIService.Spec = newAPIService.Spec

		if err := r.client.Update(context.TODO(), updatedAPIService); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updatedAPIService.Name, updatedAPIService.Namespace, apiServiceKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		logger.Info("Update APIService", "name", newAPIService.Name)

		result.Requeue = true
	}

	return result, nil
}

func newMetricsServerService(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.Service, string) {
	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := getDefaultAnnotations(dda)

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
					TargetPort: intstr.FromInt(int(getClusterAgentMetricsProviderPort(dda.Spec.ClusterAgent.Config))),
					Port:       datadoghqv1alpha1.DefaultMetricsServerServicePort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
	hash, _ := comparison.SetMD5DatadogAgentGenerationAnnotation(&service.ObjectMeta, &service.Spec)

	return service, hash
}

func newMetricsServerAPIService(dda *datadoghqv1alpha1.DatadogAgent) (*apiregistrationv1.APIService, string) {
	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := getDefaultAnnotations(dda)

	port := int32(datadoghqv1alpha1.DefaultMetricsServerServicePort)
	apiService := &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getMetricsServerAPIServiceName(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: apiregistrationv1.APIServiceSpec{
			Service: &apiregistrationv1.ServiceReference{
				Name:      getMetricsServerServiceName(dda),
				Namespace: dda.Namespace,
				Port:      &port,
			},
			Version:               "v1beta1",
			InsecureSkipTLSVerify: true,
			Group:                 "external.metrics.k8s.io",
			GroupPriorityMinimum:  100,
			VersionPriority:       100,
		},
	}
	hash, _ := comparison.SetMD5DatadogAgentGenerationAnnotation(&apiService.ObjectMeta, &apiService.Spec)
	return apiService, hash
}

func newAdmissionControllerService(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.Service, string) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getAdmissionControllerServiceName(dda),
			Namespace:   dda.Namespace,
			Labels:      getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda)),
			Annotations: getDefaultAnnotations(dda),
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
					TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultAdmissionControllerTargetPort),
					Port:       datadoghqv1alpha1.DefaultAdmissionControllerServicePort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
	hash, _ := comparison.SetMD5DatadogAgentGenerationAnnotation(&service.ObjectMeta, &service.Spec)

	return service, hash
}
