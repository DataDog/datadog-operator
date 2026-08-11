// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// serviceConvergenceKey returns a canonical digest of every Service's ports and
// annotations. The renderer compares it across passes to detect when shared
// resources (the local Service, fed by per-DDAI port claims) have stabilized.
func serviceConvergenceKey(ctx context.Context, c client.Client) (string, error) {
	svcList := &corev1.ServiceList{}
	if err := c.List(ctx, svcList); err != nil {
		return "", err
	}
	parts := make([]string, 0, len(svcList.Items))
	for i := range svcList.Items {
		svc := &svcList.Items[i]
		var b strings.Builder
		fmt.Fprintf(&b, "%s/%s|", svc.Namespace, svc.Name)
		for _, p := range svc.Spec.Ports {
			fmt.Fprintf(&b, "%s:%d/%d/%s;", p.Name, p.Port, p.TargetPort.IntValue(), p.Protocol)
		}
		b.WriteString("|")
		annKeys := make([]string, 0, len(svc.Annotations))
		for k := range svc.Annotations {
			annKeys = append(annKeys, k)
		}
		sort.Strings(annKeys)
		for _, k := range annKeys {
			fmt.Fprintf(&b, "%s=%s;", k, svc.Annotations[k])
		}
		parts = append(parts, b.String())
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n"), nil
}

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

// appendInputStatuses re-fetches the DDA and DAPs from the fake client (which
// holds their reconciler-updated status) and appends them to the resource list.
func appendInputStatuses(ctx context.Context, c client.Client, resources []client.Object, opts Options) ([]client.Object, error) {
	dda := &datadoghqv2alpha1.DatadogAgent{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(opts.DDA), dda); err != nil {
		return nil, fmt.Errorf("re-fetching DDA for status: %w", err)
	}
	resources = append(resources, dda)

	for _, dap := range opts.DAPs {
		fetched := &datadoghqv1alpha1.DatadogAgentProfile{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(dap), fetched); err != nil {
			return nil, fmt.Errorf("re-fetching DAP %s for status: %w", dap.Name, err)
		}
		resources = append(resources, fetched)
	}
	return resources, nil
}
