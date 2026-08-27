// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"fmt"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// EnabledSetterFunc writes a feature's enabled flag. Each feature registers its
// own so the flag written stays next to the Configure that reads it.
type EnabledSetterFunc func(ddaSpec *v2alpha1.DatadogAgentSpec, enabled bool)

// enabledSetters is filled in by each feature's init.
var enabledSetters = map[IDType]EnabledSetterFunc{}

// RegisterEnabledSetter registers the enabled flag setter for a feature.
func RegisterEnabledSetter(id IDType, setter EnabledSetterFunc) error {
	if _, found := enabledSetters[id]; found {
		return fmt.Errorf("the enabled setter %s is registered already", id)
	}
	enabledSetters[id] = setter
	return nil
}

// SetEnabled sets feature id's enabled flag on ddaSpec. It fails if the feature
// registered no setter.
func SetEnabled(ddaSpec *v2alpha1.DatadogAgentSpec, id IDType, enabled bool) error {
	setter, found := enabledSetters[id]
	if !found {
		return fmt.Errorf("no enabled setter is registered for %s", id)
	}
	if ddaSpec.Features == nil {
		ddaSpec.Features = &v2alpha1.DatadogFeatures{}
	}
	setter(ddaSpec, enabled)
	return nil
}
