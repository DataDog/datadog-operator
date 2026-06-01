// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
	jsonObject, _ := ParseCollectorJsonWithDiagnostics(collectorOutput)
	return jsonObject
}

func ParseCollectorJsonWithDiagnostics(collectorOutput string) (map[string]any, string) {
	candidateLines := []string{}
	invalidJSON := []string{}
	nonStatusJSON := []string{}

	for lineNumber, line := range strings.Split(collectorOutput, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		candidateLines = appendDiagnostic(candidateLines, fmt.Sprintf("%d", lineNumber+1))

		var jsonObject map[string]any
		decoder := json.NewDecoder(strings.NewReader(line))
		if err := decoder.Decode(&jsonObject); err != nil {
			invalidJSON = appendDiagnostic(invalidJSON, fmt.Sprintf("line %d: %v", lineNumber+1, err))
			continue
		}

		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			invalidJSON = appendDiagnostic(invalidJSON, fmt.Sprintf("line %d: extra data after JSON object", lineNumber+1))
			continue
		}

		if isAgentStatusJSON(jsonObject) {
			return jsonObject, ""
		}

		nonStatusJSON = appendDiagnostic(nonStatusJSON, fmt.Sprintf("line %d keys=%v", lineNumber+1, sortedMapKeys(jsonObject)))
	}

	return map[string]any{}, fmt.Sprintf("no Agent status JSON found; candidate_lines=%v invalid_json=%v non_status_json=%v", candidateLines, invalidJSON, nonStatusJSON)
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

func appendDiagnostic(items []string, item string) []string {
	const maxDiagnostics = 5
	if len(items) < maxDiagnostics {
		return append(items, item)
	}
	if len(items) == maxDiagnostics {
		return append(items, "...")
	}
	return items
}

func sortedMapKeys(jsonObject map[string]any) []string {
	keys := make([]string, 0, len(jsonObject))
	for key := range jsonObject {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func projectRoot() string {
	_, b, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Join(filepath.Dir(b), "../../..")
	}
	return ""
}
