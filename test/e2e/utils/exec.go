// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"bytes"
	"strings"
	"testing"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecInPod executes a command in a running ready pod and returns its stdout and stderr outputs
func ExecInPod(t *testing.T, f *framework.Framework, namespace string, pod string, container string, command []string) (string, string, error) {
	restClient := f.KubeClient.CoreV1().RESTClient()

	req := restClient.Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		Param("container", container)

	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   command,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	exec, err := remotecommand.NewSPDYExecutor(f.KubeConfig, "POST", req.URL())
	if err != nil {
		return "", "", err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err != nil {
		return "", "", err
	}

	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), nil
}
