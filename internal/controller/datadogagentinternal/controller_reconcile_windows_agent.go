// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Windows agent reconciler — prototype for CONTP-1448.
// Creates a Windows-targeted DaemonSet alongside the Linux one when
// spec.override.windowsNodeAgent is present in the DatadogAgentInternal.
//
// Known limitations:
//   - Only reached in the non-EDS path of reconcileV2Agent. EDS + Windows is
//     not supported; document as out-of-scope for the prototype.
//   - Also not reached when nodeAgent is disabled-by-override (reconcileV2Agent
//     returns early before the Windows call). Linux-disabled / Windows-only
//     configurations cannot work until the Windows reconcile is hoisted out.
//   - FIPS + Windows is not supported: ApplyGlobalSettingsNodeAgent calls
//     updateContainerImages which may produce "agent:X.Y.Z-servercore-fips",
//     a tag that does not exist. FIPS users should set windowsNodeAgent.Disabled.

package datadogagentinternal

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// windowsSupportedFeatures is the allowlist of feature IDs whose ManageNodeAgent hook
// runs against the Windows DaemonSet. Features absent here are gated out so they don't
// inject env vars / config that would misconfigure the Windows agent (eBPF/system-probe
// features in particular). This is an allowlist (not a denylist) so new Linux-only
// features don't silently leak onto Windows. The `default` feature is required — it
// configures the base agent. Expand as Windows support for more features is verified.
var windowsSupportedFeatures = map[feature.IDType]bool{
	feature.DefaultIDType:              true, // base agent config (required)
	feature.APMIDType:                  true, // trace-agent runs on Windows (TCP; see non-local-traffic)
	feature.LogCollectionIDType:        true, // log collection — Windows host-log mounts added post-strip
	feature.LiveContainerIDType:        true, // container collection
	feature.LiveProcessIDType:          true, // process collection (no eBPF on Windows)
	feature.ProcessDiscoveryIDType:     true,
	feature.DogstatsdIDType:            true, // dogstatsd (UDP non-local; named pipe future)
	feature.OrchestratorExplorerIDType: true, // node-side config is env-only
	feature.RemoteConfigurationIDType:  true,
	feature.PrometheusScrapeIDType:     true,
	feature.EventCollectionIDType:      true,
}

// windowsContainersFromFeatures derives the Windows agent container set used to undo the
// single-container-strategy rewrite (which collapses the list to unprivileged-single-agent).
//
// It re-derives each feature's *node-agent* required containers by calling Configure (the same
// source the feature factory uses), then keeps only the Windows-supported containers. This is
// deliberately driven by node-agent requirements, not feature-ID presence: e.g. APM can be
// enabled cluster-side only (SSI) with node APM disabled, in which case the apm feature does
// NOT require the trace-agent and we must not add an unconfigured one. The core agent is always
// included. Configure is called against a deep copy of the spec to avoid mutating the original.
func windowsContainersFromFeatures(features []feature.Feature, ddai *datadoghqv1alpha1.DatadogAgentInternal) []apicommon.AgentContainerName {
	specCopy := ddai.Spec.DeepCopy()
	merged := &feature.RequiredComponents{}
	for _, feat := range features {
		rc := feat.Configure(ddai, specCopy, ddai.Status.RemoteConfigConfiguration)
		merged.Merge(&rc)
	}

	containers := []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
	seen := map[apicommon.AgentContainerName]bool{}
	for _, c := range merged.Agent.Containers {
		// Only the Windows-supported sidecars; the core agent is already included and
		// Linux-only containers (system-probe, …) never appear under single-container strategy.
		if (c == apicommon.TraceAgentContainerName || c == apicommon.ProcessAgentContainerName) && !seen[c] {
			seen[c] = true
			containers = append(containers, c)
		}
	}
	return containers
}

// windowsLogPaths extracts the configured logCollection host paths from the DDAI spec so the
// Windows log mounts honor user overrides. Empty fields fall back to the Windows defaults.
func windowsLogPaths(ddai *datadoghqv1alpha1.DatadogAgentInternal) componentagent.WindowsLogPaths {
	var p componentagent.WindowsLogPaths
	if ddai.Spec.Features == nil || ddai.Spec.Features.LogCollection == nil {
		return p
	}
	lc := ddai.Spec.Features.LogCollection
	if lc.TempStoragePath != nil {
		p.TempStoragePath = *lc.TempStoragePath
	}
	if lc.PodLogsPath != nil {
		p.PodLogsPath = *lc.PodLogsPath
	}
	if lc.ContainerLogsPath != nil {
		p.ContainerLogsPath = *lc.ContainerLogsPath
	}
	return p
}

// reconcileV2WindowsAgent creates or updates the Windows agent DaemonSet when
// spec.override.windowsNodeAgent is present. It is called at the end of reconcileV2Agent
// (non-EDS path only) after the Linux DaemonSet has been reconciled.
func (r *Reconciler) reconcileV2WindowsAgent(
	ctx context.Context,
	requiredComponents feature.RequiredComponents,
	features []feature.Feature,
	ddai *datadoghqv1alpha1.DatadogAgentInternal,
	resourcesManager feature.ResourceManagers,
	newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus,
) (reconcile.Result, error) {
	// No windowsNodeAgent override: ensure no stale Windows DaemonSet lingers from a
	// previous opt-in (the key may have been removed), then no-op.
	windowsOverride, ok := ddai.Spec.Override[datadoghqv2alpha1.WindowsNodeAgentComponentName]
	if !ok {
		return reconcile.Result{}, r.ensureWindowsDaemonSetAbsent(ctx, ddai, newStatus)
	}

	// FIPS + Windows is not supported: updateContainerImages would produce
	// agent:X.Y.Z-servercore-fips which does not exist, causing ImagePullBackOff.
	// Surface a clear condition AND remove any previously-created Windows DaemonSet
	// rather than leaving it running misconfigured.
	if ddai.Spec.Global != nil && apiutils.BoolValue(ddai.Spec.Global.UseFIPSAgent) {
		condition.UpdateDatadogAgentInternalStatusConditions(
			newStatus,
			metav1.NewTime(time.Now()),
			"WindowsAgentReconcile",
			metav1.ConditionFalse,
			"FIPSWindowsUnsupported",
			"windowsNodeAgent cannot be used with global.useFIPSAgent: the Windows servercore image has no FIPS variant",
			true,
		)
		return reconcile.Result{}, r.ensureWindowsDaemonSetAbsent(ctx, ddai, newStatus)
	}

	// Honour the Disabled flag the same way the Linux agent reconciler does.
	if apiutils.BoolValue(windowsOverride.Disabled) {
		return reconcile.Result{}, r.ensureWindowsDaemonSetAbsent(ctx, ddai, newStatus)
	}

	logger := ctrl.LoggerFrom(ctx).WithValues("component", "windowsNodeAgent")
	ctx = ctrl.LoggerInto(ctx, logger)

	// Build the Windows DaemonSet from the Windows-specific template.
	// The single-container strategy is a Linux optimization (the factory rewrites the
	// container list to just unprivileged-single-agent); Windows always uses separate
	// containers, so rebuild the container set from the enabled features instead — otherwise
	// the Windows DS would silently run only the core agent and drop trace/process.
	agentComponent := requiredComponents.Agent
	if agentComponent.SingleContainerStrategyEnabled() {
		agentComponent = feature.RequiredComponent{
			IsRequired: agentComponent.IsRequired,
			Containers: windowsContainersFromFeatures(features, ddai),
		}
	}

	instanceName := componentagent.GetWindowsAgentName(ddai)
	daemonset := componentagent.NewDefaultWindowsAgentDaemonset(ddai, &r.options.ExtendedDaemonsetOptions, agentComponent, instanceName)
	podManagers := feature.NewPodTemplateManagers(&daemonset.Spec.Template)

	// Apply global settings: site, credentials (DD_API_KEY), cluster-agent token, etc.
	// Note: ApplyGlobalSettingsNodeAgent may also inject Linux hostPath volumes/mounts
	// when global.criSocketPath, hostCAPath, dockerSocketPath, or useVSock are set.
	// StripLinuxOnlySettings (called below) removes those before the DS is applied.
	global.ApplyGlobalSettingsNodeAgent(logger, podManagers, ddai.GetObjectMeta(), &ddai.Spec, resourcesManager, false, requiredComponents)

	// Run ManageNodeAgent only for features supported on Windows. Linux/eBPF-only
	// features (NPM, USM, CWS, CSPM, OOMKill, eBPF check, TCP queue length, GPU,
	// host-profiler, …) are gated out so they don't inject env vars telling the Windows
	// agent to do Linux-only things (e.g. dial a non-existent system-probe socket).
	// StripLinuxOnlySettings is still applied below as a pod-spec safety net.
	logCollectionEnabled := false
	for _, feat := range features {
		if !windowsSupportedFeatures[feat.ID()] {
			logger.V(1).Info("skipping feature on Windows node agent (no Windows support)", "feature", feat.ID())
			continue
		}
		if feat.ID() == feature.LogCollectionIDType {
			logCollectionEnabled = true
		}
		if err := feat.ManageNodeAgent(podManagers); err != nil {
			return reconcile.Result{}, err
		}
	}

	// Apply spec.override.windowsNodeAgent on top of features + global settings.
	override.PodTemplateSpec(logger, podManagers, windowsOverride, datadoghqv2alpha1.WindowsNodeAgentComponentName, ddai.Name)
	override.DaemonSet(daemonset, windowsOverride)

	// Re-assert the -servercore image suffix: the image override above replaces the whole
	// tag, so a user pinning image.tag=7.81.0 would otherwise get the Linux agent image.
	componentagent.EnsureWindowsServercoreImage(&daemonset.Spec.Template.Spec)

	// Strip Linux-incompatible mutations AFTER overrides so that user-supplied
	// Linux hostPath volumes in the override are also caught.
	// Strips: Linux hostPath volumes, corresponding mounts, Linux-only containers
	// (system-probe, security-agent, …), Linux-only init containers (seccomp-setup, …),
	// and Linux security context fields (capabilities, seccomp, SELinux, readOnlyRootFilesystem).
	// Preserves: feature env vars, emptyDir/configMap volumes, Windows-specific mounts.
	componentagent.StripLinuxOnlySettingsFromTemplate(&daemonset.Spec.Template)

	// Force APM/DogStatsD non-local traffic AFTER features + strip so it wins over the
	// DogStatsD feature (which writes DD_DOGSTATSD_NON_LOCAL_TRAFFIC=<user value, default
	// false> with last-writer-wins merge). Windows has no Unix socket, so the agents must
	// listen on non-local TCP/UDP to be reachable.
	// NOTE: the component=agent local services created by the APM/DogStatsD features do NOT
	// select Windows pods (labeled component=agent-windows), so workload->agent traffic on
	// Windows currently requires hostPort (features.apm/dogstatsd hostPort) or a future
	// Windows-specific local service. See windowsNodeAgent docs.
	componentagent.EnsureWindowsIntakeReachable(podManagers)

	// Add the Windows host-log hostPath volumes/mounts when log collection is enabled. The
	// logcollection feature only injects the Linux paths (stripped above); these Windows-path
	// mounts survive the strip. Honor any configured logCollection paths (Windows defaults
	// otherwise) so custom Windows log/runtime paths take effect.
	if logCollectionEnabled {
		componentagent.AddWindowsLogCollectionVolumes(&daemonset.Spec.Template.Spec, windowsLogPaths(ddai))
	}

	return r.createOrUpdateDaemonset(ctx, ddai, daemonset, newStatus, updateDSStatusV2WithWindowsAgent)
}

func updateDSStatusV2WithWindowsAgent(dsName string, ds *appsv1.DaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	// Track the Windows DaemonSet rollout in its own status field, separate from the
	// Linux Agent field, so the two don't clobber each other.
	newStatus.AgentWindows = condition.UpdateDaemonSetStatusDDAI(dsName, ds, newStatus.AgentWindows, &updateTime)
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, updateTime, "WindowsAgentReconcile", status, reason, message, true)
}

// ensureWindowsDaemonSetAbsent removes the Windows DaemonSet(s) and clears the AgentWindows
// status field. It is idempotent and used by every path that must NOT run a Windows
// DaemonSet: the override key being absent (opt-out / removed), Disabled, FIPS, and EDS.
//
// It lists by the Windows component label (agent.datadoghq.com/component=agent-windows)
// rather than the default name so a user-renamed DaemonSet (via override.name) is also
// cleaned up. It only ever touches AgentWindows, never the Linux Agent status.
func (r *Reconciler) ensureWindowsDaemonSetAbsent(ctx context.Context, ddai *datadoghqv1alpha1.DatadogAgentInternal, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus) error {
	dsList := &appsv1.DaemonSetList{}
	if err := r.client.List(ctx, dsList,
		client.InNamespace(ddai.GetNamespace()),
		client.MatchingLabels{
			apicommon.AgentDeploymentComponentLabelKey: componentagent.WindowsAgentResourceSuffix,
			// Owner-scope by part-of so we never delete another DatadogAgent(Internal)'s
			// Windows DaemonSet that happens to share the namespace.
			kubernetes.AppKubernetesPartOfLabelKey: object.NewPartOfLabelValue(ddai).String(),
		},
	); err != nil {
		return err
	}
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		if err := r.client.Delete(ctx, ds); err != nil && !errors.IsNotFound(err) {
			return err
		}
		ctrl.LoggerFrom(ctx).WithValues("object.kind", "DaemonSet", "object.namespace", ds.Namespace, "object.name", ds.Name).Info("Deleted Windows DaemonSet")
		event := buildEventInfo(ds.Name, ds.Namespace, kubernetes.DaemonSetKind, datadog.DeletionEvent)
		r.recordEvent(ddai, event)
	}
	newStatus.AgentWindows = nil
	return nil
}
