// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package flare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	"github.com/DataDog/datadog-operator/version"

	"github.com/mholt/archiver"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	httpTimeout = 60 * time.Second
	flareURL    = "https://%s-flare.agent.datadoghq.%s/support/flare"
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
	configFlags   *genericclioptions.ConfigFlags
	args          []string
	client        client.Client
	clientset     *kubernetes.Clientset
	zip           *archiver.Zip
	site          string
	userNamespace string
	caseID        string
}

// newOptions provides an instance of options with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	return &options{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
		zip:         archiver.NewZip(),
	}
}

// New provides a cobra command wrapping options
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "flare [Case ID]",
		Short:        "Collect a Datadog's Operator flare and send it to Datadog",
		Example:      fmt.Sprintf(flareExample, "kubectl dd"),
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

	cmd.Flags().StringVarP(&email, "email", "e", "", "Your email")
	cmd.Flags().StringVarP(&apiKey, "apiKey", "k", "", "Your api key, could also be taken from stdin")
	cmd.Flags().StringVarP(&ddSite, "ddSite", "d", "us", "Your Datadog site US or EU (default: US)")

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	var err error

	clientConfig := o.configFlags.ToRawKubeConfigLoader()

	// Create the Client for Read/Write operations.
	o.client, err = common.NewClient(clientConfig)
	if err != nil {
		return fmt.Errorf("unable to instantiate client: %v", err)
	}

	// Create the Clientset for pod logss collection.
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
		ddSite, err = common.AskForInput("Please enter your Datatod site [us/eu] (default us): ")
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
		o.site = "com"
	case "e", "eu", "EU":
		o.site = "eu"
	default:
		return fmt.Errorf("invalid datadog site %s", ddSite)
	}

	return nil
}

// run runs the flare command
func (o *options) run() error {
	// Prepare base directory
	baseDir := filepath.Join(os.TempDir(), "datadog-operator")
	if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
		return err
	}

	// Collect the existing datadogagent custom resource definitons
	if err := o.createCRFiles(baseDir); err != nil {
		fmt.Println(fmt.Sprintf("Couldn't collect custom resources: %v", err))
	}

	// Collect logs from all operator pods
	if err := o.createLogFiles(baseDir); err != nil {
		fmt.Println(fmt.Sprintf("Couldn't collect log files: %v", err))
	}

	// Collect operator deployment template
	if err := o.createDeploymentTemplate(baseDir); err != nil {
		fmt.Println(fmt.Sprintf("Couldn't collect deployment template: %v", err))
	}

	// Identify the leader pod
	leaderPod, err := o.getLeader()
	if err != nil {
		fmt.Println(fmt.Sprintf("Couldn't identify operator leader pod: %v", err))
	}

	// Collect leader metrics
	if err = o.createMetricsFile(leaderPod, baseDir); err != nil {
		fmt.Println(fmt.Sprintf("Couldn't collect operator pod metrics: %v", err))
	}

	// Collect leader status
	if err = o.createStatusFile(leaderPod, baseDir); err != nil {
		fmt.Println(fmt.Sprintf("Couldn't collect operator pod status: %v", err))
	}

	// Collect operator version
	if err = o.createVersionFile(leaderPod, baseDir); err != nil {
		fmt.Println(fmt.Sprintf("Couldn't collect operator version: %v", err))
	}

	// Create zip with the collected files
	zipFilePath := getArchivePath()
	if err = o.zip.Archive([]string{baseDir}, zipFilePath); err != nil {
		return err
	}

	// Get the operator version
	version, err := o.getVersion(leaderPod)
	if err != nil {
		fmt.Println(fmt.Sprintf("Couldn't get operator version: %v", err))

		// Fallback to a default version used to build the flare URL
		version = "0.1.0"
	}

	// ask for confirmation before sending the flare file and opening the support ticket
	if !common.AskForConfirmation("Are you sure you want to upload a flare? [y/N]") {
		fmt.Println(fmt.Sprintf("Aborting. (You can still use %s)", zipFilePath))
		return nil
	}

	// Send the flare
	caseID, err := o.sendFlare(zipFilePath, version)
	if err != nil {
		return err
	}

	fmt.Println("Flare were successfully uploaded. For future reference, your internal case id is", caseID)
	return nil
}

// createCRFiles gets the available datadogagent custom resource definitions
func (o *options) createCRFiles(dir string) error {
	// List all custom resources
	ddList := &v1alpha1.DatadogAgentList{}
	if err := o.client.List(context.TODO(), ddList, &client.ListOptions{Namespace: o.userNamespace}); err != nil {
		return err
	}
	if len(ddList.Items) == 0 {
		return errors.New("custom resources not found")
	}

	// Get custom resources yaml
	template, err := yaml.Marshal(ddList.Items)
	if err != nil {
		return err
	}

	return redactAndSave(filepath.Join(dir, "datadog-custom-resources.yaml"), template)
}

// redactAndSave uses a redacting writer to write a new file
func redactAndSave(filePath string, data []byte) error {
	file, err := createFile(filePath)
	if err != nil {
		return err
	}
	defer func() {
		if err = file.Close(); err != nil {
			fmt.Println(fmt.Sprintf("Couldn't close file: %v", err))
		}
	}()

	writer := newRedactingWriter(file)

	_, err = writer.Write(data)
	return err
}

// createLogFiles gets log files of the operator pods
func (o *options) createLogFiles(dir string) error {
	// List all Datadog operator pods
	podOpts := metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=datadog-operator",
	}
	pods, err := o.clientset.CoreV1().Pods(o.userNamespace).List(podOpts)
	if err != nil {
		return err
	}

	// Create log files for all the pods found
	for _, pod := range pods.Items {
		err := o.savePodLogs(pod, dir)
		if err != nil {
			fmt.Println(fmt.Sprintf("Skipping logs of pod %s: %v", pod.Name, err))
			continue
		}
	}

	return nil
}

// createDeploymentTemplate gets the deployment template of the operator
func (o *options) createDeploymentTemplate(dir string) error {
	// Get Datadog operator deployment
	deploy, err := o.clientset.ExtensionsV1beta1().Deployments(o.userNamespace).Get("datadog-operator", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Get deployment template file
	template, err := yaml.Marshal(deploy)
	if err != nil {
		return err
	}

	return redactAndSave(filepath.Join(dir, "datadog-operator-deployment.yaml"), template)
}

// createMetricsFile gets metrics payload and stores it in a file
func (o *options) createMetricsFile(pod *corev1.Pod, dir string) error {
	if pod == nil {
		return errors.New("nil leader pod")
	}

	// Query the /metrics endpoint of the leader pod
	result := o.clientset.CoreV1().RESTClient().Get().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(fmt.Sprintf("%s:8383", pod.Name)).
		SubResource("proxy").
		Suffix("metrics").
		Do()

	metrics, err := result.Raw()
	if err != nil {
		return err
	}

	return redactAndSave(filepath.Join(dir, fmt.Sprintf("%s-metrics.txt", pod.Name)), metrics)
}

// createStatusFile gets status of a pod and stores it in a file
func (o *options) createStatusFile(pod *corev1.Pod, dir string) error {
	if pod == nil {
		return errors.New("nil leader pod")
	}

	// Get pod status in yaml
	status, err := yaml.Marshal(pod.Status)
	if err != nil {
		return err
	}

	return redactAndSave(filepath.Join(dir, fmt.Sprintf("%s-status.txt", pod.Name)), status)
}

// createVersionFile gets the version from the operator pod and stores it in a file
func (o *options) createVersionFile(pod *corev1.Pod, dir string) error {
	if pod == nil {
		return errors.New("nil leader pod")
	}

	// Prepare command and execute it
	versionCmd := []string{"bash", "-c", "/usr/local/bin/datadog-operator --version --version-format text"}
	version, err := o.execInPod(versionCmd, pod)
	if err != nil {
		return err
	}

	return redactAndSave(filepath.Join(dir, fmt.Sprintf("%s-version.txt", pod.Name)), version)
}

// getOperatorVersion gets the version from the operator pod
func (o *options) getVersion(pod *corev1.Pod) (string, error) {
	if pod == nil {
		return "", errors.New("nil leader pod")
	}

	// Prepare command and execute it
	versionCmd := []string{"bash", "-c", "/usr/local/bin/datadog-operator --version --version-format json"}
	versionJSON, err := o.execInPod(versionCmd, pod)
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

	version := strings.Split(decoded.Version, "_")[0]
	return strings.TrimLeft(version, "v"), nil
}

func (o *options) getLeader() (*corev1.Pod, error) {
	// Get operator lock configmap to identify the leader
	cm, err := o.clientset.CoreV1().ConfigMaps(o.userNamespace).Get("datadog-operator-lock", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Get leader from owner refs
	leaderName := ""
	for _, ref := range cm.GetOwnerReferences() {
		leaderName = ref.Name
		break
	}

	if leaderName == "" {
		return nil, errors.New("leader name not found in lock")
	}

	// Get operator leader pod
	return o.clientset.CoreV1().Pods(o.userNamespace).Get(leaderName, metav1.GetOptions{})
}

// execInPod execs a given command in a given pod
func (o *options) execInPod(command []string, pod *corev1.Pod) ([]byte, error) {
	req := o.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(o.userNamespace).
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

	restConfig, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return []byte{}, err
	}

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return []byte{}, err
	}

	var stdout bytes.Buffer
	if err := exec.Stream(remotecommand.StreamOptions{Stdout: &stdout}); err != nil {
		return []byte{}, err
	}

	return stdout.Bytes(), nil
}

// savePodLogs retrieves pod logs and save them in a file
func (o *options) savePodLogs(pod corev1.Pod, dir string) error {
	podLogOpts := corev1.PodLogOptions{}
	req := o.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs, err := req.Stream()
	if err != nil {
		return err
	}
	defer func() {
		if err = podLogs.Close(); err != nil {
			fmt.Println(fmt.Sprintf("Couldn't close pod-logs stream: %v", err))
		}
	}()

	// Convert podLogs stream into bytes
	logBytes, err := common.StreamToBytes(podLogs)
	if err != nil {
		return err
	}

	return redactAndSave(filepath.Join(dir, fmt.Sprintf("%s.json", pod.Name)), logBytes)
}

// getArchivePath builds the zip file path in a temporary directory
func getArchivePath() string {
	timeString := time.Now().Format("2006-01-02-15-04-05")
	fileName := strings.Join([]string{"datadog", "operator", timeString}, "-")
	fileName = strings.Join([]string{fileName, "zip"}, ".")
	return filepath.Join(os.TempDir(), fileName)
}

func createFile(path string) (*os.File, error) {
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	return os.OpenFile(path, flags, 0644)
}

type flareResponse struct {
	CaseID int    `json:"case_id,omitempty"`
	Error  string `json:"error,omitempty"`
}

// sendFlare sends a flare to Datadog
func (o *options) sendFlare(archivePath, version string) (string, error) {
	url, err := o.buildFlareURL(version)
	if err != nil {
		return "", err
	}

	r, err := o.readAndPostFlareFile(archivePath, url, version)
	if err != nil {
		return "", err
	}
	defer func() {
		if err = r.Body.Close(); err != nil {
			fmt.Println(fmt.Sprintf("Couldn't close request body: %v", err))
		}
	}()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	flare := flareResponse{}
	if err := json.Unmarshal(body, &flare); err != nil {
		return "", err
	}

	if flare.Error != "" {
		return "", errors.New(flare.Error)
	}

	return strconv.Itoa(flare.CaseID), nil
}

// readAndPostFlareFile prepares request and post the flare to Datadog
func (o *options) readAndPostFlareFile(archivePath, url, version string) (*http.Response, error) {
	request, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}

	// We need to set the Content-Type header here, but we still haven't created the writer
	// to obtain it from. Here we create one which only purpose is to give us a proper
	// Content-Type. Note that this Content-Type header will contain a random multipart
	// boundary, so we need to make sure that the actual writter uses the same boundary.
	boundaryWriter := multipart.NewWriter(nil)
	request.Header.Set("Content-Type", boundaryWriter.FormDataContentType())

	// Manually set the Body and ContentLenght. http.NewRequest doesn't do all of this
	// for us, since a PipeReader is not one of the Reader types it knows how to handle.
	request.Body, err = o.getFlareReader(boundaryWriter.Boundary(), archivePath, version)
	if err != nil {
		return nil, err
	}

	// -1 here means 'unknown' and makes this a 'chunked' request. See https://github.com/golang/go/issues/18117
	request.ContentLength = -1
	// Setting a GetBody function makes the request replayable in case there is a redirect.
	// Otherwise, since the body is a pipe, what has been already read can't be read again.
	request.GetBody = func() (io.ReadCloser, error) {
		return o.getFlareReader(boundaryWriter.Boundary(), archivePath, version)
	}

	client := &http.Client{
		Timeout: httpTimeout,
	}

	return client.Do(request)
}

func (o *options) getFlareReader(multipartBoundary, archivePath, version string) (io.ReadCloser, error) {
	// No need to close the reader, http.Client does it for us
	bodyReader, bodyWriter := io.Pipe()

	writer := multipart.NewWriter(bodyWriter)
	if err := writer.SetBoundary(multipartBoundary); err != nil {
		return nil, err
	}

	// Write stuff to the pipe will block until it is read from the other end, so we don't load everything in memory
	go func() {
		// defer order matters to avoid empty result when reading the form.
		defer func() {
			if err := bodyWriter.Close(); err != nil {
				fmt.Println(fmt.Sprintf("Couldn't close body writer: %v", err))
			}
		}()
		defer func() {
			if err := writer.Close(); err != nil {
				fmt.Println(fmt.Sprintf("Couldn't close writer: %v", err))
			}
		}()

		if o.caseID != "" {
			_ = writer.WriteField("case_id", o.caseID)
		}
		if email != "" {
			_ = writer.WriteField("email", email)
		}

		p, err := writer.CreateFormFile("flare_file", filepath.Base(archivePath))
		if err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}
		file, err := os.Open(archivePath)
		if err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}
		defer func() {
			if err = file.Close(); err != nil {
				fmt.Println(fmt.Sprintf("Couldn't close file: %v", err))
			}
		}()

		_, err = io.Copy(p, file)
		if err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}

		_ = writer.WriteField("operator_version", version)

		// Hostname discarded in operator flares
		_ = writer.WriteField("hostname", "datadog-operator")
	}()

	return bodyReader, nil
}

func (o *options) buildFlareURL(version string) (string, error) {
	url, err := url.Parse(fmt.Sprintf(flareURL, strings.ReplaceAll(version, ".", "-"), o.site))
	if err != nil {
		return "", err
	}

	if o.caseID != "" {
		url.Path = path.Join(url.Path, o.caseID)
	}

	query := url.Query()
	query.Set("api_key", apiKey)
	url.RawQuery = query.Encode()

	return url.String(), nil
}
