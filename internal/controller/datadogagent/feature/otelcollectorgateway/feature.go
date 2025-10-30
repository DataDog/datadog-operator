// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelcollectorgateway

import (
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.OtelCollectorGatewayIDType, buildOtelCollectorGatewayFeature)
	if err != nil {
		panic(err)
	}
}

type otelCollectorGatewayFeature struct {
	owner  metav1.Object
	logger logr.Logger
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
	}

	return reqComp
}

func (f *otelCollectorGatewayFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	// No dependencies to manage for now
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
	// For now, the OtelCollectorGateway deployment is configured via the default deployment
	// No additional configuration is needed here
	return nil
}
