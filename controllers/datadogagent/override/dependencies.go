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
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// Dependencies is used to override any resource/dependency settings with a v2alpha1.DatadogAgentComponentOverride.
func Dependencies(logger logr.Logger, manager feature.ResourceManagers, overrides map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride, namespace string) (errs []error) {
	for component, override := range overrides {
		err := overrideRBAC(logger, manager, override, component, namespace)
		if err != nil {
			errs = append(errs, err)
		}

		// Handle custom check configurations
		errs = append(errs, overrideExtraConfigs(manager, override.ExtraConfd, namespace, v2alpha1.ExtraConfdConfigMapName, true)...)

		// Handle custom check files
		errs = append(errs, overrideExtraConfigs(manager, override.ExtraChecksd, namespace, v2alpha1.ExtraChecksdConfigMapName, false)...)
	}
	return errs
}

func overrideRBAC(logger logr.Logger, manager feature.ResourceManagers, override *v2alpha1.DatadogAgentComponentOverride, component v2alpha1.ComponentName, namespace string) error {
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

func overrideExtraConfigs(manager feature.ResourceManagers, multiCustomConfig *v2alpha1.MultiCustomConfig, namespace, configMapName string, isYaml bool) (errs []error) {
	if multiCustomConfig != nil && multiCustomConfig.ConfigMap == nil && len(multiCustomConfig.ConfigDataMap) > 0 {
		cm, err := configmap.BuildConfigMapMulti(namespace, multiCustomConfig.ConfigDataMap, configMapName, isYaml)
		if err != nil {
			errs = append(errs, err)
		}
		if cm != nil {
			if err := manager.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}
