// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/secrets"
	"github.com/DataDog/datadog-operator/pkg/version"
)

func IsValidSecretConfig(secretConfig *v2alpha1.SecretConfig) bool {
	return secretConfig != nil && secretConfig.SecretName != "" && secretConfig.KeyName != ""
}

func GetDCATokenChecksumAnnotationKey() string {
	return object.GetChecksumAnnotationKey("dca-token")
}

func getURLEndpoint(ddaSpec *v2alpha1.DatadogAgentSpec) string {
	if ddaSpec.Global.Endpoint != nil && ddaSpec.Global.Endpoint.URL != nil {
		return *ddaSpec.Global.Endpoint.URL
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

func useSystemProbeCustomSeccomp(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	if componentOverride, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if container, ok := componentOverride.Containers[apicommon.SystemProbeContainerName]; ok {
			// Only ConfigMap is supported for now
			if container.SeccompConfig != nil && container.SeccompConfig.CustomProfile != nil && container.SeccompConfig.CustomProfile.ConfigMap != nil {
				return true
			}
		}
	}
	return false
}

func SetGlobalFromDDA(dda *v2alpha1.DatadogAgent, ddaiGlobal *v2alpha1.GlobalConfig) {
	setCredentialsFromDDA(dda, ddaiGlobal)
	setDCATokenFromDDA(dda, ddaiGlobal)
}

// setCredentialsFromDDA sets credentials in the DDAI based on the DDA
// ddaiGlobal is copied from the DDA's global config
func setCredentialsFromDDA(dda metav1.Object, ddaiGlobal *v2alpha1.GlobalConfig) {
	// Use new credentials struct so we don't keep plain text credentials from the DDA
	newCredentials := &v2alpha1.DatadogCredentials{
		APISecret: &v2alpha1.SecretConfig{
			SecretName: secrets.GetDefaultCredentialsSecretName(dda),
			KeyName:    v2alpha1.DefaultAPIKeyKey,
		},
	}
	// Prioritize existing secret
	if IsValidSecretConfig(ddaiGlobal.Credentials.APISecret) {
		newCredentials.APISecret = ddaiGlobal.Credentials.APISecret
	}

	// App key is optional so it may not be set
	if ddaiGlobal.Credentials.AppSecret != nil || ddaiGlobal.Credentials.AppKey != nil {
		newCredentials.AppSecret = &v2alpha1.SecretConfig{
			SecretName: secrets.GetDefaultCredentialsSecretName(dda),
			KeyName:    v2alpha1.DefaultAPPKeyKey,
		}
		// Prioritize existing secret
		if IsValidSecretConfig(ddaiGlobal.Credentials.AppSecret) {
			newCredentials.AppSecret = ddaiGlobal.Credentials.AppSecret
		}
	}
	ddaiGlobal.Credentials = newCredentials
}

func setDCATokenFromDDA(dda metav1.Object, ddaiGlobal *v2alpha1.GlobalConfig) {
	// Use existing ClusterAgentTokenSecret if already set
	if IsValidSecretConfig(ddaiGlobal.ClusterAgentTokenSecret) {
		return
	}

	// Otherwise use default secret name and key since a token will be generated in the global dependencies
	ddaiGlobal.ClusterAgentTokenSecret = &v2alpha1.SecretConfig{
		SecretName: secrets.GetDefaultDCATokenSecretName(dda),
		KeyName:    common.DefaultTokenKey,
	}
}
