// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"os"
	"path/filepath"
	"runtime"
)

var (
	NamespaceName     = "e2e-operator"
	K8sVersion        = GetEnv("K8S_VERSION", "1.26")
	ImgPullPassword   = GetEnv("IMAGE_PULL_PASSWORD", "")
	OperatorImageName = GetEnv("IMG", "")

	DdaMinimalPath = filepath.Join(ManifestsPath, "datadog-agent-minimum.yaml")
	ManifestsPath  = filepath.Join(ProjectRootPath, "test/e2e/manifests")

	ProjectRootPath = projectRoot()
)

const (
	NodeAgentSelector          = "agent.datadoghq.com/component=agent"
	ClusterAgentSelector       = "agent.datadoghq.com/component=cluster-agent"
	ClusterCheckRunnerSelector = "agent.datadoghq.com/component=cluster-checks-runner"
)

// GetAbsPath Return absolute path for given path
func GetAbsPath(path string) (string, error) {
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

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func projectRoot() string {
	_, b, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Join(filepath.Dir(b), "../../..")
	}
	return ""
}
