// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateFromObject performs an update and forces previous ResourceVersion to be set
func UpdateFromObject(ctx context.Context, c client.Client, newObject client.Object, oldMeta metav1.ObjectMeta) error {
	newObject.SetResourceVersion(oldMeta.ResourceVersion)
	return c.Update(ctx, newObject)
}
