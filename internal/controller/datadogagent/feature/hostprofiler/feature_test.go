package hostprofiler

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
		{
			Name: "host profiler enabled - dependencies",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("foo").
				WithAnnotations(map[string]string{"agent.datadoghq.com/host-profiler-enabled": "true"}).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDependencies,
		},
	}
	tests.Run(t, buildHostProfilerFeature)
}

func testExpectedDependencies(t testing.TB, storeClient store.StoreClient) {
	obj, found := storeClient.Get(kubernetes.ConfigMapKind, "", "foo-host-profiler-seccomp")
	assert.True(t, found, "host-profiler-seccomp ConfigMap should be created")
	if !found {
		return
	}
	cm, ok := obj.(*corev1.ConfigMap)
	assert.True(t, ok)
	assert.Contains(t, cm.Data, seccompKey, "ConfigMap should contain the seccomp profile key")
	assert.Contains(t, cm.Data[seccompKey], "SCMP_ACT_ERRNO", "seccomp profile should deny by default")
	assert.Contains(t, cm.Data[seccompKey], `"bpf"`, "bpf syscall must be in the profile")
	assert.Contains(t, cm.Data[seccompKey], `"perf_event_open"`, "perf_event_open must be in the profile")
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
					assert.Equal(t, seccompProfileName, *sc.SeccompProfile.LocalhostProfile)
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

				// host-profiler-security ConfigMap volume
				secVolumeFound := false
				for _, v := range mgr.VolumeMgr.Volumes {
					if v.Name == securityVolumeName {
						secVolumeFound = true
						assert.NotNil(t, v.ConfigMap, "host-profiler-security volume should reference a ConfigMap")
					}
				}
				assert.True(t, secVolumeFound, "host-profiler-security volume should be present")

				// seccomp-root volume
				seccompRootFound := false
				for _, v := range mgr.VolumeMgr.Volumes {
					if v.Name == common.SeccompRootVolumeName {
						seccompRootFound = true
					}
				}
				assert.True(t, seccompRootFound, "seccomp-root volume should be present")

				// Init container: host-profiler-seccomp-setup
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
					expectedDst := common.SeccompRootVolumePath + "/" + seccompProfileName
					assert.Contains(t, setupContainer.Command, expectedDst, "cp command should target the kubelet seccomp path")
					mountNames := map[string]bool{}
					for _, m := range setupContainer.VolumeMounts {
						mountNames[m.Name] = true
					}
					assert.True(t, mountNames[securityVolumeName], "init container should mount host-profiler-security")
					assert.True(t, mountNames[common.SeccompRootVolumeName], "init container should mount seccomp-root")
				}
			},
		)
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

func TestDefaultSyscalls(t *testing.T) {
	syscalls := defaultSyscalls()

	syscallSet := make(map[string]bool)
	for _, s := range syscalls {
		syscallSet[s] = true
	}

	assert.NotEmpty(t, syscalls)
	assert.True(t, syscallSet["bpf"], "host-profiler requires bpf syscall")
	assert.True(t, syscallSet["perf_event_open"], "host-profiler requires perf_event_open")
	assert.True(t, syscallSet["openat"], "host-profiler requires openat for /proc/<pid>/map_files access")
	assert.True(t, syscallSet["read"], "host-profiler requires read")
	assert.False(t, syscallSet["ptrace"], "host-profiler should not need ptrace syscall (uses /proc instead)")
}

func TestDefaultSeccompConfigData(t *testing.T) {
	data := defaultSeccompConfigData()

	assert.Contains(t, data, seccompKey, "seccomp configmap should have the host-profiler key")

	profile := data[seccompKey]
	assert.Contains(t, profile, "SCMP_ACT_ERRNO", "default action should be SCMP_ACT_ERRNO")
	assert.Contains(t, profile, "SCMP_ACT_ALLOW", "syscalls should be allowed")
	assert.Contains(t, profile, `"bpf"`, "bpf syscall must be in the profile")
	assert.Contains(t, profile, `"perf_event_open"`, "perf_event_open syscall must be in the profile")
}
