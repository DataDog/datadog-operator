package get

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	getExample = `
  # view all DatadogMetric in the current namespace
  %[1]s get

  # get the DatadogMetric named foo
  %[1]s get foo
`
)

// options provides information required by Datadog get command
type options struct {
	genericclioptions.IOStreams
	common.Options
	args              []string
	datadogMetricName string
}

// newOptions provides an instance of getOptions with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "get" sub command
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "get [DatadogMetric name] [flags]",
		Short:        "Get DatadogMetric deployment(s)",
		Example:      fmt.Sprintf(getExample, "kubectl datadog metrics"),
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

// complete sets all information required for processing the command
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.datadogMetricName = args[0]
	}
	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided
func (o *options) validate() error {
	if len(o.args) > 1 {
		return errors.New("either one or no arguments are allowed")
	}
	return nil
}

// run runs the get command
func (o *options) run() error {
	ddList := &v1alpha1.DatadogMetricList{}
	if o.datadogMetricName == "" {
		if err := o.Client.List(context.TODO(), ddList, &client.ListOptions{Namespace: o.UserNamespace}); err != nil {
			return fmt.Errorf("unable to list DatadogMetric: %v", err)
		}
	} else {
		dd := &v1alpha1.DatadogMetric{}
		err := o.Client.Get(context.TODO(), client.ObjectKey{Namespace: o.UserNamespace, Name: o.datadogMetricName}, dd)
		if err != nil && apierrors.IsNotFound(err) {
			return fmt.Errorf("DatadogMetric %s/%s not found", o.UserNamespace, o.datadogMetricName)
		} else if err != nil {
			return fmt.Errorf("unable to get DatadogMetric: %v", err)
		}
		ddList.Items = append(ddList.Items, *dd)
	}

	table := newTable(o.Out)
	for _, item := range ddList.Items {
		data := []string{item.Namespace, item.Name}
		if item.Status.Conditions != nil {
			for i := range item.Status.Conditions {
				if item.Status.Conditions[i].Type == v1alpha1.DatadogMetricConditionTypeActive {
					if string(item.Status.Conditions[i].Status) != "" {
						data = append(data, string(item.Status.Conditions[i].Status))
					} else {
						data = append(data, "")
					}
				}
				if item.Status.Conditions[i].Type == v1alpha1.DatadogMetricConditionTypeValid {
					if string(item.Status.Conditions[i].Status) != "" {
						data = append(data, string(item.Status.Conditions[i].Status))
					} else {
						data = append(data, "")
					}
				}
			}
		}
		if item.Status.Value != "" {
			data = append(data, item.Status.Value)
		} else {
			data = append(data, "")
		}
		if item.Status.AutoscalerReferences != "" {
			data = append(data, item.Status.AutoscalerReferences)
		} else {
			data = append(data, "")
		}
		if item.Status.Conditions != nil {
			for i := range item.Status.Conditions {
				if item.Status.Conditions[i].Type == v1alpha1.DatadogMetricConditionTypeUpdated {
					if !item.Status.Conditions[i].LastUpdateTime.IsZero() {
						age := duration.HumanDuration(time.Since(item.Status.Conditions[i].LastUpdateTime.Time))
						data = append(data, age)
					} else {
						data = append(data, "")
					}
				}
			}
		}
		table.Append(data)
	}

	// Send output
	table.Render()

	return nil
}

func newTable(out io.Writer) *tablewriter.Table {
	table := tablewriter.NewWriter(out)
	table.SetHeader([]string{"NAMESPACE", "NAME", "ACTIVE", "VALID", "VALUE", "REFERENCES", "UPDATE TIME"})
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
