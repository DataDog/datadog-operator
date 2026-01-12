// Package install provides functionality to install and configure Karpenter
// autoscaling on EKS clusters, including CloudFormation stack creation,
// Helm chart deployment, and resource configuration.
package install

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/davecgh/go-spew/spew"
	"github.com/fatih/color"
	"github.com/pkg/browser"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/registry"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/helm"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/k8s"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	"github.com/DataDog/datadog-operator/pkg/version"

	_ "embed"
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

// CreateKarpenterResources defines which Karpenter resources to create
type CreateKarpenterResources string

const (
	// CreateKarpenterResourcesNone does not create any Karpenter resources
	CreateKarpenterResourcesNone CreateKarpenterResources = "none"
	// CreateKarpenterResourcesEC2NodeClass creates only EC2NodeClass resources
	CreateKarpenterResourcesEC2NodeClass CreateKarpenterResources = "ec2nodeclass"
	// CreateKarpenterResourcesAll creates both EC2NodeClass and NodePool resources
	CreateKarpenterResourcesAll CreateKarpenterResources = "all"
)

// String returns the string representation of CreateKarpenterResources
func (c *CreateKarpenterResources) String() string {
	return string(*c)
}

// Set sets the CreateKarpenterResources value from a string
func (c *CreateKarpenterResources) Set(s string) error {
	switch s {
	case "none":
		*c = CreateKarpenterResourcesNone
	case "ec2nodeclass":
		*c = CreateKarpenterResourcesEC2NodeClass
	case "all":
		*c = CreateKarpenterResourcesAll
	default:
		return fmt.Errorf("create-karpenter-resources must be one of none, ec2nodeclass or all")
	}

	return nil
}

// Type returns the type name for pflag
func (_ *CreateKarpenterResources) Type() string {
	return "CreateKarpenterResources"
}

// InferenceMethod defines how to infer EC2NodeClass and NodePool properties
type InferenceMethod string

const (
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
	case "nodes":
		*i = InferenceMethodNodes
	case "nodegroups":
		*i = InferenceMethodNodeGroups
	default:
		return fmt.Errorf("inference-method must be one of nodes or nodegroups")
	}

	return nil
}

// Type returns the type name for pflag
func (_ *InferenceMethod) Type() string {
	return "InferenceMethod"
}

var (
	clusterName              string
	karpenterNamespace       string
	karpenterVersion         string
	createKarpenterResources = CreateKarpenterResourcesAll
	inferenceMethod          = InferenceMethodNodeGroups
	debug                    bool
	installExample           = `
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
	cmd.Flags().Var(&createKarpenterResources, "create-karpenter-resources", "Which Karpenter resources to create: none, ec2nodeclass, all (default: all)")
	cmd.Flags().Var(&inferenceMethod, "inference-method", "Method to infer EC2NodeClass and NodePool properties: nodes, nodegroups")
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

	if !slices.Contains([]CreateKarpenterResources{CreateKarpenterResourcesNone, CreateKarpenterResourcesEC2NodeClass, CreateKarpenterResourcesAll}, createKarpenterResources) {
		return errors.New("create-karpenter-resources must be one of none, ec2nodeclass or all")
	}

	if !slices.Contains([]InferenceMethod{InferenceMethodNodes, InferenceMethodNodeGroups}, inferenceMethod) {
		return errors.New("inference-method must be one of nodes or nodegroups")
	}

	return nil
}

func (o *options) run(cmd *cobra.Command) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.SetOutput(cmd.OutOrStderr())
	ctrl.SetLogger(zap.New(zap.UseDevMode(false), zap.WriteTo(cmd.ErrOrStderr())))

	if clusterName == "" {
		if name, err := clients.GetClusterNameFromKubeconfig(ctx, o.ConfigFlags); err != nil {
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

	cli, err := clients.Build(ctx, o.ConfigFlags, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to build clients: %w", err)
	}

	if err = createCloudFormationStacks(ctx, cli, clusterName, karpenterNamespace); err != nil {
		return err
	}

	if err = updateAwsAuthConfigMap(ctx, cli, clusterName); err != nil {
		return err
	}

	if err = o.installHelmChart(ctx, clusterName, karpenterNamespace, karpenterVersion, debug); err != nil {
		return err
	}

	if err = createNodePoolResources(ctx, cmd, cli, clusterName, createKarpenterResources, inferenceMethod, debug); err != nil {
		return err
	}

	return displaySuccessMessage(cmd, clusterName, createKarpenterResources)
}

func createCloudFormationStacks(ctx context.Context, cli *clients.Clients, clusterName string, karpenterNamespace string) error {
	if err := aws.CreateOrUpdateStack(ctx, cli.CloudFormation, "dd-karpenter-"+clusterName+"-karpenter", KarpenterCfn, map[string]string{
		"ClusterName": clusterName,
	}); err != nil {
		return fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
	}

	isUnmanagedEKSPIAInstalled, err := guess.IsThereUnmanagedEKSPodIdentityAgentInstalled(ctx, cli.EKS, clusterName)
	if err != nil {
		return fmt.Errorf("failed to check if EKS pod identity agent is installed: %w", err)
	}

	if err := aws.CreateOrUpdateStack(ctx, cli.CloudFormation, "dd-karpenter-"+clusterName+"-dd-karpenter", DdKarpenterCfn, map[string]string{
		"ClusterName":            clusterName,
		"KarpenterNamespace":     karpenterNamespace,
		"DeployPodIdentityAddon": strconv.FormatBool(!isUnmanagedEKSPIAInstalled),
	}); err != nil {
		return fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
	}

	return nil
}

func updateAwsAuthConfigMap(ctx context.Context, cli *clients.Clients, clusterName string) error {
	awsAuthConfigMapPresent, err := guess.IsAwsAuthConfigMapPresent(ctx, cli.K8sClientset)
	if err != nil {
		return fmt.Errorf("failed to check if aws-auth ConfigMap is present: %w", err)
	}

	if !awsAuthConfigMapPresent {
		log.Println("aws-auth ConfigMap not present, skipping role addition.")
		return nil
	}

	// Get AWS account ID
	callerIdentity, err := cli.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get identity caller: %w", err)
	}
	if callerIdentity.Account == nil {
		return errors.New("unable to determine AWS account ID from STS GetCallerIdentity")
	}
	accountID := *callerIdentity.Account

	// Add role mapping in the `aws-auth` ConfigMap
	if err = aws.EnsureAwsAuthRole(ctx, cli.K8sClientset, aws.RoleMapping{
		RoleArn:  "arn:aws:iam::" + accountID + ":role/KarpenterNodeRole-" + clusterName,
		Username: "system:node:{{EC2PrivateDNSName}}",
		Groups:   []string{"system:bootstrappers", "system:nodes"},
	}); err != nil {
		return fmt.Errorf("failed to update aws-auth ConfigMap: %w", err)
	}

	return nil
}

func (o *options) installHelmChart(ctx context.Context, clusterName string, karpenterNamespace string, karpenterVersion string, debug bool) error {
	actionConfig, err := helm.NewActionConfig(o.ConfigFlags, karpenterNamespace)
	if err != nil {
		return err
	}

	if actionConfig.RegistryClient, err = registry.NewClient(
		registry.ClientOptDebug(debug),
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(log.Writer()),
	); err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	values := map[string]any{
		"additionalLabels": map[string]string{
			"app.kubernetes.io/managed-by": "kubectl-datadog",
			"app.kubernetes.io/version":    version.GetVersion(),
		},
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

func createNodePoolResources(ctx context.Context, cmd *cobra.Command, cli *clients.Clients, clusterName string, createResources CreateKarpenterResources, inferenceMethod InferenceMethod, debug bool) error {
	if createResources == CreateKarpenterResourcesNone {
		return nil
	}

	var nodePoolsSet *guess.NodePoolsSet
	var err error

	switch inferenceMethod {
	case InferenceMethodNodes:
		nodePoolsSet, err = guess.GetNodesProperties(ctx, cli.K8sClientset, cli.EC2)
		if err != nil {
			return fmt.Errorf("failed to gather nodes properties: %w", err)
		}

	case InferenceMethodNodeGroups:
		nodePoolsSet, err = guess.GetNodeGroupsProperties(ctx, cli.EKS, cli.EC2, clusterName)
		if err != nil {
			return fmt.Errorf("failed to gather node groups properties: %w", err)
		}
	}

	if debug {
		cmd.Printf("Creating the following resources:\n %s\n", spew.Sdump(nodePoolsSet))
	}

	if createResources == CreateKarpenterResourcesEC2NodeClass || createResources == CreateKarpenterResourcesAll {
		for _, nc := range nodePoolsSet.GetEC2NodeClasses() {
			if err = k8s.CreateOrUpdateEC2NodeClass(ctx, cli.K8sClient, clusterName, nc); err != nil {
				return fmt.Errorf("failed to create or update EC2NodeClass %s: %w", nc.GetName(), err)
			}
		}
	}

	if createResources == CreateKarpenterResourcesAll {
		for _, np := range nodePoolsSet.GetNodePools() {
			if err = k8s.CreateOrUpdateNodePool(ctx, cli.K8sClient, np); err != nil {
				return fmt.Errorf("failed to create or update NodePool %s: %w", np.GetName(), err)
			}
		}
	}

	return nil
}

func displaySuccessMessage(cmd *cobra.Command, clusterName string, createResources CreateKarpenterResources) error {
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

	var lines []string

	switch createResources {
	case CreateKarpenterResourcesNone:
		lines = []string{
			"Datadog cluster autoscaling is partially configured.",
			"",
			"No Karpenter resources were created.",
			"Use --create-karpenter-resources=ec2nodeclass or =all",
			"to create EC2NodeClass and/or NodePool resources.",
			"",
			"Navigate to the Autoscaling settings page:",
			autoscalingSettingsURL,
		}
	case CreateKarpenterResourcesEC2NodeClass:
		fallthrough
	case CreateKarpenterResourcesAll:
		lines = []string{
			"Datadog cluster autoscaling is now ready to be enabled.",
			"",
			"Navigate to the Autoscaling settings page",
			"and select cluster to start generating recommendations:",
			autoscalingSettingsURL,
		}
	}

	maxLength := slices.Max(lo.Map(lines, func(s string, _ int) int { return len(s) }))
	lines[len(lines)-1] = color.New(color.Bold, color.Underline, color.FgBlue).Sprint(autoscalingSettingsURL)

	cmd.Println("╭─" + strings.Repeat("─", maxLength) + "─╮")
	for _, line := range lines {
		cmd.Printf("│ %-*s │\n", maxLength, line)
	}
	cmd.Println("╰─" + strings.Repeat("─", maxLength) + "─╯")

	return nil
}
