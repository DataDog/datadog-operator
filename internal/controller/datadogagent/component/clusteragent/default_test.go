package clusteragent

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/testutils"
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
	dda.SetCreationTimestamp(metav1.Now())
	return dda
}

func Test_defaultClusterAgentDeployment(t *testing.T) {
	dda := defaultDatadogAgent()
	deployment := NewDefaultClusterAgentDeployment(dda)
	expectedDeployment := clusterAgentExpectedPodTemplate(dda)

	assert.Empty(t, testutils.CompareKubeResource(&deployment.Spec.Template, expectedDeployment))
}
func Test_getPodDisruptionBudget(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}
	testpdb := GetClusterAgentPodDisruptionBudget(&dda)
	assert.Equal(t, "my-datadog-agent-cluster-agent-pdb", testpdb.Name)
	assert.Equal(t, intstr.FromInt(pdbMinAvailableInstances), *testpdb.Spec.MinAvailable)
	assert.Nil(t, testpdb.Spec.MaxUnavailable)
}

func clusterAgentExpectedPodTemplate(dda *datadoghqv2alpha1.DatadogAgent) *corev1.PodTemplateSpec {
	podTemplate := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"agent.datadoghq.com/component": "cluster-agent",
				"agent.datadoghq.com/name":      "foo",
				"app.kubernetes.io/component":   "cluster-agent",
				"app.kubernetes.io/instance":    "foo-cluster-agent",
				"app.kubernetes.io/managed-by":  "datadog-operator",
				"app.kubernetes.io/name":        "datadog-agent-deployment",
				"app.kubernetes.io/part-of":     "bar-foo",
				"app.kubernetes.io/version":     "",
			},
			Annotations: make(map[string]string),
		},
		Spec: clusterAgentDefaultPodSpec(dda),
	}

	return podTemplate
}

func clusterAgentDefaultPodSpec(dda *datadoghqv2alpha1.DatadogAgent) corev1.PodSpec {
	return corev1.PodSpec{
		// from default
		Affinity:           DefaultAffinity(),
		ServiceAccountName: "foo-cluster-agent",
		Containers: []corev1.Container{
			{
				Name:      "cluster-agent",
				Image:     defaulting.GetLatestClusterAgentImage(),
				Resources: corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 5005,
						Name:          "agentport",
						Protocol:      "TCP",
					},
				},
				Env: clusterAgentDefaultEnvVars(dda),
				VolumeMounts: []corev1.VolumeMount{
					{Name: "installinfo", ReadOnly: true, SubPath: "install_info", MountPath: "/etc/datadog-agent/install_info"},
					{Name: "confd", ReadOnly: true, MountPath: "/conf.d"},
					{Name: "logdatadog", ReadOnly: false, MountPath: "/var/log/datadog"},
					{Name: "tmp", ReadOnly: false, MountPath: "/tmp"},
					{Name: "certificates", ReadOnly: false, MountPath: "/etc/datadog-agent/certificates"},
					{Name: "datadog-agent-auth", MountPath: "/etc/datadog-agent/auth"},
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
			{
				Name: "datadog-agent-auth",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
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

func clusterAgentDefaultEnvVars(dda *datadoghqv2alpha1.DatadogAgent) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DD_AUTH_TOKEN_FILE_PATH",
			Value: "/etc/datadog-agent/auth/token",
		},
		{
			Name: "DD_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name:  "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME",
			Value: fmt.Sprintf("%s-%s", testDdaName, datadoghqv2alpha1.DefaultClusterAgentResourceSuffix),
		},
		{
			Name:  "DD_LEADER_ELECTION",
			Value: "true",
		},
		{
			Name:  "DD_HEALTH_PORT",
			Value: "5555",
		},
		{
			Name:  "DD_KUBE_RESOURCES_NAMESPACE",
			Value: testDdaNamespace,
		},
		{
			Name:  "DD_INSTRUMENTATION_INSTALL_TYPE",
			Value: common.DefaultAgentInstallType,
		},
		{
			Name:  "DD_INSTRUMENTATION_INSTALL_TIME",
			Value: strconv.Itoa(int(dda.GetCreationTimestamp().Unix())),
		},
		{
			Name:  "DD_INSTRUMENTATION_INSTALL_ID",
			Value: "",
		},
	}
}
