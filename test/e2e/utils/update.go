// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	goctx "context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	framework "github.com/operator-framework/operator-sdk/pkg/test"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
)

// UpdateDatadogAgentFunc used to update a DatadogAgent with retry and timeout policy
func UpdateDatadogAgentFunc(f *framework.Framework, namespace, name string, updateFunc func(ad *datadoghqv1alpha1.DatadogAgent), retryInterval, timeout time.Duration) error {
	return wait.Poll(retryInterval, timeout, func() (bool, error) {
		ad := &datadoghqv1alpha1.DatadogAgent{}
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, ad); err != nil {
			return false, nil
		}

		updateAd := ad.DeepCopy()
		updateFunc(updateAd)
		if err := f.Client.Update(goctx.TODO(), updateAd); err != nil {
			return false, err
		}
		return true, nil
	})
}
