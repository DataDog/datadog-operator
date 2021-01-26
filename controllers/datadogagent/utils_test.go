package datadogagent

import (
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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
