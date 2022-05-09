// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logcollection

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
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
	enable                     bool
	containerCollectAll        bool
	containerCollectUsingFiles bool
	containerLogsPath          string
	podLogsPath                string
	containerSymlinksPath      string
	tempStoragePath            string
	openFilesLimit             int32

	owner metav1.Object
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *logCollectionFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	f.owner = dda
	if dda.Spec.Features.LogCollection != nil && apiutils.BoolValue(dda.Spec.Features.LogCollection.Enabled) {
		f.enable = true

		if apiutils.BoolValue(dda.Spec.Features.LogCollection.ContainerCollectAll) {
			f.containerCollectAll = true
		}
		if dda.Spec.Features.LogCollection.ContainerCollectUsingFiles == nil || apiutils.BoolValue(dda.Spec.Features.LogCollection.ContainerCollectUsingFiles) {
			f.containerCollectUsingFiles = true
		}
		if dda.Spec.Features.LogCollection.ContainerLogsPath != nil {
			f.containerLogsPath = *dda.Spec.Features.LogCollection.ContainerLogsPath
		}
		if dda.Spec.Features.LogCollection.PodLogsPath != nil {
			f.podLogsPath = *dda.Spec.Features.LogCollection.PodLogsPath
		}
		if dda.Spec.Features.LogCollection.ContainerSymlinksPath != nil {
			f.containerSymlinksPath = *dda.Spec.Features.LogCollection.ContainerSymlinksPath
		}
		if dda.Spec.Features.LogCollection.TempStoragePath != nil {
			f.tempStoragePath = *dda.Spec.Features.LogCollection.TempStoragePath
		}
		if dda.Spec.Features.LogCollection.OpenFilesLimit != nil {
			f.openFilesLimit = *dda.Spec.Features.LogCollection.OpenFilesLimit
		} else {
			f.openFilesLimit = 100
		}
	}

	return feature.RequiredComponents{
		Agent: feature.RequiredComponent{Required: &f.enable},
	}
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *logCollectionFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) feature.RequiredComponents {
	f.owner = dda

	if dda.Spec.Features.LogCollection != nil {
		if apiutils.BoolValue(dda.Spec.Features.LogCollection.Enabled) {
			f.enable = true

			if apiutils.BoolValue(dda.Spec.Features.LogCollection.LogsConfigContainerCollectAll) {
				f.containerCollectAll = true
			}
			if dda.Spec.Features.LogCollection.ContainerCollectUsingFiles == nil || *dda.Spec.Features.LogCollection.ContainerCollectUsingFiles {
				f.containerCollectUsingFiles = true
			}
			if dda.Spec.Features.LogCollection.ContainerLogsPath != nil {
				f.containerLogsPath = *dda.Spec.Features.LogCollection.ContainerLogsPath
			}
			if dda.Spec.Features.LogCollection.PodLogsPath != nil {
				f.podLogsPath = *dda.Spec.Features.LogCollection.PodLogsPath
			}
			if dda.Spec.Features.LogCollection.ContainerSymlinksPath != nil {
				f.containerSymlinksPath = *dda.Spec.Features.LogCollection.ContainerSymlinksPath
			}
			if dda.Spec.Features.LogCollection.TempStoragePath != nil {
				f.tempStoragePath = *dda.Spec.Features.LogCollection.TempStoragePath
			}
			if dda.Spec.Features.LogCollection.OpenFilesLimit != nil {
				f.openFilesLimit = *dda.Spec.Features.LogCollection.OpenFilesLimit
			} else {
				f.openFilesLimit = 100
			}
		}
	}

	return feature.RequiredComponents{
		Agent: feature.RequiredComponent{Required: &f.enable},
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *logCollectionFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *logCollectionFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *logCollectionFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// pointerdir volume mount
	var pointerVol, logPodVol, logContainerVol, symlinkVol corev1.Volume
	var pointerVolMount, logPodVolMount, logContainerVolMount, symlinkVolMount corev1.VolumeMount
	// TODO: mount as ReadWrite instead of ReadOnly
	if f.tempStoragePath != "" {
		pointerVol, pointerVolMount = volume.GetVolumes(apicommon.PointerVolumeName, f.tempStoragePath, apicommon.PointerVolumePath, false)
	} else {
		pointerVol, pointerVolMount = volume.GetVolumes(apicommon.PointerVolumeName, apicommon.LogTempStoragePath, apicommon.PointerVolumePath, false)
	}
	managers.Volume().AddVolumeToContainer(&pointerVol, &pointerVolMount, apicommonv1.CoreAgentContainerName)

	// pod logs volume mount
	if f.podLogsPath != "" {
		logPodVol, logPodVolMount = volume.GetVolumes(apicommon.LogPodVolumeName, f.podLogsPath, f.podLogsPath, true)
	} else {
		logPodVol, logPodVolMount = volume.GetVolumes(apicommon.LogPodVolumeName, apicommon.LogPodVolumePath, apicommon.LogPodVolumePath, true)
	}
	managers.Volume().AddVolumeToContainer(&logPodVol, &logPodVolMount, apicommonv1.CoreAgentContainerName)

	// container logs volume mount
	if f.containerLogsPath != "" {
		logContainerVol, logContainerVolMount = volume.GetVolumes(apicommon.LogContainerVolumeName, f.containerLogsPath, f.containerLogsPath, true)
	} else {
		logContainerVol, logContainerVolMount = volume.GetVolumes(apicommon.LogContainerVolumeName, apicommon.LogContainerVolumePath, apicommon.LogContainerVolumePath, true)
	}
	managers.Volume().AddVolumeToContainer(&logContainerVol, &logContainerVolMount, apicommonv1.CoreAgentContainerName)

	// symlink volume mount
	if f.containerSymlinksPath != "" {
		symlinkVol, symlinkVolMount = volume.GetVolumes(apicommon.SymlinkContainerVolumeName, f.containerSymlinksPath, f.containerSymlinksPath, true)
	} else {
		symlinkVol, symlinkVolMount = volume.GetVolumes(apicommon.SymlinkContainerVolumeName, apicommon.SymlinkContainerVolumePath, apicommon.SymlinkContainerVolumePath, true)
	}
	managers.Volume().AddVolumeToContainer(&symlinkVol, &symlinkVolMount, apicommonv1.CoreAgentContainerName)

	// envvars
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDLogsEnabled,
		Value: strconv.FormatBool(f.enable),
	})
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDLogsConfigContainerCollectAll,
		Value: strconv.FormatBool(f.containerCollectAll),
	})
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDLogsContainerCollectUsingFiles,
		Value: strconv.FormatBool(f.containerCollectUsingFiles),
	})
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDLogsConfigOpenFilesLimit,
		Value: strconv.FormatInt(int64(f.openFilesLimit), 10),
	})

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterCheckRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *logCollectionFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
