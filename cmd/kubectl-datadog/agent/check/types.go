// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

// AgentStatus represents an agent status
type AgentStatus struct {
	RunnerStats RunnerStats `json:"runnerStats"`
}

// RunnerStats holds check runner stats
type RunnerStats struct {
	Checks map[string]map[string]Stats `json:"Checks"`
}

// Stats holds check stats
type Stats struct {
	LastError string `json:"LastError"`
}

// Error represents LastError when not empty
type Error struct {
	Message string `json:"message"`
}
