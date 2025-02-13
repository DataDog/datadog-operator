// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-operator/cmd/helpers/secrets"
)

func main() {
	root := &cobra.Command{
		Use:   "helpers [subcommand] [flags]",
		Short: "Helpers that can be called inside the operator container",
	}

	root.AddCommand(secrets.NewCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
