// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"

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

func TestAppSecFeature(t *testing.T) {
	port443 := int32(443)
	autoDetectTrue := true
	processorAddress := "processor.example.com"
	serviceName := "appsec-processor"
	serviceNamespace := "datadog"

	test.FeatureTestSuite{
		{
			Name: "AppSec not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppSecEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "AppSec enabled with minimal config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppSecEnabled(true).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
			),
		},
		{
			Name: "AppSec enabled with autoDetect",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppSecConfig(true, &autoDetectTrue, nil, nil, nil, nil, nil).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyAutoDetect, value: "true", present: true},
			),
		},
		{
			Name: "AppSec enabled with proxies list",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppSecConfig(true, nil, []string{"envoy-gateway", "istio"}, nil, nil, nil, nil).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyProxies, value: `["envoy-gateway","istio"]`, present: true},
			),
		},
		{
			Name: "AppSec enabled with processor port",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppSecConfig(true, nil, nil, &port443, nil, nil, nil).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyProcessorPort, value: "443", present: true},
			),
		},
		{
			Name: "AppSec enabled with processor address",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppSecConfig(true, nil, nil, nil, &processorAddress, nil, nil).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyProcessorAddress, value: "processor.example.com", present: true},
			),
		},
		{
			Name: "AppSec enabled with processor service name and namespace",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppSecConfig(true, nil, nil, nil, nil, &serviceName, &serviceNamespace).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceName, value: "appsec-processor", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceNamespace, value: "datadog", present: true},
			),
		},
		{
			Name: "AppSec enabled with full config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAppSecConfig(
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
	}.Run(t, buildAppSecFeature)
}
