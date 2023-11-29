// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/merger"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
)

func init() {
	err := feature.Register(feature.APMIDType, buildAPMFeature)
	if err != nil {
		panic(err)
	}
}

func buildAPMFeature(options *feature.Options) feature.Feature {
	apmFeat := &apmFeature{}

	return apmFeat
}

type apmFeature struct {
	hostPortEnabled  bool
	hostPortHostPort int32
	useHostNetwork   bool

	udsEnabled      bool
	udsHostFilepath string

	owner metav1.Object

	forceEnableLocalService bool
	localServiceName        string

	createKubernetesNetworkPolicy bool
	createCiliumNetworkPolicy     bool

	singleStepInstrumentation *instrumentationConfig
}

type instrumentationConfig struct {
	enabled            bool
	enabledNamespaces  []string
	disabledNamespaces []string
	libVersions        map[string]string
}

// ID returns the ID of the Feature
func (f *apmFeature) ID() feature.IDType {
	return feature.APMIDType
}

func shouldEnableAPM(apmConf *v2alpha1.APMFeatureConfig) bool {
	if apmConf == nil {
		return false
	}

	if apmConf.Enabled != nil {
		return apiutils.BoolValue(apmConf.Enabled)
	}

	// SingleStepInstrumentation requires APM Enabled
	if apmConf.SingleStepInstrumentation != nil &&
		(apiutils.BoolValue(apmConf.SingleStepInstrumentation.Enabled) || len(apmConf.SingleStepInstrumentation.EnabledNamespaces) > 0) {
		return true
	}

	return false
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *apmFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	apm := dda.Spec.Features.APM
	if shouldEnableAPM(apm) {
		f.useHostNetwork = v2alpha1.IsHostNetworkEnabled(dda, v2alpha1.NodeAgentComponentName)
		// hostPort defaults to 'false' in the defaulting code
		f.hostPortEnabled = apiutils.BoolValue(apm.HostPortConfig.Enabled)
		f.hostPortHostPort = *apm.HostPortConfig.Port
		if f.hostPortEnabled {
			if enabled, flavor := v2alpha1.IsNetworkPolicyEnabled(dda); enabled {
				if flavor == v2alpha1.NetworkPolicyFlavorCilium {
					f.createCiliumNetworkPolicy = true
				} else {
					f.createKubernetesNetworkPolicy = true
				}
			}
		}
		// UDS defaults to 'true' in the defaulting code
		f.udsEnabled = apiutils.BoolValue(apm.UnixDomainSocketConfig.Enabled)
		f.udsHostFilepath = *apm.UnixDomainSocketConfig.Path

		if dda.Spec.Global.LocalService != nil {
			f.forceEnableLocalService = apiutils.BoolValue(dda.Spec.Global.LocalService.ForceEnableLocalService)
		}
		f.localServiceName = v2alpha1.GetLocalAgentServiceName(dda)

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
					apicommonv1.TraceAgentContainerName,
				},
			},
		}
		if apm.SingleStepInstrumentation != nil &&
			(dda.Spec.Features.AdmissionController != nil && apiutils.BoolValue(dda.Spec.Features.AdmissionController.Enabled)) {
			// add debug log in case Admission controller is disabled (it's a required feature).
			f.singleStepInstrumentation = &instrumentationConfig{}
			f.singleStepInstrumentation.enabled = apiutils.BoolValue(apm.SingleStepInstrumentation.Enabled)
			f.singleStepInstrumentation.disabledNamespaces = apm.SingleStepInstrumentation.DisabledNamespaces
			f.singleStepInstrumentation.enabledNamespaces = apm.SingleStepInstrumentation.EnabledNamespaces
			f.singleStepInstrumentation.libVersions = apm.SingleStepInstrumentation.LibVersions
			reqComp.ClusterAgent = feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.ClusterAgentContainerName,
				},
			}
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *apmFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	apm := dda.Spec.Agent.Apm
	if apiutils.BoolValue(apm.Enabled) {
		f.hostPortEnabled = true
		f.hostPortHostPort = *apm.HostPort
		f.useHostNetwork = v1alpha1.IsHostNetworkEnabled(dda)
		if apiutils.BoolValue(apm.UnixDomainSocket.Enabled) {
			f.udsEnabled = true
			if apm.UnixDomainSocket.HostFilepath != nil {
				f.udsHostFilepath = *apm.UnixDomainSocket.HostFilepath
			}
		}

		if dda.Spec.Agent.LocalService != nil {
			f.forceEnableLocalService = apiutils.BoolValue(dda.Spec.Agent.LocalService.ForceLocalServiceEnable)
		}
		f.localServiceName = v1alpha1.GetLocalAgentServiceName(dda)

		if enabled, flavor := v1alpha1.IsAgentNetworkPolicyEnabled(dda); enabled {
			if flavor == v1alpha1.NetworkPolicyFlavorCilium {
				f.createCiliumNetworkPolicy = true
			} else {
				f.createKubernetesNetworkPolicy = true
			}
		}

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
					apicommonv1.TraceAgentContainerName,
				},
			},
		}
	}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *apmFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	// agent local service
	if component.ShouldCreateAgentLocalService(managers.Store().GetVersionInfo(), f.forceEnableLocalService) {
		apmPort := &corev1.ServicePort{
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(int(apicommon.DefaultApmPort)),
			Port:       apicommon.DefaultApmPort,
			Name:       apicommon.DefaultApmPortName,
		}
		if f.hostPortEnabled {
			apmPort.Port = f.hostPortHostPort
			apmPort.Name = apicommon.APMHostPortName
			if f.useHostNetwork {
				apmPort.TargetPort = intstr.FromInt(int(f.hostPortHostPort))
			}
		}

		serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
		if err := managers.ServiceManager().AddService(f.localServiceName, f.owner.GetNamespace(), component.GetAgentLocalServiceSelector(f.owner), []corev1.ServicePort{*apmPort}, &serviceInternalTrafficPolicy); err != nil {
			return err
		}
	}

	// network policies
	if f.hostPortEnabled {
		policyName, podSelector := component.GetNetworkPolicyMetadata(f.owner, v2alpha1.NodeAgentComponentName)
		if f.createKubernetesNetworkPolicy {
			protocolTCP := corev1.ProtocolTCP
			ingressRules := []netv1.NetworkPolicyIngressRule{
				{
					Ports: []netv1.NetworkPolicyPort{
						{
							Port: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: f.hostPortHostPort,
							},
							Protocol: &protocolTCP,
						},
					},
				},
			}
			return managers.NetworkPolicyManager().AddKubernetesNetworkPolicy(
				policyName,
				f.owner.GetNamespace(),
				podSelector,
				nil,
				ingressRules,
				nil,
			)
		} else if f.createCiliumNetworkPolicy {
			policySpecs := []cilium.NetworkPolicySpec{
				{
					Description:      "Ingress for APM trace",
					EndpointSelector: podSelector,
					Ingress: []cilium.IngressRule{
						{
							FromEndpoints: []metav1.LabelSelector{
								{},
							},
							ToPorts: []cilium.PortRule{
								{
									Ports: []cilium.PortProtocol{
										{
											Port:     strconv.Itoa(int(f.hostPortHostPort)),
											Protocol: cilium.ProtocolTCP,
										},
									},
								},
							},
						},
					},
				},
			}
			return managers.CiliumPolicyManager().AddCiliumPolicy(policyName, f.owner.GetNamespace(), policySpecs)
		}
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	if f.singleStepInstrumentation != nil {
		if len(f.singleStepInstrumentation.disabledNamespaces) > 0 && len(f.singleStepInstrumentation.enabledNamespaces) > 0 {
			// This configuration is not supported
			return fmt.Errorf("`enabledNamespaces` and `disabledNamespaces` cannot be set together")
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAPMInstrumentationEnabled,
			Value: apiutils.BoolToString(&f.singleStepInstrumentation.enabled),
		})

		if len(f.singleStepInstrumentation.disabledNamespaces) > 0 {
			ns, err := json.Marshal(f.singleStepInstrumentation.disabledNamespaces)
			if err != nil {
				return err
			}
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAPMInstrumentationDisabledNamespaces,
				Value: string(ns),
			})
		}
		if len(f.singleStepInstrumentation.enabledNamespaces) > 0 {
			ns, err := json.Marshal(f.singleStepInstrumentation.enabledNamespaces)
			if err != nil {
				return err
			}
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAPMInstrumentationEnabledNamespaces,
				Value: string(ns),
			})
		}
		if f.singleStepInstrumentation.libVersions != nil {
			libs, err := json.Marshal(f.singleStepInstrumentation.libVersions)
			if err != nil {
				return err
			}
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAPMInstrumentationLibVersions,
				Value: string(libs),
			})
		}

	}
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.TraceAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDAPMEnabled,
		Value: "true",
	})

	// udp
	apmPort := &corev1.ContainerPort{
		Name:          apicommon.DefaultApmPortName,
		ContainerPort: apicommon.DefaultApmPort,
		Protocol:      corev1.ProtocolTCP,
	}
	if f.hostPortEnabled {
		apmPort.HostPort = f.hostPortHostPort
		// if using host network, host port should be set and needs to match container port
		if f.useHostNetwork {
			apmPort.ContainerPort = f.hostPortHostPort
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.TraceAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAPMReceiverPort,
			Value: strconv.FormatInt(int64(f.hostPortHostPort), 10),
		})
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.TraceAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAPMNonLocalTraffic,
			Value: "true",
		})
	}
	managers.Port().AddPortToContainer(apicommonv1.TraceAgentContainerName, apmPort)

	// uds
	if f.udsEnabled {
		udsHostFolder := filepath.Dir(f.udsHostFilepath)
		sockName := filepath.Base(f.udsHostFilepath)
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.TraceAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAPMReceiverSocket,
			Value: filepath.Join(apicommon.APMSocketVolumeLocalPath, sockName),
		})
		socketVol, socketVolMount := volume.GetVolumes(apicommon.APMSocketVolumeName, udsHostFolder, apicommon.APMSocketVolumeLocalPath, false)
		volType := corev1.HostPathDirectoryOrCreate // We need to create the directory on the host if it does not exist.
		socketVol.VolumeSource.HostPath.Type = &volType
		managers.VolumeMount().AddVolumeMountToContainerWithMergeFunc(&socketVolMount, apicommonv1.TraceAgentContainerName, merger.OverrideCurrentVolumeMountMergeFunction)
		managers.Volume().AddVolume(&socketVol)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
