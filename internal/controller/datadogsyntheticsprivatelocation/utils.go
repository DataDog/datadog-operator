// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
)

const (
	workerContainerName = "synthetics-private-location"

	appLabelKey       = "app.kubernetes.io/name"
	instanceLabelKey  = "app.kubernetes.io/instance"
	managedByLabelKey = "app.kubernetes.io/managed-by"

	workerAppLabelValue    = "synthetics-private-location"
	managedByLabelValue    = "datadog-operator"
	configSecretNameSuffix = "-config"

	defaultSite            = "datadoghq.com"
	defaultWorkerImageRepo = "gcr.io/datadoghq/synthetics-private-location-worker"
	defaultWorkerImageTag  = "1.73.0"
)

func configSecretName(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) string {
	return instance.Name + configSecretNameSuffix
}

// selectorLabels returns the immutable Deployment selector labels. They include
// the instance name so multiple private locations can coexist in a namespace.
func selectorLabels(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) map[string]string {
	return map[string]string{
		appLabelKey:       workerAppLabelValue,
		instanceLabelKey:  instance.Name,
		managedByLabelKey: managedByLabelValue,
	}
}

// splLabels returns the labels applied to all resources owned by the
// DatadogSyntheticsPrivateLocation instance. spec.worker.commonLabels can add
// labels but never override operator-owned keys.
func splLabels(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) map[string]string {
	labels := selectorLabels(instance)
	if instance.Spec.Worker != nil {
		for k, v := range instance.Spec.Worker.CommonLabels {
			if _, exists := labels[k]; !exists {
				labels[k] = v
			}
		}
	}
	return labels
}

func podLabels(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) map[string]string {
	labels := splLabels(instance)
	if instance.Spec.Worker != nil {
		for k, v := range instance.Spec.Worker.PodLabels {
			if _, exists := labels[k]; !exists {
				labels[k] = v
			}
		}
	}
	return labels
}

// resolveSite returns the site the worker should report to, derived from the
// operator credentials (DD_SITE / DD_URL) rather than the API response.
func resolveSite(creds config.Creds) string {
	if creds.Site != nil {
		return strings.TrimPrefix(strings.TrimPrefix(*creds.Site, "https://"), "http://")
	}
	return defaultSite
}

func workerImage(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) (string, corev1.PullPolicy) {
	repository := defaultWorkerImageRepo
	tag := defaultWorkerImageTag
	pullPolicy := corev1.PullIfNotPresent
	if w := instance.Spec.Worker; w != nil && w.Image != nil {
		if w.Image.Repository != "" {
			repository = w.Image.Repository
		}
		if w.Image.Tag != "" {
			tag = w.Image.Tag
		}
		if w.Image.PullPolicy != "" {
			pullPolicy = w.Image.PullPolicy
		}
	}
	return repository + ":" + tag, pullPolicy
}
