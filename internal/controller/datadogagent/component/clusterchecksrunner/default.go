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
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
)

// GetClusterChecksRunnerName return the Cluster-Checks-Runner name based on the DatadogAgent name
func GetClusterChecksRunnerName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultClusterChecksRunnerResourceSuffix)
}

// GetCCRRbacResourcesName returns the Cluster Checks Runner RBAC resource name
func GetCCRRbacResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultClusterChecksRunnerResourceSuffix)
}

// NewDefaultClusterChecksRunnerDeployment return a new default cluster-checks-runner deployment
func NewDefaultClusterChecksRunnerDeployment(dda metav1.Object) *appsv1.Deployment {
	deployment := common.NewDeployment(dda, constants.DefaultClusterChecksRunnerResourceSuffix, GetClusterChecksRunnerName(dda), common.GetAgentVersion(dda), nil)

	podTemplate := NewDefaultClusterChecksRunnerPodTemplateSpec(dda)
	for key, val := range deployment.GetLabels() {
		podTemplate.Labels[key] = val
	}

	for key, val := range deployment.GetAnnotations() {
		podTemplate.Annotations[key] = val
	}

	deployment.Spec.Template = *podTemplate
	deployment.Spec.Replicas = apiutils.NewInt32Pointer(defaultClusterChecksRunnerReplicas)

	return deployment
}

// NewDefaultClusterChecksRunnerPodTemplateSpec returns a default cluster-checks-runner for the cluster-agent deployment
func NewDefaultClusterChecksRunnerPodTemplateSpec(dda metav1.Object) *corev1.PodTemplateSpec {
	volumes := []corev1.Volume{
		common.GetVolumeInstallInfo(dda),
		common.GetVolumeForConfig(),
		common.GetVolumeForRmCorechecks(),
		common.GetVolumeForLogs(),
		common.GetVolumeForChecksd(),

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
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForTmp(),
		common.GetVolumeMountForRmCorechecks(),
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

func volumeMountsForInitConfig() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForInstallInfo(),
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForTmp(),
		common.GetVolumeMountForRmCorechecks(),
		common.GetVolumeMountForChecksd(),
	}
}

func GetClusterChecksRunnerPodDisruptionBudgetName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s-pdb", dda.GetName(), constants.DefaultClusterChecksRunnerResourceSuffix)
}

func GetClusterChecksRunnerPodDisruptionBudget(dda metav1.Object, useV1BetaPDB bool) client.Object {
	maxUnavailableStr := intstr.FromInt(pdbMaxUnavailableInstances)
	matchLabels := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:      dda.GetName(),
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix}
	if useV1BetaPDB {
		return &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GetClusterChecksRunnerPodDisruptionBudgetName(dda),
				Namespace: dda.GetNamespace(),
			},
			Spec: policyv1beta1.PodDisruptionBudgetSpec{
				MaxUnavailable: &maxUnavailableStr,
				Selector: &metav1.LabelSelector{
					MatchLabels: matchLabels,
				},
			},
		}
	}
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetClusterChecksRunnerPodDisruptionBudgetName(dda),
			Namespace: dda.GetNamespace(),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailableStr,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}
}

// getDefaultServiceAccountName return the default Cluster-Agent ServiceAccountName
func getDefaultServiceAccountName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultClusterChecksRunnerResourceSuffix)
}

func clusterChecksRunnerImage() string {
	return images.GetLatestAgentImage()
}

func defaultPodSpec(dda metav1.Object, volumes []corev1.Volume, volumeMounts []corev1.VolumeMount, envVars []corev1.EnvVar) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		ServiceAccountName: getDefaultServiceAccountName(dda),
		InitContainers: []corev1.Container{
			{
				Name:    "init-config",
				Image:   clusterChecksRunnerImage(),
				Command: []string{"bash", "-c"},
				Args: []string{
					"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done",
				},
				VolumeMounts: volumeMountsForInitConfig(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:         string(apicommon.ClusterChecksRunnersContainerName),
				Image:        clusterChecksRunnerImage(),
				Env:          envVars,
				VolumeMounts: volumeMounts,
				Command:      []string{"bash", "-c"},
				Args: []string{
					"agent run",
				},
				LivenessProbe:  constants.GetDefaultLivenessProbe(),
				ReadinessProbe: constants.GetDefaultReadinessProbe(),
				StartupProbe:   constants.GetDefaultStartupProbe(),
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
			Name:  common.DDClusterAgentKubeServiceName,
			Value: componentdca.GetClusterAgentServiceName(dda),
		},
		{
			Name:  common.DDClusterAgentEnabled,
			Value: "true",
		},
		{
			Name:  common.DDHealthPort,
			Value: strconv.Itoa(int(constants.DefaultAgentHealthPort)),
		},
		{
			Name:  common.KubernetesEnvVar,
			Value: "yes",
		},
		{
			Name:  DDEnableMetadataCollection,
			Value: "false",
		},
		{
			Name:  DDClcRunnerEnabled,
			Value: "true",
		},
		{
			Name: DDClcRunnerHost,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: common.FieldPathStatusPodIP,
				},
			},
		},
		{
			Name: DDClcRunnerID,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: common.FieldPathMetaName,
				},
			},
		},
		{
			Name:  common.DDDogstatsdEnabled,
			Value: "false",
		},
		{
			Name:  common.DDProcessCollectionEnabled,
			Value: "false",
		},
		{
			Name:  common.DDProcessConfigRunInCoreAgent,
			Value: "false",
		},
		{
			Name:  common.DDContainerCollectionEnabled,
			Value: "true",
		},
		{
			Name:  common.DDLogsEnabled,
			Value: "false",
		},
		{
			Name:  common.DDAPMEnabled,
			Value: "false",
		},
		{
			Name: constants.DDHostName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: common.FieldPathSpecNodeName,
				},
			},
		},
		{
			Name:  common.DDAPMErrorTrackingStandaloneEnabled,
			Value: "false",
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
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix,
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}
