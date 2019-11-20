// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	authDelegatorName   = "%s-auth-delegator"
	datadogOperatorName = "DatadogAgentDeployment"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// newAgentPodTemplate generates a PodTemplate from a DatadogAgentDeployment spec
func newAgentPodTemplate(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment) corev1.PodTemplateSpec {
	// copy Agent Spec to configure Agent Pod Template
	spec := agentdeployment.Spec.DeepCopy()
	labels := getDefaultLabels(agentdeployment, "agent", getAgentVersion(agentdeployment))
	labels[datadoghqv1alpha1.AgentDeploymentNameLabelKey] = agentdeployment.Name
	labels[datadoghqv1alpha1.AgentDeploymentComponentLabelKey] = "agent"

	annotations := getDefaultAnnotations(agentdeployment)

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: agentdeployment.Name,
			Namespace:    agentdeployment.Namespace,
			Labels:       labels,
			Annotations:  annotations,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: getAgentServiceAccount(agentdeployment),
			InitContainers:     getInitContainers(logger, agentdeployment),
			Containers: []corev1.Container{
				{
					Name:            "agent",
					Image:           spec.Agent.Image.Name,
					ImagePullPolicy: *spec.Agent.Image.PullPolicy,
					Command: []string{
						"agent",
						"start",
					},
					Resources: *spec.Agent.Config.Resources,
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 8125,
							Name:          "dogstatsdport",
							Protocol:      "UDP",
						},
					},
					Env:           getEnvVarsForAgent(logger, agentdeployment),
					VolumeMounts:  getVolumeMountsForAgent(spec),
					LivenessProbe: getDefaultLivenessProbe(),
				},
			},
			Volumes:     getVolumesForAgent(spec),
			Tolerations: agentdeployment.Spec.Agent.Config.Tolerations,
		},
	}
}

func getInitContainers(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) []corev1.Container {
	spec := &dad.Spec
	volumeMounts := getVolumeMountsForAgent(spec)
	containers := []corev1.Container{
		{
			Name:            "init-volume",
			Image:           spec.Agent.Image.Name,
			ImagePullPolicy: *spec.Agent.Image.PullPolicy,
			Resources:       *spec.Agent.Config.Resources,
			Command:         []string{"bash", "-c"},
			Args:            []string{"cp -r /etc/datadog-agent /opt"},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      datadoghqv1alpha1.ConfigVolumeName,
					MountPath: "/opt/datadog-agent",
				},
			},
		},
		{
			Name:            "init-config",
			Image:           spec.Agent.Image.Name,
			ImagePullPolicy: *spec.Agent.Image.PullPolicy,
			Resources:       *spec.Agent.Config.Resources,
			Command:         []string{"bash", "-c"},
			Args:            []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
			Env:             getEnvVarsForAgent(logger, dad),
			VolumeMounts:    volumeMounts,
		},
	}

	return containers
}

// getEnvVarsForAgent converts Agent Config into container env vars
func getEnvVarsForAgent(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) []corev1.EnvVar {
	spec := dad.Spec
	// Marshal tag fields
	podLabelsAsTags, err := json.Marshal(spec.Agent.Config.PodLabelsAsTags)
	if err != nil {
		logger.Error(err, "failed to marshal pod labels as tags")
	}
	podAnnotationsAsTags, err := json.Marshal(spec.Agent.Config.PodAnnotationsAsTags)
	if err != nil {
		logger.Error(err, "failed to marshal pod annotations as tags")
	}
	tags, err := json.Marshal(spec.Agent.Config.Tags)
	if err != nil {
		logger.Error(err, "failed to marshal tags")
	}

	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.KubernetesEnvvarName,
			Value: "yes",
		},
		{
			Name:  datadoghqv1alpha1.DDClusterName,
			Value: spec.ClusterName,
		},
		{
			Name:  datadoghqv1alpha1.DDSite,
			Value: spec.Site,
		},
		{
			Name:  datadoghqv1alpha1.DDddURL,
			Value: *spec.Agent.Config.DDUrl,
		},
		{
			Name:  datadoghqv1alpha1.DDHealthPort,
			Value: strconv.Itoa(int(datadoghqv1alpha1.DefaultAgentHealthPort)),
		},
		{
			Name:  datadoghqv1alpha1.DDLogLevel,
			Value: *spec.Agent.Config.LogLevel,
		},
		{
			Name:  datadoghqv1alpha1.DDPodLabelsAsTags,
			Value: string(podLabelsAsTags),
		},
		{
			Name:  datadoghqv1alpha1.DDPodAnnotationsAsTags,
			Value: string(podAnnotationsAsTags),
		},
		{
			Name:  datadoghqv1alpha1.DDTags,
			Value: string(tags),
		},
		{
			Name:  datadoghqv1alpha1.DDCollectKubeEvents,
			Value: strconv.FormatBool(*spec.Agent.Config.CollectEvents),
		},
		{
			Name:  datadoghqv1alpha1.DDLeaderElection,
			Value: strconv.FormatBool(*spec.Agent.Config.LeaderElection),
		},
		{
			Name:  datadoghqv1alpha1.DDLogsEnabled,
			Value: strconv.FormatBool(*spec.Agent.Log.Enabled),
		},
		{
			Name:  datadoghqv1alpha1.DDLogsConfigContainerCollectAll,
			Value: strconv.FormatBool(*spec.Agent.Log.LogsConfigContainerCollectAll),
		},
		{
			Name:  datadoghqv1alpha1.DDDogstatsdOriginDetection,
			Value: strconv.FormatBool(*spec.Agent.Config.Dogstatsd.DogstatsdOriginDetection),
		},
	}

	if spec.Credentials.APIKeyExistingSecret != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:      datadoghqv1alpha1.DDAPIKey,
			ValueFrom: getAPIKeyFromSecret(dad),
		})
	} else {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDAPIKey,
			Value: spec.Credentials.APIKey,
		})
	}
	if spec.ClusterAgent != nil {
		clusterEnv := []corev1.EnvVar{
			{
				Name:  datadoghqv1alpha1.DDClusterAgentEnabled,
				Value: strconv.FormatBool(true),
			},
			{
				Name:  datadoghqv1alpha1.DDClusterAgentKubeServiceName,
				Value: getClusterAgentServiceName(dad),
			},
			{
				Name:      datadoghqv1alpha1.DDClusterAgentAuthToken,
				ValueFrom: getClusterAgentAuthToken(dad),
			},
		}
		if *spec.ClusterAgent.Config.ClusterChecksRunnerEnabled && spec.ClusterChecksRunner == nil {
			clusterEnv = append(clusterEnv, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDExtraConfigProviders,
				Value: datadoghqv1alpha1.ClusterChecksConfigProvider,
			})
		}
		envVars = append(envVars, clusterEnv...)
	}
	return append(envVars, spec.Agent.Config.Env...)
}

// getVolumesForAgent defines volumes for the Agent
func getVolumesForAgent(spec *datadoghqv1alpha1.DatadogAgentDeploymentSpec) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: datadoghqv1alpha1.ConfdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.ConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.ProcVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/proc",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.CgroupsVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys/fs/cgroup",
				},
			},
		},
	}
	if spec.Agent.Config.CriSocket != nil && spec.Agent.Config.CriSocket.UseCriSocketVolume != nil && *spec.Agent.Config.CriSocket.UseCriSocketVolume {
		path := "/var/run/docker.sock"
		if spec.Agent.Config.CriSocket.CriSocketPath != nil {
			path = *spec.Agent.Config.CriSocket.CriSocketPath
		}
		criVolume := corev1.Volume{
			Name: datadoghqv1alpha1.CriSockerVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: path,
				},
			},
		}
		volumes = append(volumes, criVolume)
	}
	return volumes
}

// getVolumeMountsForAgent defines mounted volumes for the Agent
func getVolumeMountsForAgent(spec *datadoghqv1alpha1.DatadogAgentDeploymentSpec) []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.ConfdVolumeName,
			MountPath: datadoghqv1alpha1.ConfdVolumePath,
		},
		{
			Name:      datadoghqv1alpha1.ConfigVolumeName,
			MountPath: datadoghqv1alpha1.ConfigVolumePath,
		},
		{
			Name:      datadoghqv1alpha1.ProcVolumeName,
			MountPath: datadoghqv1alpha1.ProcVolumePath,
			ReadOnly:  datadoghqv1alpha1.ProcVolumeReadOnly,
		},
		{
			Name:      datadoghqv1alpha1.CgroupsVolumeName,
			MountPath: datadoghqv1alpha1.CgroupsVolumePath,
			ReadOnly:  datadoghqv1alpha1.CgroupsVolumeReadOnly,
		},
	}

	// Cri socket volume
	if *spec.Agent.Config.CriSocket.UseCriSocketVolume {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.CriSockerVolumeName,
			MountPath: *spec.Agent.Config.CriSocket.CriSocketPath,
			ReadOnly:  true,
		})
	}

	// Dogstatsd volume
	if *spec.Agent.Config.Dogstatsd.UseDogStatsDSocketVolume {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.DogstatsdSockerVolumeName,
			MountPath: datadoghqv1alpha1.DogstatsdSockerVolumePath,
		})
	}

	// Log volumes
	if *spec.Agent.Log.Enabled {
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      datadoghqv1alpha1.PointerVolumeName,
				MountPath: datadoghqv1alpha1.PointerVolumePath,
			},
			{
				Name:      datadoghqv1alpha1.LogPodVolumeName,
				MountPath: datadoghqv1alpha1.LogPodVolumePath,
				ReadOnly:  datadoghqv1alpha1.LogPodVolumeReadOnly,
			},
			{
				Name:      datadoghqv1alpha1.LogContainerVolumeName,
				MountPath: *spec.Agent.Log.ContainerLogsPath,
				ReadOnly:  datadoghqv1alpha1.LogContainerolumeReadOnly,
			},
		}...)
	}
	return append(volumeMounts, spec.Agent.Config.VolumeMounts...)
}

func getAgentVersion(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	// TODO implement this method
	return ""
}

func getAgentServiceAccount(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	saDefault := fmt.Sprintf("%s-agent", dad.Name)
	if dad.Spec.Agent == nil {
		return saDefault
	}
	if dad.Spec.Agent.Rbac.ServiceAccountName != nil {
		return *dad.Spec.Agent.Rbac.ServiceAccountName
	}
	return saDefault
}

// getAPIKeyFromSecret returns the Agent API key as an env var source
func getAPIKeyFromSecret(dad *datadoghqv1alpha1.DatadogAgentDeployment) *corev1.EnvVarSource {
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: getAPIKeySecretName(dad),
			},
			Key: datadoghqv1alpha1.DefaultAPIKeyKey,
		},
	}
	return authTokenValue
}

// getClusterAgentAuthToken returns the Cluster Agent auth token as an env var source
func getClusterAgentAuthToken(dad *datadoghqv1alpha1.DatadogAgentDeployment) *corev1.EnvVarSource {
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{},
	}
	authTokenValue.SecretKeyRef.Name = getAppKeySecretName(dad)
	authTokenValue.SecretKeyRef.Key = "token"
	return authTokenValue
}

func getAppKeySecretName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	if dad.Spec.Credentials.AppKeyExistingSecret != "" {
		return dad.Spec.Credentials.AppKeyExistingSecret
	}
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

// getAppKeyFromSecret returns the Agent API key as an env var source
func getAppKeyFromSecret(dad *datadoghqv1alpha1.DatadogAgentDeployment) *corev1.EnvVarSource {
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: getAppKeySecretName(dad),
			},
			Key: datadoghqv1alpha1.DefaultAPPKeyKey,
		},
	}
	return authTokenValue
}

func getAPIKeySecretName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	if dad.Spec.Credentials.APIKeyExistingSecret != "" {
		return dad.Spec.Credentials.APIKeyExistingSecret
	}
	return dad.Name
}

func getClusterAgentServiceName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

func getClusterAgentServiceAccount(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	saDefault := fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
	if dad.Spec.ClusterAgent == nil {
		return saDefault
	}
	if dad.Spec.ClusterAgent.Rbac.ServiceAccountName != nil {
		return *dad.Spec.ClusterAgent.Rbac.ServiceAccountName
	}
	return saDefault
}

func getClusterAgentVersion(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	// TODO implement this method
	return ""
}

func getMetricsServerServiceName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultMetricsServerResourceSuffix)
}

func getClusterAgentRbacResourcesName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

func getAgentRbacResourcesName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultAgentResourceSuffix)
}

func getClusterChecksRunnerRbacResourcesName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix)
}

func getHPAClusterRoleBindingName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf(authDelegatorName, getClusterAgentRbacResourcesName(dad))
}

func getClusterChecksRunnerServiceAccount(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	saDefault := fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix)
	if dad.Spec.ClusterChecksRunner == nil {
		return saDefault
	}
	if dad.Spec.ClusterChecksRunner.Rbac.ServiceAccountName != nil {
		return *dad.Spec.ClusterChecksRunner.Rbac.ServiceAccountName
	}
	return saDefault
}

func getDefaultLabels(dad *datadoghqv1alpha1.DatadogAgentDeployment, instanceName, version string) map[string]string {
	// TODO implement this method
	labels := make(map[string]string)
	labels[kubernetes.AppKubernetesNameLabelKey] = "datadog-agent-deployment"
	labels[kubernetes.AppKubernetesInstanceLabelKey] = instanceName
	labels[kubernetes.AppKubernetesPartOfLabelKey] = dad.Name
	labels[kubernetes.AppKubernetesVersionLabelKey] = version
	labels[kubernetes.AppKubernetesManageByLabelKey] = "datadog-operator"
	return labels
}

func getDefaultAnnotations(dad *datadoghqv1alpha1.DatadogAgentDeployment) map[string]string {
	// TODO implement this method
	return make(map[string]string)
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func generateRandomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func shouldReturn(result reconcile.Result, err error) bool {
	if err != nil || result.Requeue || result.RequeueAfter > 0 {
		return true
	}
	return false
}

func isMetricsProviderEnabled(spec *datadoghqv1alpha1.DatadogAgentDeploymentSpecClusterAgentSpec) bool {
	if spec == nil {
		return false
	}
	if datadoghqv1alpha1.BoolValue(spec.Config.MetricsProviderEnabled) {
		return true
	}
	return false
}

func isCreateRBACEnabled(config datadoghqv1alpha1.RbacConfig) bool {
	return datadoghqv1alpha1.BoolValue(config.Create)
}

func getDefaultLivenessProbe() *corev1.Probe {
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: datadoghqv1alpha1.DefaultLivenessProveInitialDelaySeconds,
		PeriodSeconds:       datadoghqv1alpha1.DefaultLivenessProvePeriodSeconds,
		TimeoutSeconds:      datadoghqv1alpha1.DefaultLivenessProveTimeoutSeconds,
		SuccessThreshold:    datadoghqv1alpha1.DefaultLivenessProveSuccessThreshold,
		FailureThreshold:    datadoghqv1alpha1.DefaultLivenessProveFailureThreshold,
	}
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: datadoghqv1alpha1.DefaultLivenessProveHTTPPath,
		Port: intstr.IntOrString{
			IntVal: datadoghqv1alpha1.DefaultAgentHealthPort,
		},
	}
	return livenessProbe
}

func getPodAffinity(affinity *corev1.Affinity, labelValue string) *corev1.Affinity {
	if affinity != nil {
		return affinity
	}

	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": labelValue,
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}
}

func updateDeploymentStatus(dep *appsv1.Deployment, depStatus *datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus, updateTime *metav1.Time) *datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus {
	if depStatus == nil {
		depStatus = &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{}
	}
	depStatus.CurrentHash = getHashAnnotation(dep.Annotations)
	if updateTime != nil {
		depStatus.LastUpdate = updateTime
	}
	depStatus.Replicas = dep.Status.Replicas
	depStatus.UpdatedReplicas = dep.Status.UpdatedReplicas
	depStatus.AvailableReplicas = dep.Status.AvailableReplicas
	depStatus.UnavailableReplicas = dep.Status.UnavailableReplicas
	depStatus.ReadyReplicas = dep.Status.ReadyReplicas
	depStatus.State = datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStateRunning
	return depStatus
}

func ownedByDatadogOperator(owners []metav1.OwnerReference) bool {
	for _, owner := range owners {
		if owner.Kind == datadogOperatorName {
			return true
		}
	}
	return false
}
