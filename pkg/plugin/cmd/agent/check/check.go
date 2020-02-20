// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package check

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const maxParallel = 10

var (
	checkExample = `
  # check if the running Agents have detected check-config errors
  %[1]s check
`
)

// options provides information required by agent check command
type options struct {
	genericclioptions.IOStreams
	configFlags   *genericclioptions.ConfigFlags
	args          []string
	clientset     *kubernetes.Clientset
	restConfig    *restclient.Config
	userNamespace string
}

// newOptions provides an instance of options with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	return &options{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
	}
}

// New provides a cobra command wrapping options
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "check [flags]",
		Short:        "Check check-config errors",
		Example:      fmt.Sprintf(checkExample, "kubectl datadog agent"),
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

// complete sets all information required for processing the command
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	var err error

	clientConfig := o.configFlags.ToRawKubeConfigLoader()

	o.restConfig, err = o.configFlags.ToRawKubeConfigLoader().ClientConfig()
	if err != nil {
		return fmt.Errorf("unable to instantiate restConfig: %v", err)
	}

	o.clientset, err = common.NewClientset(clientConfig)
	if err != nil {
		return fmt.Errorf("unable to instantiate clientset: %v", err)
	}

	o.userNamespace, _, err = clientConfig.Namespace()
	if err != nil {
		return err
	}

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	if ns != "" {
		o.userNamespace = ns
	}

	return nil
}

// validate ensures that all required arguments and flag values are provided
func (o *options) validate() error {
	return nil
}

// run runs the check command
func (o *options) run() error {
	agentPods, err := o.getPods("agent")
	if err != nil {
		return fmt.Errorf("unable to get Agent pods: %v", err)
	}
	clcPods, err := o.getPods("cluster-checks-runner")
	if err != nil {
		return fmt.Errorf("unable to get Agent pods: %v", err)
	}
	statusCmd := []string{
		"bash",
		"-c",
		"curl -s -k -H \"Authorization: Bearer $(< /etc/datadog-agent/auth_token)\" https://127.0.0.1:5001/agent/status",
	}
	podErrors := make(map[string][]string)
	mutex := &sync.Mutex{}
	podChan := make(chan corev1.Pod, maxParallel)
	var wg sync.WaitGroup
	for i := 0; i < maxParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pod := range podChan {
				if pod.Status.Phase != "Running" {
					fmt.Println(fmt.Sprintf("Ignoring pod: %s, phase: %s", pod.Name, pod.Status.Phase))
					continue
				}
				stdOut, stdErr, err := o.execInPod(&pod, statusCmd)
				if err != nil {
					fmt.Println(fmt.Sprintf("Ignoring pod: %s, error: %v", pod.Name, err))
					continue
				}
				if stdErr != "" {
					fmt.Println(fmt.Sprintf("Ignoring pod %s, error: %s", pod.Name, stdErr))
					continue
				}
				errors, found, err := findErrors(stdOut)
				if err != nil {
					fmt.Println(fmt.Sprintf("Ignoring pod %s, error: %v", pod.Name, err))
				}
				if found {
					mutex.Lock()
					podErrors[pod.Name] = errors
					mutex.Unlock()
				}
			}
		}()
	}
	for _, p := range append(agentPods, clcPods...) {
		podChan <- p
	}
	close(podChan)
	wg.Wait()
	for podName, errors := range podErrors {
		fmt.Println(fmt.Sprintf("Errors reported by Agent %s", podName))
		for _, err := range errors {
			fmt.Println(err)
		}
		fmt.Printf("\n")
	}
	return nil
}

// execInPod exec a command in an Agent pod
func (o *options) execInPod(pod *corev1.Pod, cmd []string) (string, string, error) {
	req := o.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(o.userNamespace).
		SubResource("exec")

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return "", "", fmt.Errorf("error adding to scheme: %v", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(o.restConfig, "POST", req.URL())
	if err != nil {
		return "", "", err
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
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

// getPods return the list of the agent pods
func (o *options) getPods(component string) ([]corev1.Pod, error) {
	opts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("agent.datadoghq.com/component=%s", component),
	}
	podList, err := o.clientset.CoreV1().Pods(o.userNamespace).List(opts)
	if err != nil {
		return []corev1.Pod{}, err
	}
	if len(podList.Items) == 0 {
		return []corev1.Pod{}, fmt.Errorf("no agent pod found. Label selector used: %v", opts.LabelSelector)
	}
	return podList.Items, nil
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
