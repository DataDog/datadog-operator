// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/version"
)

func isValidSecretConfig(secretConfig *v2alpha1.SecretConfig) bool {
	if secretConfig == nil {
		return false
	}
	if secretConfig.SecretName != "" || secretConfig.KeyName == "" {
		return false
	}

	return true
}

func buildInstallInfoConfigMap(dda metav1.Object) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.GetInstallInfoConfigMapName(dda),
			Namespace: dda.GetNamespace(),
		},
		Data: map[string]string{
			"install_info": getInstallInfoValue(),
		},
	}

	return configMap
}

func getInstallInfoValue() string {
	toolVersion := "unknown"
	if envVar := os.Getenv(DDInstallInfoToolVersion); envVar != "" {
		toolVersion = envVar
	}

	return fmt.Sprintf(installInfoDataTmpl, toolVersion, version.Version)
}

const installInfoDataTmpl = `---
install_method:
  tool: datadog-operator
  tool_version: %s
  installer_version: %s
`
