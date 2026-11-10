// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package byocmigrate

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/converter"
	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/migration"
)

func newDiffCommand(streams genericiooptions.IOStreams) *cobra.Command {
	o := newManifestOptions(streams)
	var fromFile string
	cmd := &cobra.Command{
		Use:   "diff --cloudprem-manifest <file> --opw-manifest <file>=<pipeline> [--from-file <cluster.yaml>]",
		Short: "Print the changes that the migration makes to the objects of the manifests",
		Long: `Print the changes that the migration makes to the objects of the manifests, and the findings.

diff renders the resources of the DatadogBYOCCluster, and compares the live objects with the objects after the
migration, which the API server computes with dry-run requests, so diff writes nothing but needs the permissions of
adopt. Before the cluster is applied, pass it with --from-file, as convert prints it or after editing it. Once it is
applied, omit --from-file: diff renders the live cluster, which adopt gives the objects to. It prints a unified diff
from the live CloudPrem objects to the objects that the cluster adopts, and from the live objects of the manifests to
the resources that replace them, such as the read-only metastore and the Workers, and lists the objects of the
manifests that the cluster replaces and the objects that the migration leaves as is. The findings list what prevents
the migration, and diff fails on blockers. Run it right before adopt: adopt does not compare the objects again.`,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cloudprem, pipelines, err := o.loadManifests()
			if err != nil {
				return err
			}
			var file *datadoghqv1alpha1.DatadogBYOCCluster
			if fromFile != "" {
				if file, err = readCluster(fromFile, cloudprem.Namespace); err != nil {
					return err
				}
			}
			name, err := converter.ClusterName(cloudprem)
			if err != nil {
				return err
			}
			c, kubeContext, err := o.client()
			if err != nil {
				return err
			}
			key := types.NamespacedName{Namespace: cloudprem.Namespace, Name: name}
			fmt.Fprintf(o.ErrOut, "Kubernetes context %q, DatadogBYOCCluster %s\n", kubeContext, key)
			m := &migration.Migration{Client: c, Cluster: key, Out: o.ErrOut}
			diff, findings, err := m.Diff(cmd.Context(), cloudprem, pipelines, file)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(o.Out, diff); err != nil {
				return err
			}
			return o.printFindings(findings)
		},
	}
	o.addFlags(cmd)
	cmd.Flags().StringVar(&fromFile, "from-file", "", "DatadogBYOCCluster manifest printed by convert, edited or not, before the cluster is applied")
	return cmd
}
