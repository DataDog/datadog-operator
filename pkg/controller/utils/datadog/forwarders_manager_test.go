// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"net/http"
	"testing"
)

func TestNewForwarderHTTPClient_RaisesIdleConnPool(t *testing.T) {
	c := newForwarderHTTPClient()

	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.Transport)
	}

	// Stdlib default is 2 per host — far too low for many forwarders sharing one Datadog host.
	if transport.MaxIdleConnsPerHost != forwarderMaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, forwarderMaxIdleConnsPerHost)
	}

	// Must not mutate the global default transport shared by other stdlib/library callers.
	defaultTransport := http.DefaultTransport.(*http.Transport)
	if defaultTransport.MaxIdleConnsPerHost == forwarderMaxIdleConnsPerHost {
		t.Errorf("http.DefaultTransport was mutated, want it untouched")
	}
}

func TestNewForwardersManager_BuildsSharedHTTPClient(t *testing.T) {
	fm := NewForwardersManager(nil, nil, nil)

	if fm.httpClient == nil {
		t.Fatal("expected NewForwardersManager to set a shared httpClient, got nil")
	}
}

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
