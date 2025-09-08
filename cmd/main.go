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

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"

	"github.com/L3n41c/karpenter_installer_wizard/internal/aws"
	"github.com/L3n41c/karpenter_installer_wizard/internal/guess"
	"github.com/L3n41c/karpenter_installer_wizard/internal/helm"
)

var (
	//go:embed assets/cloudformation.yaml
	CloudformationTemplate string

	//go:embed assets/karpenter-1.6.3.tgz
	HelmChart []byte
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	guessclusterName := guess.GetClusterNameFromKubeconfig(ctx)

	var (
		clusterName        = flag.String("cluster-name", guessclusterName, "Name of the EKS cluster")
		stackName          = flag.String("stack-name", "karpenter-stack-find-a-better-name", "Name of the CloudFormation stack")
		karpenterNamespace = flag.String("karpenter-namespace", "dd-karpenter", "Name of the Kubernetes namespace in which deploying Karpenter")
	)
	flag.Parse()

	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}
	cloudformationClient := cloudformation.NewFromConfig(awsConfig)

	if err := aws.CreateOrUpdateStack(ctx, cloudformationClient, *stackName, CloudformationTemplate, map[string]string{
		"ClusterName": *clusterName,
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
		"settings": map[string]any{
			"clusterName":       clusterName,
			"interruptionQueue": clusterName,
		},
	}

	if err := helm.CreateOrUpgrade(ctx, actionConfig, "karpenter", *karpenterNamespace, HelmChart, values); err != nil {
		log.Fatal(err)
	}
}
