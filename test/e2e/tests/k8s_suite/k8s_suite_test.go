// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"

	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var (
	matchTags = []*regexp.Regexp{regexp.MustCompile("kube_container_name:.*")}
	matchOpts = []client.MatchOpt[*aggregator.MetricSeries]{client.WithMatchingTags[*aggregator.MetricSeries](matchTags)}
)

const (
	coreAgentContainerName    = "agent"
	clusterAgentContainerName = "cluster-agent"
	adpContainerName          = "agent-data-plane"
	dsdSocketVolumeName       = "dsdsocket"
	dsdSocketMountPath        = "/var/run/datadog"
	dsdSocketHostPath         = "/var/run/datadog"
	dsdPort                   = int32(8125)
)

const configBackendSnapshotCommand = `set +e
export DD_LOG_LEVEL=off
echo "env.DD_CONF_NODETREEMODEL=$(printenv DD_CONF_NODETREEMODEL || true)"
for key in conf_nodetreemodel kubelet_core_check_enabled kubelet_tls_verify cluster_checks.enabled logs_enabled logs_config.container_collect_all admission_controller.enabled admission_controller.probe.enabled service_discovery.enabled discovery.enabled; do
  value="$(agent config get "$key" 2>&1 | tr '\n' ' ')"
  echo "config.${key}=${value}"
done
`

func agentStatusCommand(args ...string) []string {
	return []string{"sh", "-c", "DD_LOG_LEVEL=off exec agent status " + strings.Join(args, " ")}
}

type k8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	local bool
}

func sortedStringKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseKeyValueSnapshot(output string) map[string]string {
	snapshot := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		snapshot[key] = strings.TrimSpace(value)
	}
	return snapshot
}

func snapshotSnippet(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= 3000 {
		return output
	}
	return output[:3000] + "...(truncated)"
}

func mergeSnapshot(dst map[string]string, prefix string, src map[string]string) {
	for key, value := range src {
		dst[prefix+"."+key] = value
	}
}

func formatSnapshot(title string, snapshot map[string]string) string {
	var builder strings.Builder
	builder.WriteString("\n========== " + title + " ==========\n")
	for _, key := range sortedStringKeys(snapshot) {
		builder.WriteString(fmt.Sprintf("%s=%s\n", key, snapshot[key]))
	}
	return builder.String()
}

func formatSnapshotDiff(title, leftLabel, rightLabel string, left, right map[string]string) string {
	allKeys := map[string]struct{}{}
	for key := range left {
		allKeys[key] = struct{}{}
	}
	for key := range right {
		allKeys[key] = struct{}{}
	}

	var builder strings.Builder
	builder.WriteString("\n========== " + title + " ==========\n")
	builder.WriteString("- " + leftLabel + "\n")
	builder.WriteString("+ " + rightLabel + "\n")
	changed := false
	for _, key := range sortedStringKeys(allKeys) {
		leftValue := left[key]
		rightValue := right[key]
		if leftValue == rightValue {
			continue
		}
		changed = true
		builder.WriteString(fmt.Sprintf("- %s=%s\n", key, leftValue))
		builder.WriteString(fmt.Sprintf("+ %s=%s\n", key, rightValue))
	}
	if !changed {
		builder.WriteString("(no differences)\n")
	}
	return builder.String()
}

func collectorStatusSnapshot(output string, checkNames ...string) map[string]string {
	snapshot := map[string]string{
		"raw_snippet": snapshotSnippet(output),
	}
	parsed := common.ParseCollectorJson(output)
	snapshot["top_level_keys"] = strings.Join(sortedStringKeys(parsed), ",")

	runnerStats, ok := parsed["runnerStats"].(map[string]any)
	if !ok {
		snapshot["runnerStats.present"] = "false"
		return snapshot
	}
	snapshot["runnerStats.present"] = "true"
	snapshot["runnerStats.keys"] = strings.Join(sortedStringKeys(runnerStats), ",")

	runningChecks, ok := runnerStats["Checks"].(map[string]any)
	if !ok {
		snapshot["runnerStats.Checks.present"] = "false"
		return snapshot
	}
	snapshot["runnerStats.Checks.present"] = "true"
	snapshot["runnerStats.Checks.keys"] = strings.Join(sortedStringKeys(runningChecks), ",")

	for _, checkName := range checkNames {
		prefix := "check." + checkName
		check, ok := runningChecks[checkName].(map[string]any)
		if !ok {
			snapshot[prefix+".present"] = "false"
			continue
		}
		snapshot[prefix+".present"] = "true"
		snapshot[prefix+".instance_count"] = fmt.Sprintf("%d", len(check))

		instanceSummaries := make([]string, 0, len(check))
		for instanceName, instance := range check {
			instanceMap, ok := instance.(map[string]any)
			if !ok {
				instanceSummaries = append(instanceSummaries, instanceName+":not-a-map")
				continue
			}
			instanceSummaries = append(instanceSummaries, fmt.Sprintf("%s:CheckName=%v LastError=%v TotalErrors=%v TotalMetricSamples=%v", instanceName, instanceMap["CheckName"], instanceMap["LastError"], instanceMap["TotalErrors"], instanceMap["TotalMetricSamples"]))
		}
		sort.Strings(instanceSummaries)
		snapshot[prefix+".instances"] = strings.Join(instanceSummaries, " | ")
	}

	return snapshot
}

func (s *k8sSuite) TestGenericK8s() {
	defaultOperatorOpts := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		// RBAC/CRDs are installed via our e2e kustomize (`config/new-e2e`, namePrefix: datadog-operator-e2e-).
		// Ensure the Helm-installed operator uses the same ServiceAccount (and doesn't create its own RBAC),
		// otherwise it may run under a different SA (e.g. datadog-operator-linux) lacking new permissions.
		operatorparams.WithHelmValues(`installCRDs: false
rbac:
  create: false
serviceAccount:
  create: false
  name: datadog-operator-e2e-controller-manager
`),
	}

	defaultProvisionerOpts := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(defaultOperatorOpts...),
		provisioners.WithLocal(s.local),
	}

	defaultDDAOpts := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
	}

	// --- Suite-level cleanup (registered before any subtests run) ---
	//
	// We need to ensure the final env of the suite is left without a DatadogAgent before the
	// underlying Pulumi teardown happens; otherwise CRD deletion may race with DDA deletion.
	//
	// This runs ONCE, at the very end of the whole suite (not after each subtest).
	t := s.T()
	var lastTestName string
	updateEnv := func(testName string, opts []provisioners.KubernetesProvisionerOption) {
		lastTestName = testName
		s.UpdateEnv(provisioners.KubernetesProvisioner(opts...))
	}
	// applyDDA tears down any in-stack DatadogAgent before installing the new
	// one. Used instead of updateEnv whenever a subtest applies a DDA.
	//
	// Why the explicit two-step swap: applying a new DDA on top of a previous
	// one does delete+create concurrently, which can leave the new agent
	// DaemonSet pod stuck on resources still owned by the previous DDA. The
	// most visible cases are:
	//   - K8s <1.20: the legacy SA-token controller can't keep up with the SA
	//     delete+create churn during the swap, so the new agent pod sits in
	//     FailedMount on its auto-generated <sa>-token-<rand> Secret;
	//   - host-port subtests (APM hostPort, DSD UDP): the previous agent pod
	//     hasn't released the port yet when the new pod tries to bind.
	// Both manifest as "agent pod never reaches Running" on the new DDA.
	applyDDA := func(testName string, opts []provisioners.KubernetesProvisionerOption) {
		cleanupOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName(testName),
			provisioners.WithoutDDA(),
		}
		cleanupOpts = append(cleanupOpts, defaultProvisionerOpts...)
		updateEnv(testName, cleanupOpts)
		updateEnv(testName, opts)
	}
	t.Cleanup(func() {
		if lastTestName == "" {
			return
		}

		// Delete all Datadog custom resources before Pulumi stack destroy
		// to avoid CRD deletion timeout due to finalizers.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if k8sConfig := s.Env().KubernetesCluster.KubernetesClient.K8sConfig; k8sConfig != nil {
			if err := utils.DeleteAllDatadogResources(ctx, k8sConfig, common.NamespaceName); err != nil {
				t.Logf("Warning: failed to delete Datadog resources during cleanup: %v", err)
			}
		}
	})

	s.T().Run("Verify Operator", func(t *testing.T) {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyOperator(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client())
		}, 300*time.Second, 15*time.Second, "Could not validate operator pod in time")
	})

	collectConfigBackendSnapshot := func(label, ddaName, manifestPath, testName string) map[string]string {
		snapshot := map[string]string{
			"label":     label,
			"dda_name":  ddaName,
			"manifest":  manifestPath,
			"test_name": testName,
		}

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         ddaName,
				YamlFilePath: manifestPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName(testName),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		applyDDA(testName, provisionerOptions)

		err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		snapshot["fakeintake.flush.error"] = fmt.Sprintf("%v", err)

		nodeSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=" + ddaName
		clusterSelector := common.ClusterAgentSelector + ",agent.datadoghq.com/name=" + ddaName
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), nodeSelector)
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, clusterSelector)
		}, 5*time.Minute, 15*time.Second, "could not get comparison pods running for %s", label)

		agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{
			LabelSelector: nodeSelector,
			FieldSelector: "status.phase=Running",
		})
		if err != nil {
			snapshot["node.list.error"] = err.Error()
		} else if len(agentPods.Items) == 0 {
			snapshot["node.pods.running"] = "0"
		} else {
			podName := agentPods.Items[0].Name
			snapshot["node.pod"] = podName
			output, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, podName, coreAgentContainerName, []string{"sh", "-c", configBackendSnapshotCommand})
			snapshot["node.config.exec.error"] = fmt.Sprintf("%v", err)
			snapshot["node.config.exec.stderr"] = strings.TrimSpace(stderr)
			mergeSnapshot(snapshot, "node", parseKeyValueSnapshot(output))

			output, stderr, err = s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, podName, coreAgentContainerName, agentStatusCommand("collector", "-j"))
			snapshot["node.collector.exec.error"] = fmt.Sprintf("%v", err)
			snapshot["node.collector.exec.stderr"] = strings.TrimSpace(stderr)
			mergeSnapshot(snapshot, "node.collector", collectorStatusSnapshot(output, "kubelet", "http_check"))
		}

		clusterAgentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{
			LabelSelector: clusterSelector,
			FieldSelector: "status.phase=Running",
		})
		if err != nil {
			snapshot["cluster.list.error"] = err.Error()
		} else if len(clusterAgentPods.Items) == 0 {
			snapshot["cluster.pods.running"] = "0"
		} else {
			podName := clusterAgentPods.Items[0].Name
			snapshot["cluster.pod"] = podName
			output, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, podName, clusterAgentContainerName, []string{"sh", "-c", configBackendSnapshotCommand})
			snapshot["cluster.config.exec.error"] = fmt.Sprintf("%v", err)
			snapshot["cluster.config.exec.stderr"] = strings.TrimSpace(stderr)
			mergeSnapshot(snapshot, "cluster", parseKeyValueSnapshot(output))

			output, stderr, err = s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, podName, clusterAgentContainerName, agentStatusCommand("collector", "-j"))
			snapshot["cluster.collector.exec.error"] = fmt.Sprintf("%v", err)
			snapshot["cluster.collector.exec.stderr"] = strings.TrimSpace(stderr)
			mergeSnapshot(snapshot, "cluster.collector", collectorStatusSnapshot(output, "kubernetes_state_core"))
		}

		metricNamesToCompare := []string{"kubernetes.cpu.usage.total", "kubernetes_state.container.running", "network.http.can_connect"}
		deadline := time.Now().Add(2 * time.Minute)
		for {
			metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
			snapshot["fakeintake.metric_names.error"] = fmt.Sprintf("%v", err)
			for _, metricName := range metricNamesToCompare {
				present := "false"
				for _, receivedMetricName := range metricNames {
					if receivedMetricName == metricName {
						present = "true"
						break
					}
				}
				snapshot["fakeintake.metric."+metricName+".present"] = present

				metrics, err := s.Env().FakeIntake.Client().FilterMetrics(metricName, matchOpts...)
				snapshot["fakeintake.metric."+metricName+".filter_error"] = fmt.Sprintf("%v", err)
				snapshot["fakeintake.metric."+metricName+".series"] = fmt.Sprintf("%d", len(metrics))
				points := 0
				positivePoints := 0
				for _, metric := range metrics {
					for _, point := range metric.Points {
						points++
						if point.Value > 0 {
							positivePoints++
						}
					}
				}
				snapshot["fakeintake.metric."+metricName+".points"] = fmt.Sprintf("%d", points)
				snapshot["fakeintake.metric."+metricName+".positive_points"] = fmt.Sprintf("%d", positivePoints)
			}

			if snapshot["fakeintake.metric.kubernetes.cpu.usage.total.present"] == "true" &&
				snapshot["fakeintake.metric.kubernetes_state.container.running.present"] == "true" {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(15 * time.Second)
		}

		return snapshot
	}

	s.T().Run("Config backend comparison", func(t *testing.T) {
		defaultManifestPath, err := common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(t, err)
		viperManifestPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-minimum-viper.yaml"))
		assert.NoError(t, err)

		defaultLabel := "ORIGINAL: Agent 7.79 default config backend (NodeTree, DD_CONF_NODETREEMODEL unset)"
		viperLabel := "VIPER: Agent 7.79 with DD_CONF_NODETREEMODEL=viper (7.78-style config backend)"
		t.Log("E2E CONFIG BACKEND COMPARISON: this branch intentionally runs only the comparison subtest, then skips the long suite.")
		t.Log("LEFT/" + defaultLabel)
		t.Log("RIGHT/" + viperLabel)

		defaultSnapshot := collectConfigBackendSnapshot(defaultLabel, "dda-config-default", defaultManifestPath, "e2e-config-default")
		viperSnapshot := collectConfigBackendSnapshot(viperLabel, "dda-config-viper", viperManifestPath, "e2e-config-viper")

		t.Log(formatSnapshot("FULL SNAPSHOT: "+defaultLabel, defaultSnapshot))
		t.Log(formatSnapshot("FULL SNAPSHOT: "+viperLabel, viperSnapshot))
		t.Log(formatSnapshotDiff("DIFF: 7.79 DEFAULT NODETREE -> 7.79 VIPER BACKEND", defaultLabel, viperLabel, defaultSnapshot, viperSnapshot))
	})
	return

	s.T().Run("Minimal DDA config", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-minimum",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-minimal-dda"),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		applyDDA("e2e-operator-minimal-dda", provisionerOptions)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+",agent.datadoghq.com/name=dda-minimum")
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, common.ClusterAgentSelector+",agent.datadoghq.com/name=dda-minimum")

			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-minimum",
				FieldSelector: "status.phase=Running"})
			assert.NoError(s.T(), err)

			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, coreAgentContainerName, agentStatusCommand("collector", "-j"))
				assert.NoError(c, err)
				utils.VerifyCheck(c, output, "kubelet")
			}

			metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
			s.Assert().NoError(err)
			assert.Contains(c, metricNames, "kubernetes.cpu.usage.total")

			metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes.cpu.usage.total", matchOpts...)
			s.Assert().NoError(err)

			for _, metric := range metrics {
				for _, points := range metric.Points {
					s.Assert().Greater(points.Value, float64(0))
				}
			}

			clusterAgentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.ClusterAgentSelector + ",agent.datadoghq.com/e2e-test=datadog-agent-minimum"})
			assert.NoError(s.T(), err)

			for _, pod := range clusterAgentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, clusterAgentContainerName, agentStatusCommand("collector", "-j"))
				assert.NoError(c, err)
				utils.VerifyCheck(c, output, "kubernetes_state_core")
			}

			s.verifyKSMCheck(c)
		}, 10*time.Minute, 30*time.Second, "could not validate KSM (cluster check) metrics in time")

	})

	s.T().Run("KSM check works cluster check runner", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-ccr-enabled.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "datadog-ccr-enabled",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-ksm-ccr"),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		applyDDA("e2e-operator-ksm-ccr", provisionerOptions)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), "app.kubernetes.io/instance=datadog-ccr-enabled-agent")

			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-checks-runner")

			ccrPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-checks-runner"})
			assert.NoError(s.T(), err)

			for _, ccr := range ccrPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, ccr.Name, coreAgentContainerName, agentStatusCommand("collector", "-j"))
				assert.NoError(c, err)
				utils.VerifyCheck(c, output, "kubernetes_state_core")
			}

			s.verifyKSMCheck(c, "kubernetes_state_customresource.uptodateagents")
		}, 15*time.Minute, 15*time.Second, "could not validate kubernetes_state_core (cluster check on CCR) check in time")
	})

	s.T().Run("Autodiscovery works", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{Name: "dda-autodiscovery", YamlFilePath: ddaConfigPath}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-autodiscovery"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "nginx", Path: strings.Join([]string{common.ManifestsPath, "autodiscovery-annotation.yaml"}, "/")}),
			provisioners.WithLocal(s.local),
		}
		provisionerOptions = append(provisionerOptions, defaultProvisionerOpts...)

		// Add nginx with annotations
		applyDDA("e2e-operator-autodiscovery", provisionerOptions)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, "app=nginx")

			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+",agent.datadoghq.com/name=dda-autodiscovery")

			// check agent pods for http check
			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-autodiscovery",
				FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, coreAgentContainerName, agentStatusCommand("collector", "-j"))
				assert.NoError(c, err)

				utils.VerifyCheck(c, output, "http_check")
			}

			s.verifyHTTPCheck(c)
		}, 900*time.Second, 15*time.Second, "could not validate http_check in time")
	})

	s.T().Run("Logs collection works", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-logs.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "datadog-agent-logs",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-logs-collection"),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		applyDDA("e2e-operator-logs-collection", provisionerOptions)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		// Verify logs collection on agent pod
		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), "app.kubernetes.io/instance=datadog-agent-logs-agent")

			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/instance=datadog-agent-logs-agent"})
			assert.NoError(c, err)

			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, coreAgentContainerName, agentStatusCommand("logs", "agent", "-j"))
				assert.NoError(c, err)
				utils.VerifyAgentPodLogs(c, output)
			}

			s.verifyAPILogs(c)
		}, 900*time.Second, 15*time.Second, "could not validate logs collection in time")
	})

	s.T().Run("APM hostPort k8s service UDP works", func(t *testing.T) {
		var apmAgentSelector = ",agent.datadoghq.com/name=datadog-agent-apm"
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "apm", "datadog-agent-apm.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "datadog-agent-apm",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		ddaProvisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-apm"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{
				Name: "tracegen-deploy",
				Path: strings.Join([]string{common.ManifestsPath, "apm", "tracegen-deploy.yaml"}, "/"),
			}),
			provisioners.WithLocal(s.local),
		}
		ddaProvisionerOptions = append(ddaProvisionerOptions, defaultProvisionerOpts...)

		// Deploy APM DatadogAgent and tracegen
		applyDDA("e2e-operator-apm", ddaProvisionerOptions)

		// Verify traces collection on agent pod
		s.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify tracegen deployment is running
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, "app=tracegen-tribrid")

			// Verify agent pods are running
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+apmAgentSelector)
			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + apmAgentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			// This works because we have a single Agent pod (so located on same node as tracegen)
			// Otherwise, we would need to deploy tracegen on the same node as the Agent pod / as a DaemonSet
			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, coreAgentContainerName, agentStatusCommand("apm", "agent", "-j"))
				assert.NoError(c, err)

				utils.VerifyAgentTraces(c, output)
			}

			// Verify traces collection ingestion by fakeintake
			s.verifyAPITraces(c)
		}, 600*time.Second, 15*time.Second, "could not validate traces on agent pod") // TODO: check duration
	})

	// --- DogStatsD subtests ---

	// --- Subtest: DSD UDP, ADP disabled ---
	s.T().Run("DSD UDP without ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-udp.yaml"))
		assert.NoError(s.T(), err)
		senderPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "dsd-udp-sender.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := append([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-dsd-udp",
				YamlFilePath: ddaConfigPath,
			}),
		}, defaultDDAOpts...)

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-udp"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dsd-udp-sender", Path: senderPath}),
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		applyDDA("e2e-operator-dsd-udp", provisionerOpts)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-udp"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerAbsent(c, pod, adpContainerName)
				assertContainerHasUDPHostPort(c, pod, coreAgentContainerName, dsdPort)
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDP without ADP: pod spec verification failed")

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			s.verifyDSDMetrics(c, "e2e.dsd.udp.counter")
		}, 3*time.Minute, 10*time.Second, "DSD UDP without ADP: metrics not received by fakeintake")
	})

	// --- Subtest: DSD UDP, ADP enabled ---
	s.T().Run("DSD UDP with ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-udp-adp.yaml"))
		assert.NoError(s.T(), err)
		senderPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "dsd-udp-sender.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := append([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-dsd-udp-adp",
				YamlFilePath: ddaConfigPath,
			}),
		}, defaultDDAOpts...)

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-udp-adp"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dsd-udp-sender", Path: senderPath}),
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		applyDDA("e2e-operator-dsd-udp-adp", provisionerOpts)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-udp-adp"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerPresent(c, pod, adpContainerName)
				assertContainerHasUDPHostPort(c, pod, adpContainerName, dsdPort)
				assertContainerDoesNotHaveHostPort(c, pod, coreAgentContainerName, dsdPort)
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDP with ADP: pod spec verification failed")

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			s.verifyDSDMetrics(c, "e2e.dsd.udp.counter")
		}, 3*time.Minute, 10*time.Second, "DSD UDP with ADP: metrics not received by fakeintake")
	})

	// --- Subtest: DSD UDS, ADP disabled ---
	s.T().Run("DSD UDS without ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-uds.yaml"))
		assert.NoError(s.T(), err)
		senderPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "dsd-uds-sender.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := append([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-dsd-uds",
				YamlFilePath: ddaConfigPath,
			}),
		}, defaultDDAOpts...)

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-uds"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dsd-uds-sender", Path: senderPath}),
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		applyDDA("e2e-operator-dsd-uds", provisionerOpts)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-uds"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerAbsent(c, pod, adpContainerName)
				assertContainerHasVolumeMount(c, pod, coreAgentContainerName, dsdSocketVolumeName, dsdSocketMountPath)
				assertPodHasHostPathVolume(c, pod, dsdSocketVolumeName, dsdSocketHostPath)
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDS without ADP: pod spec verification failed")

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			s.verifyDSDMetrics(c, "e2e.dsd.uds.counter")
		}, 3*time.Minute, 10*time.Second, "DSD UDS without ADP: metrics not received by fakeintake")
	})

	// --- Subtest: DSD UDS, ADP enabled ---
	s.T().Run("DSD UDS with ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-uds-adp.yaml"))
		assert.NoError(s.T(), err)
		senderPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "dsd-uds-sender.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := append([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-dsd-uds-adp",
				YamlFilePath: ddaConfigPath,
			}),
		}, defaultDDAOpts...)

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-uds-adp"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dsd-uds-sender", Path: senderPath}),
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		applyDDA("e2e-operator-dsd-uds-adp", provisionerOpts)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-uds-adp"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerPresent(c, pod, adpContainerName)
				assertContainerHasVolumeMount(c, pod, adpContainerName, dsdSocketVolumeName, dsdSocketMountPath)
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDS with ADP: pod spec verification failed")

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			s.verifyDSDMetrics(c, "e2e.dsd.uds.counter")
		}, 3*time.Minute, 10*time.Second, "DSD UDS with ADP: metrics not received by fakeintake")
	})
}

func (s *k8sSuite) verifyDSDMetrics(c *assert.CollectT, metricName string) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metricNames, metricName)

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(metricName)
	assert.NoError(c, err)
	assert.NotEmptyf(c, metrics, "expected metric series for %s to be non-empty", metricName)
}

func (s *k8sSuite) verifyAPILogs(t assert.TestingT) {
	logs, err := s.Env().FakeIntake.Client().FilterLogs("agent")
	assert.NoError(t, err)
	assert.NotEmptyf(t, logs, "Expected fake intake-ingested logs to not be empty")
}

func (s *k8sSuite) verifyAPITraces(c *assert.CollectT) {
	traces, err := s.Env().FakeIntake.Client().GetTraces()
	assert.NoError(c, err)
	assert.NotEmptyf(c, traces, fmt.Sprintf("Expected fake intake-ingested traces to not be empty: %s", err))
}

func (s *k8sSuite) verifyKSMCheck(c *assert.CollectT, expectedMetrics ...string) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metricNames, "kubernetes_state.container.running")
	for _, metric := range expectedMetrics {
		assert.Contains(c, metricNames, metric)
	}

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes_state.container.running", matchOpts...)
	assert.NoError(c, err)
	assert.NotEmptyf(c, metrics, fmt.Sprintf("expected metric series to not be empty: %s", err))
}

func (s *k8sSuite) verifyHTTPCheck(c *assert.CollectT) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metricNames, "network.http.can_connect")
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("network.http.can_connect")
	assert.NoError(c, err)
	assert.Greater(c, len(metrics), 0)
	for _, metric := range metrics {
		for _, points := range metric.Points {
			assert.Greater(c, points.Value, float64(0))
		}
	}
}

// --- DogStatsD assertion helpers ---

// findContainer returns the container with the given name, or nil if not found.
func findContainer(pod corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}
	return nil
}

func assertContainerPresent(c *assert.CollectT, pod corev1.Pod, containerName string) {
	container := findContainer(pod, containerName)
	assert.NotNilf(c, container, "expected container %q to be present in pod %s", containerName, pod.Name)
}

func assertContainerAbsent(c *assert.CollectT, pod corev1.Pod, containerName string) {
	container := findContainer(pod, containerName)
	assert.Nilf(c, container, "expected container %q to be absent in pod %s", containerName, pod.Name)
}

func assertContainerHasUDPHostPort(c *assert.CollectT, pod corev1.Pod, containerName string, port int32) {
	container := findContainer(pod, containerName)
	if !assert.NotNilf(c, container, "container %q not found in pod %s", containerName, pod.Name) {
		return
	}
	for _, p := range container.Ports {
		if p.Protocol == corev1.ProtocolUDP && p.HostPort == port {
			return
		}
	}
	assert.Failf(c, "host port not found", "expected container %q in pod %s to have UDP host port %d", containerName, pod.Name, port)
}

func assertContainerDoesNotHaveHostPort(c *assert.CollectT, pod corev1.Pod, containerName string, port int32) {
	container := findContainer(pod, containerName)
	if container == nil {
		return // container absent means no host port
	}
	for _, p := range container.Ports {
		if p.Protocol == corev1.ProtocolUDP && p.HostPort == port {
			assert.Failf(c, "unexpected host port", "expected container %q in pod %s to NOT have UDP host port %d", containerName, pod.Name, port)
			return
		}
	}
}

func assertContainerHasEnvVar(c *assert.CollectT, pod corev1.Pod, containerName, envName, envValue string) {
	container := findContainer(pod, containerName)
	if !assert.NotNilf(c, container, "container %q not found in pod %s", containerName, pod.Name) {
		return
	}
	for _, env := range container.Env {
		if env.Name == envName {
			assert.Equalf(c, envValue, env.Value, "env var %s in container %q of pod %s", envName, containerName, pod.Name)
			return
		}
	}
	assert.Failf(c, "env var not found", "expected container %q in pod %s to have env var %s=%s", containerName, pod.Name, envName, envValue)
}

func assertContainerHasVolumeMount(c *assert.CollectT, pod corev1.Pod, containerName, volumeName, mountPath string) {
	container := findContainer(pod, containerName)
	if !assert.NotNilf(c, container, "container %q not found in pod %s", containerName, pod.Name) {
		return
	}
	for _, vm := range container.VolumeMounts {
		if vm.Name == volumeName && vm.MountPath == mountPath {
			return
		}
	}
	assert.Failf(c, "volume mount not found", "expected container %q in pod %s to have volume mount %s at %s", containerName, pod.Name, volumeName, mountPath)
}

func assertPodHasHostPathVolume(c *assert.CollectT, pod corev1.Pod, volumeName, hostPath string) {
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == volumeName {
			if !assert.NotNilf(c, vol.VolumeSource.HostPath, "volume %q in pod %s is not a HostPath volume", volumeName, pod.Name) {
				return
			}
			assert.Equalf(c, hostPath, vol.VolumeSource.HostPath.Path, "volume %q hostPath in pod %s", volumeName, pod.Name)
			return
		}
	}
	assert.Failf(c, "volume not found", "expected pod %s to have volume %s with hostPath %s", pod.Name, volumeName, hostPath)
}
