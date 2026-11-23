// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	leaderElectionID   = flare.LeaderElectionID
	operatorLabel      = "app.kubernetes.io/name=datadog-operator"
	defaultMetricsAddr = ":8080"
	// flareHostname is the hostname of flares sent with the plugin.
	flareHostname = "datadog-operator"
	// fallbackVersion builds the flare intake URL when the Operator version
	// is unknown. It isn't written to the manifest.
	fallbackVersion = "0.1.0"
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

	// The plugin only sees the Operator through the Kubernetes API, so it
	// has no Remote Configuration state.
	var operatorVersion *version.JSON
	pluginCollectors := []flare.Collector{
		{Name: "operator/version", Collect: func(ctx context.Context, c *flare.Collection) error {
			info, collectErr := o.collectVersion(ctx, c, leaderPod)
			operatorVersion = info
			return collectErr
		}},
		flare.OperatorPodConfigCollector(),
		flare.LeaderElectionCollector(leaderElectionID),
		{Name: "operator/health", Collect: func(ctx context.Context, c *flare.Collection) error {
			return o.collectHealth(ctx, c, leaderPod)
		}},
		{Name: "operator/metrics", Collect: func(ctx context.Context, c *flare.Collection) error {
			return o.collectMetrics(ctx, c, leaderPod)
		}},
		{Name: "operator/logs", Timeout: 60 * time.Second, Collect: func(ctx context.Context, c *flare.Collection) error {
			return collectOperatorLogs(ctx, c, o.Clientset, o.UserNamespace)
		}},
	}
	collectors := slices.Concat(pluginCollectors, flare.ConfigCollectors(), flare.DatadogResourcesCollectors(), []flare.Collector{flare.WorkloadsCollector(), flare.ClusterCollector()})

	manifest := flare.Run(ctx, env, archive, collectors, flare.RunOptions{})
	for _, result := range manifest.Collectors {
		if result.Status != flare.StatusOK {
			cmd.Println(fmt.Sprintf("Collector %s: %s %s", result.Name, result.Status, strings.Join(result.Errors, "; ")))
		}
	}

	// Only the site is known from the pod spec.
	manifest.Org = flare.PodOrgInfo(leaderPod)

	uploadVersion := fallbackVersion
	if operatorVersion != nil {
		manifest.OperatorVersion = operatorVersion.Version
		uploadVersion = operatorVersion.Version
	} else {
		cmd.Println(fmt.Sprintf("Couldn't get the Operator version, using %s to send the flare", fallbackVersion))
	}

	archivePath, err := archive.Close(manifest)
	if err != nil {
		return err
	}
	// The archive is in the same directory, so this is a rename.
	zipFilePath := getArchivePath()
	if err = os.Rename(archivePath, zipFilePath); err != nil {
		_ = os.Remove(archivePath)
		return err
	}

	// ask for confirmation before sending the flare file and opening the support ticket
	if !common.AskForConfirmation("Are you sure you want to upload a flare? [y/N]") {
		cmd.Println(fmt.Sprintf("Aborting. (You can still use %s)", zipFilePath))
		return nil
	}

	// Send the flare
	baseURL, err := flare.FlareURL(o.site, "", uploadVersion)
	if err != nil {
		return err
	}
	uploader := &flare.Uploader{BaseURL: baseURL, APIKey: apiKey}
	caseID, err := uploader.Upload(ctx, flare.UploadRequest{
		CaseID:          o.caseID,
		Email:           email,
		Hostname:        flareHostname,
		OperatorVersion: uploadVersion,
		ArchiveName:     filepath.Base(zipFilePath),
		ArchivePath:     zipFilePath,
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
	return flare.CollectOperatorPods(ctx, c, pods.Items)
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
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		for _, env := range container.Env {
			if env.Name == "DD_METRICS_ADDR" && env.Value != "" {
				addr = env.Value
			}
		}
		// Flags take precedence over the environment.
		if value, ok := flare.ContainerFlag(container, "metrics-addr"); ok {
			addr = value
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

// collectHealth collects the health of the leader pod: its conditions and
// probe results from the pod status, and the verbose health probe endpoints
// through the API server proxy.
func (o *options) collectHealth(ctx context.Context, c *flare.Collection, pod *corev1.Pod) error {
	if pod == nil {
		return errors.New("nil leader pod")
	}
	if err := c.AddYAML("operator/health-pod-status.yaml", flare.SummarizePodHealth(pod)); err != nil {
		return err
	}
	port := flare.HealthProbePort(pod)
	return flare.CollectHealthEndpoints(ctx, c, func(ctx context.Context, endpoint string) (string, []byte, error) {
		return o.proxyGet(ctx, pod, port, endpoint)
	})
}

// proxyGet reads an HTTP endpoint of a pod through the API server proxy. It
// returns the HTTP status and body of the pod's answer, even when it isn't
// a success.
func (o *options) proxyGet(ctx context.Context, pod *corev1.Pod, port, endpoint string) (string, []byte, error) {
	endpointPath, query, _ := strings.Cut(endpoint, "?")
	request := o.Clientset.CoreV1().RESTClient().Get().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(fmt.Sprintf("%s:%s", pod.Name, port)).
		SubResource("proxy").
		Suffix(strings.TrimPrefix(endpointPath, "/"))
	for param := range strings.SplitSeq(query, "&") {
		if param == "" {
			continue
		}
		name, value, _ := strings.Cut(param, "=")
		request = request.Param(name, value)
	}
	result := request.Do(ctx)
	code := 0
	result.StatusCode(&code)
	body, err := result.Raw()
	// client-go doesn't set the status code of plain text errors, such as
	// a 404 from the pod: it's in the error.
	var status apierrors.APIStatus
	if code == 0 && errors.As(err, &status) {
		code = int(status.Status().Code)
	}
	if code == 0 {
		if err == nil {
			err = errors.New("no HTTP status")
		}
		return "", nil, err
	}
	return fmt.Sprintf("%d %s", code, http.StatusText(code)), body, nil
}

// versionCommands are run in the Operator container to get its version:
// the json format first, then the text format of Operators without
// --version-format. The binary is exec'd directly: the image has no shell.
var versionCommands = [][]string{
	{flare.OperatorBinary, "--version", "--version-format", "json"},
	{flare.OperatorBinary, "--version"},
}

// collectVersion writes the version of the leader pod, from its binary or
// else its image tag, to operator/version.yaml.
func (o *options) collectVersion(ctx context.Context, c *flare.Collection, pod *corev1.Pod) (*version.JSON, error) {
	info, source, err := resolveVersion(ctx, pod, o.execInPod)
	if err != nil {
		return nil, err
	}
	return writeVersion(c, pod, info, source)
}

// writeVersion writes operator/version.yaml and returns the version,
// normalized.
func writeVersion(c *flare.Collection, pod *corev1.Pod, info *version.JSON, source string) (*version.JSON, error) {
	if err := c.AddYAML("operator/version.yaml", map[string]any{
		"pod":       pod.Name,
		"source":    source,
		"version":   info.Version,
		"commit":    info.Commit,
		"buildTime": info.BuildTime,
		"goVersion": info.Go,
		"platform":  info.Os,
	}); err != nil {
		return nil, err
	}
	info.Version = version.Normalize(info.Version)
	return info, nil
}

// execFunc runs a command in a container of a pod and returns its stdout.
type execFunc func(ctx context.Context, pod *corev1.Pod, container string, command []string) ([]byte, error)

// resolveVersion returns the version of an Operator pod, and where it was
// read from.
func resolveVersion(ctx context.Context, pod *corev1.Pod, exec execFunc) (*version.JSON, string, error) {
	container := flare.OperatorContainer(pod)
	if container == nil {
		return nil, "", errors.New("nil leader pod")
	}
	var errs []error
	for _, command := range versionCommands {
		out, err := exec(ctx, pod, container.Name, command)
		if err == nil {
			var info version.JSON
			if info, err = version.Parse(out); err == nil {
				return &info, "exec " + strings.Join(command, " "), nil
			}
		}
		errs = append(errs, fmt.Errorf("%s: %w", strings.Join(command, " "), err))
	}
	if tag := imageTag(container.Image); tag != "" {
		return &version.JSON{Version: tag}, "image tag of " + container.Image, nil
	}
	return nil, "", fmt.Errorf("unable to get the Operator version: %w", errors.Join(errs...))
}

// imageTag returns the tag of an image if it looks like a version, such as
// 1.22.0 for gcr.io/datadoghq/operator:1.22.0.
func imageTag(image string) string {
	image, _, _ = strings.Cut(image, "@")
	slash := strings.LastIndex(image, "/")
	colon := strings.LastIndex(image, ":")
	if colon <= slash {
		return ""
	}
	tag := image[colon+1:]
	if !imageVersionRegexp.MatchString(tag) {
		return ""
	}
	return tag
}

var imageVersionRegexp = regexp.MustCompile(`^v?\d+\.\d+\.\d+`)

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

// execInPod runs a command in a container of a pod and returns its stdout.
func (o *options) execInPod(ctx context.Context, pod *corev1.Pod, container string, command []string) ([]byte, error) {
	req := o.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")

	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		return []byte{}, err
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   command,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	restConfig, err := o.ConfigFlags.ToRESTConfig()
	if err != nil {
		return []byte{}, err
	}

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return []byte{}, err
	}

	var stdout, stderr bytes.Buffer
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr}); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			if len(msg) > 300 {
				msg = msg[:300] + "..."
			}
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
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
