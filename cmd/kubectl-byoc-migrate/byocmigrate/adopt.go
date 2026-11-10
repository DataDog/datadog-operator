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

func newAdoptCommand(streams genericiooptions.IOStreams) *cobra.Command {
	cmd, _ := newMigrationCommand(streams, &cobra.Command{
		Use:   "adopt --cloudprem-manifest <file> --opw-manifest <file>=<pipeline>",
		Short: "Give the CloudPrem objects to the DatadogBYOCCluster",
		Long: `Give the objects of the CloudPrem manifest to the DatadogBYOCCluster, which must exist.

First stop applying the manifests, for example with helm upgrade or a deployment pipeline, then apply the cluster that
convert printed, for example from a deployment pipeline. Until adopt gives it the CloudPrem objects, the controller
writes only to the resources that the manifests do not have, and reports a conflict for the others. adopt checks that
every object of the CloudPrem manifest that the cluster adopts exists and has no other controller, and asks the API
server whether the user may patch it, as kubectl auth can-i does. It writes nothing when a check fails. It does not
compare the objects with the resources of the cluster: run diff right before adopt, and review what it prints. adopt
gives the objects to the cluster one by one, and does not change the cluster. The cluster rolls out each CloudPrem
workload once it has it, and creates the Workers next to the objects of the manifests once it has every CloudPrem
object.`,
	}, func(ctx context.Context, opts *migrationOptions) error {
		cloudprem, pipelines, err := opts.loadManifests()
		if err != nil {
			return err
		}
		name, err := converter.ClusterName(cloudprem)
		if err != nil {
			return err
		}
		m, err := opts.migration(types.NamespacedName{Namespace: cloudprem.Namespace, Name: name}, "Adopt the resources above")
		if err != nil {
			return err
		}
		plan, err := m.LivePlan(ctx, cloudprem, pipelines)
		if err != nil {
			return err
		}
		if err = opts.printFindings(plan.Findings()); err != nil {
			return err
		}
		return m.Adopt(ctx, plan)
	})
	return cmd
}
