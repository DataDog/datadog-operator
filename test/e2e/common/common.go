// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func ParseCollectorJson(collectorOutput string) map[string]any {
	for _, line := range strings.Split(collectorOutput, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}

		var jsonObject map[string]any
		decoder := json.NewDecoder(strings.NewReader(line))
		if err := decoder.Decode(&jsonObject); err != nil {
			continue
		}

		if isAgentStatusJSON(jsonObject) {
			return jsonObject
		}
	}

	return map[string]any{}
}

func isAgentStatusJSON(jsonObject map[string]any) bool {
	for _, key := range []string{
		"runnerStats",
		"logsStats",
		"apmStats",
		"autoConfigStats",
		"checkSchedulerStats",
	} {
		if _, ok := jsonObject[key]; ok {
			return true
		}
	}

	return false
}

func projectRoot() string {
	_, b, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Join(filepath.Dir(b), "../../..")
	}
	return ""
}
