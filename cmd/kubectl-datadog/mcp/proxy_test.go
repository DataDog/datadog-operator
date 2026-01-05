// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"testing"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestNewProxyManager(t *testing.T) {
	opts := &options{
		IOStreams: genericclioptions.IOStreams{},
	}

	pm := NewProxyManager(opts)

	if pm == nil {
		t.Fatal("Expected proxy manager to be created")
	}

	if pm.options != opts {
		t.Error("Expected proxy manager to have correct options")
	}
}

func TestProxyManager_GetRemoteTools(t *testing.T) {
	opts := &options{
		IOStreams: genericclioptions.IOStreams{},
	}

	pm := NewProxyManager(opts)

	// Before initialization, should return nil
	tools := pm.GetRemoteTools()
	if tools != nil {
		t.Error("Expected nil remote tools before initialization")
	}
}

func TestProxyManager_GetProxyInfo(t *testing.T) {
	opts := &options{
		IOStreams: genericclioptions.IOStreams{},
	}

	pm := NewProxyManager(opts)

	// Should be able to call GetProxyInfo even before initialization
	info := pm.GetProxyInfo()
	if info == nil {
		t.Fatal("Expected non-nil proxy info")
	}

	// Check that connected is false
	if info["connected"] != "false" {
		t.Error("Expected connected=false before initialization")
	}
}

func TestProxyManager_Shutdown(t *testing.T) {
	opts := &options{
		IOStreams: genericclioptions.IOStreams{},
	}

	pm := NewProxyManager(opts)

	// Should be able to call Shutdown even before initialization
	pm.Shutdown()
}
