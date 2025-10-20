// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logcollection

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.LogCollectionIDType, buildLogCollectionFeature)
	if err != nil {
		panic(err)
	}
}

func buildLogCollectionFeature(options *feature.Options) feature.Feature {
	logCollectionFeat := &logCollectionFeature{}

	return logCollectionFeat
}

type logCollectionFeature struct {
	containerCollectAll        bool
	containerCollectUsingFiles bool
	containerLogsPath          string
	podLogsPath                string
	containerSymlinksPath      string
	tempStoragePath            string
	openFilesLimit             int32
	autoMultiLineDetection     *bool
}

// ID returns the ID of the Feature
func (f *logCollectionFeature) ID() feature.IDType {
	return feature.LogCollectionIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *logCollectionFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	if ddaSpec.Features == nil {
		return
	}

	logCollection := ddaSpec.Features.LogCollection

	if logCollection != nil && apiutils.BoolValue(logCollection.Enabled) {
		if logCollection.ContainerCollectAll != nil {
			// fallback to agent default if not set
			f.containerCollectAll = apiutils.BoolValue(logCollection.ContainerCollectAll)
		}
		f.containerCollectUsingFiles = apiutils.BoolValue(logCollection.ContainerCollectUsingFiles)
		f.containerLogsPath = *logCollection.ContainerLogsPath
		f.podLogsPath = *logCollection.PodLogsPath
		f.containerSymlinksPath = *logCollection.ContainerSymlinksPath
		f.tempStoragePath = *logCollection.TempStoragePath
		if logCollection.OpenFilesLimit != nil {
			f.openFilesLimit = *logCollection.OpenFilesLimit
		}
		f.autoMultiLineDetection = logCollection.AutoMultiLineDetection

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
				},
			},
		}
	}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *logCollectionFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *logCollectionFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *logCollectionFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.manageNodeAgent(apicommon.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *logCollectionFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.manageNodeAgent(apicommon.CoreAgentContainerName, managers, provider)
	return nil
}

func (f *logCollectionFeature) manageNodeAgent(agentContainerName apicommon.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {
	// pointerdir volume mount
	pointerVol, pointerVolMount := volume.GetVolumes(pointerVolumeName, f.tempStoragePath, pointerVolumePath, false)
	managers.VolumeMount().AddVolumeMountToContainer(&pointerVolMount, agentContainerName)
	managers.Volume().AddVolume(&pointerVol)

	// pod logs volume mount
	podLogVol, podLogVolMount := volume.GetVolumes(podLogVolumeName, f.podLogsPath, podLogVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&podLogVolMount, agentContainerName)
	managers.Volume().AddVolume(&podLogVol)

	// container logs volume mount
	containerLogVol, containerLogVolMount := volume.GetVolumes(containerLogVolumeName, f.containerLogsPath, containerLogVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&containerLogVolMount, agentContainerName)
	managers.Volume().AddVolume(&containerLogVol)

	// symlink volume mount
	symlinkVol, symlinkVolMount := volume.GetVolumes(symlinkContainerVolumeName, f.containerSymlinksPath, symlinkContainerVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&symlinkVolMount, agentContainerName)
	managers.Volume().AddVolume(&symlinkVol)

	// envvars
	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  common.DDLogsEnabled,
		Value: "true",
	})
	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  DDLogsConfigContainerCollectAll,
		Value: strconv.FormatBool(f.containerCollectAll),
	})
	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  DDLogsContainerCollectUsingFiles,
		Value: strconv.FormatBool(f.containerCollectUsingFiles),
	})
	if f.openFilesLimit != 0 {
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDLogsConfigOpenFilesLimit,
			Value: strconv.FormatInt(int64(f.openFilesLimit), 10),
		})
	}
	if f.autoMultiLineDetection != nil {
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDLogsConfigAutoMultiLineDetection,
			Value: strconv.FormatBool(*f.autoMultiLineDetection),
		})
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *logCollectionFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (f *logCollectionFeature) ManageOTelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
