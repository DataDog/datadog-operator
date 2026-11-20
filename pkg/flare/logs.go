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

// CollectPodLogs writes the logs of every container of a pod to
// dir/<pod>/<container>.log, and the logs of the previous container to
// <container>.previous.log when it restarted. Logs are tailed to
// Env.LogTailLines and Env.LogLimitBytes.
//
// When the tail doesn't hold the whole log, the start of the log is also
// written to <container>.head.log (and <container>.previous.head.log), up
// to Env.LogHeadLines and Env.LogHeadBytes, so that the startup lines (the
// version, the configuration, the features enabled) are in the flare even
// when the Operator has run for a long time.
//
// The Kubernetes API only serves the logs of the current and the previous
// instance of a container, from the log file the kubelet is writing. Files
// the kubelet rotated (containerLogMaxSize, containerLogMaxFiles) stay on
// the node and can't be read through the API, so the head is the oldest
// part of the current file: it starts at the container start unless the
// kubelet already rotated the file.
//
// Only use it for Operator pods: the Agent and Cluster Agent collect their
// own logs in their flares.
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
	truncated, err := collectContainerLog(ctx, c, pod, container, previous, base+".log")
	c.Warn(err)
	if err == nil && truncated {
		c.Warn(collectContainerLogHead(ctx, c, pod, container, previous, base+".head.log"))
	}
}

// collectContainerLog writes the tail of a container log. It returns true
// when the tail may not hold the whole log.
func collectContainerLog(ctx context.Context, c *Collection, pod *corev1.Pod, container string, previous bool, name string) (bool, error) {
	tailLines := c.Env.LogTailLines
	if tailLines <= 0 {
		tailLines = DefaultLogTailLines
	}
	limitBytes := c.Env.LogLimitBytes
	if limitBytes <= 0 {
		limitBytes = DefaultLogLimitBytes
	}
	limitBytes = min(limitBytes, c.Remaining())
	if limitBytes <= 0 {
		return false, fmt.Errorf("%w: the logs of %s/%s were not collected", ErrSizeLimit, pod.Name, container)
	}

	stream, err := c.Env.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container:  container,
		Previous:   previous,
		TailLines:  &tailLines,
		LimitBytes: &limitBytes,
		Timestamps: false,
	}).Stream(ctx)
	if err != nil {
		return false, fmt.Errorf("unable to get the logs of %s/%s (previous: %t): %w", pod.Name, container, previous, err)
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, limitBytes))
	if err != nil {
		return false, fmt.Errorf("unable to read the logs of %s/%s: %w", pod.Name, container, err)
	}
	truncated := int64(len(data)) >= limitBytes || int64(bytes.Count(data, []byte{'\n'})) >= tailLines
	return truncated, c.AddText(name, data)
}

// collectContainerLogHead writes the first lines of a container log. It
// streams the log from its start and stops after Env.LogHeadLines lines or
// Env.LogHeadBytes bytes.
func collectContainerLogHead(ctx context.Context, c *Collection, pod *corev1.Pod, container string, previous bool, name string) error {
	headLines := c.Env.LogHeadLines
	if headLines <= 0 {
		headLines = DefaultLogHeadLines
	}
	limitBytes := c.Env.LogHeadBytes
	if limitBytes <= 0 {
		limitBytes = DefaultLogHeadBytes
	}
	limitBytes = min(limitBytes, c.Remaining())
	if limitBytes <= 0 {
		return fmt.Errorf("%w: the first lines of the logs of %s/%s were not collected", ErrSizeLimit, pod.Name, container)
	}

	ctx, cancel := context.WithCancel(ctx)
	// Closing the stream early stops the API server from sending the rest of
	// the log.
	defer cancel()
	stream, err := c.Env.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container:  container,
		Previous:   previous,
		LimitBytes: &limitBytes,
		Timestamps: false,
	}).Stream(ctx)
	if err != nil {
		return fmt.Errorf("unable to get the first lines of the logs of %s/%s (previous: %t): %w", pod.Name, container, previous, err)
	}
	defer stream.Close()

	var head bytes.Buffer
	reader := bufio.NewReader(io.LimitReader(stream, limitBytes))
	for lines := int64(0); lines < headLines; lines++ {
		line, err := reader.ReadBytes('\n')
		head.Write(line)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("unable to read the first lines of the logs of %s/%s: %w", pod.Name, container, err)
		}
	}
	return c.AddText(name, head.Bytes())
}
