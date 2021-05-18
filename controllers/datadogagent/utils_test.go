package datadogagent

import (
	"reflect"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/api/v1alpha1/test"
	"github.com/DataDog/datadog-operator/controllers/testutils"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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
	require.Subset(t, env, []corev1.EnvVar{{
		Name:  datadoghqv1alpha1.DDIgnoreAutoConf,
		Value: "kubernetes_state",
	}})

	spec.Spec.Agent.Config.Env = append(spec.Spec.Agent.Config.Env, corev1.EnvVar{
		Name:  datadoghqv1alpha1.DDIgnoreAutoConf,
		Value: "redis custom",
	})
	env, err = getEnvVarsForAgent(logger, spec)
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
			Features: datadoghqv1alpha1.DatadogFeatures{
				KubeStateMetricsCore: &datadoghqv1alpha1.KubeStateMetricsCore{},
				LogCollection: &datadoghqv1alpha1.LogCollectionConfig{
					Enabled:                       &boolPtr,
					LogsConfigContainerCollectAll: &boolPtr,
					ContainerCollectUsingFiles:    &boolPtr,
					OpenFilesLimit:                &intPtr,
				},
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
				{Name: "logdatadog", MountPath: "/var/log/datadog"},
				{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
			},
		},
		{
			name: "custom config volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{CustomConfig: customConfig}),
			want: []corev1.VolumeMount{
				{Name: "logdatadog", MountPath: "/var/log/datadog"},
				{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				{Name: "custom-datadog-yaml", ReadOnly: true, MountPath: "/etc/datadog-agent/datadog.yaml", SubPath: "datadog.yaml", SubPathExpr: ""},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
			},
		},
		{
			name: "extra volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{VolumeMounts: []corev1.VolumeMount{{Name: "extra", MountPath: "/etc/datadog-agent/extra"}}}),
			want: []corev1.VolumeMount{
				{Name: "logdatadog", MountPath: "/var/log/datadog"},
				{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				{Name: "extra", MountPath: "/etc/datadog-agent/extra"},
				{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
			},
		},
		{
			name: "compliance volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{SecuritySpec: securityCompliance}),
			want: []corev1.VolumeMount{
				v1.VolumeMount{Name: "logdatadog", ReadOnly: false, MountPath: "/var/log/datadog"},
				v1.VolumeMount{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				v1.VolumeMount{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				v1.VolumeMount{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				v1.VolumeMount{Name: "cgroups", ReadOnly: true, MountPath: "/host/sys/fs/cgroup"},
				v1.VolumeMount{Name: "passwd", ReadOnly: true, MountPath: "/etc/passwd"},
				v1.VolumeMount{Name: "group", ReadOnly: true, MountPath: "/etc/group"},
				v1.VolumeMount{Name: "procdir", ReadOnly: true, MountPath: "/host/proc"},
				v1.VolumeMount{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
				v1.VolumeMount{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/root/var/run/containerd"},
				v1.VolumeMount{Name: "compliancedir", ReadOnly: true, MountPath: "/etc/datadog-agent/compliance.d"},
			},
		},
		{
			name: "compliance volumeMounts",
			dda:  testutils.NewDatadogAgent("foo", "bar", "datadog/agent:7", &testutils.NewDatadogAgentOptions{SecuritySpec: securityRuntime}),
			want: []corev1.VolumeMount{
				v1.VolumeMount{Name: "logdatadog", ReadOnly: false, MountPath: "/var/log/datadog"},
				v1.VolumeMount{Name: "datadog-agent-auth", ReadOnly: true, MountPath: "/etc/datadog-agent/auth"},
				v1.VolumeMount{Name: "config", ReadOnly: false, MountPath: "/etc/datadog-agent"},
				v1.VolumeMount{Name: "hostroot", ReadOnly: true, MountPath: "/host/root"},
				v1.VolumeMount{Name: "runtimesocketdir", ReadOnly: true, MountPath: "/host/var/run/containerd"},
				v1.VolumeMount{Name: "sysprobe-socket-dir", ReadOnly: true, MountPath: "/var/run/sysprobe"},
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
		want       []corev1.EnvVar
	}{
		{
			name: "Enabled + Service endpoints disabled",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:          datadoghqv1alpha1.NewBoolPointer(true),
				ServiceEndpoints: datadoghqv1alpha1.NewBoolPointer(false),
			},
			want: []corev1.EnvVar{
				{Name: "DD_PROMETHEUS_SCRAPE_ENABLED", Value: "true"},
				{Name: "DD_PROMETHEUS_SCRAPE_SERVICE_ENDPOINTS", Value: "false"},
			},
		},
		{
			name: "Enabled + Service endpoints enabled",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:          datadoghqv1alpha1.NewBoolPointer(true),
				ServiceEndpoints: datadoghqv1alpha1.NewBoolPointer(true),
			},
			want: []corev1.EnvVar{
				{Name: "DD_PROMETHEUS_SCRAPE_ENABLED", Value: "true"},
				{Name: "DD_PROMETHEUS_SCRAPE_SERVICE_ENDPOINTS", Value: "true"},
			},
		},
		{
			name: "Disabled + Service endpoints enabled",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:          datadoghqv1alpha1.NewBoolPointer(false),
				ServiceEndpoints: datadoghqv1alpha1.NewBoolPointer(true),
			},
			want: []corev1.EnvVar{},
		},
		{
			name: "Additional config",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled: datadoghqv1alpha1.NewBoolPointer(true),
				AdditionalConfigs: datadoghqv1alpha1.NewStringPointer(`- configurations:
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
			want: []corev1.EnvVar{
				{Name: "DD_PROMETHEUS_SCRAPE_ENABLED", Value: "true"},
				{Name: "DD_PROMETHEUS_SCRAPE_SERVICE_ENDPOINTS", Value: "false"},
				{Name: "DD_PROMETHEUS_SCRAPE_CHECKS", Value: `[{"autodiscovery":{"kubernetes_annotations":{"include":{"custom_label":"true"}},"kubernetes_container_names":["my-app"]},"configurations":[{"send_distribution_buckets":true,"timeout":5}]}]`},
			},
		},
		{
			name: "Invalid additional config",
			promConfig: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:           datadoghqv1alpha1.NewBoolPointer(true),
				AdditionalConfigs: datadoghqv1alpha1.NewStringPointer(","),
			},
			want: []corev1.EnvVar{
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
	cmKeySelector := func(name, key string) (cmSelector *corev1.ConfigMapKeySelector) {
		cmSelector = &corev1.ConfigMapKeySelector{}
		cmSelector.Name = name
		cmSelector.Key = key
		return
	}
	tests := []struct {
		name                  string
		dsdMapperProfilesConf *datadoghqv1alpha1.CustomConfigSpec
		want                  *corev1.EnvVar
	}{
		{
			name: "YAML conf data",
			dsdMapperProfilesConf: &datadoghqv1alpha1.CustomConfigSpec{
				ConfigData: datadoghqv1alpha1.NewStringPointer(`- name: my_custom_metric_profile
  prefix: custom_metric.
  mappings:
    - match: 'custom_metric.process.*.*'
      match_type: wildcard
      name: 'custom_metric.process.prod.$1.live'
      tags:
        tag_key_2: '$2'
`),
			},
			want: &corev1.EnvVar{
				Name:  "DD_DOGSTATSD_MAPPER_PROFILES",
				Value: `[{"mappings":[{"match":"custom_metric.process.*.*","match_type":"wildcard","name":"custom_metric.process.prod.$1.live","tags":{"tag_key_2":"$2"}}],"name":"my_custom_metric_profile","prefix":"custom_metric."}]`,
			},
		},
		{
			name: "JSON conf data",
			dsdMapperProfilesConf: &datadoghqv1alpha1.CustomConfigSpec{
				ConfigData: datadoghqv1alpha1.NewStringPointer(`[{"mappings":[{"match":"custom_metric.process.*.*","match_type":"wildcard","name":"custom_metric.process.prod.$1.live","tags":{"tag_key_2":"$2"}}],"name":"my_custom_metric_profile","prefix":"custom_metric."}]`),
			},
			want: &corev1.EnvVar{
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
			want: &corev1.EnvVar{
				Name:      "DD_DOGSTATSD_MAPPER_PROFILES",
				ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: cmKeySelector("dsd-config", "config")},
			},
		},
		{
			name: "conf data + config map",
			dsdMapperProfilesConf: &datadoghqv1alpha1.CustomConfigSpec{
				ConfigData: datadoghqv1alpha1.NewStringPointer("foo: bar"),
				ConfigMap: &datadoghqv1alpha1.ConfigFileConfigMapSpec{
					Name:    "dsd-config",
					FileKey: "config",
				},
			},
			want: &corev1.EnvVar{
				Name:  "DD_DOGSTATSD_MAPPER_PROFILES",
				Value: `{"foo":"bar"}`,
			},
		},
		{
			name: "invalid conf data",
			dsdMapperProfilesConf: &datadoghqv1alpha1.CustomConfigSpec{
				ConfigData: datadoghqv1alpha1.NewStringPointer(","),
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

func Test_imageHasTag(t *testing.T) {
	cases := map[string]bool{
		"foo:bar":             true,
		"foo/bar:baz":         true,
		"foo/bar:baz:tar":     true,
		"foo/bar:baz-tar":     true,
		"foo/bar:baz_tar":     true,
		"foo/bar:baz.tar":     true,
		"foo/foo/bar:baz:tar": true,
		"foo":                 false,
		":foo":                false,
		"foo:foo/bar":         false,
	}
	for tc, expected := range cases {
		assert.Equal(t, expected, imageHasTag.MatchString(tc))
	}
}

func Test_getImage(t *testing.T) {
	tests := []struct {
		name         string
		imageSpec    *datadoghqv1alpha1.ImageConfig
		specRegistry *string
		want         string
	}{
		{
			name: "backward compatible",
			imageSpec: &datadoghqv1alpha1.ImageConfig{
				Name: "gcr.io/datadoghq/agent:latest",
			},
			specRegistry: nil,
			want:         "gcr.io/datadoghq/agent:latest",
		},
		{
			name: "nominal case",
			imageSpec: &datadoghqv1alpha1.ImageConfig{
				Name: "agent",
				Tag:  "7",
			},
			specRegistry: datadoghqv1alpha1.NewStringPointer("public.ecr.aws/datadog"),
			want:         "public.ecr.aws/datadog/agent:7",
		},
		{
			name: "prioritize the full path",
			imageSpec: &datadoghqv1alpha1.ImageConfig{
				Name: "docker.io/datadog/agent:7.28.1-rc.3",
				Tag:  "latest",
			},
			specRegistry: datadoghqv1alpha1.NewStringPointer("gcr.io/datadoghq"),
			want:         "docker.io/datadog/agent:7.28.1-rc.3",
		},
		{
			name: "default registry",
			imageSpec: &datadoghqv1alpha1.ImageConfig{
				Name: "agent",
				Tag:  "latest",
			},
			specRegistry: nil,
			want:         "gcr.io/datadoghq/agent:latest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getImage(tt.imageSpec, tt.specRegistry))
		})
	}
}
