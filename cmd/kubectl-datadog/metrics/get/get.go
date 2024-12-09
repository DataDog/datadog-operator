package get

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var getExample = `
  # view all DatadogMetric in the current namespace
  %[1]s get

  # get the DatadogMetric named foo
  %[1]s get foo
`

// options provides information required by Datadog get command.
type options struct {
	genericclioptions.IOStreams
	common.Options
	args              []string
	datadogMetricName string
}

// metricData provides information about a datadogmetric's status.
type metricData struct {
	Namespace  string
	Name       string
	Active     string
	Valid      string
	Value      string
	References string
	UpdateTime string
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

// complete sets all information required for processing the command.
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.datadogMetricName = args[0]
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
	ddList := &v1alpha1.DatadogMetricList{}
	if o.datadogMetricName == "" {
		if err := o.Client.List(context.TODO(), ddList, &client.ListOptions{Namespace: o.UserNamespace}); err != nil {
			return fmt.Errorf("unable to list DatadogMetric: %w", err)
		}
	} else {
		dd := &v1alpha1.DatadogMetric{}
		err := o.Client.Get(context.TODO(), client.ObjectKey{Namespace: o.UserNamespace, Name: o.datadogMetricName}, dd)
		if err != nil && apierrors.IsNotFound(err) {
			return fmt.Errorf("DatadogMetric %s/%s not found", o.UserNamespace, o.datadogMetricName)
		} else if err != nil {
			return fmt.Errorf("unable to get DatadogMetric: %w", err)
		}
		ddList.Items = append(ddList.Items, *dd)
	}

	table := newTable(o.Out)
	for _, item := range ddList.Items {
		metric := &metricData{
			Namespace:  item.Namespace,
			Name:       item.Name,
			Value:      item.Status.Value,
			References: item.Status.AutoscalerReferences,
		}
		for i := range item.Status.Conditions {
			switch item.Status.Conditions[i].Type {
			case v1alpha1.DatadogMetricConditionTypeActive:
				metric.Active = string(item.Status.Conditions[i].Status)
			case v1alpha1.DatadogMetricConditionTypeValid:
				metric.Valid = string(item.Status.Conditions[i].Status)
			case v1alpha1.DatadogMetricConditionTypeUpdated:
				metric.UpdateTime = duration.HumanDuration(time.Since(item.Status.Conditions[i].LastUpdateTime.Time))
			}
		}
		table.Append(metricDataToData(metric))
	}

	// Send output
	table.Render()

	return nil
}

// metricDataToData converts metricData fields to use in tablewriter
func metricDataToData(metric *metricData) []string {
	return []string{
		metric.Namespace,
		metric.Name,
		metric.Active,
		metric.Valid,
		metric.Value,
		metric.References,
		metric.UpdateTime,
	}
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
