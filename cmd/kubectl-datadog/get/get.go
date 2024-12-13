// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package get

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var getExample = `
  # view all DatadogAgent in the current namespace
  %[1]s get

  # view DatadogAgent foo
  %[1]s get foo
`

// options provides information required by Datadog get command.
type options struct {
	genericclioptions.IOStreams
	common.Options
	args                 []string
	userDatadogAgentName string
}

// newOptions provides an instance of getOptions with default values.
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "get" sub command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "get [DatadogAgent name]",
		Short:        "Get DatadogAgent deployment(s)",
		Example:      fmt.Sprintf(getExample, "kubectl datadog"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.complete(c, args); err != nil {
				return err
			}
			if err := o.validate(); err != nil {
				return err
			}
			return o.run()
		},
	}

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command.
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.userDatadogAgentName = args[0]
	}
	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided.
func (o *options) validate() error {
	if len(o.args) > 1 {
		return errors.New("either one or no arguments are allowed")
	}
	return nil
}

// run runs the get command.
func (o *options) run() error {
	return o.runV2()
}

func (o *options) runV2() error {
	ddList := &v2alpha1.DatadogAgentList{}
	if o.userDatadogAgentName == "" {
		if err := o.Client.List(context.TODO(), ddList, &client.ListOptions{Namespace: o.UserNamespace}); err != nil {
			return fmt.Errorf("unable to list DatadogAgent: %w", err)
		}
	} else {
		dd := &v2alpha1.DatadogAgent{}
		err := o.Client.Get(context.TODO(), client.ObjectKey{Namespace: o.UserNamespace, Name: o.userDatadogAgentName}, dd)
		if err != nil && apierrors.IsNotFound(err) {
			return fmt.Errorf("DatadogAgent %s/%s not found", o.UserNamespace, o.userDatadogAgentName)
		} else if err != nil {
			return fmt.Errorf("unable to get DatadogAgent: %w", err)
		}
		ddList.Items = append(ddList.Items, *dd)
	}

	statuses := make([]common.StatusWrapper, 0, len(ddList.Items))
	for id := range ddList.Items {
		statuses = append(statuses, common.NewV2StatusWrapper(&ddList.Items[id]))
	}
	o.renderTable(statuses)
	return nil
}

func (o *options) renderTable(statuses []common.StatusWrapper) {
	table := newTable(o.Out)
	for _, item := range statuses {
		data := []string{item.GetObjectMeta().GetNamespace(), item.GetObjectMeta().GetName()}
		if item.GetAgentStatus() != nil {
			data = append(data, item.GetAgentStatus().Status)
		} else {
			data = append(data, "")
		}
		if item.GetClusterAgentStatus() != nil {
			data = append(data, item.GetClusterAgentStatus().Status)
		} else {
			data = append(data, "")
		}
		if item.GetClusterChecksRunnerStatus() != nil {
			data = append(data, item.GetClusterChecksRunnerStatus().Status)
		} else {
			data = append(data, "")
		}
		data = append(data, common.GetDurationAsString(item.GetObjectMeta()))
		table.Append(data)
	}
	table.Render()
}

func newTable(out io.Writer) *tablewriter.Table {
	table := tablewriter.NewWriter(out)
	table.SetHeader([]string{"Namespace", "Name", "Agent", "Cluster-Agent", "Cluster-Checks-Runner", "Age"})
	table.SetBorders(tablewriter.Border{Left: false, Top: false, Right: false, Bottom: false})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetRowLine(false)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderLine(false)
	return table
}
