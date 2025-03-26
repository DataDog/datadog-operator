// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/yaml"
)

const (
	manifestsPath       = "./manifests"
	mgrKustomizeDirPath = "../../config/e2e"
	UserData            = `#!/bin/bash
echo "User Data"
echo "Installing kubectl"
snap install kubectl --classic

echo "Verifying kubectl"
kubectl version --client

echo "Installing kubens"
curl -sLo kubens https://github.com/ahmetb/kubectx/releases/download/v0.9.5/kubens
chmod +x kubens
mv kubens /usr/local/bin/

echo '

alias k="kubectl"
alias kg="kubectl get"
alias kgp="kubectl get pod"
alias krm="kubectl delete"
alias krmp="kubectl delete pod"
alias kd="kubectl describe"
alias kdp="kubectl describe pod"
alias ke="kubectl edit"
alias kl="kubectl logs"
alias kx="kubectl exec"
' >> /home/ubuntu/.bashrc
`
	defaultMgrImageName = "gcr.io/datadoghq/operator"
	defaultMgrImgTag    = "latest"
	nodeAgentSelector   = "agent.datadoghq.com/component=agent"
)

var (
	namespaceName   = "e2e-operator"
	k8sVersion      = getEnv("K8S_VERSION", "1.26")
	imgPullPassword = getEnv("IMAGE_PULL_PASSWORD", "")

	kubeConfigPath string

	tmpDir string
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

func verifyAgentPods(t *testing.T, kubectlOptions *k8s.KubectlOptions, selector string) {
	k8s.WaitUntilAllNodesReady(t, kubectlOptions, 9, 15*time.Second)
	nodes := k8s.GetNodes(t, kubectlOptions)
	verifyNumPodsForSelector(t, kubectlOptions, len(nodes), selector)
}

func verifyNumPodsForSelector(t *testing.T, kubectlOptions *k8s.KubectlOptions, numPods int, selector string) {
	t.Log("Waiting for number of pods created", "number", numPods, "selector", selector)
	k8s.WaitUntilNumPodsCreated(t, kubectlOptions, v1.ListOptions{
		LabelSelector: selector,
		FieldSelector: "status.phase=Running",
	}, numPods, 9, 15*time.Second)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func deleteDda(t *testing.T, kubectlOptions *k8s.KubectlOptions, ddaPath string) {
	if !*KeepStacks {
		k8s.KubectlDelete(t, kubectlOptions, ddaPath)
	}
}

func loadKustomization(path string) (*types.Kustomization, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var kustomization types.Kustomization
	if err := yaml.Unmarshal(data, &kustomization); err != nil {
		return nil, err
	}

	return &kustomization, nil
}

func saveKustomization(path string, kustomization *types.Kustomization) error {
	data, err := yaml.Marshal(kustomization)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	return nil
}

// updateKustomization Updates kustomization.yaml file in given kustomize directory with extra resources and image name and tag if `IMG` environment variable is set.
func updateKustomization(kustomizeDirPath string, kustomizeResourcePaths []string) error {
	var imgName, imgTag string

	kustomizationFilePath := fmt.Sprintf("%s/kustomization.yaml", kustomizeDirPath)
	k, err := loadKustomization(kustomizationFilePath)
	if err != nil {
		return err
	}

	// Update resources with target e2e-manager resource yaml
	if kustomizeResourcePaths != nil {
		// We empty slice to avoid accumulating patches from previous tests
		k.Patches = k.Patches[:0]
		for _, res := range kustomizeResourcePaths {
			k.Patches = append(k.Patches, types.Patch{
				Path: res,
				Target: &types.Selector{
					ResId: resid.NewResIdKindOnly("Deployment", "manager"),
				},
			})
		}
	}

	// Update image
	if os.Getenv("IMG") != "" {
		imgName, imgTag = splitImageString(os.Getenv("IMG"))
	} else {
		imgName = defaultMgrImageName
		imgTag = defaultMgrImgTag
	}
	for i, img := range k.Images {
		if img.Name == "controller" {
			k.Images[i].NewName = imgName
			k.Images[i].NewTag = imgTag
		}
	}

	if err := saveKustomization(kustomizationFilePath, k); err != nil {
		return err
	}

	return nil
}

func parseCollectorJson(collectorOutput string) map[string]interface{} {
	var jsonString string
	var jsonObject map[string]interface{}

	re := regexp.MustCompile(`(\{.*\})`)
	match := re.FindStringSubmatch(collectorOutput)
	if len(match) > 0 {
		jsonString = match[0]
	} else {
		return map[string]interface{}{}
	}

	// Parse collector JSON
	err := json.Unmarshal([]byte(jsonString), &jsonObject)
	if err != nil {
		return map[string]interface{}{}
	}
	return jsonObject
}

func splitImageString(in string) (name string, tag string) {
	imageSplit := strings.Split(in, ":")
	if len(imageSplit) > 0 {
		name = imageSplit[0]
	}
	if len(imageSplit) > 1 {
		tag = imageSplit[1]
	}
	return name, tag
}
