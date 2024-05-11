// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

type condFn func() bool

func withGet(nsName types.NamespacedName, obj client.Object, desc string, condition condFn) condFn {
	return func() bool {
		err := k8sClient.Get(context.Background(), nsName, obj)
		if err != nil {
			fmt.Fprintf(GinkgoWriter, "Failed to get %s [%s/%s]: %v", desc, nsName.Namespace, nsName.Name, err)

			return false
		}

		return condition()
	}
}

func withList(listOptions []client.ListOption, obj client.ObjectList, desc string, condition condFn) condFn {
	return func() bool {
		err := k8sClient.List(context.Background(), obj, listOptions...)
		if err != nil {
			fmt.Fprintf(GinkgoWriter, "Failed to list %s: %v", desc, err)

			return false
		}

		return condition()
	}
}

func withEDS(nsName types.NamespacedName, eds *datadoghqv1alpha1.ExtendedDaemonSet, condition condFn) condFn {
	return withGet(nsName, eds, "EDS", condition)
}

func withERS(nsName types.NamespacedName, ers *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, condition condFn) condFn {
	return withGet(nsName, ers, "ERS", condition)
}
