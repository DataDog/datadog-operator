// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadcaptureproxy

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/images"
)

func init() {
	if err := feature.Register(feature.WorkloadCaptureProxyIDType, buildFeature); err != nil {
		panic(err)
	}
}

func buildFeature(*feature.Options) feature.Feature {
	return &workloadCaptureProxyFeature{}
}

type workloadCaptureProxyFeature struct {
	customImage    *v2alpha1.AgentImageConfig
	globalRegistry string
	owner          metav1.Object
	adpEnabled     bool
}

// ID returns the ID of the Feature
func (f *workloadCaptureProxyFeature) ID() feature.IDType {
	return feature.WorkloadCaptureProxyIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *workloadCaptureProxyFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if ddaSpec.Features == nil || ddaSpec.Features.WorkloadCaptureProxy == nil || !apiutils.BoolValue(ddaSpec.Features.WorkloadCaptureProxy.Enabled) {
		return reqComp
	}

	// Detect if Agent Data Plane is enabled - this affects which container
	// should consume the proxy's output socket
	f.adpEnabled = featureutils.HasAgentDataPlaneAnnotation(dda)

	// Store custom image config and registry if provided
	f.customImage = ddaSpec.Features.WorkloadCaptureProxy.Image
	if ddaSpec.Global != nil && ddaSpec.Global.Registry != nil {
		f.globalRegistry = *ddaSpec.Global.Registry
	}

	reqComp = feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.CoreAgentContainerName,
				apicommon.WorkloadCaptureProxyContainerName,
			},
		},
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
func (f *workloadCaptureProxyFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *workloadCaptureProxyFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for single container mode
func (f *workloadCaptureProxyFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Not supported in single container mode
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
func (f *workloadCaptureProxyFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Override proxy container image if custom image specified
	if f.customImage != nil {
		for i, container := range managers.PodTemplateSpec().Spec.Containers {
			if container.Name == string(apicommon.WorkloadCaptureProxyContainerName) {
				customImagePath := images.AssembleImage(f.customImage, f.globalRegistry)
				managers.PodTemplateSpec().Spec.Containers[i].Image = customImagePath
			}
		}
	}

	// Configure the downstream consumer (core-agent OR agent-data-plane) to read from
	// the proxy's output socket instead of the standard socket.
	//
	// When ADP is enabled, the dogstatsd feature configures ADP to handle dogstatsd,
	// so we redirect ADP's socket. Otherwise, we redirect the core agent's socket.
	//
	// The workload-capture-proxy always:
	//   1. Listens on the standard socket (dsd.socket) - apps send here
	//   2. Outputs to the alternate socket (dsd-agent-input.sock)
	//   3. The downstream consumer reads from the alternate socket
	//
	// This creates the data flow:
	//   Apps → dsd.socket → Workload Capture Proxy → dsd-agent-input.sock → (Core Agent OR ADP)
	alternateSocketPath := filepath.Join(common.DogstatsdSocketLocalPath, common.DogstatsdAlternateSocketName)

	if f.adpEnabled {
		// When ADP is enabled, configure ADP to read from the alternate socket
		managers.EnvVar().AddEnvVarToContainer(apicommon.AgentDataPlaneContainerName, &corev1.EnvVar{
			Name:  "DD_DOGSTATSD_SOCKET",
			Value: alternateSocketPath,
		})
	} else {
		// When ADP is NOT enabled, configure core agent to read from the alternate socket
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  "DD_DOGSTATSD_SOCKET",
			Value: alternateSocketPath,
		})
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
func (f *workloadCaptureProxyFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
