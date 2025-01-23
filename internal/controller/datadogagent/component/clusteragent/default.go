// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"fmt"
	"path/filepath"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
)

// GetClusterAgentServiceName return the Cluster-Agent service name based on the DatadogAgent name
func GetClusterAgentServiceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentPodDisruptionBudgetName return the Cluster-Agent PodDisruptionBudget name based on the DatadogAgent name
func GetClusterAgentPodDisruptionBudgetName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s-pdb", dda.GetName(), constants.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentName return the Cluster-Agent name based on the DatadogAgent name
func GetClusterAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentVersion return the Cluster-Agent version based on the DatadogAgent info
func GetClusterAgentVersion(dda metav1.Object) string {
	// Todo implement this function
	return ""
}

// GetClusterAgentRbacResourcesName return the Cluster-Agent RBAC resource name
func GetClusterAgentRbacResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultClusterAgentResourceSuffix)
}

// getDefaultServiceAccountName return the default Cluster-Agent ServiceAccountName
func getDefaultServiceAccountName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultClusterAgentResourceSuffix)
}

// NewDefaultClusterAgentDeployment return a new default cluster-agent deployment
func NewDefaultClusterAgentDeployment(dda metav1.Object) *appsv1.Deployment {
	deployment := common.NewDeployment(dda, constants.DefaultClusterAgentResourceSuffix, GetClusterAgentName(dda), GetClusterAgentVersion(dda), nil)
	podTemplate := NewDefaultClusterAgentPodTemplateSpec(dda)
	for key, val := range deployment.GetLabels() {
		podTemplate.Labels[key] = val
	}

	for key, val := range deployment.GetAnnotations() {
		podTemplate.Annotations[key] = val
	}
	deployment.Spec.Template = *podTemplate
	deployment.Spec.Replicas = apiutils.NewInt32Pointer(defaultClusterAgentReplicas)

	return deployment
}

// NewDefaultClusterAgentPodTemplateSpec return a default PodTemplateSpec for the cluster-agent deployment
func NewDefaultClusterAgentPodTemplateSpec(dda metav1.Object) *corev1.PodTemplateSpec {
	volumes := []corev1.Volume{
		common.GetVolumeInstallInfo(dda),
		common.GetVolumeForConfd(),
		common.GetVolumeForLogs(),
		common.GetVolumeForCertificates(),
		common.GetVolumeForAuth(),

		// /tmp is needed because some versions of the DCA (at least until
		// 1.19.0) write to it.
		// In some code paths, the klog lib writes to /tmp instead of using the
		// standard datadog logs path.
		// In some envs like Openshift, when running as non-root, the pod will
		// not have permissions to write on /tmp, that's why we need to mount
		// it with write perms.
		common.GetVolumeForTmp(),
	}

	volumeMounts := []corev1.VolumeMount{
		common.GetVolumeMountForInstallInfo(),
		common.GetVolumeMountForConfd(),
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForCertificates(),
		common.GetVolumeMountForAuth(false),
		common.GetVolumeMountForTmp(),
	}

	podTemplate := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: defaultPodSpec(dda, volumes, volumeMounts, defaultEnvVars(dda)),
	}

	return podTemplate
}

func defaultPodSpec(dda metav1.Object, volumes []corev1.Volume, volumeMounts []corev1.VolumeMount, envVars []corev1.EnvVar) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		ServiceAccountName: getDefaultServiceAccountName(dda),
		Containers: []corev1.Container{
			{
				Name:  string(apicommon.ClusterAgentContainerName),
				Image: fmt.Sprintf("%s/%s:%s", defaulting.DefaultImageRegistry, defaulting.DefaultClusterAgentImageName, defaulting.ClusterAgentLatestVersion),
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 5005,
						Name:          "agentport",
						Protocol:      "TCP",
					},
				},
				Env:            envVars,
				VolumeMounts:   volumeMounts,
				LivenessProbe:  constants.GetDefaultLivenessProbe(),
				ReadinessProbe: constants.GetDefaultReadinessProbe(),
				StartupProbe:   constants.GetDefaultStartupProbe(),
				Command:        nil,
				Args:           nil,
				SecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem:   apiutils.NewBoolPointer(true),
					AllowPrivilegeEscalation: apiutils.NewBoolPointer(false),
				},
			},
		},
		Affinity: DefaultAffinity(),
		Volumes:  volumes,
		// To be uncommented when the cluster-agent Dockerfile will be updated to use a non-root user by default
		// SecurityContext: &corev1.PodSecurityContext{
		// 	RunAsNonRoot: apiutils.NewBoolPointer(true),
		// },
	}

	return podSpec
}

func defaultEnvVars(dda metav1.Object) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name: v2alpha1.DDPodName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name:  v2alpha1.DDClusterAgentKubeServiceName,
			Value: GetClusterAgentServiceName(dda),
		},
		{
			Name:  v2alpha1.DDKubeResourcesNamespace,
			Value: utils.GetDatadogAgentResourceNamespace(dda),
		},
		{
			Name:  v2alpha1.DDLeaderElection,
			Value: "true",
		},
		{
			Name:  v2alpha1.DDHealthPort,
			Value: strconv.Itoa(int(constants.DefaultAgentHealthPort)),
		},
		{
			Name:  v2alpha1.DDAPMInstrumentationInstallId,
			Value: utils.GetDatadogAgentResourceUID(dda),
		},
		{
			Name:  v2alpha1.DDAPMInstrumentationInstallTime,
			Value: utils.GetDatadogAgentResourceCreationTime(dda),
		},
		{
			Name:  v2alpha1.DDAPMInstrumentationInstallType,
			Value: common.DefaultAgentInstallType,
		},
		{
			Name:  v2alpha1.DDAuthTokenFilePath,
			Value: filepath.Join(v2alpha1.AuthVolumePath, "token"),
		},
	}

	return envVars
}

// DefaultAffinity returns the pod anti affinity of the cluster agent
// the default anti affinity prefers scheduling the runners on different nodes if possible
// for better checks stability in case of node failure.
func DefaultAffinity() *corev1.Affinity {
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 50,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}
