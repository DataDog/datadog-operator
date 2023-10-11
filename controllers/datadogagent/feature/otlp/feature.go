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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/go-logr/logr"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
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

	grpcEnabled  bool
	grpcEndpoint string

	httpEnabled  bool
	httpEndpoint string

	usingAPM bool

	forceEnableLocalService bool
	localServiceName        string

	owner metav1.Object
}

// ID returns the ID of the Feature
func (f *otlpFeature) ID() feature.IDType {
	return feature.OTLPIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *otlpFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	otlp := dda.Spec.Features.OTLP
	f.owner = dda
	if apiutils.BoolValue(otlp.Receiver.Protocols.GRPC.Enabled) {
		f.grpcEnabled = true
	}
	if otlp.Receiver.Protocols.GRPC.Endpoint != nil {
		f.grpcEndpoint = *otlp.Receiver.Protocols.GRPC.Endpoint
	}

	if apiutils.BoolValue(otlp.Receiver.Protocols.HTTP.Enabled) {
		f.httpEnabled = true
	}
	if otlp.Receiver.Protocols.HTTP.Endpoint != nil {
		f.httpEndpoint = *otlp.Receiver.Protocols.HTTP.Endpoint
	}

	apm := dda.Spec.Features.APM
	if apm != nil {
		f.usingAPM = apiutils.BoolValue(apm.Enabled)
	}

	if dda.Spec.Global.LocalService != nil {
		f.forceEnableLocalService = apiutils.BoolValue(dda.Spec.Global.LocalService.ForceEnableLocalService)
	}
	f.localServiceName = v2alpha1.GetLocalAgentServiceName(dda)

	if f.grpcEnabled || f.httpEnabled {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
				},
			},
		}
		// if using APM, require the Trace Agent too.
		if f.usingAPM {
			reqComp.Agent.Containers = append(reqComp.Agent.Containers, apicommonv1.TraceAgentContainerName)
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *otlpFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	otlp := dda.Spec.Agent.OTLP
	f.owner = dda
	if apiutils.BoolValue(otlp.Receiver.Protocols.GRPC.Enabled) {
		f.grpcEnabled = true
	}
	if otlp.Receiver.Protocols.GRPC.Endpoint != nil {
		f.grpcEndpoint = *otlp.Receiver.Protocols.GRPC.Endpoint
	}

	if apiutils.BoolValue(otlp.Receiver.Protocols.HTTP.Enabled) {
		f.httpEnabled = true
	}
	if otlp.Receiver.Protocols.HTTP.Endpoint != nil {
		f.httpEndpoint = *otlp.Receiver.Protocols.HTTP.Endpoint
	}

	f.usingAPM = apiutils.BoolValue(dda.Spec.Agent.Apm.Enabled)

	if dda.Spec.Agent.LocalService != nil {
		f.forceEnableLocalService = apiutils.BoolValue(dda.Spec.Agent.LocalService.ForceLocalServiceEnable)
	}
	f.localServiceName = v1alpha1.GetLocalAgentServiceName(dda)

	if f.grpcEnabled || f.httpEnabled {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
				},
			},
		}
		// if using APM, require the Trace Agent too.
		if f.usingAPM {
			reqComp.Agent.Containers = append(reqComp.Agent.Containers, apicommonv1.TraceAgentContainerName)
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *otlpFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	if f.grpcEnabled {
		if component.ShouldCreateAgentLocalService(managers.Store().GetVersionInfo(), f.forceEnableLocalService) {
			port, err := extractPortEndpoint(f.grpcEndpoint)
			if err != nil {
				f.logger.Error(err, "failed to extract port from OTLP/gRPC endpoint")
				return fmt.Errorf("failed to extract port from OTLP/gRPC endpoint: %w", err)
			}
			servicePort := []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(int(port)),
					Port:       port,
					Name:       apicommon.OTLPGRPCPortName,
				},
			}
			serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
			if err := managers.ServiceManager().AddService(f.localServiceName, f.owner.GetNamespace(), component.GetAgentLocalServiceSelector(f.owner), servicePort, &serviceInternalTrafficPolicy); err != nil {
				return err
			}
		}
	}
	if f.httpEnabled {
		if component.ShouldCreateAgentLocalService(managers.Store().GetVersionInfo(), f.forceEnableLocalService) {
			port, err := extractPortEndpoint(f.httpEndpoint)
			if err != nil {
				f.logger.Error(err, "failed to extract port from OTLP/HTTP endpoint")
				return fmt.Errorf("failed to extract port from OTLP/HTTP endpoint: %w", err)
			}
			servicePort := []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(int(port)),
					Port:       port,
					Name:       apicommon.OTLPHTTPPortName,
				},
			}
			serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
			if err := managers.ServiceManager().AddService(f.localServiceName, f.owner.GetNamespace(), nil, servicePort, &serviceInternalTrafficPolicy); err != nil {
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

// ManageMultiProcessNodeAgent allows a feature to configure the mono-container Node Agent's corev1.PodTemplateSpec
// if mono-container usage is enabled and can be used with the current feature set
// It should do nothing if the feature doesn't need to configure it.
func (f *otlpFeature) ManageMultiProcessNodeAgent(managers feature.PodTemplateManagers) error {
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
			Name:          apicommon.OTLPGRPCPortName,
			ContainerPort: port,
			HostPort:      port,
			Protocol:      corev1.ProtocolTCP,
		}
		envVar := &corev1.EnvVar{
			Name:  apicommon.DDOTLPgRPCEndpoint,
			Value: f.grpcEndpoint,
		}
		managers.Port().AddPortToContainer(apicommonv1.NonPrivilegedMultiProcessAgentContainerName, otlpgrpcPort)
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.NonPrivilegedMultiProcessAgentContainerName, envVar)
	}

	if f.httpEnabled {
		port, err := extractPortEndpoint(f.httpEndpoint)
		if err != nil {
			f.logger.Error(err, "failed to extract port from OTLP/HTTP endpoint")
			return fmt.Errorf("failed to extract port from OTLP/HTTP endpoint: %w", err)
		}
		otlphttpPort := &corev1.ContainerPort{
			Name:          apicommon.OTLPHTTPPortName,
			ContainerPort: port,
			HostPort:      port,
			Protocol:      corev1.ProtocolTCP,
		}
		envVar := &corev1.EnvVar{
			Name:  apicommon.DDOTLPHTTPEndpoint,
			Value: f.httpEndpoint,
		}
		managers.Port().AddPortToContainer(apicommonv1.NonPrivilegedMultiProcessAgentContainerName, otlphttpPort)
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.NonPrivilegedMultiProcessAgentContainerName, envVar)
	}

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *otlpFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
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
			Name:          apicommon.OTLPGRPCPortName,
			ContainerPort: port,
			HostPort:      port,
			Protocol:      corev1.ProtocolTCP,
		}
		envVar := &corev1.EnvVar{
			Name:  apicommon.DDOTLPgRPCEndpoint,
			Value: f.grpcEndpoint,
		}
		managers.Port().AddPortToContainer(apicommonv1.CoreAgentContainerName, otlpgrpcPort)
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, envVar)
		if f.usingAPM {
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.TraceAgentContainerName, envVar)
		}
	}

	if f.httpEnabled {
		port, err := extractPortEndpoint(f.httpEndpoint)
		if err != nil {
			f.logger.Error(err, "failed to extract port from OTLP/HTTP endpoint")
			return fmt.Errorf("failed to extract port from OTLP/HTTP endpoint: %w", err)
		}
		otlphttpPort := &corev1.ContainerPort{
			Name:          apicommon.OTLPHTTPPortName,
			ContainerPort: port,
			HostPort:      port,
			Protocol:      corev1.ProtocolTCP,
		}
		envVar := &corev1.EnvVar{
			Name:  apicommon.DDOTLPHTTPEndpoint,
			Value: f.httpEndpoint,
		}
		managers.Port().AddPortToContainer(apicommonv1.CoreAgentContainerName, otlphttpPort)
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, envVar)
		if f.usingAPM {
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.TraceAgentContainerName, envVar)
		}
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *otlpFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
