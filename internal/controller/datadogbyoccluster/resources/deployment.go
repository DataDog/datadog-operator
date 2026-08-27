// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploymentResources contains the resources managed for a deployment component.
type DeploymentResources struct {
	Service             *corev1.Service
	Deployment          *appsv1.Deployment
	PodDisruptionBudget *policyv1.PodDisruptionBudget
}

// Objects returns the component resources in apply order.
func (r *DeploymentResources) Objects() []client.Object {
	if r == nil {
		return nil
	}
	objects := []client.Object{r.Service, r.Deployment}
	if r.PodDisruptionBudget != nil {
		objects = append(objects, r.PodDisruptionBudget)
	}
	return objects
}

// deploymentBuilder builds the resources for a deployment component.
type deploymentBuilder struct {
	workload workloadInput
	defaults deploymentDefaults
}

type deploymentDefaults struct {
	Strategy appsv1.DeploymentStrategy
}

type deploymentValues struct {
	Workload workloadValues
	Strategy appsv1.DeploymentStrategy
}

func newDeploymentBuilder(workload workloadInput, defaults deploymentDefaults) deploymentBuilder {
	return deploymentBuilder{workload: workload, defaults: defaults}
}

func (b deploymentBuilder) values() (serviceValues, deploymentValues, error) {
	workload, err := resolveWorkloadValues(b.workload)
	if err != nil {
		return serviceValues{}, deploymentValues{}, err
	}
	return workload.Service, deploymentValues{
		Workload: workload,
		Strategy: b.defaults.Strategy,
	}, nil
}

func (b deploymentBuilder) build() (*DeploymentResources, error) {
	service, deployment, err := b.values()
	if err != nil {
		return nil, err
	}
	podDisruptionBudget, err := newPodDisruptionBudgetBuilder(
		deployment.Workload.Metadata,
		deployment.Workload.Selector,
		b.workload.Cluster.Spec.Global.PodDisruptionBudget,
		b.workload.Spec.PodDisruptionBudget,
	).build()
	if err != nil {
		return nil, err
	}
	return &DeploymentResources{
		Service:             createService(service),
		Deployment:          createDeployment(deployment),
		PodDisruptionBudget: podDisruptionBudget,
	}, nil
}

func createDeployment(values deploymentValues) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: *values.Workload.Metadata.DeepCopy(),
		Spec: appsv1.DeploymentSpec{
			Replicas: new(values.Workload.Replicas),
			Selector: &metav1.LabelSelector{MatchLabels: maps.Clone(values.Workload.Selector)},
			Template: *values.Workload.Template.DeepCopy(),
			Strategy: values.Strategy,
		},
	}
}
