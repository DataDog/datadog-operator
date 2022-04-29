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

func (f *dummyFeature) Configure(dda *v2alpha1.DatadogAgent) bool {
	return false
}

func (f *dummyFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) bool {
	return false
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *dummyFeature) ManageDependencies(managers feature.ResourcesManagers) error {
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

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dummyFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error { return nil }

// ManageClusterCheckRunnerAgent allows a feature to configure the ClusterCheckRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dummyFeature) ManageClusterCheckRunnerAgent(managers feature.PodTemplateManagers) error {
	return nil
}
