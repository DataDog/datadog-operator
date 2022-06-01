// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

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
	checkInterval      string
	configMapConfig    *apicommonv1.ConfigMapConfig
	configMapName      string
	createSCC          bool
	createPSP          bool

	owner metav1.Object
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *cspmFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if dda.Spec.Features.CSPM != nil && apiutils.BoolValue(dda.Spec.Features.CSPM.Enabled) {
		f.enable = true
		f.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)

		if dda.Spec.Features.CSPM.CheckInterval != nil {
			f.checkInterval = strconv.FormatInt(dda.Spec.Features.CSPM.CheckInterval.Nanoseconds(), 10)
		}

		if dda.Spec.Features.CSPM.CustomBenchmarks != nil {
			f.configMapName = dda.Spec.Features.CSPM.CustomBenchmarks.Name
			f.configMapConfig = dda.Spec.Features.CSPM.CustomBenchmarks
		}

		// TODO add settings to configure f.createSCC and f.createPSP

		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
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
		f.serviceAccountName = v1alpha1.GetClusterAgentServiceAccount(dda)

		if dda.Spec.Agent.Security.Compliance.CheckInterval != nil {
			f.checkInterval = strconv.FormatInt(dda.Spec.Agent.Security.Compliance.CheckInterval.Duration.Nanoseconds(), 10)
		}

		if dda.Spec.Agent.Security.Compliance.ConfigDir != nil {
			f.configMapName = dda.Spec.Agent.Security.Compliance.ConfigDir.ConfigMapName
			f.configMapConfig = v1alpha1.ConvertConfigDirSpec(dda.Spec.Agent.Security.Compliance.ConfigDir).ConfigMap
		}

		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.SecurityAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *cspmFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	if f.createSCC {
		// Manage SecurityContextConstraints
		sccName := getSCCName(f.owner)
		scc, err := managers.PodSecurityManager().GetSecurityContextConstraints(f.owner.GetNamespace(), sccName)
		if err != nil {
			return err
		}
		scc.AllowHostPID = true
		managers.PodSecurityManager().UpdateSecurityContextConstraints(scc)
	}

	if f.createPSP {
		// Manage PodSecurityPolicy
		pspName := getPSPName(f.owner)
		psp, err := managers.PodSecurityManager().GetPodSecurityPolicy(f.owner.GetNamespace(), pspName)
		if err != nil {
			return err
		}
		psp.Spec.HostPID = true
		managers.PodSecurityManager().UpdatePodSecurityPolicy(psp)
	}

	// Manage RBAC
	rbacName := getRBACResourceName(f.owner)

	return managers.RBACManager().AddClusterPolicyRules("", rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	if f.configMapConfig != nil && f.configMapName != "" {
		cmVol, cmVolMount := volume.GetConfigMapVolumes(
			f.configMapConfig,
			f.configMapName,
			cspmConfigVolumeName,
			cspmConfigVolumePath,
		)
		managers.VolumeMount().AddVolumeMountToContainer(&cmVolMount, apicommonv1.ClusterAgentContainerName)
		managers.Volume().AddVolume(&cmVol)
	}

	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDComplianceEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, enabledEnvVar)

	if f.checkInterval != "" {
		intervalEnvVar := &corev1.EnvVar{
			Name:  apicommon.DDComplianceCheckInterval,
			Value: f.checkInterval,
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, intervalEnvVar)
	}

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// security context capabilities
	capabilities := []corev1.Capability{
		"AUDIT_CONTROL",
		"AUDIT_READ",
	}
	managers.SecurityContext().AddCapabilitiesToContainer(capabilities, apicommonv1.SecurityAgentContainerName)

	volMountMgr := managers.VolumeMount()
	VolMgr := managers.Volume()

	// configmap volume mount
	if f.configMapConfig != nil && f.configMapName != "" {
		cmVol, cmVolMount := volume.GetConfigMapVolumes(
			f.configMapConfig,
			f.configMapName,
			cspmConfigVolumeName,
			cspmConfigVolumePath,
		)
		volMountMgr.AddVolumeMountToContainer(&cmVolMount, apicommonv1.SecurityAgentContainerName)
		VolMgr.AddVolume(&cmVol)
	}

	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(apicommon.CgroupsVolumeName, apicommon.CgroupsHostPath, apicommon.CgroupsMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&cgroupsVolMount, apicommonv1.SecurityAgentContainerName)
	VolMgr.AddVolume(&cgroupsVol)

	// passwd volume mount
	passwdVol, passwdVolMount := volume.GetVolumes(apicommon.PasswdVolumeName, apicommon.PasswdHostPath, apicommon.PasswdMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&passwdVolMount, apicommonv1.SecurityAgentContainerName)
	VolMgr.AddVolume(&passwdVol)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(apicommon.ProcdirVolumeName, apicommon.ProcdirHostPath, apicommon.ProcdirMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&procdirVolMount, apicommonv1.SecurityAgentContainerName)
	VolMgr.AddVolume(&procdirVol)

	// host root volume mount
	hostRootVol, hostRootVolMount := volume.GetVolumes(apicommon.HostRootVolumeName, apicommon.HostRootHostPath, apicommon.HostRootMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&hostRootVolMount, apicommonv1.SecurityAgentContainerName)
	VolMgr.AddVolume(&hostRootVol)

	// group volume mount
	groupVol, groupVolMount := volume.GetVolumes(apicommon.GroupVolumeName, apicommon.GroupHostPath, apicommon.GroupMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&groupVolMount, apicommonv1.SecurityAgentContainerName)
	VolMgr.AddVolume(&groupVol)

	// env vars
	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDComplianceEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, enabledEnvVar)

	hostRootEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDHostRootEnvVar,
		Value: apicommon.HostRootMountPath,
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, hostRootEnvVar)

	if f.checkInterval != "" {
		intervalEnvVar := &corev1.EnvVar{
			Name:  apicommon.DDComplianceCheckInterval,
			Value: f.checkInterval,
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, intervalEnvVar)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
