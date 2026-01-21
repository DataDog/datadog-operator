// Package clients provides shared AWS and Kubernetes client initialization
// for the Karpenter install and uninstall commands.
package clients

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	karpawsv1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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
		STS:            sts.NewFromConfig(awsConfig),
		K8sClient:      k8sClient,
		K8sClientset:   k8sClientset,
	}, nil
}

// GetClusterNameFromKubeconfig extracts the EKS cluster name from the current kubeconfig context.
func GetClusterNameFromKubeconfig(ctx context.Context, configFlags *genericclioptions.ConfigFlags) (string, error) {
	kubeRawConfig, err := configFlags.ToRawKubeConfigLoader().RawConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get raw kubeconfig: %w", err)
	}

	kubeContext := ""
	if configFlags.Context != nil {
		kubeContext = *configFlags.Context
	}

	return guess.GetClusterNameFromKubeconfig(ctx, kubeRawConfig, kubeContext), nil
}
