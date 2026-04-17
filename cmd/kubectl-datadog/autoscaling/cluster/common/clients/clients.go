// Package clients provides shared AWS and Kubernetes client initialization
// for the Karpenter install and uninstall commands.
package clients

import (
	"context"
	"errors"
	"fmt"
	"log"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	karpawsv1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
)

// Clients holds all AWS and Kubernetes client instances needed for
// Karpenter installation and uninstallation operations.
type Clients struct {
	// AWS clients
	Config         awssdk.Config
	CloudFormation *cloudformation.Client
	EC2            *ec2.Client
	EKS            *eks.Client
	IAM            *iam.Client
	STS            *sts.Client

	// Kubernetes clients
	K8sClient    client.Client         // controller-runtime client
	K8sClientset *kubernetes.Clientset // typed Kubernetes client
}

// Build creates AWS and Kubernetes clients for Karpenter operations.
func Build(ctx context.Context, configFlags *genericclioptions.ConfigFlags, k8sClientset *kubernetes.Clientset) (*Clients, error) {
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	sch := runtime.NewScheme()

	if err = scheme.AddToScheme(sch); err != nil {
		return nil, fmt.Errorf("failed to add base scheme: %w", err)
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

	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get REST config: %w", err)
	}

	httpClient, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create http client: %w", err)
	}

	mapper, err := apiutil.NewDynamicRESTMapper(restConfig, httpClient)
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate mapper: %w", err)
	}

	k8sClient, err := client.New(restConfig, client.Options{
		Scheme: sch,
		Mapper: mapper,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Karpenter client: %w", err)
	}

	return &Clients{
		Config:         awsConfig,
		CloudFormation: cloudformation.NewFromConfig(awsConfig),
		EC2:            ec2.NewFromConfig(awsConfig),
		EKS:            eks.NewFromConfig(awsConfig),
		IAM:            iam.NewFromConfig(awsConfig),
		STS:            sts.NewFromConfig(awsConfig),
		K8sClient:      k8sClient,
		K8sClientset:   k8sClientset,
	}, nil
}

// resolveKubeContext loads the kubeconfig and resolves the active context name.
func resolveKubeContext(configFlags *genericclioptions.ConfigFlags) (api.Config, string, error) {
	kubeRawConfig, err := configFlags.ToRawKubeConfigLoader().RawConfig()
	if err != nil {
		return api.Config{}, "", fmt.Errorf("failed to get raw kubeconfig: %w", err)
	}

	kubeContext := ""
	if configFlags.Context != nil {
		kubeContext = *configFlags.Context
	}
	if kubeContext == "" {
		kubeContext = kubeRawConfig.CurrentContext
	}

	return kubeRawConfig, kubeContext, nil
}

// GetClusterNameFromKubeconfig extracts the EKS cluster name from the current kubeconfig context.
func GetClusterNameFromKubeconfig(configFlags *genericclioptions.ConfigFlags) (string, error) {
	kubeRawConfig, kubeContext, err := resolveKubeContext(configFlags)
	if err != nil {
		return "", err
	}

	return guess.GetClusterNameFromKubeconfig(kubeRawConfig, kubeContext), nil
}

// getAccountIDFromKubeconfig attempts to extract the AWS account ID from the
// kubeconfig context. Returns an empty string if the context is not an EKS ARN.
func getAccountIDFromKubeconfig(configFlags *genericclioptions.ConfigFlags) (string, error) {
	kubeRawConfig, kubeContext, err := resolveKubeContext(configFlags)
	if err != nil || kubeContext == "" {
		return "", err
	}

	kubeCtx, exists := kubeRawConfig.Contexts[kubeContext]
	if !exists {
		return "", fmt.Errorf("kube context %q doesn’t exist", kubeContext)
	}

	// The kubeconfig cluster field may not be an ARN (e.g. plain name,
	// eksctl FQDN). Treat that as a normal fallback, not an error.
	if !arn.IsARN(kubeCtx.Cluster) {
		return "", nil
	}

	parsed, err := arn.Parse(kubeCtx.Cluster)
	if err != nil {
		return "", fmt.Errorf("failed to parse EKS cluster ARN %q: %w", kubeCtx.Cluster, err)
	}

	return parsed.AccountID, nil
}

// GetAWSAccountID returns the AWS account ID from the current credentials.
func GetAWSAccountID(ctx context.Context, cli *Clients) (string, error) {
	callerIdentity, err := cli.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get AWS caller identity: %w", err)
	}
	if callerIdentity.Account == nil {
		return "", errors.New("unable to determine AWS account ID from STS GetCallerIdentity")
	}
	return *callerIdentity.Account, nil
}

// ValidateAWSAccountConsistency prevents accidental cross-account resource
// deployment by verifying that the current AWS credentials and the target
// EKS cluster belong to the same AWS account.
//
// The cluster's account ID is derived from the kubeconfig context ARN when
// available. This is independent of the AWS credentials and cannot be fooled
// by same-named clusters in different accounts. When the kubeconfig context
// is not an ARN, falls back to DescribeCluster.
func ValidateAWSAccountConsistency(ctx context.Context, cli *Clients, clusterName string, configFlags *genericclioptions.ConfigFlags) error {
	credentialsAccountID, err := GetAWSAccountID(ctx, cli)
	if err != nil {
		return err
	}

	clusterAccountID, err := getAccountIDFromKubeconfig(configFlags)
	if err != nil {
		log.Printf("Warning: failed to get AWS account ID from kubeconfig: %v", err)
	}
	if clusterAccountID == "" {
		cluster, err := cli.EKS.DescribeCluster(ctx, &eks.DescribeClusterInput{
			Name: awssdk.String(clusterName),
		})
		if err != nil {
			return fmt.Errorf("failed to describe EKS cluster %s: %w", clusterName, err)
		}
		if cluster.Cluster == nil || cluster.Cluster.Arn == nil {
			return fmt.Errorf("EKS cluster %s has no ARN", clusterName)
		}

		clusterArn, err := arn.Parse(*cluster.Cluster.Arn)
		if err != nil {
			return fmt.Errorf("failed to parse EKS cluster ARN %q: %w", *cluster.Cluster.Arn, err)
		}
		clusterAccountID = clusterArn.AccountID
	}

	if credentialsAccountID != clusterAccountID {
		return &AccountMismatchError{
			CredentialsAccountID: credentialsAccountID,
			ClusterAccountID:     clusterAccountID,
			ClusterName:          clusterName,
		}
	}

	return nil
}

// AccountMismatchError indicates that the AWS credentials and the EKS cluster
// belong to different AWS accounts.
type AccountMismatchError struct {
	CredentialsAccountID string
	ClusterAccountID     string
	ClusterName          string
}

func (e *AccountMismatchError) Error() string {
	return fmt.Sprintf(
		"AWS account mismatch: current credentials belong to account %s, "+
			"but EKS cluster %q belongs to account %s; "+
			"ensure your AWS credentials and kubeconfig target the same AWS account",
		e.CredentialsAccountID, e.ClusterName, e.ClusterAccountID,
	)
}
