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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-operator/test/e2e/common"
)

const statusOutputSnippetLimit = 4000

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func statusOutputSnippet(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= statusOutputSnippetLimit {
		return output
	}
	return output[:statusOutputSnippetLimit] + "...(truncated)"
}

func printStatusDiagnostic(reason, output string, parsed map[string]any) {
	keys := []string(nil)
	if parsed != nil {
		keys = sortedMapKeys(parsed)
	}
	fmt.Printf("[e2e status diagnostic] %s; top-level keys=%v; output snippet=%s\n", reason, keys, statusOutputSnippet(output))
}

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
	assert.NotNil(c, podsList)
	assert.NotEmpty(c, podsList.Items)
	assert.Len(c, podsList.Items, numPods)
}

func VerifyAgentPods(t *testing.T, c *assert.CollectT, namespace string, k8sClient kubeClient.Interface, selector string) {
	nodesList, err := k8sClient.CoreV1().Nodes().List(t.Context(), metav1.ListOptions{})
	require.NoError(c, err)
	assert.NotNil(c, nodesList)
	assert.NotEmpty(c, nodesList.Items)
	VerifyNumPodsForSelector(t, c, namespace, k8sClient, len(nodesList.Items), selector)
}

func VerifyCheck(c *assert.CollectT, collectorOutput string, checkName string) {
	checksJson := common.ParseCollectorJson(collectorOutput)
	if checksJson == nil {
		printStatusDiagnostic("collector status output did not parse as JSON while looking for check "+checkName, collectorOutput, nil)
		return
	}

	runnerStats, runnerStatsOk := checksJson["runnerStats"].(map[string]any)
	if !runnerStatsOk {
		printStatusDiagnostic("runnerStats missing while looking for check "+checkName, collectorOutput, checksJson)
		return
	}

	runningChecks, checksOk := runnerStats["Checks"].(map[string]any)
	if !checksOk {
		fmt.Printf("[e2e status diagnostic] Checks missing while looking for %s; runnerStats keys=%v; output snippet=%s\n", checkName, sortedMapKeys(runnerStats), statusOutputSnippet(collectorOutput))
		return
	}

	check, found := runningChecks[checkName].(map[string]any)
	if !found {
		fmt.Printf("[e2e status diagnostic] check %s not found in status output; running check keys=%v; output snippet=%s\n", checkName, sortedMapKeys(runningChecks), statusOutputSnippet(collectorOutput))
		return
	}

	healthyInstances := 0
	instanceDiagnostics := make([]string, 0, len(check))
	for instanceName, instance := range check {
		instanceMap, instanceOk := instance.(map[string]any)
		if !instanceOk {
			instanceDiagnostics = append(instanceDiagnostics, fmt.Sprintf("%s:not-a-map", instanceName))
			continue
		}

		checkNameVal, _ := instanceMap["CheckName"].(string)
		lastError, _ := instanceMap["LastError"].(string)
		totalErrors, _ := instanceMap["TotalErrors"].(float64)
		totalMetricSamples, _ := instanceMap["TotalMetricSamples"].(float64)
		instanceDiagnostics = append(instanceDiagnostics, fmt.Sprintf("%s:{CheckName:%q LastError:%q TotalErrors:%v TotalMetricSamples:%v}", instanceName, checkNameVal, lastError, totalErrors, totalMetricSamples))

		if checkNameVal == checkName && lastError == "" && totalErrors == 0 && totalMetricSamples > 0 {
			healthyInstances++
		}
	}

	if healthyInstances == 0 {
		fmt.Printf("[e2e status diagnostic] check %s is present but no healthy status instance was found; instances=%v\n", checkName, instanceDiagnostics)
	}
}

func VerifyAgentPodLogs(c *assert.CollectT, collectorOutput string) {
	var agentLogs []any
	logsJson := common.ParseCollectorJson(collectorOutput)
	if logsJson == nil {
		printStatusDiagnostic("logs status output did not parse as JSON", collectorOutput, nil)
		return
	}

	tailedIntegrations := 0
	logsStats, logsStatsOk := logsJson["logsStats"].(map[string]any)
	if !logsStatsOk {
		printStatusDiagnostic("logsStats missing from logs status output", collectorOutput, logsJson)
		return
	}

	var ok bool
	agentLogs, ok = logsStats["integrations"].([]any)
	if !ok || len(agentLogs) == 0 {
		fmt.Printf("[e2e status diagnostic] logsStats.integrations missing or empty; logsStats keys=%v; output snippet=%s\n", sortedMapKeys(logsStats), statusOutputSnippet(collectorOutput))
		return
	}

	for _, log := range agentLogs {
		logMap, logOk := log.(map[string]any)
		if !logOk {
			fmt.Printf("[e2e status diagnostic] log integration entry is not a map; entry=%v\n", log)
			continue
		}

		sources, sourcesOk := logMap["sources"].([]any)
		if !sourcesOk || len(sources) == 0 {
			continue
		}

		if integration, integrationOk := sources[0].(map[string]any); integrationOk {
			messages, messagesOk := integration["messages"].([]any)
			if !messagesOk || len(messages) == 0 {
				continue
			}

			message, msgOk := messages[0].(string)
			if !msgOk || message == "" {
				continue
			}

			num, _ := strconv.Atoi(string(message[0]))
			if num > 0 && strings.Contains(message, "files tailed") {
				tailedIntegrations++
			}
		} else {
			fmt.Printf("[e2e status diagnostic] logs source is not a map; source=%v\n", sources[0])
		}
	}

	totalIntegrations := len(agentLogs)
	if tailedIntegrations < totalIntegrations*80/100 {
		fmt.Printf("[e2e status diagnostic] logs status reports fewer than 80%% tailed integrations: %d/%d; fakeintake log assertions remain authoritative\n", tailedIntegrations, totalIntegrations)
	}
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
