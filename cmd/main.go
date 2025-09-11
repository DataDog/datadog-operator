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
	"helm.sh/helm/v3/pkg/cli"

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

	guessclusterName := guess.GetClusterNameFromKubeconfig(ctx)

	var (
		clusterName        = flag.String("cluster-name", guessclusterName, "Name of the EKS cluster")
		karpenterNamespace = flag.String("karpenter-namespace", "dd-karpenter", "Name of the Kubernetes namespace in which deploying Karpenter")
		kubeconfig         = flag.String("kubeconfig", clientcmd.RecommendedHomeFile, "Path to the kubeconfig file")
	)
	flag.Parse()

	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}
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

	stsClient := sts.NewFromConfig(awsConfig)
	callerIdentity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		log.Fatal(err)
	}
	if callerIdentity.Account == nil {
		log.Fatal("unable to determine AWS account ID from STS GetCallerIdentity")
	}
	accountID := *callerIdentity.Account

	k8sConfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatal(err)
	}
	k8sClientSet, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		log.Fatal(err)
	}

	if err := aws.EnsureAwsAuthRole(ctx, k8sClientSet, aws.RoleMapping{
		RoleArn:  "arn:aws:iam::" + accountID + ":role/KarpenterNodeRole-" + *clusterName,
		Username: "system:node:{{EC2PrivateDNSName}}",
		Groups:   []string{"system:bootstrappers", "system:nodes"},
	}); err != nil {
		log.Fatal(err)
	}

	settings := cli.New()
	settings.SetNamespace(*karpenterNamespace)
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), *karpenterNamespace, os.Getenv("HELM_DRIVER"), log.Printf); err != nil {
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
