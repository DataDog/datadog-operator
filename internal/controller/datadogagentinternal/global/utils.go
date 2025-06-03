// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"
	"os"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/version"
)

func isValidSecretConfig(secretConfig *v2alpha1.SecretConfig) bool {
	return secretConfig != nil && secretConfig.SecretName != "" && secretConfig.KeyName != ""
}

func getDCATokenChecksumAnnotationKey() string {
	return object.GetChecksumAnnotationKey("dca-token")
}

func getURLEndpoint(dda *v1alpha1.DatadogAgentInternal) string {
	if dda.Spec.Global.Endpoint != nil && dda.Spec.Global.Endpoint.URL != nil {
		return *dda.Spec.Global.Endpoint.URL
	}
	return ""
}

func getInstallInfoValue() string {
	toolVersion := "unknown"
	if envVar := os.Getenv(InstallInfoToolVersion); envVar != "" {
		toolVersion = envVar
	}

	return fmt.Sprintf(installInfoDataTmpl, toolVersion, version.Version)
}

func useSystemProbeCustomSeccomp(dda *v1alpha1.DatadogAgentInternal) bool {
	if componentOverride, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if container, ok := componentOverride.Containers[apicommon.SystemProbeContainerName]; ok {
			// Only ConfigMap is supported for now
			if container.SeccompConfig != nil && container.SeccompConfig.CustomProfile != nil && container.SeccompConfig.CustomProfile.ConfigMap != nil {
				return true
			}
		}
	}
	return false
}
