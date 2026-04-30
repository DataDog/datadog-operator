// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleetsuite

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	operatorPackage = "datadog-operator"

	methodStartDatadogAgentExperiment   = "operator/start_datadogagent_experiment"
	methodStopDatadogAgentExperiment    = "operator/stop_datadogagent_experiment"
	methodPromoteDatadogAgentExperiment = "operator/promote_datadogagent_experiment"

	operationUpdate = "update"
)

var datadogAgentGVK = schema.GroupVersionKind{
	Group:   "datadoghq.com",
	Version: "v2alpha1",
	Kind:    "DatadogAgent",
}

type fleetBackend struct {
	rc *fakeRCClient
}

type installerConfig struct {
	ID         string                     `json:"id"`
	Operations []fleetManagementOperation `json:"operations"`
}

type fleetManagementOperation struct {
	Operation string          `json:"operation"`
	Config    json.RawMessage `json:"config"`
}

type remoteAPIRequest struct {
	ID            string           `json:"id"`
	Package       string           `json:"package_name"`
	ExpectedState expectedState    `json:"expected_state"`
	Method        string           `json:"method"`
	Params        experimentParams `json:"params"`
}

type expectedState struct {
	StableConfig     string `json:"stable_config"`
	ExperimentConfig string `json:"experiment_config"`
}

type experimentParams struct {
	Version          string                  `json:"version"`
	GroupVersionKind schema.GroupVersionKind `json:"group_version_kind"`
	NamespacedName   types.NamespacedName    `json:"namespaced_name"`
}

func newFleetBackend(rc *fakeRCClient) *fleetBackend {
	return &fleetBackend{rc: rc}
}

func (b *fleetBackend) PutConfig(ctx context.Context, configID string, patch json.RawMessage) error {
	cfg := installerConfig{
		ID: configID,
		Operations: []fleetManagementOperation{
			{
				Operation: operationUpdate,
				Config:    patch,
			},
		},
	}
	_, err := b.rc.sendJSON(ctx, state.ProductInstallerConfig, configID, cfg)
	return err
}

func (b *fleetBackend) StartExperiment(ctx context.Context, taskID string, configID string, target types.NamespacedName, patch json.RawMessage) error {
	if err := b.PutConfig(ctx, configID, patch); err != nil {
		return err
	}
	return b.sendTask(ctx, taskID, methodStartDatadogAgentExperiment, configID, target, b.currentExpectedState())
}

func (b *fleetBackend) StopExperiment(ctx context.Context, taskID string, configID string, target types.NamespacedName) error {
	return b.sendTask(ctx, taskID, methodStopDatadogAgentExperiment, configID, target, b.currentExpectedState())
}

func (b *fleetBackend) PromoteExperiment(ctx context.Context, taskID string, configID string, target types.NamespacedName) error {
	return b.sendTask(ctx, taskID, methodPromoteDatadogAgentExperiment, configID, target, b.currentExpectedState())
}

func (b *fleetBackend) SendTaskWithExpectedState(ctx context.Context, taskID string, method string, configID string, target types.NamespacedName, expected expectedState) error {
	return b.sendTask(ctx, taskID, method, configID, target, expected)
}

func (b *fleetBackend) sendTask(ctx context.Context, taskID string, method string, configID string, target types.NamespacedName, expected expectedState) error {
	req := remoteAPIRequest{
		ID:            taskID,
		Package:       operatorPackage,
		ExpectedState: expected,
		Method:        method,
		Params: experimentParams{
			Version:          configID,
			GroupVersionKind: datadogAgentGVK,
			NamespacedName:   target,
		},
	}
	_, err := b.rc.sendJSON(ctx, state.ProductUpdaterTask, taskID, req)
	return err
}

func (b *fleetBackend) currentExpectedState() expectedState {
	pkg := b.rc.packageState(operatorPackage)
	if pkg == nil {
		return expectedState{}
	}
	return expectedState{
		StableConfig:     pkg.GetStableConfigVersion(),
		ExperimentConfig: pkg.GetExperimentConfigVersion(),
	}
}
