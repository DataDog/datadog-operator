// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

// Dependencies is used to override any resource/dependency settings with a v2alpha1.DatadogAgentComponentOverride.
func Dependencies(logger logr.Logger, manager feature.ResourceManagers, overrides map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride, namespace string) (errs []error) {
	for component, override := range overrides {
		err := overrideComponentDependencies(logger, manager, override, component, namespace)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func overrideComponentDependencies(logger logr.Logger, manager feature.ResourceManagers, override *v2alpha1.DatadogAgentComponentOverride, component v2alpha1.ComponentName, namespace string) error {
	var errs []error
	if override.CreateRbac != nil && !*override.CreateRbac {
		rbacManager := manager.RBACManager()
		logger.Info("Deleting RBACs for %s", component)
		errs = append(errs, rbacManager.DeleteServiceAccountByComponent(string(component), namespace))
		errs = append(errs, rbacManager.DeleteRoleByComponent(string(component), namespace))
		errs = append(errs, rbacManager.DeleteClusterRoleByComponent(string(component)))
	}
	return errors.NewAggregate(errs)
}
