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

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/davecgh/go-spew/spew"
	"github.com/fatih/color"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/registry"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/display"
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
	//go:embed assets/karpenter.yaml
	KarpenterCfn string

	//go:embed assets/dd-karpenter.yaml
	DdKarpenterCfn string

	//go:embed assets/dd-karpenter-fargate.yaml
	DdKarpenterFargateCfn string
)

// installModeTagKey is the CloudFormation stack tag tracking the deployment's
// install-mode. Stacks created before this tag was introduced are treated as
// install-mode=existing-nodes.
const installModeTagKey = "install-mode"

// InstallMode defines how to run the Karpenter controller.
type InstallMode string

const (
	// InstallModeFargate runs the Karpenter controller on dedicated Fargate nodes.
	InstallModeFargate InstallMode = "fargate"
	// InstallModeExistingNodes runs the Karpenter controller on existing cluster nodes.
	InstallModeExistingNodes InstallMode = "existing-nodes"
)

// String returns the string representation of the InstallMode.
func (i *InstallMode) String() string {
	return string(*i)
}

// Set sets the InstallMode value from a string.
func (i *InstallMode) Set(s string) error {
	switch s {
	case "fargate":
		*i = InstallModeFargate
	case "existing-nodes":
		*i = InstallModeExistingNodes
	default:
		return fmt.Errorf("install-mode must be one of fargate or existing-nodes")
	}

	return nil
}

// Type returns the type name for pflag.
func (_ *InstallMode) Type() string {
	return "InstallMode"
}

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
	installMode              = InstallModeFargate
	fargateSubnets           []string
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
	cmd.Flags().Var(&installMode, "install-mode", "How to run the Karpenter controller: fargate (on dedicated Fargate nodes, default) or existing-nodes (on existing cluster nodes)")
	cmd.Flags().StringSliceVar(&fargateSubnets, "fargate-subnets", nil, "Override auto-discovery of private subnets for the Fargate profile (comma-separated subnet IDs). Only used when --install-mode=fargate.")
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

	if !slices.Contains([]InstallMode{InstallModeFargate, InstallModeExistingNodes}, installMode) {
		return errors.New("install-mode must be one of fargate or existing-nodes")
	}

	if len(fargateSubnets) > 0 && installMode != InstallModeFargate {
		return errors.New("--fargate-subnets can only be used with --install-mode=fargate")
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
		if name, err := clients.GetClusterNameFromKubeconfig(o.ConfigFlags); err != nil {
			return err
		} else if name != "" {
			clusterName = name
		} else {
			return errors.New("cluster name must be specified either via --cluster-name or in the current kubeconfig context")
		}
	}

	if autoModeEnabled, err := guess.IsEKSAutoModeEnabled(o.DiscoveryClient); err != nil {
		return fmt.Errorf("failed to check for EKS auto-mode: %w", err)
	} else if autoModeEnabled {
		return displayEKSAutoModeMessage(cmd, clusterName)
	}

	k, err := guess.FindKarpenterInstallation(ctx, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to check for an existing Karpenter installation: %w", err)
	}
	if k != nil && (!k.IsOwn() || k.Namespace != karpenterNamespace) {
		return displayForeignKarpenterMessage(cmd, clusterName, k)
	}

	display.PrintBox(cmd.OutOrStdout(), "Installing Karpenter on cluster "+clusterName+".")

	cli, err := clients.Build(ctx, o.ConfigFlags, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to build clients: %w", err)
	}

	if err = clients.ValidateAWSAccountConsistency(ctx, cli, clusterName, o.ConfigFlags); err != nil {
		return err
	}

	irsaRoleArn, err := createCloudFormationStacks(ctx, cli, clusterName, karpenterNamespace, installMode, fargateSubnets)
	if err != nil {
		return err
	}

	if err = updateAwsAuthConfigMap(ctx, cli, clusterName); err != nil {
		return err
	}

	if err = o.installHelmChart(ctx, clusterName, karpenterNamespace, karpenterVersion, debug, installMode, irsaRoleArn); err != nil {
		return err
	}

	if err = createNodePoolResources(ctx, cmd, cli, clusterName, createKarpenterResources, inferenceMethod, debug); err != nil {
		return err
	}

	if err = recordClusterInfo(ctx, cli, clusterName, karpenterNamespace); err != nil {
		log.Printf("Warning: %v", err)
	}

	return displaySuccessMessage(cmd, clusterName, createKarpenterResources)
}

// createCloudFormationStacks creates (or updates) the CFN stacks required for
// the selected install mode. Returns the IRSA role ARN when the mode is
// fargate; empty string otherwise.
func createCloudFormationStacks(ctx context.Context, cli *clients.Clients, clusterName, karpenterNamespace string, mode InstallMode, fargateSubnetsOverride []string) (string, error) {
	// The first stack (karpenter.yaml) is mode-independent — its resources
	// (KarpenterNodeRole, KarpenterControllerPolicy, SQS, EventBridge) are
	// identical in both modes, so no guardrail or install-mode tag is needed.
	karpenterStackName := "dd-karpenter-" + clusterName + "-karpenter"
	if err := aws.CreateOrUpdateStack(ctx, cli.CloudFormation, karpenterStackName, KarpenterCfn, map[string]string{
		"ClusterName": clusterName,
	}, nil); err != nil {
		return "", fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
	}

	describeOut, err := cli.EKS.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: awssdk.String(clusterName)})
	if err != nil {
		return "", fmt.Errorf("failed to describe cluster %s: %w", clusterName, err)
	}
	cluster := describeOut.Cluster
	supportsAPIAuth := guess.SupportsAPIAuthenticationMode(cluster)

	ddStackName := "dd-karpenter-" + clusterName + "-dd-karpenter"
	ddStack, err := aws.GetStack(ctx, cli.CloudFormation, ddStackName)
	if err != nil {
		return "", err
	}
	if err := checkInstallModeTag(ddStack, mode); err != nil {
		return "", err
	}
	modeTags := map[string]string{installModeTagKey: string(mode)}

	switch mode {
	case InstallModeExistingNodes:
		isUnmanagedEKSPIAInstalled, err := guess.IsThereUnmanagedEKSPodIdentityAgentInstalled(ctx, cli.EKS, clusterName)
		if err != nil {
			return "", fmt.Errorf("failed to check if EKS pod identity agent is installed: %w", err)
		}
		if err := aws.CreateOrUpdateStackWithExisting(ctx, cli.CloudFormation, ddStackName, DdKarpenterCfn, map[string]string{
			"ClusterName":            clusterName,
			"KarpenterNamespace":     karpenterNamespace,
			"DeployPodIdentityAddon": strconv.FormatBool(!isUnmanagedEKSPIAInstalled),
			"DeployNodeAccessEntry":  strconv.FormatBool(supportsAPIAuth),
		}, modeTags, ddStack); err != nil {
			return "", fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
		}
		return "", nil

	case InstallModeFargate:
		issuerURL, err := guess.GetClusterOIDCIssuerURL(cluster)
		if err != nil {
			return "", fmt.Errorf("failed to get cluster OIDC issuer URL: %w", err)
		}
		oidcArn, err := guess.EnsureOIDCProvider(ctx, cli.IAM, issuerURL)
		if err != nil {
			return "", fmt.Errorf("failed to ensure OIDC provider: %w", err)
		}

		subnets := fargateSubnetsOverride
		if len(subnets) == 0 {
			subnets, err = guess.GetClusterPrivateSubnets(ctx, cli.EC2, cluster)
			if err != nil {
				return "", fmt.Errorf("failed to discover private subnets: %w", err)
			}
		}
		// Sort so that different orderings of the same subnet set produce the
		// same CloudFormation parameter value, making reruns idempotent and
		// the immutability check order-independent.
		slices.Sort(subnets)

		if err = checkFargateStackImmutability(ddStack, karpenterNamespace, subnets); err != nil {
			return "", err
		}

		if err = aws.CreateOrUpdateStackWithExisting(ctx, cli.CloudFormation, ddStackName, DdKarpenterFargateCfn, map[string]string{
			"ClusterName":           clusterName,
			"KarpenterNamespace":    karpenterNamespace,
			"OIDCProviderArn":       oidcArn,
			"OIDCProviderURL":       strings.TrimPrefix(issuerURL, "https://"),
			"FargateSubnets":        strings.Join(subnets, ","),
			"DeployNodeAccessEntry": strconv.FormatBool(supportsAPIAuth),
		}, modeTags, ddStack); err != nil {
			return "", fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
		}

		// Re-read the stack to pick up the outputs (not present pre-create).
		updated, err := aws.GetStack(ctx, cli.CloudFormation, ddStackName)
		if err != nil {
			return "", fmt.Errorf("failed to read stack outputs: %w", err)
		}
		if updated == nil {
			return "", fmt.Errorf("stack %s disappeared right after successful create/update", ddStackName)
		}
		irsaRoleArn := updated.OutputMap()["KarpenterRoleArn"]
		if irsaRoleArn == "" {
			return "", fmt.Errorf("stack %s did not produce a KarpenterRoleArn output", ddStackName)
		}
		return irsaRoleArn, nil

	default:
		return "", fmt.Errorf("unsupported install mode %q", mode)
	}
}

// checkInstallModeTag verifies that an existing CFN stack's install-mode tag
// matches the requested mode. Stacks created before this tag was introduced
// have no tag and are treated as install-mode=existing-nodes.
func checkInstallModeTag(stack *aws.Stack, expected InstallMode) error {
	if stack == nil {
		return nil // fresh install
	}
	existing := InstallModeExistingNodes
	if tag, ok := stack.TagMap()[installModeTagKey]; ok && tag != "" {
		existing = InstallMode(tag)
	}
	if existing != expected {
		return fmt.Errorf("stack %s was created with --install-mode=%s; run 'kubectl datadog autoscaling cluster uninstall' first to switch to --install-mode=%s", awssdk.ToString(stack.StackName), existing, expected)
	}
	return nil
}

// checkFargateStackImmutability refuses to update an existing Fargate stack
// when its KarpenterNamespace or FargateSubnets parameters differ from the
// current ones. The underlying AWS::EKS::FargateProfile resource has a pinned
// FargateProfileName and its Selectors/Subnets are CreateOnly properties; a
// CloudFormation update with changed parameters would fail during replacement.
func checkFargateStackImmutability(stack *aws.Stack, namespace string, subnets []string) error {
	if stack == nil {
		return nil // fresh install
	}
	stackName := awssdk.ToString(stack.StackName)
	params := stack.ParameterMap()
	if existing, ok := params["KarpenterNamespace"]; ok && existing != namespace {
		return fmt.Errorf("stack %s was created with KarpenterNamespace=%s; run 'kubectl datadog autoscaling cluster uninstall' first to change it to %s", stackName, existing, namespace)
	}
	newSubnets := strings.Join(subnets, ",")
	if existing, ok := params["FargateSubnets"]; ok && existing != newSubnets {
		return fmt.Errorf("stack %s was created with FargateSubnets=%s; run 'kubectl datadog autoscaling cluster uninstall' first to change them to %s", stackName, existing, newSubnets)
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

	accountID, err := clients.GetAWSAccountID(ctx, cli)
	if err != nil {
		return err
	}

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

func (o *options) installHelmChart(ctx context.Context, clusterName, karpenterNamespace, karpenterVersion string, debug bool, mode InstallMode, irsaRoleArn string) error {
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

	if err = helm.CreateOrUpgrade(ctx, actionConfig, "karpenter", karpenterNamespace, karpenterOCIRegistry, karpenterVersion, karpenterHelmValues(clusterName, mode, irsaRoleArn)); err != nil {
		return fmt.Errorf("failed to create or update Helm release: %w", err)
	}

	return nil
}

// karpenterHelmValues returns the Helm values for the Karpenter chart.
//
// resources are always set: the chart's default is {}, which in Fargate mode
// falls back to Fargate's default (0.25 vCPU / 0.5 GiB) and OOMs the
// controller, and in existing-nodes mode leaves the pod without any request.
// 1 vCPU / 2 GiB is the smallest valid Fargate combination for 1 vCPU and
// matches the chart's documented example for existing-nodes.
//
// In fargate mode, the service account is annotated with the IRSA role ARN so
// the controller can assume the role via sts:AssumeRoleWithWebIdentity.
func karpenterHelmValues(clusterName string, mode InstallMode, irsaRoleArn string) map[string]any {
	values := map[string]any{
		// See guess.InstalledByLabel for why these keys are Datadog-namespaced.
		"additionalLabels": map[string]any{
			guess.InstalledByLabel:      guess.InstalledByValue,
			guess.InstallerVersionLabel: version.GetVersion(),
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
		"controller": map[string]any{
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "1", "memory": "2Gi"},
				"limits":   map[string]any{"cpu": "1", "memory": "2Gi"},
			},
		},
	}

	if mode == InstallModeFargate {
		values["serviceAccount"] = map[string]any{
			"annotations": map[string]any{
				"eks.amazonaws.com/role-arn": irsaRoleArn,
			},
		}
	}

	return values
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

// recordClusterInfo classifies every node by its current management method
// and writes the snapshot to a ConfigMap. The information is consumed by the
// follow-up migration step.
func recordClusterInfo(ctx context.Context, cli *clients.Clients, clusterName, namespace string) error {
	info, err := clusterinfo.Classify(ctx, cli.K8sClientset, cli.Autoscaling, clusterName)
	if err != nil {
		return fmt.Errorf("failed to classify cluster nodes: %w", err)
	}
	if err := clusterinfo.Persist(ctx, cli.K8sClient, namespace, info); err != nil {
		return fmt.Errorf("failed to write %s ConfigMap: %w", clusterinfo.ConfigMapName, err)
	}
	log.Printf("Wrote node-management snapshot to ConfigMap %s/%s.", namespace, clusterinfo.ConfigMapName)
	return nil
}

func openAutoscalingSettingsURL(cmd *cobra.Command, clusterName string) string {
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

	return color.New(color.Bold, color.Underline, color.FgBlue).Sprint(autoscalingSettingsURL)
}

func displayEKSAutoModeMessage(cmd *cobra.Command, clusterName string) error {
	coloredURL := openAutoscalingSettingsURL(cmd, clusterName)

	display.PrintBox(cmd.OutOrStdout(),
		"EKS auto-mode is already active on cluster "+clusterName+".",
		"",
		"Karpenter is built into EKS auto-mode",
		"and does not need to be installed separately.",
		"",
		"Navigate to the Autoscaling settings page",
		"and select cluster to start generating recommendations:",
		coloredURL,
	)

	return nil
}

func displayForeignKarpenterMessage(cmd *cobra.Command, clusterName string, foreign *guess.KarpenterInstallation) error {
	coloredURL := openAutoscalingSettingsURL(cmd, clusterName)

	display.PrintBox(cmd.OutOrStdout(),
		"Karpenter is already installed on cluster "+clusterName+":",
		"Deployment "+foreign.Namespace+"/"+foreign.Name+".",
		"",
		"kubectl-datadog has nothing to install.",
		"",
		"Navigate to the Autoscaling settings page",
		"and select cluster to start generating recommendations:",
		coloredURL,
	)

	return nil
}

func displaySuccessMessage(cmd *cobra.Command, clusterName string, createResources CreateKarpenterResources) error {
	coloredURL := openAutoscalingSettingsURL(cmd, clusterName)

	switch createResources {
	case CreateKarpenterResourcesNone:
		display.PrintBox(cmd.OutOrStdout(),
			"✅ Datadog cluster autoscaling is partially configured.",
			"",
			"No Karpenter resources were created.",
			"Use --create-karpenter-resources=ec2nodeclass or =all",
			"to create EC2NodeClass and/or NodePool resources.",
			"",
			"Navigate to the Autoscaling settings page:",
			coloredURL,
		)
	case CreateKarpenterResourcesEC2NodeClass, CreateKarpenterResourcesAll:
		display.PrintBox(cmd.OutOrStdout(),
			"✅ Datadog cluster autoscaling is now ready to be enabled.",
			"",
			"Navigate to the Autoscaling settings page",
			"and select cluster to start generating recommendations:",
			coloredURL,
		)
	}

	return nil
}
