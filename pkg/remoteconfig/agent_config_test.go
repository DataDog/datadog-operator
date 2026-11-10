// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	testOrderPath    = "datadog/2/AGENT_CONFIG/configuration_order/config"
	testLogLevelPath = "datadog/2/AGENT_CONFIG/flare-log-level-abc/config"
	testFeaturesPath = "datadog/2/AGENT_CONFIG/features/config"
)

type fakeLogLevelSetter struct {
	err    error
	levels []string
	resets int
}

func (f *fakeLogLevelSetter) SetLogLevel(level string) error {
	if f.err != nil {
		return f.err
	}
	f.levels = append(f.levels, level)
	return nil
}

func (f *fakeLogLevelSetter) ResetLogLevel() {
	f.resets++
}

func newDatadogAgentFakeClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithStatusSubresource(&v2alpha1.DatadogAgent{}).
		Build()
}

func agentConfigRaw(t *testing.T, id string, content any) state.RawConfig {
	t.Helper()
	raw, err := json.Marshal(content)
	require.NoError(t, err)
	return state.RawConfig{Config: raw, Metadata: state.Metadata{Product: state.ProductAgentConfig, ID: id}}
}

func orderConfig(t *testing.T, order, internalOrder []string) state.RawConfig {
	return agentConfigRaw(t, "configuration_order", map[string][]string{"order": order, "internal_order": internalOrder})
}

func logLevelConfig(t *testing.T, id, level string) state.RawConfig {
	return agentConfigRaw(t, id, map[string]any{"name": id, "config": map[string]string{"log_level": level}})
}

func sbomConfig(t *testing.T, id string) state.RawConfig {
	return agentConfigRaw(t, id, map[string]any{"name": id, "config": map[string]any{"sbom": map[string]any{"enabled": true}}})
}

// flareLogLevelUpdate is what Fleet Automation sends before a flare: a
// flare-log-level config and the configuration order that lists it.
func flareLogLevelUpdate(t *testing.T, level string) map[string]state.RawConfig {
	return map[string]state.RawConfig{
		testOrderPath:    orderConfig(t, nil, []string{"flare-log-level-abc"}),
		testLogLevelPath: logLevelConfig(t, "flare-log-level-abc", level),
	}
}

func TestAgentConfigLogLevelSetAndRevert(t *testing.T) {
	setter := &fakeLogLevelSetter{}
	updater := NewRemoteConfigUpdater(newDatadogAgentFakeClient(t), logr.Discard(), WithLogLevelSetter(setter))

	// No DatadogAgent exists: the log level must still be applied and
	// reported without waiting for one.
	recorder := newApplyStatusRecorder()
	updater.agentConfigUpdateCallback(flareLogLevelUpdate(t, "debug"), recorder.record)
	assert.Equal(t, []string{"debug"}, setter.levels)
	assert.Equal(t, state.ApplyStatus{State: state.ApplyStateAcknowledged}, recorder.last(testLogLevelPath))
	assert.Equal(t, state.ApplyStatus{State: state.ApplyStateAcknowledged}, recorder.last(testOrderPath))

	// The same configs again don't set the level again.
	updater.agentConfigUpdateCallback(flareLogLevelUpdate(t, "debug"), newApplyStatusRecorder().record)
	assert.Equal(t, []string{"debug"}, setter.levels)
	assert.Zero(t, setter.resets)

	// Fleet Automation removes the config when the flare log level expires.
	updater.agentConfigUpdateCallback(map[string]state.RawConfig{}, newApplyStatusRecorder().record)
	assert.Equal(t, 1, setter.resets)

	// Nothing to revert any more.
	updater.agentConfigUpdateCallback(map[string]state.RawConfig{}, newApplyStatusRecorder().record)
	assert.Equal(t, 1, setter.resets)
}

func TestAgentConfigLogLevelErrors(t *testing.T) {
	tests := []struct {
		name    string
		setter  LogLevelSetter
		wantErr string
	}{
		{
			name:    "log level can't change at runtime",
			wantErr: "changing the log level at runtime is not supported",
		},
		{
			name:    "invalid log level",
			setter:  &fakeLogLevelSetter{err: errors.New(`unsupported log level "loud"`)},
			wantErr: `unsupported log level "loud"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var options []RemoteConfigUpdaterOption
			if test.setter != nil {
				options = append(options, WithLogLevelSetter(test.setter))
			}
			updater := NewRemoteConfigUpdater(newDatadogAgentFakeClient(t), logr.Discard(), options...)
			recorder := newApplyStatusRecorder()

			updater.agentConfigUpdateCallback(flareLogLevelUpdate(t, "loud"), recorder.record)

			for _, path := range []string{testLogLevelPath, testOrderPath} {
				status := recorder.last(path)
				assert.Equal(t, state.ApplyStateError, status.State, path)
				assert.Contains(t, status.Error, test.wantErr, path)
			}
		})
	}
}

func TestAgentConfigFeaturesAndLogLevel(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default"}}
	kubeClient := newDatadogAgentFakeClient(t, dda)
	setter := &fakeLogLevelSetter{}
	updater := NewRemoteConfigUpdater(kubeClient, logr.Discard(), WithLogLevelSetter(setter))
	recorder := newApplyStatusRecorder()

	updater.agentConfigUpdateCallback(map[string]state.RawConfig{
		testOrderPath:    orderConfig(t, []string{"features"}, []string{"flare-log-level-abc"}),
		testFeaturesPath: sbomConfig(t, "features"),
		testLogLevelPath: logLevelConfig(t, "flare-log-level-abc", "debug"),
	}, recorder.record)

	for _, path := range []string{testOrderPath, testFeaturesPath, testLogLevelPath} {
		assert.Equal(t, state.ApplyStatus{State: state.ApplyStateAcknowledged}, recorder.last(path), path)
	}
	assert.Equal(t, []string{"debug"}, setter.levels)

	got := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(context.Background(), client.ObjectKeyFromObject(dda), got))
	require.NotNil(t, got.Status.RemoteConfigConfiguration)
	require.NotNil(t, got.Status.RemoteConfigConfiguration.Features.SBOM)
	assert.Equal(t, ptr.To(true), got.Status.RemoteConfigConfiguration.Features.SBOM.Enabled)

	// Removing the features resets the status; removing the log level
	// restores the startup level.
	recorder = newApplyStatusRecorder()
	updater.agentConfigUpdateCallback(map[string]state.RawConfig{
		testOrderPath: orderConfig(t, nil, nil),
	}, recorder.record)
	assert.Equal(t, state.ApplyStatus{State: state.ApplyStateAcknowledged}, recorder.last(testOrderPath))
	assert.Equal(t, 1, setter.resets)
	require.NoError(t, kubeClient.Get(context.Background(), client.ObjectKeyFromObject(dda), got))
	assert.Nil(t, got.Status.RemoteConfigConfiguration.Features.SBOM)
}

func TestMergeLogLevel(t *testing.T) {
	configs := map[string]DatadogAgentRemoteConfig{
		"a":        {CoreAgent: &CoreAgentFeaturesConfig{LogLevel: "info"}},
		"b":        {CoreAgent: &CoreAgentFeaturesConfig{LogLevel: "warn"}},
		"no-level": {CoreAgent: &CoreAgentFeaturesConfig{SBOM: &SbomConfig{}}},
		"flare":    {CoreAgent: &CoreAgentFeaturesConfig{LogLevel: "debug"}},
	}
	tests := []struct {
		name  string
		order agentConfigOrder
		want  string
	}{
		{name: "no order", want: ""},
		{name: "first in order wins", order: agentConfigOrder{Order: []string{"a", "b"}}, want: "info"},
		{name: "layer without level doesn't override", order: agentConfigOrder{Order: []string{"no-level", "b"}}, want: "warn"},
		{name: "internal order wins", order: agentConfigOrder{Order: []string{"a"}, InternalOrder: []string{"flare"}}, want: "debug"},
		{name: "unknown ids are ignored", order: agentConfigOrder{Order: []string{"missing"}}, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, mergeLogLevel(test.order, configs))
		})
	}
}

func TestAtomicLogLevel(t *testing.T) {
	level := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	setter := NewAtomicLogLevel(level)

	require.NoError(t, setter.SetLogLevel("debug"))
	assert.Equal(t, zapcore.DebugLevel, level.Level())

	require.NoError(t, setter.SetLogLevel("TRACE"))
	assert.True(t, level.Enabled(zapcore.Level(-2)), "trace enables logr V(2) messages")

	require.ErrorContains(t, setter.SetLogLevel("loud"), `unsupported log level "loud"`)
	assert.Equal(t, zapcore.Level(-2), level.Level(), "an invalid level keeps the current level")

	setter.ResetLogLevel()
	assert.Equal(t, zapcore.InfoLevel, level.Level())
}

func TestParseAgentLogLevel(t *testing.T) {
	for name, want := range map[string]zapcore.Level{
		"trace":    zapcore.DebugLevel - 1,
		"debug":    zapcore.DebugLevel,
		"info":     zapcore.InfoLevel,
		"warn":     zapcore.WarnLevel,
		"warning":  zapcore.WarnLevel,
		"error":    zapcore.ErrorLevel,
		"critical": zapcore.DPanicLevel,
		"off":      zapcore.FatalLevel + 1,
		" Info ":   zapcore.InfoLevel,
	} {
		got, err := parseAgentLogLevel(name)
		require.NoError(t, err, name)
		assert.Equal(t, want, got, name)
	}
	_, err := parseAgentLogLevel("")
	require.Error(t, err)
}
