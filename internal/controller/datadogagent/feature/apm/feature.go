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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/objects"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

func init() {
	err := feature.Register(feature.APMIDType, buildAPMFeature)
	if err != nil {
		panic(err)
	}
}

func buildAPMFeature(options *feature.Options) feature.Feature {
	apmFeat := &apmFeature{}

	if options != nil {
		apmFeat.logger = options.Logger
	}

	return apmFeat
}

type apmFeature struct {
	hostPortEnabled  bool
	hostPortHostPort int32
	useHostNetwork   bool

	serviceAccountName string

	udsEnabled      bool
	udsHostFilepath string

	owner metav1.Object

	forceEnableLocalService bool
	localServiceName        string

	createKubernetesNetworkPolicy bool
	createCiliumNetworkPolicy     bool

	singleStepInstrumentation *instrumentationConfig

	processCheckRunsInCoreAgent bool

	errorTrackingStandalone bool

	logger logr.Logger
}

type instrumentationConfig struct {
	enabled            bool
	enabledNamespaces  []string
	disabledNamespaces []string
	libVersions        map[string]string
	languageDetection  *languageDetection
	injector           *injector
	targets            []v2alpha1.SSITarget
}

type languageDetection struct {
	enabled bool
}

type injector struct {
	imageTag string
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
	if apmConf.SingleStepInstrumentation != nil && apiutils.BoolValue(apmConf.SingleStepInstrumentation.Enabled) {
		return true
	}

	if apmConf.ErrorTrackingStandalone != nil {
		return apiutils.BoolValue(apmConf.DeepCopy().ErrorTrackingStandalone.Enabled)
	}

	return false
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *apmFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda
	apm := ddaSpec.Features.APM
	if shouldEnableAPM(apm) {
		f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)
		f.useHostNetwork = constants.IsHostNetworkEnabled(ddaSpec, v2alpha1.NodeAgentComponentName)

		// Check if autopilot is enabled and override defaults accordingly
		if experimental.IsAutopilotEnabled(dda) {
			f.hostPortEnabled = true
			f.udsEnabled = false
		} else {
			f.hostPortEnabled = apiutils.BoolValue(apm.HostPortConfig.Enabled)
			f.udsEnabled = apiutils.BoolValue(apm.UnixDomainSocketConfig.Enabled)
		}

		f.hostPortHostPort = *apm.HostPortConfig.Port
		if f.hostPortEnabled {
			if enabled, flavor := constants.IsNetworkPolicyEnabled(ddaSpec); enabled {
				if flavor == v2alpha1.NetworkPolicyFlavorCilium {
					f.createCiliumNetworkPolicy = true
				} else {
					f.createKubernetesNetworkPolicy = true
				}
			}
		}
		f.udsHostFilepath = *apm.UnixDomainSocketConfig.Path

		if ddaSpec.Global.LocalService != nil {
			f.forceEnableLocalService = apiutils.BoolValue(ddaSpec.Global.LocalService.ForceEnableLocalService)
		}
		f.localServiceName = constants.GetLocalAgentServiceName(dda.GetName(), ddaSpec)

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
					apicommon.TraceAgentContainerName,
				},
			},
		}

		if apm.SingleStepInstrumentation != nil &&
			(ddaSpec.Features.AdmissionController != nil && apiutils.BoolValue(ddaSpec.Features.AdmissionController.Enabled)) {
			// TODO: add debug log in case Admission controller is disabled (it's a required feature).
			f.singleStepInstrumentation = &instrumentationConfig{}
			f.singleStepInstrumentation.enabled = apiutils.BoolValue(apm.SingleStepInstrumentation.Enabled)
			f.singleStepInstrumentation.disabledNamespaces = apm.SingleStepInstrumentation.DisabledNamespaces
			f.singleStepInstrumentation.enabledNamespaces = apm.SingleStepInstrumentation.EnabledNamespaces
			f.singleStepInstrumentation.libVersions = apm.SingleStepInstrumentation.LibVersions
			f.singleStepInstrumentation.languageDetection = &languageDetection{enabled: apiutils.BoolValue(ddaSpec.Features.APM.SingleStepInstrumentation.LanguageDetection.Enabled)}
			f.singleStepInstrumentation.injector = &injector{imageTag: apm.SingleStepInstrumentation.Injector.ImageTag}
			if len(apm.SingleStepInstrumentation.Targets) > 0 {
				if supportsInstrumentationTargets(ddaSpec) {
					f.singleStepInstrumentation.targets = apm.SingleStepInstrumentation.Targets
				} else {
					f.logger.Info("Cannot enable instrumentation targets. features.apm.instrumentation.targets requires agent version >= 7.64.0")
				}
			}
			reqComp.ClusterAgent = feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.ClusterAgentContainerName,
				},
			}
		}

		f.processCheckRunsInCoreAgent = featutils.ShouldRunProcessChecksInCoreAgent(ddaSpec)
		if f.shouldEnableLanguageDetection() && !f.processCheckRunsInCoreAgent {
			reqComp.Agent.Containers = append(reqComp.Agent.Containers, apicommon.ProcessAgentContainerName)
		}

		if apm.ErrorTrackingStandalone != nil {
			f.errorTrackingStandalone = apiutils.BoolValue(apm.ErrorTrackingStandalone.Enabled)
		}
	}

	return reqComp
}

func (f *apmFeature) shouldSetCustomInjectorImage() bool {
	return f.singleStepInstrumentation != nil &&
		f.singleStepInstrumentation.enabled &&
		f.singleStepInstrumentation.injector != nil &&
		f.singleStepInstrumentation.injector.imageTag != ""
}

func (f *apmFeature) shouldEnableLanguageDetection() bool {
	return f.singleStepInstrumentation != nil &&
		f.singleStepInstrumentation.enabled &&
		f.singleStepInstrumentation.languageDetection != nil &&
		f.singleStepInstrumentation.languageDetection.enabled
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *apmFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	platformInfo := managers.Store().GetPlatformInfo()
	// agent local service
	if common.ShouldCreateAgentLocalService(platformInfo.GetVersionInfo(), f.forceEnableLocalService) {
		apmPort := &corev1.ServicePort{
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(int(constants.DefaultApmPort)),
			Port:       constants.DefaultApmPort,
			Name:       constants.DefaultApmPortName,
		}
		if f.hostPortEnabled {
			apmPort.Port = f.hostPortHostPort
			apmPort.Name = apmHostPortName
			if f.useHostNetwork {
				apmPort.TargetPort = intstr.FromInt(int(f.hostPortHostPort))
			}
		}

		serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
		if err := managers.ServiceManager().AddService(f.localServiceName, f.owner.GetNamespace(), common.GetAgentLocalServiceSelector(f.owner), []corev1.ServicePort{*apmPort}, &serviceInternalTrafficPolicy); err != nil {
			return err
		}
	}

	// network policies
	if f.hostPortEnabled {
		policyName, podSelector := objects.GetNetworkPolicyMetadata(f.owner, v2alpha1.NodeAgentComponentName)
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

	// rbacs
	if f.shouldEnableLanguageDetection() {
		rbacName := getRBACResourceName(f.owner)
		return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getLanguageDetectionRBACPolicyRules())
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	if f.singleStepInstrumentation != nil {
		if len(f.singleStepInstrumentation.disabledNamespaces) > 0 && len(f.singleStepInstrumentation.enabledNamespaces) > 0 {
			// This configuration is not supported
			return fmt.Errorf("`spec.features.apm.instrumentation.enabledNamespaces` and `spec.features.apm.instrumentation.disabledNamespaces` cannot be set together")
		}
		managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDAPMInstrumentationEnabled,
			Value: apiutils.BoolToString(&f.singleStepInstrumentation.enabled),
		})

		if f.shouldSetCustomInjectorImage() {
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  DDAPMInstrumentationInjectorImageTag,
				Value: f.singleStepInstrumentation.injector.imageTag,
			})
		}

		if f.shouldEnableLanguageDetection() {
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  DDLanguageDetectionEnabled,
				Value: "true",
			})
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  DDLanguageDetectionReportingEnabled,
				Value: "true",
			})
		}

		if len(f.singleStepInstrumentation.disabledNamespaces) > 0 {
			ns, err := json.Marshal(f.singleStepInstrumentation.disabledNamespaces)
			if err != nil {
				return err
			}
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  DDAPMInstrumentationDisabledNamespaces,
				Value: string(ns),
			})
		}
		if len(f.singleStepInstrumentation.enabledNamespaces) > 0 {
			ns, err := json.Marshal(f.singleStepInstrumentation.enabledNamespaces)
			if err != nil {
				return err
			}
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  DDAPMInstrumentationEnabledNamespaces,
				Value: string(ns),
			})
		}
		if f.singleStepInstrumentation.libVersions != nil {
			libs, err := json.Marshal(f.singleStepInstrumentation.libVersions)
			if err != nil {
				return err
			}
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  DDAPMInstrumentationLibVersions,
				Value: string(libs),
			})
		}
		if len(f.singleStepInstrumentation.targets) > 0 {
			js, err := json.Marshal(f.singleStepInstrumentation.targets)
			if err != nil {
				return err
			}
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  DDAPMInstrumentationTargets,
				Value: string(js),
			})
		}

	}
	return nil
}

const minInstrumentationTargetsVersion = "7.64.0-0"

func supportsInstrumentationTargets(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	// Agent version must >= 7.64.0 to run feature in core agent
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.ClusterAgentComponentName]; ok {
		if nodeAgent.Image != nil {
			return utils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), minInstrumentationTargetsVersion)
		}
	}
	return utils.IsAboveMinVersion(images.ClusterAgentLatestVersion, minInstrumentationTargetsVersion)
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.manageNodeAgent(apicommon.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.manageNodeAgent(apicommon.TraceAgentContainerName, managers, provider)
	return nil
}

func (f *apmFeature) manageNodeAgent(agentContainerName apicommon.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {

	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  common.DDAPMEnabled,
		Value: "true",
	})

	// udp
	apmPort := &corev1.ContainerPort{
		Name:          constants.DefaultApmPortName,
		ContainerPort: constants.DefaultApmPort,
		Protocol:      corev1.ProtocolTCP,
	}
	if f.hostPortEnabled {
		apmPort.HostPort = f.hostPortHostPort
		receiverPortEnvVarValue := constants.DefaultApmPort
		// if using host network, host port should be set and needs to match container port
		if f.useHostNetwork {
			apmPort.ContainerPort = f.hostPortHostPort
			receiverPortEnvVarValue = int(f.hostPortHostPort)
		}
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDAPMNonLocalTraffic,
			Value: "true",
		})
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDAPMReceiverPort,
			Value: strconv.Itoa(receiverPortEnvVarValue),
		})
	}
	managers.Port().AddPortToContainer(agentContainerName, apmPort)

	// APM SSI Language Detection
	if f.shouldEnableLanguageDetection() {

		// Enable language detection in core agent
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  DDLanguageDetectionEnabled,
			Value: "true",
		})
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  DDLanguageDetectionReportingEnabled,
			Value: "true",
		})

		// Enable language detection in process agent
		managers.EnvVar().AddEnvVarToContainer(apicommon.ProcessAgentContainerName, &corev1.EnvVar{
			Name:  DDLanguageDetectionEnabled,
			Value: "true",
		})
		managers.EnvVar().AddEnvVarToContainer(apicommon.ProcessAgentContainerName, &corev1.EnvVar{
			Name:  DDLanguageDetectionReportingEnabled,
			Value: "true",
		})

		// Always add this envvar to Core and Process containers
		runInCoreAgentEnvVar := &corev1.EnvVar{
			Name:  common.DDProcessConfigRunInCoreAgent,
			Value: apiutils.BoolToString(&f.processCheckRunsInCoreAgent),
		}
		managers.EnvVar().AddEnvVarToContainer(apicommon.ProcessAgentContainerName, runInCoreAgentEnvVar)
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, runInCoreAgentEnvVar)
	} else {
		// Language Detection reporting is enabled by default in the Agent
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  DDLanguageDetectionReportingEnabled,
			Value: "false",
		})
	}

	// uds
	if f.udsEnabled {
		udsHostFolder := filepath.Dir(f.udsHostFilepath)
		sockName := filepath.Base(f.udsHostFilepath)
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDAPMReceiverSocket,
			Value: filepath.Join(apmSocketVolumeLocalPath, sockName),
		})
		socketVol, socketVolMount := volume.GetVolumes(apmSocketVolumeName, udsHostFolder, apmSocketVolumeLocalPath, false)
		volType := corev1.HostPathDirectoryOrCreate // We need to create the directory on the host if it does not exist.
		socketVol.VolumeSource.HostPath.Type = &volType
		managers.VolumeMount().AddVolumeMountToContainerWithMergeFunc(&socketVolMount, agentContainerName, merger.OverrideCurrentVolumeMountMergeFunction)
		managers.Volume().AddVolume(&socketVol)
	}

	// Error Tracking standalone
	if f.errorTrackingStandalone {
		etStandaloneEnvVar := &corev1.EnvVar{
			Name:  common.DDAPMErrorTrackingStandaloneEnabled,
			Value: "true",
		}
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, etStandaloneEnvVar)
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, etStandaloneEnvVar)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (f *apmFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
