// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	var runningChecks map[string]interface{}

	checksJson := common.ParseCollectorJson(collectorOutput)
	if checksJson != nil {
		runnerStats, ok := checksJson["runnerStats"].(map[string]interface{})
		assert.True(c, ok)
		assert.NotNil(c, runnerStats)

		runningChecks, ok = runnerStats["Checks"].(map[string]interface{})
		assert.True(c, ok)
		assert.NotNil(c, runningChecks)

		if check, found := runningChecks[checkName].(map[string]interface{}); found {
			for _, instance := range check {
				assert.Equal(c, checkName, instance.(map[string]interface{})["CheckName"].(string))

				lastError, exists := instance.(map[string]interface{})["LastError"].(string)
				assert.True(c, exists)
				assert.Empty(c, lastError)

				totalErrors, exists := instance.(map[string]interface{})["TotalErrors"].(float64)
				assert.True(c, exists)
				assert.Zero(c, totalErrors)

				totalMetricSamples, exists := instance.(map[string]interface{})["TotalMetricSamples"].(float64)
				assert.True(c, exists)
				assert.Greater(c, totalMetricSamples, float64(0))
			}
		} else {
			assert.True(c, found, "Check %s not found or not yet running.", checkName)
		}
	}
}

func VerifyAgentPodLogs(c *assert.CollectT, collectorOutput string) {
	var agentLogs []interface{}
	logsJson := common.ParseCollectorJson(collectorOutput)

	tailedIntegrations := 0
	if logsJson != nil {
		var ok bool
		agentLogs, ok = logsJson["logsStats"].(map[string]interface{})["integrations"].([]interface{})
		assert.True(c, ok)
		assert.NotEmpty(c, agentLogs)
		for _, log := range agentLogs {
			sources, sourcesOk := log.(map[string]interface{})["sources"].([]interface{})
			if !sourcesOk || len(sources) == 0 {
				continue
			}

			if integration, integrationOk := sources[0].(map[string]interface{}); integrationOk {
				messages, exists := integration["messages"].([]interface{})
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
	}
	totalIntegrations := len(agentLogs)
	assert.GreaterOrEqual(c, tailedIntegrations, totalIntegrations*80/100, "Expected at least 80%% of integrations to be tailed, got %d/%d", tailedIntegrations, totalIntegrations)
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
		apmStats := apmAgentJson["apmStats"].(map[string]interface{})["receiver"].([]interface{})
		for _, service := range apmStats {
			serviceName := service.(map[string]interface{})["Service"].(string)
			tracesReceived := service.(map[string]interface{})["TracesReceived"].(float64)
			// Ensure we received at least one trace for the service
			assert.Greater(c, tracesReceived, float64(0), "Expected traces to be received for service %s", serviceName)
			// Mark the service as found
			foundServices[serviceName] = true
		}
	}
	assert.Equal(c, expectedServices, foundServices, "The found services do not match the expected services")
}
