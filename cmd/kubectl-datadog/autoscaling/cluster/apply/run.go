// Package apply contains the convergence logic shared between the `install`
// and `update` cobra commands. Both subcommands ultimately call Run with a
// RunOptions describing the desired Karpenter deployment; Run creates or
// updates the CloudFormation stacks, the Helm release, the aws-auth role, the
// optional EC2NodeClass/NodePool resources and the cluster-info ConfigMap.
package apply

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strconv"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/davecgh/go-spew/spew"
	"github.com/fatih/color"
	"github.com/pkg/browser"
	"helm.sh/helm/v3/pkg/registry"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/display"
	commoneks "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/eks"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/eksautomode"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/helm"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/karpenter"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/guess"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/k8s"
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

// RunOptions captures every parameter that callers (the install and update
// cobra commands) feed into Run.
type RunOptions struct {
	ClusterName              string
	KarpenterNamespace       string
	KarpenterVersion         string
	InstallMode              InstallMode
	FargateSubnets           []string
	CreateKarpenterResources CreateKarpenterResources
	InferenceMethod          InferenceMethod
	Debug                    bool
	// ActionLabel prefixes the opening "<label> Karpenter on cluster <c>."
	// box. install passes "Installing", update passes "Updating".
	ActionLabel string
}

// KarpenterStackName returns the name of the mode-independent CloudFormation
// stack carrying the KarpenterNodeRole, controller policy, SQS queue and
// EventBridge rules.
func KarpenterStackName(clusterName string) string {
	return "dd-karpenter-" + clusterName + "-karpenter"
}

// DDKarpenterStackName returns the name of the mode-dependent CloudFormation
// stack: aws-auth role, optional Fargate profile, IRSA role, etc.
func DDKarpenterStackName(clusterName string) string {
	return "dd-karpenter-" + clusterName + "-dd-karpenter"
}

// InstallModeTagKey is the CloudFormation stack tag tracking the deployment's
// install-mode. Stacks created before this tag was introduced have no tag and
// are treated as install-mode=existing-nodes.
const InstallModeTagKey = "install-mode"

// DetectedInstallMode reads the install-mode tag from a CFN stack. Stacks
// created before this tag was introduced have no tag and default to
// existing-nodes for backward compatibility.
func DetectedInstallMode(stack *aws.Stack) InstallMode {
	if stack == nil {
		return ""
	}
	if tag, ok := stack.TagMap()[InstallModeTagKey]; ok && tag != "" {
		return InstallMode(tag)
	}
	return InstallModeExistingNodes
}

// Run converges the cluster towards the Karpenter deployment described by
// opts. It is idempotent: calling it twice with the same opts is a no-op on
// the second run.
func Run(ctx context.Context, streams genericclioptions.IOStreams, configFlags *genericclioptions.ConfigFlags, clientset *kubernetes.Clientset, opts RunOptions) error {
	log.SetOutput(streams.ErrOut)
	ctrl.SetLogger(zap.New(zap.UseDevMode(false), zap.WriteTo(streams.ErrOut)))

	if autoModeEnabled, err := eksautomode.IsEnabled(clientset.Discovery()); err != nil {
		return fmt.Errorf("failed to check for EKS auto-mode: %w", err)
	} else if autoModeEnabled {
		return displayEKSAutoModeMessage(streams, opts.ClusterName)
	}

	k, err := karpenter.FindInstallation(ctx, clientset)
	if err != nil {
		return fmt.Errorf("failed to check for an existing Karpenter installation: %w", err)
	}
	if k != nil && (!k.IsOwn() || k.Namespace != opts.KarpenterNamespace) {
		return displayForeignKarpenterMessage(streams, opts.ClusterName, k)
	}

	display.PrintBox(streams.Out, opts.ActionLabel+" Karpenter on cluster "+opts.ClusterName+".")

	cli, err := clients.Build(ctx, configFlags, clientset)
	if err != nil {
		return fmt.Errorf("failed to build clients: %w", err)
	}

	if err = clients.ValidateAWSAccountConsistency(ctx, cli, opts.ClusterName, configFlags); err != nil {
		return err
	}

	irsaRoleArn, err := createCloudFormationStacks(ctx, cli, opts)
	if err != nil {
		return err
	}

	if err = updateAwsAuthConfigMap(ctx, cli, opts.ClusterName); err != nil {
		return err
	}

	if err = installHelmChart(ctx, configFlags, opts, irsaRoleArn); err != nil {
		return err
	}

	if err = createNodePoolResources(ctx, streams, cli, opts); err != nil {
		return err
	}

	if err = recordClusterInfo(ctx, cli, clientset.Discovery(), opts.ClusterName, opts.KarpenterNamespace); err != nil {
		log.Printf("Warning: %v", err)
	}

	return displaySuccessMessage(streams, opts.ClusterName, opts.CreateKarpenterResources)
}

// createCloudFormationStacks creates (or updates) the CFN stacks required for
// the selected install mode. Returns the IRSA role ARN when the mode is
// fargate; empty string otherwise.
func createCloudFormationStacks(ctx context.Context, cli *clients.Clients, opts RunOptions) (string, error) {
	// The first stack (karpenter.yaml) is mode-independent — its resources
	// (KarpenterNodeRole, KarpenterControllerPolicy, SQS, EventBridge) are
	// identical in both modes, so no guardrail or install-mode tag is needed.
	karpenterStackName := KarpenterStackName(opts.ClusterName)
	if err := aws.CreateOrUpdateStack(ctx, cli.CloudFormation, karpenterStackName, KarpenterCfn, map[string]string{
		"ClusterName": opts.ClusterName,
	}, nil); err != nil {
		return "", fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
	}

	describeOut, err := cli.EKS.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: awssdk.String(opts.ClusterName)})
	if err != nil {
		return "", fmt.Errorf("failed to describe cluster %s: %w", opts.ClusterName, err)
	}
	cluster := describeOut.Cluster
	supportsAPIAuth := commoneks.SupportsAPIAuthenticationMode(cluster)

	ddStackName := DDKarpenterStackName(opts.ClusterName)
	ddStack, err := aws.GetStack(ctx, cli.CloudFormation, ddStackName)
	if err != nil {
		return "", err
	}
	if err := checkInstallModeTag(ddStack, opts.InstallMode); err != nil {
		return "", err
	}
	modeTags := map[string]string{InstallModeTagKey: string(opts.InstallMode)}

	switch opts.InstallMode {
	case InstallModeExistingNodes:
		isUnmanagedEKSPIAInstalled, err := commoneks.IsThereUnmanagedEKSPodIdentityAgentInstalled(ctx, cli.EKS, opts.ClusterName)
		if err != nil {
			return "", fmt.Errorf("failed to check if EKS pod identity agent is installed: %w", err)
		}
		if err := aws.CreateOrUpdateStackWithExisting(ctx, cli.CloudFormation, ddStackName, DdKarpenterCfn, map[string]string{
			"ClusterName":            opts.ClusterName,
			"KarpenterNamespace":     opts.KarpenterNamespace,
			"DeployPodIdentityAddon": strconv.FormatBool(!isUnmanagedEKSPIAInstalled),
			"DeployNodeAccessEntry":  strconv.FormatBool(supportsAPIAuth),
		}, modeTags, ddStack); err != nil {
			return "", fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
		}
		return "", nil

	case InstallModeFargate:
		issuerURL, err := commoneks.GetClusterOIDCIssuerURL(cluster)
		if err != nil {
			return "", fmt.Errorf("failed to get cluster OIDC issuer URL: %w", err)
		}
		oidcArn, err := commoneks.EnsureOIDCProvider(ctx, cli.IAM, issuerURL)
		if err != nil {
			return "", fmt.Errorf("failed to ensure OIDC provider: %w", err)
		}

		subnets := opts.FargateSubnets
		if len(subnets) == 0 {
			subnets, err = commoneks.GetClusterPrivateSubnets(ctx, cli.EC2, cluster)
			if err != nil {
				return "", fmt.Errorf("failed to discover private subnets: %w", err)
			}
		}
		// Sort so that different orderings of the same subnet set produce the
		// same CloudFormation parameter value, making reruns idempotent and
		// the immutability check order-independent.
		slices.Sort(subnets)

		if err = checkFargateStackImmutability(ddStack, opts.KarpenterNamespace, subnets); err != nil {
			return "", err
		}

		if err = aws.CreateOrUpdateStackWithExisting(ctx, cli.CloudFormation, ddStackName, DdKarpenterFargateCfn, map[string]string{
			"ClusterName":           opts.ClusterName,
			"KarpenterNamespace":    opts.KarpenterNamespace,
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
		return "", fmt.Errorf("unsupported install mode %q", opts.InstallMode)
	}
}

// checkInstallModeTag verifies that an existing CFN stack's install-mode tag
// matches the requested mode. Stacks created before this tag was introduced
// have no tag and are treated as install-mode=existing-nodes.
func checkInstallModeTag(stack *aws.Stack, expected InstallMode) error {
	if stack == nil {
		return nil // fresh install
	}
	existing := DetectedInstallMode(stack)
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

func installHelmChart(ctx context.Context, configFlags *genericclioptions.ConfigFlags, opts RunOptions, irsaRoleArn string) error {
	actionConfig, err := helm.NewActionConfig(configFlags, opts.KarpenterNamespace)
	if err != nil {
		return err
	}

	if actionConfig.RegistryClient, err = registry.NewClient(
		registry.ClientOptDebug(opts.Debug),
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(log.Writer()),
	); err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	if err = helm.CreateOrUpgrade(ctx, actionConfig, "karpenter", opts.KarpenterNamespace, karpenterOCIRegistry, opts.KarpenterVersion, karpenterHelmValues(opts.ClusterName, opts.InstallMode, irsaRoleArn)); err != nil {
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
// The Karpenter OpenMetrics check is wired differently per install mode:
//   - existing-nodes: pod-level Autodiscovery annotation, picked up by the
//     node agent colocated with the controller pod.
//   - fargate: endpoint-check annotation on the Karpenter Service, paired
//     with `ad.datadoghq.com/endpoints.resolve: ip`. The default `auto`
//     resolution would attach the backing Pod's NodeName to each scheduled
//     check and the cluster agent would dispatch them to the (non-existent)
//     node agent on the Fargate node. The `ip` value tells the cluster agent
//     to ignore what's behind each endpoint IP, leaving the check as a plain
//     cluster check dispatched to the cluster check runner.
//
// In fargate mode, the service account is annotated with the IRSA role ARN so
// the controller can assume the role via sts:AssumeRoleWithWebIdentity.
func karpenterHelmValues(clusterName string, mode InstallMode, irsaRoleArn string) map[string]any {
	const karpenterCheckConfig = `{
  "karpenter": {
    "init_config": {},
    "instances": [
      {
        "openmetrics_endpoint": "http://%%host%%:8080/metrics"
      }
    ]
  }
}`

	values := map[string]any{
		// See karpenter.InstalledByLabel for why these keys are Datadog-namespaced.
		"additionalLabels": map[string]any{
			karpenter.InstalledByLabel:      karpenter.InstalledByValue,
			karpenter.InstallerVersionLabel: version.GetVersion(),
		},
		"settings": map[string]any{
			"clusterName":       clusterName,
			"interruptionQueue": clusterName,
		},
		"controller": map[string]any{
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "1", "memory": "2Gi"},
				"limits":   map[string]any{"cpu": "1", "memory": "2Gi"},
			},
		},
	}

	if mode == InstallModeFargate {
		values["service"] = map[string]any{
			"annotations": map[string]any{
				common.ADPrefix + "endpoints.checks":  karpenterCheckConfig,
				common.ADPrefix + "endpoints.resolve": "ip",
			},
		}
		values["serviceAccount"] = map[string]any{
			"annotations": map[string]any{
				"eks.amazonaws.com/role-arn": irsaRoleArn,
			},
		}
	} else {
		values["podAnnotations"] = map[string]any{
			common.ADPrefix + "controller.checks": karpenterCheckConfig,
		}
	}

	return values
}

func createNodePoolResources(ctx context.Context, streams genericclioptions.IOStreams, cli *clients.Clients, opts RunOptions) error {
	if opts.CreateKarpenterResources == CreateKarpenterResourcesNone {
		return nil
	}

	var nodePoolsSet *guess.NodePoolsSet
	var err error

	switch opts.InferenceMethod {
	case InferenceMethodNodes:
		nodePoolsSet, err = guess.GetNodesProperties(ctx, cli.K8sClientset, cli.EC2)
		if err != nil {
			return fmt.Errorf("failed to gather nodes properties: %w", err)
		}

	case InferenceMethodNodeGroups:
		nodePoolsSet, err = guess.GetNodeGroupsProperties(ctx, cli.EKS, cli.EC2, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to gather node groups properties: %w", err)
		}
	}

	if opts.Debug {
		fmt.Fprintf(streams.Out, "Creating the following resources:\n %s\n", spew.Sdump(nodePoolsSet))
	}

	if opts.CreateKarpenterResources == CreateKarpenterResourcesEC2NodeClass || opts.CreateKarpenterResources == CreateKarpenterResourcesAll {
		for _, nc := range nodePoolsSet.GetEC2NodeClasses() {
			if err = k8s.CreateOrUpdateEC2NodeClass(ctx, cli.K8sClient, opts.ClusterName, nc); err != nil {
				return fmt.Errorf("failed to create or update EC2NodeClass %s: %w", nc.GetName(), err)
			}
		}
	}

	if opts.CreateKarpenterResources == CreateKarpenterResourcesAll {
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
func recordClusterInfo(ctx context.Context, cli *clients.Clients, discoveryClient discovery.DiscoveryInterface, clusterName, namespace string) error {
	info, err := clusterinfo.Classify(ctx, clusterinfo.ClassifyInput{
		K8sClient:   cli.K8sClientset,
		CtrlClient:  cli.K8sClient,
		Autoscaling: cli.Autoscaling,
		EKS:         cli.EKS,
		Discovery:   discoveryClient,
		ClusterName: clusterName,
	})
	if err != nil {
		return fmt.Errorf("failed to classify cluster nodes: %w", err)
	}
	if err := clusterinfo.Persist(ctx, cli.K8sClient, namespace, info); err != nil {
		return fmt.Errorf("failed to write %s ConfigMap: %w", clusterinfo.ConfigMapName, err)
	}
	log.Printf("Wrote node-management snapshot to ConfigMap %s/%s.", namespace, clusterinfo.ConfigMapName)
	return nil
}

func openAutoscalingSettingsURL(streams genericclioptions.IOStreams, clusterName string) string {
	autoscalingSettingsURL := (&url.URL{
		Scheme:   "https",
		Host:     "app.datadoghq.com",
		Path:     "orchestration/scaling/settings",
		RawQuery: url.Values{"query": []string{"kube_cluster_name:" + clusterName}}.Encode(),
	}).String()

	browser.Stdout = streams.Out
	browser.Stderr = streams.ErrOut
	if err := browser.OpenURL(autoscalingSettingsURL); err != nil {
		log.Printf("Failed to open URL in browser: %v", err)
	}

	return color.New(color.Bold, color.Underline, color.FgBlue).Sprint(autoscalingSettingsURL)
}

func displayEKSAutoModeMessage(streams genericclioptions.IOStreams, clusterName string) error {
	coloredURL := openAutoscalingSettingsURL(streams, clusterName)

	display.PrintBox(streams.Out,
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

func displayForeignKarpenterMessage(streams genericclioptions.IOStreams, clusterName string, foreign *karpenter.Installation) error {
	coloredURL := openAutoscalingSettingsURL(streams, clusterName)

	display.PrintBox(streams.Out,
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

func displaySuccessMessage(streams genericclioptions.IOStreams, clusterName string, createResources CreateKarpenterResources) error {
	coloredURL := openAutoscalingSettingsURL(streams, clusterName)

	switch createResources {
	case CreateKarpenterResourcesNone:
		display.PrintBox(streams.Out,
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
		display.PrintBox(streams.Out,
			"✅ Datadog cluster autoscaling is now ready to be enabled.",
			"",
			"Navigate to the Autoscaling settings page",
			"and select cluster to start generating recommendations:",
			coloredURL,
		)
	}

	return nil
}
