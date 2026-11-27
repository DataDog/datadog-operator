// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

const (
	// orderedShutdownAgentBudgetSeconds is the part of the grace period reserved
	// for the other containers (mainly the core agent's final flush) after
	// system-probe has exited.
	orderedShutdownAgentBudgetSeconds int64 = 15
	// orderedShutdownSystemProbeBudgetSeconds is the longest the other
	// containers wait for system-probe to exit before they receive SIGTERM.
	orderedShutdownSystemProbeBudgetSeconds int64 = 60
	// defaultTerminationGracePeriodSeconds is the Kubernetes default, used when
	// the pod template doesn't set one.
	defaultTerminationGracePeriodSeconds int64 = 30
)

// IsOrderedShutdownEnabled reports whether the DDA/DDAI opted in to ordered
// shutdown of the node Agent pod with the
// experimental.agent.datadoghq.com/ordered-shutdown annotation.
func IsOrderedShutdownEnabled(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	return strings.EqualFold(getExperimentalAnnotation(obj, ExperimentalOrderedShutdownSubkey), "true")
}

// ApplyOrderedShutdown makes system-probe stop before every other container of
// the node Agent pod, so a slow system-probe teardown doesn't lengthen the
// collection gap of the other containers.
//
// system-probe takes an exclusive flock on a file in the shared
// sysprobe-socket-dir volume for its whole lifetime. Every other container gets
// a preStop hook that waits for a shared lock on the same file, which it only
// gets once system-probe has exited, so the kubelet holds back their SIGTERM
// until then (or until the wait budget runs out). Every failure mode fails open
// to the default, unordered shutdown.
//
// It is a no-op unless the pod template has a system-probe container that still
// runs the operator default command. It must run after the features and the
// user overrides so it sees the final container list and commands.
func ApplyOrderedShutdown(logger logr.Logger, manager feature.PodTemplateManagers) {
	logger = logger.WithName("OrderedShutdown")
	podSpec := &manager.PodTemplateSpec().Spec

	spIdx := slices.IndexFunc(podSpec.Containers, func(c corev1.Container) bool {
		return c.Name == string(apicommon.SystemProbeContainerName)
	})
	if spIdx < 0 {
		logger.V(1).Info("Ordered shutdown not applied: no system-probe container")
		return
	}
	sp := &podSpec.Containers[spIdx]
	if !slices.Equal(sp.Command, common.DefaultSystemProbeCommand()) {
		logger.V(1).Info("Ordered shutdown not applied: system-probe command is overridden", "command", sp.Command)
		return
	}
	// The wrapper runs through `sh -c`, which would treat any args as its own
	// positional parameters and drop them, so leave a container with custom args alone.
	if len(sp.Args) > 0 {
		logger.V(1).Info("Ordered shutdown not applied: system-probe args are overridden", "args", sp.Args)
		return
	}

	// system-probe must be able to create the lock file, or the wrapper's
	// `exec 9>` makes the shell exit and system-probe never starts. Every
	// system-probe feature mounts sysprobe-socket-dir read-write on it; anything
	// else at that path means a user override, so fail open.
	if m, ok := mountAtPath(sp, common.SystemProbeSocketVolumePath); ok && (m.Name != common.SystemProbeSocketVolumeName || m.ReadOnly) {
		logger.V(1).Info("Ordered shutdown not applied: system-probe doesn't mount the socket volume read-write",
			"path", common.SystemProbeSocketVolumePath, "volume", m.Name, "readOnly", m.ReadOnly)
		return
	}

	sp.Command = orderedShutdownSystemProbeCommand()

	// Every system-probe feature already adds the volume and the read-write
	// mount on system-probe. Add them defensively, keeping any existing ones.
	vol, rwMount := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, false)
	_ = manager.Volume().AddVolumeWithMergeFunc(&vol, merger.IgnoreNewVolumeMergeFunction)
	_ = manager.VolumeMount().AddVolumeMountToContainerWithMergeFunc(&rwMount, apicommon.SystemProbeContainerName, merger.IgnoreNewVolumeMountMergeFunction)
	_, roMount := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, true)

	grace := max(defaultTerminationGracePeriodSeconds, orderedShutdownSystemProbeBudgetSeconds+orderedShutdownAgentBudgetSeconds)
	if podSpec.TerminationGracePeriodSeconds != nil {
		grace = max(*podSpec.TerminationGracePeriodSeconds, orderedShutdownSystemProbeBudgetSeconds+orderedShutdownAgentBudgetSeconds)
	}
	podSpec.TerminationGracePeriodSeconds = &grace
	wait := min(orderedShutdownSystemProbeBudgetSeconds, grace-orderedShutdownAgentBudgetSeconds)

	for i := range podSpec.Containers {
		c := &podSpec.Containers[i]
		if c.Name == string(apicommon.SystemProbeContainerName) {
			continue
		}
		// Only adds the mount when the container doesn't already mount the
		// volume (or something else at the same path).
		_ = manager.VolumeMount().AddVolumeMountToContainerWithMergeFunc(&roMount, apicommon.AgentContainerName(c.Name), merger.IgnoreNewVolumeMountMergeFunction)

		if c.Lifecycle != nil && c.Lifecycle.PreStop != nil {
			logger.V(1).Info("Keeping existing preStop hook; container won't wait for system-probe", "container", c.Name)
			continue
		}
		if c.Lifecycle == nil {
			c.Lifecycle = &corev1.Lifecycle{}
		}
		c.Lifecycle.PreStop = &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{Command: orderedShutdownPreStopCommand(wait)},
		}
	}
}

func mountAtPath(c *corev1.Container, path string) (corev1.VolumeMount, bool) {
	for _, m := range c.VolumeMounts {
		if m.MountPath == path {
			return m, true
		}
	}
	return corev1.VolumeMount{}, false
}

// orderedShutdownSystemProbeCommand wraps the default system-probe command so
// it holds an exclusive lock on the lifetime lock file until it exits. The
// shell execs system-probe, so system-probe stays PID 1 and inherits fd 9.
func orderedShutdownSystemProbeCommand() []string {
	return []string{
		"sh", "-c",
		fmt.Sprintf("exec 9>%s && flock -x 9 && exec %s",
			common.SystemProbeLifetimeLockPath,
			strings.Join(common.DefaultSystemProbeCommand(), " ")),
	}
}

// orderedShutdownPreStopCommand waits up to waitSeconds for system-probe to
// release its lifetime lock. Any flock error (missing file, timeout) is
// ignored so SIGTERM is never blocked longer than the budget.
func orderedShutdownPreStopCommand(waitSeconds int64) []string {
	return []string{
		"sh", "-c",
		fmt.Sprintf("flock -s -w %d %s true || true", waitSeconds, common.SystemProbeLifetimeLockPath),
	}
}
