// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/clusteragent"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/object/configmap"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// Dependencies is used to override any resource/dependency settings with a v2alpha1.DatadogAgentComponentOverride.
func Dependencies(logger logr.Logger, manager feature.ResourceManagers, dda *v2alpha1.DatadogAgent) (errs []error) {
	overrides := dda.Spec.Override
	namespace := dda.Namespace

	for component, override := range overrides {
		err := overrideRBAC(logger, manager, override, component, constants.GetServiceAccountByComponent(dda, component), namespace)
		if err != nil {
			errs = append(errs, err)
		}

		// Handle custom agent configurations (datadog.yaml, cluster-agent.yaml, etc.)
		errs = append(errs, overrideCustomConfigs(logger, manager, override.CustomConfigurations, component, dda.Name, namespace)...)

		// Handle custom check configurations
		confdCMName := fmt.Sprintf(extraConfdConfigMapName, strings.ToLower((string(component))))
		errs = append(errs, overrideExtraConfigs(logger, manager, override.ExtraConfd, namespace, confdCMName, true)...)

		// Handle custom check files
		checksdCMName := fmt.Sprintf(extraChecksdConfigMapName, strings.ToLower((string(component))))
		errs = append(errs, overrideExtraConfigs(logger, manager, override.ExtraChecksd, namespace, checksdCMName, false)...)

		errs = append(errs, overridePodDisruptionBudget(logger, manager, dda, override.CreatePodDisruptionBudget, component)...)
	}

	return errs
}

func overridePodDisruptionBudget(logger logr.Logger, manager feature.ResourceManagers, dda *v2alpha1.DatadogAgent, createPdb *bool, component v2alpha1.ComponentName) (errs []error) {
	if createPdb != nil && *createPdb {
		platformInfo := manager.Store().GetPlatformInfo()
		useV1BetaPDB := platformInfo.UseV1Beta1PDB()
		if component == v2alpha1.ClusterAgentComponentName {
			pdb := componentdca.GetClusterAgentPodDisruptionBudget(dda, useV1BetaPDB)
			if err := manager.Store().AddOrUpdate(kubernetes.PodDisruptionBudgetsKind, pdb); err != nil {
				errs = append(errs, err)
			}
		} else if component == v2alpha1.ClusterChecksRunnerComponentName &&
			(dda.Spec.Features.ClusterChecks.UseClusterChecksRunners == nil ||
				*dda.Spec.Features.ClusterChecks.UseClusterChecksRunners) {
			pdb := componentccr.GetClusterChecksRunnerPodDisruptionBudget(dda, useV1BetaPDB)
			if err := manager.Store().AddOrUpdate(kubernetes.PodDisruptionBudgetsKind, pdb); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

func overrideRBAC(logger logr.Logger, manager feature.ResourceManagers, override *v2alpha1.DatadogAgentComponentOverride, component v2alpha1.ComponentName, saName string, namespace string) error {
	var errs []error

	// Service account annotations
	if len(override.ServiceAccountAnnotations) > 0 {
		if err := manager.RBACManager().AddServiceAccountAnnotations(namespace, saName, override.ServiceAccountAnnotations); err != nil {
			errs = append(errs, err)
		}
	}

	// Delete created RBACs if CreateRbac is set to false
	if !createRBAC(override) {
		rbacManager := manager.RBACManager()
		logger.Info("Deleting RBACs for %s", component, nil)
		errs = append(errs, rbacManager.DeleteServiceAccountByComponent(string(component), namespace))
		errs = append(errs, rbacManager.DeleteRoleByComponent(string(component), namespace))
		errs = append(errs, rbacManager.DeleteClusterRoleByComponent(string(component)))
	}

	// Note: ServiceAccountName overrides are taken into account in the global dependencies code (out of pattern)

	return errors.NewAggregate(errs)
}

func overrideCustomConfigs(logger logr.Logger, manager feature.ResourceManagers, customConfigMap map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig, componentName v2alpha1.ComponentName, ddaName, namespace string) (errs []error) {
	for fileName, customConfig := range customConfigMap {
		// Favor ConfigMap setting; if it is specified, then move on
		if customConfig.ConfigMap != nil {
			continue
		} else if customConfig.ConfigData != nil {
			configMapName := fmt.Sprintf("%s-%s", getDefaultConfigMapName(ddaName, string(fileName)), strings.ToLower(string(componentName)))
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
			annotations := object.MergeAnnotationsLabels(logger, cm.GetAnnotations(), map[string]string{annotationKey: hash}, "*")
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
		annotations := object.MergeAnnotationsLabels(logger, cm.GetAnnotations(), map[string]string{annotationKey: hash}, "*")
		cm.SetAnnotations(annotations)

		if cm != nil {
			if err := manager.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

// createRBAC returns whether the RBAC should be created
func createRBAC(override *v2alpha1.DatadogAgentComponentOverride) bool {
	if override == nil || override.CreateRbac == nil {
		return true
	}
	return *override.CreateRbac
}
