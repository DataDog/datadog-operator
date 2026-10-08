// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dashboard implements the "kubectl datadog dashboard" command.
package dashboard

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	dash "github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
	"github.com/DataDog/datadog-operator/pkg/plugin/dashboard/render"
	"github.com/DataDog/datadog-operator/pkg/plugin/helm"
	"github.com/DataDog/datadog-operator/pkg/rollout"
	"github.com/DataDog/datadog-operator/pkg/version"
)

var dashboardExample = `
  # show the DatadogAgent of the current namespace
  %[1]s dashboard

  # look for the DatadogAgent in all namespaces, and show failing agent pods
  %[1]s dashboard -A --pods

  # emit the dashboard as JSON
  %[1]s dashboard -n datadog -o json

  # hide the healthy profiles with no agent pods, largest profiles first
  %[1]s dashboard --hide-empty --sort=desired

  # also hide the profiles with no agent pods that carry stale warnings
  %[1]s dashboard --hide-empty-force

  # tolerate 1%% unavailable agent pods per DaemonSet once a rollout converged
  %[1]s dashboard --max-unavailable=1%%
`

const (
	outputJSON = "json"

	defaultStallAfter = 10 * time.Minute

	clientQPS   = 50
	clientBurst = 100
)

// options provides information required by the dashboard command.
type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string

	ddaName       string
	allNamespaces bool
	output        string
	ascii         bool
	noColor       bool
	pods          bool
	stallAfter    time.Duration
	noHelm        bool
	hideEmpty     bool
	hideForce     bool
	sort          string
	profileSort   dash.ProfileSort
	// maxUnav is the raw --max-unavailable, parsed into maxUnavailable.
	maxUnav        string
	maxUnavailable rollout.MaxUnavailable

	// newStore builds the store; tests replace it.
	newStore func(o *options) (dash.Store, error)
	// isTerminal reports whether w is a terminal.
	isTerminal func(w io.Writer) bool
	// now is the clock.
	now func() time.Time
	// getenv reads the environment.
	getenv func(string) string
}

// newOptions provides an instance of options with default values.
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams:  streams,
		stallAfter: defaultStallAfter,
		newStore:   newListStore,
		isTerminal: isTerminal,
		now:        time.Now,
		getenv:     os.Getenv,
	}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "dashboard" sub command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	return newCmd(newOptions(streams))
}

func newCmd(o *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "dashboard [DatadogAgent name]",
		Aliases:      []string{"dash"},
		Short:        "Show the health of the DatadogAgent, its profiles and its workloads",
		Long:         "Show the health of the DatadogAgent, its profiles, internals and workloads, with rollout progress, errors and a summary of the other Datadog resources. The command is read-only.",
		Example:      fmt.Sprintf(dashboardExample, "kubectl datadog"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.complete(c, args); err != nil {
				return err
			}
			if err := o.validate(); err != nil {
				return err
			}
			return o.run(c.Context())
		},
	}

	o.ConfigFlags.AddFlags(cmd.Flags())
	cmd.Flags().BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "Look for the DatadogAgent in all namespaces")
	cmd.Flags().StringVarP(&o.output, "output", "o", "", `Output format: "" (styled text) or "json"`)
	cmd.Flags().BoolVar(&o.ascii, "ascii", false, "Use ASCII characters only")
	cmd.Flags().BoolVar(&o.noColor, "no-color", false, "Disable colors (also set by the NO_COLOR environment variable)")
	cmd.Flags().BoolVar(&o.pods, "pods", false, "Read the agent pods: enables Stalled detection and shows failing pods and their nodes")
	cmd.Flags().DurationVar(&o.stallAfter, "stall-after", defaultStallAfter, "Time without progress before a rollout is Stalled; only with --pods")
	cmd.Flags().BoolVar(&o.noHelm, "no-helm", false, "Skip the Helm release history lookup (no Secret access)")
	cmd.Flags().BoolVar(&o.hideEmpty, "hide-empty", false, "Hide the healthy profiles whose agent DaemonSet has 0 desired pods; profiles with an issue, a rollout or no DaemonSet stay shown")
	cmd.Flags().BoolVar(&o.hideForce, "hide-empty-force", false, "Like --hide-empty, but also hide the profiles with 0 desired pods that have warnings or errors, and their issues; profiles with a rollout in progress, no status, no DDAI or no DaemonSet stay shown")
	cmd.Flags().StringVar(&o.maxUnav, "max-unavailable", "0", "Unavailable agent pods tolerated per DaemonSet or Deployment once its rollout converged, as a count or a percentage of desired pods rounded down (e.g. 2 or 1%); more show Settling for 5 minutes after the rollout, then Degraded with a warning")
	cmd.Flags().StringVar(&o.sort, "sort", string(dash.SortName), `Profile order: "name" or "desired" (agent DaemonSet desired pods, most first; profiles without a DaemonSet last)`)

	return cmd
}

// complete sets all information required for processing the command.
func (o *options) complete(_ *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.ddaName = args[0]
	}
	ns, _, err := o.GetClientConfig().Namespace()
	if err != nil {
		return err
	}
	o.SetNamespace(ns)
	return nil
}

// validate ensures that all required arguments and flag values are provided.
func (o *options) validate() error {
	if len(o.args) > 1 {
		return errors.New("either one or no arguments are allowed")
	}
	if o.output != "" && o.output != outputJSON {
		return fmt.Errorf("unsupported output format %q: use \"json\" or leave it empty", o.output)
	}
	if o.stallAfter <= 0 {
		return errors.New("--stall-after must be positive")
	}
	sort, err := dash.ParseProfileSort(o.sort)
	if err != nil {
		return fmt.Errorf("invalid --sort: %w", err)
	}
	o.profileSort = sort
	maxUnavailable, err := rollout.ParseMaxUnavailable(o.maxUnav)
	if err != nil {
		return fmt.Errorf("invalid --max-unavailable: %w", err)
	}
	o.maxUnavailable = maxUnavailable
	return nil
}

// run reads the cluster once and renders the dashboard.
func (o *options) run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	store, err := o.newStore(o)
	if err != nil {
		return err
	}
	defer store.Close()
	if err = store.Start(ctx); err != nil {
		return err
	}

	now := o.now()
	v, err := dash.Build(store.Snapshot(), o.buildConfig(), nil, now)
	if err == nil {
		// The static command expects exactly one DDA.
		err = v.SelectionError()
	}
	if err != nil {
		return o.buildError(err)
	}
	if o.output == outputJSON {
		return render.JSON(o.Out, v)
	}
	_, err = io.WriteString(o.Out, render.Text(v, o.theme(), now))
	return err
}

func (o *options) buildConfig() dash.BuildConfig {
	return dash.BuildConfig{DDAName: o.ddaName, Pods: o.pods, StallAfter: o.stallAfter, HideEmpty: o.hideEmpty, HideEmptyForce: o.hideForce, Sort: o.profileSort, MaxUnavailable: o.maxUnavailable}
}

// buildError explains the DatadogAgent selection errors.
func (o *options) buildError(err error) error {
	var multi *dash.MultipleDDAsError
	switch {
	case errors.As(err, &multi):
		fmt.Fprintln(o.ErrOut, "Warning: more than one DatadogAgent found; this command expects exactly one:")
		for _, n := range multi.Names {
			fmt.Fprintf(o.ErrOut, "  %s\n", n)
		}
		fmt.Fprintln(o.ErrOut, "Pass the name of the one to show: kubectl datadog dashboard <name>")
		return dash.ErrMultipleDDAs
	case errors.Is(err, dash.ErrNoDDA) && !o.allNamespaces:
		return fmt.Errorf("%w in namespace %q (use -n or -A to look elsewhere)", err, o.UserNamespace)
	default:
		return err
	}
}

// theme selects the rendering from the output: styled on a terminal unless
// colors are disabled, plain text otherwise.
func (o *options) theme() render.Theme {
	t := render.Theme{ASCII: o.ascii, Color: colorprofile.NoTTY, Width: render.DefaultWidth}
	if f, ok := o.Out.(*os.File); ok && o.isTerminal(f) {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
			t.Width = w
		}
		t.Color = colorprofile.Detect(f, os.Environ())
		if o.noColor || o.getenv("NO_COLOR") != "" {
			t.Color = min(t.Color, colorprofile.ASCII)
		}
		return t
	}
	if w, err := strconv.Atoi(o.getenv("COLUMNS")); err == nil && w > 0 {
		t.Width = w
	}
	return t
}

// isTerminal reports whether w is a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// newListStore builds the one-shot store reading the cluster.
func newListStore(o *options) (dash.Store, error) {
	clients, cfg, err := o.storeConfig()
	if err != nil {
		return nil, err
	}
	return dash.NewListStore(clients, cfg), nil
}

// storeConfig builds the clients and the configuration of the stores.
func (o *options) storeConfig() (dash.ListClients, dash.ListConfig, error) {
	helmDriver := ""
	if !o.noHelm {
		// The Helm CLI stores releases where HELM_DRIVER says.
		helmDriver = cmp.Or(o.getenv("HELM_DRIVER"), helm.DriverSecret)
	}
	clients, err := newListClients(o.ConfigFlags, helmDriver)
	if err != nil {
		return dash.ListClients{}, dash.ListConfig{}, err
	}
	raw, err := o.GetClientConfig().RawConfig()
	if err != nil {
		return dash.ListClients{}, dash.ListConfig{}, err
	}
	kubeContext := raw.CurrentContext
	if c := o.ConfigFlags.Context; c != nil && *c != "" {
		kubeContext = *c
	}
	return clients, dash.ListConfig{
		Namespace:     o.UserNamespace,
		AllNamespaces: o.allNamespaces,
		DDAName:       o.ddaName,
		Pods:          o.pods,
		Cluster:       dash.ClusterInfo{Context: kubeContext, PluginVersion: version.GetVersion()},
		Now:           o.now,
	}, nil
}

// newListClients builds the clients of the store from the kubeconfig flags.
// The Helm history lookup, from helmDriver, is the only Secret access;
// an empty helmDriver disables it.
func newListClients(flags *genericclioptions.ConfigFlags, helmDriver string) (dash.ListClients, error) {
	restConfig, err := flags.ToRESTConfig()
	if err != nil {
		return dash.ListClients{}, fmt.Errorf("unable to get the REST config: %w", err)
	}
	// The store sends about twenty lists at once; the client-go defaults
	// (5 QPS, burst 10) would delay them by seconds.
	restConfig = rest.CopyConfig(restConfig)
	if restConfig.QPS == 0 {
		restConfig.QPS, restConfig.Burst = clientQPS, clientBurst
	}
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return dash.ListClients{}, fmt.Errorf("unable to create the dynamic client: %w", err)
	}
	kube, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return dash.ListClients{}, fmt.Errorf("unable to create the clientset: %w", err)
	}
	clients := dash.ListClients{Dynamic: dyn, Kube: kube}
	if helmDriver != "" {
		clients.HelmHistory = func(ctx context.Context, namespace, release string) ([]helm.Revision, error) {
			return helm.HistoryFrom(ctx, flags, helmDriver, namespace, release, 2)
		}
	}
	return clients, nil
}
