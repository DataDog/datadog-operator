// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// PrintCanaryPods prints the list of canary pods.
func PrintCanaryPods(c client.Client, ns, edsName string, out io.Writer) error {
	eds := &v1alpha1.ExtendedDaemonSet{}
	err := c.Get(context.TODO(), client.ObjectKey{Namespace: ns, Name: edsName}, eds)
	if err != nil && errors.IsNotFound(err) {
		return fmt.Errorf("ExtendedDaemonSet %s/%s not found", ns, edsName)
	} else if err != nil {
		return fmt.Errorf("unable to get ExtendedDaemonSet, err: %w", err)
	}

	if eds.Status.Canary == nil {
		return fmt.Errorf("the ExtendedDaemonset is not currently running a canary replicaset")
	}

	req, err := labels.NewRequirement(v1alpha1.ExtendedDaemonSetReplicaSetNameLabelKey, selection.Equals, []string{eds.Status.Canary.ReplicaSet})
	if err != nil {
		return fmt.Errorf("couldn't query canary pods: %w", err)
	}

	rsSelector := labels.NewSelector().Add(*req)

	return printPods(c, rsSelector, out, false)
}

// PrintNotReadyPods prints the list of not ready pods.
func PrintNotReadyPods(c client.Client, ns, edsName string, out io.Writer) error {
	eds := &v1alpha1.ExtendedDaemonSet{}
	err := c.Get(context.TODO(), client.ObjectKey{Namespace: ns, Name: edsName}, eds)
	if err != nil && errors.IsNotFound(err) {
		return fmt.Errorf("ExtendedDaemonSet %s/%s not found", ns, edsName)
	} else if err != nil {
		return fmt.Errorf("unable to get ExtendedDaemonSet, err: %w", err)
	}

	req, err := labels.NewRequirement(v1alpha1.ExtendedDaemonSetNameLabelKey, selection.Equals, []string{edsName})
	if err != nil {
		return fmt.Errorf("couldn't query daemon pods: %w", err)
	}

	edsSelector := labels.NewSelector().Add(*req)

	return printPods(c, edsSelector, out, true)
}

func printPods(c client.Client, selector labels.Selector, out io.Writer, notReadyOnly bool) error {
	podList := &corev1.PodList{}
	err := c.List(context.TODO(), podList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("couldn't get pods: %w", err)
	}

	table := newPodsTable(out)
	for _, pod := range podList.Items {
		podNotready, reason := isPodNotReady(&pod)
		if notReadyOnly && !podNotready {
			continue
		}
		ready, containers, restarts := containersInfo(&pod)
		table.Append([]string{pod.Name, ready, string(pod.Status.Phase), reason, containers, restarts, pod.Spec.NodeName, getNodeReadiness(c, pod.Spec.NodeName), GetDuration(&pod.ObjectMeta)})
	}

	table.Render()

	return nil
}
