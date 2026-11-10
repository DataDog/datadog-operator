// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
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
)

// CollectPodLogs writes the logs of every container of a pod to
// dir/<pod>/<container>.log, and the logs of the previous container to
// <container>.previous.log when it restarted. Logs are tailed to
// Env.LogTailLines and Env.LogLimitBytes. Only use it for Operator pods:
// the Agent and Cluster Agent collect their own logs in their flares.
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
		c.Warn(collectContainerLog(ctx, c, pod, container.Name, false, base+".log"))
		if restarts[container.Name] > 0 {
			c.Warn(collectContainerLog(ctx, c, pod, container.Name, true, base+".previous.log"))
		}
	}
}

func collectContainerLog(ctx context.Context, c *Collection, pod *corev1.Pod, container string, previous bool, name string) error {
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
		return fmt.Errorf("%w: the logs of %s/%s were not collected", ErrSizeLimit, pod.Name, container)
	}

	stream, err := c.Env.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container:  container,
		Previous:   previous,
		TailLines:  &tailLines,
		LimitBytes: &limitBytes,
		Timestamps: false,
	}).Stream(ctx)
	if err != nil {
		return fmt.Errorf("unable to get the logs of %s/%s (previous: %t): %w", pod.Name, container, previous, err)
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, limitBytes))
	if err != nil {
		return fmt.Errorf("unable to read the logs of %s/%s: %w", pod.Name, container, err)
	}
	return c.AddText(name, data)
}
