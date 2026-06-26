package hostprofiler

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var (
	tracingfsVolumeMount = corev1.VolumeMount{
		Name:      "tracingfs",
		MountPath: "/sys/kernel/tracing",
		ReadOnly:  true,
	}
	defaultVolumeMounts = []corev1.VolumeMount{tracingfsVolumeMount}
	wantIpcEnvVars      = []*corev1.EnvVar{
		{Name: common.DDAgentIpcPort, Value: "5009"},
		{Name: common.DDAgentIpcConfigRefreshInterval, Value: "60"},
	}
)

func Test_hostProfilerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "host profiler disabled without config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{"agent.datadoghq.com/host-profiler-enabled": "false"}).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "host profiler enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{"agent.datadoghq.com/host-profiler-enabled": "true"}).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.HostProfiler, defaultVolumeMounts),
		},
	}
	tests.Run(t, buildHostProfilerFeature)
}

func Test_hostProfilerFeature_SeccompDisabled(t *testing.T) {
	hostProfilerImage := "gcr.io/datadoghq/agent:7.99.0-fips"
	dda := testutils.NewDatadogAgentBuilder().
		WithName("datadog-agent").
		WithAnnotations(map[string]string{
			"agent.datadoghq.com/host-profiler-enabled":         "true",
			"agent.datadoghq.com/host-profiler-seccomp-enabled": "false",
		}).
		Build()

	manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: string(apicommon.CoreAgentContainerName), Image: images.GetLatestAgentImage()},
				{
					Name:            string(apicommon.HostProfiler),
					Image:           hostProfilerImage,
					SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: ptr.To(true)},
				},
			},
		},
	})

	hostProfilerFeat := buildHostProfilerFeature(nil).(*hostProfilerFeature)
	reqComp := hostProfilerFeat.Configure(dda, &dda.Spec, nil)
	assert.NotNil(t, reqComp.Agent.IsRequired)
	assert.NoError(t, hostProfilerFeat.ManageNodeAgent(manager))

	var hpContainer *corev1.Container
	for i := range manager.Tpl.Spec.Containers {
		if manager.Tpl.Spec.Containers[i].Name == string(apicommon.HostProfiler) {
			hpContainer = &manager.Tpl.Spec.Containers[i]
			break
		}
	}
	assert.NotNil(t, hpContainer)
	// Seccomp profile must NOT be set when disabled, but other hardening stays in place.
	assert.Nil(t, hpContainer.SecurityContext.SeccompProfile, "SeccompProfile must be nil when seccomp is disabled")
	assert.NotNil(t, hpContainer.SecurityContext.AllowPrivilegeEscalation)
	assert.False(t, *hpContainer.SecurityContext.AllowPrivilegeEscalation)
	assert.NotNil(t, hpContainer.SecurityContext.Capabilities)
	assert.True(t, apiutils.IsEqualStruct(hpContainer.SecurityContext.Capabilities.Add, defaultCapabilities()))

	// AppArmor annotation is independent of seccomp and must remain.
	assert.Equal(t, "unconfined", manager.AnnotationMgr.Annotations[common.AppArmorAnnotationKey+"/"+string(apicommon.HostProfiler)])

	// seccomp-root volume must be absent.
	for _, v := range manager.VolumeMgr.Volumes {
		assert.NotEqual(t, common.SeccompRootVolumeName, v.Name, "seccomp-root volume must be absent when seccomp is disabled")
	}

	// seccomp setup init container must be absent.
	for _, c := range manager.Tpl.Spec.InitContainers {
		assert.NotEqual(t, string(apicommon.HostProfilerSeccompSetupContainerName), c.Name, "seccomp setup init container must be absent when seccomp is disabled")
	}
}

func testExpectedAgent(agentContainerName apicommon.AgentContainerName, expectedVolumeMount []corev1.VolumeMount) *test.ComponentTest {
	// Pre-populate both containers so ManageNodeAgent can find and mutate the host-profiler SecurityContext.
	// This mirrors the real flow where default.go's hostProfilerContainer() runs before features.
	hostProfilerImage := "gcr.io/datadoghq/agent:7.99.0-fips"
	hostProfilerPTS := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: string(apicommon.CoreAgentContainerName), Image: images.GetLatestAgentImage()},
				{
					Name:  string(apicommon.HostProfiler),
					Image: hostProfilerImage,
					SecurityContext: &corev1.SecurityContext{
						ReadOnlyRootFilesystem: ptr.To(true),
					},
				},
			},
		},
	}
	return test.NewDefaultComponentTest().
		WithCreateFunc(func(t testing.TB) (feature.PodTemplateManagers, string) {
			return fake.NewPodTemplateManagers(t, hostProfilerPTS), kubernetes.DefaultProvider
		}).
		WithWantFunc(
			func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
				mgr := mgrInterface.(*fake.PodTemplateManagers)

				agentMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
				assert.True(t, apiutils.IsEqualStruct(agentMounts, expectedVolumeMount), "%s volume mounts \ndiff = %s", agentContainerName, cmp.Diff(agentMounts, expectedVolumeMount))

				assert.Equal(t, true, mgr.Tpl.Spec.HostPID)

				// IPC env vars
				coreEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
				assert.True(t, apiutils.IsEqualStruct(coreEnvVars, wantIpcEnvVars), "Core agent IPC env vars \ndiff = %s", cmp.Diff(coreEnvVars, wantIpcEnvVars))

				hostProfilerEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.HostProfiler]
				assert.True(t, apiutils.IsEqualStruct(hostProfilerEnvVars, wantIpcEnvVars), "HostProfiler IPC env vars \ndiff = %s", cmp.Diff(hostProfilerEnvVars, wantIpcEnvVars))

				// Security context: AllowPrivilegeEscalation, SeccompProfile, capabilities (Drop ALL + Add)
				var hpContainer *corev1.Container
				for i := range mgr.Tpl.Spec.Containers {
					if mgr.Tpl.Spec.Containers[i].Name == string(apicommon.HostProfiler) {
						hpContainer = &mgr.Tpl.Spec.Containers[i]
						break
					}
				}
				assert.NotNil(t, hpContainer, "host-profiler container should exist in the pod template")
				if hpContainer != nil && hpContainer.SecurityContext != nil {
					sc := hpContainer.SecurityContext
					assert.NotNil(t, sc.AllowPrivilegeEscalation)
					assert.False(t, *sc.AllowPrivilegeEscalation, "AllowPrivilegeEscalation must be false")
					assert.NotNil(t, sc.SeccompProfile)
					assert.Equal(t, corev1.SeccompProfileTypeLocalhost, sc.SeccompProfile.Type)
					assert.Equal(t, seccompProfileName(hostProfilerImage), *sc.SeccompProfile.LocalhostProfile)
					assert.NotNil(t, sc.Capabilities)
					assert.Contains(t, sc.Capabilities.Drop, corev1.Capability("ALL"))
					assert.True(t, apiutils.IsEqualStruct(sc.Capabilities.Add, defaultCapabilities()), "capabilities.Add \ndiff = %s", cmp.Diff(sc.Capabilities.Add, defaultCapabilities()))
				}

				// AppArmor annotation
				expectedAnnotations := map[string]string{
					common.AppArmorAnnotationKey + "/" + string(apicommon.HostProfiler): "unconfined",
				}
				annotations := mgr.AnnotationMgr.Annotations
				assert.True(t, apiutils.IsEqualStruct(annotations, expectedAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, expectedAnnotations))

				// seccomp-root volume
				seccompRootFound := false
				for _, v := range mgr.VolumeMgr.Volumes {
					if v.Name == common.SeccompRootVolumeName {
						seccompRootFound = true
					}
				}
				assert.True(t, seccompRootFound, "seccomp-root volume should be present")

				// Init container: host-profiler-seccomp-setup copies from the image path
				var setupContainer *corev1.Container
				for i := range mgr.Tpl.Spec.InitContainers {
					if mgr.Tpl.Spec.InitContainers[i].Name == "host-profiler-seccomp-setup" {
						setupContainer = &mgr.Tpl.Spec.InitContainers[i]
						break
					}
				}
				assert.NotNil(t, setupContainer, "host-profiler-seccomp-setup init container should be present")
				if setupContainer != nil {
					assert.Equal(t, hostProfilerImage, setupContainer.Image)
					assert.Contains(t, setupContainer.Command, seccompSourcePath, "cp source should be the in-image seccomp path")
					expectedDst := common.SeccompRootVolumePath + "/" + seccompProfileName(hostProfilerImage)
					assert.Contains(t, setupContainer.Command, expectedDst, "cp command should target the kubelet seccomp path")
					// Init container should only mount seccomp-root, not the ConfigMap volume
					mountNames := map[string]bool{}
					for _, m := range setupContainer.VolumeMounts {
						mountNames[m.Name] = true
					}
					assert.True(t, mountNames[common.SeccompRootVolumeName], "init container should mount seccomp-root")
				}
			},
		)
}

func TestResolveHostProfilerImage(t *testing.T) {
	baseImage := "docker.io/datadog/agent:7.63.0"
	tests := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{
			name: "no annotations",
			want: baseImage,
		},
		{
			name:        "annotation absent",
			annotations: map[string]string{"some.other/annotation": "value"},
			want:        baseImage,
		},
		{
			name: "experimental annotation — full ref in name",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"host-profiler":{"name":"gcr.io/x/host-profiler:v2"}}`,
			},
			want: "gcr.io/x/host-profiler:v2",
		},
		{
			name: "experimental annotation — tagged name preserves registry",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"host-profiler":{"name":"host-profiler:v2"}}`,
			},
			want: "docker.io/datadog/host-profiler:v2",
		},
		{
			name: "experimental annotation — name and tag fields preserve registry",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"host-profiler":{"name":"host-profiler","tag":"v2"}}`,
			},
			want: "docker.io/datadog/host-profiler:v2",
		},
		{
			name: "experimental annotation — name with tag takes precedence over tag field",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"host-profiler":{"name":"host-profiler:v1","tag":"v2"}}`,
			},
			want: "docker.io/datadog/host-profiler:v1",
		},
		{
			name: "experimental annotation — different container, ignored",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"agent":{"name":"gcr.io/x/agent:v2"}}`,
			},
			want: baseImage,
		},
		{
			name: "experimental annotation — malformed json",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `not-json`,
			},
			want: baseImage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &metav1.ObjectMeta{Annotations: tt.annotations}
			assert.Equal(t, tt.want, resolveHostProfilerImage(dda, baseImage))
		})
	}
}

func TestHostProfilerSeccompImageStaysAlignedThroughOverrides(t *testing.T) {
	ifNotPresent := corev1.PullIfNotPresent
	tests := []struct {
		name              string
		annotations       map[string]string
		baseImage         string
		componentOverride *v2alpha1.DatadogAgentComponentOverride
		wantImage         string
	}{
		{
			name:      "node agent image override does not rewrite host-profiler seccomp image",
			baseImage: "gcr.io/datadoghq/agent:7.80.1",
			componentOverride: &v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{Name: "agent", Tag: "nightly", PullPolicy: &ifNotPresent},
			},
			wantImage: "gcr.io/datadoghq/agent:7.80.1",
		},
		// Tag-only experimental overrides are intentionally omitted here: host-profiler must select
		// an image name that contains the profiler, while tag-only would keep the agent image name.
		{
			name: "experimental name and tag preserves registry after node agent override",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"host-profiler":{"name":"host-profiler","tag":"v2"}}`,
			},
			baseImage: "custom.registry/agent:7.80.1-fips",
			componentOverride: &v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{Name: "agent", Tag: "nightly", PullPolicy: &ifNotPresent},
			},
			wantImage: "custom.registry/host-profiler:v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotations := map[string]string{"agent.datadoghq.com/host-profiler-enabled": "true"}
			for k, v := range tt.annotations {
				annotations[k] = v
			}
			dda := testutils.NewDatadogAgentBuilder().
				WithName("datadog-agent").
				WithAnnotations(annotations).
				Build()
			dda.Spec.Override[v2alpha1.NodeAgentComponentName] = tt.componentOverride

			manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: string(apicommon.CoreAgentContainerName), Image: "gcr.io/datadoghq/agent:7.80.1"},
						{
							Name:  string(apicommon.HostProfiler),
							Image: tt.baseImage,
							SecurityContext: &corev1.SecurityContext{
								ReadOnlyRootFilesystem: ptr.To(true),
							},
						},
					},
				},
			})

			hostProfilerFeat := buildHostProfilerFeature(nil).(*hostProfilerFeature)
			hostProfilerFeat.Configure(dda, &dda.Spec, nil)
			assert.NoError(t, hostProfilerFeat.ManageNodeAgent(manager))

			override.PodTemplateSpec(logr.Discard(), manager, tt.componentOverride, v2alpha1.NodeAgentComponentName, dda.Name)
			experimental.ApplyExperimentalOverrides(logr.Discard(), dda, manager)

			var hostProfilerContainer *corev1.Container
			for i := range manager.PodTemplateSpec().Spec.Containers {
				if manager.PodTemplateSpec().Spec.Containers[i].Name == string(apicommon.HostProfiler) {
					hostProfilerContainer = &manager.PodTemplateSpec().Spec.Containers[i]
					break
				}
			}
			assert.NotNil(t, hostProfilerContainer)
			if hostProfilerContainer != nil {
				assert.Equal(t, tt.wantImage, hostProfilerContainer.Image)
				assert.Equal(t, ifNotPresent, hostProfilerContainer.ImagePullPolicy)
				assert.Equal(t, seccompProfileName(tt.wantImage), *hostProfilerContainer.SecurityContext.SeccompProfile.LocalhostProfile)
			}

			var setupContainer *corev1.Container
			for i := range manager.PodTemplateSpec().Spec.InitContainers {
				if manager.PodTemplateSpec().Spec.InitContainers[i].Name == string(apicommon.HostProfilerSeccompSetupContainerName) {
					setupContainer = &manager.PodTemplateSpec().Spec.InitContainers[i]
					break
				}
			}
			assert.NotNil(t, setupContainer)
			if setupContainer != nil {
				assert.Equal(t, tt.wantImage, setupContainer.Image)
				assert.Equal(t, ifNotPresent, setupContainer.ImagePullPolicy)
				assert.Contains(t, setupContainer.Command, common.SeccompRootVolumePath+"/"+seccompProfileName(tt.wantImage))
			}
		})
	}
}

func TestDefaultCapabilities(t *testing.T) {
	caps := defaultCapabilities()

	capSet := make(map[corev1.Capability]bool)
	for _, c := range caps {
		capSet[c] = true
	}

	assert.False(t, capSet["SYS_ADMIN"], "host-profiler should not have SYS_ADMIN")
	assert.True(t, capSet["BPF"], "host-profiler requires BPF for eBPF programs")
	assert.True(t, capSet["PERFMON"], "host-profiler requires PERFMON for perf_event_open")
	assert.True(t, capSet["CHECKPOINT_RESTORE"], "host-profiler requires CHECKPOINT_RESTORE for /proc/<pid>/map_files access")
	assert.True(t, capSet["SYS_PTRACE"], "host-profiler requires SYS_PTRACE for process tracing")
}
