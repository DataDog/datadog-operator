package hostprofiler

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

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
	tests := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{
			name:        "no annotations",
			annotations: nil,
			want:        "",
		},
		{
			name:        "annotation absent",
			annotations: map[string]string{"some.other/annotation": "value"},
			want:        "",
		},
		{
			name: "host-profiler override present",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"host-profiler":{"name":"gcr.io/x/host-profiler:v2"}}`,
			},
			want: "gcr.io/x/host-profiler:v2",
		},
		{
			name: "override for different container",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"agent":{"name":"gcr.io/x/agent:v2"}}`,
			},
			want: "",
		},
		{
			name: "name without tag, tag field set",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"host-profiler":{"name":"gcr.io/x/host-profiler","tag":"v2"}}`,
			},
			want: "gcr.io/x/host-profiler:v2",
		},
		{
			name: "name with tag, tag field also set — name wins",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `{"host-profiler":{"name":"gcr.io/x/host-profiler:v1","tag":"v2"}}`,
			},
			want: "gcr.io/x/host-profiler:v1",
		},
		{
			name: "malformed json",
			annotations: map[string]string{
				"experimental.agent.datadoghq.com/image-override-config": `not-json`,
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &metav1.ObjectMeta{Annotations: tt.annotations}
			assert.Equal(t, tt.want, resolveHostProfilerImage(dda))
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
