// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

type envVar struct {
	name    string
	value   string
	present bool
}

func assertEnv(envVars ...envVar) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

			for _, envVar := range envVars {
				if !envVar.present {
					for _, env := range agentEnvs {
						require.NotEqual(t, envVar.name, env.Name)
					}
					continue
				}

				expected := &corev1.EnvVar{
					Name:  envVar.name,
					Value: envVar.value,
				}
				require.Contains(t, agentEnvs, expected)
			}
		},
	)
}

func TestAppsecFeature(t *testing.T) {
	test.FeatureTestSuite{
		{
			Name: "Appsec not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Appsec enabled with minimal config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.73.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorAutoDetect:           "true",
					AnnotationInjectorProcessorServiceName: "appsec-processor",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyAutoDetect, value: "true", present: true},
			),
		},
		{
			Name: "Appsec enabled with autoDetect true",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.73.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorAutoDetect:           "true",
					AnnotationInjectorProcessorServiceName: "appsec-processor",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyAutoDetect, value: "true", present: true},
			),
		},
		{
			Name: "Appsec enabled with autoDetect false",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.73.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorAutoDetect:           "false",
					AnnotationInjectorProxies:              `["envoy-gateway"]`,
					AnnotationInjectorProcessorServiceName: "appsec-processor",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyAutoDetect, value: "false", present: true},
				envVar{name: DDAppsecProxyProxies, value: `["envoy-gateway"]`, present: true},
			),
		},
		{
			Name: "Appsec enabled with proxies list",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.73.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorProxies:              `["envoy-gateway","istio"]`,
					AnnotationInjectorProcessorServiceName: "appsec-processor",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyProxies, value: `["envoy-gateway","istio"]`, present: true},
			),
		},
		{
			Name: "Appsec enabled with processor port",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.73.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorAutoDetect:           "true",
					AnnotationInjectorProcessorPort:        "443",
					AnnotationInjectorProcessorServiceName: "appsec-processor",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyAutoDetect, value: "true", present: true},
				envVar{name: DDAppsecProxyProcessorPort, value: "443", present: true},
			),
		},
		{
			Name: "Appsec enabled with processor address",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.73.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorAutoDetect:           "true",
					AnnotationInjectorProcessorAddress:     "processor.example.com",
					AnnotationInjectorProcessorServiceName: "appsec-processor",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyAutoDetect, value: "true", present: true},
				envVar{name: DDAppsecProxyProcessorAddress, value: "processor.example.com", present: true},
			),
		},
		{
			Name: "Appsec enabled with processor service name and namespace",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.73.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:                   "true",
					AnnotationInjectorAutoDetect:                "true",
					AnnotationInjectorProcessorServiceName:      "appsec-processor",
					AnnotationInjectorProcessorServiceNamespace: "datadog",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyAutoDetect, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceName, value: "appsec-processor", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceNamespace, value: "datadog", present: true},
			),
		},
		{
			Name: "Appsec enabled with full config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.73.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:                   "true",
					AnnotationInjectorAutoDetect:                "true",
					AnnotationInjectorProxies:                   `["envoy-gateway","istio"]`,
					AnnotationInjectorProcessorPort:             "443",
					AnnotationInjectorProcessorAddress:          "processor.example.com",
					AnnotationInjectorProcessorServiceName:      "appsec-processor",
					AnnotationInjectorProcessorServiceNamespace: "datadog",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyAutoDetect, value: "true", present: true},
				envVar{name: DDAppsecProxyProxies, value: `["envoy-gateway","istio"]`, present: true},
				envVar{name: DDAppsecProxyProcessorPort, value: "443", present: true},
				envVar{name: DDAppsecProxyProcessorAddress, value: "processor.example.com", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceName, value: "appsec-processor", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceNamespace, value: "datadog", present: true},
			),
		},
	}.Run(t, buildAppsecFeature)
}

func TestAppsecFeatureID(t *testing.T) {
	f := buildAppsecFeature(nil)
	assert.Equal(t, string(feature.AppsecIDType), string(f.ID()))
}

func TestAppsecVersionCheck(t *testing.T) {
	tests := []struct {
		name            string
		clusterAgentTag string
		wantConfigured  bool
	}{
		{
			name:            "version below minimum 7.72.0",
			clusterAgentTag: "7.72.0",
			wantConfigured:  false,
		},
		{
			name:            "version below minimum 7.60.0",
			clusterAgentTag: "7.60.0",
			wantConfigured:  false,
		},
		{
			name:            "version at exact minimum 7.73.0",
			clusterAgentTag: "7.73.0",
			wantConfigured:  true,
		},
		{
			name:            "version above minimum 7.74.0",
			clusterAgentTag: "7.74.0",
			wantConfigured:  true,
		},
		{
			name:            "version far above minimum 8.0.0",
			clusterAgentTag: "8.0.0",
			wantConfigured:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag(tt.clusterAgentTag).
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorAutoDetect:           "true",
					AnnotationInjectorProcessorServiceName: "appsec-processor",
				}).
				Build()

			f := buildAppsecFeature(nil).(*appsecFeature)
			reqComp := f.Configure(dda, &dda.Spec, nil)

			if tt.wantConfigured {
				assert.True(t, reqComp.ClusterAgent.IsRequired != nil && *reqComp.ClusterAgent.IsRequired,
					"Feature should be configured for version %s", tt.clusterAgentTag)
				assert.True(t, f.config.Enabled, "Config should be enabled for valid version")
			} else {
				assert.False(t, reqComp.ClusterAgent.IsRequired != nil && *reqComp.ClusterAgent.IsRequired,
					"Feature should not be configured for version %s", tt.clusterAgentTag)
			}
		})
	}
}

func TestAppsecFeatureConfigure(t *testing.T) {
	tests := []struct {
		name              string
		annotations       map[string]string
		wantEnabled       bool
		wantClusterAgent  bool
		wantAutoDetect    *bool
		wantProxies       []string
		wantProcessorPort int
	}{
		{
			name:             "Appsec Injector not enabled",
			annotations:      map[string]string{},
			wantEnabled:      false,
			wantClusterAgent: false,
		},
		{
			name: "Appsec enabled with RequiredComponents",
			annotations: map[string]string{
				AnnotationInjectorEnabled:              "true",
				AnnotationInjectorAutoDetect:           "true",
				AnnotationInjectorProcessorServiceName: "appsec-processor",
			},
			wantEnabled:      true,
			wantClusterAgent: true,
		},
		{
			name: "Appsec with all configs",
			annotations: map[string]string{
				AnnotationInjectorEnabled:                   "true",
				AnnotationInjectorAutoDetect:                "true",
				AnnotationInjectorProxies:                   `["envoy-gateway","istio"]`,
				AnnotationInjectorProcessorPort:             "443",
				AnnotationInjectorProcessorServiceName:      "appsec-processor",
				AnnotationInjectorProcessorServiceNamespace: "datadog",
			},
			wantEnabled:       true,
			wantClusterAgent:  true,
			wantAutoDetect:    boolPtr(true),
			wantProxies:       []string{"envoy-gateway", "istio"},
			wantProcessorPort: 443,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.73.0").
				WithAnnotations(tt.annotations).
				Build()

			f := buildAppsecFeature(nil).(*appsecFeature)
			reqComp := f.Configure(dda, &dda.Spec, nil)

			assert.Equal(t, tt.wantEnabled, f.config.Enabled)

			if tt.wantClusterAgent {
				assert.NotNil(t, reqComp.ClusterAgent.IsRequired)
				assert.True(t, *reqComp.ClusterAgent.IsRequired)
				assert.Contains(t, reqComp.ClusterAgent.Containers, apicommon.ClusterAgentContainerName)
			} else {
				if reqComp.ClusterAgent.IsRequired != nil {
					assert.False(t, *reqComp.ClusterAgent.IsRequired)
				}
			}

			if tt.wantAutoDetect != nil {
				assert.Equal(t, tt.wantAutoDetect, f.config.AutoDetect)
			}

			if tt.wantProxies != nil {
				assert.Equal(t, tt.wantProxies, f.config.Proxies)
			}

			if tt.wantProcessorPort != 0 {
				assert.Equal(t, tt.wantProcessorPort, f.config.ProcessorPort)
			}
		})
	}
}

func TestAppsecFeatureManageClusterAgentDisabled(t *testing.T) {
	// Test that ManageClusterAgent does nothing when feature is disabled
	dda := testutils.NewDatadogAgentBuilder().
		Build()

	f := buildAppsecFeature(nil).(*appsecFeature)
	f.Configure(dda, &dda.Spec, nil)

	mgr := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	err := f.ManageClusterAgent(mgr, "")

	assert.NoError(t, err)
	envVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
	assert.Empty(t, envVars)
}

func TestAppsecFeatureManageClusterAgentEnabled(t *testing.T) {
	// Test that ManageClusterAgent adds env vars when feature is enabled
	dda := testutils.NewDatadogAgentBuilder().
		WithClusterAgentTag("7.73.0").
		WithAnnotations(map[string]string{
			AnnotationInjectorEnabled:              "true",
			AnnotationInjectorAutoDetect:           "true",
			AnnotationInjectorProcessorServiceName: "appsec-processor",
		}).
		Build()

	f := buildAppsecFeature(nil).(*appsecFeature)
	f.Configure(dda, &dda.Spec, nil)

	mgr := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	err := f.ManageClusterAgent(mgr, "")

	assert.NoError(t, err)
	envVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
	assert.NotEmpty(t, envVars)

	// Check that required env vars are set
	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	assert.Equal(t, "true", envMap[DDAppsecProxyEnabled])
	assert.Equal(t, "true", envMap[DDClusterAgentAppsecInjectorEnabled])
	assert.Equal(t, "true", envMap[DDAppsecProxyAutoDetect])
}

func TestFromAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantConfig  Config
		wantErr     bool
	}{
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			wantConfig:  Config{},
			wantErr:     false,
		},
		{
			name: "enabled only",
			annotations: map[string]string{
				AnnotationInjectorEnabled:              "true",
				AnnotationInjectorProcessorServiceName: "appsec-svc",
			},
			wantConfig: Config{
				Enabled:              true,
				ProcessorServiceName: "appsec-svc",
			},
			wantErr: false,
		},
		{
			name: "enabled with autoDetect",
			annotations: map[string]string{
				AnnotationInjectorEnabled:              "true",
				AnnotationInjectorAutoDetect:           "true",
				AnnotationInjectorProcessorServiceName: "appsec-svc",
			},
			wantConfig: Config{
				Enabled:              true,
				AutoDetect:           boolPtr(true),
				ProcessorServiceName: "appsec-svc",
			},
			wantErr: false,
		},
		{
			name: "enabled with proxies",
			annotations: map[string]string{
				AnnotationInjectorEnabled:              "true",
				AnnotationInjectorProxies:              `["envoy-gateway","istio"]`,
				AnnotationInjectorProcessorServiceName: "appsec-svc",
			},
			wantConfig: Config{
				Enabled:              true,
				Proxies:              []string{"envoy-gateway", "istio"},
				ProcessorServiceName: "appsec-svc",
			},
			wantErr: false,
		},
		{
			name: "enabled with processor port",
			annotations: map[string]string{
				AnnotationInjectorEnabled:              "true",
				AnnotationInjectorProcessorPort:        "443",
				AnnotationInjectorProcessorServiceName: "appsec-svc",
			},
			wantConfig: Config{
				Enabled:              true,
				ProcessorPort:        443,
				ProcessorServiceName: "appsec-svc",
			},
			wantErr: false,
		},
		{
			name: "invalid enabled value",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid autoDetect value",
			annotations: map[string]string{
				AnnotationInjectorEnabled:    "true",
				AnnotationInjectorAutoDetect: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid proxies JSON",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "true",
				AnnotationInjectorProxies: "not-json",
			},
			wantErr: true,
		},
		{
			name: "invalid processor port",
			annotations: map[string]string{
				AnnotationInjectorEnabled:       "true",
				AnnotationInjectorProcessorPort: "not-a-number",
			},
			wantErr: true,
		},
		{
			name: "full config",
			annotations: map[string]string{
				AnnotationInjectorEnabled:                   "true",
				AnnotationInjectorAutoDetect:                "false",
				AnnotationInjectorProxies:                   `["envoy-gateway"]`,
				AnnotationInjectorProcessorPort:             "8080",
				AnnotationInjectorProcessorAddress:          "processor.example.com",
				AnnotationInjectorProcessorServiceName:      "appsec-svc",
				AnnotationInjectorProcessorServiceNamespace: "datadog",
			},
			wantConfig: Config{
				Enabled:                   true,
				AutoDetect:                boolPtr(false),
				Proxies:                   []string{"envoy-gateway"},
				ProcessorPort:             8080,
				ProcessorAddress:          "processor.example.com",
				ProcessorServiceName:      "appsec-svc",
				ProcessorServiceNamespace: "datadog",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := FromAnnotations(tt.annotations)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantConfig.Enabled, config.Enabled)
				assert.Equal(t, tt.wantConfig.AutoDetect, config.AutoDetect)
				assert.Equal(t, tt.wantConfig.Proxies, config.Proxies)
				assert.Equal(t, tt.wantConfig.ProcessorAddress, config.ProcessorAddress)
				assert.Equal(t, tt.wantConfig.ProcessorPort, config.ProcessorPort)
				assert.Equal(t, tt.wantConfig.ProcessorServiceName, config.ProcessorServiceName)
				assert.Equal(t, tt.wantConfig.ProcessorServiceNamespace, config.ProcessorServiceNamespace)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config with autoDetect",
			config: Config{
				Enabled:              true,
				AutoDetect:           boolPtr(true),
				ProcessorServiceName: "appsec-processor",
			},
			wantErr: false,
		},
		{
			name: "valid config with proxies",
			config: Config{
				Enabled:              true,
				Proxies:              []string{"envoy-gateway"},
				ProcessorServiceName: "appsec-processor",
			},
			wantErr: false,
		},
		{
			name: "invalid port - negative",
			config: Config{
				Enabled:              true,
				AutoDetect:           boolPtr(true),
				ProcessorPort:        -1,
				ProcessorServiceName: "appsec-processor",
			},
			wantErr: true,
		},
		{
			name: "invalid port - too high",
			config: Config{
				Enabled:              true,
				AutoDetect:           boolPtr(true),
				ProcessorPort:        70000,
				ProcessorServiceName: "appsec-processor",
			},
			wantErr: true,
		},
		{
			name: "invalid proxy value",
			config: Config{
				Enabled:              true,
				Proxies:              []string{"invalid-proxy"},
				ProcessorServiceName: "appsec-processor",
			},
			wantErr: true,
		},
		{
			name: "missing service name",
			config: Config{
				Enabled:    true,
				AutoDetect: boolPtr(true),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigIsEnabled(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		wantEnabled bool
	}{
		{
			name: "enabled with autoDetect true",
			config: Config{
				Enabled:    true,
				AutoDetect: boolPtr(true),
			},
			wantEnabled: true,
		},
		{
			name: "enabled with autoDetect false and proxies",
			config: Config{
				Enabled:    true,
				AutoDetect: boolPtr(false),
				Proxies:    []string{"envoy-gateway"},
			},
			wantEnabled: true,
		},
		{
			name: "enabled with autoDetect false but no proxies",
			config: Config{
				Enabled:    true,
				AutoDetect: boolPtr(false),
			},
			wantEnabled: false,
		},
		{
			name: "not enabled",
			config: Config{
				Enabled: false,
			},
			wantEnabled: false,
		},
		{
			name: "enabled with proxies but no autoDetect",
			config: Config{
				Enabled: true,
				Proxies: []string{"envoy-gateway"},
			},
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantEnabled, tt.config.isEnabled())
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
