// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"cmp"
	"encoding/json"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
	"github.com/DataDog/datadog-operator/pkg/constants"
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
	return appSecFeat
}

type appsecFeature struct {
	enabled              bool
	autoDetect           *bool
	proxies              []string
	processorAddress     *string
	processorPort        *int32
	processorServiceName *string
	processorServiceNs   *string
	owner                metav1.Object
	serviceAccountName   string
	rbacSuffix           string
}

// ID returns the ID of the Feature
func (f *appsecFeature) ID() feature.IDType {
	return feature.AppsecIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *appsecFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddaSpecRC *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	mergeConfigs(ddaSpec, ddaSpecRC)

	appsec := ddaSpec.Features.Appsec
	if !apiutils.BoolValue(appsec.Injector.Enabled) || (!apiutils.BoolValue(appsec.Injector.AutoDetect) && len(appsec.Injector.Proxies) == 0) {
		return feature.RequiredComponents{}
	}

	f.owner = dda
	f.enabled = true
	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)
	f.autoDetect = appsec.Injector.AutoDetect
	f.proxies = appsec.Injector.Proxies

	// Process processor configuration
	f.processorAddress = appsec.Injector.Processor.Address
	f.processorPort = appsec.Injector.Processor.Port
	f.processorServiceName = appsec.Injector.Processor.Service.Name
	f.processorServiceNs = appsec.Injector.Processor.Service.Namespace

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

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *appsecFeature) ManageDependencies(managers feature.ResourceManagers, _ string) error {
	if !f.enabled {
		return nil
	}

	// Manage RBAC permissions
	rbacName := getAppsecRBACResourceName(f.owner, f.rbacSuffix)
	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *appsecFeature) ManageClusterAgent(managers feature.PodTemplateManagers, _ string) error {
	if !f.enabled {
		return nil
	}

	// Always set the base enabled flags
	if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDAppsecProxyEnabled,
		Value: "true",
	}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
		return err
	}

	if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDClusterAgentAppsecInjectorEnabled,
		Value: "true",
	}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
		return err
	}

	// Set auto-detect if explicitly specified (default is true in cluster-agent if not set)
	if f.autoDetect != nil {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDAppsecProxyAutoDetect,
			Value: apiutils.BoolToString(f.autoDetect),
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	// Set proxies list if specified
	if len(f.proxies) > 0 {
		proxiesJSON, err := json.Marshal(f.proxies)
		if err != nil {
			return err
		}
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDAppsecProxyProxies,
			Value: string(proxiesJSON),
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	// Set processor port if specified
	if f.processorPort != nil {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDAppsecProxyProcessorPort,
			Value: strconv.Itoa(int(*f.processorPort)),
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	// Set processor address if specified
	if f.processorAddress != nil {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDAppsecProxyProcessorAddress,
			Value: *f.processorAddress,
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	// Set processor service name if specified
	if f.processorServiceName != nil {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDClusterAgentAppsecInjectorProcessorServiceName,
			Value: *f.processorServiceName,
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	// Set processor service namespace if specified
	if f.processorServiceNs != nil {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDClusterAgentAppsecInjectorProcessorServiceNamespace,
			Value: *f.processorServiceNs,
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
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

func mergeConfigs(ddaSpec *v2alpha1.DatadogAgentSpec, ddaRCStatus *v2alpha1.RemoteConfigConfiguration) {
	if ddaRCStatus == nil || ddaRCStatus.Features == nil || ddaRCStatus.Features.Appsec == nil || ddaRCStatus.Features.Appsec.Injector == nil || ddaRCStatus.Features.Appsec.Injector.Enabled == nil {
		return
	}

	// Fill up empty nested structs to avoid nil pointer dereference
	ddaRCStatus.Features = cmp.Or(ddaRCStatus.Features, &v2alpha1.DatadogFeatures{})
	ddaRCStatus.Features.Appsec = cmp.Or(ddaRCStatus.Features.Appsec, &v2alpha1.AppsecFeatureConfig{})
	ddaRCStatus.Features.Appsec.Injector = cmp.Or(ddaRCStatus.Features.Appsec.Injector, &v2alpha1.AppsecInjectorConfig{})
	ddaRCStatus.Features.Appsec.Injector.Processor = cmp.Or(ddaRCStatus.Features.Appsec.Injector.Processor, &v2alpha1.AppsecProcessorConfig{})
	ddaRCStatus.Features.Appsec.Injector.Processor.Service = cmp.Or(ddaRCStatus.Features.Appsec.Injector.Processor.Service, &v2alpha1.AppsecProcessorServiceConfig{})

	ddaSpec.Features = cmp.Or(ddaSpec.Features, &v2alpha1.DatadogFeatures{})
	ddaSpec.Features.Appsec = cmp.Or(ddaSpec.Features.Appsec, &v2alpha1.AppsecFeatureConfig{})
	ddaSpec.Features.Appsec.Injector = cmp.Or(ddaSpec.Features.Appsec.Injector, &v2alpha1.AppsecInjectorConfig{})
	ddaSpec.Features.Appsec.Injector.Processor = cmp.Or(ddaSpec.Features.Appsec.Injector.Processor, &v2alpha1.AppsecProcessorConfig{})
	ddaSpec.Features.Appsec.Injector.Processor.Service = cmp.Or(ddaSpec.Features.Appsec.Injector.Processor.Service, &v2alpha1.AppsecProcessorServiceConfig{})

	// Merge AppSec feature configuration from Remote Config status into DDA spec
	ddaSpec.Features.Appsec.Injector.Enabled = cmp.Or(ddaSpec.Features.Appsec.Injector.Enabled, ddaRCStatus.Features.Appsec.Injector.Enabled)
	ddaSpec.Features.Appsec.Injector.AutoDetect = cmp.Or(ddaSpec.Features.Appsec.Injector.AutoDetect, ddaRCStatus.Features.Appsec.Injector.AutoDetect)
	ddaSpec.Features.Appsec.Injector.Processor.Address = cmp.Or(ddaSpec.Features.Appsec.Injector.Processor.Address, ddaRCStatus.Features.Appsec.Injector.Processor.Address)
	ddaSpec.Features.Appsec.Injector.Processor.Port = cmp.Or(ddaSpec.Features.Appsec.Injector.Processor.Port, ddaRCStatus.Features.Appsec.Injector.Processor.Port)
	ddaSpec.Features.Appsec.Injector.Processor.Service.Name = cmp.Or(ddaSpec.Features.Appsec.Injector.Processor.Service.Name, ddaRCStatus.Features.Appsec.Injector.Processor.Service.Name)
	ddaSpec.Features.Appsec.Injector.Processor.Service.Namespace = cmp.Or(ddaSpec.Features.Appsec.Injector.Processor.Service.Namespace, ddaRCStatus.Features.Appsec.Injector.Processor.Service.Namespace)

	if len(ddaSpec.Features.Appsec.Injector.Proxies) == 0 && len(ddaRCStatus.Features.Appsec.Injector.Proxies) > 0 {
		ddaSpec.Features.Appsec.Injector.Proxies = ddaRCStatus.Features.Appsec.Injector.Proxies
	}
}
