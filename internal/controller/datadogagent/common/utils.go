// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
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

func GetDefaultMetadata(owner metav1.Object, componentKind, instanceName, version string, selector *metav1.LabelSelector) (map[string]string, map[string]string, *metav1.LabelSelector) {
	labels := GetDefaultLabels(owner, componentKind, instanceName, version)
	annotations := object.GetDefaultAnnotations(owner)

	if selector != nil {
		maps.Copy(labels, selector.MatchLabels)
		// if update metadata is disabled, use agent deployment name and component as the selector
	} else if val, ok := owner.GetAnnotations()[apicommon.UpdateMetadataAnnotationKey]; ok && val == "false" {
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				apicommon.AgentDeploymentNameLabelKey:      owner.GetName(),
				apicommon.AgentDeploymentComponentLabelKey: componentKind,
			},
		}
	} else {
		// default to using the k8s instance label as the selector
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				kubernetes.AppKubernetesInstanceLabelKey:   instanceName,
				apicommon.AgentDeploymentComponentLabelKey: componentKind,
			},
		}
	}

	return labels, annotations, selector
}

func GetDefaultLabels(owner metav1.Object, componentKind, instanceName, version string) map[string]string {
	name := constants.GetDDAName(owner)

	labels := object.GetDefaultLabels(owner, instanceName, version)
	labels[apicommon.AgentDeploymentNameLabelKey] = name // Always use DDA name
	labels[apicommon.AgentDeploymentComponentLabelKey] = componentKind
	labels[kubernetes.AppKubernetesComponentLabelKey] = componentKind

	return labels
}

// GetAgentVersion return the Agent version based on the DatadogAgent info
func GetAgentVersion(dda metav1.Object) string {
	// TODO implement this method
	return ""
}

func agentSpec(dda metav1.Object) *v2alpha1.DatadogAgentSpec {
	switch d := dda.(type) {
	case *v2alpha1.DatadogAgent:
		if d != nil {
			return &d.Spec
		}
	case *v1alpha1.DatadogAgentInternal:
		if d != nil {
			return &d.Spec
		}
	}
	return nil
}

// AgentImageConfigForComponent returns the image override for a component.
func AgentImageConfigForComponent(dda metav1.Object, componentName v2alpha1.ComponentName) *v2alpha1.AgentImageConfig {
	return AgentImageConfigForComponentSpec(agentSpec(dda), componentName)
}

// AgentImageConfigForComponentSpec returns the image override for a component in spec.
func AgentImageConfigForComponentSpec(spec *v2alpha1.DatadogAgentSpec, componentName v2alpha1.ComponentName) *v2alpha1.AgentImageConfig {
	if spec != nil {
		if override := spec.Override[componentName]; override != nil {
			return override.Image
		}
	}
	return nil
}

// GetComponentVersion returns the version selected by the DDA spec for a component.
func GetComponentVersion(dda metav1.Object, componentName v2alpha1.ComponentName) string {
	return GetComponentVersionFromSpec(agentSpec(dda), componentName)
}

// GetComponentVersionFromSpec returns the version of the Agent image selected by the spec.
// The experimental.agent.datadoghq.com/image-override-config annotation can replace
// individual Node Agent container images after the component override is applied.
func GetComponentVersionFromSpec(spec *v2alpha1.DatadogAgentSpec, componentName v2alpha1.ComponentName) string {
	return images.FromString(ResolveComponentImage(spec, componentName)).Version()
}

// ResolveComponentImage applies the spec's image override to the bundled image for
// the Node Agent, Cluster Agent, or Cluster Checks Runner.
func ResolveComponentImage(spec *v2alpha1.DatadogAgentSpec, componentName v2alpha1.ComponentName) string {
	image := images.GetLatestAgentImage()
	if componentName == v2alpha1.ClusterAgentComponentName {
		image = images.GetLatestClusterAgentImage()
	}
	return images.OverrideAgentImage(image, AgentImageConfigForComponentSpec(spec, componentName))
}

// GetDefaultSeccompConfigMapName returns the default seccomp configmap name based on the DatadogAgent name
func GetDefaultSeccompConfigMapName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", constants.GetDDAName(dda), SystemProbeAgentSecurityConfigMapSuffixName)
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
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
	}
}

func GetOtelAgentGatewayServiceSelector(dda metav1.Object) map[string]string {
	return map[string]string{
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(dda).String(),
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultOtelAgentGatewayResourceSuffix,
	}
}

// ShouldCreateAgentLocalService returns whether the node agent local service should be created based on the Kubernetes version
func ShouldCreateAgentLocalService(versionInfo *version.Info, forceEnableLocalService bool) bool {
	if versionInfo == nil || versionInfo.GitVersion == "" {
		return false
	}
	// Service Internal Traffic Policy is enabled by default since 1.22
	return utils.IsAboveMinVersion(versionInfo.GitVersion, localServiceDefaultMinimumVersion, nil) || forceEnableLocalService
}

// DefaultSystemProbeCommand returns the command the operator sets on the
// system-probe container by default. A fresh slice is returned on each call so
// callers can compare against it or mutate it freely.
func DefaultSystemProbeCommand() []string {
	return []string{
		SystemProbeBinaryName,
		fmt.Sprintf("--config=%s", SystemProbeConfigFilePath),
	}
}
