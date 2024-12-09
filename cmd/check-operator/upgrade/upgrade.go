// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Options provides information required to manage canary.
type Options struct {
	genericclioptions.IOStreams
	common.Options
	args               []string
	datadogAgentName   string
	checkPeriod        time.Duration
	checkTimeout       time.Duration
	agentCompletionPct float64
	agentCompletionMin int32
	dcaMinUpToDate     int32
	clcMinUpToDate     int32
}

// NewOptions provides an instance of Options with default values.
func NewOptions(streams genericclioptions.IOStreams) *Options {
	opts := &Options{
		IOStreams:          streams,
		checkPeriod:        30 * time.Second,
		checkTimeout:       2 * time.Hour,
		agentCompletionPct: 0.95,
		agentCompletionMin: 10,
		dcaMinUpToDate:     1,
		clcMinUpToDate:     2,
	}

	opts.SetConfigFlags()

	if val, found := os.LookupEnv("AGENT_COMPLETION_PCT"); found {
		if iVal, err := strconv.ParseFloat(val, 64); err == nil {
			opts.agentCompletionPct = iVal / 100
		}
	}

	if val, found := os.LookupEnv("AGENT_COMPLETION_MIN"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.agentCompletionMin = int32(iVal)
		}
	}

	if val, found := os.LookupEnv("DCA_MIN_UP_TO_DATE"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.dcaMinUpToDate = int32(iVal)
		}
	}

	if val, found := os.LookupEnv("CLC_MIN_UP_TO_DATE"); found {
		if iVal, err := strconv.ParseInt(val, 10, 32); err == nil {
			opts.clcMinUpToDate = int32(iVal)
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
		Use:          "upgrade [DatadogAgent name]",
		Short:        "Wait until the rolling-update of all agent components is finished",
		Example:      "./check-operator upgrade datadog-agent",
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

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// Complete sets all information required for processing the command.
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.datadogAgentName = args[0]
	}

	return o.Init(cmd)
}

// Validate ensures that all required arguments and flag values are provided.
func (o *Options) Validate() error {
	if o.datadogAgentName == "" {
		return fmt.Errorf("the DatadogAgent name is required")
	}

	return nil
}

func (o *Options) getV2Status() (common.StatusWrapper, error) {
	datadogAgent := &v2alpha1.DatadogAgent{}
	err := o.Client.Get(context.TODO(), client.ObjectKey{Namespace: o.UserNamespace, Name: o.datadogAgentName}, datadogAgent)
	if err != nil {
		if errors.IsNotFound(err) {

			return nil, err
		}

		return nil, fmt.Errorf("unable to get DatadogAgent, err: %w", err)
	}
	return common.NewV2StatusWrapper(datadogAgent), nil
}

func isReconcileError(conditions []metav1.Condition) error {
	for _, condition := range conditions {
		switch {
		case condition.Type == "DatadogAgentReconcileError" && condition.Status == metav1.ConditionTrue:
			return fmt.Errorf("datadogAgent reconciliation error message: %s", condition.Message)
		case condition.Type == "AgentReconcile" && condition.Status == metav1.ConditionFalse:
			return fmt.Errorf("agent reconciliation error message: %s", condition.Message)
		case condition.Type == "ClusterAgentReconcile" && condition.Status == metav1.ConditionFalse:
			return fmt.Errorf("cluster Agent reconciliation error message: %s", condition.Message)
		case condition.Type == "ClusterChecksRunnerReconcile" && condition.Status == metav1.ConditionFalse:
			return fmt.Errorf("cluster Check Runner reconciliation error message: %s", condition.Message)
		}
	}
	return nil
}

// Run use to run the command.
func (o *Options) Run() error {
	o.printOutf("Start checking rolling-update status")
	checkFunc := func() (bool, error) {
		var agentDone, dcaDone, ccrDone, reconcileError bool
		var status common.StatusWrapper
		o.printOutf("v2alpha1 is available")
		status, err := o.getV2Status()

		if errors.IsNotFound(err) {
			o.printOutf("Got a not found error while getting %s/%s. Assuming this DatadogAgent CR has never been deployed in this environment", o.UserNamespace, o.datadogAgentName)
			return true, nil
		} else if err != nil {
			return false, fmt.Errorf("unable to get the DatadogAgent.status, err:%w", err)
		}

		if err = isReconcileError(status.GetStatusCondition()); err != nil {
			o.printOutf("received a reconcile error: %v", err)
			reconcileError = true
		}

		if !agentDone {
			agentDone = o.isAgentDone(status.GetAgentStatus())
		}

		if !dcaDone {
			dcaDone = o.isDeploymentDone(status.GetClusterAgentStatus(), o.dcaMinUpToDate, "Cluster Agent")
		}

		if !ccrDone {
			ccrDone = o.isDeploymentDone(status.GetClusterChecksRunnerStatus(), o.clcMinUpToDate, "Cluster Check Runner")
		}

		if agentDone && dcaDone && ccrDone && !reconcileError {
			return true, nil
		}

		o.printOutf("One or multiple components are still upgrading...")

		if status.GetAgentStatus() != nil {
			o.printOutf("[Agent] nb pods: %d, nb updated pods: %d", status.GetAgentStatus().Current, status.GetAgentStatus().UpToDate)
		}

		if status.GetClusterAgentStatus() != nil {
			o.printOutf("[Cluster Agent] nb pods: %d, nb updated pods: %d", status.GetClusterAgentStatus().Replicas, status.GetClusterAgentStatus().UpdatedReplicas)
		}

		if status.GetClusterChecksRunnerStatus() != nil {
			o.printOutf("[Cluster Check Runner] nb pods: %d, nb updated pods: %d", status.GetClusterChecksRunnerStatus().Replicas, status.GetClusterChecksRunnerStatus().UpdatedReplicas)
		}

		return false, nil
	}

	return wait.Poll(o.checkPeriod, o.checkTimeout, checkFunc)
}

func (o *Options) isAgentDone(status *v2alpha1.DaemonSetStatus) bool {
	if status == nil {
		return true
	}

	if float64(status.UpToDate) > float64(status.Current)*o.agentCompletionPct || status.Current-status.UpToDate <= o.agentCompletionMin {
		o.printOutf("[Agent] upgrade is now finished (reached threshold): %d, nb updated pods: %d, threshold pct: %f, min threshold: %d", status.Current, status.UpToDate, o.agentCompletionPct, o.agentCompletionMin)

		return true
	}

	return false
}

func (o *Options) isDeploymentDone(status *v2alpha1.DeploymentStatus, minUpToDate int32, component string) bool {
	if status == nil {
		return true
	}

	if status.UpdatedReplicas >= minUpToDate {
		o.printOutf("[%s] upgrade is now finished (reached threshold): %d, nb updated pods: %d, min up-to-date threshold: %d", component, status.Replicas, status.UpdatedReplicas, o.dcaMinUpToDate)

		return true
	}

	return false
}

func (o *Options) printOutf(format string, a ...interface{}) {
	args := []interface{}{time.Now().UTC().Format("2006-01-02T15:04:05.999Z"), o.UserNamespace, o.datadogAgentName}
	args = append(args, a...)
	_, _ = fmt.Fprintf(o.Out, "[%s] DatadogAgent '%s/%s': "+format+"\n", args...)
}
