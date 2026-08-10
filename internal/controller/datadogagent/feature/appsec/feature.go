// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
	appsecFeat := &appsecFeature{
		rbacSuffix: common.ClusterAgentSuffix,
	}

	if options != nil {
		appsecFeat.logger = options.Logger.WithValues("feature", "appsec")
	}

	return appsecFeat
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

func clusterAgentVersion(ddaSpec *v2alpha1.DatadogAgentSpec) string {
	if ddaSpec == nil {
		return images.AgentLatestVersion
	}
	if clusterAgent, ok := ddaSpec.Override[v2alpha1.ClusterAgentComponentName]; ok {
		if clusterAgent.Image != nil {
			return common.GetAgentVersionFromImage(*clusterAgent.Image)
		}
	}
	return images.AgentLatestVersion
}

func isAboveMinVersion(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return utils.IsAboveMinVersion(clusterAgentVersion(ddaSpec), ClusterAgentMinVersion, nil)
}

// appsecAnnotationPrefix is the shared prefix of every AppSec annotation in const.go.
const appsecAnnotationPrefix = "agent.datadoghq.com/appsec."

// hasAppsecAnnotations reports whether any AppSec annotation is present, whatever its
// value. It is a local prefix scan on purpose: pkg/plugin/common.IsAnnotated would drag
// the kubectl plugin and its dependencies into the controller.
func hasAppsecAnnotations(annotations map[string]string) bool {
	for key := range annotations {
		if strings.HasPrefix(key, appsecAnnotationPrefix) {
			return true
		}
	}
	return false
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *appsecFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddaSpecRC *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	// Warn before anything else, including every early return below, so that users who
	// disabled the feature or run a cluster-agent too old to support it still hear about
	// the migration to spec.features.appsec.injector.
	if hasAppsecAnnotations(dda.GetAnnotations()) {
		f.logger.V(0).Info("appsec.* annotations are deprecated; migrate to spec.features.appsec.injector")
	}

	// Everything below reads ddaSpec, and constants.GetClusterAgentServiceAccount
	// dereferences it unconditionally, so a nil spec can configure nothing.
	if ddaSpec == nil {
		return feature.RequiredComponents{}
	}

	var inj *v2alpha1.AppsecInjectorConfig
	if ddaSpec.Features != nil && ddaSpec.Features.Appsec != nil {
		inj = ddaSpec.Features.Appsec.Injector
	}

	// parseAnnotations skips the fallible annotations whose field the CRD already sets, so
	// a malformed annotation on a CRD-set field is not an error here. A malformed
	// annotation on a field the CRD leaves unset still is.
	cfg, err := parseAnnotations(dda.GetAnnotations(), inj)
	if err != nil {
		f.logger.Error(err, "failed to parse AppSec annotations")
		return feature.RequiredComponents{}
	}

	// Merge before validating: a CRD field can rescue configuration that the annotations
	// alone would not satisfy. mergeInjectorConfig is a no-op when inj is nil.
	cfg = mergeInjectorConfig(cfg, inj)
	if validateErr := cfg.Validate(); validateErr != nil {
		f.logger.Error(validateErr, "invalid AppSec configuration")
		return feature.RequiredComponents{}
	}
	f.config = cfg

	if !isAboveMinVersion(ddaSpec) {
		f.logger.V(1).Info("agent version is too low")
		return feature.RequiredComponents{}
	}

	if !f.config.isEnabled() {
		f.logger.V(1).Info("feature is disabled")
		return feature.RequiredComponents{}
	}

	if f.config.requiresNginxSupport() && !utils.IsAboveMinVersion(clusterAgentVersion(ddaSpec), ClusterAgentNginxMinVersion, nil) {
		f.logger.Info("ingress-nginx injection requires cluster-agent >= " + ClusterAgentNginxMinVersion)
		return feature.RequiredComponents{}
	}

	if f.config.requiresGKESupport() && !utils.IsAboveMinVersion(clusterAgentVersion(ddaSpec), ClusterAgentGKEMinVersion, nil) {
		f.logger.Info("gke-gateway injection requires cluster-agent >= " + ClusterAgentGKEMinVersion)
		return feature.RequiredComponents{}
	}

	f.owner = dda
	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

	// The cluster agent is required for the AppSec feature.
	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: new(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.ClusterAgentContainerName,
			},
		},
	}
}

// ManageDependencies adds the RBAC necessary for the appsec feature to be enabled and is still required when disabled
// to be able to do cleanup
func (f *appsecFeature) ManageDependencies(managers feature.ResourceManagers) error {
	rbacName := getAppsecRBACResourceName(f.owner, f.rbacSuffix)
	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *appsecFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
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

	// Set processor port only when explicitly configured (zero means unset)
	if f.config.ProcessorPort != 0 {
		if err := addEnvVar(DDAppsecProxyProcessorPort, strconv.Itoa(f.config.ProcessorPort)); err != nil {
			return err
		}
	}

	// Set optional string env vars (key → value, skipped when value is empty)
	for key, value := range map[string]string{
		DDAppsecProxyProcessorAddress:                             f.config.ProcessorAddress,
		DDClusterAgentAppsecInjectorProcessorServiceName:          f.config.ProcessorServiceName,
		DDClusterAgentAppsecInjectorProcessorServiceNamespace:     f.config.ProcessorServiceNamespace,
		DDClusterAgentAppsecInjectorMode:                          f.config.Mode,
		DDAdmissionControllerAppsecSidecarImage:                   f.config.SidecarImage,
		DDAdmissionControllerAppsecSidecarImageTag:                f.config.SidecarImageTag,
		DDAdmissionControllerAppsecSidecarPort:                    f.config.SidecarPort,
		DDAdmissionControllerAppsecSidecarHealthPort:              f.config.SidecarHealthPort,
		DDAdmissionControllerAppsecSidecarResourcesRequestsCPU:    f.config.SidecarResourcesRequestsCPU,
		DDAdmissionControllerAppsecSidecarResourcesRequestsMemory: f.config.SidecarResourcesRequestsMemory,
		DDAdmissionControllerAppsecSidecarResourcesLimitsCPU:      f.config.SidecarResourcesLimitsCPU,
		DDAdmissionControllerAppsecSidecarResourcesLimitsMemory:   f.config.SidecarResourcesLimitsMemory,
		DDAdmissionControllerAppsecSidecarBodyParsingSizeLimit:    f.config.SidecarBodyParsingSizeLimit,
		DDAdmissionControllerAppsecNginxModuleMountPath:           f.config.NginxModuleMountPath,
	} {
		if value != "" {
			if err := addEnvVar(key, value); err != nil {
				return err
			}
		}
	}

	return nil
}

func (f *appsecFeature) ManageSingleContainerNodeAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *appsecFeature) ManageNodeAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *appsecFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *appsecFeature) ManageOtelAgentGateway(_ feature.PodTemplateManagers) error {
	return nil
}
