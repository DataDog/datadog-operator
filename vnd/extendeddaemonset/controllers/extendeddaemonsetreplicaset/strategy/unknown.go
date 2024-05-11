// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package strategy

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	podutils "github.com/DataDog/extendeddaemonset/pkg/controller/utils/pod"
)

// ManageUnknown use to manage ReplicaSet with unknown status.
func ManageUnknown(client client.Client, params *Parameters) (*Result, error) {
	result := &Result{}
	// remove canary node if define
	for _, nodeName := range params.CanaryNodes {
		delete(params.PodByNodeName, params.NodeByName[nodeName])
	}
	now := time.Now()
	metaNow := metav1.NewTime(now)
	var desiredPods, currentPods, availablePods, readyPods, nbIgnoredUnresponsiveNodes int32

	for node, pod := range params.PodByNodeName {
		desiredPods++
		if pod != nil {
			if compareCurrentPodWithNewPod(params, pod, node) {
				if podutils.HasPodSchedulerIssue(pod) {
					nbIgnoredUnresponsiveNodes++

					continue
				}

				currentPods++
				if podutils.IsPodAvailable(pod, 0, metaNow) {
					availablePods++
				}
				if podutils.IsPodReady(pod) {
					readyPods++
				}
			}
		}
	}

	result.NewStatus = params.NewStatus.DeepCopy()
	result.NewStatus.Status = string(ReplicaSetStatusUnknown)
	result.NewStatus.Desired = 0
	result.NewStatus.Ready = readyPods
	result.NewStatus.Current = currentPods
	result.NewStatus.Available = availablePods
	result.NewStatus.IgnoredUnresponsiveNodes = nbIgnoredUnresponsiveNodes
	params.Logger.V(1).Info("Status:", "Desired", result.NewStatus.Desired, "Ready", readyPods, "Available", availablePods)

	if result.NewStatus.Desired != result.NewStatus.Ready {
		result.Result.Requeue = true
	}
	result.Result.RequeueAfter = time.Second

	return result, nil
}
