// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelcollectorgateway

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func init() {
	err := feature.Register(feature.OtelCollectorGatewayIDType, buildOtelCollectorGatewayFeature)
	if err != nil {
		panic(err)
	}
}

type otelCollectorGatewayFeature struct {
	owner            metav1.Object
	logger           logr.Logger
	ports            []*corev1.ContainerPort
	localServiceName string
}

func buildOtelCollectorGatewayFeature(options *feature.Options) feature.Feature {
	feature := &otelCollectorGatewayFeature{}
	if options != nil {
		feature.logger = options.Logger
	}
	return feature
}

// ID returns the ID of the Feature
func (f *otelCollectorGatewayFeature) ID() feature.IDType {
	return feature.OtelCollectorGatewayIDType
}

func (f *otelCollectorGatewayFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	if ddaSpec.Features.OtelCollectorGateway != nil && apiutils.BoolValue(ddaSpec.Features.OtelCollectorGateway.Enabled) {
		f.owner = dda

		reqComp = feature.RequiredComponents{
			OtelCollectorGateway: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{apicommon.OtelCollectorGatewayContainerName},
			},
		}

		f.localServiceName = constants.GetOTelCollectorGatewayServiceName(dda.GetName())
		if len(ddaSpec.Features.OtelCollectorGateway.Ports) == 0 {
			f.ports = []*corev1.ContainerPort{
				{
					Name:          "otel-grpc",
					ContainerPort: 4317,
					Protocol:      corev1.ProtocolTCP,
				},
				{
					Name:          "otel-http",
					ContainerPort: 4318,
					Protocol:      corev1.ProtocolTCP,
				},
			}
		} else {
			f.ports = ddaSpec.Features.OtelCollectorGateway.Ports
		}
	}

	return reqComp
}

func (f *otelCollectorGatewayFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	grpcPort := 4317
	httpPort := 4318
	for _, port := range f.ports {
		if port.Name == "otel-grpc" {
			grpcPort = int(port.ContainerPort)
		}
		if port.Name == "otel-http" {
			httpPort = int(port.ContainerPort)
		}
	}

	otlpGrpcPort := &corev1.ServicePort{
		Name:       "otlpgrpcport",
		Port:       int32(grpcPort),
		Protocol:   corev1.ProtocolTCP,
		TargetPort: intstr.FromInt(grpcPort),
	}
	otlpHttpPort := &corev1.ServicePort{
		Name:       "otlphttpport",
		Port:       int32(httpPort),
		Protocol:   corev1.ProtocolTCP,
		TargetPort: intstr.FromInt(httpPort),
	}

	internalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
	if err := managers.ServiceManager().AddService(
		f.localServiceName,
		f.owner.GetNamespace(),
		common.GetOtelCollectorGatewayServiceSelector(f.owner),
		[]corev1.ServicePort{*otlpGrpcPort, *otlpHttpPort},
		&internalTrafficPolicy,
	); err != nil {
		return err
	}
	return nil
}

func (f *otelCollectorGatewayFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	// OtelCollectorGateway doesn't need to configure the Cluster Agent
	return nil
}

func (f *otelCollectorGatewayFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// OtelCollectorGateway doesn't need to configure the Node Agent
	return nil
}

func (f *otelCollectorGatewayFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// OtelCollectorGateway doesn't need to configure the Node Agent
	return nil
}

func (f *otelCollectorGatewayFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	// OtelCollectorGateway doesn't need to configure the Cluster Checks Runner
	return nil
}

func (f *otelCollectorGatewayFeature) ManageOtelCollectorGateway(managers feature.PodTemplateManagers, provider string) error {
	// Add ports
	for _, port := range f.ports {
		managers.Port().AddPortToContainer(apicommon.OtelCollectorGatewayContainerName, port)
	}
	return nil
}
