// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	plugincommon "github.com/DataDog/datadog-operator/pkg/plugin/common"
)

// waitInterval is a variable for tests.
var waitInterval = 2 * time.Second

// CommandPath is the path of the dpa commands, for examples.
const CommandPath = "kubectl datadog autoscaling dpa"

// AnnotationValue is the desired annotation on one object: set to Value, or removed.
type AnnotationValue struct {
	Value  string
	Remove bool
}

// Set returns an AnnotationValue that sets the annotation to v.
func Set(v string) AnnotationValue { return AnnotationValue{Value: v} }

// Removed is an AnnotationValue that removes the annotation.
var Removed = AnnotationValue{Remove: true}

// appliedTo reports whether annotations already hold v.
func (v AnnotationValue) appliedTo(annotations map[string]string, key string) bool {
	return currentAnnotation(annotations, key) == v
}

// currentAnnotation returns the annotation as found on an object.
func currentAnnotation(annotations map[string]string, key string) AnnotationValue {
	if v, found := annotations[key]; found {
		return Set(v)
	}
	return Removed
}

// String renders v for the user: <none> when absent, "" when empty.
func (v AnnotationValue) String() string {
	switch {
	case v.Remove:
		return "<none>"
	case v.Value == "":
		return strconv.Quote(v.Value)
	}
	return v.Value
}

// AnnotationSpec describes a command that sets or removes an annotation.
type AnnotationSpec struct {
	Key string
	// Validate, if set, checks the command flags before connecting to the cluster.
	Validate func() error
	// Value returns the annotation to write on dpa. An error fails that
	// object only.
	Value func(dpa *v1alpha2.DatadogPodAutoscaler) (AnnotationValue, error)
	// PastTense completes "could not be %s", e.g. "paused".
	PastTense string
	// UntouchedState completes "already %s" for objects left untouched, e.g. "not paused".
	UntouchedState string
	// Confirmation, if set, is waited for after patching.
	Confirmation *Confirmation
}

// Confirmation describes how the Cluster Agent reports an annotation in the
// DatadogPodAutoscaler status.
type Confirmation struct {
	Name string // e.g. "Active=False with reason LocallyPaused"
	// Reflects reports whether the status of dpa reflects value.
	Reflects func(dpa *v1alpha2.DatadogPodAutoscaler, value AnnotationValue) bool
	// NotReported, if set, explains when a set value is not reported.
	NotReported string
	// Advisory reports unconfirmed objects as a warning, not a failure.
	Advisory bool
}

// SetValue returns an AnnotationSpec.Value that always writes v.
func SetValue(v string) func(*v1alpha2.DatadogPodAutoscaler) (AnnotationValue, error) {
	return func(*v1alpha2.DatadogPodAutoscaler) (AnnotationValue, error) { return Set(v), nil }
}

// RemoveKey is an AnnotationSpec.Value that removes the annotation.
func RemoveKey(*v1alpha2.DatadogPodAutoscaler) (AnnotationValue, error) { return Removed, nil }

type annotationOptions struct {
	genericclioptions.IOStreams
	plugincommon.Options
	selection

	spec    AnnotationSpec
	yes     bool
	dryRun  bool
	wait    bool
	timeout time.Duration
}

type change struct {
	dpa   v1alpha2.DatadogPodAutoscaler
	value AnnotationValue
}

// NewAnnotationCommand adds the flags and RunE that apply spec to cmd.
func NewAnnotationCommand(streams genericclioptions.IOStreams, spec AnnotationSpec, cmd *cobra.Command) *cobra.Command {
	o := &annotationOptions{IOStreams: streams, spec: spec}
	o.SetConfigFlags()

	cmd.SilenceUsage = true
	cmd.RunE = func(c *cobra.Command, args []string) error {
		o.names = args
		if err := o.validate(); err != nil {
			return err
		}
		if spec.Validate != nil {
			if err := spec.Validate(); err != nil {
				return err
			}
		}
		if err := o.Init(c); err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(c.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		return o.run(ctx)
	}

	o.addFlags(cmd.Flags())
	cmd.Flags().BoolVar(&o.yes, "yes", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "Print the DatadogPodAutoscalers that would be changed, without changing them")
	if spec.Confirmation != nil {
		cmd.Flags().BoolVar(&o.wait, "wait", true, "Wait for the Cluster Agent to report the change in the DatadogPodAutoscaler status")
		cmd.Flags().DurationVar(&o.timeout, "timeout", 30*time.Second, "How long to wait for the Cluster Agent")
	}
	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

func (o *annotationOptions) run(ctx context.Context) error {
	dpas, err := o.resolve(ctx, o.Client, o.UserNamespace)
	if err != nil {
		return err
	}
	if len(dpas) == 0 {
		fmt.Fprintln(o.ErrOut, "No DatadogPodAutoscaler matched the selection.")
		return nil
	}

	var (
		changes  []change
		already  int
		failures int
	)
	for _, dpa := range dpas {
		value, err := o.spec.Value(&dpa)
		if err != nil {
			failures++
			fmt.Fprintf(o.ErrOut, "%s/%s: not %s: %v\n", dpa.Namespace, dpa.Name, o.spec.PastTense, err)
			continue
		}
		if value.appliedTo(dpa.Annotations, o.spec.Key) {
			already++
			continue
		}
		changes = append(changes, change{dpa: dpa, value: value})
	}
	if already > 0 {
		fmt.Fprintf(o.ErrOut, "%d DatadogPodAutoscaler(s) already %s, left untouched.\n", already, o.spec.UntouchedState)
	}
	if len(changes) == 0 {
		return o.failed(failures)
	}

	if o.dryRun {
		for _, ch := range changes {
			fmt.Fprintln(o.Out, o.describe(ch))
		}
		fmt.Fprintf(o.ErrOut, "Dry run: %d DatadogPodAutoscaler(s) would be %s.\n", len(changes), o.spec.PastTense)
		return o.failed(failures)
	}

	// Prompt on the selection, not on what changes: a wide selection is
	// confirmed even when most of it is already in the requested state.
	if len(dpas) > 1 && !o.yes {
		if !isTerminal(o.In) {
			return errNoTerminal
		}
		fmt.Fprintf(o.Out, "%d DatadogPodAutoscaler(s) selected; the following %d will be %s:\n", len(dpas), len(changes), o.spec.PastTense)
		for _, ch := range changes {
			fmt.Fprintf(o.Out, "  • %s\n", o.describe(ch))
		}
		ok, err := confirm(ctx, o.IOStreams, "Continue?")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(o.Out, "Cancelled.")
			return nil
		}
	}

	var patched []change
	for i, ch := range changes {
		if ctx.Err() != nil {
			fmt.Fprintf(o.ErrOut, "Interrupted: %d DatadogPodAutoscaler(s) %s, %d failed, %d left untouched.\n", len(patched), o.spec.PastTense, failures, len(changes)-i)
			return ctx.Err()
		}
		key := client.ObjectKeyFromObject(&ch.dpa)
		if err := patchAnnotation(ctx, o.Client, &ch.dpa, o.spec.Key, ch.value); err != nil {
			failures++
			fmt.Fprintf(o.ErrOut, "%s: not %s: %v\n", key, o.spec.PastTense, err)
			continue
		}
		patched = append(patched, ch)
		fmt.Fprintf(o.Out, "%s %s\n", key, o.spec.PastTense)
	}

	if o.spec.Confirmation != nil && o.wait && len(patched) > 0 {
		unconfirmed, err := o.waitForConfirmation(ctx, patched)
		if err != nil {
			return err
		}
		if !o.spec.Confirmation.Advisory {
			failures += unconfirmed
		}
	}
	return o.failed(failures)
}

func (o *annotationOptions) failed(failures int) error {
	if failures > 0 {
		return fmt.Errorf("%d DatadogPodAutoscaler(s) could not be %s", failures, o.spec.PastTense)
	}
	return nil
}

// describe renders a change as "namespace/name: current → new".
func (o *annotationOptions) describe(ch change) string {
	return fmt.Sprintf("%s/%s: %s → %s", ch.dpa.Namespace, ch.dpa.Name, currentAnnotation(ch.dpa.Annotations, o.spec.Key), ch.value)
}

// listByKey lists the DatadogPodAutoscalers of the namespaces of changes, with
// one call per namespace rather than one per object.
func (o *annotationOptions) listByKey(ctx context.Context, changes []change) (map[client.ObjectKey]v1alpha2.DatadogPodAutoscaler, error) {
	byKey := map[client.ObjectKey]v1alpha2.DatadogPodAutoscaler{}
	listed := map[string]bool{}
	for _, ch := range changes {
		if listed[ch.dpa.Namespace] {
			continue
		}
		listed[ch.dpa.Namespace] = true
		list := v1alpha2.DatadogPodAutoscalerList{}
		if err := o.Client.List(ctx, &list, client.InNamespace(ch.dpa.Namespace)); err != nil {
			return nil, err
		}
		for _, dpa := range list.Items {
			byKey[client.ObjectKeyFromObject(&dpa)] = dpa
		}
	}
	return byKey, nil
}

// waitForConfirmation returns how many objects spec.Confirmation did not
// confirm. observedGeneration cannot be used: annotations do not bump the
// generation. The status before the patch tells a new state from a stale one.
func (o *annotationOptions) waitForConfirmation(ctx context.Context, changes []change) (int, error) {
	confirmation := o.spec.Confirmation
	var pending, unverifiable []change
	for _, ch := range changes {
		if !ch.value.Remove && confirmation.Reflects(&ch.dpa, ch.value) {
			unverifiable = append(unverifiable, ch)
		} else {
			pending = append(pending, ch)
		}
	}
	if len(unverifiable) > 0 {
		fmt.Fprintf(o.Out, "%d DatadogPodAutoscaler(s) already reported %s before this change: the new value cannot be confirmed from the status.\n", len(unverifiable), confirmation.Name)
	}
	if len(pending) == 0 {
		return 0, nil
	}

	fmt.Fprintf(o.Out, "Waiting up to %s for the Cluster Agent to confirm...\n", o.timeout)
	total := len(pending)
	var (
		listed   bool
		firstErr error
	)
	// The condition never fails: the only error is the timeout or an interruption.
	_ = wait.PollUntilContextTimeout(ctx, waitInterval, o.timeout, true, func(ctx context.Context) (bool, error) {
		current, err := o.listByKey(ctx, pending)
		if err != nil {
			// Retried until the timeout: near the poll deadline, the client
			// rate limiter fails at once rather than waiting past it, and an
			// in-flight request is cut off. Neither is a status check failure.
			// The first error is kept: the first tick runs right after the
			// patch, far from the deadline, so it carries the real cause.
			if firstErr == nil {
				firstErr = err
			}
			return false, nil
		}
		listed = true
		var still []change
		for _, ch := range pending {
			dpa, found := current[client.ObjectKeyFromObject(&ch.dpa)]
			if found && !confirmation.Reflects(&dpa, ch.value) {
				still = append(still, ch)
			}
			// Deleted objects have nothing to confirm.
		}
		pending = still
		return len(pending) == 0, nil
	})
	// An interruption is reported as such, not as a timeout.
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	if len(pending) == 0 {
		fmt.Fprintf(o.Out, "Confirmed by the Cluster Agent for all %d DatadogPodAutoscaler(s).\n", total)
		return 0, nil
	}
	prefix := ""
	if confirmation.Advisory {
		prefix = "Warning: "
	}
	if !listed {
		// Every List failed: a real API problem (RBAC, connectivity) must stay
		// visible, and nothing is known about the Cluster Agent.
		fmt.Fprintf(o.ErrOut, "%sUnable to read the DatadogPodAutoscaler status: %v\n", prefix, firstErr)
		o.listUnconfirmed(prefix, pending)
		return len(pending), nil
	}
	var set, removed []change
	for _, ch := range pending {
		if ch.value.Remove {
			removed = append(removed, ch)
		} else {
			set = append(set, ch)
		}
	}
	if len(set) > 0 {
		o.listUnconfirmed(prefix, set)
		fmt.Fprintf(o.ErrOut, "%s is not reported: the Cluster Agent is older than %s, or has not reconciled these objects yet.\n", confirmation.Name, MinClusterAgentVersion)
		if confirmation.NotReported != "" {
			fmt.Fprintln(o.ErrOut, confirmation.NotReported)
		}
		fmt.Fprintln(o.ErrOut, "Check with: kubectl describe dpa <name>")
	}
	if len(removed) > 0 {
		o.listUnconfirmed(prefix, removed)
		fmt.Fprintf(o.ErrOut, "%s is still reported: the Cluster Agent has not reconciled these objects yet.\n", confirmation.Name)
	}
	return len(pending), nil
}

func (o *annotationOptions) listUnconfirmed(prefix string, changes []change) {
	fmt.Fprintf(o.ErrOut, "%sNot confirmed by the Cluster Agent after %s:\n", prefix, o.timeout)
	for _, ch := range changes {
		fmt.Fprintf(o.ErrOut, "  • %s/%s\n", ch.dpa.Namespace, ch.dpa.Name)
	}
}
