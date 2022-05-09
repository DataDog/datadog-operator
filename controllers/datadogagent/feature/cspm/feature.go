// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
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
	err := feature.Register(feature.CSPMIDType, buildCSPMFeature)
	if err != nil {
		panic(err)
	}
}

func buildCSPMFeature(options *feature.Options) feature.Feature {
	cspmFeat := &cspmFeature{}

	return cspmFeat
}

type cspmFeature struct {
	enable             bool
	serviceAccountName string
	owner              metav1.Object
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *cspmFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if dda.Spec.Features.CSPM != nil && apiutils.BoolValue(dda.Spec.Features.CSPM.Enabled) {
		f.enable = true
		f.serviceAccountName = "cluster-agent" // CELENE FIX ME
		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{Required: apiutils.NewBoolPointer(true)},
			Agent: feature.RequiredComponent{
				Required: apiutils.NewBoolPointer(true),
				RequiredContainers: []apicommonv1.AgentContainerName{
					apicommonv1.SecurityAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *cspmFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if dda.Spec.Agent.Security != nil && *dda.Spec.Agent.Security.Compliance.Enabled {
		f.enable = true

		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{Required: apiutils.NewBoolPointer(true)},
			Agent: feature.RequiredComponent{
				Required: apiutils.NewBoolPointer(true),
				RequiredContainers: []apicommonv1.AgentContainerName{
					apicommonv1.SecurityAgentContainerName,
				},
			},
		}

	}

	// CELENE TODO
	// hostRootEnvVarIsSet := false
	// for _, envVar := range dda.Spec.Agent.SystemProbe.Env {
	// 	if envVar.Name == apicommon.DDHostRootEnvVar && envVar.Value == apicommon.HostRootMountPath {
	// 		hostRootEnvVarIsSet = true
	// 	}
	// }

	// hostRootVolIsSet := false
	// for _, volMount := range dda.Spec.Agent.SystemProbe.VolumeMounts {
	// 	if volMount.Name == apicommon.HostRootVolumeName && volMount.MountPath == apicommon.HostRootMountPath {
	// 		hostRootVolIsSet = true
	// 	}
	// }

	// if dda.Spec.Agent.SystemProbe != nil && *dda.Spec.Agent.SystemProbe.Enabled && hostRootEnvVarIsSet && hostRootVolIsSet {
	// 	f.enable = true

	// 	reqComp = feature.RequiredComponents{
	// 		Agent: feature.RequiredComponent{
	// 			Required: &f.enable,
	// 			RequiredContainers: []apicommonv1.AgentContainerName{
	// 				apicommonv1.SystemProbeContainerName,
	// 			},
	// 		},
	// 	}
	// }

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *cspmFeature) ManageDependencies(managers feature.ResourceManagers) error {
	// Manage SecurityContextConstraints and PodSecurityPolicy
	sccName := getSCCName()
	if err := managers.SecurityManager().UpdateSecurityContextConstraints(f.owner.GetNamespace(), sccName, "AllowHostPID", "true"); err != nil {
		return err
	}

	pspName := getPSPName()
	if err := managers.SecurityManager().UpdatePodSecurityPolicy(f.owner.GetNamespace(), pspName, "AllowHostPID", "true"); err != nil {
		return err
	}

	// Manage RBAC
	rbacName := getRBACResourceName(f.owner)

	return managers.RBACManager().AddClusterPolicyRules("", rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDComplianceEnabledEnvVar,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, enabledEnvVar)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(apicommon.CgroupsVolumeName, apicommon.CgroupsHostPath, apicommon.CgroupsMountPath)
	managers.Volume().AddVolumeToContainer(&cgroupsVol, &cgroupsVolMount, apicommonv1.SecurityAgentContainerName)

	// passwd volume mount
	passwdVol, passwdVolMount := volume.GetVolumes(apicommon.PasswdVolumeName, apicommon.PasswdHostPath, apicommon.PasswdMountPath)
	managers.Volume().AddVolumeToContainer(&passwdVol, &passwdVolMount, apicommonv1.SecurityAgentContainerName)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(apicommon.ProcdirVolumeName, apicommon.ProcdirHostPath, apicommon.ProcdirMountPath)
	managers.Volume().AddVolumeToContainer(&procdirVol, &procdirVolMount, apicommonv1.SecurityAgentContainerName)

	// host root volume mount
	hostRootVol, hostRootVolMount := volume.GetVolumes(apicommon.HostRootVolumeName, apicommon.HostRootHostPath, apicommon.HostRootMountPath)
	managers.Volume().AddVolumeToContainer(&hostRootVol, &hostRootVolMount, apicommonv1.SecurityAgentContainerName)

	// group volume mount
	groupVol, groupVolMount := volume.GetVolumes(apicommon.GroupVolumeName, apicommon.GroupHostPath, apicommon.GroupMountPath)
	managers.Volume().AddVolumeToContainer(&groupVol, &groupVolMount, apicommonv1.SecurityAgentContainerName)

	// env vars
	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDComplianceEnabledEnvVar,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, enabledEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
