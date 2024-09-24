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

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/go-logr/logr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
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

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *otlpFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	platformInfo := managers.Store().GetPlatformInfo()

	if f.grpcEnabled {
		if common.ShouldCreateAgentLocalService(platformInfo.GetVersionInfo(), f.forceEnableLocalService) {
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
			if err := managers.ServiceManager().AddService(f.localServiceName, f.owner.GetNamespace(), common.GetAgentLocalServiceSelector(f.owner), servicePort, &serviceInternalTrafficPolicy); err != nil {
				return err
			}
		}
	}
	if f.httpEnabled {
		if common.ShouldCreateAgentLocalService(platformInfo.GetVersionInfo(), f.forceEnableLocalService) {
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
			Name:          apicommon.OTLPGRPCPortName,
			ContainerPort: port,
			HostPort:      port,
			Protocol:      corev1.ProtocolTCP,
		}
		envVar := &corev1.EnvVar{
			Name:  apicommon.DDOTLPgRPCEndpoint,
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
			Name:          apicommon.OTLPHTTPPortName,
			ContainerPort: port,
			HostPort:      port,
			Protocol:      corev1.ProtocolTCP,
		}
		envVar := &corev1.EnvVar{
			Name:  apicommon.DDOTLPHTTPEndpoint,
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
			Name:          apicommon.OTLPGRPCPortName,
			ContainerPort: port,
			HostPort:      port,
			Protocol:      corev1.ProtocolTCP,
		}
		envVar := &corev1.EnvVar{
			Name:  apicommon.DDOTLPgRPCEndpoint,
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
			Name:          apicommon.OTLPHTTPPortName,
			ContainerPort: port,
			HostPort:      port,
			Protocol:      corev1.ProtocolTCP,
		}
		envVar := &corev1.EnvVar{
			Name:  apicommon.DDOTLPHTTPEndpoint,
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
