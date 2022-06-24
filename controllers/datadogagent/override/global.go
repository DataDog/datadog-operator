// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplyGlobalSettings use to apply global setting to a PodTemplateSpec
func ApplyGlobalSettings(manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, componentName v2alpha1.ComponentName) *corev1.PodTemplateSpec {
	config := dda.Spec.Global
	// TODO(operator-ga): implement ApplyGlobalSettings

	// set image registry
	// NetworkPolicy contains the network configuration.
	if config.NetworkPolicy != nil {
		if apiutils.BoolValue(config.NetworkPolicy.Create) {
			switch config.NetworkPolicy.Flavor {
			case v2alpha1.NetworkPolicyFlavorCilium:
				var ddURL string
				var dnsSelectorEndpoints []metav1.LabelSelector
				if config.Endpoint != nil && *config.Endpoint.URL != "" {
					ddURL = *config.Endpoint.URL
				}
				if config.NetworkPolicy.DNSSelectorEndpoints != nil {
					dnsSelectorEndpoints = config.NetworkPolicy.DNSSelectorEndpoints
				}
				resourcesManager.CiliumPolicyManager().SetupCiliumManager(*config.Site, ddURL, v2alpha1.IsHostNetworkEnabled(dda, v2alpha1.ClusterAgentComponentName), dnsSelectorEndpoints)
				_ = resourcesManager.CiliumPolicyManager().BuildCiliumPolicy(dda, componentName)
			}
		}
	}

	return manager.PodTemplateSpec()
}
