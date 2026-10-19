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
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

// EnableJemallocAnnotation temporarily gates jemalloc defaults during rollout.
const EnableJemallocAnnotation = "agent.datadoghq.com/jemalloc-enabled"

// JemallocEnvVars returns opt-in allocator defaults for supported Agent images.
func JemallocEnvVars(dda metav1.Object, image *v2alpha1.AgentImageConfig) []corev1.EnvVar {
	if dda.GetAnnotations()[EnableJemallocAnnotation] != "true" {
		return nil
	}
	if dda.GetAnnotations()[kubernetes.ProviderAnnotationKey] == kubernetes.GKEAutopilotProvider {
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
	if !utils.IsAboveMinVersion(version, "7.84.0-0", new(true)) {
		return nil
	}
	return []corev1.EnvVar{
		// Pod environment variables replace image defaults, so retain the Agent image's nosys shim.
		{Name: "LD_PRELOAD", Value: "/opt/lib/nosys.so /opt/datadog-agent/embedded/lib/libjemalloc.so"},
		{Name: "MALLOC_CONF", Value: "background_thread:true,dirty_decay_ms:5000"},
	}
}
