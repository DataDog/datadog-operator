// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"testing"

	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewPortForwarder(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &rest.Config{
		Host: "https://kubernetes.default.svc",
	}

	pf := NewPortForwarder(PortForwarderConfig{
		Clientset:  clientset,
		RestConfig: config,
		Namespace:  "test-namespace",
		PodName:    "test-pod",
		RemotePort: 5000,
	})

	if pf == nil {
		t.Fatal("Expected port forwarder to be created")
	}

	if pf.namespace != "test-namespace" {
		t.Errorf("Expected namespace 'test-namespace', got %s", pf.namespace)
	}

	if pf.podName != "test-pod" {
		t.Errorf("Expected pod name 'test-pod', got %s", pf.podName)
	}

	if pf.remotePort != 5000 {
		t.Errorf("Expected remote port 5000, got %d", pf.remotePort)
	}

	if pf.localPort != 0 {
		t.Errorf("Expected initial local port 0, got %d", pf.localPort)
	}
}

func TestPortForwarder_LocalPort(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &rest.Config{
		Host: "https://kubernetes.default.svc",
	}

	pf := NewPortForwarder(PortForwarderConfig{
		Clientset:  clientset,
		RestConfig: config,
		Namespace:  "test-namespace",
		PodName:    "test-pod",
		RemotePort: 5000,
	})

	// Before start, local port should be 0
	if port := pf.LocalPort(); port != 0 {
		t.Errorf("Expected local port 0 before start, got %d", port)
	}
}

func TestPortForwarder_BaseURL(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &rest.Config{
		Host: "https://kubernetes.default.svc",
	}

	pf := NewPortForwarder(PortForwarderConfig{
		Clientset:  clientset,
		RestConfig: config,
		Namespace:  "test-namespace",
		PodName:    "test-pod",
		RemotePort: 5000,
	})

	// Manually set local port for testing
	pf.localPort = 12345

	expectedURL := "http://localhost:12345"
	if url := pf.BaseURL(); url != expectedURL {
		t.Errorf("Expected base URL %s, got %s", expectedURL, url)
	}
}

func TestPortForwarder_Stop(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &rest.Config{
		Host: "https://kubernetes.default.svc",
	}

	pf := NewPortForwarder(PortForwarderConfig{
		Clientset:  clientset,
		RestConfig: config,
		Namespace:  "test-namespace",
		PodName:    "test-pod",
		RemotePort: 5000,
	})

	// Stop should not panic even if not started
	pf.Stop()
}
