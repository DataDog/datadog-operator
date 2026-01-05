// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForwarder manages port-forwarding to a cluster-agent pod
type PortForwarder struct {
	clientset  kubernetes.Interface
	restConfig *rest.Config
	namespace  string
	podName    string
	remotePort int
	localPort  int

	// Internal state
	stopChan  chan struct{}
	readyChan chan struct{}
	errChan   chan error
	forwarder *portforward.PortForwarder
}

// PortForwarderConfig contains configuration for creating a PortForwarder
type PortForwarderConfig struct {
	Clientset  kubernetes.Interface
	RestConfig *rest.Config
	Namespace  string
	PodName    string
	RemotePort int
}

// NewPortForwarder creates a new port forwarder to a pod
func NewPortForwarder(config PortForwarderConfig) *PortForwarder {
	return &PortForwarder{
		clientset:  config.Clientset,
		restConfig: config.RestConfig,
		namespace:  config.Namespace,
		podName:    config.PodName,
		remotePort: config.RemotePort,
		localPort:  0, // 0 means OS will assign a free port
		stopChan:   make(chan struct{}, 1),
		readyChan:  make(chan struct{}),
		errChan:    make(chan error, 1),
	}
}

// Start initiates the port-forward (non-blocking)
// The port-forward runs in a goroutine until Stop() is called or an error occurs
func (pf *PortForwarder) Start() error {
	// Build the URL to the pod's port-forward subresource
	req := pf.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(pf.namespace).
		Name(pf.podName).
		SubResource("portforward")

	// Create SPDY roundtripper for the connection
	transport, upgrader, err := spdy.RoundTripperFor(pf.restConfig)
	if err != nil {
		return fmt.Errorf("failed to create SPDY roundtripper: %w", err)
	}

	// Find a free local port
	lc := &net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to find free local port: %w", err)
	}
	pf.localPort = listener.Addr().(*net.TCPAddr).Port
	listener.Close() // Close immediately, portforward will bind to it

	// Create dialer
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	// Prepare port specifications
	ports := []string{fmt.Sprintf("%d:%d", pf.localPort, pf.remotePort)}

	// Create output writers (discard output to avoid spam)
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	// Create the port forwarder
	forwarder, err := portforward.New(dialer, ports, pf.stopChan, pf.readyChan, out, errOut)
	if err != nil {
		return fmt.Errorf("failed to create port forwarder: %w", err)
	}

	pf.forwarder = forwarder

	// Run port forwarding in background
	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			select {
			case pf.errChan <- fmt.Errorf("port forwarding failed: %w", err):
			default:
				// Error channel full, skip
			}
		}
	}()

	return nil
}

// Stop terminates the port-forward
func (pf *PortForwarder) Stop() {
	if pf.stopChan != nil {
		close(pf.stopChan)
		// Note: We don't close errChan here to avoid a race condition where the
		// ForwardPorts goroutine might still be writing to it. Since errChan is
		// buffered (size 1) and only written to once, it will be garbage collected
		// when the PortForwarder is no longer referenced.
		// readyChan is managed and closed by the k8s portforward library.
	}
}

// WaitReady blocks until port-forward is ready or times out
func (pf *PortForwarder) WaitReady(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case <-pf.readyChan:
		return nil
	case err := <-pf.errChan:
		return fmt.Errorf("port forward failed during startup: %w", err)
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for port forward to be ready after %v", timeout)
	}
}

// LocalPort returns the local port being used
// Returns 0 if port-forward hasn't been started yet
func (pf *PortForwarder) LocalPort() int {
	return pf.localPort
}

// GetForwardedPorts returns the list of ports being forwarded
// Returns nil if port-forward hasn't been started yet
func (pf *PortForwarder) GetForwardedPorts() ([]portforward.ForwardedPort, error) {
	if pf.forwarder == nil {
		return nil, fmt.Errorf("port forwarder not started")
	}

	ports, err := pf.forwarder.GetPorts()
	if err != nil {
		return nil, fmt.Errorf("failed to get forwarded ports: %w", err)
	}

	return ports, nil
}

// BaseURL returns the base URL to connect to the forwarded port
// Example: "http://localhost:12345"
func (pf *PortForwarder) BaseURL() string {
	return fmt.Sprintf("http://localhost:%d", pf.localPort)
}

// GetError returns any error that occurred during port forwarding
// Returns nil if no error has occurred
func (pf *PortForwarder) GetError() error {
	select {
	case err := <-pf.errChan:
		return err
	default:
		return nil
	}
}
