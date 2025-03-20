// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
)

func isValidSecretConfig(secretConfig *v2alpha1.SecretConfig) bool {
	return secretConfig != nil && secretConfig.SecretName != "" && secretConfig.KeyName != ""
}

func getDCATokenChecksumAnnotationKey() string {
	return object.GetChecksumAnnotationKey("dca-token")
}

func getURLEndpoint(dda *v2alpha1.DatadogAgent) string {
	if dda.Spec.Global.Endpoint != nil && dda.Spec.Global.Endpoint.URL != nil {
		return *dda.Spec.Global.Endpoint.URL
	}
	return ""
}

func setImageRegistry(manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent, componentName v2alpha1.ComponentName) string {
	// Registry is defaulted
	if *dda.Spec.Global.Registry != defaulting.DefaultImageRegistry {
		image := defaulting.DefaultAgentImageName
		version := defaulting.AgentLatestVersion
		if componentName == v2alpha1.ClusterAgentComponentName {
			image = defaulting.DefaultClusterAgentImageName
			version = defaulting.ClusterAgentLatestVersion
		}
		fullImage := fmt.Sprintf("%s/%s:%s", *dda.Spec.Global.Registry, image, version)

		for idx := range manager.PodTemplateSpec().Spec.InitContainers {
			manager.PodTemplateSpec().Spec.InitContainers[idx].Image = fullImage
		}

		for idx := range manager.PodTemplateSpec().Spec.Containers {
			manager.PodTemplateSpec().Spec.Containers[idx].Image = fullImage
		}
	}

	return ""
}
