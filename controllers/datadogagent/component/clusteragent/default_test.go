package clusteragent

import (
	"fmt"
	"strconv"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/pkg/defaulting"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	testDdaName      = "foo"
	testDdaNamespace = "bar"
	agentConfigFile  = "/etc/datadog-agent/datadog.yaml"
)

func defaultDatadogAgent() *datadoghqv2alpha1.DatadogAgent {
	dda := &datadoghqv2alpha1.DatadogAgent{}
	dda.SetName("foo")
	dda.SetNamespace("bar")
	return dda
}

func Test_defaultClusterAgentDeployment(t *testing.T) {
	dda := defaultDatadogAgent()
	deployment := NewDefaultClusterAgentDeployment(dda)
	deployment.Spec.Template = *clusterAgentDefaultPodTemplateSpec()

	assert.Equal(t, clusterAgentDefaultPodTemplateSpec(), &deployment.Spec.Template)
}

func clusterAgentDefaultPodTemplateSpec() *corev1.PodTemplateSpec {
	podTemplate := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: clusterAgentDefaultPodSpec(),
	}

	return podTemplate
}

func clusterAgentDefaultPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		// from default
		Affinity:           DefaultAffinity(),
		ServiceAccountName: "foo-cluster-agent",
		Containers: []corev1.Container{
			{
				Name:            "cluster-agent",
				Image:           defaulting.GetLatestClusterAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 5005,
						Name:          "agentport",
						Protocol:      "TCP",
					},
				},
				Env: clusterAgentDefaultEnvVars(),
				VolumeMounts: []corev1.VolumeMount{
					{Name: "installinfo", ReadOnly: true, SubPath: "install_info", MountPath: "/etc/datadog-agent/install_info"},
					{Name: "confd", ReadOnly: true, MountPath: "/conf.d"},
					{Name: "orchestrator-explorer-config", ReadOnly: true, MountPath: "/etc/datadog-agent/conf.d/orchestrator.d"},
					{Name: "logdatadog", ReadOnly: false, MountPath: "/var/log/datadog"},
					{Name: "tmp", ReadOnly: false, MountPath: "/tmp"},
					{Name: "certificates", ReadOnly: false, MountPath: "/etc/datadog-agent/certificates"},
				},
				LivenessProbe:  defaultLivenessProbe(),
				ReadinessProbe: defaultReadinessProbe(),
				StartupProbe:   defaultStartupProbe(),
				SecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem:   apiutils.NewBoolPointer(true),
					AllowPrivilegeEscalation: apiutils.NewBoolPointer(false),
				},
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "installinfo",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "foo-install-info",
						},
					},
				},
			},
			{
				Name:         "confd",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
			{
				Name: "orchestrator-explorer-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "foo-orchestrator-explorer-config",
						},
					},
				},
			},
			{
				Name: "logdatadog",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: "tmp",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: "certificates",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
	}
}

func authTokenValue() *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: testDdaName,
			},
			Key: "token",
		},
	}
}

func apiKeyValue() *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: testDdaName,
			},
			Key: "api_key",
		},
	}
}

func defaultLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 15,
		PeriodSeconds:       15,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    6,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/live",
				Port: intstr.IntOrString{
					IntVal: 5555,
				},
			},
		},
	}
}

func defaultReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 15,
		PeriodSeconds:       15,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    6,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/ready",
				Port: intstr.IntOrString{
					IntVal: 5555,
				},
			},
		},
	}
}

func defaultStartupProbe() *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 15,
		PeriodSeconds:       15,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    6,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/startup",
				Port: intstr.IntOrString{
					IntVal: 5555,
				},
			},
		},
	}
}

func clusterAgentDefaultEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "DD_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name:  "DD_CLUSTER_CHECKS_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME",
			Value: fmt.Sprintf("%s-%s", testDdaName, apicommon.DefaultClusterAgentResourceSuffix),
		},
		{
			Name:      "DD_CLUSTER_AGENT_AUTH_TOKEN",
			ValueFrom: authTokenValue(),
		},
		{
			Name:  "DD_LEADER_ELECTION",
			Value: "true",
		},
		{
			Name:  "DD_LEADER_LEASE_NAME",
			Value: fmt.Sprintf("%s-leader-election", testDdaName),
		},
		{
			Name:  "DD_COMPLIANCE_CONFIG_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_COLLECT_KUBERNETES_EVENTS",
			Value: "false",
		},
		{
			Name:  "DD_HEALTH_PORT",
			Value: "5555",
		},
		{
			Name:  "DD_LOG_LEVEL",
			Value: "INFO",
		},
		{
			Name:      "DD_API_KEY",
			ValueFrom: apiKeyValue(),
		},
		{
			Name:  "DD_ORCHESTRATOR_EXPLORER_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_ORCHESTRATOR_EXPLORER_CONTAINER_SCRUBBING_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_CLUSTER_AGENT_TOKEN_NAME",
			Value: fmt.Sprintf("%stoken", testDdaName),
		},
		{
			Name:  "DD_KUBE_RESOURCES_NAMESPACE",
			Value: testDdaNamespace,
		},
		{
			Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
			Value: strconv.FormatInt(test.AgentInstallTime.Time.Unix(), 10),
		},
		{
			Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
			Value: component.DefaultAgentInstallType,
		},
		{
			Name:  "DD_INSTRUMENTATION_INSTALL_ID",
			Value: string(test.AgentInstallId),
		},
	}
}
