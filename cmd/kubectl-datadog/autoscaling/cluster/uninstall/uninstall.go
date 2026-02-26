// Package uninstall provides functionality to uninstall Karpenter
// autoscaling from EKS clusters, including resource cleanup and
// CloudFormation stack deletion.
package uninstall

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	karpawsv1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/display"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/helm"
	commonk8s "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/k8s"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

const (
	maxWaitDuration = 15 * time.Minute
)

var (
	clusterName        string
	karpenterNamespace string
	yes                bool
	uninstallExample   = `
  # uninstall autoscaling
  %[1]s uninstall
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
		Use:          "uninstall",
		Short:        "Uninstall autoscaling from an EKS cluster",
		Example:      fmt.Sprintf(uninstallExample, "kubectl datadog autoscaling cluster"),
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
	cmd.Flags().StringVar(&karpenterNamespace, "karpenter-namespace", "dd-karpenter", "Name of the Kubernetes namespace where Karpenter is deployed")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation prompt")

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

	return nil
}

func (o *options) run(cmd *cobra.Command) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.SetOutput(cmd.OutOrStderr())
	ctrl.SetLogger(zap.New(zap.UseDevMode(false), zap.WriteTo(cmd.ErrOrStderr())))

	if clusterName == "" {
		if name, err := clients.GetClusterNameFromKubeconfig(ctx, o.ConfigFlags); err != nil {
			return err
		} else if name != "" {
			clusterName = name
		} else {
			return errors.New("cluster name must be specified either via --cluster-name or in the current kubeconfig context")
		}
	}

	display.PrintBox(cmd.OutOrStdout(), "Uninstalling Karpenter from cluster "+clusterName+".")

	cli, err := clients.Build(ctx, o.ConfigFlags, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to build clients: %w", err)
	}

	nodePoolNames, nodes := displayResourceSummary(ctx, cmd, cli, clusterName)
	if len(nodes) > 0 && !yes {
		if promptConfirmation(cmd) != nil {
			return nil // User cancelled
		}
	}

	// Accumulate errors from cleanup steps - continue on failure to clean up as much as possible
	var errs []error

	if err = deleteKarpenterNodePools(ctx, cli); err != nil {
		log.Printf("Warning: failed to delete NodePools: %v", err)
		errs = append(errs, fmt.Errorf("NodePool deletion: %w", err))
	}

	if err = deleteKarpenterEC2NodeClasses(ctx, cli); err != nil {
		log.Printf("Warning: failed to delete EC2NodeClasses: %v", err)
		errs = append(errs, fmt.Errorf("EC2NodeClass deletion: %w", err))
	}

	if err = waitForKarpenterNodesToTerminate(ctx, cli, clusterName, nodePoolNames); err != nil {
		log.Printf("Warning: failed to wait for Karpenter nodes to terminate: %v", err)
		errs = append(errs, fmt.Errorf("node termination wait: %w", err))
	}

	if err = o.uninstallHelmChart(ctx, karpenterNamespace); err != nil {
		log.Printf("Warning: failed to uninstall Helm chart: %v", err)
		errs = append(errs, fmt.Errorf("Helm uninstall: %w", err))
	}

	if err = removeAwsAuthConfigMapRole(ctx, cli, clusterName); err != nil {
		log.Printf("Warning: failed to remove aws-auth role: %v", err)
		errs = append(errs, fmt.Errorf("aws-auth role removal: %w", err))
	}

	if err = deleteKarpenterInstanceProfiles(ctx, cli, clusterName); err != nil {
		log.Printf("Warning: failed to delete Karpenter instance profiles: %v", err)
		errs = append(errs, fmt.Errorf("instance profile cleanup: %w", err))
	}

	if err = deleteCloudFormationStacks(ctx, cli, clusterName); err != nil {
		log.Printf("Warning: failed to delete CloudFormation stacks: %v", err)
		errs = append(errs, fmt.Errorf("CloudFormation stack deletion: %w", err))
	}

	if len(errs) > 0 {
		display.PrintBox(cmd.OutOrStdout(),
			color.RedString("❌ Uninstall completed with %d errors.", len(errs)),
			color.RedString("Some resources may not have been cleaned up."),
		)
		return fmt.Errorf("uninstall encountered %d error(s):\n%w", len(errs), errors.Join(errs...))
	}

	display.PrintBox(cmd.OutOrStdout(), "✅ Karpenter uninstalled from cluster "+clusterName+".")

	return nil
}

func displayResourceSummary(ctx context.Context, cmd *cobra.Command, cli *clients.Clients, clusterName string) (nodePools []string, nodes []string) {
	cmd.Println("\nThis will delete:")

	if n, err := listKarpenterNodePools(ctx, cli); err != nil {
		cmd.Printf("  - NodePools: (unable to list: %v)\n", err)
	} else if len(n) == 0 {
		cmd.Println("  - NodePools: none found")
	} else {
		nodePools = n
		cmd.Printf("  - %d NodePool(s):\n", len(nodePools))
		for _, np := range nodePools {
			cmd.Printf("      • %s\n", np)
		}
	}

	nodeClasses, err := listKarpenterEC2NodeClasses(ctx, cli)
	if err != nil {
		cmd.Printf("  - EC2NodeClasses: (unable to list: %v)\n", err)
	} else if len(nodeClasses) == 0 {
		cmd.Println("  - EC2NodeClasses: none found")
	} else {
		cmd.Printf("  - %d EC2NodeClass(es):\n", len(nodeClasses))
		for _, nc := range nodeClasses {
			cmd.Printf("      • %s\n", nc)
		}
	}

	if err != nil {
		cmd.Println("  - Karpenter nodes: (unable to list - depends on EC2NodeClasses)")
	} else if n, err := listKarpenterNodes(ctx, cli, nodeClasses); err != nil {
		cmd.Printf("  - Karpenter nodes: (unable to list: %v)\n", err)
	} else if len(n) == 0 {
		cmd.Println("  - Karpenter nodes: none found")
	} else {
		nodes = n
		cmd.Printf("  - %d Karpenter-managed node(s):\n", len(nodes))
		for _, node := range nodes {
			cmd.Printf("      • %s\n", node)
		}
	}

	cmd.Println("  - The Karpenter Helm release")

	if stacks, err := listCloudFormationStacks(ctx, cli, clusterName); err != nil {
		cmd.Printf("  - CloudFormation stacks: (unable to list: %v)\n", err)
	} else if len(stacks) == 0 {
		cmd.Println("  - CloudFormation stacks: none found")
	} else {
		cmd.Printf("  - %d CloudFormation stack(s):\n", len(stacks))
		for _, stack := range stacks {
			cmd.Printf("      • %s\n", stack)
		}
	}

	cmd.Println("  - aws-auth ConfigMap role mappings (if applicable)")

	if len(nodes) > 0 {
		cmd.Println()
		cmd.Println(color.YellowString("⚠ WARNING: %d Karpenter node(s) will be drained and terminated.", len(nodes)))
	}

	return nodePools, nodes
}

func promptConfirmation(cmd *cobra.Command) error {
	cmd.Print("\nContinue? (y/N): ")

	var response string
	fmt.Fscanln(cmd.InOrStdin(), &response)
	if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
		cmd.Println("Uninstall cancelled.")
		return fmt.Errorf("cancelled")
	}
	return nil
}

func listKarpenterNodePools(ctx context.Context, cli *clients.Clients) ([]string, error) {
	nodePoolList := &karpv1.NodePoolList{}
	nodePoolList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "karpenter.sh",
		Version: "v1",
		Kind:    "NodePoolList",
	})

	if err := cli.K8sClient.List(ctx, nodePoolList, client.MatchingLabels{
		"app.kubernetes.io/managed-by":      "kubectl-datadog",
		"autoscaling.datadoghq.com/created": "true",
	}); err != nil {
		if meta.IsNoMatchError(err) {
			return nil, nil // CRD not installed, no NodePools
		}
		return nil, err
	}

	return lo.Map(nodePoolList.Items, func(np karpv1.NodePool, _ int) string {
		return np.Name
	}), nil
}

func deleteKarpenterNodePools(ctx context.Context, cli *clients.Clients) error {
	log.Println("Deleting Karpenter NodePool resources…")

	nodePoolList := &karpv1.NodePoolList{}
	nodePoolList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "karpenter.sh",
		Version: "v1",
		Kind:    "NodePoolList",
	})

	if err := cli.K8sClient.List(ctx, nodePoolList, client.MatchingLabels{
		"app.kubernetes.io/managed-by":      "kubectl-datadog",
		"autoscaling.datadoghq.com/created": "true",
	}); err != nil {
		if meta.IsNoMatchError(err) {
			log.Println("NodePool CRD not found, skipping deletion.")
			return nil
		}
		return fmt.Errorf("failed to list NodePools: %w", err)
	}

	log.Printf("Found %d NodePool resource(s) to delete.", len(nodePoolList.Items))

	for _, np := range nodePoolList.Items {
		if err := commonk8s.Delete(ctx, cli.K8sClient, &np); err != nil {
			return err
		}
	}

	return nil
}

func listKarpenterEC2NodeClasses(ctx context.Context, cli *clients.Clients) ([]string, error) {
	ec2NodeClassList := &karpawsv1.EC2NodeClassList{}
	ec2NodeClassList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "karpenter.k8s.aws",
		Version: "v1",
		Kind:    "EC2NodeClassList",
	})

	if err := cli.K8sClient.List(ctx, ec2NodeClassList, client.MatchingLabels{
		"app.kubernetes.io/managed-by":      "kubectl-datadog",
		"autoscaling.datadoghq.com/created": "true",
	}); err != nil {
		if meta.IsNoMatchError(err) {
			return nil, nil // CRD not installed, no EC2NodeClasses
		}
		return nil, err
	}

	return lo.Map(ec2NodeClassList.Items, func(nc karpawsv1.EC2NodeClass, _ int) string {
		return nc.Name
	}), nil
}

func deleteKarpenterEC2NodeClasses(ctx context.Context, cli *clients.Clients) error {
	log.Println("Deleting Karpenter EC2NodeClass resources…")

	ec2NodeClassList := &karpawsv1.EC2NodeClassList{}
	ec2NodeClassList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "karpenter.k8s.aws",
		Version: "v1",
		Kind:    "EC2NodeClassList",
	})

	if err := cli.K8sClient.List(ctx, ec2NodeClassList, client.MatchingLabels{
		"app.kubernetes.io/managed-by":      "kubectl-datadog",
		"autoscaling.datadoghq.com/created": "true",
	}); err != nil {
		if meta.IsNoMatchError(err) {
			log.Println("EC2NodeClass CRD not found, skipping deletion.")
			return nil
		}
		return fmt.Errorf("failed to list EC2NodeClasses: %w", err)
	}

	log.Printf("Found %d EC2NodeClass resource(s) to delete.", len(ec2NodeClassList.Items))

	for _, nc := range ec2NodeClassList.Items {
		if err := commonk8s.Delete(ctx, cli.K8sClient, &nc); err != nil {
			return err
		}
	}

	return nil
}

func listKarpenterNodes(ctx context.Context, cli *clients.Clients, ec2NodeClassNames []string) ([]string, error) {
	if len(ec2NodeClassNames) == 0 {
		return nil, nil // No EC2NodeClasses to match
	}

	// List all Karpenter-managed nodes
	nodesList, err := cli.K8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: "karpenter.k8s.aws/ec2nodeclass",
	})
	if err != nil {
		return nil, err
	}

	// Filter nodes that belong to our EC2NodeClasses
	nodeClassSet := lo.SliceToMap(ec2NodeClassNames, func(name string) (string, struct{}) {
		return name, struct{}{}
	})

	return lo.FilterMap(nodesList.Items, func(node corev1.Node, _ int) (string, bool) {
		nodeClass := node.Labels["karpenter.k8s.aws/ec2nodeclass"]
		_, matches := nodeClassSet[nodeClass]
		return node.Name, matches
	}), nil
}

func waitForKarpenterNodesToTerminate(ctx context.Context, cli *clients.Clients, clusterName string, nodePoolNames []string) error {
	if len(nodePoolNames) == 0 {
		log.Println("No managed NodePools to wait for.")
		return nil
	}

	log.Println("Waiting for Karpenter-managed nodes to terminate…")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(maxWaitDuration)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for Karpenter nodes to terminate after %v", maxWaitDuration)
		case <-ticker.C:
			result, err := cli.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				Filters: []ec2types.Filter{
					{
						Name:   awssdk.String("tag:kubernetes.io/cluster/" + clusterName),
						Values: []string{"owned"},
					},
					{
						Name:   awssdk.String("tag:karpenter.sh/nodepool"),
						Values: nodePoolNames,
					},
					{
						Name: awssdk.String("instance-state-name"),
						Values: []string{
							string(ec2types.InstanceStateNamePending),
							string(ec2types.InstanceStateNameRunning),
							string(ec2types.InstanceStateNameStopping),
							string(ec2types.InstanceStateNameStopped),
							string(ec2types.InstanceStateNameShuttingDown),
						},
					},
				},
			})
			if err != nil {
				return fmt.Errorf("failed to describe EC2 instances: %w", err)
			}

			instanceCount := lo.SumBy(result.Reservations, func(r ec2types.Reservation) int {
				return len(r.Instances)
			})

			if instanceCount == 0 {
				log.Println("All Karpenter-managed nodes have been terminated.")
				return nil
			}

			log.Printf("Waiting for %d Karpenter-managed instance(s) to terminate…", instanceCount)
		}
	}
}

func (o *options) uninstallHelmChart(ctx context.Context, karpenterNamespace string) error {
	actionConfig, err := helm.NewActionConfig(o.ConfigFlags, karpenterNamespace)
	if err != nil {
		return err
	}

	if err := helm.Uninstall(ctx, actionConfig, "karpenter"); err != nil {
		return fmt.Errorf("failed to uninstall Helm release: %w", err)
	}

	return nil
}

func removeAwsAuthConfigMapRole(ctx context.Context, cli *clients.Clients, clusterName string) error {
	awsAuthConfigMapPresent, err := guess.IsAwsAuthConfigMapPresent(ctx, cli.K8sClientset)
	if err != nil {
		return fmt.Errorf("failed to check if aws-auth ConfigMap is present: %w", err)
	}

	if !awsAuthConfigMapPresent {
		log.Println("aws-auth ConfigMap not present, skipping role removal.")
		return nil
	}

	// Get AWS account ID
	callerIdentity, err := cli.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get identity caller: %w", err)
	}
	if callerIdentity.Account == nil {
		return errors.New("unable to determine AWS account ID from STS GetCallerIdentity")
	}
	accountID := *callerIdentity.Account

	roleArn := "arn:aws:iam::" + accountID + ":role/KarpenterNodeRole-" + clusterName

	if err = aws.RemoveAwsAuthRole(ctx, cli.K8sClientset, roleArn); err != nil {
		return fmt.Errorf("failed to remove aws-auth role: %w", err)
	}

	return nil
}

func deleteKarpenterInstanceProfiles(ctx context.Context, cli *clients.Clients, clusterName string) error {
	log.Println("Cleaning up Karpenter instance profiles…")

	roleName := "KarpenterNodeRole-" + clusterName

	// List instance profiles associated with the Karpenter node role
	out, err := cli.IAM.ListInstanceProfilesForRole(ctx, &iam.ListInstanceProfilesForRoleInput{
		RoleName: awssdk.String(roleName),
	})
	if err != nil {
		// If the role doesn't exist, there's nothing to clean up
		var noSuchEntity *iamtypes.NoSuchEntityException
		if errors.As(err, &noSuchEntity) {
			log.Println("KarpenterNodeRole not found, skipping instance profile cleanup.")
			return nil
		}
		return fmt.Errorf("failed to list instance profiles for role %s: %w", roleName, err)
	}

	for _, profile := range out.InstanceProfiles {
		profileName := awssdk.ToString(profile.InstanceProfileName)

		// Remove all roles from the instance profile
		for _, role := range profile.Roles {
			rn := awssdk.ToString(role.RoleName)
			log.Printf("Removing role %s from instance profile %s…", rn, profileName)
			if _, err := cli.IAM.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
				InstanceProfileName: profile.InstanceProfileName,
				RoleName:            role.RoleName,
			}); err != nil {
				return fmt.Errorf("failed to remove role %s from instance profile %s: %w", rn, profileName, err)
			}
		}

		// Delete the instance profile
		log.Printf("Deleting instance profile %s…", profileName)
		if _, err := cli.IAM.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{
			InstanceProfileName: profile.InstanceProfileName,
		}); err != nil {
			return fmt.Errorf("failed to delete instance profile %s: %w", profileName, err)
		}
	}

	log.Printf("Cleaned up %d instance profile(s).", len(out.InstanceProfiles))
	return nil
}

func listCloudFormationStacks(ctx context.Context, cli *clients.Clients, clusterName string) ([]string, error) {
	stackNames := []string{
		"dd-karpenter-" + clusterName + "-karpenter",
		"dd-karpenter-" + clusterName + "-dd-karpenter",
	}

	var existing []string
	for _, name := range stackNames {
		exists, err := aws.DoesStackExist(ctx, cli.CloudFormation, name)
		if err != nil {
			return nil, err
		}
		if exists {
			existing = append(existing, name)
		}
	}
	return existing, nil
}

func deleteCloudFormationStacks(ctx context.Context, cli *clients.Clients, clusterName string) error {
	if err := aws.DeleteStack(ctx, cli.CloudFormation, "dd-karpenter-"+clusterName+"-dd-karpenter"); err != nil {
		return fmt.Errorf("failed to delete dd-karpenter CloudFormation stack: %w", err)
	}

	if err := aws.DeleteStack(ctx, cli.CloudFormation, "dd-karpenter-"+clusterName+"-karpenter"); err != nil {
		return fmt.Errorf("failed to delete karpenter CloudFormation stack: %w", err)
	}

	return nil
}
