// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// kubectl-byoc-migrate migrates the objects of the CloudPrem and Observability Pipelines Worker Helm charts to a
// DatadogBYOCCluster.
// It is a separate kubectl plugin from kubectl-datadog, so that it can be removed once the migrations are done.
package main

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-byoc-migrate", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := byocmigrate.New(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
