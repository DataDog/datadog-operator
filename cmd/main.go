package main

import (
	"context"
	_ "embed"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/davecgh/go-spew/spew"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpawsv1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"

	"github.com/L3n41c/karpenter_installer_wizard/internal/aws"
	"github.com/L3n41c/karpenter_installer_wizard/internal/guess"
	"github.com/L3n41c/karpenter_installer_wizard/internal/helm"
	"github.com/L3n41c/karpenter_installer_wizard/internal/k8s"
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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Get default kube config
	k8sClientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)

	// Get the default cluster name of the default context of the default kube config.
	kubeRawConfig, err := k8sClientConfig.RawConfig()
	if err != nil {
		log.Fatal(err)
	}
	guessedClusterName := guess.GetClusterNameFromKubeconfig(ctx, kubeRawConfig, "")

	// parse CLI flags
	var (
		clusterName        = flag.String("cluster-name", guessedClusterName, "Name of the EKS cluster")
		karpenterNamespace = flag.String("karpenter-namespace", "dd-karpenter", "Name of the Kubernetes namespace in which deploying Karpenter")
		kubeConfig         = flag.String("kube-config", clientcmd.RecommendedHomeFile, "Path to the kubeconfig file")
		kubeContext        = flag.String("kube-context", "", "Name of the kubeconfig context to use")
		inferenceMethod    = flag.String("inference-method", string(InferenceMethodNone), "Method to infer EC2NodeClass and NodePool properties: none, nodes, nodegroups")
	)
	flag.Parse()

	// Update kube config with the command line parameters
	k8sClientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: *kubeConfig},
		&clientcmd.ConfigOverrides{CurrentContext: *kubeContext},
	)

	// Update default cluster name
	if *clusterName == guessedClusterName &&
		(*kubeConfig != clientcmd.RecommendedHomeFile || *kubeContext != "") {
		kubeRawConfig, err = k8sClientConfig.RawConfig()
		if err != nil {
			log.Fatal(err)
		}
		*clusterName = guess.GetClusterNameFromKubeconfig(ctx, kubeRawConfig, *kubeContext)
		log.Printf("Using cluster name: %s", *clusterName)
	}

	if *clusterName == "" {
		log.Fatal("cluster name must be specified either via --cluster-name or in the current kubeconfig context")
	}

	// Get AWS config
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Check if the EKS Pod Identity Agent is already installed and unmanaged
	eksClient := eks.NewFromConfig(awsConfig)
	isUnmanagedEKSPIAInstalled, err := guess.IsThereUnmanagedEKSPodIdentityAgentInstalled(ctx, eksClient, *clusterName)
	if err != nil {
		log.Fatal(err)
	}

	// Create CloudFormation stacks
	cloudformationClient := cloudformation.NewFromConfig(awsConfig)

	if err := aws.CreateOrUpdateStack(ctx, cloudformationClient, "dd-karpenter-"+*clusterName+"-karpenter", KarpenterCfn, map[string]string{
		"ClusterName": *clusterName,
	}); err != nil {
		log.Fatal(err)
	}

	if err := aws.CreateOrUpdateStack(ctx, cloudformationClient, "dd-karpenter-"+*clusterName+"-dd-karpenter", DdKarpenterCfn, map[string]string{
		"ClusterName":            *clusterName,
		"KarpenterNamespace":     *karpenterNamespace,
		"DeployPodIdentityAddon": strconv.FormatBool(!isUnmanagedEKSPIAInstalled),
	}); err != nil {
		log.Fatal(err)
	}

	// Get kube client
	kubeClientConfig, err := k8sClientConfig.ClientConfig()
	if err != nil {
		log.Fatal(err)
	}
	kubeClientSet, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		log.Fatal(err)
	}

	awsAuthConfigMapPresent, err := guess.IsAwsAuthConfigMapPresent(ctx, kubeClientSet)
	if err != nil {
		log.Fatal(err)
	}

	if awsAuthConfigMapPresent {
		// Get AWS account ID
		stsClient := sts.NewFromConfig(awsConfig)
		callerIdentity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			log.Fatal(err)
		}
		if callerIdentity.Account == nil {
			log.Fatal("unable to determine AWS account ID from STS GetCallerIdentity")
		}
		accountID := *callerIdentity.Account

		// Add role mapping in the `aws-auth` ConfigMap
		if err := aws.EnsureAwsAuthRole(ctx, kubeClientSet, aws.RoleMapping{
			RoleArn:  "arn:aws:iam::" + accountID + ":role/KarpenterNodeRole-" + *clusterName,
			Username: "system:node:{{EC2PrivateDNSName}}",
			Groups:   []string{"system:bootstrappers", "system:nodes"},
		}); err != nil {
			log.Fatal(err)
		}
	}

	// Install Helm chart
	restClientGetter := kube.GetConfig(*kubeConfig, *kubeContext, *karpenterNamespace)
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(restClientGetter, *karpenterNamespace, os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		log.Fatal(err)
	}

	values := map[string]any{
		"settings": map[string]any{
			"clusterName":       *clusterName,
			"interruptionQueue": *clusterName,
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

	if err := helm.CreateOrUpgrade(ctx, actionConfig, "karpenter", *karpenterNamespace, KarpenterHelmChart, values); err != nil {
		log.Fatal(err)
	}

	ec2Client := ec2.NewFromConfig(awsConfig)

	// Create EC2NodeClass and NodePool
	var nodePoolsSet *guess.NodePoolsSet
	switch InferenceMethod(*inferenceMethod) {
	case InferenceMethodNone:
		log.Printf("Karpenter has been successfully installed, but no EC2NodeClass nor NodePool have been created yet. " +
			"Those objects are mandatory for Karpenter to be able to auto-scale the cluster. " +
			"Use --inference-method=nodes or --inference-method=nodegroups to create some " +
			"with reasonable defaults based on the existing nodes of the cluster.")
		return

	case InferenceMethodNodes:
		nodePoolsSet, err = guess.GetNodesProperties(ctx, kubeClientSet, ec2Client)
		if err != nil {
			log.Fatal(err)
		}

	case InferenceMethodNodeGroups:
		nodePoolsSet, err = guess.GetNodeGroupsProperties(ctx, eksClient, ec2Client, *clusterName)
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("Creating the following node pools:\n %s", spew.Sdump(nodePoolsSet))

	sch := runtime.NewScheme()
	if err := scheme.AddToScheme(sch); err != nil {
		log.Fatal(err)
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

	k8sClient, err := client.New(kubeClientConfig, client.Options{Scheme: sch})
	if err != nil {
		log.Fatal(err)
	}

	for _, nc := range nodePoolsSet.GetEC2NodeClasses() {
		if err := k8s.CreateOrUpdateEC2NodeClass(ctx, k8sClient, *clusterName, nc); err != nil {
			log.Fatal(err)
		}
	}

	for _, np := range nodePoolsSet.GetNodePools() {
		if err := k8s.CreateOrUpdateNodePool(ctx, k8sClient, np); err != nil {
			log.Fatal(err)
		}
	}

	log.Println("Karpenter is now fully up and running.")
	log.Println("You can now go to https://app.datadoghq.com/orchestration/scaling/cluster to enable Datadog managed cluster autoscaling.")
}
