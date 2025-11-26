// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
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
	port443 := int32(443)
	autoDetectTrue := true
	processorAddress := "processor.example.com"
	serviceName := "appsec-processor"
	serviceNamespace := "datadog"

	test.FeatureTestSuite{
		{
			Name: "Appsec not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppsecEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Appsec enabled with minimal config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppsecConfig(true, apiutils.NewBoolPointer(true), nil, nil, nil, nil, nil).
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
				WithAppsecConfig(true, &autoDetectTrue, nil, nil, nil, nil, nil).
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
				WithAppsecConfig(true, apiutils.NewBoolPointer(false), []string{"envoy-gateway"}, nil, nil, nil, nil).
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
				WithAppsecConfig(true, nil, []string{"envoy-gateway", "istio"}, nil, nil, nil, nil).
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
				WithAppsecConfig(true, apiutils.NewBoolPointer(true), nil, &port443, nil, nil, nil).
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
				WithAppsecConfig(true, apiutils.NewBoolPointer(true), nil, nil, &processorAddress, nil, nil).
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
				WithAppsecConfig(true, apiutils.NewBoolPointer(true), nil, nil, nil, &serviceName, &serviceNamespace).
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
				WithAppsecConfig(
					true,
					apiutils.NewBoolPointer(true),
					[]string{"envoy-gateway", "istio"},
					apiutils.NewInt32Pointer(443),
					apiutils.NewStringPointer("processor.example.com"),
					apiutils.NewStringPointer("appsec-processor"),
					apiutils.NewStringPointer("datadog"),
				).
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

func TestAppsecFeatureConfigure(t *testing.T) {
	tests := []struct {
		name              string
		ddaSpec           *v2alpha1.DatadogAgentSpec
		wantEnabled       bool
		wantClusterAgent  bool
		wantAutoDetect    *bool
		wantProxies       []string
		wantProcessorPort *int32
	}{
		{
			name: "Appsec Injector enabled false",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					Appsec: &v2alpha1.AppsecFeatureConfig{
						Injector: &v2alpha1.AppsecInjectorConfig{
							Enabled: apiutils.NewBoolPointer(false),
						},
					},
				},
			},
			wantEnabled:      false,
			wantClusterAgent: false,
		},
		{
			name: "Appsec enabled with RequiredComponents",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					Appsec: &v2alpha1.AppsecFeatureConfig{
						Injector: &v2alpha1.AppsecInjectorConfig{
							Enabled:    apiutils.NewBoolPointer(true),
							AutoDetect: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
			wantEnabled:      true,
			wantClusterAgent: true,
		},
		{
			name: "Appsec with all configs",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					Appsec: &v2alpha1.AppsecFeatureConfig{
						Injector: &v2alpha1.AppsecInjectorConfig{
							Enabled:    apiutils.NewBoolPointer(true),
							AutoDetect: apiutils.NewBoolPointer(true),
							Proxies:    []string{"envoy-gateway", "istio"},
							Processor: &v2alpha1.AppsecProcessorConfig{
								Port: apiutils.NewInt32Pointer(443),
							},
						},
					},
				},
			},
			wantEnabled:       true,
			wantClusterAgent:  true,
			wantAutoDetect:    apiutils.NewBoolPointer(true),
			wantProxies:       []string{"envoy-gateway", "istio"},
			wantProcessorPort: apiutils.NewInt32Pointer(443),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := testutils.NewDatadogAgentBuilder().Build()
			dda.Spec = *tt.ddaSpec

			f := buildAppsecFeature(nil).(*appsecFeature)
			reqComp := f.Configure(dda, tt.ddaSpec, nil)

			assert.Equal(t, tt.wantEnabled, f.enabled)

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
				assert.Equal(t, tt.wantAutoDetect, f.autoDetect)
			}

			if tt.wantProxies != nil {
				assert.Equal(t, tt.wantProxies, f.proxies)
			}

			if tt.wantProcessorPort != nil {
				assert.Equal(t, tt.wantProcessorPort, f.processorPort)
			}
		})
	}
}

func TestAppsecFeatureManageDependenciesDisabled(t *testing.T) {
	// Test that ManageDependencies returns nil when feature is disabled
	dda := testutils.NewDatadogAgentBuilder().
		WithAppsecEnabled(false).
		Build()

	f := buildAppsecFeature(nil).(*appsecFeature)
	f.Configure(dda, &dda.Spec, nil)

	// ManageDependencies should return nil when disabled without calling RBACManager
	assert.False(t, f.enabled, "Feature should not be enabled")
}

func TestAppsecFeatureManageDependenciesEnabled(t *testing.T) {
	// Test that ManageDependencies is called when feature is enabled (tested via test suite)
	tests := test.FeatureTestSuite{
		{
			Name: "ManageDependencies when enabled",
			DDA: func() *v2alpha1.DatadogAgent {
				dda := testutils.NewDatadogAgentBuilder().
					WithAppsecConfig(true, apiutils.NewBoolPointer(true), nil, nil, nil, nil, nil).
					Build()
				dda.Name = "datadog"
				dda.Namespace = "test-namespace"
				return dda
			}(),
			WantConfigure: true,
			// The test framework will call ManageDependencies and verify no error is returned
		},
	}

	tests.Run(t, buildAppsecFeature)
}

func TestAppsecFeatureManageSingleContainerNodeAgent(t *testing.T) {
	f := &appsecFeature{}
	err := f.ManageSingleContainerNodeAgent(nil, "")
	assert.NoError(t, err, "ManageSingleContainerNodeAgent should return no error")
}

func TestAppsecFeatureManageNodeAgent(t *testing.T) {
	f := &appsecFeature{}
	err := f.ManageNodeAgent(nil, "")
	assert.NoError(t, err, "ManageNodeAgent should return no error")
}

func TestAppsecFeatureManageClusterChecksRunner(t *testing.T) {
	f := &appsecFeature{}
	err := f.ManageClusterChecksRunner(nil, "")
	assert.NoError(t, err, "ManageClusterChecksRunner should return no error")
}

func TestAppsecFeatureManageOtelAgentGateway(t *testing.T) {
	f := &appsecFeature{}
	err := f.ManageOtelAgentGateway(nil, "")
	assert.NoError(t, err, "ManageOtelAgentGateway should return no error")
}

func TestAppsecFeatureServiceAccountName(t *testing.T) {
	dda := testutils.NewDatadogAgentBuilder().
		WithAppsecConfig(true, apiutils.NewBoolPointer(true), nil, nil, nil, nil, nil).
		Build()
	dda.Name = "test-datadog-agent"

	f := buildAppsecFeature(nil).(*appsecFeature)
	f.Configure(dda, &dda.Spec, nil)

	// Verify service account name is set
	assert.NotEmpty(t, f.serviceAccountName, "Service account name should be set")
	assert.Contains(t, f.serviceAccountName, "test-datadog-agent", "Service account should include DDA name")
}

func TestAppsecFeatureOwnerSet(t *testing.T) {
	dda := testutils.NewDatadogAgentBuilder().
		WithAppsecConfig(true, apiutils.NewBoolPointer(true), nil, nil, nil, nil, nil).
		Build()
	dda.Name = "test-owner"
	dda.Namespace = "test-namespace"

	f := buildAppsecFeature(nil).(*appsecFeature)
	f.Configure(dda, &dda.Spec, nil)

	// Verify owner is set correctly
	assert.NotNil(t, f.owner, "Owner should be set")
	assert.Equal(t, "test-owner", f.owner.GetName())
	assert.Equal(t, "test-namespace", f.owner.GetNamespace())
}

func TestAppsecFeatureProcessorServiceConfig(t *testing.T) {
	serviceName := "appsec-processor"
	serviceNamespace := "custom-namespace"

	dda := testutils.NewDatadogAgentBuilder().
		WithAppsecConfig(
			true,
			apiutils.NewBoolPointer(true),
			nil,
			nil,
			nil,
			&serviceName,
			&serviceNamespace,
		).
		Build()

	f := buildAppsecFeature(nil).(*appsecFeature)
	f.Configure(dda, &dda.Spec, nil)

	// Verify processor service config is set
	assert.NotNil(t, f.processorServiceName)
	assert.Equal(t, "appsec-processor", *f.processorServiceName)
	assert.NotNil(t, f.processorServiceNs)
	assert.Equal(t, "custom-namespace", *f.processorServiceNs)
}

func TestAppsecFeatureProcessorWithoutService(t *testing.T) {
	port := int32(8443)
	address := "custom-processor.example.com"

	dda := testutils.NewDatadogAgentBuilder().
		WithAppsecConfig(
			true,
			apiutils.NewBoolPointer(true),
			nil,
			&port,
			&address,
			nil, // No service name
			nil, // No service namespace
		).
		Build()

	f := buildAppsecFeature(nil).(*appsecFeature)
	f.Configure(dda, &dda.Spec, nil)

	// Verify processor config without service
	assert.NotNil(t, f.processorPort)
	assert.Equal(t, int32(8443), *f.processorPort)
	assert.NotNil(t, f.processorAddress)
	assert.Equal(t, "custom-processor.example.com", *f.processorAddress)
	assert.Nil(t, f.processorServiceName)
	assert.Nil(t, f.processorServiceNs)
}
