// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// TestSetupControllers_StarterErrorsAreBestEffort confirms that starter
// failures are logged at ERROR by SetupControllers but never propagated up:
// one controller's misconfiguration must not bring the whole operator down
// or prevent other controllers from starting. The untaint controller follows
// this same best-effort pattern.
func TestSetupControllers_StarterErrorsAreBestEffort(t *testing.T) {
	originalStarters := controllerStarters
	t.Cleanup(func() { controllerStarters = originalStarters })

	failing := func(logr.Logger, manager.Manager, kubernetes.PlatformInfo, SetupOptions, datadog.MetricsForwardersManager) error {
		return errors.New("simulated starter failure")
	}
	controllerStarters = map[string]starterFunc{
		agentControllerName:   failing,
		untaintControllerName: failing,
	}

	assert.NoError(t, SetupControllers(
		log.Log,
		nil,
		kubernetes.PlatformInfo{},
		SetupOptions{UntaintControllerEnabled: true},
	))
}
