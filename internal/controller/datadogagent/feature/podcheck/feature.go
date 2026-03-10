// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package podcheck

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

const (
	podCheckRBACPrefix      = "pod-check"
	podCheckVolumeName      = "crd-check-conf"
	crdConfigDirectory      = "crd-conf.d"
	podCheckConfigMapSuffix = "crd-check-conf"
)

func init() {
	err := feature.Register(feature.PodCheckIDType, buildPodCheckFeature)
	if err != nil {
		panic(err)
	}
}

func buildPodCheckFeature(_ *feature.Options) feature.Feature {
	return &podCheckFeature{}
}

type podCheckFeature struct {
	owner              metav1.Object
	configMapName      string
	serviceAccountName string
	rbacSuffix         string
}

func (f *podCheckFeature) ID() feature.IDType {
	return feature.PodCheckIDType
}

func (f *podCheckFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	f.owner = dda
	f.configMapName = fmt.Sprintf("%s-%s", dda.GetName(), podCheckConfigMapSuffix)
	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)
	f.rbacSuffix = common.ClusterAgentSuffix

	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
		},
		Agent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
		},
	}
}

func (f *podCheckFeature) ManageDependencies(managers feature.ResourceManagers, _ string) error {
	// Create the empty ConfigMap that the DCA will populate with check configs.
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.configMapName,
			Namespace: f.owner.GetNamespace(),
		},
		Data: map[string]string{},
	}
	if err := managers.Store().AddOnly(kubernetes.ConfigMapKind, cm); err != nil {
		return err
	}

	// RBAC: allow the Cluster Agent to read DatadogPodCheck CRDs and update the ConfigMap.
	rbacName := getRBACResourceName(f.owner, f.rbacSuffix)
	return managers.RBACManager().AddClusterPolicyRules(
		f.owner.GetNamespace(),
		rbacName,
		f.serviceAccountName,
		getPolicyRules(f.configMapName),
	)
}

func (f *podCheckFeature) ManageClusterAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func (f *podCheckFeature) ManageNodeAgent(managers feature.PodTemplateManagers, _ string) error {
	vol := volume.GetBasicVolume(f.configMapName, podCheckVolumeName)
	volMount := corev1.VolumeMount{
		Name:      podCheckVolumeName,
		MountPath: fmt.Sprintf("%s/%s", common.ConfigVolumePath, crdConfigDirectory),
		ReadOnly:  true,
	}

	managers.Volume().AddVolume(&vol)
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.CoreAgentContainerName)
	return nil
}

func (f *podCheckFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, _ string) error {
	vol := volume.GetBasicVolume(f.configMapName, podCheckVolumeName)
	volMount := corev1.VolumeMount{
		Name:      podCheckVolumeName,
		MountPath: fmt.Sprintf("%s/%s", common.ConfigVolumePath, crdConfigDirectory),
		ReadOnly:  true,
	}

	managers.Volume().AddVolume(&vol)
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.UnprivilegedSingleAgentContainerName)
	return nil
}

func (f *podCheckFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func (f *podCheckFeature) ManageOtelAgentGateway(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func getRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), podCheckRBACPrefix, suffix)
}

func getPolicyRules(configMapName string) []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.DatadogAPIGroup},
			Resources: []string{"datadogpodchecks"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups:     []string{rbac.CoreAPIGroup},
			Resources:     []string{rbac.ConfigMapsResource},
			ResourceNames: []string{configMapName},
			Verbs:         []string{"get", "update"},
		},
	}
}
