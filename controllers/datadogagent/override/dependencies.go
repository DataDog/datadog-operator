// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

// Dependencies is used to override any resource/dependency settings with a v2alpha1.DatadogAgentComponentOverride.
func Dependencies(manager feature.ResourceManagers, overrides map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride, namespace string) (errs []error) {
	for component, override := range overrides {
		err := overrideComponentDependencies(manager, override, component, namespace)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func overrideComponentDependencies(manager feature.ResourceManagers, override *v2alpha1.DatadogAgentComponentOverride, component v2alpha1.ComponentName, namespace string) error {
	var errs []error
	if override.CreateRbac != nil && !*override.CreateRbac {
		rbacManager := manager.RBACManager()
		errs = append(errs, rbacManager.DeleteServiceAccountByComponent(string(component), namespace))
		errs = append(errs, rbacManager.DeleteRoleByComponent(string(component), namespace))
		errs = append(errs, rbacManager.DeleteClusterRoleByComponent(string(component), namespace))
	}
	return errors.NewAggregate(errs)
}
