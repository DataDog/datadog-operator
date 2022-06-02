// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecksrunner

import (
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
)

// NewDefaultClusterChecksRunnerDeployment return a new default cluster-checks-runner deployment
func NewDefaultClusterChecksRunnerDeployment(dda metav1.Object) *appsv1.Deployment {
	deployment := component.NewDeployment(dda, apicommon.DefaultClusterChecksRunnerResourceSuffix, component.GetClusterChecksRunnerName(dda), component.GetAgentVersion(dda), nil)

	podTemplate := NewDefaultClusterChecksRunnerPodTemplateSpec(dda)
	for key, val := range deployment.GetLabels() {
		podTemplate.Labels[key] = val
	}

	for key, val := range deployment.GetAnnotations() {
		podTemplate.Annotations[key] = val
	}

	deployment.Spec.Template = *podTemplate
	deployment.Spec.Replicas = apiutils.NewInt32Pointer(apicommon.DefaultClusterChecksRunnerReplicas)

	return deployment
}

// NewDefaultClusterChecksRunnerPodTemplateSpec returns a default cluster-checks-runner for the cluster-agent deployment
func NewDefaultClusterChecksRunnerPodTemplateSpec(dda metav1.Object) *corev1.PodTemplateSpec {
	volumes := []corev1.Volume{
		component.GetVolumeInstallInfo(dda),
		component.GetVolumeForConfig(),
		component.GetVolumeForRmCorechecks(),
		component.GetVolumeForLogs(),

		// /tmp is needed because some versions of the DCA (at least until
		// 1.19.0) write to it.
		// In some code paths, the klog lib writes to /tmp instead of using the
		// standard datadog logs path.
		// In some envs like Openshift, when running as non-root, the pod will
		// not have permissions to write on /tmp, that's why we need to mount
		// it with write perms.
		component.GetVolumeForTmp(),
	}

	volumeMounts := []corev1.VolumeMount{
		component.GetVolumeMountForInstallInfo(),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForTmp(),
		component.GetVolumeMountForRmCorechecks(),
	}

	template := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: defaultPodSpec(dda, volumes, volumeMounts, defaultEnvVars(dda)),
	}

	return template
}

// GetDefaultServiceAccountName return the default Cluster-Agent ServiceAccountName
func GetDefaultServiceAccountName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

func defaultPodSpec(dda metav1.Object, volumes []corev1.Volume, volumeMounts []corev1.VolumeMount, envVars []corev1.EnvVar) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		ServiceAccountName: GetDefaultServiceAccountName(dda),
		InitContainers: []corev1.Container{
			{
				Name:    "init-config",
				Image:   fmt.Sprintf("%s:%s", apicommon.DefaultAgentImageName, defaulting.AgentLatestVersion),
				Command: []string{"bash", "-c"},
				Args: []string{
					"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done",
				},
				VolumeMounts: volumeMounts,
			},
		},
		Containers: []corev1.Container{
			{
				Name:         string(apicommonv1.ClusterChecksRunnersContainerName),
				Image:        fmt.Sprintf("%s:%s", apicommon.DefaultAgentImageName, defaulting.AgentLatestVersion),
				Env:          envVars,
				VolumeMounts: volumeMounts,
				Command:      []string{"bash", "-c"},
				Args: []string{
					"rm -rf /etc/datadog-agent/conf.d && touch /etc/datadog-agent/datadog.yaml && exec agent run",
				},
				LivenessProbe:  apicommon.GetDefaultLivenessProbe(),
				ReadinessProbe: apicommon.GetDefaultReadinessProbe(),
				SecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem:   apiutils.NewBoolPointer(true),
					AllowPrivilegeEscalation: apiutils.NewBoolPointer(false),
				},
			},
		},
		Affinity: DefaultAffinity(),
		Volumes:  volumes,
		// To be uncommented when the agent Dockerfile will be updated to use a non-root user by default
		// SecurityContext: &corev1.PodSecurityContext{
		// 	RunAsNonRoot: apiutils.NewBoolPointer(true),
		// },
	}
	return podSpec
}

func defaultEnvVars(dda metav1.Object) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  apicommon.DDClusterAgentKubeServiceName,
			Value: component.GetClusterAgentServiceName(dda),
		},
		{
			Name:  apicommon.DDClusterChecksEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDClusterAgentEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDHealthPort,
			Value: strconv.Itoa(int(apicommon.DefaultAgentHealthPort)),
		},
		{
			Name:  apicommon.KubernetesEnvVar,
			Value: "yes",
		},
		{
			Name:  apicommon.DDExtraConfigProviders,
			Value: apicommon.ClusterChecksConfigProvider,
		},
		{
			Name:  apicommon.DDEnableMetadataCollection,
			Value: "false",
		},
		{
			Name:  apicommon.DDCLCRunnerEnabled,
			Value: "true",
		},
		{
			Name: apicommon.DDCLCRunnerHost,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: apicommon.FieldPathStatusPodIP,
				},
			},
		},
		{
			Name: apicommon.DDCLCRunnerID,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: apicommon.FieldPathMetaName,
				},
			},
		},
		{
			Name:  apicommon.DDDogstatsdEnabled,
			Value: "false",
		},
		{
			Name:  apicommon.DDProcessAgentEnabled,
			Value: "false",
		},
		{
			Name:  apicommon.DDLogsEnabled,
			Value: "false",
		},
		{
			Name:  apicommon.DDAPMEnabled,
			Value: "false",
		},
		{
			Name: apicommon.DDHostname,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: apicommon.FieldPathSpecNodeName,
				},
			},
		},
	}

	return envVars
}

// DefaultAffinity returns the pod anti affinity of the cluster checks runners
// The default anti affinity prefers scheduling the runners on different nodes if possible
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
								apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultClusterChecksRunnerResourceSuffix,
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}
