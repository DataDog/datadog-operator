// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/version"
)

func init() {
	err := feature.Register(feature.DefaultIDType, buildDefaultFeature)
	if err != nil {
		panic(err)
	}
}

func buildDefaultFeature(options *feature.Options) feature.Feature {
	dF := &defaultFeature{}

	if options != nil {
		dF.logger = options.Logger
	}

	return dF
}

type defaultFeature struct {
	owner metav1.Object

	logger     logr.Logger
	adpEnabled bool
}

// ID returns the ID of the Feature
func (f *defaultFeature) ID() feature.IDType {
	return feature.DefaultIDType
}

func (f *defaultFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	trueValue := true
	f.owner = dda

	if dda.ObjectMeta.Annotations != nil {
		f.adpEnabled = featureutils.HasAgentDataPlaneAnnotation(dda)
	}

	agentContainers := make([]apicommon.AgentContainerName, 0)

	// If Agent Data Plane is enabled, add the ADP container to the list of required containers for the Agent feature.
	if f.adpEnabled {
		agentContainers = append(agentContainers, apicommon.AgentDataPlaneContainerName)
	}

	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: &trueValue,
			Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
		},
		Agent: feature.RequiredComponent{
			IsRequired: &trueValue,
			Containers: agentContainers,
		},
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *defaultFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	var errs []error
	// Create install-info configmap
	installInfoCM := buildInstallInfoConfigMap(f.owner)
	if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, installInfoCM); err != nil {
		return err
	}

	if components.Agent.IsEnabled() {
		if err := f.agentDependencies(managers, components.Agent); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.NewAggregate(errs)
}

func (f *defaultFeature) agentDependencies(managers feature.ResourceManagers, requiredComponent feature.RequiredComponent) error {
	var errs []error

	// Create a configmap for the default seccomp profile in the System Probe.
	// This is mounted in the init-volume container in the agent default code.
	for _, containerName := range requiredComponent.Containers {
		if containerName == apicommon.SystemProbeContainerName {
			errs = append(errs, managers.ConfigMapManager().AddConfigMap(
				common.GetDefaultSeccompConfigMapName(f.owner),
				f.owner.GetNamespace(),
				DefaultSeccompConfigDataForSystemProbe(),
			))
		}
	}

	return errors.NewAggregate(errs)
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.ManageNodeAgent(managers, provider)

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	if f.adpEnabled {
		// When ADP is enabled, we signal this to the Core Agent by setting an environment variable.
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  common.DDADPEnabled,
			Value: "true",
		})
	}

	if f.adpEnabled {
		// When ADP is enabled, we signal this to the Core Agent by setting an environment variable.
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  common.DDADPEnabled,
			Value: "true",
		})
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}

func buildInstallInfoConfigMap(dda metav1.Object) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.GetInstallInfoConfigMapName(dda),
			Namespace: dda.GetNamespace(),
		},
		Data: map[string]string{
			"install_info": getInstallInfoValue(),
		},
	}

	return configMap
}

func getInstallInfoValue() string {
	toolVersion := "unknown"
	if envVar := os.Getenv(InstallInfoToolVersion); envVar != "" {
		toolVersion = envVar
	}

	return fmt.Sprintf(installInfoDataTmpl, toolVersion, version.Version)
}

const installInfoDataTmpl = `---
install_method:
  tool: datadog-operator
  tool_version: %s
  installer_version: %s
`
