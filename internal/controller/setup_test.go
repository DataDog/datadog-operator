// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"errors"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

type csiMgrStub struct {
	cli client.Client
	sch *runtime.Scheme
	rec record.EventRecorder
}

func (s *csiMgrStub) GetClient() client.Client { return s.cli }

func (s *csiMgrStub) GetScheme() *runtime.Scheme { return s.sch }

func (s *csiMgrStub) GetEventRecorderFor(string) record.EventRecorder { return s.rec }

func TestNewDatadogCSIDriverReconciler_UntaintInjectCSIStartupToleration(t *testing.T) {
	s := runtime.NewScheme()
	cli := fake.NewClientBuilder().WithScheme(s).Build()
	rec := record.NewFakeRecorder(1)
	stub := &csiMgrStub{cli: cli, sch: s, rec: rec}

	for _, tc := range []struct {
		name    string
		untaint bool
		waitCSI bool
		want    bool
	}{
		{"both true", true, true, true},
		{"untaint off", false, true, false},
		{"wait CSI off", true, false, false},
		{"both off", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newDatadogCSIDriverReconciler(stub, SetupOptions{
				UntaintControllerEnabled:          tc.untaint,
				UntaintControllerWaitForCSIDriver: tc.waitCSI,
			})
			assert.Equal(t, tc.want, r.UntaintInjectCSIStartupToleration)
		})
	}
}

func TestStartUntaint_NewUntaintReconcilerErrorIsWrapped(t *testing.T) {
	ctrl.SetLogger(logr.Discard())
	t.Setenv(EnvTimeoutPolicy, "unknown")
	t.Cleanup(func() { _ = os.Unsetenv(EnvTimeoutPolicy) })

	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	mgr, err := ctrl.NewManager(&rest.Config{}, manager.Options{Scheme: s, LeaderElection: false})
	require.NoError(t, err)

	err = startUntaint(logr.Discard(), mgr, kubernetes.PlatformInfo{}, SetupOptions{UntaintControllerEnabled: true}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "untaint controller setup")
}
