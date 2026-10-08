// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"cmp"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// operatorContainerName is the name of the operator container in the
// official manifests and chart.
const operatorContainerName = "manager"

// buildOperator finds the operator Deployment, preferring the DDA namespace,
// and its leader Lease. It returns nil when no operator was found.
func buildOperator(s *Snapshot, ddaNamespace string) *OperatorView {
	obj := pickOperator(s.Objects[SourceOperator], ddaNamespace)
	if obj == nil {
		return nil
	}
	var dep appsv1.Deployment
	if err := fromUnstructured(obj, &dep); err != nil {
		return nil
	}

	op := &OperatorView{
		Namespace: dep.Namespace,
		Name:      dep.Name,
		Ready:     dep.Status.ReadyReplicas,
		Desired:   deploymentDesired(&dep),
	}
	containers := dep.Spec.Template.Spec.Containers
	for i, c := range containers {
		if c.Name == operatorContainerName || i == len(containers)-1 {
			op.Image = c.Image
			op.Version = imageTag(c.Image)
			break
		}
	}

	for _, obj := range s.Objects[SourceLease] {
		if obj.GetName() != OperatorLeaseName || obj.GetNamespace() != dep.Namespace {
			continue
		}
		var lease coordinationv1.Lease
		if err := fromUnstructured(obj, &lease); err != nil {
			break
		}
		if lease.Spec.HolderIdentity != nil {
			op.LeaseHolder = *lease.Spec.HolderIdentity
		}
		if lease.Spec.RenewTime != nil {
			op.LeaseRenew = timePtr(lease.Spec.RenewTime.Time)
		}
	}
	return op
}

// pickOperator returns the operator Deployment to show: the first one in the
// DDA namespace, otherwise the first one by namespace and name. It returns
// nil when there is none.
func pickOperator(deps []*unstructured.Unstructured, ddaNamespace string) *unstructured.Unstructured {
	if len(deps) == 0 {
		return nil
	}
	return slices.MinFunc(deps, func(a, b *unstructured.Unstructured) int {
		if c := cmp.Compare(boolRank(a.GetNamespace() != ddaNamespace), boolRank(b.GetNamespace() != ddaNamespace)); c != 0 {
			return c
		}
		if c := cmp.Compare(a.GetNamespace(), b.GetNamespace()); c != 0 {
			return c
		}
		return cmp.Compare(a.GetName(), b.GetName())
	})
}

// imageTag returns the tag of an image reference, without any digest, or
// "" when it has none.
func imageTag(image string) string {
	image, _, _ = strings.Cut(image, "@")
	slash := strings.LastIndex(image, "/")
	if colon := strings.LastIndex(image, ":"); colon > slash {
		return image[colon+1:]
	}
	return ""
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}
