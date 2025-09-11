package main

import (
	"context"
	_ "embed"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"

	"github.com/L3n41c/karpenter_installer_wizard/internal/aws"
	"github.com/L3n41c/karpenter_installer_wizard/internal/guess"
	"github.com/L3n41c/karpenter_installer_wizard/internal/helm"
)

var (
	//go:embed assets/cfn/podidentityrole.yaml
	PodIdentityRoleCfn string

	//go:embed assets/cfn/karpenter.yaml
	KarpenterCfn string

	//go:embed assets/charts/eks-pod-identity-agent-0.1.33.tgz
	EksPodIdentityAgentHelmChart []byte

	//go:embed assets/charts/karpenter-1.6.3.tgz
	KarpenterHelmChart []byte
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
		kubeConfig         = flag.String("kubeconfig", clientcmd.RecommendedHomeFile, "Path to the kubeconfig file")
		kubeContext        = flag.String("kube-context", "", "Name of the kubeconfig context to use")
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

	// Create CloudFormation stacks
	cloudformationClient := cloudformation.NewFromConfig(awsConfig)

	if err := aws.CreateOrUpdateStack(ctx, cloudformationClient, "dd-karpenter-"+*clusterName+"-karpenter", KarpenterCfn, map[string]string{
		"ClusterName": *clusterName,
	}); err != nil {
		log.Fatal(err)
	}

	if err := aws.CreateOrUpdateStack(ctx, cloudformationClient, "dd-karpenter-"+*clusterName+"-podidentityrole", PodIdentityRoleCfn, map[string]string{
		"ClusterName": *clusterName,
	}); err != nil {
		log.Fatal(err)
	}

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

	// Get kube client
	kubeClientConfig, err := k8sClientConfig.ClientConfig()
	if err != nil {
		log.Fatal(err)
	}
	kubeClientSet, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		log.Fatal(err)
	}

	// Add role mapping in the `aws-auth` ConfigMap
	if err := aws.EnsureAwsAuthRole(ctx, kubeClientSet, aws.RoleMapping{
		RoleArn:  "arn:aws:iam::" + accountID + ":role/KarpenterNodeRole-" + *clusterName,
		Username: "system:node:{{EC2PrivateDNSName}}",
		Groups:   []string{"system:bootstrappers", "system:nodes"},
	}); err != nil {
		log.Fatal(err)
	}

	// Install Helm charts
	restClientGetter := kube.GetConfig(*kubeConfig, *kubeContext, *karpenterNamespace)
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(restClientGetter, *karpenterNamespace, os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
		log.Fatal(err)
	}

	values := map[string]any{
		"clusterName": *clusterName,
		"env": map[string]any{
			"AWS_REGION": awsConfig.Region,
		},
	}

	if err := helm.CreateOrUpgrade(ctx, actionConfig, "eks-pod-identity-agent", *karpenterNamespace, EksPodIdentityAgentHelmChart, values); err != nil {
		log.Fatal(err)
	}

	values = map[string]any{
		"settings": map[string]any{
			"clusterName":       clusterName,
			"interruptionQueue": clusterName,
		},
	}

	if err := helm.CreateOrUpgrade(ctx, actionConfig, "karpenter", *karpenterNamespace, KarpenterHelmChart, values); err != nil {
		log.Fatal(err)
	}
}
