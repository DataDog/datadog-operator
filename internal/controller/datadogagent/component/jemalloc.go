// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package component

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

// DisableJemallocAnnotation disables the Agent image's default jemalloc preload.
const DisableJemallocAnnotation = "agent.datadoghq.com/jemalloc-disable"

// JemallocEnvVars returns the jemalloc opt-out override for supported Agent images.
func JemallocEnvVars(dda metav1.Object, image *v2alpha1.AgentImageConfig) []corev1.EnvVar {
	if dda.GetAnnotations()[DisableJemallocAnnotation] != "true" {
		return nil
	}
	if experimental.IsAutopilotEnabled(dda) {
		return nil
	}
	// Windows profiles are transformed after defaults are built.
	if dda.GetLabels()[constants.ProfileLabelKey] != "" && dda.GetAnnotations()[kubernetes.ProviderAnnotationKey] == kubernetes.WindowsProvider {
		return nil
	}
	version := images.AgentLatestVersion
	if image != nil {
		version = common.GetAgentVersionFromImage(*image)
	}
	if !utils.IsAboveMinVersion(version, "7.87.0-0", new(true)) {
		return nil
	}
	return []corev1.EnvVar{
		// Pod environment variables replace image defaults, so retain the Agent image's nosys shim.
		{Name: "LD_PRELOAD", Value: "/opt/lib/nosys.so"},
	}
}
