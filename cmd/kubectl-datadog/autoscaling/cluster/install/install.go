package install

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/davecgh/go-spew/spew"
	"github.com/fatih/color"
	"github.com/pkg/browser"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpawsv1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/registry"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/helm"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/k8s"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

const (
	karpenterOCIRegistry = "oci://public.ecr.aws/karpenter/karpenter"
)

var (
	//go:embed assets/dd-karpenter.yaml
	DdKarpenterCfn string

	//go:embed assets/karpenter.yaml
	KarpenterCfn string
)

// InferenceMethod defines how to infer EC2NodeClass and NodePool properties
type InferenceMethod string

const (
	// InferenceMethodNone does not infer any properties, creates empty resources
	InferenceMethodNone InferenceMethod = "none"
	// InferenceMethodNodes infers properties from existing Kubernetes nodes
	InferenceMethodNodes InferenceMethod = "nodes"
	// InferenceMethodNodeGroups infers properties from EKS node groups
	InferenceMethodNodeGroups InferenceMethod = "nodegroups"
)

// String returns the string representation of the InferenceMethod
func (i *InferenceMethod) String() string {
	return string(*i)
}

// Set sets the InferenceMethod value from a string
func (i *InferenceMethod) Set(s string) error {
	switch s {
	case "none", "nodes", "nodegroups":
		*i = InferenceMethod(s)
		return nil
	default:
		return fmt.Errorf("inference-method must be one of none, nodes or nodegroups")
	}
}

// Type returns the type name for pflag
func (_ *InferenceMethod) Type() string {
	return "InferenceMethod"
}

var (
	clusterName        string
	karpenterNamespace string
	karpenterVersion   string
	inferenceMethod    = InferenceMethodNodeGroups
	debug              bool
	installExample     = `
  # install autoscaling
  %[1]s install
`
)

type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string
}

func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "install",
		Short:        "Install autoscaling on an EKS cluster",
		Example:      fmt.Sprintf(installExample, "kubectl datadog autoscaling cluster"),
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

	cmd.Flags().StringVar(&clusterName, "cluster-name", "", "Name of the EKS cluster")
	cmd.Flags().StringVar(&karpenterNamespace, "karpenter-namespace", "dd-karpenter", "Name of the Kubernetes namespace to deploy Karpenter into")
	cmd.Flags().StringVar(&karpenterVersion, "karpenter-version", "", "Version of Karpenter to install (default to latest)")
	cmd.Flags().Var(&inferenceMethod, "inference-method", "Method to infer EC2NodeClass and NodePool properties: none, nodes, nodegroups")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logs")

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command.
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided.
func (o *options) validate() error {
	if len(o.args) > 0 {
		return errors.New("no arguments are allowed")
	}

	if !slices.Contains([]InferenceMethod{InferenceMethodNone, InferenceMethodNodes, InferenceMethodNodeGroups}, inferenceMethod) {
		return errors.New("inference-method must be one of none, nodes or nodegroups")
	}

	return nil
}

func (o *options) run(cmd *cobra.Command) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.SetOutput(cmd.OutOrStderr())

	if clusterName == "" {
		if name, err := o.getClusterNameFromKubeconfig(ctx); err != nil {
			return err
		} else if name != "" {
			clusterName = name
		} else {
			return errors.New("cluster name must be specified either via --cluster-name or in the current kubeconfig context")
		}
	}

	msg := "Installing Karpenter on cluster " + clusterName + "."
	cmd.Println("╭─" + strings.Repeat("─", len(msg)) + "─╮")
	cmd.Println("│ " + msg + " │")
	cmd.Println("╰─" + strings.Repeat("─", len(msg)) + "─╯")

	clients, err := o.buildClients(ctx)
	if err != nil {
		return fmt.Errorf("failed to build clients: %w", err)
	}

	if err = createCloudFormationStacks(ctx, clients, clusterName, karpenterNamespace); err != nil {
		return err
	}

	if err = updateAwsAuthConfigMap(ctx, clients, clusterName); err != nil {
		return err
	}

	if err = o.installHelmChart(ctx, clusterName, karpenterNamespace, karpenterVersion, debug); err != nil {
		return err
	}

	if err = createNodePoolResources(ctx, cmd, clients, clusterName, inferenceMethod, debug); err != nil {
		return err
	}

	return displaySuccessMessage(cmd, clusterName)
}

type clients struct {
	// AWS clients
	config         awssdk.Config
	cloudFormation *cloudformation.Client
	ec2            *ec2.Client
	eks            *eks.Client
	sts            *sts.Client

	// Kubernetes clients
	k8sClient    client.Client         // controller-runtime client
	k8sClientset *kubernetes.Clientset // typed Kubernetes client
}

func (o *options) getClusterNameFromKubeconfig(ctx context.Context) (string, error) {
	kubeRawConfig, err := o.ConfigFlags.ToRawKubeConfigLoader().RawConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get raw kubeconfig: %w", err)
	}

	kubeContext := ""
	if o.ConfigFlags.Context != nil {
		kubeContext = *o.ConfigFlags.Context
	}

	return guess.GetClusterNameFromKubeconfig(ctx, kubeRawConfig, kubeContext), nil
}

func (o *options) buildClients(ctx context.Context) (*clients, error) {
	// Load AWS config
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create AWS clients
	return &clients{
		config:         awsConfig,
		cloudFormation: cloudformation.NewFromConfig(awsConfig),
		ec2:            ec2.NewFromConfig(awsConfig),
		eks:            eks.NewFromConfig(awsConfig),
		sts:            sts.NewFromConfig(awsConfig),
		k8sClient:      o.Client,
		k8sClientset:   o.Clientset,
	}, nil
}

func createCloudFormationStacks(ctx context.Context, clients *clients, clusterName string, karpenterNamespace string) error {
	if err := aws.CreateOrUpdateStack(ctx, clients.cloudFormation, "dd-karpenter-"+clusterName+"-karpenter", KarpenterCfn, map[string]string{
		"ClusterName": clusterName,
	}); err != nil {
		return fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
	}

	isUnmanagedEKSPIAInstalled, err := guess.IsThereUnmanagedEKSPodIdentityAgentInstalled(ctx, clients.eks, clusterName)
	if err != nil {
		return fmt.Errorf("failed to check if EKS pod identity agent is installed: %w", err)
	}

	if err := aws.CreateOrUpdateStack(ctx, clients.cloudFormation, "dd-karpenter-"+clusterName+"-dd-karpenter", DdKarpenterCfn, map[string]string{
		"ClusterName":            clusterName,
		"KarpenterNamespace":     karpenterNamespace,
		"DeployPodIdentityAddon": strconv.FormatBool(!isUnmanagedEKSPIAInstalled),
	}); err != nil {
		return fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
	}

	return nil
}

func updateAwsAuthConfigMap(ctx context.Context, clients *clients, clusterName string) error {
	awsAuthConfigMapPresent, err := guess.IsAwsAuthConfigMapPresent(ctx, clients.k8sClientset)
	if err != nil {
		return fmt.Errorf("failed to check if aws-auth ConfigMap is present: %w", err)
	}

	if awsAuthConfigMapPresent {
		// Get AWS account ID
		callerIdentity, err := clients.sts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return fmt.Errorf("failed to get identity caller: %w", err)
		}
		if callerIdentity.Account == nil {
			return errors.New("unable to determine AWS account ID from STS GetCallerIdentity")
		}
		accountID := *callerIdentity.Account

		// Add role mapping in the `aws-auth` ConfigMap
		if err = aws.EnsureAwsAuthRole(ctx, clients.k8sClientset, aws.RoleMapping{
			RoleArn:  "arn:aws:iam::" + accountID + ":role/KarpenterNodeRole-" + clusterName,
			Username: "system:node:{{EC2PrivateDNSName}}",
			Groups:   []string{"system:bootstrappers", "system:nodes"},
		}); err != nil {
			return fmt.Errorf("failed to update aws-auth ConfigMap: %w", err)
		}
	}

	return nil
}

func (o *options) installHelmChart(ctx context.Context, clusterName string, karpenterNamespace string, karpenterVersion string, debug bool) error {
	kubeConfig := ""
	if o.ConfigFlags.KubeConfig != nil {
		kubeConfig = *o.ConfigFlags.KubeConfig
	}
	kubeContext := ""
	if o.ConfigFlags.Context != nil {
		kubeContext = *o.ConfigFlags.Context
	}
	restClientGetter := kube.GetConfig(kubeConfig, kubeContext, karpenterNamespace)
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(restClientGetter, karpenterNamespace, os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		return fmt.Errorf("failed to initialize Helm configuration: %w", err)
	}

	var err error
	if actionConfig.RegistryClient, err = registry.NewClient(
		registry.ClientOptDebug(debug),
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(log.Writer()),
	); err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	values := map[string]any{
		"settings": map[string]any{
			"clusterName":       clusterName,
			"interruptionQueue": clusterName,
		},
		"podAnnotations": map[string]any{
			"ad.datadoghq.com/controller.checks": `{
  "karpenter": {
    "init_config": {},
    "instances": [
      {
        "openmetrics_endpoint": "http://%%host%%:8080/metrics"
      }
    ]
  }
}`,
		},
	}

	if err = helm.CreateOrUpgrade(ctx, actionConfig, "karpenter", karpenterNamespace, karpenterOCIRegistry, karpenterVersion, values); err != nil {
		return fmt.Errorf("failed to create or update Helm release: %w", err)
	}

	return nil
}

func createNodePoolResources(ctx context.Context, cmd *cobra.Command, clients *clients, clusterName string, inferenceMethod InferenceMethod, debug bool) error {
	var nodePoolsSet *guess.NodePoolsSet
	var err error

	switch inferenceMethod {
	case InferenceMethodNone:
		cmd.Printf("Karpenter has been successfully installed, but no EC2NodeClass nor NodePool have been created yet. " +
			"Those objects are mandatory for Karpenter to be able to auto-scale the cluster. " +
			"Use --inference-method=nodes or --inference-method=nodegroups to create some " +
			"with reasonable defaults based on the existing nodes of the cluster.\n")
		return nil

	case InferenceMethodNodes:
		nodePoolsSet, err = guess.GetNodesProperties(ctx, clients.k8sClientset, clients.ec2)
		if err != nil {
			return fmt.Errorf("failed to gather nodes properies: %w", err)
		}

	case InferenceMethodNodeGroups:
		nodePoolsSet, err = guess.GetNodeGroupsProperties(ctx, clients.eks, clients.ec2, clusterName)
		if err != nil {
			return fmt.Errorf("failed to gather node groups properties: %w", err)
		}
	}

	if debug {
		cmd.Printf("Creating the following node pools:\n %s\n", spew.Sdump(nodePoolsSet))
	}

	sch := runtime.NewScheme()
	if err = scheme.AddToScheme(sch); err != nil {
		return fmt.Errorf("failed to add runtime scheme: %w", err)
	}
	sch.AddKnownTypes(
		schema.GroupVersion{Group: "karpenter.sh", Version: "v1"},
		&karpv1.NodePool{},
		&karpv1.NodePoolList{},
	)
	metav1.AddToGroupVersion(sch, schema.GroupVersion{Group: "karpenter.sh", Version: "v1"})
	sch.AddKnownTypes(
		schema.GroupVersion{Group: "karpenter.k8s.aws", Version: "v1"},
		&karpawsv1.EC2NodeClass{},
		&karpawsv1.EC2NodeClassList{},
	)
	metav1.AddToGroupVersion(sch, schema.GroupVersion{Group: "karpenter.k8s.aws", Version: "v1"})

	for _, nc := range nodePoolsSet.GetEC2NodeClasses() {
		if err = k8s.CreateOrUpdateEC2NodeClass(ctx, clients.k8sClient, clusterName, nc); err != nil {
			return fmt.Errorf("failed to create or update EC2NodeClass %s: %w", nc.GetName(), err)
		}
	}

	for _, np := range nodePoolsSet.GetNodePools() {
		if err = k8s.CreateOrUpdateNodePool(ctx, clients.k8sClient, np); err != nil {
			return fmt.Errorf("failed to create or update NodePool %s: %w", np.GetName(), err)
		}
	}

	return nil
}

func displaySuccessMessage(cmd *cobra.Command, clusterName string) error {
	autoscalingSettingsURL := (&url.URL{
		Scheme:   "https",
		Host:     "app.datadoghq.com",
		Path:     "orchestration/scaling/settings",
		RawQuery: url.Values{"query": []string{"kube_cluster_name:" + clusterName}}.Encode(),
	}).String()

	browser.Stdout = cmd.OutOrStdout()
	browser.Stderr = cmd.ErrOrStderr()
	if err := browser.OpenURL(autoscalingSettingsURL); err != nil {
		log.Printf("Failed to open URL in browser: %v", err)
	}

	lines := []string{
		"Karpenter is now fully up and running.",
		"",
		"Navigate to the Autoscaling settings page",
		"and select cluster to start generating recommendations:",
		autoscalingSettingsURL,
	}

	maxLength := slices.Max(lo.Map(lines, func(s string, _ int) int { return len(s) }))
	lines[4] = color.New(color.Bold, color.Underline, color.FgBlue).Sprint(autoscalingSettingsURL)

	cmd.Println("╭─" + strings.Repeat("─", maxLength) + "─╮")
	for _, line := range lines {
		cmd.Printf("│ %-*s │\n", maxLength, line)
	}
	cmd.Println("╰─" + strings.Repeat("─", maxLength) + "─╯")

	return nil
}
