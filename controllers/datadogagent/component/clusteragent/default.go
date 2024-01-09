// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// NewDefaultClusterAgentDeployment return a new default cluster-agent deployment
func NewDefaultClusterAgentDeployment(dda metav1.Object) *appsv1.Deployment {
	deployment := component.NewDeployment(dda, apicommon.DefaultClusterAgentResourceSuffix, GetClusterAgentName(dda), GetClusterAgentVersion(dda), nil)

	podTemplate := NewDefaultClusterAgentPodTemplateSpec(dda)
	for key, val := range deployment.GetLabels() {
		podTemplate.Labels[key] = val
	}

	for key, val := range deployment.GetAnnotations() {
		podTemplate.Annotations[key] = val
	}
	deployment.Spec.Template = *podTemplate
	deployment.Spec.Replicas = apiutils.NewInt32Pointer(apicommon.DefaultClusterAgentReplicas)

	return deployment
}

// NewDefaultClusterAgentPodTemplateSpec return a default PodTemplateSpec for the cluster-agent deployment
func NewDefaultClusterAgentPodTemplateSpec(dda metav1.Object) *corev1.PodTemplateSpec {
	volumes := []corev1.Volume{
		component.GetVolumeInstallInfo(dda),
		component.GetVolumeForConfd(),
		component.GetVolumeForLogs(),
		component.GetVolumeForCertificates(),

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
		component.GetVolumeMountForConfd(),
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForCertificates(),
		component.GetVolumeMountForTmp(),
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

// GetDefaultServiceAccountName return the default Cluster-Agent ServiceAccountName
func GetDefaultServiceAccountName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

func defaultPodSpec(dda metav1.Object, volumes []corev1.Volume, volumeMounts []corev1.VolumeMount, envVars []corev1.EnvVar) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		ServiceAccountName: GetDefaultServiceAccountName(dda),
		Containers: []corev1.Container{
			{
				Name:  string(apicommonv1.ClusterAgentContainerName),
				Image: fmt.Sprintf("%s/%s:%s", apicommon.DefaultImageRegistry, apicommon.DefaultClusterAgentImageName, defaulting.ClusterAgentLatestVersion),
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 5005,
						Name:          "agentport",
						Protocol:      "TCP",
					},
				},
				Env:          envVars,
				VolumeMounts: volumeMounts,
				Command:      nil,
				Args:         nil,
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
			Name: apicommon.DDPodName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name:  apicommon.DDClusterAgentKubeServiceName,
			Value: GetClusterAgentServiceName(dda),
		},
		{
			Name:  apicommon.DDKubeResourcesNamespace,
			Value: utils.GetDatadogAgentResourceNamespace(dda),
		},
		{
			Name:  apicommon.DDLeaderElection,
			Value: "true",
		},
		{
			Name:  apicommon.DDHealthPort,
			Value: strconv.Itoa(int(apicommon.DefaultAgentHealthPort)),
		},
		{
			Name:  apicommon.DDAPMInstrumentationInstallId,
			Value: component.AgentInstallId,
		},
		{
			Name:  apicommon.DDAPMInstrumentationInstallTime,
			Value: component.AgentInstallTime,
		},
		{
			Name:  apicommon.DDAPMInstrumentationInstallType,
			Value: component.DefaultAgentInstallType,
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
								apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}

// GetDefaultClusterAgentRolePolicyRules returns the default policy rules for the Cluster Agent
// Can be used by the Agent if the Cluster Agent is disabled
func GetDefaultClusterAgentRolePolicyRules(dda metav1.Object) []rbacv1.PolicyRule {
	rules := []rbacv1.PolicyRule{}

	rules = append(rules, GetLeaderElectionPolicyRule(dda)...)
	rules = append(rules, rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{rbac.ConfigMapsResource},
		ResourceNames: []string{
			common.DatadogClusterIDResourceName,
		},
		Verbs: []string{rbac.GetVerb, rbac.UpdateVerb, rbac.CreateVerb},
	})
	return rules
}

// GetDefaultClusterAgentClusterRolePolicyRules returns the default policy rules for the Cluster Agent
// Can be used by the Agent if the Cluster Agent is disabled
func GetDefaultClusterAgentClusterRolePolicyRules(dda metav1.Object) []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.ServicesResource,
				rbac.EventsResource,
				rbac.EndpointsResource,
				rbac.PodsResource,
				rbac.NodesResource,
				rbac.ComponentStatusesResource,
				rbac.ConfigMapsResource,
				rbac.NamespaceResource,
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
			},
		},
		{
			APIGroups: []string{rbac.OpenShiftQuotaAPIGroup},
			Resources: []string{rbac.ClusterResourceQuotasResource},
			Verbs:     []string{rbac.GetVerb, rbac.ListVerb},
		},
		{
			NonResourceURLs: []string{rbac.VersionURL, rbac.HealthzURL},
			Verbs:           []string{rbac.GetVerb},
		},
		{
			// Horizontal Pod Autoscaling
			APIGroups: []string{rbac.AutoscalingAPIGroup},
			Resources: []string{rbac.HorizontalPodAutoscalersRecource},
			Verbs:     []string{rbac.ListVerb, rbac.WatchVerb},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.NamespaceResource},
			ResourceNames: []string{
				common.KubeSystemResourceName,
			},
			Verbs: []string{rbac.GetVerb},
		},
	}
}

// GetLeaderElectionPolicyRule returns the policy rules for leader election
func GetLeaderElectionPolicyRule(dda metav1.Object) []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			ResourceNames: []string{
				common.DatadogLeaderElectionOldResourceName, // Kept for backward compatibility with agent <7.37.0
				utils.GetDatadogLeaderElectionResourceName(dda),
			},
			Verbs: []string{rbac.GetVerb, rbac.UpdateVerb},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.ConfigMapsResource},
			Verbs:     []string{rbac.CreateVerb},
		},
		{
			APIGroups: []string{rbac.CoordinationAPIGroup},
			Resources: []string{rbac.LeasesResource},
			Verbs:     []string{rbac.CreateVerb},
		},
		{
			APIGroups: []string{rbac.CoordinationAPIGroup},
			Resources: []string{rbac.LeasesResource},
			ResourceNames: []string{
				utils.GetDatadogLeaderElectionResourceName(dda),
			},
			Verbs: []string{rbac.GetVerb, rbac.UpdateVerb},
		},
	}
}
