// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"strings"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

// NewDeployment use to generate the skeleton of a new deployment based on few information
func NewDeployment(owner metav1.Object, componentKind, componentName, version string, inputSelector *metav1.LabelSelector) *appsv1.Deployment {
	labels, annotations, selector := GetDefaultMetadata(owner, componentKind, componentName, version, inputSelector)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentName,
			Namespace:   owner.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: selector,
		},
	}

	return deployment
}

func GetDefaultMetadata(owner metav1.Object, componentKind, componentName, version string, selector *metav1.LabelSelector) (map[string]string, map[string]string, *metav1.LabelSelector) {
	labels := GetDefaultLabels(owner, componentKind, componentName, version)
	annotations := object.GetDefaultAnnotations(owner)

	if selector != nil {
		for key, val := range selector.MatchLabels {
			labels[key] = val
		}
	} else {
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				apicommon.AgentDeploymentNameLabelKey:      owner.GetName(),
				apicommon.AgentDeploymentComponentLabelKey: componentKind,
			},
		}
	}
	return labels, annotations, selector
}

func GetDefaultLabels(owner metav1.Object, componentKind, componentName, version string) map[string]string {
	labels := object.GetDefaultLabels(owner, componentName, version)
	labels[apicommon.AgentDeploymentNameLabelKey] = owner.GetName()
	labels[apicommon.AgentDeploymentComponentLabelKey] = componentKind
	labels[kubernetes.AppKubernetesComponentLabelKey] = componentKind

	return labels
}

// GetAgentVersion return the Agent version based on the DatadogAgent info
func GetAgentVersion(dda metav1.Object) string {
	// TODO implement this method
	return ""
}

// GetDefaultSeccompConfigMapName returns the default seccomp configmap name based on the DatadogAgent name
func GetDefaultSeccompConfigMapName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), v2alpha1.SystemProbeAgentSecurityConfigMapSuffixName)
}

// GetAgentVersionFromImage returns the Agent version based on the AgentImageConfig
func GetAgentVersionFromImage(imageConfig v2alpha1.AgentImageConfig) string {
	version := ""
	if imageConfig.Name != "" {
		version = strings.TrimSuffix(utils.GetTagFromImageName(imageConfig.Name), "-jmx")
	}
	// Give priority to image Tag setting
	if imageConfig.Tag != "" {
		version = imageConfig.Tag
	}
	return version
}

// BuildEnvVarFromSource return an *corev1.EnvVar from a Env Var name and *corev1.EnvVarSource
func BuildEnvVarFromSource(name string, source *corev1.EnvVarSource) *corev1.EnvVar {
	return &corev1.EnvVar{
		Name:      name,
		ValueFrom: source,
	}
}

// BuildEnvVarFromSecret return an corev1.EnvVarSource correspond to a secret reference
func BuildEnvVarFromSecret(name, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: name,
			},
			Key: key,
		},
	}
}

const (
	localServiceDefaultMinimumVersion = "1.22-0"
)

// GetAgentLocalServiceSelector creates the selector to be used for the agent local service
func GetAgentLocalServiceSelector(dda metav1.Object) map[string]string {
	return map[string]string{
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(dda).String(),
		apicommon.AgentDeploymentComponentLabelKey: v2alpha1.DefaultAgentResourceSuffix,
	}
}

// ShouldCreateAgentLocalService returns whether the node agent local service should be created based on the Kubernetes version
func ShouldCreateAgentLocalService(versionInfo *version.Info, forceEnableLocalService bool) bool {
	if versionInfo == nil || versionInfo.GitVersion == "" {
		return false
	}
	// Service Internal Traffic Policy is enabled by default since 1.22
	return utils.IsAboveMinVersion(versionInfo.GitVersion, localServiceDefaultMinimumVersion) || forceEnableLocalService
}
