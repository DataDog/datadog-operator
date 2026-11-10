// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/pkg/config"
)

type testCreds struct{}

func (testCreds) GetCredentialsForMetadata() (config.Creds, error) { return config.Creds{}, nil }

func TestNewFlareRunner(t *testing.T) {
	opts := &options{metricsAddr: ":8383", datadogAgentEnabled: true, remoteConfigEnabled: true, enableLeaderElection: true}
	runner, err := newFlareRunner(opts, &rest.Config{Host: "http://127.0.0.1:1"}, fake.NewClientBuilder().Build(), testCreds{}, func() bool { return true }, func() any { return nil })
	require.NoError(t, err)
	require.NotNil(t, runner)

	_, err = newFlareRunner(opts, &rest.Config{Host: "http://127.0.0.1:1", Username: "u", BearerToken: "t"}, fake.NewClientBuilder().Build(), testCreds{}, nil, nil)
	assert.Error(t, err, "basic and bearer auth together are rejected")
}

func TestFlareFeatures(t *testing.T) {
	opts := &options{datadogAgentEnabled: true, datadogMonitorEnabled: false, remoteConfigEnabled: true, secureMetrics: true}
	features := opts.flareFeatures()
	assert.True(t, features["datadogAgent"])
	assert.False(t, features["datadogMonitor"])
	assert.True(t, features["remoteConfig"])
	assert.True(t, features["secureMetrics"])
	assert.Len(t, features, 25)
}
