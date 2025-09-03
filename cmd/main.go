package main

import (
	"context"
	_ "embed"
	"flag"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"

	"github.com/L3n41c/karpenter_installer_wizard/internal/aws"
	"github.com/L3n41c/karpenter_installer_wizard/internal/guess"
)

var (
	//go:embed assets/cloudformation.yaml
	CloudformationTemplate string
)

func main() {
	ctx := context.Background()
	guessclusterName := guess.GetClusterNameFromKubeconfig(ctx)

	var (
		clusterName = flag.String("cluster-name", guessclusterName, "EKS cluster name")
		stackName   = flag.String("stack-name", "karpenter-stack-find-a-better-name", "Name of the CloudFormation stack")
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
}
