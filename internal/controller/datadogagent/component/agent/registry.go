// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

// podBuild is the context shared by the container specs of a node Agent pod.
type podBuild struct {
	dda metav1.Object
	// containers are the requested node Agent containers, in request order.
	containers []apicommon.AgentContainerName
	// coreAgent is true when the core Agent container runs in the pod.
	coreAgent bool
	// coreScaffold is true when a container needs the core Agent pod scaffolding:
	// init-config, the agent image init-volume and the base volume set.
	coreScaffold bool
}

// containerSpec declares how a node Agent container is built and what it brings to the pod.
type containerSpec struct {
	build func(b *podBuild) corev1.Container
	// initContainers are appended after the scaffolding init containers.
	initContainers func(b *podBuild) []corev1.Container
	// volumes are appended after the scaffolding volumes; duplicates are dropped by name.
	volumes func(b *podBuild) []corev1.Volume
	// needsCoreScaffold is false only for containers that can run without the core Agent.
	needsCoreScaffold bool
}

// containerRegistry is a lookup table: pod order comes from podBuild.containers, never from the map.
var containerRegistry = map[apicommon.AgentContainerName]containerSpec{
	apicommon.CoreAgentContainerName:               scaffolded(coreAgentContainer),
	apicommon.UnprivilegedSingleAgentContainerName: scaffolded(agentSingleContainer),
	apicommon.TraceAgentContainerName:              scaffolded(traceAgentContainer),
	apicommon.ProcessAgentContainerName:            scaffolded(processAgentContainer),
	apicommon.SecurityAgentContainerName:           scaffolded(securityAgentContainer),
	apicommon.PrivateActionRunnerContainerName:     scaffolded(privateActionRunnerContainer),
	apicommon.AgentDataPlaneContainerName:          scaffolded(agentDataPlaneContainer),
	apicommon.FlightRecorderContainerName:          scaffolded(flightRecorderContainer),
	apicommon.AgentCheckRunnerContainerName:        scaffolded(agentCheckRunnerContainer),
	apicommon.SystemProbeContainerName: {
		build: func(b *podBuild) corev1.Container { return systemProbeContainer(b.dda) },
		initContainers: func(*podBuild) []corev1.Container {
			return []corev1.Container{initSeccompSetupContainer()}
		},
		volumes: func(b *podBuild) []corev1.Volume {
			return []corev1.Volume{common.GetVolumeForSecurity(b.dda), common.GetVolumeForSeccomp()}
		},
		needsCoreScaffold: true,
	},
	apicommon.OtelAgent: {
		build: func(b *podBuild) corev1.Container {
			if b.coreAgent {
				return otelAgentContainer(b.dda)
			}
			return standaloneOtelAgentContainer(b.dda)
		},
		volumes: standaloneVolumes,
	},
	apicommon.HostProfiler: {
		build: func(b *podBuild) corev1.Container {
			if b.coreAgent {
				return hostProfilerContainer(b.dda)
			}
			return standaloneHostProfilerContainer(b.dda)
		},
		volumes: standaloneVolumes,
	},
}

func scaffolded(build func(dda metav1.Object) corev1.Container) containerSpec {
	return containerSpec{
		build:             func(b *podBuild) corev1.Container { return build(b.dda) },
		needsCoreScaffold: true,
	}
}

func newPodBuild(dda metav1.Object, containers []apicommon.AgentContainerName) *podBuild {
	b := &podBuild{
		dda:        dda,
		containers: containers,
		coreAgent:  slices.Contains(containers, apicommon.CoreAgentContainerName),
	}
	for _, name := range containers {
		if spec, ok := containerRegistry[name]; ok && spec.needsCoreScaffold {
			b.coreScaffold = true
		}
	}
	return b
}

// specs returns the registered container specs in pod order: core Agent first, then request order.
func (b *podBuild) specs() []containerSpec {
	var specs []containerSpec
	if b.coreAgent {
		specs = append(specs, containerRegistry[apicommon.CoreAgentContainerName])
	}
	for _, name := range b.containers {
		if spec, ok := containerRegistry[name]; ok && name != apicommon.CoreAgentContainerName {
			specs = append(specs, spec)
		}
	}
	return specs
}

func (b *podBuild) buildContainers() []corev1.Container {
	specs := b.specs()
	containers := make([]corev1.Container, 0, len(specs))
	for _, spec := range specs {
		containers = append(containers, spec.build(b))
	}
	return containers
}

func (b *podBuild) buildInitContainers() []corev1.Container {
	var initContainers []corev1.Container
	if b.coreScaffold {
		initContainers = append(initContainers, initVolumeContainer(agentImage()), initConfigContainer(b.dda))
	} else {
		initContainers = append(initContainers, initVolumeContainer(ddotCollectorImage()))
	}
	for _, spec := range b.specs() {
		if spec.initContainers != nil {
			initContainers = append(initContainers, spec.initContainers(b)...)
		}
	}
	return initContainers
}

func (b *podBuild) buildVolumes() []corev1.Volume {
	var volumes []corev1.Volume
	if b.coreScaffold {
		volumes = scaffoldVolumes(b.dda)
	}
	for _, spec := range b.specs() {
		if spec.volumes == nil {
			continue
		}
		for _, vol := range spec.volumes(b) {
			if !slices.ContainsFunc(volumes, func(v corev1.Volume) bool { return v.Name == vol.Name }) {
				volumes = append(volumes, vol)
			}
		}
	}
	return volumes
}

// scaffoldVolumes are the volumes shared by the core Agent and the containers depending on it.
func scaffoldVolumes(dda metav1.Object) []corev1.Volume {
	return []corev1.Volume{
		common.GetVolumeForLogs(),
		common.GetVolumeForAuth(false),
		common.GetVolumeInstallInfo(dda),
		common.GetVolumeForChecksd(),
		common.GetVolumeForConfd(),
		common.GetVolumeForConfig(),
		common.GetVolumeForProc(),
		common.GetVolumeForCgroups(),
		common.GetVolumeForDogstatsd(),
		common.GetVolumeForRuntimeSocket(),
		common.GetVolumeForRunPath(),
		common.GetVolumeForTmp(),
	}
}

// standaloneVolumes back the mounts of containers able to run without the core Agent.
func standaloneVolumes(*podBuild) []corev1.Volume {
	return []corev1.Volume{
		common.GetVolumeForLogs(),
		common.GetVolumeForConfig(),
		common.GetVolumeForAuth(false),
		common.GetVolumeForTmp(),
		common.GetVolumeForRunPath(),
	}
}

func standaloneOtelAgentContainer(dda metav1.Object) corev1.Container {
	container := otelAgentContainer(dda)
	container.Command = []string{"otel-agent"}
	container.Env = standaloneEnvVars(dda)
	// Without the core Agent, otel-agent owns the auth volume.
	container.VolumeMounts = []corev1.VolumeMount{
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForAuth(false),
		common.GetVolumeMountForTmp(),
		common.GetVolumeMountForRunPath(),
	}
	return container
}

func standaloneHostProfilerContainer(dda metav1.Object) corev1.Container {
	container := hostProfilerContainer(dda)
	container.Command = []string{"host-profiler"}
	container.Env = standaloneEnvVars(dda)
	return container
}

// standaloneEnvVars are the common env vars without the Cluster Agent ones: pods without the
// core Agent run in modes that disable the Cluster Agent.
func standaloneEnvVars(dda metav1.Object) []corev1.EnvVar {
	clusterAgentEnvVars := []string{
		common.DDClusterAgentEnabled,
		common.DDClusterAgentKubeServiceName,
		common.DDClusterAgentTokenName,
	}
	return slices.DeleteFunc(commonEnvVars(dda), func(env corev1.EnvVar) bool {
		return slices.Contains(clusterAgentEnvVars, env.Name)
	})
}
