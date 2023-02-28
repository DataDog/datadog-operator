// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"github.com/go-logr/logr"

	securityv1 "github.com/openshift/api/security/v1"
	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	ddacomponent "github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// Dependencies is used to override any resource/dependency settings with a v2alpha1.DatadogAgentComponentOverride.
func Dependencies(logger logr.Logger, manager feature.ResourceManagers, dda *v2alpha1.DatadogAgent) (errs []error) {
	overrides := dda.Spec.Override
	namespace := dda.Namespace

	for component, override := range overrides {
		err := overrideRBAC(logger, manager, override, component, namespace)
		if err != nil {
			errs = append(errs, err)
		}

		// Handle custom agent configurations (datadog.yaml, cluster-agent.yaml, etc.)
		errs = append(errs, overrideCustomConfigs(logger, manager, override.CustomConfigurations, dda.Name, namespace)...)

		// Handle custom check configurations
		errs = append(errs, overrideExtraConfigs(logger, manager, override.ExtraConfd, namespace, v2alpha1.ExtraConfdConfigMapName, true)...)

		// Handle custom check files
		errs = append(errs, overrideExtraConfigs(logger, manager, override.ExtraChecksd, namespace, v2alpha1.ExtraChecksdConfigMapName, false)...)

		// Handle scc
		errs = append(errs, overrideSCC(manager, dda)...)
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

func overrideCustomConfigs(logger logr.Logger, manager feature.ResourceManagers, customConfigMap map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig, ddaName, namespace string) (errs []error) {
	for fileName, customConfig := range customConfigMap {
		// Favor ConfigMap setting; if it is specified, then move on
		if customConfig.ConfigMap != nil {
			continue
		} else if customConfig.ConfigData != nil {
			configMapName := getDefaultConfigMapName(ddaName, string(fileName))
			cm, err := configmap.BuildConfigMapConfigData(namespace, customConfig.ConfigData, configMapName, string(fileName))
			if err != nil {
				errs = append(errs, err)
			}

			// Add md5 hash annotation for custom config
			hash, err := comparison.GenerateMD5ForSpec(customConfig)
			if err != nil {
				logger.Error(err, "couldn't generate hash for custom config", "filename", fileName)
			}
			annotationKey := object.GetChecksumAnnotationKey(string(fileName))
			annotations := object.MergeAnnotationsLabels(logger, cm.GetAnnotations(), map[string]string{annotationKey: hash}, "")
			cm.SetAnnotations(annotations)

			if cm != nil {
				if err := manager.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errs
}

func overrideExtraConfigs(logger logr.Logger, manager feature.ResourceManagers, multiCustomConfig *v2alpha1.MultiCustomConfig, namespace, configMapName string, isYaml bool) (errs []error) {
	if multiCustomConfig != nil && multiCustomConfig.ConfigMap == nil && len(multiCustomConfig.ConfigDataMap) > 0 {
		cm, err := configmap.BuildConfigMapMulti(namespace, multiCustomConfig.ConfigDataMap, configMapName, isYaml)
		if err != nil {
			errs = append(errs, err)
		}

		// Add md5 hash annotation for custom config
		hash, err := comparison.GenerateMD5ForSpec(multiCustomConfig)
		if err != nil {
			logger.Error(err, "couldn't generate hash for extra custom config")
		}
		annotationKey := object.GetChecksumAnnotationKey(configMapName)
		annotations := object.MergeAnnotationsLabels(logger, cm.GetAnnotations(), map[string]string{annotationKey: hash}, "")
		cm.SetAnnotations(annotations)

		if cm != nil {
			if err := manager.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

func overrideSCC(manager feature.ResourceManagers, dda *v2alpha1.DatadogAgent) (errs []error) {
	for component, override := range dda.Spec.Override {
		sccConfig := override.SecurityContextConstraints
		if sccConfig != nil && apiutils.BoolValue(sccConfig.Create) {
			var sccName string
			scc := &securityv1.SecurityContextConstraints{}

			switch component {
			case v2alpha1.NodeAgentComponentName:
				sccName = ddacomponent.GetAgentSCCName(dda)
				scc = agent.GetDefaultSCC(dda)
			case v2alpha1.ClusterAgentComponentName:
				sccName = ddacomponent.GetClusterAgentSCCName(dda)
				scc = clusteragent.GetDefaultSCC(dda)
			}

			if sccConfig.CustomConfiguration != nil {
				scc = sccConfig.CustomConfiguration
			}

			errs = append(errs, manager.PodSecurityManager().AddSecurityContextConstraints(sccName, dda.Namespace, scc))
		}
	}

	return errs
}
