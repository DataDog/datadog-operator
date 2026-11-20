// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// FlareState is the state of the Remote Configuration client, as written to
// Operator flares. It holds no credentials.
type FlareState struct {
	Started        bool                `json:"started"`
	ClientID       string              `json:"clientID,omitempty"`
	Site           string              `json:"site,omitempty"`
	Endpoint       string              `json:"endpoint,omitempty"`
	ClusterName    string              `json:"clusterName,omitempty"`
	UpdaterTags    []string            `json:"updaterTags,omitempty"`
	Products       []string            `json:"products"`
	Subscriptions  []string            `json:"subscriptions,omitempty"`
	RemoteLogLevel string              `json:"remoteLogLevel,omitempty"`
	Packages       []FlarePackageState `json:"packages,omitempty"`
	AgentTasks     []FlareTaskState    `json:"agentTasks,omitempty"`
}

// FlarePackageState is a package reported to Fleet Automation.
type FlarePackageState struct {
	Package             string `json:"package"`
	StableVersion       string `json:"stableVersion,omitempty"`
	ExperimentVersion   string `json:"experimentVersion,omitempty"`
	StableConfigVersion string `json:"stableConfigVersion,omitempty"`
}

// FlareTaskState is the status of an agent task.
type FlareTaskState struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// FlareState returns the state of the Remote Configuration client.
func (r *RemoteConfigUpdater) FlareState() FlareState {
	st := FlareState{
		Products: []string{string(state.ProductAgentConfig), string(state.ProductOrchestratorK8sCRDs), string(state.ProductAgentTask)},
	}

	r.clientMu.RLock()
	if r.rcClient != nil {
		st.Started = true
		st.ClientID = r.rcClient.ID
		// serviceConf is set before the client starts.
		if r.serviceConf.cfg != nil {
			st.Site = r.serviceConf.cfg.GetString("site")
		}
		st.Endpoint = r.serviceConf.baseRawURL
		st.ClusterName = r.serviceConf.clusterName
		for _, pkg := range r.rcClient.GetInstallerState().GetPackages() {
			st.Packages = append(st.Packages, FlarePackageState{
				Package:             pkg.GetPackage(),
				StableVersion:       pkg.GetStableVersion(),
				ExperimentVersion:   pkg.GetExperimentVersion(),
				StableConfigVersion: pkg.GetStableConfigVersion(),
			})
		}
	}
	st.UpdaterTags = append([]string(nil), r.updaterTags...)
	for _, subscription := range r.subscriptions {
		st.Subscriptions = append(st.Subscriptions, subscription.product)
	}
	r.clientMu.RUnlock()

	r.logLevelMu.RLock()
	st.RemoteLogLevel = r.remoteLogLevel
	r.logLevelMu.RUnlock()

	r.tasksMu.Lock()
	for id, status := range r.taskStatuses {
		st.AgentTasks = append(st.AgentTasks, FlareTaskState{ID: id, State: applyStateName(status.State), Error: status.Error})
	}
	r.tasksMu.Unlock()
	sort.Slice(st.AgentTasks, func(i, j int) bool { return st.AgentTasks[i].ID < st.AgentTasks[j].ID })
	return st
}

func applyStateName(s state.ApplyState) string {
	switch s {
	case state.ApplyStateUnacknowledged:
		return "unacknowledged"
	case state.ApplyStateAcknowledged:
		return "acknowledged"
	case state.ApplyStateError:
		return "error"
	default:
		return "unknown"
	}
}
