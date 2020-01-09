// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	framework "github.com/operator-framework/operator-sdk/pkg/test"

	dynclient "sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"
)

// WaitForFuncOnDatadogAgent used to wait a valid condition on a DatadogAgent
func WaitForFuncOnDatadogAgent(t *testing.T, client framework.FrameworkClient, namespace, name string, f func(ad *datadoghqv1alpha1.DatadogAgent) (bool, error), retryInterval, timeout time.Duration) error {
	return wait.Poll(retryInterval, timeout, func() (bool, error) {
		objKey := dynclient.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}
		agentdeployment := &datadoghqv1alpha1.DatadogAgent{}
		err := client.Get(context.TODO(), objKey, agentdeployment)
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of %s DatadogAgent\n", name)
				return false, nil
			}
			return false, err
		}

		ok, err := f(agentdeployment)
		t.Logf("Waiting for condition function to be true ok for %s DatadogAgent (%t/%v)\n", name, ok, err)
		return ok, err
	})
}

// WaitForFuncOnExtendedDaemonSet used to wait a valid condition on a ExtendedDaemonSet
func WaitForFuncOnExtendedDaemonSet(t *testing.T, client framework.FrameworkClient, namespace, name string, f func(eds *edsdatadoghqv1alpha1.ExtendedDaemonSet) (bool, error), retryInterval, timeout time.Duration) error {
	return wait.Poll(retryInterval, timeout, func() (bool, error) {
		objKey := dynclient.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}
		extendeddaemonset := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
		err := client.Get(context.TODO(), objKey, extendeddaemonset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of %s ExtendedDaemonSet\n", name)
				return false, nil
			}
			return false, err
		}

		ok, err := f(extendeddaemonset)
		t.Logf("Waiting for condition function to be true ok for %s ExtendedDaemonSet (%t/%v)\n", name, ok, err)
		return ok, err
	})
}

// WaitForFuncOnDaemonSet used to wait a valid condition on a Daemonset
func WaitForFuncOnDaemonSet(t *testing.T, client framework.FrameworkClient, namespace, name string, f func(ds *appsv1.DaemonSet) (bool, error), retryInterval, timeout time.Duration) error {
	return wait.Poll(retryInterval, timeout, func() (bool, error) {
		objKey := dynclient.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}
		daemonset := &appsv1.DaemonSet{}
		err := client.Get(context.TODO(), objKey, daemonset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of %s DaemonSet\n", name)
				return false, nil
			}
			return false, err
		}

		ok, err := f(daemonset)
		t.Logf("Waiting for condition function to be true ok for %s DaemonSet (%t/%v)\n", name, ok, err)
		return ok, err
	})
}

// WaitForFuncOnClusterAgentDeployment used to wait a valid condition on a Cluster Agent Deployment
func WaitForFuncOnClusterAgentDeployment(t *testing.T, client framework.FrameworkClient, namespace, name string, f func(dca *appsv1.Deployment) (bool, error), retryInterval, timeout time.Duration) error {
	return wait.Poll(retryInterval, timeout, func() (bool, error) {
		objKey := dynclient.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}
		dca := &appsv1.Deployment{}
		err := client.Get(context.TODO(), objKey, dca)
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of %s Cluster Agent Deployment\n", name)
				return false, nil
			}
			return false, err
		}

		ok, err := f(dca)
		t.Logf("Waiting for condition function to be true ok for %s Cluster Agent Deployment (%t/%v)\n", name, ok, err)
		return ok, err
	})
}

// WaitForFuncOnPods used to wait a valid condition on list of pods
func WaitForFuncOnPods(t *testing.T, client framework.FrameworkClient, namespace string, labelSelector string, f func(pod *corev1.Pod) (bool, error), retryInterval, timeout time.Duration) error {
	return wait.Poll(retryInterval, timeout, func() (bool, error) {
		pods, err := FindPodsByLabels(t, client, namespace, labelSelector)
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for pods with label %s\n", labelSelector)
				return false, nil
			}
			return false, err
		}

		if len(pods.Items) == 0 {
			t.Logf("Waiting for pods with label %s\n", labelSelector)
			return false, nil
		}

		for _, pod := range pods.Items {
			ok, err := f(&pod)
			if !ok {
				t.Logf("Waiting for condition function to be true for Pod %s (%t/%v)\n", pod.ObjectMeta.Name, ok, err)
				return false, err
			}
		}
		t.Logf("Condition satisfied for all pods with label %s\n", labelSelector)
		return true, nil
	})
}

// ExecValidationFunc checks results of a command execution in a running pod
type ExecValidationFunc func(stdout, stderr string, returnErr error) (bool, error)

// WaitForExecInPod is used to wait for a condition based on execution of a command in a running pod
func WaitForExecInPod(t *testing.T, f *framework.Framework, namespace string, pod string, container string, command []string, validate ExecValidationFunc, retryInterval, timeout time.Duration) error {
	return wait.Poll(retryInterval, timeout, func() (bool, error) {
		stdout, stderr, err := ExecInPod(t, f, namespace, pod, container, command)

		ok, err := validate(stdout, stderr, err)
		t.Logf("Waiting for condition function to be true for command execution (%s) in Pod %s (%t/%v)\n",
			strings.Join(command, " "),
			pod,
			ok,
			err,
		)
		return true, nil
	})
}
