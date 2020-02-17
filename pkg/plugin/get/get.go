// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package get

import (
	"context"
	"fmt"
	"io"

	"github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	getExample = `
  # view all DatadogAgent in the current namespace
  %[1]s get

  # view DatadogAgent foo
  %[1]s get foo
`
)

// getOptions provides information required by Datadog get command
type getOptions struct {
	genericclioptions.IOStreams
	configFlags          *genericclioptions.ConfigFlags
	args                 []string
	client               client.Client
	userNamespace        string
	userDatadogAgentName string
}

// newGetOptions provides an instance of getOptions with default values
func newGetOptions(streams genericclioptions.IOStreams) *getOptions {
	return &getOptions{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
	}
}

// NewCmdGet provides a cobra command wrapping getOptions
func NewCmdGet(streams genericclioptions.IOStreams) *cobra.Command {
	o := newGetOptions(streams)
	cmd := &cobra.Command{
		Use:          "get [DatadogAgent name]",
		Short:        "Get DatadogAgent deployment(s)",
		Example:      fmt.Sprintf(getExample, "kubectl dd"),
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

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command
func (o *getOptions) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	var err error

	clientConfig := o.configFlags.ToRawKubeConfigLoader()

	// Create the Client for Read/Write operations.
	o.client, err = common.NewClient(clientConfig)
	if err != nil {
		return fmt.Errorf("unable to instantiate client: %v", err)
	}

	o.userNamespace, _, err = clientConfig.Namespace()
	if err != nil {
		return err
	}

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	if ns != "" {
		o.userNamespace = ns
	}

	if len(args) > 0 {
		o.userDatadogAgentName = args[0]
	}

	return nil
}

// validate ensures that all required arguments and flag values are provided
func (o *getOptions) validate() error {
	if len(o.args) > 1 {
		return fmt.Errorf("either one or no arguments are allowed")
	}
	return nil
}

// run runs the get command
func (o *getOptions) run() error {
	ddList := &v1alpha1.DatadogAgentList{}
	if o.userDatadogAgentName == "" {
		if err := o.client.List(context.TODO(), ddList, &client.ListOptions{Namespace: o.userNamespace}); err != nil {
			return fmt.Errorf("unable to list DatadogAgent: %v", err)
		}
	} else {
		dd := &v1alpha1.DatadogAgent{}
		err := o.client.Get(context.TODO(), client.ObjectKey{Namespace: o.userNamespace, Name: o.userDatadogAgentName}, dd)
		if err != nil && errors.IsNotFound(err) {
			return fmt.Errorf("DatadogAgent %s/%s not found", o.userNamespace, o.userDatadogAgentName)
		} else if err != nil {
			return fmt.Errorf("unable to get DatadogAgent: %v", err)
		}
		ddList.Items = append(ddList.Items, *dd)
	}

	table := newGetTable(o.Out)
	for _, item := range ddList.Items {
		data := []string{item.Namespace, item.Name}
		if item.Status.Agent != nil {
			data = append(data, item.Status.Agent.State)
		} else {
			data = append(data, "")
		}
		if item.Status.ClusterAgent != nil {
			data = append(data, item.Status.ClusterAgent.State)
		} else {
			data = append(data, "")
		}
		if item.Status.ClusterChecksRunner != nil {
			data = append(data, item.Status.ClusterChecksRunner.State)
		} else {
			data = append(data, "")
		}
		data = append(data, common.GetDuration(&item.ObjectMeta))
		table.Append(data)
	}

	// Send output
	table.Render()

	return nil
}

func newGetTable(out io.Writer) *tablewriter.Table {
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
