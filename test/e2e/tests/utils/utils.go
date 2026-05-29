// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-operator/test/e2e/common"
)

func VerifyOperator(t *testing.T, c *assert.CollectT, namespace string, k8sClient kubeClient.Interface) {
	VerifyNumPodsForSelector(t, c, namespace, k8sClient, 1, "app.kubernetes.io/name=datadog-operator")
}

func VerifyNumPodsForSelector(t *testing.T, c *assert.CollectT, namespace string, k8sClient kubeClient.Interface, numPods int, selector string) {
	t.Log("Waiting for number of pods created", "number", numPods, "selector", selector)
	podsList, err := k8sClient.CoreV1().Pods(namespace).List(t.Context(), metav1.ListOptions{
		LabelSelector: selector,
		FieldSelector: "status.phase=Running",
	})
	require.NoError(c, err)
	require.NotNil(c, podsList)
	assert.NotEmpty(c, podsList.Items)

	readyPods := make([]corev1.Pod, 0, len(podsList.Items))
	for _, pod := range podsList.Items {
		if isPodReady(pod) {
			readyPods = append(readyPods, pod)
		}
	}
	assert.Lenf(c, readyPods, numPods, "expected %d ready running pod(s) for selector %q, found %d ready out of %d running pod(s): %s", numPods, selector, len(readyPods), len(podsList.Items), podReadinessSummary(podsList.Items))
}

func VerifyAgentPods(t *testing.T, c *assert.CollectT, namespace string, k8sClient kubeClient.Interface, selector string) {
	nodesList, err := k8sClient.CoreV1().Nodes().List(t.Context(), metav1.ListOptions{})
	require.NoError(c, err)
	assert.NotNil(c, nodesList)
	assert.NotEmpty(c, nodesList.Items)
	VerifyNumPodsForSelector(t, c, namespace, k8sClient, len(nodesList.Items), selector)
}

func VerifyCheck(c *assert.CollectT, collectorOutput string, checkName string) {
	var runningChecks map[string]any

	checksJson := common.ParseCollectorJson(collectorOutput)
	if len(checksJson) == 0 {
		assert.Failf(c, "agent collector status JSON not found", "raw output: %s", truncateStatusOutput(collectorOutput))
		return
	}

	runnerStats, runnerStatsOk := checksJson["runnerStats"].(map[string]any)
	if !runnerStatsOk {
		assert.Failf(c, "runnerStats field is not a map or is nil", "top-level status keys: %s; raw output: %s", strings.Join(sortedMapKeys(checksJson), ", "), truncateStatusOutput(collectorOutput))
		return
	}

	var checksOk bool
	runningChecks, checksOk = runnerStats["Checks"].(map[string]any)
	if !checksOk {
		assert.Failf(c, "Checks field is not a map or is nil", "runnerStats keys: %s; raw output: %s", strings.Join(sortedMapKeys(runnerStats), ", "), truncateStatusOutput(collectorOutput))
		return
	}

	if check, found := runningChecks[checkName].(map[string]any); found {
		for _, instance := range check {
			instanceMap, instanceOk := instance.(map[string]any)
			if !instanceOk {
				continue
			}

			checkNameVal, checkNameOk := instanceMap["CheckName"].(string)
			if checkNameOk {
				assert.Equal(c, checkName, checkNameVal)
			}

			lastError, exists := instanceMap["LastError"].(string)
			assert.True(c, exists)
			assert.Empty(c, lastError)

			totalErrors, exists := instanceMap["TotalErrors"].(float64)
			assert.True(c, exists)
			assert.Zero(c, totalErrors)

			totalMetricSamples, exists := instanceMap["TotalMetricSamples"].(float64)
			assert.True(c, exists)
			assert.Greater(c, totalMetricSamples, float64(0))
		}
	} else {
		assert.Failf(c, "Check not found", "Check %s not found or not yet running; available checks: %s", checkName, strings.Join(sortedMapKeys(runningChecks), ", "))
	}
}

func VerifyAgentPodLogs(c *assert.CollectT, collectorOutput string) {
	var agentLogs []any
	logsJson := common.ParseCollectorJson(collectorOutput)

	tailedIntegrations := 0
	if len(logsJson) == 0 {
		assert.Failf(c, "agent logs status JSON not found", "raw output: %s", truncateStatusOutput(collectorOutput))
		return
	}

	var ok bool
	logsStats, logsStatsOk := logsJson["logsStats"].(map[string]any)
	if !logsStatsOk {
		assert.Failf(c, "logsStats field is not a map or is nil", "top-level status keys: %s; raw output: %s", strings.Join(sortedMapKeys(logsJson), ", "), truncateStatusOutput(collectorOutput))
		return
	}
	agentLogs, ok = logsStats["integrations"].([]any)
	assert.Truef(c, ok, "logsStats keys: %s; raw output: %s", strings.Join(sortedMapKeys(logsStats), ", "), truncateStatusOutput(collectorOutput))
	assert.NotEmpty(c, agentLogs)
	for _, log := range agentLogs {
		logMap, logOk := log.(map[string]any)
		if !logOk {
			assert.Failf(c, "logs integration entry is not a map", "entry: %v; raw output: %s", log, truncateStatusOutput(collectorOutput))
			continue
		}

		sources, sourcesOk := logMap["sources"].([]any)
		if !sourcesOk || len(sources) == 0 {
			continue
		}

		if integration, integrationOk := sources[0].(map[string]any); integrationOk {
			messages, exists := integration["messages"].([]any)
			assert.True(c, exists)
			assert.NotEmpty(c, messages)

			if len(messages) == 0 {
				continue
			}

			message, msgOk := messages[0].(string)
			assert.True(c, msgOk)
			assert.NotEmpty(c, message)
			num, _ := strconv.Atoi(string(message[0]))
			if num > 0 && strings.Contains(message, "files tailed") {
				tailedIntegrations++
			}
		} else {
			assert.True(c, integrationOk, "Failed to get sources from logs. Possible causes: missing 'sources' field, empty array, or incorrect data format.")
		}
	}
	totalIntegrations := len(agentLogs)
	assert.GreaterOrEqual(c, tailedIntegrations, totalIntegrations*80/100, "Expected at least 80%% of integrations to be tailed, got %d/%d", tailedIntegrations, totalIntegrations)
}

func isPodReady(pod corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podReadinessSummary(pods []corev1.Pod) string {
	summaries := make([]string, 0, len(pods))
	for _, pod := range pods {
		readyStatus := "missing"
		readyReason := ""
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				readyStatus = string(condition.Status)
				readyReason = condition.Reason
				break
			}
		}

		containerSummaries := make([]string, 0, len(pod.Status.ContainerStatuses))
		for _, container := range pod.Status.ContainerStatuses {
			containerSummaries = append(containerSummaries, fmt.Sprintf("%s ready=%t restarts=%d", container.Name, container.Ready, container.RestartCount))
		}
		summaries = append(summaries, fmt.Sprintf("%s phase=%s ready=%s reason=%s containers=[%s]", pod.Name, pod.Status.Phase, readyStatus, readyReason, strings.Join(containerSummaries, "; ")))
	}
	return strings.Join(summaries, " | ")
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func truncateStatusOutput(output string) string {
	const maxOutputLength = 2000
	output = strings.TrimSpace(output)
	if len(output) <= maxOutputLength {
		return output
	}
	return output[:maxOutputLength] + "...<truncated>"
}

// isInternalTrafficPolicySupported checks if the internalTrafficPolicy field is supported in the current Kubernetes version.
// This is accomplished by checking if the Kubernetes minor version is >= 22.
func isInternalTrafficPolicySupported() bool {
	k8sVersion := common.K8sVersion
	splits := strings.Split(k8sVersion, ".")
	// Avoid panics by checking if the version is in the expected format (X.Y)
	if len(splits) < 2 {
		return false
	}
	minorVersion, err := strconv.Atoi(splits[1])
	if err != nil {
		return false
	}
	return minorVersion >= 22
}

func VerifyAgentTraces(c *assert.CollectT, collectorOutput string) {
	apmAgentJson := common.ParseCollectorJson(collectorOutput)
	// The order of services in the Agent JSON output is not guaranteed.
	// We use a map to assert that we have received traces for all expected services.
	expectedServices := map[string]bool{
		"e2e-test-apm-hostip": true,
		"e2e-test-apm-socket": true,
	}
	// On Kubernetes >= 1.22, the node Agent k8s service is created since internalTrafficPolicy is supported.
	if isInternalTrafficPolicySupported() {
		expectedServices["e2e-test-apm-agent-service"] = true
	}
	// Track found services
	foundServices := map[string]bool{}

	if apmAgentJson != nil {
		apmStatsMap, apmStatsOk := apmAgentJson["apmStats"].(map[string]any)
		if !apmStatsOk {
			assert.Fail(c, "apmStats field is not a map or is nil")
			return
		}

		receiver, receiverOk := apmStatsMap["receiver"].([]any)
		if !receiverOk {
			assert.Fail(c, "receiver field is not an array or is nil")
			return
		}

		for _, service := range receiver {
			serviceMap, serviceOk := service.(map[string]any)
			if !serviceOk {
				continue
			}

			serviceName, serviceNameOk := serviceMap["Service"].(string)
			if !serviceNameOk {
				continue
			}

			tracesReceived, tracesOk := serviceMap["TracesReceived"].(float64)
			if !tracesOk {
				continue
			}

			// Ensure we received at least one trace for the service
			assert.Greater(c, tracesReceived, float64(0), "Expected traces to be received for service %s", serviceName)
			// Mark the service as found
			foundServices[serviceName] = true
		}
	}
	assert.Equal(c, expectedServices, foundServices, "The found services do not match the expected services")
}
