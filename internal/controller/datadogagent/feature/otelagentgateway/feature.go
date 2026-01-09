// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagentgateway

import (
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.OtelAgentGatewayIDType, buildOtelAgentGatewayFeature)
	if err != nil {
		panic(err)
	}
}

type otelAgentGatewayFeature struct {
	logger logr.Logger
}

func buildOtelAgentGatewayFeature(options *feature.Options) feature.Feature {
	feature := &otelAgentGatewayFeature{}
	if options != nil {
		feature.logger = options.Logger
	}
	return feature
}

// ID returns the ID of the Feature
func (f *otelAgentGatewayFeature) ID() feature.IDType {
	return feature.OtelAgentGatewayIDType
}

func (f *otelAgentGatewayFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	return reqComp
}

func (f *otelAgentGatewayFeature) ManageDependencies(feature.ResourceManagers, string) error {
	return nil
}

func (f *otelAgentGatewayFeature) ManageClusterAgent(feature.PodTemplateManagers, string) error {
	// OtelAgentGateway doesn't need to configure the Cluster Agent
	return nil
}

func (f *otelAgentGatewayFeature) ManageSingleContainerNodeAgent(feature.PodTemplateManagers, string) error {
	// OtelAgentGateway doesn't need to configure the Node Agent
	return nil
}

func (f *otelAgentGatewayFeature) ManageNodeAgent(feature.PodTemplateManagers, string) error {
	// OtelAgentGateway doesn't need to configure the Node Agent
	return nil
}

func (f *otelAgentGatewayFeature) ManageClusterChecksRunner(feature.PodTemplateManagers, string) error {
	// OtelAgentGateway doesn't need to configure the Cluster Checks Runner
	return nil
}

func (f *otelAgentGatewayFeature) ManageOtelAgentGateway(feature.PodTemplateManagers, string) error {
	return nil
}
