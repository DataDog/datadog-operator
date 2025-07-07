// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	hostCAPath            = "/host/ca/path/ca.crt"
	agentCAPath           = "/agent/ca/path/ca.crt"
	podResourcesSocketDir = "/var/lib/kubelet/pod-resources/"
	podResourcesSocket    = podResourcesSocketDir + "kubelet.sock"
	dockerSocketPath      = "/docker/socket/path/docker.sock"
	secretBackendCommand  = "foo.sh"
	secretBackendArgs     = "bar baz"
	secretBackendTimeout  = 60
	ddaName               = "datadog"
	ddaNamespace          = "system"
	secretNamespace       = "postgres"
)

var secretNames = []string{"db-username", "db-password"}

func TestNodeAgentComponenGlobalSettings(t *testing.T) {
	logger := logf.Log.WithName("TestRequiredComponents")

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	tests := []struct {
		name                           string
		dda                            *v2alpha1.DatadogAgent
		singleContainerStrategyEnabled bool
		wantCoreAgentVolumeMounts      []*corev1.VolumeMount
		wantVolumeMounts               []*corev1.VolumeMount
		wantVolumes                    []*corev1.Volume
		wantCoreAgentEnvVars           []*corev1.EnvVar
		wantEnvVars                    []*corev1.EnvVar
		want                           func(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedCoreAgentEnvVars, expectedEnvVars []*corev1.EnvVar, expectedVolumes []*corev1.Volume, expectedCoreAgentVolumeMounts, expectedVolumeMounts []*corev1.VolumeMount)
		wantDependency                 func(t testing.TB, resourcesManager feature.ResourceManagers)
	}{
		{
			name:                           "Kubelet volume configured",
			singleContainerStrategyEnabled: false,
			dda: testutils.NewDatadogAgentBuilder().
				WithGlobalKubeletConfig(hostCAPath, agentCAPath, true, podResourcesSocketDir).
				WithGlobalDockerSocketPath(dockerSocketPath).
				WithCredentials("apiKey", "appKey").
				BuildWithDefaults(),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPIKeyKey,
						},
					},
				},
				{
					Name: constants.DDAppKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPPKeyKey,
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-token",
							},
							Key: common.DefaultTokenKey,
						},
					},
				},
				{
					Name:  DDKubeletTLSVerify,
					Value: "true",
				},
				{
					Name:  DDKubeletCAPath,
					Value: agentCAPath,
				},
				{
					Name:  DockerHost,
					Value: "unix:///host" + dockerSocketPath,
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(kubeletCAVolumes, criSocketVolume),
			wantVolumeMounts:          getExpectedVolumeMounts(kubeletCAVolumes, criSocketVolume),
			wantVolumes:               getExpectedVolumes(kubeletCAVolumes, criSocketVolume),
			want:                      assertAll,
		},
		{
			name:                           "Kubelet volume configured",
			singleContainerStrategyEnabled: true,
			dda: testutils.NewDatadogAgentBuilder().
				WithGlobalKubeletConfig(hostCAPath, agentCAPath, true, podResourcesSocketDir).
				WithGlobalDockerSocketPath(dockerSocketPath).
				WithCredentials("apiKey", "appKey").
				BuildWithDefaults(),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPIKeyKey,
						},
					},
				},
				{
					Name: constants.DDAppKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPPKeyKey,
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-token",
							},
							Key: common.DefaultTokenKey,
						},
					},
				},
				{
					Name:  DDKubeletTLSVerify,
					Value: "true",
				},
				{
					Name:  DDKubeletCAPath,
					Value: agentCAPath,
				},
				{
					Name:  DockerHost,
					Value: "unix:///host" + dockerSocketPath,
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(kubeletCAVolumes, criSocketVolume),
			wantVolumeMounts:          getExpectedVolumeMounts(kubeletCAVolumes, criSocketVolume),
			wantVolumes:               getExpectedVolumes(kubeletCAVolumes, criSocketVolume),
			want:                      assertAllAgentSingleContainer,
		},
		{
			name:                           "Checks tag cardinality set to orchestrator",
			singleContainerStrategyEnabled: false,
			dda: testutils.NewDatadogAgentBuilder().
				WithChecksTagCardinality("orchestrator").
				WithCredentials("apiKey", "appKey").
				BuildWithDefaults(),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  DDChecksTagCardinality,
					Value: "orchestrator",
				},
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPIKeyKey,
						},
					},
				},
				{
					Name: constants.DDAppKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPPKeyKey,
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-token",
							},
							Key: common.DefaultTokenKey,
						},
					},
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(),
			wantVolumeMounts:          getExpectedVolumeMounts(),
			wantVolumes:               getExpectedVolumes(),
			want:                      assertAll,
		},
		{
			name:                           "Unified origin detection activated",
			singleContainerStrategyEnabled: false,
			dda: testutils.NewDatadogAgentBuilder().
				WithOriginDetectionUnified(true).
				WithCredentials("apiKey", "appKey").
				BuildWithDefaults(),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  DDOriginDetectionUnified,
					Value: "true",
				},
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPIKeyKey,
						},
					},
				},
				{
					Name: constants.DDAppKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPPKeyKey,
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-token",
							},
							Key: common.DefaultTokenKey,
						},
					},
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(),
			wantVolumeMounts:          getExpectedVolumeMounts(),
			wantVolumes:               getExpectedVolumes(),
			want:                      assertAll,
		},
		{
			name:                           "Global environment variable configured",
			singleContainerStrategyEnabled: false,
			dda: testutils.NewDatadogAgentBuilder().
				WithCredentials("apiKey", "appKey").
				WithEnvVars([]corev1.EnvVar{
					{
						Name:  "envA",
						Value: "valueA",
					},
					{
						Name:  "envB",
						Value: "valueB",
					},
				}).
				BuildWithDefaults(),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  "envA",
					Value: "valueA",
				},
				{
					Name:  "envB",
					Value: "valueB",
				},
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPIKeyKey,
						},
					},
				},
				{
					Name: constants.DDAppKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-secret",
							},
							Key: v2alpha1.DefaultAPPKeyKey,
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "-token",
							},
							Key: common.DefaultTokenKey,
						},
					},
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(),
			wantVolumeMounts:          getExpectedVolumeMounts(),
			wantVolumes:               getExpectedVolumes(),
			want:                      assertAll,
		},
		{
			name:                           "Secret backend - global permissions",
			singleContainerStrategyEnabled: false,
			dda: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				testutils.NewDatadogAgentBuilder().
					WithGlobalSecretBackendGlobalPerms(secretBackendCommand, secretBackendArgs, secretBackendTimeout, 0).
					WithCredentials("apiKey", "appKey").
					BuildWithDefaults(),
			),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  DDSecretBackendCommand,
					Value: secretBackendCommand,
				},
				{
					Name:  DDSecretBackendArguments,
					Value: secretBackendArgs,
				},
				{
					Name:  DDSecretBackendTimeout,
					Value: "60",
				},
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-secret",
							},
							Key: v2alpha1.DefaultAPIKeyKey,
						},
					},
				},
				{
					Name: constants.DDAppKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-secret",
							},
							Key: v2alpha1.DefaultAPPKeyKey,
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-token",
							},
							Key: common.DefaultTokenKey,
						},
					},
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(),
			wantVolumeMounts:          getExpectedVolumeMounts(),
			wantVolumes:               getExpectedVolumes(),
			want:                      assertAll,
			wantDependency:            assertSecretBackendGlobalPerms,
		},
		{
			name:                           "Secret backend - with refresh interval",
			singleContainerStrategyEnabled: false,
			dda: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				testutils.NewDatadogAgentBuilder().
					WithGlobalSecretBackendGlobalPerms(secretBackendCommand, secretBackendArgs, secretBackendTimeout, 3600).
					WithCredentials("apiKey", "appKey").
					BuildWithDefaults(),
			),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  DDSecretBackendCommand,
					Value: secretBackendCommand,
				},
				{
					Name:  DDSecretBackendArguments,
					Value: secretBackendArgs,
				},
				{
					Name:  DDSecretBackendTimeout,
					Value: "60",
				},
				{
					Name:  DDSecretRefreshInterval,
					Value: "3600",
				},
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-secret",
							},
							Key: v2alpha1.DefaultAPIKeyKey,
						},
					},
				},
				{
					Name: constants.DDAppKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-secret",
							},
							Key: v2alpha1.DefaultAPPKeyKey,
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-token",
							},
							Key: common.DefaultTokenKey,
						},
					},
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(),
			wantVolumeMounts:          getExpectedVolumeMounts(),
			wantVolumes:               getExpectedVolumes(),
			want:                      assertAll,
			wantDependency:            assertSecretBackendGlobalPerms,
		},
		{
			name:                           "Secret backend - specific secret permissions",
			singleContainerStrategyEnabled: false,
			dda: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				testutils.NewDatadogAgentBuilder().
					WithGlobalSecretBackendSpecificRoles(secretBackendCommand, secretBackendArgs, secretBackendTimeout, 0, secretNamespace, secretNames).
					WithCredentials("apiKey", "appKey").
					BuildWithDefaults(),
			),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  DDSecretBackendCommand,
					Value: secretBackendCommand,
				},
				{
					Name:  DDSecretBackendArguments,
					Value: secretBackendArgs,
				},
				{
					Name:  DDSecretBackendTimeout,
					Value: "60",
				},
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-secret",
							},
							Key: v2alpha1.DefaultAPIKeyKey,
						},
					},
				},
				{
					Name: constants.DDAppKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-secret",
							},
							Key: v2alpha1.DefaultAPPKeyKey,
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-token",
							},
							Key: common.DefaultTokenKey,
						},
					},
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(),
			wantVolumeMounts:          getExpectedVolumeMounts(),
			wantVolumes:               getExpectedVolumes(),
			want:                      assertAll,
			wantDependency:            assertSecretBackendSpecificPerms,
		},
		{
			name:                           "Credentials + token from existing secret",
			singleContainerStrategyEnabled: false,
			dda: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				testutils.NewDatadogAgentBuilder().
					WithCredentialsFromSecret("apiKeyName", "apiKeyKey", "appKeyName", "appKeyKey").
					WithDCATokenFromSecret("tokenName", "tokenKey").
					BuildWithDefaults(),
			),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "apiKeyName",
							},
							Key: "apiKeyKey",
						},
					},
				},
				{
					Name: constants.DDAppKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "appKeyName",
							},
							Key: "appKeyKey",
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "tokenName",
							},
							Key: "tokenKey",
						},
					},
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(),
			wantVolumeMounts:          getExpectedVolumeMounts(),
			wantVolumes:               getExpectedVolumes(),
			want:                      assertAll,
		},
		{
			name:                           "One cred from existing secret, token specified",
			singleContainerStrategyEnabled: false,
			dda: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				testutils.NewDatadogAgentBuilder().
					WithCredentialsFromSecret("apiKeyName", "apiKeyKey", "", "").
					WithDCAToken("token").
					BuildWithDefaults(),
			),
			wantCoreAgentEnvVars: nil,
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name: constants.DDAPIKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "apiKeyName",
							},
							Key: "apiKeyKey",
						},
					},
				},
				{
					Name: DDClusterAgentAuthToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "datadog-token",
							},
							Key: common.DefaultTokenKey,
						},
					},
				},
			}...),
			wantCoreAgentVolumeMounts: getExpectedVolumeMounts(),
			wantVolumeMounts:          getExpectedVolumeMounts(),
			wantVolumes:               getExpectedVolumes(),
			want:                      assertAll,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podTemplateManager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
			store := store.NewStore(tt.dda, storeOptions)
			resourcesManager := feature.NewResourceManagers(store)
			reqComp := feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)}
			requiredComponents := feature.RequiredComponents{
				ClusterAgent: reqComp,
				Agent:        reqComp,
			}
			ApplyGlobalComponentDependencies(logger, tt.dda.GetObjectMeta(), &tt.dda.Spec, &tt.dda.Status, resourcesManager, v2alpha1.NodeAgentComponentName, reqComp, false)
			ApplyGlobalSettingsNodeAgent(logger, podTemplateManager, tt.dda.GetObjectMeta(), &tt.dda.Spec, resourcesManager, tt.singleContainerStrategyEnabled, requiredComponents)

			tt.want(t, podTemplateManager, tt.wantCoreAgentEnvVars, tt.wantEnvVars, tt.wantVolumes, tt.wantCoreAgentVolumeMounts, tt.wantVolumeMounts)
			// Assert dependencies if and only if a dependency is expected
			if tt.wantDependency != nil {
				tt.wantDependency(t, resourcesManager)
			}
		})
	}
}

func assertAll(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedCoreAgentEnvVars, expectedEnvVars []*corev1.EnvVar, expectedVolumes []*corev1.Volume, expectedCoreAgentVolumeMounts, expectedVolumeMounts []*corev1.VolumeMount) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
	traceAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.TraceAgentContainerName]
	processAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ProcessAgentContainerName]

	assert.ElementsMatch(t, coreAgentVolumeMounts, expectedCoreAgentVolumeMounts, "core-agent volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, expectedCoreAgentVolumeMounts))
	assert.ElementsMatch(t, traceAgentVolumeMounts, expectedVolumeMounts, "trace-agent volume mounts \ndiff = %s", cmp.Diff(traceAgentVolumeMounts, expectedVolumeMounts))
	assert.ElementsMatch(t, processAgentVolumeMounts, expectedVolumeMounts, "process-agent volume mounts \ndiff = %s", cmp.Diff(processAgentVolumeMounts, expectedVolumeMounts))

	volumes := mgr.VolumeMgr.Volumes
	assert.ElementsMatch(t, volumes, expectedVolumes, "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))

	coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
	assert.ElementsMatch(t, coreAgentEnvVars, expectedCoreAgentEnvVars, "core-agent envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, expectedCoreAgentEnvVars))

	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.AllContainers]
	assert.ElementsMatch(t, agentEnvVars, expectedEnvVars, "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedEnvVars))
}

func assertAllAgentSingleContainer(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedCoreAgentEnvVars, expectedEnvVars []*corev1.EnvVar, expectedVolumes []*corev1.Volume, expectedCoreAgentVolumeMounts, expectedVolumeMounts []*corev1.VolumeMount) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	agentSingleContainerVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.UnprivilegedSingleAgentContainerName]

	assert.True(t, apiutils.IsEqualStruct(agentSingleContainerVolumeMounts, expectedCoreAgentVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(agentSingleContainerVolumeMounts, expectedCoreAgentVolumeMounts))

	volumes := mgr.VolumeMgr.Volumes
	assert.True(t, apiutils.IsEqualStruct(volumes, expectedVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))

	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedEnvVars))
}

func getExpectedEnvVars(addedEnvVars ...*corev1.EnvVar) []*corev1.EnvVar {
	defaultEnvVars := []*corev1.EnvVar{
		{
			Name:  constants.DDSite,
			Value: "datadoghq.com",
		},
		{
			Name:  constants.DDLogLevel,
			Value: "info",
		},
	}

	return append(defaultEnvVars, addedEnvVars...)
}

type volumeConfig string

const kubeletCAVolumes volumeConfig = "kubeletCA"
const criSocketVolume volumeConfig = "criSocket"

func getExpectedVolumes(configs ...volumeConfig) []*corev1.Volume {
	volumes := []*corev1.Volume{}

	// Order is important for the comparisons in the assertion, so respect that
	if slices.Contains(configs, kubeletCAVolumes) {
		volumes = append(volumes, &corev1.Volume{
			Name: kubeletCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostCAPath,
				},
			},
		})
	}

	if slices.Contains(configs, criSocketVolume) {
		volumes = append(volumes, &corev1.Volume{
			Name: common.CriSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: dockerSocketPath,
				},
			},
		})
	}

	return volumes
}

func getExpectedVolumeMounts(configs ...volumeConfig) []*corev1.VolumeMount {
	mounts := []*corev1.VolumeMount{}

	if slices.Contains(configs, kubeletCAVolumes) {
		mounts = append(mounts, &corev1.VolumeMount{
			Name:      kubeletCAVolumeName,
			MountPath: agentCAPath,
			ReadOnly:  true,
		})
	}

	if slices.Contains(configs, criSocketVolume) {
		mounts = append(mounts, &corev1.VolumeMount{
			Name:      common.CriSocketVolumeName,
			MountPath: "/host" + dockerSocketPath,
			ReadOnly:  true,
		})
	}

	return mounts
}

func addNameNamespaceToDDA(name string, namespace string, dda *v2alpha1.DatadogAgent) *v2alpha1.DatadogAgent {
	dda.Name = name
	dda.Namespace = namespace
	return dda
}

func assertSecretBackendGlobalPerms(t testing.TB, resourcesManager feature.ResourceManagers) {
	store := resourcesManager.Store()
	// ClusterRole and ClusterRoleBinding use the same name
	expectedName := fmt.Sprintf("%s-%s-%s", ddaNamespace, ddaName, "secret-backend")
	expectedPolicyRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.SecretsResource},
			Verbs:     []string{rbac.GetVerb},
		},
	}
	crObj, found := store.Get(kubernetes.ClusterRolesKind, "", expectedName)
	if !found {
		t.Error("Should have created ClusterRole")
	} else {
		cr := crObj.(*rbacv1.ClusterRole)
		assert.True(
			t,
			apiutils.IsEqualStruct(cr.Rules, expectedPolicyRules),
			"ClusterRole Policy Rules \ndiff = %s", cmp.Diff(cr.Rules, expectedPolicyRules),
		)
	}

	expectedRoleRef := rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     rbac.ClusterRoleKind,
		Name:     expectedName,
	}

	expectedSubject := []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      ddaName + "-" + constants.DefaultAgentResourceSuffix,
			Namespace: ddaNamespace,
		},
	}

	crbObj, found := store.Get(kubernetes.ClusterRoleBindingKind, "", expectedName)
	if !found {
		t.Error("Should have created ClusterRoleBinding")
	} else {
		crb := crbObj.(*rbacv1.ClusterRoleBinding)
		// Validate ClusterRoleBinding roleRef name
		assert.True(
			t,
			apiutils.IsEqualStruct(crb.RoleRef, expectedRoleRef),
			"ClusterRoleBinding Role Ref \ndiff = %s", cmp.Diff(crb.RoleRef, expectedRoleRef),
		)
		// Validate ClusterRoleBinding subject
		assert.True(
			t,
			apiutils.IsEqualStruct(crb.Subjects, expectedSubject),
			"ClusterRoleBinding Subject \ndiff = %s", cmp.Diff(crb.Subjects, expectedSubject),
		)
	}
}

func assertSecretBackendSpecificPerms(t testing.TB, resourcesManager feature.ResourceManagers) {
	store := resourcesManager.Store()

	// Role and RoleBinding use the same name
	expectedName := fmt.Sprintf("%s-%s-%s", secretNamespace, ddaName, "secret-backend")
	expectedPolicyRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{rbac.CoreAPIGroup},
			Resources:     []string{rbac.SecretsResource},
			ResourceNames: secretNames,
			Verbs:         []string{rbac.GetVerb},
		},
	}
	rObj, found := store.Get(kubernetes.RolesKind, secretNamespace, expectedName)
	if !found {
		t.Error("Should have created Role")
	} else {
		r := rObj.(*rbacv1.Role)
		assert.True(
			t,
			apiutils.IsEqualStruct(r.Rules, expectedPolicyRules),
			"Role Policy Rules \ndiff = %s", cmp.Diff(r.Rules, expectedPolicyRules),
		)
	}

	expectedRoleRef := rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     rbac.RoleKind,
		Name:     expectedName,
	}

	expectedSubject := []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      ddaName + "-" + constants.DefaultAgentResourceSuffix,
			Namespace: ddaNamespace,
		},
	}

	rbObj, found := store.Get(kubernetes.RoleBindingKind, secretNamespace, expectedName)
	if !found {
		t.Error("Should have created RoleBinding")
	} else {
		rb := rbObj.(*rbacv1.RoleBinding)
		// Validate RoleBinding roleRef name
		assert.True(
			t,
			apiutils.IsEqualStruct(rb.RoleRef, expectedRoleRef),
			"RoleBinding Role Ref \ndiff = %s", cmp.Diff(rb.RoleRef, expectedRoleRef),
		)
		// Validate RoleBinding subject
		assert.True(
			t,
			apiutils.IsEqualStruct(rb.Subjects, expectedSubject),
			"RoleBinding Subject \ndiff = %s", cmp.Diff(rb.Subjects, expectedSubject),
		)
	}
}

func Test_UseFIPSAgent(t *testing.T) {
	logger := logf.Log.WithName("Test_UseFIPSAgent")

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	noFipsAgentImage := "gcr.io/datadoghq/agent:7.64.0"

	initContainer := &corev1.Container{
		Name:  string(apicommon.CoreAgentContainerName),
		Image: noFipsAgentImage,
	}
	agentContainer := &corev1.Container{
		Name:  string(apicommon.CoreAgentContainerName),
		Image: noFipsAgentImage,
	}
	processAgentContainer := &corev1.Container{
		Name:  string(apicommon.ProcessAgentContainerName),
		Image: noFipsAgentImage,
	}
	systemProbeContainer := &corev1.Container{
		Name:  string(apicommon.SystemProbeContainerName),
		Image: noFipsAgentImage,
	}

	dda := testutils.NewDatadogAgentBuilder().
		WithCredentials("api-key", "app-key").
		WithUseFIPSAgent().
		BuildWithDefaults()

	store := store.NewStore(dda, storeOptions)

	existingManager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{*initContainer},
			Containers:     []corev1.Container{*agentContainer, *processAgentContainer, *systemProbeContainer},
		},
	})

	reqComp := feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)}
	requiredComponents := feature.RequiredComponents{
		ClusterAgent: reqComp,
		Agent:        reqComp,
	}

	t.Run("test", func(t *testing.T) {
		resourcesManager := feature.NewResourceManagers(store)

		applyGlobalSettings(logger, existingManager, dda.GetObjectMeta(), &dda.Spec, resourcesManager, requiredComponents)

		checkFIPSImages(t, existingManager)
	})
}

func checkFIPSImages(t testing.TB, mgr *fake.PodTemplateManagers) {
	for _, container := range mgr.PodTemplateSpec().Spec.Containers {
		assert.True(t, strings.HasSuffix(container.Image, "-fips"), "Container %s has image %s", container.Name, container.Image)
	}
	for _, container := range mgr.PodTemplateSpec().Spec.InitContainers {
		assert.True(t, strings.HasSuffix(container.Image, "-fips"), "Container %s has image %s", container.Name, container.Image)
	}
}
