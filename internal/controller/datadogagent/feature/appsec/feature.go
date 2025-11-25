// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
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
	err := feature.Register(feature.AppsecIDType, buildAppsecFeature)
	if err != nil {
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
	enabled               bool
	autoDetect            *bool
	proxies               []string
	processorAddress      *string
	processorPort         *int32
	processorServiceName  *string
	processorServiceNs    *string
	owner                 metav1.Object
	serviceAccountName    string
	rbacSuffix            string
}

// ID returns the ID of the Feature
func (f *appsecFeature) ID() feature.IDType {
	return feature.AppsecIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *appsecFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	f.owner = dda

	appSec := ddaSpec.Features.Appsec
	if appSec == nil || appSec.Injector == nil || !apiutils.BoolValue(appSec.Injector.Enabled) {
		return feature.RequiredComponents{}
	}

	f.enabled = true
	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)
	f.autoDetect = appSec.Injector.AutoDetect
	f.proxies = appSec.Injector.Proxies

	// Process processor configuration
	if appSec.Injector.Processor != nil {
		processor := appSec.Injector.Processor
		f.processorAddress = processor.Address
		f.processorPort = processor.Port

		if processor.Service != nil {
			f.processorServiceName = processor.Service.Name
			f.processorServiceNs = processor.Service.Namespace
		}
	}

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
func (f *appsecFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	if !f.enabled {
		return nil
	}

	// Manage RBAC permissions
	rbacName := GetAppsecRBACResourceName(f.owner, f.rbacSuffix)
	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *appsecFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
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
