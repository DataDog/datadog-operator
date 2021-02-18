package datadogagent

import (
	"reflect"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/testutils"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
)

func TestKSMCoreGetEnvVarsForAgent(t *testing.T) {
	enabledFeature := true
	spec := generateSpec()
	spec.Spec.ClusterAgent.Config.ClusterChecksEnabled = &enabledFeature
	spec.Spec.Features.KubeStateMetricsCore.Enabled = &enabledFeature
	env, err := getEnvVarsForAgent(spec)
	require.Subset(t, env, []corev1.EnvVar{{
		Name:  datadoghqv1alpha1.DDIgnoreAutoConf,
		Value: "kubernetes_state",
	}})

	spec.Spec.Agent.Config.Env = append(spec.Spec.Agent.Config.Env, corev1.EnvVar{
		Name:  datadoghqv1alpha1.DDIgnoreAutoConf,
		Value: "redis custom",
	})
	env, err = getEnvVarsForAgent(spec)
	require.NoError(t, err)
	require.Subset(t, env, []corev1.EnvVar{{
		Name:  datadoghqv1alpha1.DDIgnoreAutoConf,
		Value: "redis custom kubernetes_state",
	}})
}

func generateSpec() *datadoghqv1alpha1.DatadogAgent {
	var boolPtr bool
	var intPtr int32
	return &datadoghqv1alpha1.DatadogAgent{
		Spec: datadoghqv1alpha1.DatadogAgentSpec{
			Features: &datadoghqv1alpha1.DatadogFeatures{
				KubeStateMetricsCore: &datadoghqv1alpha1.KubeStateMetricsCore{},
			},
			ClusterAgent: &datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				CustomConfig: &datadoghqv1alpha1.CustomConfigSpec{},
				Affinity:     &corev1.Affinity{},
				Replicas:     &intPtr,
			},
			Agent: &datadoghqv1alpha1.DatadogAgentSpecAgentSpec{
				Config: datadoghqv1alpha1.NodeAgentConfig{
					PodAnnotationsAsTags: map[string]string{},
					PodLabelsAsTags:      map[string]string{},
					CollectEvents:        &boolPtr,
					LeaderElection:       &boolPtr,
					Dogstatsd: &datadoghqv1alpha1.DogstatsdConfig{
						DogstatsdOriginDetection: &boolPtr,
						UnixDomainSocket: &datadoghqv1alpha1.DSDUnixDomainSocketSpec{
							Enabled: &boolPtr,
						},
					},
				},
				Log: datadoghqv1alpha1.LogSpec{
					Enabled:                       &boolPtr,
					LogsConfigContainerCollectAll: &boolPtr,
					ContainerCollectUsingFiles:    &boolPtr,
					OpenFilesLimit:                &intPtr,
				},
			},
		},
	}
}

func Test_getLocalFilepath(t *testing.T) {
	type args struct {
		filePath  string
		localPath string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "basic test",
			args: args{
				"/host/var/file.txt",
				"/local/foo",
			},
			want: "/local/foo/file.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getLocalFilepath(tt.args.filePath, tt.args.localPath); got != tt.want {
				t.Errorf("getLocalFilepath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getVolumeMountsForSecurityAgent(t *testing.T) {
	customConfig := &datadoghqv1alpha1.CustomConfigSpec{
		ConfigMap: &datadoghqv1alpha1.ConfigFileConfigMapSpec{
			Name:    "foo-cm",
			FileKey: "datadog.yaml",
		},
	}

	securityCompliance := &datadoghqv1alpha1.SecuritySpec{
		Compliance: datadoghqv1alpha1.ComplianceSpec{
			Enabled: datadoghqv1alpha1.NewBoolPointer(true),
			ConfigDir: &datadoghqv1alpha1.ConfigDirSpec{
				ConfigMapName: "compliance-cm",
			},
		},
	}
	securityRuntime := &datadoghqv1alpha1.SecuritySpec{
		Runtime: datadoghqv1alpha1.RuntimeSecuritySpec{
			Enabled: datadoghqv1alpha1.NewBoolPointer(true),
			PoliciesDir: &datadoghqv1alpha1.ConfigDirSpec{
				ConfigMapName: "runtime-cm",
			},
		},
	}

	tests := []struct {
		name string
		dda  *datadoghqv1alpha1.DatadogAgent
		want []corev1.VolumeMount
	}{
		{
			name: "default volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", nil),
			want: []corev1.VolumeMount{
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run"},
			},
		},
		{
			name: "custom config volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{CustomConfig: customConfig}),
			want: []corev1.VolumeMount{
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "custom-datadog-yaml", ReadOnly: true, MountPath: "/etc/datadog-agent/datadog.yaml", SubPath: "datadog.yaml", SubPathExpr: ""},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run"},
			},
		},
		{
			name: "extra volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{VolumeMounts: []corev1.VolumeMount{{Name: "extra", MountPath: "/etc/datadog-agent/extra"}}}),
			want: []corev1.VolumeMount{
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "extra", MountPath: "/etc/datadog-agent/extra"},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run"},
			},
		},
		{
			name: "compliance volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{SecuritySpec: securityCompliance}),
			want: []corev1.VolumeMount{
				v1.VolumeMount{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				v1.VolumeMount{Name: "cgroups", ReadOnly: true, MountPath: "/host/sys/fs/cgroup"},
				v1.VolumeMount{Name: "passwd", ReadOnly: true, MountPath: "/etc/passwd"},
				v1.VolumeMount{Name: "group", ReadOnly: true, MountPath: "/etc/group"},
				v1.VolumeMount{Name: "procdir", ReadOnly: true, MountPath: "/host/proc"},
				v1.VolumeMount{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				v1.VolumeMount{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run"},
				v1.VolumeMount{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/root/var/run"},
				v1.VolumeMount{Name: "compliancedir", ReadOnly: true, MountPath: "/etc/datadog-agent/compliance.d"},
			},
		},
		{
			name: "compliance volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{SecuritySpec: securityRuntime}),
			want: []corev1.VolumeMount{
				v1.VolumeMount{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				v1.VolumeMount{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run"},
				v1.VolumeMount{Name: "sysprobe-socket-dir", ReadOnly: true, MountPath: "/var/run/sysprobe"},
				v1.VolumeMount{Name: "runtimepoliciesdir", ReadOnly: true, MountPath: "/etc/datadog-agent/runtime-security.d"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getVolumeMountsForSecurityAgent(tt.dda)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getVolumeMountsForSecurityAgent() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
