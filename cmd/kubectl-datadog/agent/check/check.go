// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

const (
	maxParallel = 10
)

var (
	podName       string
	containerName string
	node          string
	checkExample  = `
  # check if the running Agents have detected check errors
  %[1]s check

  # check if the Agent foo has detected check errors
  %[1]s check --pod-name foo

  # check if the Agent running on node bar has detected check errors
  # if both --pod-name and --node flags are present, the --node flag is ignored
  %[1]s check --node bar
`
)

// options provides information required by agent check command
type options struct {
	genericclioptions.IOStreams
	common.Options
	args       []string
	restConfig *restclient.Config
}

// newOptions provides an instance of options with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()

	return o
}

// New provides a cobra command wrapping options for "check" sub command
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "check [flags]",
		Short:        "Find check errors",
		Example:      fmt.Sprintf(checkExample, "kubectl datadog agent"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.complete(c, args); err != nil {
				return err
			}

			if err := o.validate(c); err != nil {
				return err
			}

			return o.run(c)
		},
	}

	cmd.Flags().StringVarP(&podName, "pod-name", "p", "", "The Pod name of the Agent to check")
	cmd.Flags().StringVarP(&containerName, "container-name", "c", "agent", "The container name of the Agent to check (default: agent for agent pod and cluster-checks-runner for cluster check runners)")
	cmd.Flags().StringVarP(&node, "node", "", "", "The node name where the Agent is running")

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	var err error
	o.restConfig, err = o.ConfigFlags.ToRawKubeConfigLoader().ClientConfig()
	if err != nil {
		return fmt.Errorf("unable to instantiate restConfig: %w", err)
	}

	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided
func (o *options) validate(cmd *cobra.Command) error {
	if podName != "" && node != "" {
		cmd.Println("pod-name and node flags are both set, ignoring the node flag")
	}

	return nil
}

// run runs the check command
func (o *options) run(cmd *cobra.Command) error {
	var goRoutinesCount int
	var pods []corev1.Pod

	switch {
	case podName != "":
		var err error
		pod, err := o.getPodByName(podName)
		if err != nil {
			return fmt.Errorf("unable to get Agent pod: %w", err)
		}
		pods = append(pods, pod)
		cmd.Println(fmt.Sprintf("Agent %s found", podName))
		goRoutinesCount = 1
	case node != "":
		var err error
		pods, err = o.getPodsByOptions(metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", node),
			LabelSelector: common.AgentLabel,
		})
		if err != nil {
			return fmt.Errorf("unable to get Agent pods: %w", err)
		}
		cmd.Println(fmt.Sprintf("Agents running on node %s found", node))
		goRoutinesCount = 1
	default:
		var err error
		pods, err = o.getPodsByOptions(metav1.ListOptions{
			LabelSelector: common.AgentLabel,
		})
		if err != nil {
			return fmt.Errorf("unable to get Agent pods: %w", err)
		}
		clcPods, err := o.getPodsByOptions(metav1.ListOptions{
			LabelSelector: common.ClcRunnerLabel,
		})
		if err != nil {
			return fmt.Errorf("unable to get Agent pods: %w", err)
		}
		cmd.Println(fmt.Sprintf("Found %d node Agents and %d Cluster-Check Runners", len(pods), len(clcPods)))
		pods = append(pods, clcPods...)
		goRoutinesCount = maxParallel
	}

	statusCmd := []string{
		"bash",
		"-c",
		"DD_LOG_LEVEL=off agent status --json",
	}
	podErrors := make(map[string][]string)
	mutex := &sync.Mutex{}
	podChan := make(chan corev1.Pod, maxParallel)
	var wg sync.WaitGroup
	for i := 0; i < goRoutinesCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pod := range podChan {
				if pod.Status.Phase != "Running" {
					cmd.Println(fmt.Sprintf("Ignoring pod: %s, phase: %s", pod.Name, pod.Status.Phase))
					continue
				}
				container := containerName
				if isCLCRunner(pod) {
					container = "cluster-checks-runner"
				}
				stdOut, stdErr, err := o.execInPod(&pod, statusCmd, container)
				if err != nil {
					cmd.Println(fmt.Sprintf("Ignoring pod: %s, error: %v", pod.Name, err))
					continue
				}
				if stdErr != "" {
					cmd.Println(fmt.Sprintf("Ignoring pod %s, error: %s", pod.Name, stdErr))
					continue
				}
				errors, found, err := findErrors(stdOut)
				if err != nil {
					cmd.Println(fmt.Sprintf("Ignoring pod %s, error: %v", pod.Name, err))
				}
				if found {
					mutex.Lock()
					podErrors[pod.Name] = errors
					mutex.Unlock()
				}
			}
		}()
	}

	for _, p := range pods {
		podChan <- p
	}
	close(podChan)
	wg.Wait()

	for podName, errors := range podErrors {
		cmd.Println(fmt.Sprintf("Errors reported by Agent %s", podName))
		for _, err := range errors {
			cmd.Println(err)
		}
		fmt.Printf("\n") //nolint:forbidigo
	}

	return nil
}

// execInPod exec a command in an Agent pod
func (o *options) execInPod(pod *corev1.Pod, cmd []string, container string) (string, string, error) {
	req := o.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(o.UserNamespace).
		SubResource("exec")

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return "", "", fmt.Errorf("error adding to scheme: %w", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   cmd,
		Container: container,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(o.restConfig, "POST", req.URL())
	if err != nil {
		return "", "", err
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return "", "", err
	}

	return stdout.String(), stderr.String(), nil
}

// getPodsByOptions returns a list of the pods by ListOptions
func (o *options) getPodsByOptions(opts metav1.ListOptions) ([]corev1.Pod, error) {
	podList, err := o.Clientset.CoreV1().Pods(o.UserNamespace).List(context.TODO(), opts)
	if err != nil {
		return []corev1.Pod{}, err
	}

	return podList.Items, nil
}

// getPodByName returns a pod by name
func (o *options) getPodByName(name string) (corev1.Pod, error) {
	pod, err := o.Clientset.CoreV1().Pods(o.UserNamespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return corev1.Pod{}, err
	}

	return *pod, nil
}

func findErrors(statusJSON string) ([]string, bool, error) {
	status := AgentStatus{}
	if err := json.Unmarshal([]byte(statusJSON), &status); err != nil {
		return nil, false, err
	}
	errors := []string{}
	for _, check := range status.RunnerStats.Checks {
		for checkName, stat := range check {
			if stat.LastError != "" {
				errMessage := ""
				lastError := []Error{}
				err := json.Unmarshal([]byte(stat.LastError), &lastError)
				if err != nil {
					errMessage = stat.LastError
				} else {
					errMessage = lastError[0].Message
				}
				errors = append(errors, fmt.Sprintf("%s:%s", checkName, errMessage))
			}
		}
	}

	return errors, len(errors) > 0, nil
}

func isCLCRunner(pod corev1.Pod) bool {
	if value, found := pod.GetLabels()[common.ComponentLabelKey]; found && value == common.ClcRunnerLabelValue {
		return true
	}

	return false
}
