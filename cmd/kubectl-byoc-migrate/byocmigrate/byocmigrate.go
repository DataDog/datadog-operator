// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package byocmigrate provides the "byoc-migrate" command, which converts the objects that the CloudPrem and Observability
// Pipelines Worker Helm charts rendered to a DatadogBYOCCluster, and its subcommands, which migrate them to the cluster.
package byocmigrate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/converter"
	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/migration"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

const example = `
  # write the objects of the CloudPrem and Observability Pipelines Worker Helm releases to files
  helm get manifest cloudprem > cloudprem.yaml
  helm get manifest logs > logs.yaml

  # convert the objects of the current namespace, the Worker of logs.yaml becoming the pipeline "logs"
  kubectl byoc-migrate convert --cloudprem-manifest cloudprem.yaml --opw-manifest logs.yaml=logs > cluster.yaml

  # review what the migration changes, after editing the converted cluster if needed
  kubectl byoc-migrate diff --cloudprem-manifest cloudprem.yaml --opw-manifest logs.yaml=logs --from-file cluster.yaml

  # once the manifests are no longer applied, apply the cluster, for example from a deployment pipeline
  kubectl apply -f cluster.yaml

  # review the changes again with the applied cluster, right before adopt
  kubectl byoc-migrate diff --cloudprem-manifest cloudprem.yaml --opw-manifest logs.yaml=logs

  # migrate the objects to the cluster
  kubectl byoc-migrate adopt --cloudprem-manifest cloudprem.yaml --opw-manifest logs.yaml=logs

  # revert the migration, then delete the cluster, for example from the deployment pipeline
  kubectl byoc-migrate abort --cloudprem-manifest cloudprem.yaml --opw-manifest logs.yaml=logs
  kubectl delete -f cluster.yaml
`

// New provides the "byoc-migrate" command, whose subcommands convert and migrate the objects of the charts.
func New(streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "byoc-migrate",
		Short: "Migrate the objects of the CloudPrem and Observability Pipelines Worker Helm charts to a DatadogBYOCCluster",
		Long: `Migrate the objects that the CloudPrem and Observability Pipelines Worker Helm charts rendered to a
DatadogBYOCCluster.

The manifests are the objects of the charts, as helm template or helm get manifest prints them, whether Helm or another
tool applied them. The cluster takes over the CloudPrem objects in place, and its pipelines replace the Observability
Pipelines Worker objects with new Workers. convert prints the cluster, diff prints what the migration changes, adopt
gives the objects to the cluster, and abort takes them back from it. The commands do not create, change, or delete the
cluster: apply and delete it yourself, for example from a deployment pipeline. Pass the same manifests to every
command. Once the Workers take the traffic, scale the Observability Pipelines Worker StatefulSets to zero, and delete
the objects that diff lists as replaced.`,
		Example:      example,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
	}
	cmd.AddCommand(newConvertCommand(streams), newDiffCommand(streams), newAdoptCommand(streams), newAbortCommand(streams))
	return cmd
}

// manifestOptions select the manifests of the objects to migrate.
type manifestOptions struct {
	genericiooptions.IOStreams
	configFlags       *genericclioptions.ConfigFlags
	cloudpremManifest string
	opwManifests      []string
}

func newManifestOptions(streams genericiooptions.IOStreams) manifestOptions {
	return manifestOptions{IOStreams: streams, configFlags: genericclioptions.NewConfigFlags(false)}
}

func (o *manifestOptions) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.cloudpremManifest, "cloudprem-manifest", "", "file with the objects that the CloudPrem chart rendered, as helm template or helm get manifest prints them")
	cmd.Flags().StringArrayVar(&o.opwManifests, "opw-manifest", nil, "file with the objects that an Observability Pipelines Worker chart rendered and the pipeline they become, as <file>=<pipeline>; repeat for each file, and omit it when there is none")
	_ = cmd.MarkFlagRequired("cloudprem-manifest")
	o.configFlags.AddFlags(cmd.Flags())
}

// loadManifests reads the CloudPrem manifest and the Observability Pipelines Worker manifests. Objects without a
// namespace are in the namespace of the command.
func (o *manifestOptions) loadManifests() (converter.Manifest, []converter.PipelineManifest, error) {
	pipelineFiles, err := parseOPWManifests(o.opwManifests)
	if err != nil {
		return converter.Manifest{}, nil, err
	}
	namespace, _, err := o.configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return converter.Manifest{}, nil, fmt.Errorf("resolve namespace: %w", err)
	}
	cloudprem, err := loadManifest(o.cloudpremManifest, namespace)
	if err != nil {
		return converter.Manifest{}, nil, err
	}
	pipelines := make([]converter.PipelineManifest, 0, len(pipelineFiles))
	for _, file := range pipelineFiles {
		manifest, loadErr := loadManifest(file.path, namespace)
		if loadErr != nil {
			return converter.Manifest{}, nil, loadErr
		}
		pipelines = append(pipelines, converter.PipelineManifest{Pipeline: file.pipeline, Manifest: manifest})
	}
	return cloudprem, pipelines, nil
}

// client returns a client of the Kubernetes context of the command, and the name of the context.
func (o *manifestOptions) client() (client.Client, string, error) {
	loader := o.configFlags.ToRawKubeConfigLoader()
	config, err := loader.RawConfig()
	if err != nil {
		return nil, "", fmt.Errorf("read kubeconfig: %w", err)
	}
	kubeContext := config.CurrentContext
	if o.configFlags.Context != nil && *o.configFlags.Context != "" {
		kubeContext = *o.configFlags.Context
	}
	c, err := common.NewClient(loader)
	if err != nil {
		return nil, "", err
	}
	return c, kubeContext, nil
}

// printFindings prints the findings and returns an error when one of them is a blocker.
func (o *manifestOptions) printFindings(findings []converter.Finding) error {
	blockers := 0
	for _, finding := range findings {
		fmt.Fprintln(o.ErrOut, finding)
		if finding.Severity == converter.SeverityBlocker {
			blockers++
		}
	}
	switch {
	case blockers == 1:
		return errors.New("1 blocker prevents the migration")
	case blockers > 1:
		return fmt.Errorf("%d blockers prevent the migration", blockers)
	}
	return nil
}

type migrationOptions struct {
	manifestOptions
	dryRun bool
	yes    bool
}

// newMigrationCommand completes cmd with the flags shared by the migration commands, and returns its options.
func newMigrationCommand(streams genericiooptions.IOStreams, cmd *cobra.Command, run func(context.Context, *migrationOptions) error) (*cobra.Command, *migrationOptions) {
	o := &migrationOptions{manifestOptions: newManifestOptions(streams)}
	cmd.SilenceUsage = true
	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		return run(ctx, o)
	}
	o.addFlags(cmd)
	cmd.Flags().BoolVarP(&o.yes, "yes", "y", false, "write without asking for a confirmation")
	return cmd, o
}

// migration returns the migration to the DatadogBYOCCluster with the key. It prints the Kubernetes context, and asks
// to confirm the action before the first write, unless --yes is set.
func (o *migrationOptions) migration(cluster types.NamespacedName, action string) (*migration.Migration, error) {
	c, kubeContext, err := o.client()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(o.Out, "Kubernetes context %q, DatadogBYOCCluster %s\n", kubeContext, cluster)
	m := &migration.Migration{Client: c, Cluster: cluster, Out: o.Out, DryRun: o.dryRun}
	if !o.yes {
		m.Confirm = func() error {
			fmt.Fprintf(o.Out, "%s in context %q? [y/N] ", action, kubeContext)
			answer, err := bufio.NewReader(o.In).ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return fmt.Errorf("read the confirmation: %w", err)
			}
			if answer = strings.TrimSpace(answer); !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
				return errors.New("not confirmed, so nothing was written; answer y, or set --yes")
			}
			return nil
		}
	}
	return m, nil
}

// readCluster reads a DatadogBYOCCluster manifest, and sets namespace on a cluster that does not set one.
func readCluster(path, namespace string) (*datadoghqv1alpha1.DatadogBYOCCluster, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
	if err := yaml.UnmarshalStrict(content, cluster); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if cluster.Spec.Datadog == nil || cluster.Spec.Components == nil {
		return nil, fmt.Errorf("%s has no spec.datadog or spec.components", path)
	}
	if cluster.Namespace == "" {
		cluster.Namespace = namespace
	}
	return cluster, nil
}

type opwManifest struct {
	path     string
	pipeline string
}

func parseOPWManifests(values []string) ([]opwManifest, error) {
	manifests := make([]opwManifest, 0, len(values))
	for _, value := range values {
		// The pipeline name has no "=", unlike the path.
		separator := strings.LastIndex(value, "=")
		if separator <= 0 || separator == len(value)-1 {
			return nil, fmt.Errorf("invalid --opw-manifest %q: expected <file>=<pipeline>", value)
		}
		manifests = append(manifests, opwManifest{path: value[:separator], pipeline: value[separator+1:]})
	}
	return manifests, nil
}

func loadManifest(path, namespace string) (converter.Manifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return converter.Manifest{}, err
	}
	manifest, err := converter.NewManifest(path, namespace, string(content))
	if err != nil {
		return converter.Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return manifest, nil
}
