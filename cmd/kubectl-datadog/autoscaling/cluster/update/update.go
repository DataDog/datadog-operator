// Package update refreshes an existing kubectl-datadog Karpenter
// installation on an EKS cluster. It auto-detects immutable parameters
// (namespace, install-mode, fargate-subnets) from the CloudFormation stack
// laid down at install time and refuses to touch a Karpenter installation
// managed by another tool.
package update

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/display"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var updateExample = `
  # update a previously installed autoscaling
  %[1]s update
`

type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string

	clusterName              string
	karpenterNamespace       string
	karpenterVersion         string
	installMode              install.InstallMode
	fargateSubnets           []string
	createKarpenterResources install.CreateKarpenterResources
	inferenceMethod          install.InferenceMethod
	debug                    bool
}

func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
		// Default --install-mode mirrors install so a user passing it
		// explicitly still gets the same flag semantics. The actual mode
		// the update applies is the one detected on the cluster — see
		// resolveOptions.
		installMode: install.InstallModeFargate,
		// --create-karpenter-resources defaults to none on update so a
		// refresh does not blindly overwrite EC2NodeClass / NodePool
		// resources the user may have hand-edited. Pass --create-karpenter-resources=all
		// to regenerate them.
		createKarpenterResources: install.CreateKarpenterResourcesNone,
		inferenceMethod:          install.InferenceMethodNodeGroups,
	}
	o.SetConfigFlags()
	return o
}

// New returns the cobra command for `kubectl datadog autoscaling cluster update`.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "update",
		Short:        "Update an existing kubectl-datadog Karpenter installation on an EKS cluster",
		Example:      fmt.Sprintf(updateExample, "kubectl datadog autoscaling cluster"),
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

	cmd.Flags().StringVar(&o.clusterName, "cluster-name", "", "Name of the EKS cluster")
	cmd.Flags().StringVar(&o.karpenterNamespace, "karpenter-namespace", "dd-karpenter", "Override the auto-detected namespace where Karpenter is deployed (must match the existing install)")
	cmd.Flags().StringVar(&o.karpenterVersion, "karpenter-version", "", "Version of Karpenter to install (default to latest)")
	cmd.Flags().Var(&o.installMode, "install-mode", "Override the auto-detected install mode (must match the existing install): fargate or existing-nodes")
	cmd.Flags().StringSliceVar(&o.fargateSubnets, "fargate-subnets", nil, "Override the auto-detected Fargate profile subnets (must match the existing install). Only used when --install-mode=fargate.")
	cmd.Flags().Var(&o.createKarpenterResources, "create-karpenter-resources", "Which Karpenter resources to (re-)create: none (default), ec2nodeclass, all")
	cmd.Flags().Var(&o.inferenceMethod, "inference-method", "Method to infer EC2NodeClass and NodePool properties when --create-karpenter-resources is set: nodes, nodegroups")
	cmd.Flags().BoolVar(&o.debug, "debug", false, "Enable debug logs")

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

	if !slices.Contains([]install.InstallMode{install.InstallModeFargate, install.InstallModeExistingNodes}, o.installMode) {
		return errors.New("install-mode must be one of fargate or existing-nodes")
	}

	if !slices.Contains([]install.CreateKarpenterResources{install.CreateKarpenterResourcesNone, install.CreateKarpenterResourcesEC2NodeClass, install.CreateKarpenterResourcesAll}, o.createKarpenterResources) {
		return errors.New("create-karpenter-resources must be one of none, ec2nodeclass or all")
	}

	if !slices.Contains([]install.InferenceMethod{install.InferenceMethodNodes, install.InferenceMethodNodeGroups}, o.inferenceMethod) {
		return errors.New("inference-method must be one of nodes or nodegroups")
	}

	return nil
}

func (o *options) run(cmd *cobra.Command) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.SetOutput(o.ErrOut)
	ctrl.SetLogger(zap.New(zap.UseDevMode(false), zap.WriteTo(o.ErrOut)))

	clusterName := o.clusterName
	if clusterName == "" {
		if name, err := clients.GetClusterNameFromKubeconfig(o.ConfigFlags); err != nil {
			return err
		} else if name != "" {
			clusterName = name
		} else {
			return errors.New("cluster name must be specified either via --cluster-name or in the current kubeconfig context")
		}
	}

	cli, err := clients.Build(ctx, o.ConfigFlags, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to build clients: %w", err)
	}

	if err = clients.ValidateAWSAccountConsistency(ctx, cli, clusterName, o.ConfigFlags); err != nil {
		return err
	}

	// Refuse if no kubectl-datadog Karpenter is installed, or if a foreign
	// one occupies the cluster — we never modify what we did not install.
	k, err := guess.FindAnyKarpenterInstallation(ctx, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to check for an existing Karpenter installation: %w", err)
	}
	if k == nil {
		return fmt.Errorf("no Karpenter installation found on cluster %s; run 'kubectl datadog autoscaling cluster install' first", clusterName)
	}
	if !k.IsOwn() {
		display.PrintBox(o.Out,
			"A Karpenter installation managed by another tool was found on cluster "+clusterName+":",
			"Deployment "+k.Namespace+"/"+k.Name+".",
			"",
			"kubectl-datadog will not modify a Karpenter installation it did not create.",
		)
		return fmt.Errorf("refusing to update foreign Karpenter installation %s/%s", k.Namespace, k.Name)
	}

	ddStackName := install.DDKarpenterStackName(clusterName)
	ddStack, err := aws.GetStack(ctx, cli.CloudFormation, ddStackName)
	if err != nil {
		return fmt.Errorf("failed to read CloudFormation stack %s: %w", ddStackName, err)
	}
	if ddStack == nil {
		return fmt.Errorf("Karpenter Deployment %s/%s exists but its CloudFormation stack %s was not found; the install state is inconsistent — re-run 'install' to recover",
			k.Namespace, k.Name, ddStackName)
	}

	opts, err := o.resolveOptions(cmd.Flags().Changed, clusterName, ddStack)
	if err != nil {
		return err
	}

	// FindAnyKarpenterInstallation only inspects the first matching
	// Deployment, so a foreign install elsewhere on the cluster could slip
	// past the earlier guard if our Deployment was returned first. Re-scan
	// against the resolved namespace and refuse before delegating —
	// install.Run would otherwise treat the foreign install as a no-op
	// success.
	if foreign, err := guess.FindForeignKarpenterInstallation(ctx, o.Clientset, opts.KarpenterNamespace); err != nil {
		return fmt.Errorf("failed to check for additional Karpenter installations: %w", err)
	} else if foreign != nil {
		// FindForeignKarpenterInstallation also surfaces a kubectl-datadog
		// Deployment running in a different namespace, so the message is
		// kept tool-agnostic.
		display.PrintBox(o.Out,
			"An additional Karpenter installation was found on cluster "+clusterName+":",
			"Deployment "+foreign.Namespace+"/"+foreign.Name+".",
			"",
			"kubectl-datadog will not update while another Karpenter controller coexists.",
		)
		return fmt.Errorf("refusing to update while another Karpenter installation %s/%s coexists", foreign.Namespace, foreign.Name)
	}

	return install.Run(ctx, o.IOStreams, o.ConfigFlags, o.Clientset, opts)
}

// resolveOptions builds the install.RunOptions to pass to install.Run, using
// the dd-karpenter CFN stack as the source of truth for immutable parameters
// (namespace, install-mode, fargate-subnets). Flags the user explicitly set
// must match the detected values; flags left at their default are filled in
// from the stack. `changed` reports whether a flag was explicitly set by the
// user — typically `cmd.Flags().Changed`, parameterised so this function can
// be unit-tested without a cobra.Command.
func (o *options) resolveOptions(changed func(name string) bool, clusterName string, ddStack *aws.Stack) (install.RunOptions, error) {
	params := ddStack.ParameterMap()

	detectedNamespace := params["KarpenterNamespace"]
	if detectedNamespace == "" {
		return install.RunOptions{}, fmt.Errorf("CloudFormation stack %s has no KarpenterNamespace parameter; the install state is inconsistent",
			install.DDKarpenterStackName(clusterName))
	}

	detectedMode := install.DetectedInstallMode(ddStack)
	// Reject a corrupt or unknown tag here so we surface a clear error
	// before install.Run mutates the mode-independent CFN stack and only
	// then crashes on the unsupported mode in its switch statement.
	switch detectedMode {
	case install.InstallModeFargate, install.InstallModeExistingNodes:
	default:
		return install.RunOptions{}, fmt.Errorf("CloudFormation stack %s has unsupported install-mode tag %q",
			install.DDKarpenterStackName(clusterName), detectedMode)
	}

	var detectedSubnets []string
	if detectedMode == install.InstallModeFargate {
		if raw := params["FargateSubnets"]; raw != "" {
			detectedSubnets = strings.Split(raw, ",")
			slices.Sort(detectedSubnets)
		}
	}

	opts := install.RunOptions{
		ClusterName:              clusterName,
		KarpenterVersion:         o.karpenterVersion,
		CreateKarpenterResources: o.createKarpenterResources,
		InferenceMethod:          o.inferenceMethod,
		Debug:                    o.debug,
		ActionLabel:              "Updating",
		// run() pre-scans for a foreign Karpenter both before resolving
		// the namespace and after, so install.Run does not need to scan
		// a third time.
		SkipForeignKarpenterCheck: true,
	}

	if changed("karpenter-namespace") {
		if o.karpenterNamespace != detectedNamespace {
			return install.RunOptions{}, fmt.Errorf("--karpenter-namespace=%s does not match the detected install namespace %s; run 'uninstall' first to change it",
				o.karpenterNamespace, detectedNamespace)
		}
		opts.KarpenterNamespace = o.karpenterNamespace
	} else {
		opts.KarpenterNamespace = detectedNamespace
	}

	if changed("install-mode") {
		if o.installMode != detectedMode {
			return install.RunOptions{}, fmt.Errorf("--install-mode=%s does not match the detected install mode %s; run 'uninstall' first to switch",
				o.installMode, detectedMode)
		}
		opts.InstallMode = o.installMode
	} else {
		opts.InstallMode = detectedMode
	}

	if changed("fargate-subnets") {
		if opts.InstallMode != install.InstallModeFargate {
			return install.RunOptions{}, errors.New("--fargate-subnets can only be used with --install-mode=fargate")
		}
		userSubnets := slices.Clone(o.fargateSubnets)
		slices.Sort(userSubnets)
		if !slices.Equal(userSubnets, detectedSubnets) {
			return install.RunOptions{}, fmt.Errorf("--fargate-subnets %v does not match the detected install subnets %v; run 'uninstall' first to change them",
				o.fargateSubnets, detectedSubnets)
		}
		opts.FargateSubnets = o.fargateSubnets
	} else if opts.InstallMode == install.InstallModeFargate {
		opts.FargateSubnets = detectedSubnets
	}

	return opts, nil
}
