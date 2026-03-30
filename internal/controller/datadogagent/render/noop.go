// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package render

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

// noopForwarders implements datadog.MetricsForwardersManager as a no-op.
// Used for offline rendering where no metrics forwarding is needed.
type noopForwarders struct{}

var _ datadog.MetricsForwardersManager = noopForwarders{}

func (noopForwarders) Register(client.Object)                     {}
func (noopForwarders) Unregister(client.Object)                   {}
func (noopForwarders) ProcessError(client.Object, error)          {}
func (noopForwarders) ProcessEvent(client.Object, datadog.Event)  {}
func (noopForwarders) SetEnabledFeatures(client.Object, []string) {}
func (noopForwarders) MetricsForwarderStatusForObj(client.Object) *datadog.ConditionCommon {
	return nil
}
