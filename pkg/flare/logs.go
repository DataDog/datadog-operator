// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"

	corev1 "k8s.io/api/core/v1"
)

const (
	// DefaultLogTailLines caps the lines collected per container log.
	DefaultLogTailLines int64 = 10000
	// DefaultLogLimitBytes caps the bytes collected per container log.
	DefaultLogLimitBytes int64 = 10 * 1024 * 1024
	// DefaultLogHeadLines caps the lines collected from the start of a
	// container log, when the tail doesn't hold the whole log.
	DefaultLogHeadLines int64 = 2000
	// DefaultLogHeadBytes caps the bytes collected from the start of a
	// container log.
	DefaultLogHeadBytes int64 = 1024 * 1024
)

// CollectPodLogs writes the logs of every container of a pod, and of the
// previous container after a restart, to dir/<pod>/<container>[.previous].log.
// When the tail is truncated, the start of the log (the startup lines) is also
// written to <container>[.previous].head.log. Only use it for Operator pods:
// the Agent and Cluster Agent flares collect their own logs.
func CollectPodLogs(ctx context.Context, c *Collection, dir string, pod *corev1.Pod) {
	if c.Env.Clientset == nil {
		c.Warn(fmt.Errorf("unable to collect the logs of %s: no Kubernetes clientset", pod.Name))
		return
	}
	restarts := map[string]int32{}
	for _, status := range pod.Status.ContainerStatuses {
		restarts[status.Name] = status.RestartCount
	}
	for _, status := range pod.Status.InitContainerStatuses {
		restarts[status.Name] = status.RestartCount
	}

	containers := append([]corev1.Container{}, pod.Spec.InitContainers...)
	containers = append(containers, pod.Spec.Containers...)
	for _, container := range containers {
		base := path.Join(dir, safeName(pod.Name), safeName(container.Name))
		collectContainerLogs(ctx, c, pod, container.Name, false, base)
		if restarts[container.Name] > 0 {
			collectContainerLogs(ctx, c, pod, container.Name, true, base+".previous")
		}
	}
}

// collectContainerLogs writes the tail of a container log to base.log and,
// when the tail is truncated, its head to base.head.log.
func collectContainerLogs(ctx context.Context, c *Collection, pod *corev1.Pod, container string, previous bool, base string) {
	truncated, err := collectContainerLog(ctx, c, pod, container, previous, false, base+".log")
	c.Warn(err)
	if err == nil && truncated {
		_, err = collectContainerLog(ctx, c, pod, container, previous, true, base+".head.log")
		c.Warn(err)
	}
}

// collectContainerLog writes the tail of a container log, or its first lines
// when head is true. It returns true when the tail may not hold the whole log.
func collectContainerLog(ctx context.Context, c *Collection, pod *corev1.Pod, container string, previous, head bool, name string) (bool, error) {
	lines, limitBytes, what := c.Env.LogTailLines, c.Env.LogLimitBytes, "the logs"
	if lines <= 0 {
		lines = DefaultLogTailLines
	}
	if limitBytes <= 0 {
		limitBytes = DefaultLogLimitBytes
	}
	opts := &corev1.PodLogOptions{Container: container, Previous: previous, TailLines: &lines}
	if head {
		lines, limitBytes, what = c.Env.LogHeadLines, c.Env.LogHeadBytes, "the first lines of the logs"
		if lines <= 0 {
			lines = DefaultLogHeadLines
		}
		if limitBytes <= 0 {
			limitBytes = DefaultLogHeadBytes
		}
		opts.TailLines = nil
	}
	limitBytes = min(limitBytes, c.Remaining())
	if limitBytes <= 0 {
		return false, fmt.Errorf("%w: %s of %s/%s were not collected", ErrSizeLimit, what, pod.Name, container)
	}
	opts.LimitBytes = &limitBytes

	// Canceling the context when done stops the API server from sending the
	// rest of the log.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := c.Env.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts).Stream(ctx)
	if err != nil {
		return false, fmt.Errorf("unable to get %s of %s/%s (previous: %t): %w", what, pod.Name, container, previous, err)
	}
	defer stream.Close()

	var data bytes.Buffer
	reader := bufio.NewReader(io.LimitReader(stream, limitBytes))
	for n := int64(0); !head || n < lines; n++ {
		line, err := reader.ReadBytes('\n')
		data.Write(line)
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, fmt.Errorf("unable to read %s of %s/%s: %w", what, pod.Name, container, err)
		}
	}
	truncated := int64(data.Len()) >= limitBytes || int64(bytes.Count(data.Bytes(), []byte{'\n'})) >= lines
	return truncated, c.AddText(name, data.Bytes())
}

// CollectOperatorPods writes a summary of the Operator pods to
// operator/pods.yaml and their logs to operator/logs.
func CollectOperatorPods(ctx context.Context, c *Collection, pods []corev1.Pod) error {
	summaries := make([]PodSummary, 0, len(pods))
	for i := range pods {
		summaries = append(summaries, SummarizePod(&pods[i]))
		CollectPodLogs(ctx, c, "operator/logs", &pods[i])
	}
	return c.AddYAML("operator/pods.yaml", summaries)
}
