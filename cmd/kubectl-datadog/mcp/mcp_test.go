// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"testing"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestOptions_validate(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "no arguments is valid",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "with one argument is invalid",
			args:    []string{"foo"},
			wantErr: true,
		},
		{
			name:    "with multiple arguments is invalid",
			args:    []string{"foo", "bar"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &options{
				IOStreams: genericclioptions.IOStreams{},
				args:      tt.args,
			}
			err := o.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewOptions(t *testing.T) {
	streams := genericclioptions.IOStreams{}
	o := newOptions(streams)

	if o == nil {
		t.Fatal("newOptions() returned nil")
	}

	if o.ConfigFlags == nil {
		t.Error("newOptions() did not initialize ConfigFlags")
	}
}

func TestNew(t *testing.T) {
	streams := genericclioptions.IOStreams{}
	cmd := New(streams)

	if cmd == nil {
		t.Fatal("New() returned nil")
	}

	if cmd.Use != "mcp" {
		t.Errorf("New() Use = %v, want %v", cmd.Use, "mcp")
	}

	if cmd.Short == "" {
		t.Error("New() Short description is empty")
	}

	if cmd.Long == "" {
		t.Error("New() Long description is empty")
	}

	if cmd.RunE == nil {
		t.Error("New() RunE is nil")
	}
}
