// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hako/durafmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IntToString converts int32 into string.
func IntToString(i int32) string {
	return strconv.Itoa(int(i))
}

// GetDuration gets uptime duration of a resource.
func GetDuration(obj *metav1.ObjectMeta) string {
	return durafmt.ParseShort(time.Since(obj.CreationTimestamp.Time)).String()
}

// isPodNotReady returns whether the pod is ready, returns the reason if not ready.
func isPodNotReady(pod *corev1.Pod) (bool, string) {
	if pod.Status.Phase != corev1.PodRunning {
		return true, pod.Status.Reason
	}

	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
			return true, cond.Reason
		}
	}

	return false, ""
}

// containersInfo returns containers readiness and restart details.
func containersInfo(pod *corev1.Pod) (string, string, string) {
	notreadyContainers := []string{}
	containersCount := len(pod.Status.ContainerStatuses)
	notreadyCount := 0
	restartCount := int32(0)
	for _, ctr := range pod.Status.ContainerStatuses {
		restartCount += ctr.RestartCount
		if !ctr.Ready {
			notreadyCount++
			notreadyContainers = append(notreadyContainers, ctr.Name)
		}
	}

	return fmt.Sprintf("%d/%d", containersCount-notreadyCount, containersCount), strings.Join(notreadyContainers, ", "), IntToString(restartCount)
}

// getNodeReadiness returns whether a node is ready.
func getNodeReadiness(c client.Client, nodename string) string {
	isNodeReady := func(node *corev1.Node) bool {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				return true
			}
		}

		return false
	}

	node := &corev1.Node{}
	readiness := "Unknown"
	if err := c.Get(context.TODO(), client.ObjectKey{Name: nodename}, node); err == nil {
		readiness = strconv.FormatBool(isNodeReady(node))
	}

	return readiness
}
