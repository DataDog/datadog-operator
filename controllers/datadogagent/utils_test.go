package datadogagent

import (
	"reflect"
	"testing"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/testutils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestKSMCoreGetEnvVarsForAgent(t *testing.T) {
	logger := logf.Log.WithName(t.Name())
	enabledFeature := true
	spec := generateSpec()
	spec.Spec.ClusterAgent.Config.ClusterChecksEnabled = &enabledFeature
	spec.Spec.Features.KubeStateMetricsCore.Enabled = &enabledFeature
	env, err := getEnvVarsForAgent(logger, spec)
	require.NoError(t, err)
	require.Subset(t, env, []v1.EnvVar{{
		Name:  datadoghqv1alpha1.DDIgnoreAutoConf,
		Value: "kubernetes_state",
	}})

	spec.Spec.Agent.Config.Env = append(spec.Spec.Agent.Config.Env, v1.EnvVar{
		Name:  datadoghqv1alpha1.DDIgnoreAutoConf,
		Value: "redis custom",
	})
	env, err = getEnvVarsForAgent(logger, spec)
	require.NoError(t, err)
	require.Subset(t, env, []v1.EnvVar{{
		Name:  datadoghqv1alpha1.DDIgnoreAutoConf,
		Value: "redis custom kubernetes_state",
	}})
}

func generateSpec() *datadoghqv1alpha1.DatadogAgent {
	var boolPtr bool
	var intPtr int32
	dda := &datadoghqv1alpha1.DatadogAgent{
		Spec: datadoghqv1alpha1.DatadogAgentSpec{
			Features: datadoghqv1alpha1.DatadogFeatures{
				KubeStateMetricsCore: &datadoghqv1alpha1.KubeStateMetricsCore{},
				LogCollection: &datadoghqv1alpha1.LogCollectionConfig{
					Enabled:                       &boolPtr,
					LogsConfigContainerCollectAll: &boolPtr,
					ContainerCollectUsingFiles:    &boolPtr,
					OpenFilesLimit:                &intPtr,
				},
			},
			ClusterAgent: datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				CustomConfig: &datadoghqv1alpha1.CustomConfigSpec{},
				Affinity:     &v1.Affinity{},
				Replicas:     &intPtr,
			},
			Agent: datadoghqv1alpha1.DatadogAgentSpecAgentSpec{
				Config: &datadoghqv1alpha1.NodeAgentConfig{
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
			},
		},
	}
	_ = datadoghqv1alpha1.DefaultDatadogAgent(dda)
	return dda
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
			Enabled: apiutils.NewBoolPointer(true),
			ConfigDir: &datadoghqv1alpha1.ConfigDirSpec{
				ConfigMapName: "compliance-cm",
			},
		},
	}
	securityRuntime := &datadoghqv1alpha1.SecuritySpec{
		Runtime: datadoghqv1alpha1.RuntimeSecuritySpec{
			Enabled: apiutils.NewBoolPointer(true),
			PoliciesDir: &datadoghqv1alpha1.ConfigDirSpec{
				ConfigMapName: "runtime-cm",
			},
		},
	}

	tests := []struct {
		name string
		dda  *datadoghqv1alpha1.DatadogAgent
		want []v1.VolumeMount
	}{
		{
			name: "default volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", nil),
			want: []v1.VolumeMount{
				{Name: "logdatadog", MountPath: "/var/log/datadog"},
				{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				{Name: "dsdsocket", ReadOnly: true, MountPath: "/var/run/datadog/statsd"},
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
			},
		},
		{
			name: "custom config volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{CustomConfig: customConfig}),
			want: []v1.VolumeMount{
				{Name: "logdatadog", MountPath: "/var/log/datadog"},
				{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				{Name: "dsdsocket", ReadOnly: true, MountPath: "/var/run/datadog/statsd"},
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				{Name: "custom-datadog-yaml", ReadOnly: true, MountPath: "/etc/datadog-agent/datadog.yaml", SubPath: "datadog.yaml", SubPathExpr: ""},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
			},
		},
		{
			name: "extra volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{VolumeMounts: []v1.VolumeMount{{Name: "extra", MountPath: "/etc/datadog-agent/extra"}}}),
			want: []v1.VolumeMount{
				{Name: "logdatadog", MountPath: "/var/log/datadog"},
				{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				{Name: "dsdsocket", ReadOnly: true, MountPath: "/var/run/datadog/statsd"},
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				{Name: "extra", MountPath: "/etc/datadog-agent/extra"},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
			},
		},
		{
			name: "compliance volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{SecuritySpec: securityCompliance}),
			want: []v1.VolumeMount{
				{Name: "logdatadog", ReadOnly: false, MountPath: "/var/log/datadog"},
				{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				{Name: "dsdsocket", ReadOnly: true, MountPath: "/var/run/datadog/statsd"},
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				{Name: "cgroups", ReadOnly: true, MountPath: "/host/sys/fs/cgroup"},
				{Name: "passwd", ReadOnly: true, MountPath: "/etc/passwd"},
				{Name: "group", ReadOnly: true, MountPath: "/etc/group"},
				{Name: "procdir", ReadOnly: true, MountPath: "/host/proc"},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/root/var/run/containerd"},
				{Name: "compliancedir", ReadOnly: true, MountPath: "/etc/datadog-agent/compliance.d"},
			},
		},
		{
			name: "runtime volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{SecuritySpec: securityRuntime}),
			want: []v1.VolumeMount{
				{Name: "logdatadog", ReadOnly: false, MountPath: "/var/log/datadog"},
				{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				{Name: "dsdsocket", ReadOnly: true, MountPath: "/var/run/datadog/statsd"},
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				{Name: "runtimepoliciesdir", ReadOnly: true, MountPath: "/etc/datadog-agent/runtime-security.d"},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
				{Name: "sysprobe-socket-dir", ReadOnly: true, MountPath: "/var/run/sysprobe"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getVolumeMountsForSecurityAgent(tt.dda)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getVolumeMountsForSecurityAgent() = %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func Test_prometheusScrapeEnvVars(t *testing.T) {
	tests := []struct {
		name       string
		promConfig *datadoghqv1alpha1.PrometheusScrapeConfig
		want       []v1.EnvVar
	}{
		{
			name: "Enabled + Service endpoints disabled",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:          apiutils.NewBoolPointer(true),
				ServiceEndpoints: apiutils.NewBoolPointer(false),
			},
			want: []v1.EnvVar{
				{Name: "DD_PROMETHEUS_SCRAPE_ENABLED", Value: "true"},
				{Name: "DD_PROMETHEUS_SCRAPE_SERVICE_ENDPOINTS", Value: "false"},
			},
		},
		{
			name: "Enabled + Service endpoints enabled",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:          apiutils.NewBoolPointer(true),
				ServiceEndpoints: apiutils.NewBoolPointer(true),
			},
			want: []v1.EnvVar{
				{Name: "DD_PROMETHEUS_SCRAPE_ENABLED", Value: "true"},
				{Name: "DD_PROMETHEUS_SCRAPE_SERVICE_ENDPOINTS", Value: "true"},
			},
		},
		{
			name: "Disabled + Service endpoints enabled",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:          apiutils.NewBoolPointer(false),
				ServiceEndpoints: apiutils.NewBoolPointer(true),
			},
			want: []v1.EnvVar{},
		},
		{
			name: "Additional config",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled: apiutils.NewBoolPointer(true),
				AdditionalConfigs: apiutils.NewStringPointer(`- configurations:
  - timeout: 5
    send_distribution_buckets: true
  autodiscovery:
    kubernetes_container_names:
      - my-app
    kubernetes_annotations:
      include:
        custom_label: 'true'
`),
			},
			want: []v1.EnvVar{
				{Name: "DD_PROMETHEUS_SCRAPE_ENABLED", Value: "true"},
				{Name: "DD_PROMETHEUS_SCRAPE_SERVICE_ENDPOINTS", Value: "false"},
				{Name: "DD_PROMETHEUS_SCRAPE_CHECKS", Value: `[{"autodiscovery":{"kubernetes_annotations":{"include":{"custom_label":"true"}},"kubernetes_container_names":["my-app"]},"configurations":[{"send_distribution_buckets":true,"timeout":5}]}]`},
			},
		},
		{
			name: "Invalid additional config",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:           apiutils.NewBoolPointer(true),
				AdditionalConfigs: apiutils.NewStringPointer(","),
			},
			want: []v1.EnvVar{
				{Name: "DD_PROMETHEUS_SCRAPE_ENABLED", Value: "true"},
				{Name: "DD_PROMETHEUS_SCRAPE_SERVICE_ENDPOINTS", Value: "false"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := test.NewDefaultedDatadogAgent("foo", "bar", &test.NewDatadogAgentOptions{
				Features: &datadoghqv1alpha1.DatadogFeatures{
					PrometheusScrape: tt.promConfig,
				},
			})
			assert.EqualValues(t, tt.want, prometheusScrapeEnvVars(logf.Log.WithName(t.Name()), dda))
		})
	}
}

func Test_dsdMapperProfilesEnvVar(t *testing.T) {
	cmKeySelector := func(name, key string) (cmSelector *v1.ConfigMapKeySelector) {
		cmSelector = &v1.ConfigMapKeySelector{}
		cmSelector.Name = name
		cmSelector.Key = key
		return
	}
	tests := []struct {
		name                  string
		dsdMapperProfilesConf *datadoghqv1alpha1.CustomConfigSpec
		want                  *v1.EnvVar
	}{
		{
			name: "YAML conf data",
			dsdMapperProfilesConf: &datadoghqv1alpha1.CustomConfigSpec{
				ConfigData: apiutils.NewStringPointer(`- name: my_custom_metric_profile
  prefix: custom_metric.
  mappings:
    - match: 'custom_metric.process.*.*'
      match_type: wildcard
      name: 'custom_metric.process.prod.$1.live'
      tags:
        tag_key_2: '$2'
`),
			},
			want: &v1.EnvVar{
				Name:  "DD_DOGSTATSD_MAPPER_PROFILES",
				Value: `[{"mappings":[{"match":"custom_metric.process.*.*","match_type":"wildcard","name":"custom_metric.process.prod.$1.live","tags":{"tag_key_2":"$2"}}],"name":"my_custom_metric_profile","prefix":"custom_metric."}]`,
			},
		},
		{
			name: "JSON conf data",
			dsdMapperProfilesConf: &datadoghqv1alpha1.CustomConfigSpec{
				ConfigData: apiutils.NewStringPointer(`[{"mappings":[{"match":"custom_metric.process.*.*","match_type":"wildcard","name":"custom_metric.process.prod.$1.live","tags":{"tag_key_2":"$2"}}],"name":"my_custom_metric_profile","prefix":"custom_metric."}]`),
			},
			want: &v1.EnvVar{
				Name:  "DD_DOGSTATSD_MAPPER_PROFILES",
				Value: `[{"mappings":[{"match":"custom_metric.process.*.*","match_type":"wildcard","name":"custom_metric.process.prod.$1.live","tags":{"tag_key_2":"$2"}}],"name":"my_custom_metric_profile","prefix":"custom_metric."}]`,
			},
		},
		{
			name: "config map",
			dsdMapperProfilesConf: &datadoghqv1alpha1.CustomConfigSpec{
				ConfigMap: &datadoghqv1alpha1.ConfigFileConfigMapSpec{
					Name:    "dsd-config",
					FileKey: "config",
				},
			},
			want: &v1.EnvVar{
				Name:      "DD_DOGSTATSD_MAPPER_PROFILES",
				ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: cmKeySelector("dsd-config", "config")},
			},
		},
		{
			name: "conf data + config map",
			dsdMapperProfilesConf: &datadoghqv1alpha1.CustomConfigSpec{
				ConfigData: apiutils.NewStringPointer("foo: bar"),
				ConfigMap: &datadoghqv1alpha1.ConfigFileConfigMapSpec{
					Name:    "dsd-config",
					FileKey: "config",
				},
			},
			want: &v1.EnvVar{
				Name:  "DD_DOGSTATSD_MAPPER_PROFILES",
				Value: `{"foo":"bar"}`,
			},
		},
		{
			name: "invalid conf data",
			dsdMapperProfilesConf: &datadoghqv1alpha1.CustomConfigSpec{
				ConfigData: apiutils.NewStringPointer(","),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := test.NewDefaultedDatadogAgent("foo", "bar", &test.NewDatadogAgentOptions{
				NodeAgentConfig: &datadoghqv1alpha1.NodeAgentConfig{
					Dogstatsd: &datadoghqv1alpha1.DogstatsdConfig{MapperProfiles: tt.dsdMapperProfilesConf},
				},
			})
			assert.EqualValues(t, tt.want, dsdMapperProfilesEnvVar(logf.Log.WithName(t.Name()), dda))
		})
	}
}

func Test_mergeAnnotationsLabels(t *testing.T) {
	type args struct {
		previousVal map[string]string
		newVal      map[string]string
		filter      string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "basic test",
			args: args{
				previousVal: map[string]string{
					"foo":               "bar",
					"foo-datadoghq.com": "dog-bar",
					"foo-removed":       "foo",
					"foo.match":         "foomatch",
				},
				newVal: map[string]string{
					"foo": "baz",
				},
				filter: "*.match",
			},
			want: map[string]string{
				"foo":               "baz",
				"foo-datadoghq.com": "dog-bar",
				"foo.match":         "foomatch",
			},
		},
		{
			name: "no filter test",
			args: args{
				previousVal: map[string]string{
					"foo":               "bar",
					"foo-datadoghq.com": "dog-bar",
					"foo-removed":       "foo",
					"foo.match":         "foomatch",
				},
				newVal: map[string]string{
					"foo": "baz",
				},
			},
			want: map[string]string{
				"foo":               "baz",
				"foo-datadoghq.com": "dog-bar",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			got := mergeAnnotationsLabels(logger, tt.args.previousVal, tt.args.newVal, tt.args.filter)
			diff := cmp.Diff(tt.want, got)
			assert.Empty(t, diff)
		})
	}
}

func Test_getImage(t *testing.T) {
	tests := []struct {
		name      string
		imageSpec *commonv1.AgentImageConfig
		registry  *string
		want      string
	}{
		{
			name: "backward compatible",
			imageSpec: &commonv1.AgentImageConfig{
				Name: defaulting.GetLatestAgentImage(),
			},
			registry: nil,
			want:     defaulting.GetLatestAgentImage(),
		},
		{
			name: "nominal case",
			imageSpec: &commonv1.AgentImageConfig{
				Name: "agent",
				Tag:  "7",
			},
			registry: apiutils.NewStringPointer("public.ecr.aws/datadog"),
			want:     "public.ecr.aws/datadog/agent:7",
		},
		{
			name: "prioritize the full path",
			imageSpec: &commonv1.AgentImageConfig{
				Name: "docker.io/datadog/agent:7.28.1-rc.3",
				Tag:  "latest",
			},
			registry: apiutils.NewStringPointer("gcr.io/datadoghq"),
			want:     "docker.io/datadog/agent:7.28.1-rc.3",
		},
		{
			name: "default registry",
			imageSpec: &commonv1.AgentImageConfig{
				Name: "agent",
				Tag:  "latest",
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:latest",
		},
		{
			name: "add jmx",
			imageSpec: &commonv1.AgentImageConfig{
				Name:       "agent",
				Tag:        defaulting.AgentLatestVersion,
				JMXEnabled: true,
			},
			registry: nil,
			want:     defaulting.GetLatestAgentImageJMX(),
		},
		{
			name: "cluster-agent",
			imageSpec: &commonv1.AgentImageConfig{
				Name:       "cluster-agent",
				Tag:        defaulting.ClusterAgentLatestVersion,
				JMXEnabled: false,
			},
			registry: nil,
			want:     defaulting.GetLatestClusterAgentImage(),
		},
		{
			name: "do not duplicate jmx",
			imageSpec: &commonv1.AgentImageConfig{
				Name:       "agent",
				Tag:        "latest-jmx",
				JMXEnabled: true,
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:latest-jmx",
		},
		{
			name: "do not add jmx",
			imageSpec: &commonv1.AgentImageConfig{
				Name:       "agent",
				Tag:        "latest-jmx",
				JMXEnabled: true,
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:latest-jmx",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getImage(tt.imageSpec, tt.registry))
		})
	}
}

func Test_getReplicas(t *testing.T) {
	tests := []struct {
		name    string
		current *int32
		new     *int32
		want    *int32
	}{
		{
			name:    "both not nil",
			current: apiutils.NewInt32Pointer(2),
			new:     apiutils.NewInt32Pointer(3),
			want:    apiutils.NewInt32Pointer(3),
		},
		{
			name:    "new is nil",
			current: apiutils.NewInt32Pointer(2),
			new:     nil,
			want:    apiutils.NewInt32Pointer(2),
		},
		{
			name:    "current is nil",
			current: nil,
			new:     apiutils.NewInt32Pointer(3),
			want:    apiutils.NewInt32Pointer(3),
		},
		{
			name:    "both nil",
			current: nil,
			new:     nil,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getReplicas(tt.current, tt.new)
			assert.Equal(t, tt.want, got)
			if got != nil {
				// Assert the result's address and
				// the input pointers are not equal
				assert.NotSame(t, tt.current, got)
				assert.NotSame(t, tt.new, got)
			}
		})
	}
}
