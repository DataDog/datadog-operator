// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var mcpExample = `
  # Start MCP server for use with Claude Desktop
  %[1]s mcp

  # Start MCP server for specific namespace
  %[1]s mcp --namespace datadog
`

// options provides information required by Datadog mcp command.
type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string

	debug bool
}

// newOptions provides an instance of options with default values.
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "mcp" sub command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server for DatadogAgent resource access",
		Long: `Start an MCP (Model Context Protocol) server that provides read-only
access to DatadogAgent resources and status information via stdio.

This command is designed for integration with Claude Desktop and other
MCP clients. The server runs until the client disconnects.

The MCP server exposes tools to:
- List DatadogAgent resources
- Get DatadogAgent configuration details
- Query agent runtime status
- View enabled features
- Inspect component overrides`,
		Example:      fmt.Sprintf(mcpExample, "kubectl datadog"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.complete(c, args); err != nil {
				return err
			}
			if err := o.validate(); err != nil {
				return err
			}
			return o.run(c)
		},
	}

	o.ConfigFlags.AddFlags(cmd.Flags())

	cmd.Flags().BoolVar(&o.debug, "debug", false, "Enable debug logging")

	return cmd
}

// complete sets all information required for processing the command.
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided.
func (o *options) validate() error {
	if len(o.args) > 0 {
		return errors.New("no arguments allowed")
	}
	return nil
}

// run runs the mcp command by starting the MCP server.
func (o *options) run(cmd *cobra.Command) error {
	ctx := cmd.Context()

	if o.debug {
		fmt.Fprintln(o.ErrOut, "CEDTEST: Debug logging enabled")
		fmt.Fprintln(o.ErrOut, "CEDTEST: RUN start")
		defer fmt.Fprintln(o.ErrOut, "CEDTEST: RUN end")
	}

	// Create MCP server with registered tools
	server := o.createMCPServer()
	if o.debug {
		fmt.Fprintln(o.ErrOut, "CEDTEST: MCP server created")
	}

	// Run the server with stdio transport - this blocks until client disconnects
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("MCP server failed: %w", err)
	}
	return nil
}
