package utils

import (
	"fmt"
	"github.com/DataDog/datadog-operator/test/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var timeout int64 = 60

func VerifyOperator(t *testing.T, kubectlOptions *k8s.KubectlOptions) {
	VerifyNumPodsForSelector(t, kubectlOptions, 1, "app.kubernetes.io/name=datadog-operator")
}

func VerifyNumPodsForSelector(t *testing.T, kubectlOptions *k8s.KubectlOptions, numPods int, selector string) {
	t.Log("Waiting for number of pods created", "number", numPods, "selector", selector)
	k8s.WaitUntilNumPodsCreated(t, kubectlOptions, metav1.ListOptions{
		LabelSelector:  selector,
		FieldSelector:  "status.phase=Running",
		TimeoutSeconds: &timeout,
	}, numPods, 9, 15*time.Second)
}

func VerifyAgentPods(t *testing.T, kubectlOptions *k8s.KubectlOptions, selector string) {
	k8s.WaitUntilAllNodesReady(t, kubectlOptions, 9, 15*time.Second)
	nodes := k8s.GetNodes(t, kubectlOptions)
	VerifyNumPodsForSelector(t, kubectlOptions, len(nodes), selector)
}

func VerifyCheck(c *assert.CollectT, collectorOutput string, checkName string) {
	var runningChecks map[string]interface{}

	checksJson := common.ParseCollectorJson(collectorOutput)
	if checksJson != nil {
		runningChecks = checksJson["runnerStats"].(map[string]interface{})["Checks"].(map[string]interface{})
		if check, found := runningChecks[checkName].(map[string]interface{}); found {
			for _, instance := range check {
				assert.EqualValues(c, checkName, instance.(map[string]interface{})["CheckName"].(string))

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
			assert.True(c, found, fmt.Sprintf("Check %s not found or not yet running.", checkName))
		}
	}
}

func VerifyAgentPodLogs(c *assert.CollectT, collectorOutput string) {
	var agentLogs []interface{}
	logsJson := common.ParseCollectorJson(collectorOutput)

	tailedIntegrations := 0
	if logsJson != nil {
		agentLogs = logsJson["logsStats"].(map[string]interface{})["integrations"].([]interface{})
		for _, log := range agentLogs {
			if integration, ok := log.(map[string]interface{})["sources"].([]interface{})[0].(map[string]interface{}); ok {
				message, exists := integration["messages"].([]interface{})[0].(string)
				if exists && len(message) > 0 {
					num, _ := strconv.Atoi(string(message[0]))
					if num > 0 && strings.Contains(message, "files tailed") {
						tailedIntegrations++
					}
				}
			} else {
				assert.True(c, ok, "Failed to get sources from logs. Possible causes: missing 'sources' field, empty array, or incorrect data format.")
			}
		}
	}
	totalIntegrations := len(agentLogs)
	assert.True(c, tailedIntegrations >= totalIntegrations*80/100, "Expected at least 80%% of integrations to be tailed, got %d/%d", tailedIntegrations, totalIntegrations)
}

func ContextConfig(kubeConfig string) (cleanupFunc func(), err error) {
	tmpDir := "/tmp"
	KubeConfigPath := filepath.Join(tmpDir, ".kubeconfig")

	kcFile, err := os.Create(KubeConfigPath)
	if err != nil {
		return nil, err
	}
	defer kcFile.Close()

	_, err = kcFile.WriteString(kubeConfig)
	return func() {
		_ = os.Remove(KubeConfigPath)
	}, nil
}

func DeleteDda(t *testing.T, kubectlOptions *k8s.KubectlOptions, ddaPath string) {
	if !*e2e.KeepStacks {
		k8s.KubectlDelete(t, kubectlOptions, ddaPath)
	}
}
