// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package otlp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/objects"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

var (
	portRegexp = regexp.MustCompile(":([0-9]+)$")
)

func init() {
	err := feature.Register(feature.OTLPIDType, buildOTLPFeature)
	if err != nil {
		panic(err)
	}
}

func buildOTLPFeature(options *feature.Options) feature.Feature {
	otlpFeat := &otlpFeature{logger: options.Logger}

	return otlpFeat
}

type otlpFeature struct {
	logger logr.Logger

	grpcEnabled         bool
	grpcHostPortEnabled bool
	grpcCustomHostPort  int32
	grpcEndpoint        string

	httpEnabled         bool
	httpHostPortEnabled bool
	httpCustomHostPort  int32
	httpEndpoint        string

	usingAPM bool

	forceEnableLocalService bool
	localServiceName        string

	createKubernetesNetworkPolicy bool
	createCiliumNetworkPolicy     bool

	owner metav1.Object
}

// ID returns the ID of the Feature
func (f *otlpFeature) ID() feature.IDType {
	return feature.OTLPIDType
}

// Configure is used to configure the feature from a v1alpha1.DatadogAgentInternal instance.
func (f *otlpFeature) Configure(ddai *v1alpha1.DatadogAgentInternal) (reqComp feature.RequiredComponents) {
	otlp := ddai.Spec.Features.OTLP
	f.owner = ddai
	if apiutils.BoolValue(otlp.Receiver.Protocols.GRPC.Enabled) {
		f.grpcEnabled = true
	}
	if otlp.Receiver.Protocols.GRPC.HostPortConfig != nil {
		f.grpcHostPortEnabled = apiutils.BoolValue(otlp.Receiver.Protocols.GRPC.HostPortConfig.Enabled)
		if otlp.Receiver.Protocols.GRPC.HostPortConfig.Port != nil {
			f.grpcCustomHostPort = *otlp.Receiver.Protocols.GRPC.HostPortConfig.Port
		}
	}
	if otlp.Receiver.Protocols.GRPC.Endpoint != nil {
		f.grpcEndpoint = *otlp.Receiver.Protocols.GRPC.Endpoint
	}

	if apiutils.BoolValue(otlp.Receiver.Protocols.HTTP.Enabled) {
		f.httpEnabled = true
	}
	if otlp.Receiver.Protocols.HTTP.HostPortConfig != nil {
		f.httpHostPortEnabled = apiutils.BoolValue(otlp.Receiver.Protocols.HTTP.HostPortConfig.Enabled)
		if otlp.Receiver.Protocols.HTTP.HostPortConfig.Port != nil {
			f.httpCustomHostPort = *otlp.Receiver.Protocols.HTTP.HostPortConfig.Port
		}
	}
	if otlp.Receiver.Protocols.HTTP.Endpoint != nil {
		f.httpEndpoint = *otlp.Receiver.Protocols.HTTP.Endpoint
	}

	apm := ddai.Spec.Features.APM
	if apm != nil {
		f.usingAPM = apiutils.BoolValue(apm.Enabled)
	}

	if ddai.Spec.Global.LocalService != nil {
		f.forceEnableLocalService = apiutils.BoolValue(ddai.Spec.Global.LocalService.ForceEnableLocalService)
	}
	f.localServiceName = constants.GetLocalAgentServiceNameDDAI(ddai)

	if f.grpcEnabled || f.httpEnabled {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
				},
			},
		}
		// if using APM, require the Trace Agent too.
		if f.usingAPM {
			reqComp.Agent.Containers = append(reqComp.Agent.Containers, apicommon.TraceAgentContainerName)
		}
	}
	if f.grpcEnabled || f.httpEnabled {
		if enabled, flavor := constants.IsNetworkPolicyEnabledDDAI(ddai); enabled {
			if flavor == v2alpha1.NetworkPolicyFlavorCilium {
				f.createCiliumNetworkPolicy = true
			} else {
				f.createKubernetesNetworkPolicy = true
			}
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *otlpFeature) ManageDependencies(managers feature.ResourceManagers) error {
	platformInfo := managers.Store().GetPlatformInfo()
	versionInfo := platformInfo.GetVersionInfo()

	if f.grpcEnabled {
		port, err := extractPortEndpoint(f.grpcEndpoint)
		if err != nil {
			f.logger.Error(err, "failed to extract port from OTLP/gRPC endpoint")
			return fmt.Errorf("failed to extract port from OTLP/gRPC endpoint: %w", err)
		}
		if f.grpcHostPortEnabled && f.grpcCustomHostPort != 0 {
			port = f.grpcCustomHostPort
		}
		if common.ShouldCreateAgentLocalService(versionInfo, f.forceEnableLocalService) {
			servicePort := []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(int(port)),
					Port:       port,
					Name:       otlpGRPCPortName,
				},
			}
			serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
			if err := managers.ServiceManager().AddService(f.localServiceName, f.owner.GetNamespace(), common.GetAgentLocalServiceSelector(f.owner), servicePort, &serviceInternalTrafficPolicy); err != nil {
				return err
			}
		}
		//network policies for gRPC OTLP
		policyName, podSelector := objects.GetNetworkPolicyMetadata(f.owner, v2alpha1.NodeAgentComponentName)
		if f.createKubernetesNetworkPolicy {
			protocolTCP := corev1.ProtocolTCP
			ingressRules := []netv1.NetworkPolicyIngressRule{
				{
					Ports: []netv1.NetworkPolicyPort{
						{
							Port: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: port,
							},
							Protocol: &protocolTCP,
						},
					},
				},
			}
			if err := managers.NetworkPolicyManager().AddKubernetesNetworkPolicy(
				policyName,
				f.owner.GetNamespace(),
				podSelector,
				nil,
				ingressRules,
				nil,
			); err != nil {
				return err
			}
		} else if f.createCiliumNetworkPolicy {
			policySpecs := []cilium.NetworkPolicySpec{
				{
					Description:      "Ingress for gRPC OTLP",
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
											Port:     strconv.Itoa(int(port)),
											Protocol: cilium.ProtocolTCP,
										},
									},
								},
							},
						},
					},
				},
			}
			if err := managers.CiliumPolicyManager().AddCiliumPolicy(policyName, f.owner.GetNamespace(), policySpecs); err != nil {
				return err
			}
		}
	}
	if f.httpEnabled {
		port, err := extractPortEndpoint(f.httpEndpoint)
		if err != nil {
			f.logger.Error(err, "failed to extract port from OTLP/HTTP endpoint")
			return fmt.Errorf("failed to extract port from OTLP/HTTP endpoint: %w", err)
		}
		if f.httpHostPortEnabled && f.httpCustomHostPort != 0 {
			port = f.httpCustomHostPort
		}
		if common.ShouldCreateAgentLocalService(versionInfo, f.forceEnableLocalService) {
			servicePort := []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(int(port)),
					Port:       port,
					Name:       otlpHTTPPortName,
				},
			}
			serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
			if err := managers.ServiceManager().AddService(f.localServiceName, f.owner.GetNamespace(), nil, servicePort, &serviceInternalTrafficPolicy); err != nil {
				return err
			}
		}
		//network policies for HTTP OTLP
		policyName, podSelector := objects.GetNetworkPolicyMetadata(f.owner, v2alpha1.NodeAgentComponentName)
		if f.createKubernetesNetworkPolicy {
			protocolTCP := corev1.ProtocolTCP
			ingressRules := []netv1.NetworkPolicyIngressRule{
				{
					Ports: []netv1.NetworkPolicyPort{
						{
							Port: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: port,
							},
							Protocol: &protocolTCP,
						},
					},
				},
			}
			if err := managers.NetworkPolicyManager().AddKubernetesNetworkPolicy(
				policyName,
				f.owner.GetNamespace(),
				podSelector,
				nil,
				ingressRules,
				nil,
			); err != nil {
				return err
			}
		} else if f.createCiliumNetworkPolicy {
			policySpecs := []cilium.NetworkPolicySpec{
				{
					Description:      "Ingress for HTTP OTLP",
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
											Port:     strconv.Itoa(int(port)),
											Protocol: cilium.ProtocolTCP,
										},
									},
								},
							},
						},
					},
				},
			}
			if err := managers.CiliumPolicyManager().AddCiliumPolicy(policyName, f.owner.GetNamespace(), policySpecs); err != nil {
				return err
			}
		}
	}
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *otlpFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func validateOTLPGRPCEndpoint(endpoint string) error {
	for _, protocol := range []string{"unix", "unix-abstract"} {
		if strings.HasPrefix(endpoint, protocol+":") {
			return fmt.Errorf("%q protocol is not currently supported", protocol)
		}
	}

	return nil
}

func extractPortEndpoint(endpoint string) (int32, error) {
	if match := portRegexp.FindStringSubmatch(endpoint); match != nil {
		if len(match) < 2 {
			return 0, fmt.Errorf("no match for port on %q", endpoint)
		}
		portStr := match[1]

		port, err := strconv.Atoi(portStr)
		if err != nil {
			return 0, fmt.Errorf("could not cast port %q from endpoint %q to int: %w", portStr, endpoint, err)
		}

		if port < 0 || port > 65535 {
			return 0, fmt.Errorf("port is outside valid range: %d", port)
		}

		return int32(port), nil
	}
	return 0, fmt.Errorf("%q does not have a port explicitly set", endpoint)
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *otlpFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	if f.grpcEnabled {
		if err := validateOTLPGRPCEndpoint(f.grpcEndpoint); err != nil {
			f.logger.Error(err, "invalid OTLP/gRPC endpoint")
			return fmt.Errorf("invalid OTLP/gRPC endpoint: %w", err)
		}
		port, err := extractPortEndpoint(f.grpcEndpoint)
		if err != nil {
			f.logger.Error(err, "failed to extract port from OTLP/gRPC endpoint")
			return fmt.Errorf("failed to extract port from OTLP/gRPC endpoint: %w", err)
		}
		otlpgrpcPort := &corev1.ContainerPort{
			Name:          otlpGRPCPortName,
			ContainerPort: port,
			Protocol:      corev1.ProtocolTCP,
		}
		if f.grpcHostPortEnabled {
			otlpgrpcPort.HostPort = f.grpcCustomHostPort
			if f.grpcCustomHostPort == 0 {
				otlpgrpcPort.HostPort = port
			}
		}
		envVar := &corev1.EnvVar{
			Name:  DDOTLPgRPCEndpoint,
			Value: f.grpcEndpoint,
		}
		managers.Port().AddPortToContainer(apicommon.UnprivilegedSingleAgentContainerName, otlpgrpcPort)
		managers.EnvVar().AddEnvVarToContainer(apicommon.UnprivilegedSingleAgentContainerName, envVar)
	}

	if f.httpEnabled {
		port, err := extractPortEndpoint(f.httpEndpoint)
		if err != nil {
			f.logger.Error(err, "failed to extract port from OTLP/HTTP endpoint")
			return fmt.Errorf("failed to extract port from OTLP/HTTP endpoint: %w", err)
		}
		otlphttpPort := &corev1.ContainerPort{
			Name:          otlpHTTPPortName,
			ContainerPort: port,
			Protocol:      corev1.ProtocolTCP,
		}
		if f.httpHostPortEnabled {
			otlphttpPort.HostPort = f.httpCustomHostPort
			if f.httpCustomHostPort == 0 {
				otlphttpPort.HostPort = port
			}
		}
		envVar := &corev1.EnvVar{
			Name:  DDOTLPHTTPEndpoint,
			Value: f.httpEndpoint,
		}
		managers.Port().AddPortToContainer(apicommon.UnprivilegedSingleAgentContainerName, otlphttpPort)
		managers.EnvVar().AddEnvVarToContainer(apicommon.UnprivilegedSingleAgentContainerName, envVar)
	}

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *otlpFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	if f.grpcEnabled {
		if err := validateOTLPGRPCEndpoint(f.grpcEndpoint); err != nil {
			f.logger.Error(err, "invalid OTLP/gRPC endpoint")
			return fmt.Errorf("invalid OTLP/gRPC endpoint: %w", err)
		}
		port, err := extractPortEndpoint(f.grpcEndpoint)
		if err != nil {
			f.logger.Error(err, "failed to extract port from OTLP/gRPC endpoint")
			return fmt.Errorf("failed to extract port from OTLP/gRPC endpoint: %w", err)
		}
		otlpgrpcPort := &corev1.ContainerPort{
			Name:          otlpGRPCPortName,
			ContainerPort: port,
			Protocol:      corev1.ProtocolTCP,
		}
		if f.grpcHostPortEnabled {
			otlpgrpcPort.HostPort = port
			if f.grpcCustomHostPort != 0 {
				otlpgrpcPort.HostPort = f.grpcCustomHostPort
			}
		}
		envVar := &corev1.EnvVar{
			Name:  DDOTLPgRPCEndpoint,
			Value: f.grpcEndpoint,
		}
		managers.Port().AddPortToContainer(apicommon.CoreAgentContainerName, otlpgrpcPort)
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, envVar)
		if f.usingAPM {
			managers.EnvVar().AddEnvVarToContainer(apicommon.TraceAgentContainerName, envVar)
		}
	}

	if f.httpEnabled {
		port, err := extractPortEndpoint(f.httpEndpoint)
		if err != nil {
			f.logger.Error(err, "failed to extract port from OTLP/HTTP endpoint")
			return fmt.Errorf("failed to extract port from OTLP/HTTP endpoint: %w", err)
		}
		otlphttpPort := &corev1.ContainerPort{
			Name:          otlpHTTPPortName,
			ContainerPort: port,
			Protocol:      corev1.ProtocolTCP,
		}
		if f.httpHostPortEnabled {
			otlphttpPort.HostPort = port
			if f.httpCustomHostPort != 0 {
				otlphttpPort.HostPort = f.httpCustomHostPort
			}
		}
		envVar := &corev1.EnvVar{
			Name:  DDOTLPHTTPEndpoint,
			Value: f.httpEndpoint,
		}
		managers.Port().AddPortToContainer(apicommon.CoreAgentContainerName, otlphttpPort)
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, envVar)
		if f.usingAPM {
			managers.EnvVar().AddEnvVarToContainer(apicommon.TraceAgentContainerName, envVar)
		}
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *otlpFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
