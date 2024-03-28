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
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	manifestsPath       = "./manifests"
	mgrKustomizeDirPath = "../../config/default"
	imagePullSecretName = "registry-credentials"
)

var (
	k8sVersion      = getEnv("K8S_VERSION", "1.26")
	imageTag        = getEnv("TARGET_IMAGE", "gcr.io/datadoghq/operator:latest")
	imgPullPassword = getEnv("IMAGE_PULL_PASSWORD", "")
	tmpDir          string
	kubeConfigPath  string
	kubectlOptions  *k8s.KubectlOptions

	ddaMinimalPath = filepath.Join(manifestsPath, "datadog-agent-minimum.yaml")

	namespaceName = "system"
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

func operatorTransformationFunc() func(state map[string]interface{}, opts ...pulumi.ResourceOption) {
	return func(state map[string]interface{}, opts ...pulumi.ResourceOption) {
		name := state["metadata"].(map[string]interface{})["name"]

		if imageTag != "" && state["kind"] == "Deployment" && name == "datadog-operator-manager" {
			template := state["spec"].(map[string]interface{})["template"]
			templateSpec := template.(map[string]interface{})["spec"]
			templateSpec.(map[string]interface{})["imagePullSecrets"] = []map[string]interface{}{{"name": imagePullSecretName}}
			containers := templateSpec.(map[string]interface{})["containers"]
			container := containers.([]interface{})[0]
			container.(map[string]interface{})["image"] = imageTag

		}
	}
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
