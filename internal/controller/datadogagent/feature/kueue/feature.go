// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kueue

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

func init() {
	err := feature.Register(feature.KueueIDType, buildKueueFeature)
	if err != nil {
		panic(err)
	}
}

func buildKueueFeature(options *feature.Options) feature.Feature {
	kueueFeat := &kueueFeature{}

	if options != nil {
		kueueFeat.logger = options.Logger
	}
	return kueueFeat
}

type kueueFeature struct {
	// checkEnabled is false when cluster checks are disabled: the Kueue check can
	// only be scheduled as an endpoints check, so only the Cluster Agent metadata
	// collection is enabled.
	checkEnabled          bool
	collectWorkloadEvents bool
	serviceName           string
	serviceNamespace      string

	clusterAgentServiceAccountName string
	agentServiceAccountName        string

	owner         metav1.Object
	customConfig  *v2alpha1.CustomConfig
	configMapName string

	logger logr.Logger
}

// ID returns the ID of the Feature
func (f *kueueFeature) ID() feature.IDType {
	return feature.KueueIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *kueueFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if ddaSpec.Features == nil || ddaSpec.Features.Kueue == nil || !apiutils.BoolValue(ddaSpec.Features.Kueue.Enabled) {
		return reqComp
	}
	kueue := ddaSpec.Features.Kueue

	if !utils.IsAboveMinVersion(common.GetComponentVersion(dda, v2alpha1.ClusterAgentComponentName), kueueMinVersion, nil) {
		f.logger.Info("Kueue feature requires Cluster Agent >= 7.82.0; not enabling it")
		return reqComp
	}

	reqComp.ClusterAgent.IsRequired = new(true)
	reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
	f.clusterAgentServiceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

	if !constants.IsClusterChecksEnabled(ddaSpec) {
		f.logger.Info("Kueue check requires features.clusterChecks.enabled; only Kueue metadata collection is enabled in the Cluster Agent")
		return reqComp
	}

	if !utils.IsAboveMinVersion(common.GetComponentVersion(dda, v2alpha1.NodeAgentComponentName), kueueMinVersion, nil) {
		f.logger.Info("Kueue check requires Agent >= 7.82.0; only Kueue metadata collection is enabled in the Cluster Agent")
		return reqComp
	}

	f.checkEnabled = true
	reqComp.Agent.IsRequired = new(true)
	reqComp.Agent.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
	f.agentServiceAccountName = constants.GetAgentServiceAccount(dda.GetName(), ddaSpec)

	f.collectWorkloadEvents = kueue.CollectWorkloadEvents == nil || *kueue.CollectWorkloadEvents
	f.serviceName = defaultMetricsServiceName
	f.serviceNamespace = defaultMetricsServiceNamespace
	if svc := kueue.MetricsService; svc != nil {
		if svc.Name != nil && *svc.Name != "" {
			f.serviceName = *svc.Name
		}
		if svc.Namespace != nil && *svc.Namespace != "" {
			f.serviceNamespace = *svc.Namespace
		}
	}

	f.customConfig = kueue.Conf
	f.configMapName = constants.GetConfName(dda, f.customConfig, defaultKueueConf)

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *kueueFeature) ManageDependencies(managers feature.ResourceManagers) error {
	ns := f.owner.GetNamespace()

	if err := managers.RBACManager().AddClusterPolicyRules(ns, getKueueRBACResourceName(f.owner, common.ClusterAgentSuffix), f.clusterAgentServiceAccountName, kueueClusterAgentRBACPolicyRules); err != nil {
		return fmt.Errorf("failed to add Kueue Cluster Agent RBAC: %w", err)
	}

	if !f.checkEnabled {
		return nil
	}

	cm, err := f.buildKueueCheckConfigMap()
	if err != nil {
		return fmt.Errorf("failed to build Kueue check ConfigMap: %w", err)
	}
	if cm != nil {
		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
			return fmt.Errorf("failed to add Kueue check ConfigMap to the store: %w", err)
		}
	}

	if !f.collectWorkloadEvents {
		return nil
	}

	if err := managers.RBACManager().AddClusterPolicyRules(ns, getKueueRBACResourceName(f.owner, common.NodeAgentSuffix), f.agentServiceAccountName, kueueNodeAgentRBACPolicyRules); err != nil {
		return fmt.Errorf("failed to add Kueue node Agent RBAC: %w", err)
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *kueueFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDClusterAgentKueueEnabled,
		Value: "true",
	})

	if !f.checkEnabled {
		return nil
	}

	var vol corev1.Volume
	var volMount corev1.VolumeMount
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		vol, volMount = volume.GetVolumesFromConfigMap(
			f.customConfig.ConfigMap,
			kueueConfigVolumeName,
			f.configMapName,
			kueueCheckFolderName,
		)
	} else {
		vol = volume.GetBasicVolume(f.configMapName, kueueConfigVolumeName)
		volMount = corev1.VolumeMount{
			Name:      kueueConfigVolumeName,
			MountPath: fmt.Sprintf("%s%s/%s", common.ConfigVolumePath, common.ConfdVolumePath, kueueCheckFolderName),
			ReadOnly:  true,
		}
	}
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.ClusterAgentContainerName)
	managers.Volume().AddVolume(&vol)

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *kueueFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *kueueFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *kueueFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}

func (f *kueueFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers) error {
	return nil
}
