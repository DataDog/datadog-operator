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
	"github.com/DataDog/datadog-operator/pkg/images"
)

const (
	workerContainerName = "synthetics-private-location"

	appLabelKey       = "app.kubernetes.io/name"
	instanceLabelKey  = "app.kubernetes.io/instance"
	managedByLabelKey = "app.kubernetes.io/managed-by"

	workerAppLabelValue    = "synthetics-private-location"
	managedByLabelValue    = "datadog-operator"
	configSecretNameSuffix = "-config"

	defaultSite = "datadoghq.com"

	defaultWorkerImageRepo = images.GCRContainerRegistry + "/" + images.DefaultSyntheticsPrivateLocationWorkerImageName
)

func configSecretName(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) string {
	return instance.Name + configSecretNameSuffix
}

// splLabels returns the labels applied to all resources owned by the
// DatadogSyntheticsPrivateLocation instance, which are also the immutable
// Deployment selector. They include the instance name so multiple private
// locations can coexist in a namespace.
func splLabels(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) map[string]string {
	return map[string]string{
		appLabelKey:       workerAppLabelValue,
		instanceLabelKey:  instance.Name,
		managedByLabelKey: managedByLabelValue,
	}
}

// resolveSite returns the site the worker should report to. It is the site of
// the API endpoint that created the private location, so the worker keys are
// valid for it.
func resolveSite(credsManager *config.CredentialManager) string {
	if site := credsManager.Site(); site != "" {
		return strings.TrimPrefix(strings.TrimPrefix(site, "https://"), "http://")
	}
	return defaultSite
}

func workerImage(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) (string, corev1.PullPolicy) {
	repository := defaultWorkerImageRepo
	pullPolicy := corev1.PullIfNotPresent
	if w := instance.Spec.Worker; w != nil && w.Image != nil {
		if w.Image.Repository != "" {
			repository = w.Image.Repository
		}
		if w.Image.PullPolicy != "" {
			pullPolicy = w.Image.PullPolicy
		}
	}
	return repository + ":" + workerImageTag(instance), pullPolicy
}

func workerImageTag(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) string {
	if w := instance.Spec.Worker; w != nil && w.Image != nil && w.Image.Tag != "" {
		return w.Image.Tag
	}
	return images.SyntheticsPrivateLocationWorkerLatestVersion
}
