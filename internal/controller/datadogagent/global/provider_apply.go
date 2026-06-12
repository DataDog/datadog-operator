// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the behaviour for the provider-conditional global node-agent
// mutations declared in provider_spec.go: the declarative-spec applier plus the
// imperative helpers for mutations the declarative model can't express (pod
// labels, container args/commands, in-place volume edits).

package global

import (
	"maps"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// ApplyGlobalNodeAgentSpec applies the provider-conditional global mutations to
// the node agent pod template. It must run AFTER ApplyGlobalSettingsNodeAgent so
// that removals (e.g. the CRI socket added from global.criSocketPath) see the
// volumes they target.
func ApplyGlobalNodeAgentSpec(mgr feature.PodTemplateManagers, provider string) {
	providercaps.ApplyNodeAgentProviderCapabilities(mgr, provider, NodeAgentProviderSpec)
	applyProviderPodLabels(mgr, provider)
	if provider == kubernetes.GKEAutopilotProvider {
		applyAutopilotGlobalExtras(mgr)
	}
}

// applyProviderPodLabels merges the provider's pod-template labels (providercaps
// has no label support, so labels are applied here).
func applyProviderPodLabels(mgr feature.PodTemplateManagers, provider string) {
	labels := nodeAgentProviderPodLabels[provider]
	if len(labels) == 0 {
		return
	}
	tmpl := mgr.PodTemplateSpec()
	if tmpl.Labels == nil {
		tmpl.Labels = map[string]string{}
	}
	maps.Copy(tmpl.Labels, labels)
}

// applyAutopilotGlobalExtras applies the GKE Autopilot mutations to default-layer
// objects that cannot be expressed as providercaps entries: init-volume args, the
// read-only seccomp init mount, and the trace/process-agent commands.
//
// The run-path hostPath remap is NOT here: the run-path is the log-collection
// pointer volume, added by the logCollection feature, so its provider variation is
// colocated in that feature's NodeAgentProviderCapabilities.
func applyAutopilotGlobalExtras(mgr feature.PodTemplateManagers) {
	tmpl := mgr.PodTemplateSpec()

	for i := range tmpl.Spec.InitContainers {
		c := &tmpl.Spec.InitContainers[i]
		switch c.Name {
		case autopilotInitVolumeName:
			// The allowlist forbids the default -vn flags.
			c.Args = []string{"cp -r /etc/datadog-agent /opt"}
		}
		// The allowlist requires the seccomp-security mount to be read-only.
		for j := range c.VolumeMounts {
			if c.VolumeMounts[j].Name == common.SeccompSecurityVolumeName {
				c.VolumeMounts[j].ReadOnly = true
			}
		}
	}

	// Autopilot forbids the default trace/process-agent commands (they reference
	// removed config volumes); point them at the in-pod config.
	for i := range tmpl.Spec.Containers {
		c := &tmpl.Spec.Containers[i]
		switch c.Name {
		case string(apicommon.TraceAgentContainerName):
			c.Command = []string{"trace-agent", "-config=/etc/datadog-agent/datadog.yaml"}
		case string(apicommon.ProcessAgentContainerName):
			c.Command = []string{"process-agent", "-config=/etc/datadog-agent/datadog.yaml"}
		}
	}
}
