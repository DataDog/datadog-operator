// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

func init() {
	if err := feature.Register(feature.AppsecIDType, buildAppsecFeature); err != nil {
		panic(err)
	}
}

func buildAppsecFeature(options *feature.Options) feature.Feature {
	appSecFeat := &appsecFeature{
		rbacSuffix: common.ClusterAgentSuffix,
	}

	if options != nil {
		appSecFeat.logger = options.Logger.WithValues("feature", "appsec")
	}

	return appSecFeat
}

type appsecFeature struct {
	config             Config
	owner              metav1.Object
	serviceAccountName string
	rbacSuffix         string

	logger logr.Logger
}

// ID returns the ID of the Feature
func (f *appsecFeature) ID() feature.IDType {
	return feature.AppsecIDType
}

func isAboveMinVersion(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	// Agent version must >= 7.73.0 to run appsec feature
	image := images.AgentLatestVersion
	if clusterAgent, ok := ddaSpec.Override[v2alpha1.ClusterAgentComponentName]; ok {
		if clusterAgent.Image != nil {
			image = common.GetAgentVersionFromImage(*clusterAgent.Image)
		}
	}
	return utils.IsAboveMinVersion(image, ClusterAgentMinVersion, nil)
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *appsecFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddaSpecRC *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	var err error
	f.config, err = FromAnnotations(dda.GetAnnotations())
	if err != nil {
		f.logger.Error(err, "failed to parse annotations")
		return feature.RequiredComponents{}
	}

	if err := f.config.validate(); err != nil {
		f.logger.Error(err, "failed to validate annotations")
		return feature.RequiredComponents{}
	}

	if !isAboveMinVersion(ddaSpec) {
		f.logger.V(1).Info("agent version is too low")
		return feature.RequiredComponents{}
	}

	if !f.config.isEnabled() {
		f.logger.V(1).Info("feature is disabled")
		return feature.RequiredComponents{}
	}

	f.owner = dda
	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

	// The cluster agent is required for the AppSec feature.
	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.ClusterAgentContainerName,
			},
		},
	}
}

// ManageDependencies adds the RBAC necessary for the appsec feature to be enabled and is still required when disabled
// to be able to do cleanup
func (f *appsecFeature) ManageDependencies(managers feature.ResourceManagers, _ string) error {
	rbacName := getAppsecRBACResourceName(f.owner, f.rbacSuffix)
	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *appsecFeature) ManageClusterAgent(managers feature.PodTemplateManagers, _ string) error {
	if !f.config.isEnabled() {
		f.logger.V(2).Info("feature is disabled, adding no environment variables")
		return nil
	}

	addEnvVar := func(key, value string) error {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  key,
			Value: value,
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return fmt.Errorf("adding env var %s to the cluster-agent returned an error: %w", key, err)
		}

		return nil
	}

	if err := addEnvVar(DDAppsecProxyEnabled, "true"); err != nil {
		return err
	}

	if err := addEnvVar(DDClusterAgentAppsecInjectorEnabled, "true"); err != nil {
		return err
	}

	// Set auto-detect if explicitly specified (default is true in cluster-agent if not set)
	if f.config.AutoDetect != nil {
		if err := addEnvVar(DDAppsecProxyAutoDetect, apiutils.BoolToString(f.config.AutoDetect)); err != nil {
			return err
		}
	}

	// Set proxies list if specified
	if len(f.config.Proxies) > 0 {
		proxiesJSON, err := json.Marshal(f.config.Proxies)
		if err != nil {
			return fmt.Errorf("could not marshal AppSec proxies list to JSON: %w", err)
		}
		if err := addEnvVar(DDAppsecProxyProxies, string(proxiesJSON)); err != nil {
			return err
		}
	}

	// Set processor port if specified
	if f.config.ProcessorPort != 0 {
		if err := addEnvVar(DDAppsecProxyProcessorPort, strconv.Itoa(f.config.ProcessorPort)); err != nil {
			return err
		}
	}

	// Set processor address if specified
	if f.config.ProcessorAddress != "" {
		if err := addEnvVar(DDAppsecProxyProcessorAddress, f.config.ProcessorAddress); err != nil {
			return err
		}
	}

	// Set processor service name if specified
	if f.config.ProcessorServiceName != "" {
		if err := addEnvVar(DDClusterAgentAppsecInjectorProcessorServiceName, f.config.ProcessorServiceName); err != nil {
			return err
		}
	}

	// Set processor service namespace if specified
	if f.config.ProcessorServiceNamespace != "" {
		if err := addEnvVar(DDClusterAgentAppsecInjectorProcessorServiceNamespace, f.config.ProcessorServiceNamespace); err != nil {
			return err
		}
	}

	return nil
}

func (f *appsecFeature) ManageSingleContainerNodeAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func (f *appsecFeature) ManageNodeAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func (f *appsecFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func (f *appsecFeature) ManageOtelAgentGateway(_ feature.PodTemplateManagers, _ string) error {
	return nil
}
