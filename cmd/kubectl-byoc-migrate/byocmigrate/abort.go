// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package byocmigrate

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/converter"
)

func newAbortCommand(streams genericiooptions.IOStreams) *cobra.Command {
	cmd, o := newMigrationCommand(streams, &cobra.Command{
		Use:   "abort --cloudprem-manifest <file> --opw-manifest <file>=<pipeline>",
		Short: "Revert the migration to the objects of the manifests",
		Long: `Revert the migration to the objects of the manifests, as long as they exist.

First scale the StatefulSets of the Observability Pipelines Worker manifests back, and move their clients back to them:
abort checks that their Pods are ready, because deleting the DatadogBYOCCluster next deletes the Workers, and that the
user may patch the CloudPrem objects that the cluster owns, and writes nothing otherwise. abort then removes the cluster
from the owners of the CloudPrem objects, and checks that none of them references the cluster. It does not delete the
cluster: delete it afterwards, for example from the deployment pipeline that applies it, which deletes only the
resources that it created, such as the Workers. Do not delete the cluster before abort, because its deletion deletes the
CloudPrem objects that it owns. Apply the manifests again afterwards, for example with helm upgrade and the same values,
to restore the settings of the charts. With a server-side apply, as with Helm 4, force the conflicts: the controller
still manages the fields of the CloudPrem objects, so the apply fails with conflicts otherwise.`,
	}, func(ctx context.Context, opts *migrationOptions) error {
		cloudprem, pipelines, err := opts.loadManifests()
		if err != nil {
			return err
		}
		name, err := converter.ClusterName(cloudprem)
		if err != nil {
			return err
		}
		m, err := opts.migration(types.NamespacedName{Namespace: cloudprem.Namespace, Name: name}, "Remove the DatadogBYOCCluster from the owners of the CloudPrem objects")
		if err != nil {
			return err
		}
		return m.Abort(ctx, cloudprem, pipelines)
	})
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "run the checks and print the changes without writing anything")
	return cmd
}
