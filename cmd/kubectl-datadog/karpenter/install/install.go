package install

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"

	karpawsv1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/karpenter/install/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/karpenter/install/guess"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/karpenter/install/helm"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/karpenter/install/k8s"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var (
	//go:embed assets/cfn/dd-karpenter.yaml
	DdKarpenterCfn string

	//go:embed assets/cfn/karpenter.yaml
	KarpenterCfn string

	//go:embed assets/charts/karpenter-1.8.2.tgz
	KarpenterHelmChart []byte
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
	inferenceMethod    = InferenceMethodNone
	installExample     = `
  # install Karpenter
  %[1]s karpenter install
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
		Short:        "Install Karpenter on an EKS cluster",
		Example:      fmt.Sprintf(installExample, "kubectl datadog karpenter"),
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
	cmd.Flags().Var(&inferenceMethod, "inference-method", "Method to infer EC2NodeClass and NodePool properties: none, nodes, nodegroups")

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

	if clusterName == "" {
		kubeRawConfig, err := o.ConfigFlags.ToRawKubeConfigLoader().RawConfig()
		if err != nil {
			return fmt.Errorf("failed to get raw kubeconfig: %w", err)
		}

		kubeContext := ""
		if o.ConfigFlags.Context != nil {
			kubeContext = *o.ConfigFlags.Context
		}

		clusterName = guess.GetClusterNameFromKubeconfig(ctx, kubeRawConfig, kubeContext)
	}

	if clusterName == "" {
		return errors.New("cluster name must be specified either via --cluster-name or in the current kubeconfig context")
	}

	cmd.Printf("Installing Karpenter on cluster %s.", clusterName)

	// Get AWS config
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Check if the EKS Pod Identity Agent is already installed and unmanaged
	eksClient := eks.NewFromConfig(awsConfig)
	isUnmanagedEKSPIAInstalled, err := guess.IsThereUnmanagedEKSPodIdentityAgentInstalled(ctx, eksClient, clusterName)
	if err != nil {
		return fmt.Errorf("failed to check if EKS pod identity agent is installed: %w", err)
	}

	// Create CloudFormation stacks
	cloudformationClient := cloudformation.NewFromConfig(awsConfig)

	if err = aws.CreateOrUpdateStack(ctx, cloudformationClient, "dd-karpenter-"+clusterName+"-karpenter", KarpenterCfn, map[string]string{
		"ClusterName": clusterName,
	}); err != nil {
		return fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
	}

	if err = aws.CreateOrUpdateStack(ctx, cloudformationClient, "dd-karpenter-"+clusterName+"-dd-karpenter", DdKarpenterCfn, map[string]string{
		"ClusterName":            clusterName,
		"KarpenterNamespace":     karpenterNamespace,
		"DeployPodIdentityAddon": strconv.FormatBool(!isUnmanagedEKSPIAInstalled),
	}); err != nil {
		return fmt.Errorf("failed to create or update Cloud Formation stack: %w", err)
	}

	awsAuthConfigMapPresent, err := guess.IsAwsAuthConfigMapPresent(ctx, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to check if aws-auth ConfigMap is present: %w", err)
	}

	if awsAuthConfigMapPresent {
		// Get AWS account ID
		stsClient := sts.NewFromConfig(awsConfig)
		var callerIdentity *sts.GetCallerIdentityOutput
		callerIdentity, err = stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return fmt.Errorf("failed to get identity caller: %w", err)
		}
		if callerIdentity.Account == nil {
			return errors.New("unable to determine AWS account ID from STS GetCallerIdentity")
		}
		accountID := *callerIdentity.Account

		// Add role mapping in the `aws-auth` ConfigMap
		if err = aws.EnsureAwsAuthRole(ctx, o.Clientset, aws.RoleMapping{
			RoleArn:  "arn:aws:iam::" + accountID + ":role/KarpenterNodeRole-" + clusterName,
			Username: "system:node:{{EC2PrivateDNSName}}",
			Groups:   []string{"system:bootstrappers", "system:nodes"},
		}); err != nil {
			return fmt.Errorf("failed to update aws-auth ConfigMap: %w", err)
		}
	}

	// Install Helm chart
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

	if err = actionConfig.Init(restClientGetter, karpenterNamespace, os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		return fmt.Errorf("failed to initialize Helm configuration: %w", err)
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

	if err = helm.CreateOrUpgrade(ctx, actionConfig, "karpenter", karpenterNamespace, KarpenterHelmChart, values); err != nil {
		return fmt.Errorf("failed to create or update Helm release: %w", err)
	}

	ec2Client := ec2.NewFromConfig(awsConfig)

	// Create EC2NodeClass and NodePool
	var nodePoolsSet *guess.NodePoolsSet
	switch inferenceMethod {
	case InferenceMethodNone:
		log.Printf("Karpenter has been successfully installed, but no EC2NodeClass nor NodePool have been created yet. " +
			"Those objects are mandatory for Karpenter to be able to auto-scale the cluster. " +
			"Use --inference-method=nodes or --inference-method=nodegroups to create some " +
			"with reasonable defaults based on the existing nodes of the cluster.")
		return nil

	case InferenceMethodNodes:
		nodePoolsSet, err = guess.GetNodesProperties(ctx, o.Clientset, ec2Client)
		if err != nil {
			return fmt.Errorf("failed to gather nodes properies: %w", err)
		}

	case InferenceMethodNodeGroups:
		nodePoolsSet, err = guess.GetNodeGroupsProperties(ctx, eksClient, ec2Client, clusterName)
		if err != nil {
			return fmt.Errorf("failed to gather node groups properties: %w", err)
		}
	}

	log.Printf("Creating the following node pools:\n %s", spew.Sdump(nodePoolsSet))

	sch := runtime.NewScheme()
	if err := scheme.AddToScheme(sch); err != nil {
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
		if err := k8s.CreateOrUpdateEC2NodeClass(ctx, o.Client, clusterName, nc); err != nil {
			return fmt.Errorf("failed to create or update EC2NodeClass %s: %w", nc.Name, err)
		}
	}

	for _, np := range nodePoolsSet.GetNodePools() {
		if err := k8s.CreateOrUpdateNodePool(ctx, o.Client, np); err != nil {
			return fmt.Errorf("failed to create or update NodePool %s: %w", np.Name, err)
		}
	}

	cmd.Println("Karpenter is now fully up and running.")
	cmd.Println("You can now go to https://app.datadoghq.com/orchestration/scaling/cluster to enable Datadog managed cluster autoscaling.")

	return nil
}
