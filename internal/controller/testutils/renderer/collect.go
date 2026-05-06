// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// collectResources lists every resource type the operator creates from the fake client.
func collectResources(
	ctx context.Context,
	c client.Client,
	platformInfo kubernetes.PlatformInfo,
	supportCilium bool,
) ([]client.Object, error) {
	var all []client.Object

	// ── Dependency resources (managed by the store) ──────────────────────────
	for _, kind := range platformInfo.GetAgentResourcesKind(supportCilium) {
		list := kubernetes.ObjectListFromKind(kind, platformInfo)
		if list == nil {
			continue
		}
		if err := c.List(ctx, list); err != nil {
			return nil, err
		}
		items, err := apimeta.ExtractList(list)
		if err != nil {
			return nil, err
		}
		for _, obj := range items {
			co, ok := obj.(client.Object)
			if !ok {
				continue
			}
			all = append(all, co)
		}
	}

	// ── Workloads ─────────────────────────────────────────────────────────────
	dsList := &appsv1.DaemonSetList{}
	if err := c.List(ctx, dsList); err != nil {
		return nil, err
	}
	for i := range dsList.Items {
		all = append(all, &dsList.Items[i])
	}

	deployList := &appsv1.DeploymentList{}
	if err := c.List(ctx, deployList); err != nil {
		return nil, err
	}
	for i := range deployList.Items {
		all = append(all, &deployList.Items[i])
	}

	// ── DatadogAgentInternal CRs ─────────────────────────────────────────────
	ddaiList := &datadoghqv1alpha1.DatadogAgentInternalList{}
	if err := c.List(ctx, ddaiList); err != nil {
		return nil, err
	}
	for i := range ddaiList.Items {
		all = append(all, &ddaiList.Items[i])
	}

	return all, nil
}
