// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package byocmigrate

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/converter"
)

func newConvertCommand(streams genericiooptions.IOStreams) *cobra.Command {
	o := newManifestOptions(streams)
	cmd := &cobra.Command{
		Use:   "convert --cloudprem-manifest <file> --opw-manifest <file>=<pipeline>",
		Short: "Convert the objects of the manifests to a DatadogBYOCCluster",
		Long: `Convert the objects of the manifests to a DatadogBYOCCluster, and print it.

convert prints the findings: the blockers that prevent the migration, which make the command fail, and what needs
attention that diff does not show. diff shows the settings of the charts that the cluster loses or changes. Without
Observability Pipelines Worker objects, omit --opw-manifest and add a pipeline to the converted cluster. convert reads
nothing from the API server.`,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cloudprem, pipelines, err := o.loadManifests()
			if err != nil {
				return err
			}
			result, err := converter.Convert(cloudprem, pipelines)
			if err != nil {
				return err
			}
			output, err := converter.MarshalCluster(result.Cluster)
			if err != nil {
				return err
			}
			if _, err := o.Out.Write(output); err != nil {
				return err
			}
			return o.printFindings(result.Findings)
		},
	}
	o.addFlags(cmd)
	return cmd
}
