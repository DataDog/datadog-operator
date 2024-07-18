// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	manifestsPath       = "./manifests"
	mgrKustomizeDirPath = "../../config/e2e"
)

var (
	namespaceName   = "system"
	k8sVersion      = getEnv("K8S_VERSION", "1.26")
	imgPullPassword = getEnv("IMAGE_PULL_PASSWORD", "")

	kubeConfigPath string
	kubectlOptions *k8s.KubectlOptions

	tmpDir         string
	ddaMinimalPath = filepath.Join(manifestsPath, "datadog-agent-minimum.yaml")
)

// getAbsPath Return absolute path for given path
func getAbsPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	_, err = os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if os.IsNotExist(err) {
		return "", err
	}

	return absPath, nil
}

func contextConfig(kubeConfig string) (cleanupFunc func(), err error) {
	tmpDir = "/tmp"
	kubeConfigPath = filepath.Join(tmpDir, ".kubeconfig")

	kcFile, err := os.Create(kubeConfigPath)
	if err != nil {
		return nil, err
	}
	defer kcFile.Close()

	_, err = kcFile.WriteString(kubeConfig)
	return func() {
		_ = os.Remove(kubeConfigPath)
	}, nil
}

func verifyOperator(t *testing.T, kubectlOptions *k8s.KubectlOptions) {
	verifyNumPodsForSelector(t, kubectlOptions, 1, "app.kubernetes.io/name=datadog-operator")
}

func verifyAgent(t *testing.T, kubectlOptions *k8s.KubectlOptions) {
	k8s.WaitUntilAllNodesReady(t, kubectlOptions, 9, 15*time.Second)
	nodes := k8s.GetNodes(t, kubectlOptions)

	verifyNumPodsForSelector(t, kubectlOptions, len(nodes), "agent.datadoghq.com/component=agent")
	verifyNumPodsForSelector(t, kubectlOptions, 1, "agent.datadoghq.com/component=cluster-agent")
	verifyNumPodsForSelector(t, kubectlOptions, 1, "agent.datadoghq.com/component=cluster-checks-runner")
}

func verifyNumPodsForSelector(t *testing.T, kubectlOptions *k8s.KubectlOptions, numPods int, selector string) {
	t.Log("Waiting for number of pods created", "number", numPods, "selector", selector)
	k8s.WaitUntilNumPodsCreated(t, kubectlOptions, v1.ListOptions{
		LabelSelector: selector,
	}, numPods, 9, 15*time.Second)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func deleteDda(t *testing.T, kubectlOptions *k8s.KubectlOptions, ddaPath string) {
	if !*keepStacks {
		k8s.KubectlDelete(t, kubectlOptions, ddaPath)
	}
}
