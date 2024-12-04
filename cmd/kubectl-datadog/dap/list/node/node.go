// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	asTable        bool
	profileName    string
	profileExample = `
  # list the node(s) datadogagentprofile foo applies to
  %[1]s node -p foo

  # list the node(s) each datadogagentprofile applies to
  %[1]s node

  # list the node(s) each datadogagentprofile applies to in table form
  %[1]s node --table
`
)

// options provides information required by Datadog node command
type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string
}

// newOptions provides an instance of getOptions with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "node" sub command
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "node [flags]",
		Short:        "List the nodes a given datadogagentprofile applies to",
		Example:      fmt.Sprintf(profileExample, "kubectl datadog dap list"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.complete(c); err != nil {
				return err
			}
			if err := o.validate(); err != nil {
				return err
			}
			return o.run(c)
		},
	}

	cmd.Flags().StringVarP(&profileName, "profile", "p", "", "Profile name")
	cmd.Flags().BoolVar(&asTable, "table", false, "Output as a table")

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command
func (o *options) complete(cmd *cobra.Command) error {
	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided
func (o *options) validate() error {
	argsCount := len(o.args)
	if argsCount > 1 {
		return fmt.Errorf("at most one argument is allowed, got %d", argsCount)
	}
	return nil
}

// run runs the node command
func (o *options) run(cmd *cobra.Command) error {
	nodeGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	nodes, err := o.MetadataClient.Resource(nodeGVR).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	nodesPerProfile := getNodesPerProfile(nodes.Items)

	if asTable {
		o.renderTable(nodesPerProfile)
		return nil
	}

	jsonData, _ := json.MarshalIndent(nodesPerProfile, "", "  ")
	cmd.Println(string(jsonData))

	return nil
}

// getNodesPerProfile returns a list of nodes per profile
func getNodesPerProfile(nodes []metav1.PartialObjectMetadata) map[string][]string {
	nodesPerProfile := map[string][]string{}
	for _, nodeMeta := range nodes {
		if profileFromLabel, ok := nodeMeta.ObjectMeta.Labels[agentprofile.ProfileLabelKey]; ok {
			if profileName != "" && profileFromLabel != profileName { // list nodes for only a single profile
				continue
			}
			nodesPerProfile[profileFromLabel] = append(nodesPerProfile[profileFromLabel], nodeMeta.Name)
		}
	}
	return nodesPerProfile
}

func (o *options) renderTable(nodesPerProfile map[string][]string) {
	table := newTable(o.Out)
	tableHeader := []string{}

	data := [][]string{}
	for profile, nodes := range nodesPerProfile {
		tableHeader = append(tableHeader, profile)
		col := len(tableHeader) - 1
		for row, node := range nodes {
			rowData := make([]string, len(nodesPerProfile))
			if len(data) > row && data[row] != nil {
				rowData = data[row]
			}
			rowData[col] = node
			if len(data) > row {
				data[row] = rowData
			} else {
				data = append(data, rowData)
			}
		}
	}
	table.AppendBulk(data)
	table.SetHeader(tableHeader)
	table.Render()
}

func newTable(out io.Writer) *tablewriter.Table {
	table := tablewriter.NewWriter(out)
	table.SetBorders(tablewriter.Border{Left: false, Top: false, Right: false, Bottom: false})
	table.SetAutoFormatHeaders(false) // false = no capitalisation, true = capitalised
	table.SetCenterSeparator("|")
	return table
}
