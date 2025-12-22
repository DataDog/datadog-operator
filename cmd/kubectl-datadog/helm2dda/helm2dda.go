// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm2dda

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/mapper"
)

// New provides a cobra command wrapping options for "helm2dda" sub command
// It returns an instance of the cmd/yaml-mapper/cmd with command name and usage overrides.
func New(streams genericiooptions.IOStreams) *cobra.Command {
	newCmd := mapper.NewCmdMap(streams)

	usageExample := `
kubectl datadog helm2dda --sourcePath=example_source.yaml
`

	newCmd.Use = "helm2dda [DatadogAgent name] --sourcePath <path> [flags]"
	newCmd.Short = "Map Datadog Helm values to DatadogAgent CRD schema"
	newCmd.Example = usageExample
	newCmd.SilenceUsage = true

	return newCmd
}
