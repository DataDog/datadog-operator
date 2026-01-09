// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import "testing"

func TestForwardersManager_unregisterForwarder_Idempotent(t *testing.T) {
	t.Parallel()

	fm := &ForwardersManager{
		metricsForwarders: make(map[string]*metricsForwarder),
	}

	id := "DatadogAgentInternal/foo/test"
	fm.metricsForwarders[id] = &metricsForwarder{
		stopChan: make(chan struct{}),
	}

	// First unregister removes it.
	if err := fm.unregisterForwarder(id); err != nil {
		t.Fatalf("expected no error when unregistering existing forwarder, got: %v", err)
	}

	// Second unregister should still be a no-op.
	if err := fm.unregisterForwarder(id); err != nil {
		t.Fatalf("expected no error when unregistering already-unregistered forwarder, got: %v", err)
	}
}
