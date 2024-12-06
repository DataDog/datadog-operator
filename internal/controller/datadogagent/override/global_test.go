// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"

	apicommon "github.com/DataDog/datadog-operator/api/crds/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	hostCAPath           = "/host/ca/path/ca.crt"
	agentCAPath          = "/agent/ca/path/ca.crt"
	dockerSocketPath     = "/docker/socket/path/docker.sock"
	secretBackendCommand = "foo.sh"
	secretBackendArgs    = "bar baz"
	secretBackendTimeout = 60
	ddaName              = "datadog"
	ddaNamespace         = "system"
	secretNamespace      = "postgres"
)

var secretNames = []string{"db-username", "db-password"}

func TestNodeAgentComponenGlobalSettings(t *testing.T) {
	logger := logf.Log.WithName("TestRequiredComponents")

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	var emptyVolumeMounts []*corev1.VolumeMount
	emptyVolumes := []*corev1.Volume{}

	tests := []struct {
		name                           string
		dda                            *v2alpha1.DatadogAgent
		singleContainerStrategyEnabled bool
		wantVolumeMounts               []*corev1.VolumeMount
		wantVolumes                    []*corev1.Volume
		wantEnvVars                    []*corev1.EnvVar
		want                           func(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedEnvVars []*corev1.EnvVar, expectedVolumes []*corev1.Volume, expectedVolumeMounts []*corev1.VolumeMount)
		wantDependency                 func(t testing.TB, resourcesManager feature.ResourceManagers)
	}{
		{
			name:                           "Kubelet volume configured",
			singleContainerStrategyEnabled: false,
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithGlobalKubeletConfig(hostCAPath, agentCAPath, true).
				WithGlobalDockerSocketPath(dockerSocketPath).
				BuildWithDefaults(),
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  apicommon.DDKubeletTLSVerify,
					Value: "true",
				},
				{
					Name:  apicommon.DDKubeletCAPath,
					Value: agentCAPath,
				},
				{
					Name:  apicommon.DockerHost,
					Value: "unix:///host" + dockerSocketPath,
				},
			}...),
			wantVolumeMounts: getExpectedVolumeMounts(),
			wantVolumes:      getExpectedVolumes(),
			want:             assertAll,
		},
		{
			name:                           "Kubelet volume configured",
			singleContainerStrategyEnabled: true,
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithGlobalKubeletConfig(hostCAPath, agentCAPath, true).
				WithGlobalDockerSocketPath(dockerSocketPath).
				BuildWithDefaults(),
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  apicommon.DDKubeletTLSVerify,
					Value: "true",
				},
				{
					Name:  apicommon.DDKubeletCAPath,
					Value: agentCAPath,
				},
				{
					Name:  apicommon.DockerHost,
					Value: "unix:///host" + dockerSocketPath,
				},
			}...),
			wantVolumeMounts: getExpectedVolumeMounts(),
			wantVolumes:      getExpectedVolumes(),
			want:             assertAllAgentSingleContainer,
		},
		{
			name:                           "Checks tag cardinality set to orchestrator",
			singleContainerStrategyEnabled: false,
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithChecksTagCardinality("orchestrator").
				BuildWithDefaults(),
			wantEnvVars: getExpectedEnvVars(&corev1.EnvVar{
				Name:  apicommon.DDChecksTagCardinality,
				Value: "orchestrator",
			}),
			wantVolumeMounts: emptyVolumeMounts,
			wantVolumes:      emptyVolumes,
			want:             assertAll,
		},
		{
			name:                           "Unified origin detection activated",
			singleContainerStrategyEnabled: false,
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithOriginDetectionUnified(true).
				BuildWithDefaults(),
			wantEnvVars: getExpectedEnvVars(&corev1.EnvVar{
				Name:  apicommon.DDOriginDetectionUnified,
				Value: "true",
			}),
			wantVolumeMounts: emptyVolumeMounts,
			wantVolumes:      emptyVolumes,
			want:             assertAll,
		},
		{
			name:                           "Global environment variable configured",
			singleContainerStrategyEnabled: false,
			dda: v2alpha1test.NewDatadogAgentBuilder().
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
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  "envA",
					Value: "valueA",
				},
				{
					Name:  "envB",
					Value: "valueB",
				},
			}...),
			wantVolumeMounts: emptyVolumeMounts,
			wantVolumes:      emptyVolumes,
			want:             assertAll,
		},
		{
			name:                           "Secret backend - global permissions",
			singleContainerStrategyEnabled: false,
			dda: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				v2alpha1test.NewDatadogAgentBuilder().
					WithGlobalSecretBackendGlobalPerms(secretBackendCommand, secretBackendArgs, secretBackendTimeout).
					BuildWithDefaults(),
			),
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  apicommon.DDSecretBackendCommand,
					Value: secretBackendCommand,
				},
				{
					Name:  apicommon.DDSecretBackendArguments,
					Value: secretBackendArgs,
				},
				{
					Name:  apicommon.DDSecretBackendTimeout,
					Value: "60",
				},
			}...),
			wantVolumeMounts: emptyVolumeMounts,
			wantVolumes:      emptyVolumes,
			want:             assertAll,
			wantDependency:   assertSecretBackendGlobalPerms,
		},
		{
			name:                           "Secret backend - specific secret permissions",
			singleContainerStrategyEnabled: false,
			dda: addNameNamespaceToDDA(
				ddaName,
				ddaNamespace,
				v2alpha1test.NewDatadogAgentBuilder().
					WithGlobalSecretBackendSpecificRoles(secretBackendCommand, secretBackendArgs, secretBackendTimeout, secretNamespace, secretNames).
					BuildWithDefaults(),
			),
			wantEnvVars: getExpectedEnvVars([]*corev1.EnvVar{
				{
					Name:  apicommon.DDSecretBackendCommand,
					Value: secretBackendCommand,
				},
				{
					Name:  apicommon.DDSecretBackendArguments,
					Value: secretBackendArgs,
				},
				{
					Name:  apicommon.DDSecretBackendTimeout,
					Value: "60",
				},
			}...),
			wantVolumeMounts: emptyVolumeMounts,
			wantVolumes:      emptyVolumes,
			want:             assertAll,
			wantDependency:   assertSecretBackendSpecificPerms,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podTemplateManager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
			store := store.NewStore(tt.dda, storeOptions)
			resourcesManager := feature.NewResourceManagers(store)

			ApplyGlobalSettingsNodeAgent(logger, podTemplateManager, tt.dda, resourcesManager, tt.singleContainerStrategyEnabled)

			tt.want(t, podTemplateManager, tt.wantEnvVars, tt.wantVolumes, tt.wantVolumeMounts)
			// Assert dependencies if and only if a dependency is expected
			if tt.wantDependency != nil {
				tt.wantDependency(t, resourcesManager)
			}
		})
	}
}

func assertAll(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedEnvVars []*corev1.EnvVar, expectedVolumes []*corev1.Volume, expectedVolumeMounts []*corev1.VolumeMount) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
	traceAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.TraceAgentContainerName]
	processAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ProcessAgentContainerName]

	assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, expectedVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
	assert.True(t, apiutils.IsEqualStruct(traceAgentVolumeMounts, expectedVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(traceAgentVolumeMounts, []*corev1.VolumeMount(nil)))
	assert.True(t, apiutils.IsEqualStruct(processAgentVolumeMounts, expectedVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(processAgentVolumeMounts, []*corev1.VolumeMount(nil)))

	volumes := mgr.VolumeMgr.Volumes
	assert.True(t, apiutils.IsEqualStruct(volumes, expectedVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))

	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedEnvVars))
}

func assertAllAgentSingleContainer(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedEnvVars []*corev1.EnvVar, expectedVolumes []*corev1.Volume, expectedVolumeMounts []*corev1.VolumeMount) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	agentSingleContainerVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.UnprivilegedSingleAgentContainerName]

	assert.True(t, apiutils.IsEqualStruct(agentSingleContainerVolumeMounts, expectedVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(agentSingleContainerVolumeMounts, []*corev1.VolumeMount(nil)))

	volumes := mgr.VolumeMgr.Volumes
	assert.True(t, apiutils.IsEqualStruct(volumes, expectedVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))

	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedEnvVars))
}

func getExpectedEnvVars(addedEnvVars ...*corev1.EnvVar) []*corev1.EnvVar {
	defaultEnvVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDSite,
			Value: "datadoghq.com",
		},
		{
			Name:  apicommon.DDLogLevel,
			Value: "info",
		},
	}

	return append(defaultEnvVars, addedEnvVars...)
}

func getExpectedVolumes() []*corev1.Volume {
	return []*corev1.Volume{
		{
			Name: apicommon.KubeletCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostCAPath,
				},
			},
		},
		{
			Name: apicommon.CriSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: dockerSocketPath,
				},
			},
		},
	}
}

func getExpectedVolumeMounts() []*corev1.VolumeMount {
	return []*corev1.VolumeMount{
		{
			Name:      apicommon.KubeletCAVolumeName,
			MountPath: agentCAPath,
			ReadOnly:  true,
		},
		{
			Name:      apicommon.CriSocketVolumeName,
			MountPath: "/host" + dockerSocketPath,
			ReadOnly:  true,
		},
	}
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
			Name:      ddaName + "-" + v2alpha1.DefaultAgentResourceSuffix,
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
			Name:      ddaName + "-" + v2alpha1.DefaultAgentResourceSuffix,
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
