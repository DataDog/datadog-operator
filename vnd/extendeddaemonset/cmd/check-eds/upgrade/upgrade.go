// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package upgrade contains upgrade plugin command logic.
package upgrade

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/common"
)

var upgradeExample = `
	# wait until the end of the extendeddaemonset foo upgrade
	%[1]s upgrade foo
`

// Options provides information required to manage canary.
type Options struct {
	configFlags *genericclioptions.ConfigFlags
	args        []string

	client client.Client

	genericclioptions.IOStreams

	userNamespace             string
	userExtendedDaemonSetName string
	checkPeriod               time.Duration
	checkTimeout              time.Duration
	nodeCompletionPct         float64
	nodeCompletionMin         int32
}

// NewOptions provides an instance of Options with default values.
func NewOptions(streams genericclioptions.IOStreams) *Options {
	opts := &Options{
		configFlags: genericclioptions.NewConfigFlags(false),

		IOStreams:         streams,
		checkPeriod:       30 * time.Second,
		checkTimeout:      2 * time.Hour,
		nodeCompletionPct: 0.95,
		nodeCompletionMin: 10,
	}

	if val, found := os.LookupEnv("NODE_COMPLETION_PCT"); found {
		if iVal, err := strconv.ParseFloat(val, 64); err == nil {
			opts.nodeCompletionPct = iVal / 100
		}
	}

	if val, found := os.LookupEnv("NODE_COMPLETION_MIN"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.nodeCompletionMin = int32(iVal)
		}
	}

	if val, found := os.LookupEnv("CHECK_TIMEOUT_MINUTES"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.checkTimeout = time.Duration(iVal) * time.Minute
		}
	}

	return opts
}

// NewCmdUpgrade provides a cobra command wrapping Options.
func NewCmdUpgrade(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewOptions(streams)

	cmd := &cobra.Command{
		Use:          "upgrade [ExtendedDaemonSet name]",
		Short:        "wait until end of an ExtendedDaemonSet upgrade",
		Example:      fmt.Sprintf(upgradeExample, "kubectl"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}

			return o.Run()
		},
	}

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// Complete sets all information required for processing the command.
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
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

// Validate ensures that all required arguments and flag values are provided.
func (o *Options) Validate() error {
	if o.userExtendedDaemonSetName == "" {
		return fmt.Errorf("the ExtendedDaemonset name needs to be provided")
	}

	return nil
}

// Run use to run the command.
func (o *Options) Run() error {
	o.printOutf("start checking deployment state")

	checkUpgradeDown := func(ctx context.Context) (bool, error) {
		eds := &v1alpha1.ExtendedDaemonSet{}
		err := o.client.Get(context.TODO(), client.ObjectKey{Namespace: o.userNamespace, Name: o.userExtendedDaemonSetName}, eds)
		if err != nil && errors.IsNotFound(err) {
			return false, fmt.Errorf("ExtendedDaemonSet %s/%s not found", o.userNamespace, o.userExtendedDaemonSetName)
		} else if err != nil {
			return false, fmt.Errorf("unable to get ExtendedDaemonSet, err: %w", err)
		}

		if eds.Status.Canary != nil {
			o.printOutf("canary running")

			return false, nil
		}

		// We need to look at the activeReplicaSet of the current ExtendedDaemonSet object, and look at the creationTimestamp of
		// that replicaset. If the creationTimestamp is older than the last occurrence of the "CanaryFailed" condition of the ExtendedDaemonSet,
		// it is safe to assume that the canary failed and we should fail this check.
		var canaryFailedConditionPresent bool
		var canaryFailedCondition v1alpha1.ExtendedDaemonSetCondition
		for _, condition := range eds.Status.Conditions {
			if condition.Type == v1alpha1.ConditionTypeEDSCanaryFailed {
				canaryFailedConditionPresent = true
				canaryFailedCondition = condition
				break
			}
		}

		if canaryFailedConditionPresent {
			ers := &v1alpha1.ExtendedDaemonSetReplicaSet{}
			err = o.client.Get(context.TODO(), client.ObjectKey{Namespace: o.userNamespace, Name: eds.Status.ActiveReplicaSet}, ers)
			if err == nil {
				if ers.CreationTimestamp.Before(&canaryFailedCondition.LastTransitionTime) {
					return false, fmt.Errorf("active canary has a creation timestamp before the last CanaryFailed condition, meaning the deployment failed")
				}
			} else {
				o.printOutf("error getting replicaset %s: %s", eds.Status.ActiveReplicaSet, err.Error())
			}
		}

		if float64(eds.Status.UpToDate) > float64(eds.Status.Current)*o.nodeCompletionPct ||
			eds.Status.Current-eds.Status.UpToDate < o.nodeCompletionMin {
			o.printOutf("upgrade is now finished (reached threshold): %d, nb updated pods: %d, threshold pct: %f, min threshold: %d", eds.Status.Current, eds.Status.UpToDate, o.nodeCompletionPct, o.nodeCompletionMin)

			return true, nil
		}

		o.printOutf("still upgrading nb pods: %d, nb updated pods: %d", eds.Status.Current, eds.Status.UpToDate)

		return false, nil
	}

	return wait.PollUntilContextTimeout(context.TODO(), o.checkPeriod, o.checkTimeout, false, checkUpgradeDown)
}

func (o *Options) printOutf(format string, a ...interface{}) {
	args := []interface{}{time.Now().UTC().Format("2006-01-02T15:04:05.999Z"), o.userNamespace, o.userExtendedDaemonSetName}
	args = append(args, a...)
	_, _ = fmt.Fprintf(o.Out, "[%s] ExtendedDaemonset '%s/%s': "+format+"\n", args...)
}
