package clusteragent

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
)

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
				Name: string(apicommonv1.ClusterAgentContainerName),
				// Image:           getImage(clusterAgentSpec.Image, dda.Spec.Registry),
				// ImagePullPolicy: *clusterAgentSpec.Image.PullPolicy,
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
			Name:  apicommon.DDClusterAgentKubeServiceName,
			Value: component.GetClusterAgentServiceName(dda),
		},
		{
			Name:  apicommon.DDLeaderElection,
			Value: "true",
		},
		{
			Name:  apicommon.DDHealthPort,
			Value: strconv.Itoa(int(apicommon.DefaultAgentHealthPort)),
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
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}
}
