// Package upgrade provides functionality to upgrade a Karpenter installation
// that was previously deployed by the kubectl-datadog plugin.
package upgrade

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/display"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/helm"
	commonk8s "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/k8s"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var (
	clusterName              string
	karpenterVersion         string
	updateKarpenterResources bool
	inferenceMethod          = install.InferenceMethodNodeGroups
	debug                    bool
	upgradeExample           = `
  # upgrade autoscaling
  %[1]s upgrade

  # upgrade to a specific Karpenter version
  %[1]s upgrade --karpenter-version 1.1.0

  # upgrade and re-infer NodePool/EC2NodeClass resources
  %[1]s upgrade --update-karpenter-resources
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

// New provides a cobra command for upgrading an existing Karpenter installation.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "upgrade",
		Short:        "Upgrade Karpenter on an EKS cluster",
		Long:         "Upgrade a Karpenter installation that was previously deployed by kubectl-datadog. The installation namespace is auto-detected. Helm values are reset to the plugin's defaults on each upgrade.",
		Example:      fmt.Sprintf(upgradeExample, "kubectl datadog autoscaling cluster"),
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

	cmd.Flags().StringVar(&clusterName, "cluster-name", "", "Name of the EKS cluster (auto-detected from existing installation if not specified)")
	cmd.Flags().StringVar(&karpenterVersion, "karpenter-version", "", "Version of Karpenter to upgrade to (default to latest)")
	cmd.Flags().BoolVar(&updateKarpenterResources, "update-karpenter-resources", false, "Re-infer and update NodePool/EC2NodeClass resources")
	cmd.Flags().Var(&inferenceMethod, "inference-method", "Method to infer EC2NodeClass and NodePool properties: nodes, nodegroups (only used with --update-karpenter-resources)")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logs")

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	return o.Init(cmd)
}

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

	// Detect the existing installation and get the Helm release in one pass
	karpenterNamespace, rel, err := o.detectInstallation(ctx)
	if err != nil {
		return err
	}

	// Resolve cluster name: flag > Helm release values > kubeconfig
	if clusterName == "" {
		clusterName = extractClusterName(rel.Config)
	}
	if clusterName == "" {
		name, err := clients.GetClusterNameFromKubeconfig(ctx, o.ConfigFlags)
		if err != nil {
			return err
		}
		if name != "" {
			clusterName = name
		} else {
			return errors.New("cluster name must be specified either via --cluster-name, from the existing installation, or in the current kubeconfig context")
		}
	}

	autoModeEnabled, err := guess.IsEKSAutoModeEnabled(o.DiscoveryClient)
	if err != nil {
		return fmt.Errorf("failed to check for EKS auto-mode: %w", err)
	}
	if autoModeEnabled {
		return fmt.Errorf("EKS auto-mode is active on cluster %s; Karpenter is built into EKS auto-mode and cannot be upgraded separately", clusterName)
	}

	display.PrintBox(cmd.OutOrStdout(), "Upgrading Karpenter on cluster "+clusterName+" (namespace: "+karpenterNamespace+").")

	cli, err := clients.Build(ctx, o.ConfigFlags, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to build clients: %w", err)
	}

	if err = install.CreateCloudFormationStacks(ctx, cli, clusterName, karpenterNamespace); err != nil {
		return err
	}

	if err = install.UpdateAwsAuthConfigMap(ctx, cli, clusterName); err != nil {
		return err
	}

	if err = install.InstallOrUpgradeHelmChart(ctx, o.ConfigFlags, clusterName, karpenterNamespace, karpenterVersion, debug); err != nil {
		return err
	}

	if updateKarpenterResources {
		if err = install.CreateNodePoolResources(ctx, cmd, cli, clusterName, install.CreateKarpenterResourcesAll, inferenceMethod, debug); err != nil {
			return err
		}
	}

	display.PrintBox(cmd.OutOrStdout(), "Karpenter upgraded on cluster "+clusterName+".")

	return nil
}

// detectInstallation finds the existing Karpenter installation namespace and
// returns the Helm release. Checks webhooks first, then falls back to Helm secret scanning.
func (o *options) detectInstallation(ctx context.Context) (string, *release.Release, error) {
	found, namespace, err := commonk8s.DetectActiveKarpenter(ctx, o.Clientset)
	if err != nil {
		return "", nil, fmt.Errorf("failed to detect active Karpenter: %w", err)
	}

	if found && namespace == "" {
		return "", nil, fmt.Errorf("an external Karpenter installation was detected (URL-based webhook); upgrade is only supported for installations created by kubectl-datadog")
	}

	if found {
		isOurs, rel, checkErr := helm.IsOurRelease(ctx, o.ConfigFlags, namespace, "karpenter")
		if checkErr != nil {
			return "", nil, fmt.Errorf("failed to check Karpenter Helm release ownership: %w", checkErr)
		}
		if !isOurs {
			return "", nil, fmt.Errorf("the Karpenter installation in namespace %q was not created by kubectl-datadog", namespace)
		}
		log.Printf("Found kubectl-datadog Karpenter installation in namespace %q.", namespace)
		return namespace, rel, nil
	}

	// Fallback: scan for Helm release secrets across namespaces
	helmFound, namespaces, err := commonk8s.FindKarpenterHelmRelease(ctx, o.Clientset)
	if err != nil {
		return "", nil, fmt.Errorf("failed to find Karpenter Helm release: %w", err)
	}

	if !helmFound {
		return "", nil, errors.New("no Karpenter installation found; use 'kubectl datadog autoscaling cluster install' first")
	}

	var ours []string
	var oursRelease *release.Release
	var checkErrors []error
	for _, ns := range namespaces {
		isOurs, rel, checkErr := helm.IsOurRelease(ctx, o.ConfigFlags, ns, "karpenter")
		if checkErr != nil {
			checkErrors = append(checkErrors, fmt.Errorf("namespace %q: %w", ns, checkErr))
			continue
		}
		if isOurs {
			ours = append(ours, ns)
			oursRelease = rel
		}
	}

	switch len(ours) {
	case 0:
		if len(checkErrors) > 0 {
			return "", nil, fmt.Errorf("failed to check Karpenter Helm release ownership: %w", errors.Join(checkErrors...))
		}
		return "", nil, fmt.Errorf("found Karpenter Helm release(s) but none were created by kubectl-datadog")
	case 1:
		log.Printf("Found kubectl-datadog Karpenter installation in namespace %q.", ours[0])
		return ours[0], oursRelease, nil
	default:
		return "", nil, fmt.Errorf("multiple kubectl-datadog Karpenter installations found in namespaces %v; this is unexpected — please uninstall the extra release(s) first", ours)
	}
}

func extractClusterName(config map[string]any) string {
	if config == nil {
		return ""
	}
	settings, ok := config["settings"].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := settings["clusterName"].(string)
	return name
}
