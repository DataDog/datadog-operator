// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/flare"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	leaderElectionID   = "datadog-operator-lock"
	operatorLabel      = "app.kubernetes.io/name=datadog-operator"
	defaultMetricsAddr = ":8080"
	// flareHostname is the hostname of flares sent with the plugin.
	flareHostname = "datadog-operator"
)

var (
	email        string
	apiKey       string
	ddSite       string
	flareExample = `
  # send flare for an existing case 123 (api key from stdin)
  %[1]s flare 123 --email foo@bar.com

  # send flare and create a new case (email and api key from stdin)
  %[1]s flare
`
)

// options provides information required by Datadog flare command
type options struct {
	genericclioptions.IOStreams
	common.Options
	args   []string
	site   string
	caseID string
}

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
		Use:          "flare [Case ID]",
		Short:        "Collect a Datadog's Operator flare and send it to Datadog",
		Example:      fmt.Sprintf(flareExample, "kubectl datadog"),
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

	cmd.Flags().StringVarP(&email, "email", "e", "", "Your email")
	cmd.Flags().StringVarP(&apiKey, "apiKey", "k", "", "Your api key, could also be taken from stdin")
	cmd.Flags().StringVarP(&ddSite, "ddSite", "d", "us", "Your Datadog site US or EU (default: US)")

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

	if len(args) > 0 {
		o.caseID = args[0]
	}

	if email == "" {
		email, err = common.AskForInput("Please enter your email: ")
		if err != nil {
			return err
		}
	}

	if apiKey == "" {
		apiKey, err = common.AskForInput("Please enter your api key: ")
		if err != nil {
			return err
		}
	}

	if ddSite == "" {
		ddSite, err = common.AskForInput("Please enter your Datatog site [us/eu] (default us): ")
		if err != nil {
			return err
		}
	}

	return nil
}

// validate ensures that all required arguments and flag values are provided
func (o *options) validate() error {
	if len(o.args) > 1 {
		return errors.New("either one or no arguments are allowed")
	}

	if email == "" {
		return errors.New("email is missing")
	}

	if apiKey == "" {
		return errors.New("apiKey is missing")
	}

	switch ddSite {
	case "", "u", "us", "US":
		o.site = "datadoghq.com"
	case "e", "eu", "EU":
		o.site = "datadoghq.eu"
	default:
		return fmt.Errorf("invalid datadog site %s", ddSite)
	}

	return nil
}

// run runs the flare command
func (o *options) run(cmd *cobra.Command) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Identify the leader pod
	leaderPod, err := getLeader(ctx, o.Client, o.Clientset, o.UserNamespace)
	if err != nil {
		cmd.Println(fmt.Sprintf("Couldn't identify operator leader pod: %v", err))
	}

	env := flare.Env{
		Reader:            o.Client,
		Clientset:         o.Clientset,
		OperatorNamespace: o.UserNamespace,
		OperatorPod:       leaderPod,
	}
	archive := flare.NewArchive(flare.ArchiveOptions{Root: "datadog-operator"})

	pluginCollectors := []flare.Collector{
		{Name: "operator/logs", Timeout: 60 * time.Second, Collect: func(ctx context.Context, c *flare.Collection) error {
			return collectOperatorLogs(ctx, c, o.Clientset, o.UserNamespace)
		}},
		{Name: "operator/metrics", Collect: func(ctx context.Context, c *flare.Collection) error {
			return o.collectMetrics(ctx, c, leaderPod)
		}},
		{Name: "operator/version", Collect: func(ctx context.Context, c *flare.Collection) error {
			return o.collectVersion(ctx, c, leaderPod)
		}},
	}
	collectors := slices.Concat(pluginCollectors, flare.ConfigCollectors(), flare.DatadogResourcesCollectors(), []flare.Collector{flare.WorkloadsCollector(), flare.ClusterCollector()})

	manifest := flare.Run(ctx, env, archive, collectors, flare.RunOptions{})
	for _, result := range manifest.Collectors {
		if result.Status != flare.StatusOK {
			cmd.Println(fmt.Sprintf("Collector %s: %s %s", result.Name, result.Status, strings.Join(result.Errors, "; ")))
		}
	}

	// Get the operator version
	operatorVersion, err := o.getVersion(ctx, leaderPod)
	if err != nil {
		cmd.Println(fmt.Sprintf("Couldn't get operator version: %v", err))

		// Fallback to a default version used to build the flare URL
		operatorVersion = "0.1.0"
	}
	manifest.OperatorVersion = operatorVersion

	data, err := archive.Close(manifest)
	if err != nil {
		return err
	}
	zipFilePath := getArchivePath()
	if err = os.WriteFile(zipFilePath, data, 0o600); err != nil {
		return err
	}

	// ask for confirmation before sending the flare file and opening the support ticket
	if !common.AskForConfirmation("Are you sure you want to upload a flare? [y/N]") {
		cmd.Println(fmt.Sprintf("Aborting. (You can still use %s)", zipFilePath))
		return nil
	}

	// Send the flare
	baseURL, err := flare.FlareURL(o.site, "", operatorVersion)
	if err != nil {
		return err
	}
	uploader := &flare.Uploader{BaseURL: baseURL, APIKey: apiKey}
	caseID, err := uploader.Upload(ctx, flare.UploadRequest{
		CaseID:          o.caseID,
		Email:           email,
		Hostname:        flareHostname,
		OperatorVersion: operatorVersion,
		ArchiveName:     filepath.Base(zipFilePath),
		Archive:         data,
	})
	if err != nil {
		return err
	}

	cmd.Println("Flare were successfully uploaded. For future reference, your internal case id is", caseID)
	return nil
}

// collectOperatorLogs collects the logs of all the Operator pods.
func collectOperatorLogs(ctx context.Context, c *flare.Collection, clientset kubernetes.Interface, namespace string) error {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: operatorLabel})
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no pod with label %s in namespace %s", operatorLabel, namespace)
	}
	summaries := make([]flare.PodSummary, 0, len(pods.Items))
	for i := range pods.Items {
		summaries = append(summaries, flare.SummarizePod(&pods.Items[i]))
		flare.CollectPodLogs(ctx, c, "operator/logs", &pods.Items[i])
	}
	return c.AddYAML("operator/pods.yaml", summaries)
}

// collectMetrics gets the metrics of the leader pod through the API server
// proxy, from the metrics port the Operator is configured with.
func (o *options) collectMetrics(ctx context.Context, c *flare.Collection, pod *corev1.Pod) error {
	if pod == nil {
		return errors.New("nil leader pod")
	}
	port, err := metricsPort(pod)
	if err != nil {
		return err
	}
	metrics, err := o.Clientset.CoreV1().RESTClient().Get().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(fmt.Sprintf("%s:%s", pod.Name, port)).
		SubResource("proxy").
		Suffix("metrics").
		Do(ctx).
		Raw()
	if err != nil {
		return err
	}
	return c.AddText(fmt.Sprintf("operator/%s-metrics.txt", pod.Name), metrics)
}

// metricsPort returns the port of the metrics endpoint of an Operator pod,
// from its -metrics-addr flag or DD_METRICS_ADDR environment variable.
func metricsPort(pod *corev1.Pod) (string, error) {
	addr := ""
	for _, container := range pod.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == "DD_METRICS_ADDR" && env.Value != "" {
				addr = env.Value
			}
		}
		// Flags take precedence over the environment.
		args := append(append([]string{}, container.Command...), container.Args...)
		for i, arg := range args {
			if !strings.HasPrefix(arg, "-") {
				continue
			}
			name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if name != "metrics-addr" {
				continue
			}
			if hasValue {
				addr = value
			} else if i+1 < len(args) {
				addr = args[i+1]
			}
		}
	}
	if addr == "" {
		addr = defaultMetricsAddr
	}
	if addr == "0" {
		return "", errors.New("the metrics endpoint of the Operator is disabled")
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid metrics address %q: %w", addr, err)
	}
	return port, nil
}

// collectVersion gets the version from the operator pod
func (o *options) collectVersion(ctx context.Context, c *flare.Collection, pod *corev1.Pod) error {
	if pod == nil {
		return errors.New("nil leader pod")
	}

	versionCmd := []string{"bash", "-c", "/usr/local/bin/datadog-operator --version --version-format text"}
	versionText, err := o.execInPod(ctx, versionCmd, pod)
	if err != nil {
		return err
	}

	return c.AddText(fmt.Sprintf("operator/%s-version.txt", pod.Name), versionText)
}

// getVersion gets the version from the operator pod
func (o *options) getVersion(ctx context.Context, pod *corev1.Pod) (string, error) {
	if pod == nil {
		return "", errors.New("nil leader pod")
	}

	// Prepare command and execute it
	versionCmd := []string{"bash", "-c", "/usr/local/bin/datadog-operator --version --version-format json"}
	versionJSON, err := o.execInPod(ctx, versionCmd, pod)
	if err != nil {
		return "", err
	}

	// Unmarshal payload
	decoded := version.JSON{}
	if err := json.Unmarshal(versionJSON, &decoded); err != nil {
		return "", err
	}
	if decoded.Error != "" {
		return "", errors.New(decoded.Error)
	}
	if decoded.Version == "" {
		return "", errors.New("empty version")
	}

	version, _, _ := strings.Cut(decoded.Version, "_")
	return strings.TrimLeft(version, "v"), nil
}

// getLeader returns the leader pod, from the leader election Lease, or the
// ConfigMap older Operators used.
func getLeader(ctx context.Context, reader client.Reader, clientset kubernetes.Interface, namespace string) (*corev1.Pod, error) {
	leaderName := ""
	lease := &coordinationv1.Lease{}
	if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: leaderElectionID}, lease); err == nil {
		if lease.Spec.HolderIdentity != nil {
			// The identity is <pod name>_<uuid>.
			leaderName, _, _ = strings.Cut(*lease.Spec.HolderIdentity, "_")
		}
	} else {
		cm, cmErr := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, leaderElectionID, metav1.GetOptions{})
		if cmErr != nil {
			return nil, errors.Join(err, cmErr)
		}
		for _, ref := range cm.GetOwnerReferences() {
			leaderName = ref.Name
			break
		}
	}

	if leaderName == "" {
		return nil, errors.New("leader name not found in lock")
	}

	// Get operator leader pod
	return clientset.CoreV1().Pods(namespace).Get(ctx, leaderName, metav1.GetOptions{})
}

// execInPod execs a given command in a given pod
func (o *options) execInPod(ctx context.Context, command []string, pod *corev1.Pod) ([]byte, error) {
	req := o.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(o.UserNamespace).
		SubResource("exec")

	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		return []byte{}, err
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Command: command,
		Stdin:   false,
		Stdout:  true,
		Stderr:  false,
		TTY:     false,
	}, parameterCodec)

	restConfig, err := o.ConfigFlags.ToRESTConfig()
	if err != nil {
		return []byte{}, err
	}

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return []byte{}, err
	}

	var stdout bytes.Buffer
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout}); err != nil {
		return []byte{}, err
	}

	return stdout.Bytes(), nil
}

// getArchivePath builds the zip file path in a temporary directory
func getArchivePath() string {
	timeString := time.Now().Format("2006-01-02-15-04-05")
	fileName := strings.Join([]string{"datadog", "operator", timeString}, "-")
	fileName = strings.Join([]string{fileName, "zip"}, ".")
	return filepath.Join(os.TempDir(), fileName)
}
