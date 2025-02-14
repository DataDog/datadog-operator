// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/check-operator/root"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	flags := pflag.NewFlagSet("check-operator", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := root.NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
