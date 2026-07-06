package hostprofiler

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
)

var errHostPIDDisabledManually = errors.New("Host PID is required for host profiler")

type hostProfilerFeature struct {
	owner                   metav1.Object
	hostPIDDisabledManually bool
	seccompEnabled          bool

	logger logr.Logger
}

func init() {
	err := feature.Register(feature.HostProfilerIDType, buildHostProfilerFeature)
	if err != nil {
		panic(err)
	}
}

func buildHostProfilerFeature(options *feature.Options) feature.Feature {
	hostProfilerFeat := &hostProfilerFeature{}

	if options != nil {
		hostProfilerFeat.logger = options.Logger
	}

	return hostProfilerFeat
}

func (o *hostProfilerFeature) ID() feature.IDType {
	return feature.HostProfilerIDType
}

func (o *hostProfilerFeature) Configure(dda metav1.Object, _ *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	o.owner = dda

	if !featureutils.HasFeatureEnableAnnotation(dda, featureutils.EnableHostProfilerAnnotation) {
		return feature.RequiredComponents{}
	}

	// Seccomp profile is enabled by default; opt out via the seccomp annotation set to "false".
	o.seccompEnabled = true
	if str, ok := dda.GetAnnotations()[featureutils.EnableHostProfilerSeccompAnnotation]; ok {
		value, err := strconv.ParseBool(str)
		if err == nil {
			o.seccompEnabled = value
		} else {
			o.logger.Info("host profiler: invalid seccomp annotation value, defaulting to enabled", "value", str)
		}
	}

	return feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			IsRequired: ptr.To(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.CoreAgentContainerName,
				apicommon.HostProfiler,
			},
		},
	}
}

func (o *hostProfilerFeature) ManageDependencies(managers feature.ResourceManagers) error {
	if o.hostPIDDisabledManually {
		return errHostPIDDisabledManually
	}
	return nil
}

func (o *hostProfilerFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (o *hostProfilerFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	if o.hostPIDDisabledManually {
		return errHostPIDDisabledManually
	}

	// Host PID
	managers.PodTemplateSpec().Spec.HostPID = true

	// Security context: drop all caps, add only what host-profiler needs, lock down privilege escalation,
	// and apply a localhost seccomp profile. AllowPrivilegeEscalation must be explicitly false so that
	// runc applies the seccomp filter before its own setuid/setgid/capset calls during container setup.
	var hostProfilerContainer *corev1.Container
	for i := range managers.PodTemplateSpec().Spec.Containers {
		if managers.PodTemplateSpec().Spec.Containers[i].Name == string(apicommon.HostProfiler) {
			hostProfilerContainer = &managers.PodTemplateSpec().Spec.Containers[i]
			break
		}
	}

	if hostProfilerContainer == nil {
		return fmt.Errorf("host-profiler container not found in pod template spec")
	}

	if hostProfilerContainer.SecurityContext == nil {
		hostProfilerContainer.SecurityContext = &corev1.SecurityContext{}
	}

	// Experimental image overrides are applied after ManageNodeAgent, so mirror their image
	// resolution here to keep the seccomp profile and init container aligned with the final
	// host-profiler container image.
	hostProfilerImage := resolveHostProfilerImage(o.owner, hostProfilerContainer.Image)

	sc := hostProfilerContainer.SecurityContext
	sc.AllowPrivilegeEscalation = ptr.To(false)
	sc.Capabilities = &corev1.Capabilities{
		Drop: []corev1.Capability{"ALL"},
		Add:  defaultCapabilities(),
	}

	// Seccomp profile and its setup init container are gated on the seccomp annotation (default enabled).
	// When disabled, the container runs Unconfined and the init container that installs the profile on
	// the node is omitted.
	if o.seccompEnabled {
		sc.SeccompProfile = &corev1.SeccompProfile{
			Type:             corev1.SeccompProfileTypeLocalhost,
			LocalhostProfile: ptr.To(seccompProfileName(hostProfilerImage)),
		}

		// seccomp-root EmptyDir volume (shared with system-probe when both are enabled; VolumeManager deduplicates)
		seccompRootVol := common.GetVolumeForSeccomp()
		managers.Volume().AddVolume(&seccompRootVol)

		// Init container: copy seccomp profile JSON to the kubelet seccomp directory on the host.
		// Appended after the base init containers (init-volume, init-config) added by default.go.
		initContainer := buildSeccompSetupInitContainer(hostProfilerImage)
		managers.PodTemplateSpec().Spec.InitContainers = append(managers.PodTemplateSpec().Spec.InitContainers, initContainer)
	} else {
		sc.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeUnconfined,
		}
	}

	// AppArmor: unconfined so the default containerd profile doesn't block ptrace cross-profile,
	// which host-profiler requires to read /proc/<pid>/map_files for process profiling.
	managers.Annotation().AddAnnotation(common.AppArmorAnnotationKey+"/"+string(apicommon.HostProfiler), "unconfined")

	// Tracingfs volume
	volumeTracingfs := corev1.Volume{
		Name: "tracingfs",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/sys/kernel/tracing",
			},
		},
	}
	managers.Volume().AddVolume(&volumeTracingfs)

	tracingfsMount := corev1.VolumeMount{
		Name:      "tracingfs",
		MountPath: "/sys/kernel/tracing",
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&tracingfsMount, apicommon.HostProfiler)

	// (todo: mackjmr): remove this once IPC port is enabled by default. Enabling this port is required to fetch the API key from
	// core agent when secrets backend is used.
	agentIpcPortEnvVar := &corev1.EnvVar{
		Name:  common.DDAgentIpcPort,
		Value: "5009",
	}
	agentIpcConfigRefreshIntervalEnvVar := &corev1.EnvVar{
		Name:  common.DDAgentIpcConfigRefreshInterval,
		Value: "60",
	}
	for _, container := range []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.HostProfiler} {
		managers.EnvVar().AddEnvVarToContainer(container, agentIpcPortEnvVar)
		managers.EnvVar().AddEnvVarToContainer(container, agentIpcConfigRefreshIntervalEnvVar)
	}

	return nil
}

func (o *hostProfilerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (o *hostProfilerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}

func (o *hostProfilerFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers) error {
	return nil
}

// resolveHostProfilerImage returns the host-profiler image to use for the seccomp init container
// and profile name. It uses the same experimental image override semantics as the final pod
// mutation so the seccomp init image cannot drift from the host-profiler container image.
func resolveHostProfilerImage(dda metav1.Object, baseImage string) string {
	hostProfilerImage, err := experimental.ResolveImageOverride(dda, string(apicommon.HostProfiler), baseImage)
	if err != nil {
		return baseImage
	}
	return hostProfilerImage
}
