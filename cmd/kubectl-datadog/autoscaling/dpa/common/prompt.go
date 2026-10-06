// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
)

// isTerminal is a variable for tests.
var isTerminal = func(r io.Reader) bool { return printers.IsTerminal(r) }

var errNoTerminal = errors.New("confirmation required but stdin is not a terminal: re-run with --yes")

// confirm asks a yes/no question on a terminal. It returns ctx.Err() when ctx
// is cancelled first: the signal handler disables the default exit on Ctrl-C.
func confirm(ctx context.Context, streams genericclioptions.IOStreams, question string) (bool, error) {
	fmt.Fprintf(streams.Out, "%s (y/N): ", question)
	answer := make(chan string, 1)
	go func() {
		var response string
		// An empty answer is a decline.
		_, _ = fmt.Fscanln(streams.In, &response)
		answer <- response
	}()
	select {
	case <-ctx.Done():
		// The reader goroutine stays blocked on stdin until the process exits.
		fmt.Fprintln(streams.Out)
		return false, ctx.Err()
	case response := <-answer:
		response = strings.ToLower(strings.TrimSpace(response))
		return response == "y" || response == "yes", nil
	}
}
