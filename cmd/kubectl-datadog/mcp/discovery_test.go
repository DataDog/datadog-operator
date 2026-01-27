// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestNewClusterAgentDiscovery(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(scheme)

	c := ctrlruntimefake.NewClientBuilder().WithScheme(scheme).Build()
	clientset := fake.NewSimpleClientset()
	discoveryClient := clientset.Discovery()

	d := NewClusterAgentDiscovery(c, clientset, discoveryClient, "test-namespace")

	if d == nil {
		t.Fatal("Expected discovery to be created")
	}

	if d.namespace != "test-namespace" {
		t.Errorf("Expected namespace 'test-namespace', got %s", d.namespace)
	}
}

func TestClusterAgentDiscovery_GetClusterAgentPod(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a running cluster-agent pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-cluster-agent-12345",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"agent.datadoghq.com/component": "cluster-agent",
				"agent.datadoghq.com/name":      "datadog-agent",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(pod)
	c := ctrlruntimefake.NewClientBuilder().WithScheme(scheme).Build()
	discoveryClient := clientset.Discovery()

	d := NewClusterAgentDiscovery(c, clientset, discoveryClient, "test-namespace")

	foundPod, err := d.GetClusterAgentPod("datadog-agent")
	if err != nil {
		t.Fatalf("Expected to find pod, got error: %v", err)
	}

	if foundPod.Name != "datadog-cluster-agent-12345" {
		t.Errorf("Expected pod name 'datadog-cluster-agent-12345', got %s", foundPod.Name)
	}
}

func TestClusterAgentDiscovery_GetClusterAgentPod_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	clientset := fake.NewSimpleClientset()
	c := ctrlruntimefake.NewClientBuilder().WithScheme(scheme).Build()
	discoveryClient := clientset.Discovery()

	d := NewClusterAgentDiscovery(c, clientset, discoveryClient, "test-namespace")

	_, err := d.GetClusterAgentPod("datadog-agent")
	if err == nil {
		t.Fatal("Expected error when no pods found")
	}
}

func TestClusterAgentDiscovery_SelectDatadogAgent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(scheme)

	// Create a DatadogAgent with cluster-agent enabled
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent",
			Namespace: "test-namespace",
		},
		Status: v2alpha1.DatadogAgentStatus{
			ClusterAgent: &v2alpha1.DeploymentStatus{},
		},
	}

	c := ctrlruntimefake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dda).
		Build()

	clientset := fake.NewSimpleClientset()
	discoveryClient := clientset.Discovery()

	d := NewClusterAgentDiscovery(c, clientset, discoveryClient, "test-namespace")

	// Test with explicit name
	name, err := d.SelectDatadogAgent("datadog-agent")
	if err != nil {
		t.Fatalf("Expected to select agent, got error: %v", err)
	}

	if name != "datadog-agent" {
		t.Errorf("Expected name 'datadog-agent', got %s", name)
	}
}

func TestClusterAgentDiscovery_SelectDatadogAgent_AutoSelect(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(scheme)

	// Create a DatadogAgent with cluster-agent enabled
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent",
			Namespace: "test-namespace",
		},
		Status: v2alpha1.DatadogAgentStatus{
			ClusterAgent: &v2alpha1.DeploymentStatus{},
		},
	}

	c := ctrlruntimefake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dda).
		Build()

	clientset := fake.NewSimpleClientset()
	discoveryClient := clientset.Discovery()

	d := NewClusterAgentDiscovery(c, clientset, discoveryClient, "test-namespace")

	// Test auto-select (empty name)
	name, err := d.SelectDatadogAgent("")
	if err != nil {
		t.Fatalf("Expected to auto-select agent, got error: %v", err)
	}

	if name != "datadog-agent" {
		t.Errorf("Expected name 'datadog-agent', got %s", name)
	}
}

func TestClusterAgentDiscovery_hasClusterAgentEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(scheme)

	c := ctrlruntimefake.NewClientBuilder().WithScheme(scheme).Build()
	clientset := fake.NewSimpleClientset()
	discoveryClient := clientset.Discovery()

	d := NewClusterAgentDiscovery(c, clientset, discoveryClient, "test-namespace")

	// Test with cluster-agent in status
	ddaWithStatus := &v2alpha1.DatadogAgent{
		Status: v2alpha1.DatadogAgentStatus{
			ClusterAgent: &v2alpha1.DeploymentStatus{},
		},
	}

	if !d.hasClusterAgentEnabled(ddaWithStatus) {
		t.Error("Expected cluster-agent to be enabled when present in status")
	}

	// Test with default (no explicit disable)
	ddaDefault := &v2alpha1.DatadogAgent{}

	if !d.hasClusterAgentEnabled(ddaDefault) {
		t.Error("Expected cluster-agent to be enabled by default")
	}

	// Test with explicitly disabled
	disabled := true
	ddaDisabled := &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.ClusterAgentComponentName: {
					Disabled: &disabled,
				},
			},
		},
	}

	if d.hasClusterAgentEnabled(ddaDisabled) {
		t.Error("Expected cluster-agent to be disabled when explicitly disabled")
	}
}

func createFakeDiscoveryWithLeaseSupport(supported bool) discovery.DiscoveryInterface {
	fakeDiscovery := &discoveryfake.FakeDiscovery{Fake: &clienttesting.Fake{}}

	groups := &metav1.APIGroupList{
		Groups: []metav1.APIGroup{},
	}

	if supported {
		groups.Groups = append(groups.Groups, metav1.APIGroup{
			Name: "coordination.k8s.io",
			Versions: []metav1.GroupVersionForDiscovery{
				{
					GroupVersion: "coordination.k8s.io/v1",
					Version:      "v1",
				},
			},
		})
	}

	fakeDiscovery.Fake.AddReactor("get", "group", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, groups, nil
	})

	return fakeDiscovery
}
