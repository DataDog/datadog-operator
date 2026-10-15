// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package jemalloc

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	ldPreload   = "LD_PRELOAD"
	mallocConf  = "MALLOC_CONF"
	libraryPath = "/opt/datadog-agent/embedded/lib/libjemalloc.so"
	nosysPath   = "/opt/lib/nosys.so"
	// Pod environment variables replace image defaults, so retain the Agent image's nosys shim.
	defaultPreload    = nosysPath + " " + libraryPath
	defaultMallocConf = "background_thread:true,dirty_decay_ms:5000"
	minAgentVersion   = "7.84.0-0"
)

func init() {
	if err := feature.Register(feature.JemallocIDType, buildJemallocFeature); err != nil {
		panic(err)
	}
}

func buildJemallocFeature(_ *feature.Options) feature.Feature {
	return &jemallocFeature{}
}

type jemallocFeature struct {
	nodeAgentEnabled           bool
	clusterChecksRunnerEnabled bool
}

func (f *jemallocFeature) ID() feature.IDType {
	return feature.JemallocIDType
}

// Configure enables jemalloc when the annotation is "true" and the component's Agent version is at least 7.84.
func (f *jemallocFeature) Configure(dda metav1.Object, spec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	enabled := featureutils.HasFeatureEnableAnnotation(dda, featureutils.EnableJemallocAnnotation)
	f.nodeAgentEnabled = enabled && supportsJemalloc(spec, v2alpha1.NodeAgentComponentName)
	f.clusterChecksRunnerEnabled = enabled && supportsJemalloc(spec, v2alpha1.ClusterChecksRunnerComponentName)
	var required feature.RequiredComponents
	if f.nodeAgentEnabled {
		// Configure containers enabled by other features without requesting optional containers.
		required.Agent = feature.RequiredComponent{
			IsRequired: &f.nodeAgentEnabled,
			Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
		}
	}
	if f.clusterChecksRunnerEnabled {
		// Configure runners only if another feature enables them.
		required.ClusterChecksRunner.Containers = []apicommon.AgentContainerName{apicommon.ClusterChecksRunnersContainerName}
	}
	return required
}

func supportsJemalloc(spec *v2alpha1.DatadogAgentSpec, component v2alpha1.ComponentName) bool {
	version := images.AgentLatestVersion
	if spec != nil {
		if override := spec.Override[component]; override != nil && override.Image != nil {
			version = common.GetAgentVersionFromImage(*override.Image)
		}
	}
	return utils.IsAboveMinVersion(version, minAgentVersion, new(true))
}

func (f *jemallocFeature) ManageDependencies(_ feature.ResourceManagers) error {
	return nil
}

func (f *jemallocFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	if !f.nodeAgentEnabled {
		return nil
	}
	return enableJemalloc(managers,
		apicommon.CoreAgentContainerName,
		apicommon.SystemProbeContainerName,
		apicommon.TraceAgentContainerName,
	)
}

func (f *jemallocFeature) ManageSingleContainerNodeAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *jemallocFeature) ManageClusterAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *jemallocFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	if !f.clusterChecksRunnerEnabled {
		return nil
	}
	return enableJemalloc(managers)
}

func (f *jemallocFeature) ManageOtelAgentGateway(_ feature.PodTemplateManagers) error {
	return nil
}

func enableJemalloc(managers feature.PodTemplateManagers, containerNames ...apicommon.AgentContainerName) error {
	for _, env := range []corev1.EnvVar{
		{Name: ldPreload, Value: defaultPreload},
		{Name: mallocConf, Value: defaultMallocConf},
	} {
		if len(containerNames) == 0 {
			managers.EnvVar().AddEnvVar(&env)
		} else {
			managers.EnvVar().AddEnvVarToContainers(containerNames, &env)
		}
	}
	return nil
}
