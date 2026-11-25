// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package migrate implements "kubectl datadog monitor migrate", which converts
// a DatadogMonitor into a DatadogGenericResource that adopts the same monitor.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

const (
	outputYAML     = "yaml"
	outputJSON     = "json"
	defaultTimeout = 5 * time.Minute
	pollInterval   = 2 * time.Second
)

var migrateExample = `
  # Migrate the DatadogMonitor foo to a DatadogGenericResource that keeps the same monitor
  %[1]s monitor migrate foo

  # Show what the migration would do, without changing anything
  %[1]s monitor migrate foo --dry-run

  # GitOps: print the annotated DatadogMonitor and the DatadogGenericResource manifests to commit
  %[1]s monitor migrate foo -o yaml

  # GitOps: print only the monitor ID and jsonSpec, for a Helm or Kustomize template
  %[1]s monitor migrate foo -o json

  # GitOps: only annotate the live DatadogMonitor, so that pruning it keeps the monitor
  %[1]s monitor migrate foo --annotate-only

  # Report the migration state of foo
  %[1]s monitor migrate foo --check
`

// options provides information required by the migrate command.
type options struct {
	genericclioptions.IOStreams
	common.Options
	name         string
	output       string
	dryRun       bool
	annotateOnly bool
	check        bool
	timeout      time.Duration
}

func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{IOStreams: streams}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "monitor migrate".
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:   "migrate <DatadogMonitor name>",
		Short: "Migrate a DatadogMonitor to a DatadogGenericResource that keeps the same monitor",
		Long: `Migrate a DatadogMonitor to a DatadogGenericResource that adopts the same Datadog monitor, so the monitor keeps its ID, history and references.

The command:
  1. Checks that the DatadogMonitor is in sync and that no DatadogGenericResource uses its monitor ID.
  2. Annotates the DatadogMonitor with datadoghq.com/management-policy=orphan, and waits for the Operator to acknowledge it.
  3. Creates a DatadogGenericResource with the same name, spec.id set to the monitor ID, and the definition the Operator sends for the DatadogMonitor.
  4. Deletes the DatadogMonitor, which keeps the monitor in Datadog.
  5. Waits for the DatadogGenericResource to adopt the monitor and write its definition.

Running the command again after an interruption continues from where it stopped. The Operator must support spec.id and the orphan management policy, and run both the DatadogMonitor and the DatadogGenericResource controllers.`,
		Example:      fmt.Sprintf(migrateExample, "kubectl datadog"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.validate(); err != nil {
				return err
			}
			if err := o.complete(c, args); err != nil {
				return err
			}
			return o.run(c.Context())
		},
	}

	cmd.Flags().StringVarP(&o.output, "output", "o", "", "Print manifests instead of migrating: yaml (annotated DatadogMonitor and DatadogGenericResource) or json (monitor ID and jsonSpec)")
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "Show what the migration would do, without changing anything")
	cmd.Flags().BoolVar(&o.annotateOnly, "annotate-only", false, "Only add the orphan annotation to the DatadogMonitor")
	cmd.Flags().BoolVar(&o.check, "check", false, "Report the migration state without changing anything")
	cmd.Flags().DurationVar(&o.timeout, "timeout", defaultTimeout, "How long to wait for each step done by the Operator")
	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.name = args[0]
	return o.Init(cmd)
}

func (o *options) validate() error {
	modes := 0
	for _, set := range []bool{o.output != "", o.dryRun, o.annotateOnly, o.check} {
		if set {
			modes++
		}
	}
	if modes > 1 {
		return errors.New("--output, --dry-run, --annotate-only and --check are mutually exclusive")
	}
	if o.output != "" && o.output != outputYAML && o.output != outputJSON {
		return fmt.Errorf("unsupported output %q: use yaml or json", o.output)
	}
	if o.timeout <= 0 {
		return errors.New("--timeout must be positive")
	}
	return nil
}

func (o *options) run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	m := newMigrator(o.Client, o.Out, o.ErrOut, o.UserNamespace, o.name, o.timeout)
	switch {
	case o.check:
		return m.check(ctx)
	case o.output != "":
		return m.print(ctx, o.output)
	case o.annotateOnly:
		return m.annotateOnly(ctx)
	default:
		return m.migrate(ctx, o.dryRun)
	}
}
