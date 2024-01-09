// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dummy

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.DummyIDType, buildDummyfeature)
	if err != nil {
		panic(err)
	}
}

func buildDummyfeature(options *feature.Options) feature.Feature {
	return &dummyFeature{}
}

const (
	dummyLabelKey   = "feature.com/dummy"
	dummyLabelValue = "true"
)

type dummyFeature struct{}

// ID returns the ID of the Feature
func (f *dummyFeature) ID() feature.IDType {
	return feature.DummyIDType
}

func (f *dummyFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	return feature.RequiredComponents{}
}

func (f *dummyFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) feature.RequiredComponents {
	return feature.RequiredComponents{}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *dummyFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dummyFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	podTemplate := managers.PodTemplateSpec()
	if podTemplate.Labels == nil {
		podTemplate.Labels = make(map[string]string)
	}
	podTemplate.Labels[dummyLabelKey] = dummyLabelValue
	return nil
}

// ManageMultiProcessNodeAgent allows a feature to configure the multi-process container for Node Agent's corev1.PodTemplateSpec
// if multi-process container usage is enabled and can be used with the current feature set
// It should do nothing if the feature doesn't need to configure it.
func (f *dummyFeature) ManageMultiProcessNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dummyFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dummyFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
