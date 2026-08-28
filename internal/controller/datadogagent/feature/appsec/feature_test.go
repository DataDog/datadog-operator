// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"strings"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/testutils"
	"github.com/DataDog/datadog-operator/pkg/utils"
	"github.com/stretchr/testify/assert"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
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

// gkeGatewayClassesDDA builds the inline CRD fixture for the gatewayClasses env var. The
// happy and the unset case share it so that they are mechanically identical except for
// gatewayClasses itself.
//
// The 7.82.0 cluster-agent tag is load-bearing: a non-empty gatewayClasses makes
// requiresGKESupport() true, and the default cluster-agent version
// (images.AgentLatestVersion) is below ClusterAgentGKEMinVersion, so an unpinned fixture is
// gated off, emits no env var whatsoever, and makes every env assertion meaningless.
func gkeGatewayClassesDDA(gatewayClasses []string) *v2alpha1.DatadogAgent {
	dda := testutils.NewDatadogAgentBuilder().
		WithClusterAgentTag("7.82.0").
		BuildWithDefaults()
	dda.Spec.Features.Appsec = &v2alpha1.AppsecFeatureConfig{
		Injector: &v2alpha1.AppsecInjectorConfig{
			Enabled: ptr.To(true),
			GKE:     &v2alpha1.AppsecInjectorGKEConfig{GatewayClasses: gatewayClasses},
		},
	}
	return dda
}

// crdFullInjector sets every CRD field that the feature can turn into a cluster-agent
// environment variable, so one fixture pins the whole env surface declared in const.go
// plus the CRD-only DD_APPSEC_PROXY_GKE_GATEWAY_CLASSES.
//
// Mode is "external" because gke-gateway is listed in proxies and Validate() rejects any
// other mode for it; ProcessorServiceName is therefore mandatory as well. Listing
// ingress-nginx and setting nginx.moduleMountPath also arms the nginx version gate, which
// 7.82.0 clears along with the GKE one.
func crdFullInjector() *v2alpha1.AppsecInjectorConfig {
	return &v2alpha1.AppsecInjectorConfig{
		Enabled:    ptr.To(true),
		AutoDetect: ptr.To(true),
		Proxies:    []string{"envoy-gateway", "gke-gateway", "ingress-nginx"},
		Mode:       ptr.To("external"),
		Processor: &v2alpha1.AppsecInjectorProcessorConfig{
			Address: ptr.To("processor.example.com"),
			Port:    ptr.To(int32(8443)),
			Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{
				Name:      ptr.To("appsec-processor"),
				Namespace: ptr.To("datadog"),
			},
		},
		Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{
			Image:                ptr.To("datadog/appsec-proxy"),
			ImageTag:             ptr.To("v2.8.2"),
			Port:                 ptr.To(int32(8080)),
			HealthPort:           ptr.To(int32(8081)),
			BodyParsingSizeLimit: ptr.To(int64(1048576)),
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
		},
		Nginx: &v2alpha1.AppsecInjectorNginxConfig{ModuleMountPath: ptr.To("/modules_mount")},
		GKE:   &v2alpha1.AppsecInjectorGKEConfig{GatewayClasses: []string{"gke-l7-global-external-managed"}},
	}
}

// gkeExternalDDA builds the GKE version-gate matched pair from a single fixture, reusing
// gkeExternalInjector so the two cases differ ONLY in the cluster-agent tag. "Same config
// otherwise" is then guaranteed by construction rather than by review.
func gkeExternalDDA(clusterAgentTag string) *v2alpha1.DatadogAgent {
	return testutils.NewDatadogAgentBuilder().
		WithClusterAgentTag(clusterAgentTag).
		WithAppsecInjector(gkeExternalInjector()).
		Build()
}

func TestAppsecFeature(t *testing.T) {
	// Annotation-only regression fixture, built here so the require.Nil below can prove
	// mechanically that it carries no CRD block at all and therefore really exercises the
	// pre-migration path.
	annotationOnlyDDA := testutils.NewDatadogAgentBuilder().
		WithClusterAgentTag("7.76.0").
		WithAnnotations(map[string]string{
			AnnotationInjectorEnabled:                   "true",
			AnnotationInjectorAutoDetect:                "true",
			AnnotationInjectorProxies:                   `["envoy-gateway","istio"]`,
			AnnotationInjectorMode:                      "external",
			AnnotationInjectorProcessorPort:             "443",
			AnnotationInjectorProcessorAddress:          "processor.example.com",
			AnnotationInjectorProcessorServiceName:      "appsec-processor",
			AnnotationInjectorProcessorServiceNamespace: "datadog",
		}).
		Build()
	require.Nil(t, annotationOnlyDDA.Spec.Features.Appsec,
		"the annotation-only regression fixture must not carry a CRD appsec block")

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
				WithClusterAgentTag("7.76.0").
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
				WithClusterAgentTag("7.76.0").
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
				WithClusterAgentTag("7.76.0").
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
				WithClusterAgentTag("7.76.0").
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
				WithClusterAgentTag("7.76.0").
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
			Name: "Appsec enabled without processor port does not inject port 0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorAutoDetect:           "true",
					AnnotationInjectorProcessorServiceName: "appsec-processor",
				}).
				Build(),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyProcessorPort, present: false},
			),
		},
		{
			Name: "Appsec enabled with processor address",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
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
				WithClusterAgentTag("7.76.0").
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
				WithClusterAgentTag("7.76.0").
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
		{
			Name: "Appsec enabled with istio-gateway proxy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:    "true",
					AnnotationInjectorProxies:    `["istio-gateway"]`,
					AnnotationInjectorAutoDetect: "false",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyProxies, value: `["istio-gateway"]`, present: true},
			),
		},
		{
			Name: "Appsec enabled in sidecar mode without ProcessorServiceName",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:    "true",
					AnnotationInjectorAutoDetect: "true",
					AnnotationInjectorMode:       "sidecar",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorMode, value: "sidecar", present: true},
			),
		},
		{
			Name: "Appsec enabled in sidecar mode with full sidecar config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:                "true",
					AnnotationInjectorAutoDetect:             "true",
					AnnotationInjectorMode:                   "sidecar",
					AnnotationSidecarImage:                   "datadog/appsec-proxy",
					AnnotationSidecarImageTag:                "latest",
					AnnotationSidecarPort:                    "8080",
					AnnotationSidecarHealthPort:              "8081",
					AnnotationSidecarResourcesRequestsCPU:    "100m",
					AnnotationSidecarResourcesRequestsMemory: "128Mi",
					AnnotationSidecarResourcesLimitsCPU:      "500m",
					AnnotationSidecarResourcesLimitsMemory:   "256Mi",
					AnnotationSidecarBodyParsingSizeLimit:    "1048576",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorMode, value: "sidecar", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarImage, value: "datadog/appsec-proxy", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarImageTag, value: "latest", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarPort, value: "8080", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarHealthPort, value: "8081", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarResourcesRequestsCPU, value: "100m", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarResourcesRequestsMemory, value: "128Mi", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarResourcesLimitsCPU, value: "500m", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarResourcesLimitsMemory, value: "256Mi", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarBodyParsingSizeLimit, value: "1048576", present: true},
			),
		},
		{
			Name: "Appsec enabled in external mode requires ProcessorServiceName",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorAutoDetect:           "true",
					AnnotationInjectorMode:                 "external",
					AnnotationInjectorProcessorServiceName: "appsec-processor",
				}).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorMode, value: "external", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceName, value: "appsec-processor", present: true},
			),
		},
		{
			Name: "Appsec enabled with nginx module mount path requires cluster-agent >= 7.79.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:      "true",
					AnnotationInjectorAutoDetect:   "true",
					AnnotationNginxModuleMountPath: "/modules_mount",
				}).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Appsec enabled with nginx module mount path on 7.79.0",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.79.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:      "true",
					AnnotationInjectorAutoDetect:   "true",
					AnnotationNginxModuleMountPath: "/modules_mount",
				}).
				Build(),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAdmissionControllerAppsecNginxModuleMountPath, value: "/modules_mount", present: true},
			),
		},
		{
			Name: "Appsec enabled without nginx annotations does not inject nginx env vars",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:    "true",
					AnnotationInjectorAutoDetect: "true",
				}).
				Build(),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAdmissionControllerAppsecNginxModuleMountPath, present: false},
			),
		},
		{
			Name: "Appsec enabled with ingress-nginx proxy on old cluster-agent is rejected",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:    "true",
					AnnotationInjectorAutoDetect: "true",
					AnnotationInjectorProxies:    `["ingress-nginx"]`,
				}).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Appsec enabled with empty nginx annotations does not inject nginx env vars",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:      "true",
					AnnotationInjectorAutoDetect:   "true",
					AnnotationNginxModuleMountPath: "",
				}).
				Build(),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAdmissionControllerAppsecNginxModuleMountPath, present: false},
			),
		},
		{
			Name:          "Appsec enabled with GKE gatewayClasses on 7.82.0",
			DDA:           gkeGatewayClassesDDA([]string{"gke-l7-global-external-managed", "gke-l7-regional-external-managed"}),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyGKEGatewayClasses, value: `["gke-l7-global-external-managed","gke-l7-regional-external-managed"]`, present: true},
			),
		},
		{
			Name:          "Appsec enabled without GKE gatewayClasses does not inject gateway classes",
			DDA:           gkeGatewayClassesDDA(nil),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyGKEGatewayClasses, present: false},
			),
		},
		{
			Name: "Appsec enabled from the CRD alone with no annotations",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAppsecInjector(&v2alpha1.AppsecInjectorConfig{Enabled: ptr.To(true)}).
				Build(),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
			),
		},
		{
			// The 7.82.0 tag is load-bearing. crdFullInjector lists gke-gateway and sets
			// gatewayClasses, so requiresGKESupport() is true; on the default cluster-agent
			// version the GKE gate would drop the feature, BuildFeatures would return an
			// empty list, ClusterAgent.WantFunc would never run, and every expectation
			// below would pass vacuously. WantConfigure: true is what forbids that:
			// verifyFeatures compares IsConfigured() against it, so a gated-off fixture
			// fails before the env expectations can be silently skipped.
			Name: "Appsec enabled from a full CRD config emits every env var",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.82.0").
				WithAppsecInjector(crdFullInjector()).
				Build(),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyAutoDetect, value: "true", present: true},
				envVar{name: DDAppsecProxyProxies, value: `["envoy-gateway","gke-gateway","ingress-nginx"]`, present: true},
				envVar{name: DDAppsecProxyProcessorPort, value: "8443", present: true},
				envVar{name: DDAppsecProxyProcessorAddress, value: "processor.example.com", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceName, value: "appsec-processor", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceNamespace, value: "datadog", present: true},
				envVar{name: DDClusterAgentAppsecInjectorMode, value: "external", present: true},
				envVar{name: DDAppsecProxyGKEGatewayClasses, value: `["gke-l7-global-external-managed"]`, present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarImage, value: "datadog/appsec-proxy", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarImageTag, value: "v2.8.2", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarPort, value: "8080", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarHealthPort, value: "8081", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarResourcesRequestsCPU, value: "100m", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarResourcesRequestsMemory, value: "128Mi", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarResourcesLimitsCPU, value: "500m", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarResourcesLimitsMemory, value: "256Mi", present: true},
				envVar{name: DDAdmissionControllerAppsecSidecarBodyParsingSizeLimit, value: "1048576", present: true},
				envVar{name: DDAdmissionControllerAppsecNginxModuleMountPath, value: "/modules_mount", present: true},
			),
		},
		{
			Name: "Appsec CRD mode external beats annotation mode sidecar in the emitted env",
			DDA: testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
				WithAnnotations(map[string]string{
					AnnotationInjectorEnabled:              "true",
					AnnotationInjectorMode:                 "sidecar",
					AnnotationInjectorProcessorServiceName: "annotation-svc",
				}).
				WithAppsecInjector(&v2alpha1.AppsecInjectorConfig{
					Mode: ptr.To("external"),
					Processor: &v2alpha1.AppsecInjectorProcessorConfig{
						Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{Name: ptr.To("crd-svc")},
					},
				}).
				Build(),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorMode, value: "external", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceName, value: "crd-svc", present: true},
			),
		},
		{
			Name:          "Appsec annotation-only config is unaffected by the CRD migration",
			DDA:           annotationOnlyDDA,
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
				envVar{name: DDClusterAgentAppsecInjectorMode, value: "external", present: true},
				envVar{name: DDAppsecProxyGKEGatewayClasses, present: false},
			),
		},
		{
			// (e) GKE positive. Matched pair with the 7.81.0 case below: both come from
			// gkeExternalDDA, so only the cluster-agent tag differs.
			Name:          "Appsec GKE gateway proxy on 7.82.0 configures and emits the proxies env",
			DDA:           gkeExternalDDA("7.82.0"),
			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{name: DDAppsecProxyEnabled, value: "true", present: true},
				envVar{name: DDClusterAgentAppsecInjectorEnabled, value: "true", present: true},
				envVar{name: DDAppsecProxyProxies, value: `["gke-gateway"]`, present: true},
				envVar{name: DDClusterAgentAppsecInjectorMode, value: "external", present: true},
				envVar{name: DDClusterAgentAppsecInjectorProcessorServiceName, value: "appsec-processor", present: true},
			),
		},
		{
			// (f) GKE negative, the same config one patch version lower. The triple that
			// proves the version gate is what rejected it lives in
			// TestAppsecGKEGatewayMatchedPair, because the shared suite cannot reach
			// f.config.
			Name:          "Appsec GKE gateway proxy on 7.81.0 is gated off",
			DDA:           gkeExternalDDA("7.81.0"),
			WantConfigure: false,
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
			name:            "version below minimum 7.75.0",
			clusterAgentTag: "7.75.0",
			wantConfigured:  false,
		},
		{
			name:            "version below minimum 7.60.0",
			clusterAgentTag: "7.60.0",
			wantConfigured:  false,
		},
		{
			name:            "version at exact minimum 7.76.0",
			clusterAgentTag: "7.76.0",
			wantConfigured:  true,
		},
		{
			name:            "version above minimum 7.77.0",
			clusterAgentTag: "7.77.0",
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
			wantAutoDetect:    ptr.To(true),
			wantProxies:       []string{"envoy-gateway", "istio"},
			wantProcessorPort: 443,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.76.0").
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
	err := f.ManageClusterAgent(mgr)

	assert.NoError(t, err)
	envVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
	assert.Empty(t, envVars)
}

func TestAppsecFeatureManageClusterAgentEnabled(t *testing.T) {
	// Test that ManageClusterAgent adds env vars when feature is enabled
	dda := testutils.NewDatadogAgentBuilder().
		WithClusterAgentTag("7.76.0").
		WithAnnotations(map[string]string{
			AnnotationInjectorEnabled:              "true",
			AnnotationInjectorAutoDetect:           "true",
			AnnotationInjectorProcessorServiceName: "appsec-processor",
		}).
		Build()

	f := buildAppsecFeature(nil).(*appsecFeature)
	f.Configure(dda, &dda.Spec, nil)

	mgr := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	err := f.ManageClusterAgent(mgr)

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

// TestManageClusterAgentGKEGatewayClasses pins the DD_APPSEC_PROXY_GKE_GATEWAY_CLASSES
// emission by driving Configure and ManageClusterAgent directly, so the assertions cannot
// be skipped the way a gated-off feature skips test.FeatureTestSuite's WantFunc. Every case
// additionally asserts that the feature really configured and really emitted its baseline
// env vars, which is what makes the "absent" cases meaningful instead of vacuous.
func TestManageClusterAgentGKEGatewayClasses(t *testing.T) {
	tests := []struct {
		name           string
		gatewayClasses []string
		wantEnv        string
		wantPresent    bool
	}{
		{
			name:           "two gateway classes are emitted as a JSON array",
			gatewayClasses: []string{"gke-l7-global-external-managed", "gke-l7-regional-external-managed"},
			wantEnv:        `["gke-l7-global-external-managed","gke-l7-regional-external-managed"]`,
			wantPresent:    true,
		},
		{
			name:           "a single gateway class is still a JSON array",
			gatewayClasses: []string{"gke-l7-global-external-managed"},
			wantEnv:        `["gke-l7-global-external-managed"]`,
			wantPresent:    true,
		},
		{
			name:           "nil gateway classes emit nothing",
			gatewayClasses: nil,
		},
		{
			// An empty list must be skipped entirely rather than marshalled to "[]",
			// which the cluster-agent would read as "no gateway class is eligible".
			name:           "an explicitly empty gateway class list emits nothing, not []",
			gatewayClasses: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := gkeGatewayClassesDDA(tt.gatewayClasses)

			f := buildAppsecFeature(nil).(*appsecFeature)
			got := f.Configure(dda, &dda.Spec, nil)

			// Non-vacuity controls: a gated-off feature returns no required component and
			// emits nothing, which would satisfy every "absent" assertion below for the
			// wrong reason.
			require.NotNil(t, got.ClusterAgent.IsRequired, "fixture must configure the feature")
			require.True(t, *got.ClusterAgent.IsRequired)
			if tt.wantPresent {
				require.Equal(t, tt.gatewayClasses, f.config.GatewayClasses)
			} else {
				// mergeInjectorConfig only copies a non-empty list, so an empty input leaves
				// the field nil: the absence below is caused by the list, not by a lost field.
				require.Empty(t, f.config.GatewayClasses)
			}

			mgr := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
			require.NoError(t, f.ManageClusterAgent(mgr))

			envVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			require.Contains(t, envVars, &corev1.EnvVar{Name: DDAppsecProxyEnabled, Value: "true"},
				"the feature must have emitted its baseline env vars")

			if tt.wantPresent {
				assert.Contains(t, envVars, &corev1.EnvVar{Name: DDAppsecProxyGKEGatewayClasses, Value: tt.wantEnv})
				return
			}

			for _, env := range envVars {
				assert.NotEqual(t, DDAppsecProxyGKEGatewayClasses, env.Name,
					"an empty gateway class list must emit no env var at all")
			}
		})
	}
}

// deprecationLogMessage duplicates the literal Configure logs on purpose: if the
// production string changes, these tests must be updated deliberately.
const deprecationLogMessage = "appsec.* annotations are deprecated; migrate to spec.features.appsec.injector"

// captureLogger returns a logr.Logger that records every emitted line, plus a pointer
// to the recorded lines.
//
// The shared test.FeatureTestSuite overwrites feature.Options.Logger with its own zap
// logger (internal/controller/datadogagent/feature/test/testsuite.go), so the suite can
// never observe what a feature logs. Any assertion on log output has to build the
// feature directly through buildAppsecFeature with a capturing sink.
//
// Verbosity is set high enough to record every V(n) level, so a test asserting that the
// deprecation warning is absent cannot pass merely because the sink filtered it out.
func captureLogger() (logr.Logger, *[]string) {
	var lines []string
	logger := funcr.New(func(prefix, args string) {
		lines = append(lines, prefix+" "+args)
	}, funcr.Options{Verbosity: 10})
	return logger, &lines
}

func TestHasAppsecAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "nil map",
			annotations: nil,
			want:        false,
		},
		{
			name:        "empty map",
			annotations: map[string]string{},
			want:        false,
		},
		{
			name:        "unrelated annotations only",
			annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}", "agent.datadoghq.com/other": "x"},
			want:        false,
		},
		{
			name:        "appsec annotation with an empty value still counts",
			annotations: map[string]string{AnnotationNginxModuleMountPath: ""},
			want:        true,
		},
		{
			name:        "appsec annotation mixed with unrelated ones",
			annotations: map[string]string{"foo": "bar", AnnotationInjectorEnabled: "false"},
			want:        true,
		},
		{
			name:        "prefix alone with no sub-key is not an appsec annotation",
			annotations: map[string]string{"agent.datadoghq.com/appsec": "true"},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasAppsecAnnotations(tt.annotations))
		})
	}
}

// TestHasAppsecAnnotationsCoversEveryAnnotationConst mechanically pins that every
// annotation this package understands is matched by the deprecation detector, so a
// future annotation added outside the prefix cannot silently escape the warning.
func TestHasAppsecAnnotationsCoversEveryAnnotationConst(t *testing.T) {
	for _, key := range []string{
		AnnotationInjectorEnabled,
		AnnotationInjectorAutoDetect,
		AnnotationInjectorProxies,
		AnnotationInjectorProcessorAddress,
		AnnotationInjectorProcessorPort,
		AnnotationInjectorProcessorServiceName,
		AnnotationInjectorProcessorServiceNamespace,
		AnnotationInjectorMode,
		AnnotationSidecarImage,
		AnnotationSidecarImageTag,
		AnnotationSidecarPort,
		AnnotationSidecarHealthPort,
		AnnotationSidecarResourcesRequestsCPU,
		AnnotationSidecarResourcesRequestsMemory,
		AnnotationSidecarResourcesLimitsCPU,
		AnnotationSidecarResourcesLimitsMemory,
		AnnotationSidecarBodyParsingSizeLimit,
		AnnotationNginxModuleMountPath,
	} {
		t.Run(key, func(t *testing.T) {
			assert.True(t, hasAppsecAnnotations(map[string]string{key: "v"}),
				"annotation %q is not covered by the deprecation warning", key)
		})
	}
}

// TestConfigureLogsAnnotationDeprecation pins that the deprecation warning is the very
// first thing Configure does. It has to fire even when the feature is disabled, when the
// cluster-agent is too old, and when the spec is nil, because those users are exactly
// the ones who still need to hear about the migration. Every early return sits after it.
func TestConfigureLogsAnnotationDeprecation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		clusterTag  string
		nilSpec     bool
		wantLog     bool
	}{
		{
			name: "enabled appsec annotation warns",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "true",
			},
			clusterTag: "7.76.0",
			wantLog:    true,
		},
		{
			name: "disabled appsec annotation still warns",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "false",
			},
			clusterTag: "7.76.0",
			wantLog:    true,
		},
		{
			name: "appsec annotation on a too-old cluster-agent still warns",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "true",
			},
			clusterTag: "7.60.0",
			wantLog:    true,
		},
		{
			name: "malformed appsec annotation still warns",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "not-a-bool",
			},
			clusterTag: "7.76.0",
			wantLog:    true,
		},
		{
			name: "appsec annotation with a nil spec still warns",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "true",
			},
			nilSpec: true,
			wantLog: true,
		},
		{
			name:        "no annotations does not warn",
			annotations: nil,
			clusterTag:  "7.76.0",
			wantLog:     false,
		},
		{
			name:        "unrelated annotations do not warn",
			annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
			clusterTag:  "7.76.0",
			wantLog:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := testutils.NewDatadogAgentBuilder()
			if tt.clusterTag != "" {
				builder = builder.WithClusterAgentTag(tt.clusterTag)
			}
			dda := builder.WithAnnotations(tt.annotations).Build()

			logger, lines := captureLogger()
			f := buildAppsecFeature(&feature.Options{Logger: logger}).(*appsecFeature)

			spec := &dda.Spec
			if tt.nilSpec {
				spec = nil
			}
			f.Configure(dda, spec, nil)

			logged := strings.Join(*lines, "\n")
			if tt.wantLog {
				assert.Contains(t, logged, deprecationLogMessage)
			} else {
				assert.NotContains(t, logged, deprecationLogMessage)
			}
		})
	}
}

// TestConfigureDoesNotWarnForCRDOnlyConfiguration pins that a user who has fully
// migrated to spec.features.appsec.injector never sees the deprecation warning.
func TestConfigureDoesNotWarnForCRDOnlyConfiguration(t *testing.T) {
	dda := testutils.NewDatadogAgentBuilder().
		WithClusterAgentTag("7.76.0").
		WithAppsecInjector(&v2alpha1.AppsecInjectorConfig{
			Enabled: ptr.To(true),
		}).
		Build()

	logger, lines := captureLogger()
	f := buildAppsecFeature(&feature.Options{Logger: logger}).(*appsecFeature)
	got := f.Configure(dda, &dda.Spec, nil)

	assert.NotContains(t, strings.Join(*lines, "\n"), deprecationLogMessage)
	require.NotNil(t, got.ClusterAgent.IsRequired)
	assert.True(t, *got.ClusterAgent.IsRequired, "a CRD-only DatadogAgent must enable the feature")
}

// TestConfigureNilSpecDoesNotPanic pins the nil-spec guard. Without it, an *enabled*
// appsec annotation walks past every version gate (clusterAgentVersion is nil-safe and
// falls back to the latest agent version) and reaches
// constants.GetClusterAgentServiceAccount, which dereferences ddaSpec.Override and
// panics. The guard is load-bearing, not decorative.
func TestConfigureNilSpecDoesNotPanic(t *testing.T) {
	dda := testutils.NewDatadogAgentBuilder().
		WithAnnotations(map[string]string{
			AnnotationInjectorEnabled: "true",
		}).
		Build()

	f := buildAppsecFeature(nil).(*appsecFeature)

	var got feature.RequiredComponents
	require.NotPanics(t, func() {
		got = f.Configure(dda, nil, nil)
	}, "Configure must not dereference a nil ddaSpec")

	assert.Nil(t, got.ClusterAgent.IsRequired, "a nil spec cannot require the cluster-agent")
}

// TestConfigureNilSafety walks every shape the spec.features.appsec chain can take,
// including empty nested objects, and pins that none of them panics.
func TestConfigureNilSafety(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(dda *v2alpha1.DatadogAgent)
		wantCfg bool
	}{
		{
			name:   "Features nil",
			mutate: func(dda *v2alpha1.DatadogAgent) { dda.Spec.Features = nil },
		},
		{
			name:   "Features present, Appsec nil",
			mutate: func(dda *v2alpha1.DatadogAgent) { dda.Spec.Features = &v2alpha1.DatadogFeatures{} },
		},
		{
			name: "Appsec present, Injector nil",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				dda.Spec.Features.Appsec = &v2alpha1.AppsecFeatureConfig{}
			},
		},
		{
			name: "Injector present but entirely empty",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				dda.Spec.Features.Appsec = &v2alpha1.AppsecFeatureConfig{Injector: &v2alpha1.AppsecInjectorConfig{}}
			},
		},
		{
			name: "every nested object present but empty",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				dda.Spec.Features.Appsec = &v2alpha1.AppsecFeatureConfig{
					Injector: &v2alpha1.AppsecInjectorConfig{
						Processor: &v2alpha1.AppsecInjectorProcessorConfig{
							Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{},
						},
						Sidecar: &v2alpha1.AppsecInjectorSidecarConfig{},
						Nginx:   &v2alpha1.AppsecInjectorNginxConfig{},
						GKE:     &v2alpha1.AppsecInjectorGKEConfig{},
					},
				}
			},
		},
		{
			name: "empty nested objects with enabled set still configure",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				dda.Spec.Features.Appsec = &v2alpha1.AppsecFeatureConfig{
					Injector: &v2alpha1.AppsecInjectorConfig{
						Enabled:   ptr.To(true),
						Processor: &v2alpha1.AppsecInjectorProcessorConfig{},
						Sidecar:   &v2alpha1.AppsecInjectorSidecarConfig{},
						Nginx:     &v2alpha1.AppsecInjectorNginxConfig{},
						GKE:       &v2alpha1.AppsecInjectorGKEConfig{},
					},
				}
			},
			wantCfg: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.82.0").
				Build()
			tt.mutate(dda)

			f := buildAppsecFeature(nil).(*appsecFeature)

			var got feature.RequiredComponents
			require.NotPanics(t, func() {
				got = f.Configure(dda, &dda.Spec, nil)
			})

			if tt.wantCfg {
				require.NotNil(t, got.ClusterAgent.IsRequired)
				assert.True(t, *got.ClusterAgent.IsRequired)
			} else {
				assert.Nil(t, got.ClusterAgent.IsRequired)
			}
		})
	}
}

// TestConfigureAnnotationParsePrecedence pins the CRD-aware parse skip. A malformed
// annotation whose field the CRD already sets must not fail Configure, because the parsed
// value would have been discarded by the merge anyway. A malformed annotation on a field
// the CRD leaves unset must still fail, preserving annotation-only strictness.
func TestConfigureAnnotationParsePrecedence(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		injector    *v2alpha1.AppsecInjectorConfig
		wantCfg     bool
		assertCfg   func(t *testing.T, cfg Config)
	}{
		{
			name: "malformed proxies rescued by CRD proxies",
			annotations: map[string]string{
				AnnotationInjectorEnabled:              "true",
				AnnotationInjectorProxies:              "not-json",
				AnnotationInjectorProcessorServiceName: "appsec-processor",
			},
			injector: &v2alpha1.AppsecInjectorConfig{Proxies: []string{"envoy-gateway"}},
			wantCfg:  true,
			assertCfg: func(t *testing.T, cfg Config) {
				assert.Equal(t, []string{"envoy-gateway"}, cfg.Proxies)
			},
		},
		{
			name: "malformed proxies without CRD proxies is rejected",
			annotations: map[string]string{
				AnnotationInjectorEnabled:              "true",
				AnnotationInjectorProxies:              "not-json",
				AnnotationInjectorProcessorServiceName: "appsec-processor",
			},
			injector: nil,
			wantCfg:  false,
		},
		{
			name: "malformed proxies with a CRD injector that does not set proxies is rejected",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "true",
				AnnotationInjectorProxies: "not-json",
			},
			injector: &v2alpha1.AppsecInjectorConfig{Enabled: ptr.To(true)},
			wantCfg:  false,
		},
		{
			name: "malformed enabled rescued by CRD enabled",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "not-a-bool",
			},
			injector: &v2alpha1.AppsecInjectorConfig{Enabled: ptr.To(true)},
			wantCfg:  true,
			assertCfg: func(t *testing.T, cfg Config) {
				assert.True(t, cfg.Enabled)
			},
		},
		{
			name: "malformed enabled without CRD enabled is rejected",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "not-a-bool",
			},
			injector: nil,
			wantCfg:  false,
		},
		{
			name: "malformed autoDetect rescued by CRD autoDetect",
			annotations: map[string]string{
				AnnotationInjectorEnabled:    "true",
				AnnotationInjectorAutoDetect: "not-a-bool",
			},
			injector: &v2alpha1.AppsecInjectorConfig{AutoDetect: ptr.To(true)},
			wantCfg:  true,
			assertCfg: func(t *testing.T, cfg Config) {
				require.NotNil(t, cfg.AutoDetect)
				assert.True(t, *cfg.AutoDetect)
			},
		},
		{
			name: "malformed processor port rescued by CRD processor port",
			annotations: map[string]string{
				AnnotationInjectorEnabled:       "true",
				AnnotationInjectorProcessorPort: "not-a-number",
			},
			injector: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{Port: ptr.To(int32(8443))},
			},
			wantCfg: true,
			assertCfg: func(t *testing.T, cfg Config) {
				assert.Equal(t, 8443, cfg.ProcessorPort)
			},
		},
		{
			name: "CRD mode rescues an annotation-only external config missing the service name",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "true",
				AnnotationInjectorMode:    "external",
			},
			injector: &v2alpha1.AppsecInjectorConfig{
				Processor: &v2alpha1.AppsecInjectorProcessorConfig{
					Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{Name: ptr.To("appsec-processor")},
				},
			},
			wantCfg: true,
			assertCfg: func(t *testing.T, cfg Config) {
				assert.Equal(t, "appsec-processor", cfg.ProcessorServiceName)
			},
		},
		{
			name: "annotation-only external config without a service name is still rejected",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "true",
				AnnotationInjectorMode:    "external",
			},
			injector: nil,
			wantCfg:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag("7.82.0").
				WithAnnotations(tt.annotations)
			if tt.injector != nil {
				builder = builder.WithAppsecInjector(tt.injector)
			}
			dda := builder.Build()

			f := buildAppsecFeature(nil).(*appsecFeature)
			got := f.Configure(dda, &dda.Spec, nil)

			if tt.wantCfg {
				require.NotNil(t, got.ClusterAgent.IsRequired)
				assert.True(t, *got.ClusterAgent.IsRequired)
				assert.Contains(t, got.ClusterAgent.Containers, apicommon.ClusterAgentContainerName)
			} else {
				assert.Nil(t, got.ClusterAgent.IsRequired)
			}

			if tt.assertCfg != nil {
				tt.assertCfg(t, f.config)
			}
		})
	}
}

// TestConfigureCRDWinsOverAnnotations pins per-field precedence end to end through
// Configure: whenever both sources set a field, f.config holds the CRD value.
func TestConfigureCRDWinsOverAnnotations(t *testing.T) {
	dda := testutils.NewDatadogAgentBuilder().
		WithClusterAgentTag("7.82.0").
		WithAnnotations(map[string]string{
			AnnotationInjectorEnabled:                   "false",
			AnnotationInjectorAutoDetect:                "false",
			AnnotationInjectorProxies:                   `["istio"]`,
			AnnotationInjectorMode:                      "sidecar",
			AnnotationInjectorProcessorPort:             "1111",
			AnnotationInjectorProcessorAddress:          "annotation.example.com",
			AnnotationInjectorProcessorServiceName:      "annotation-svc",
			AnnotationInjectorProcessorServiceNamespace: "annotation-ns",
			AnnotationNginxModuleMountPath:              "/annotation/modules",
		}).
		WithAppsecInjector(&v2alpha1.AppsecInjectorConfig{
			Enabled:    ptr.To(true),
			AutoDetect: ptr.To(true),
			Proxies:    []string{"envoy-gateway"},
			Mode:       ptr.To("external"),
			Processor: &v2alpha1.AppsecInjectorProcessorConfig{
				Address: ptr.To("crd.example.com"),
				Port:    ptr.To(int32(2222)),
				Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{
					Name:      ptr.To("crd-svc"),
					Namespace: ptr.To("crd-ns"),
				},
			},
			Nginx: &v2alpha1.AppsecInjectorNginxConfig{ModuleMountPath: ptr.To("/crd/modules")},
		}).
		Build()

	f := buildAppsecFeature(nil).(*appsecFeature)
	got := f.Configure(dda, &dda.Spec, nil)

	require.NotNil(t, got.ClusterAgent.IsRequired, "the CRD enables the feature even though the annotation disables it")
	assert.True(t, *got.ClusterAgent.IsRequired)

	assert.True(t, f.config.Enabled)
	require.NotNil(t, f.config.AutoDetect)
	assert.True(t, *f.config.AutoDetect)
	assert.Equal(t, []string{"envoy-gateway"}, f.config.Proxies)
	assert.Equal(t, "external", f.config.Mode)
	assert.Equal(t, 2222, f.config.ProcessorPort)
	assert.Equal(t, "crd.example.com", f.config.ProcessorAddress)
	assert.Equal(t, "crd-svc", f.config.ProcessorServiceName)
	assert.Equal(t, "crd-ns", f.config.ProcessorServiceNamespace)
	assert.Equal(t, "/crd/modules", f.config.NginxModuleMountPath)
}

// gkeExternalInjector returns a CRD injector configuration that reaches the GKE version
// gate. Enabled and ProcessorServiceName are both mandatory here: without Enabled the
// isEnabled() gate returns first, and without ProcessorServiceName the external-mode rule
// in Validate() returns first. Either omission would make a "gated off" assertion pass for
// the wrong reason.
func gkeExternalInjector() *v2alpha1.AppsecInjectorConfig {
	return &v2alpha1.AppsecInjectorConfig{
		Enabled: ptr.To(true),
		Mode:    ptr.To("external"),
		Proxies: []string{"gke-gateway"},
		Processor: &v2alpha1.AppsecInjectorProcessorConfig{
			Service: &v2alpha1.AppsecInjectorProcessorServiceConfig{Name: ptr.To("appsec-processor")},
		},
	}
}

// gkeGatewayClassesOnlyInjector relies on agent-side autoDetect: gatewayClasses alone
// triggers the GKE version gate without requiring external mode.
func gkeGatewayClassesOnlyInjector() *v2alpha1.AppsecInjectorConfig {
	return &v2alpha1.AppsecInjectorConfig{
		Enabled: ptr.To(true),
		GKE:     &v2alpha1.AppsecInjectorGKEConfig{GatewayClasses: []string{"gke-l7-global-external-managed"}},
	}
}

// TestConfigureGKEVersionGate pins the cluster-agent minimum for GKE Gateway injection.
// Every negative is paired with an identical-except-version positive control, and each
// gated-off case additionally asserts that the config it rejected was enabled, valid and
// GKE-requiring, which proves the version gate is what stopped it rather than isEnabled()
// or Validate() short-circuiting earlier.
func TestConfigureGKEVersionGate(t *testing.T) {
	tests := []struct {
		name       string
		clusterTag string
		injector   *v2alpha1.AppsecInjectorConfig
		wantCfg    bool
	}{
		{
			name:       "gke-gateway proxy on 7.81.0 is gated off",
			clusterTag: "7.81.0",
			injector:   gkeExternalInjector(),
			wantCfg:    false,
		},
		{
			name:       "gke-gateway proxy on 7.82.0 configures",
			clusterTag: "7.82.0",
			injector:   gkeExternalInjector(),
			wantCfg:    true,
		},
		{
			name:       "gke-gateway proxy on 7.83.0 configures",
			clusterTag: "7.83.0",
			injector:   gkeExternalInjector(),
			wantCfg:    true,
		},
		{
			name:       "gke-gateway proxy on 8.0.0 configures",
			clusterTag: "8.0.0",
			injector:   gkeExternalInjector(),
			wantCfg:    true,
		},
		{
			name:       "gatewayClasses only on 7.81.0 is gated off",
			clusterTag: "7.81.0",
			injector:   gkeGatewayClassesOnlyInjector(),
			wantCfg:    false,
		},
		{
			name:       "gatewayClasses only on 7.82.0 configures",
			clusterTag: "7.82.0",
			injector:   gkeGatewayClassesOnlyInjector(),
			wantCfg:    true,
		},
		{
			// utils.IsAboveMinVersion returns its fallback, true, for a tag it cannot parse
			// (pkg/utils/version.go), so custom and development images are never gated. This
			// matches the existing base and nginx gates and is deliberate, not accidental.
			name:       "unparseable custom tag fails open and configures",
			clusterTag: "latest",
			injector:   gkeExternalInjector(),
			wantCfg:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := testutils.NewDatadogAgentBuilder().
				WithClusterAgentTag(tt.clusterTag).
				WithAppsecInjector(tt.injector).
				Build()

			f := buildAppsecFeature(nil).(*appsecFeature)
			got := f.Configure(dda, &dda.Spec, nil)

			if tt.wantCfg {
				require.NotNil(t, got.ClusterAgent.IsRequired)
				assert.True(t, *got.ClusterAgent.IsRequired)
				assert.Contains(t, got.ClusterAgent.Containers, apicommon.ClusterAgentContainerName)
				return
			}

			assert.Nil(t, got.ClusterAgent.IsRequired)

			// Non-vacuity controls: the rejected config passed every earlier gate.
			assert.True(t, f.config.isEnabled(), "fixture must reach the version gate: isEnabled() returned first")
			assert.NoError(t, f.config.Validate(), "fixture must reach the version gate: Validate() returned first")
			assert.True(t, f.config.requiresGKESupport(), "fixture must actually require GKE support")
		})
	}
}

// TestConfigureGKEGateUsesDefaultClusterAgentVersion pins the reality of an unpinned
// cluster-agent: clusterAgentVersion falls back to images.AgentLatestVersion, which is
// below ClusterAgentGKEMinVersion today, so a GKE configuration with no image override is
// correctly gated off. This is intended behavior, not a bug.
func TestConfigureGKEGateUsesDefaultClusterAgentVersion(t *testing.T) {
	require.False(t, utils.IsAboveMinVersion(images.AgentLatestVersion, ClusterAgentGKEMinVersion, nil),
		"this test only means something while the default agent version is below %s", ClusterAgentGKEMinVersion)

	dda := testutils.NewDatadogAgentBuilder().
		WithAppsecInjector(gkeExternalInjector()).
		Build()
	require.Empty(t, dda.Spec.Override, "the fixture must not pin a cluster-agent image")

	f := buildAppsecFeature(nil).(*appsecFeature)
	got := f.Configure(dda, &dda.Spec, nil)

	assert.Nil(t, got.ClusterAgent.IsRequired)
	assert.True(t, f.config.isEnabled())
	assert.NoError(t, f.config.Validate())
	assert.True(t, f.config.requiresGKESupport())
}

// TestAppsecGKEGatewayMatchedPair is the non-vacuity companion to the two suite cases
// "Appsec GKE gateway proxy on 7.8{1,2}.0 ...". test.FeatureTestSuite never exposes the
// feature instance, so the triple that proves the *version gate* rejected 7.81.0 - rather
// than isEnabled() or Validate() short-circuiting earlier - has to be asserted by driving
// Configure directly. Both legs are built from the same gkeExternalDDA fixture, so they
// differ only in the cluster-agent tag.
func TestAppsecGKEGatewayMatchedPair(t *testing.T) {
	for _, tt := range []struct {
		clusterAgentTag string
		wantCfg         bool
	}{
		{clusterAgentTag: "7.81.0", wantCfg: false},
		{clusterAgentTag: "7.82.0", wantCfg: true},
	} {
		t.Run(tt.clusterAgentTag, func(t *testing.T) {
			dda := gkeExternalDDA(tt.clusterAgentTag)

			f := buildAppsecFeature(nil).(*appsecFeature)
			got := f.Configure(dda, &dda.Spec, nil)

			if !tt.wantCfg {
				assert.Nil(t, got.ClusterAgent.IsRequired)

				// Non-vacuity controls: the rejected config was enabled, valid, and really
				// GKE-requiring, so only the version gate can have returned.
				assert.True(t, f.config.isEnabled(), "fixture must reach the version gate: isEnabled() returned first")
				assert.NoError(t, f.config.Validate(), "fixture must reach the version gate: Validate() returned first")
				assert.True(t, f.config.requiresGKESupport(), "fixture must actually require GKE support")
				return
			}

			require.NotNil(t, got.ClusterAgent.IsRequired)
			assert.True(t, *got.ClusterAgent.IsRequired)
			assert.Contains(t, got.ClusterAgent.Containers, apicommon.ClusterAgentContainerName)

			mgr := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
			require.NoError(t, f.ManageClusterAgent(mgr))

			envVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			assert.Contains(t, envVars, &corev1.EnvVar{Name: DDAppsecProxyEnabled, Value: "true"})
			assert.Contains(t, envVars, &corev1.EnvVar{Name: DDAppsecProxyProxies, Value: `["gke-gateway"]`})
			assert.Contains(t, envVars, &corev1.EnvVar{Name: DDClusterAgentAppsecInjectorMode, Value: "external"})
			assert.Contains(t, envVars, &corev1.EnvVar{Name: DDClusterAgentAppsecInjectorProcessorServiceName, Value: "appsec-processor"})
		})
	}
}

// TestConfigureQAScenarios are the two hand-run QA scenarios for this change.
func TestConfigureQAScenarios(t *testing.T) {
	t.Run("QA happy: CRD injector enabled on 7.82.0 requires the cluster-agent", func(t *testing.T) {
		dda := testutils.NewDatadogAgentBuilder().
			WithClusterAgentTag("7.82.0").
			WithAppsecInjector(&v2alpha1.AppsecInjectorConfig{Enabled: ptr.To(true)}).
			Build()

		f := buildAppsecFeature(nil).(*appsecFeature)
		got := f.Configure(dda, &dda.Spec, nil)

		require.NotNil(t, got.ClusterAgent.IsRequired)
		assert.True(t, *got.ClusterAgent.IsRequired)
	})

	t.Run("QA failure: matched pair around the GKE minimum", func(t *testing.T) {
		for _, tc := range []struct {
			tag     string
			wantCfg bool
		}{
			{tag: "7.81.0", wantCfg: false},
			{tag: "7.82.0", wantCfg: true},
		} {
			t.Run(tc.tag, func(t *testing.T) {
				injector := gkeExternalInjector()
				require.NotNil(t, injector.Enabled, "matched-pair fixture must set Enabled")
				require.True(t, *injector.Enabled)
				require.NotNil(t, injector.Processor.Service.Name, "matched-pair fixture must set ProcessorServiceName")

				dda := testutils.NewDatadogAgentBuilder().
					WithClusterAgentTag(tc.tag).
					WithAppsecInjector(injector).
					Build()

				f := buildAppsecFeature(nil).(*appsecFeature)
				got := f.Configure(dda, &dda.Spec, nil)

				if tc.wantCfg {
					require.NotNil(t, got.ClusterAgent.IsRequired)
					assert.True(t, *got.ClusterAgent.IsRequired)
				} else {
					assert.Nil(t, got.ClusterAgent.IsRequired)
				}
			})
		}
	})
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
				AutoDetect:           ptr.To(true),
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
			name: "enabled in sidecar mode without ProcessorServiceName",
			annotations: map[string]string{
				AnnotationInjectorEnabled:    "true",
				AnnotationInjectorAutoDetect: "true",
				AnnotationInjectorMode:       "sidecar",
			},
			wantConfig: Config{
				Enabled:    true,
				AutoDetect: ptr.To(true),
				Mode:       "sidecar",
			},
			wantErr: false,
		},
		{
			name: "enabled in external mode without ProcessorServiceName returns error",
			annotations: map[string]string{
				AnnotationInjectorEnabled:    "true",
				AnnotationInjectorAutoDetect: "true",
				AnnotationInjectorMode:       "external",
			},
			wantErr: true,
		},
		{
			name: "invalid mode value",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "true",
				AnnotationInjectorMode:    "invalid-mode",
			},
			wantErr: true,
		},
		{
			name: "invalid sidecar port annotation",
			annotations: map[string]string{
				AnnotationInjectorEnabled: "true",
				AnnotationSidecarPort:     "99999",
			},
			wantErr: true,
		},
		{
			name: "invalid sidecar health port annotation",
			annotations: map[string]string{
				AnnotationInjectorEnabled:   "true",
				AnnotationSidecarHealthPort: "0",
			},
			wantErr: true,
		},
		{
			name: "invalid resource quantity annotation",
			annotations: map[string]string{
				AnnotationInjectorEnabled:           "true",
				AnnotationSidecarResourcesLimitsCPU: "not-valid",
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
				AutoDetect:                ptr.To(false),
				Proxies:                   []string{"envoy-gateway"},
				ProcessorPort:             8080,
				ProcessorAddress:          "processor.example.com",
				ProcessorServiceName:      "appsec-svc",
				ProcessorServiceNamespace: "datadog",
			},
			wantErr: false,
		},
		{
			name: "nginx module mount path parsed correctly",
			annotations: map[string]string{
				AnnotationInjectorEnabled:      "true",
				AnnotationInjectorAutoDetect:   "true",
				AnnotationNginxModuleMountPath: "/custom/modules",
			},
			wantConfig: Config{
				Enabled:              true,
				AutoDetect:           ptr.To(true),
				NginxModuleMountPath: "/custom/modules",
			},
			wantErr: false,
		},
		{
			name: "nginx annotations unset results in empty fields",
			annotations: map[string]string{
				AnnotationInjectorEnabled:    "true",
				AnnotationInjectorAutoDetect: "true",
			},
			wantConfig: Config{
				Enabled:    true,
				AutoDetect: ptr.To(true),
			},
			wantErr: false,
		},
		{
			name: "nginx annotations empty string results in empty fields",
			annotations: map[string]string{
				AnnotationInjectorEnabled:      "true",
				AnnotationInjectorAutoDetect:   "true",
				AnnotationNginxModuleMountPath: "",
			},
			wantConfig: Config{
				Enabled:    true,
				AutoDetect: ptr.To(true),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A nil injector is the pure annotation path: no CRD field is set, so no
			// annotation is skipped and every parse failure still surfaces.
			config, err := parseAnnotations(tt.annotations, nil)
			if err == nil {
				err = config.Validate()
			}

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
				assert.Equal(t, tt.wantConfig.NginxModuleMountPath, config.NginxModuleMountPath)
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
				AutoDetect:           ptr.To(true),
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
			name: "gke-gateway is a valid proxy value",
			config: Config{
				Enabled:              true,
				Proxies:              []string{"gke-gateway"},
				Mode:                 "external",
				ProcessorServiceName: "x",
			},
			wantErr: false,
		},
		{
			name: "invalid port - negative",
			config: Config{
				Enabled:              true,
				AutoDetect:           ptr.To(true),
				ProcessorPort:        -1,
				ProcessorServiceName: "appsec-processor",
			},
			wantErr: true,
		},
		{
			name: "invalid port - too high",
			config: Config{
				Enabled:              true,
				AutoDetect:           ptr.To(true),
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
			name: "missing service name in external mode",
			config: Config{
				Enabled:    true,
				AutoDetect: ptr.To(true),
				Mode:       "external",
			},
			wantErr: true,
		},
		{
			name: "missing service name in sidecar mode is allowed",
			config: Config{
				Enabled:    true,
				AutoDetect: ptr.To(true),
				Mode:       "sidecar",
			},
			wantErr: false,
		},
		{
			name: "missing service name with no mode is allowed (defaults to sidecar)",
			config: Config{
				Enabled:    true,
				AutoDetect: ptr.To(true),
			},
			wantErr: false,
		},
		{
			name: "invalid mode value",
			config: Config{
				Enabled: true,
				Mode:    "invalid-mode",
			},
			wantErr: true,
		},
		{
			name: "istio-gateway is a valid proxy value",
			config: Config{
				Enabled: true,
				Proxies: []string{"istio-gateway"},
			},
			wantErr: false,
		},
		{
			name: "ingress-nginx is a valid proxy value",
			config: Config{
				Enabled: true,
				Proxies: []string{"ingress-nginx"},
			},
			wantErr: false,
		},
		{
			name: "invalid sidecar port - not a number",
			config: Config{
				Enabled:     true,
				SidecarPort: "not-a-port",
			},
			wantErr: true,
		},
		{
			name: "invalid sidecar port - out of range",
			config: Config{
				Enabled:     true,
				SidecarPort: "99999",
			},
			wantErr: true,
		},
		{
			name: "invalid sidecar port - zero",
			config: Config{
				Enabled:     true,
				SidecarPort: "0",
			},
			wantErr: true,
		},
		{
			name: "valid sidecar port",
			config: Config{
				Enabled:     true,
				SidecarPort: "8080",
			},
			wantErr: false,
		},
		{
			name: "invalid sidecar health port - out of range",
			config: Config{
				Enabled:           true,
				SidecarHealthPort: "0",
			},
			wantErr: true,
		},
		{
			name: "invalid body parsing size limit - not a number",
			config: Config{
				Enabled:                     true,
				SidecarBodyParsingSizeLimit: "abc",
			},
			wantErr: true,
		},
		{
			name: "valid body parsing size limit - positive",
			config: Config{
				Enabled:                     true,
				SidecarBodyParsingSizeLimit: "1048576",
			},
			wantErr: false,
		},
		{
			name: "valid body parsing size limit - negative (disables)",
			config: Config{
				Enabled:                     true,
				SidecarBodyParsingSizeLimit: "-1",
			},
			wantErr: false,
		},
		{
			name: "invalid resource quantity - CPU",
			config: Config{
				Enabled:                   true,
				SidecarResourcesLimitsCPU: "not-a-quantity",
			},
			wantErr: true,
		},
		{
			name: "valid resource quantities",
			config: Config{
				Enabled:                        true,
				SidecarResourcesRequestsCPU:    "100m",
				SidecarResourcesRequestsMemory: "128Mi",
				SidecarResourcesLimitsCPU:      "500m",
				SidecarResourcesLimitsMemory:   "256Mi",
			},
			wantErr: false,
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
				AutoDetect: ptr.To(true),
			},
			wantEnabled: true,
		},
		{
			name: "enabled with autoDetect false and proxies",
			config: Config{
				Enabled:    true,
				AutoDetect: ptr.To(false),
				Proxies:    []string{"envoy-gateway"},
			},
			wantEnabled: true,
		},
		{
			name: "enabled with autoDetect false but no proxies",
			config: Config{
				Enabled:    true,
				AutoDetect: ptr.To(false),
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
