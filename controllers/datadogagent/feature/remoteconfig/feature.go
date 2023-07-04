// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/go-logr/logr"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.RemoteConfigurationIDType, buildRCFeature)
	if err != nil {
		panic(err)
	}
}

func buildRCFeature(options *feature.Options) feature.Feature {
	rcFeat := &rcFeature{
		// Current default for Remote Config enablement
		enabled: false,
	}

	if options != nil {
		rcFeat.logger = options.Logger
	}

	return rcFeat
}

type rcFeature struct {
	owner  metav1.Object
	logger logr.Logger

	enabled bool
}

// ID returns the ID of the Feature
func (f *rcFeature) ID() feature.IDType {
	return feature.RemoteConfigurationIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *rcFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	reqComp = feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{
				apicommonv1.CoreAgentContainerName,
			},
		},
	}

	if dda.Spec.Features != nil && dda.Spec.Features.RemoteConfiguration != nil && dda.Spec.Features.RemoteConfiguration.Enabled != nil {
		// If a value exists, explicitely enable or disable Remote Config and override the default
		if apiutils.BoolValue(dda.Spec.Features.RemoteConfiguration.Enabled) {
			f.enabled = true
		} else {
			f.enabled = false
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *rcFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	return
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *rcFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *rcFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDRemoteConfigurationEnabled,
		Value: apiutils.BoolToString(&f.enabled),
	}
	managers.EnvVar().AddEnvVar(enabledEnvVar)

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *rcFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDRemoteConfigurationEnabled,
		Value: apiutils.BoolToString(&f.enabled),
	}
	managers.EnvVar().AddEnvVar(enabledEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *rcFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
