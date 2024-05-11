// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023 Datadog, Inc.

package diff

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/common"
	jy "github.com/ghodss/yaml"
	"github.com/spf13/cobra"
)

const (
	// colorActive is green
	colorActive = "\033[0;32m"
	// colorCanary is yellow
	colorCanary = "\033[0;33m"
	// colorNone resets the color
	colorNone = "\033[0m"
)

var diffExample = `
	# %[1]s between active and canary ExtendedReplicaSets for an ExtendedDaemonSet
	kubectl-eds diff foo
`

// diffOptions provides information required to manage ExtendedDaemonSet.
type diffOptions struct {
	configFlags *genericclioptions.ConfigFlags
	args        []string
	client      client.Client
	genericclioptions.IOStreams
	userNamespace             string
	userExtendedDaemonSetName string
}

// newDiffOptions provides an instance of diffOptions with default values.
func newDiffOptions(streams genericclioptions.IOStreams) *diffOptions {
	return &diffOptions{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
	}
}

// NewCmdDiff provides a cobra command wrapping diffOptions.
func NewCmdDiff(streams genericclioptions.IOStreams) *cobra.Command {
	o := newDiffOptions(streams)

	cmd := &cobra.Command{
		Use:          "diff [ExtendedDaemonSet name]",
		Short:        "diff between the canary and the active ExtendedReplicaSet of the ExtendedDaemonSet",
		Example:      fmt.Sprintf(diffExample, "diff"),
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

// complete sets all information required for processing the command.
func (o *diffOptions) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	var err error

	clientConfig := o.configFlags.ToRawKubeConfigLoader()
	// Create the Client for Read/Write operations.
	o.client, err = common.NewClient(clientConfig)
	if err != nil {
		return fmt.Errorf("unable to instantiate client, err: %w", err)
	}

	o.userNamespace, _, err = clientConfig.Namespace()
	if err != nil {
		return err
	}

	ns, err2 := cmd.Flags().GetString("namespace")
	if err2 != nil {
		return err
	}
	if ns != "" {
		o.userNamespace = ns
	}

	if len(args) > 0 {
		o.userExtendedDaemonSetName = args[0]
	}

	return nil
}

// validate ensures that all required arguments and flag values are provided.
func (o *diffOptions) validate() error {
	if len(o.args) < 1 {
		return fmt.Errorf("the extendeddaemonset name is required")
	}

	return nil
}

// run use to run the command.
func (o *diffOptions) run() error {
	eds := &v1alpha1.ExtendedDaemonSet{}
	err := o.client.Get(context.TODO(), client.ObjectKey{Namespace: o.userNamespace, Name: o.userExtendedDaemonSetName}, eds)
	if err != nil && errors.IsNotFound(err) {
		return fmt.Errorf("ExtendedDaemonSet %s/%s not found", o.userNamespace, o.userExtendedDaemonSetName)
	} else if err != nil {
		return fmt.Errorf("unable to get ExtendedDaemonSet, err: %w", err)
	}

	if eds.Spec.Strategy.Canary == nil {
		return fmt.Errorf("the ExtendedDaemonset %s/%s does not have a canary strategy", o.userNamespace, o.userExtendedDaemonSetName)
	}
	if eds.Status.Canary == nil {
		return fmt.Errorf("the ExtendedDaemonset %s/%s does not have an active canary deployment", o.userNamespace, o.userExtendedDaemonSetName)
	}

	ersActiveName := eds.Status.ActiveReplicaSet
	ersCanaryName := eds.Status.Canary.ReplicaSet

	fmt.Fprintf(o.Out, "Diffing between %s%s (active) %sand %s%s (canary)%s\n", colorActive, ersActiveName, colorNone, colorCanary, ersCanaryName, colorNone)

	activeErsSpec, err := getErsSpec(ersActiveName, o)
	if err != nil {
		return fmt.Errorf("unable to get extendedreplicaset, err: %w", err)
	}
	canaryErsSpec, err := getErsSpec(ersCanaryName, o)
	if err != nil {
		return fmt.Errorf("unable to get extendedreplicaset, err: %w", err)
	}

	activeErsSpecFileName, err := writeSpecTempYAMLFile(activeErsSpec)
	if err != nil {
		return fmt.Errorf("unable to save to temporary file the extendedreplicaset spec, err: %w", err)
	}
	canaryErsSpecFileName, err := writeSpecTempYAMLFile(canaryErsSpec)
	if err != nil {
		return fmt.Errorf("unable to save to temporary file the extendedreplicaset spec, err: %w", err)
	}

	// -y for side-by-side comparison and color argument to make it easier to read
	cmd := exec.Command("diff", "-y", "--color=always", activeErsSpecFileName, canaryErsSpecFileName)
	output, err := cmd.CombinedOutput()
	exitCode := cmd.ProcessState.ExitCode()
	switch exitCode {
	// 0: files are identical. This should never happen between a canary and an active ExtendedReplicaSet
	case 0:
		fmt.Fprintf(o.Out, "Diff exit code is 0 : the files are identical")
	// 1: files are different.
	case 1:
		fmt.Fprintf(o.Out, "%s\n%sThe active YAML spec can be found at: %s%s\n%sThe canary YAML spec can be found at: %s%s", string(output), colorActive, colorNone, activeErsSpecFileName, colorCanary, colorNone, canaryErsSpecFileName)
	// 2 or other exit code. diff was unable to run.
	default:
		return fmt.Errorf("unable to execute the diff, err: %w", err)
	}

	return nil
}

// getErsSpec retrieves the spec of an ExtendedReplicaSet for a given name
func getErsSpec(name string, o *diffOptions) (*v1alpha1.ExtendedDaemonSetReplicaSetSpec, error) {
	ers := &v1alpha1.ExtendedDaemonSetReplicaSet{}
	err := o.client.Get(context.TODO(), client.ObjectKey{Namespace: o.userNamespace, Name: name}, ers)
	if err != nil {
		return nil, err
	}
	return &ers.Spec, nil
}

// writeSpecTempYAMLFile writes into a temporary file the YAML conversion of an ExtendedReplicaSet spec
func writeSpecTempYAMLFile(ersSpec *v1alpha1.ExtendedDaemonSetReplicaSetSpec) (string, error) {
	// Convert ExtendedReplicaSet spec struct to YAML []byte
	y, err := jy.Marshal(ersSpec)
	if err != nil {
		return "", err
	}
	yamlTempFile, err := os.CreateTemp("", "temp-ers-yaml")
	if err != nil {
		return "", err
	}
	_, err = yamlTempFile.Write(y)
	if err != nil {
		return "", err
	}
	return yamlTempFile.Name(), nil
}
