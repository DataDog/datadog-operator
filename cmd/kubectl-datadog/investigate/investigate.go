package investigate

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

// options provides information required by Datadog investigate command
type options struct {
	genericclioptions.IOStreams
	common.Options

	args []string

	agentPodName string
}

const investigateExample = `
# run investigation flare for an existing case 123 (api key from stdin)
%[1]s flare 123 --email foo@bar.com

# send flare and create a new case (email and api key from stdin)
%[1]s flare
`

// newOptions provides an instance of options with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "flare" sub command
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "investigate [datadog-agent pod name]",
		Short:        "Run banch of command to provide a first investigation level",
		Example:      fmt.Sprintf(investigateExample, "kubectl datadog"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.complete(c, args); err != nil {
				return err
			}
			if err := o.validate(); err != nil {
				return err
			}
			return o.run(c)
		},
	}

	cmd.Flags().StringVarP(&o.agentPodName, "pod-name", "p", "", "pod name")
	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args

	err := o.Init(cmd)
	if err != nil {
		return err
	}

	return nil
}

// validate ensures that all required arguments and flag values are provided
func (o *options) validate() error {
	return nil
}

// run runs the flare command
func (o *options) run(cmd *cobra.Command) error {

	return o.getNodePods(cmd)
}

func (o *options) getNodePods(cmd *cobra.Command) error {
	cmd.Context()
	agentPod, err := o.Clientset.CoreV1().Pods(o.UserNamespace).Get(cmd.Context(), o.agentPodName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("datadog agent pod %s/%s not found, err: %w", o.UserNamespace, o.agentPodName, err)
		}
		return fmt.Errorf("get pod, unknow error: %w", err)
	}

	nodeName := agentPod.Spec.NodeName

	agentNode, err := o.Clientset.CoreV1().Nodes().Get(cmd.Context(), nodeName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("node %s not found, err: %w", nodeName, err)
		}
		return fmt.Errorf("get node, unknow error: %w", err)
	}

	printTitle(o.Out, fmt.Sprintf("Node: %s", nodeName), 0)
	fmt.Fprintf(o.Out, "- hostIP: %s\n", agentPod.Status.HostIP)

	printSubTitle(o.Out, "Node Conditions", 0, false)
	for _, condition := range agentNode.Status.Conditions {
		fmt.Fprintf(o.Out, "- %s: %s %s\n", condition.Reason, condition.Type, condition.Status)
	}

	_ = o.getNodeEvents(cmd.Context(), nodeName)

	_ = o.getAgentPodEvents(cmd.Context(), o.agentPodName)

	podsNodeList, err := o.Clientset.CoreV1().Pods("").List(cmd.Context(), v1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("pods on node %s not found, err: %w", nodeName, err)
		}
		return fmt.Errorf("pods on node, unknow error: %w", err)
	}
	printPodsInfo(o.Out, *podsNodeList)

	return nil
}

func (o *options) getNodeEvents(context context.Context, nodeName string) error {
	printSubTitle(o.Out, "Node Events", 0, true)
	nodeEvents, err := o.Clientset.EventsV1().Events("").List(context, v1.ListOptions{
		FieldSelector: "involvedObject.kind=Node",
	})

	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Fprintf(o.Out, "node's events (node:%s) not found\n", nodeName)
			return nil
		}
		return fmt.Errorf("node's events, unknow error: %w", err)
	}
	for _, event := range nodeEvents.Items {
		fmt.Fprintf(o.Out, "- %s: action:%s reason:%s controller:%s\n", event.Name, event.Action, event.Reason, event.ReportingController)
	}
	return nil
}

func (o *options) getAgentPodEvents(context context.Context, podName string) error {
	printSubTitle(o.Out, "Datadog Agent pod Events", 0, true)

	fieldSelector := fields.Set{
		"involvedObject.kind": "Pod",
		//"involvedObject.name":      podName,
		//"involvedObject.namespace": o.UserNamespace,
	}.AsSelector()

	fmt.Fprintln(o.Out, "CEDTEST:", fieldSelector.String(), "namespace", o.UserNamespace)
	podEvents, err := o.Clientset.EventsV1beta1().Events(o.UserNamespace).List(context, v1.ListOptions{
		FieldSelector: fieldSelector.String(),
	})
	fmt.Fprintln(o.Out, "CEDTEST:", len(podEvents.Items))
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Fprintf(o.Out, "pod's events (pod:%s/%s) not found\n", o.UserNamespace, podName)
			return nil
		}
		return fmt.Errorf("pod's events, unknow error: %w", err)
	}
	for _, event := range podEvents.Items {
		fmt.Fprintf(o.Out, "- %s: action:%s reason:%s controller:%s\n", event.Name, event.Action, event.Reason, event.ReportingController)
	}
	return nil
}

func printPodsInfo(out io.Writer, podsList corev1.PodList) {
	printSubTitle(out, "Pods", 0, true)
	for _, pod := range podsList.Items {
		printPodInfo(out, &pod)
	}
}

func printPodInfo(out io.Writer, pod *corev1.Pod) {
	fmt.Fprintf(out, "- Pod: %s/%s\n", pod.Namespace, pod.Name)
	fmt.Fprintf(out, "  Phase:  %s\n", pod.Status.Phase)
	fmt.Fprintf(out, "  QOSClass:  %s\n", pod.Status.QOSClass)

	conditionsList := make([]string, 0, len(pod.Status.Conditions))
	for _, condition := range pod.Status.Conditions {
		conditionsList = append(conditionsList, fmt.Sprintf("%s:%s", condition.Type, condition.Status))
	}
	fmt.Fprintf(out, "  Conditions:  %s\n", strings.Join(conditionsList, ", "))
	fmt.Fprintln(out, "  Containers:")
	for _, container := range pod.Status.ContainerStatuses {
		status := "ready"
		if !container.Ready {
			status = "unready"
		}
		fmt.Fprintf(out, "  - %s: [%s] [restart:%v] [state:%s]\n", container.Name, status, container.RestartCount, buildContainerState(container.State))
		if container.LastTerminationState.Terminated != nil {
			fmt.Fprintf(out, "    lastTerminatedState:%s\n", buildContainerState(container.LastTerminationState))
		}
	}
}

func buildContainerState(state corev1.ContainerState) string {
	stateString := ""
	if state.Running != nil {
		stateString = "running"
	} else if state.Waiting != nil {
		stateString = fmt.Sprintf("waiting: {reason:%s}", state.Waiting.Reason)
	} else if state.Terminated != nil {
		stateString = fmt.Sprintf("terminated: {exitCode:%v, reason:%s}", state.Terminated.ExitCode, state.Terminated.Reason)
	}
	return stateString
}

func printTitle(out io.Writer, in string, indent int) {
	indentString := strings.Repeat(" ", indent)
	fmt.Fprintln(out, indentString, strings.Repeat("=", len(in)))
	fmt.Fprintln(out, indentString, in)
	fmt.Fprintln(out, indentString, strings.Repeat("=", len(in)))
}

func printSubTitle(out io.Writer, in string, indent int, preLine bool) {
	indentString := ""
	if indent > 0 {
		indentString = strings.Repeat(" ", indent-1)
	}
	if preLine {
		fmt.Fprintln(out, "")
	}
	fmt.Fprintln(out, indentString, in)
	fmt.Fprintln(out, indentString, strings.Repeat("-", len(in)))
}
