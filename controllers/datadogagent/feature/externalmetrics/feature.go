// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package externalmetrics

import (
	"fmt"
	"strconv"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	componentdca "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

func init() {
	err := feature.Register(feature.ExternalMetricsIDType, buildExternalMetricsFeature)
	if err != nil {
		panic(err)
	}
}

func buildExternalMetricsFeature(options *feature.Options) feature.Feature {
	externalMetricsFeat := &externalMetricsFeature{}

	return externalMetricsFeat
}

type externalMetricsFeature struct {
	wpaController      bool
	useDDM             bool
	port               int32
	url                string
	keySecret          map[string]secret
	serviceAccountName string
	owner              metav1.Object
}

type secret struct {
	name string
	key  string
	data []byte
}

// ID returns the ID of the Feature
func (f *externalMetricsFeature) ID() feature.IDType {
	return feature.ExternalMetricsIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *externalMetricsFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	em := dda.Spec.Features.ExternalMetricsServer

	if em != nil && apiutils.BoolValue(em.Enabled) {
		f.wpaController = apiutils.BoolValue(em.WPAController)
		f.useDDM = apiutils.BoolValue(em.UseDatadogMetrics)
		f.port = *em.Port
		if em.Endpoint != nil {
			if em.Endpoint.URL != nil {
				f.url = *em.Endpoint.URL
			}
			creds := em.Endpoint.Credentials
			if creds != nil {
				f.keySecret = make(map[string]secret)
				if !v2alpha1.CheckAPIKeySufficiency(creds, apicommon.DDExternalMetricsProviderAPIKey) ||
					!v2alpha1.CheckAppKeySufficiency(creds, apicommon.DDExternalMetricsProviderAppKey) {
					// neither secrets nor the external metrics api/app key env vars are defined,
					// so store key data to create secret later
					for keyType, keyData := range v2alpha1.GetKeysFromCredentials(creds) {
						f.keySecret[keyType] = secret{
							data: keyData,
						}
					}
				}
				if v2alpha1.CheckAPIKeySufficiency(creds, apicommon.DDExternalMetricsProviderAPIKey) {
					// api key secret exists; store secret name and key instead
					if isSet, secretName, secretKey := v2alpha1.GetAPIKeySecret(creds, componentdca.GetDefaultExternalMetricSecretName(f.owner)); isSet {
						f.keySecret[apicommon.DefaultAPIKeyKey] = secret{
							name: secretName,
							key:  secretKey,
						}
					}
				}
				if v2alpha1.CheckAppKeySufficiency(creds, apicommon.DDExternalMetricsProviderAppKey) {
					// app key secret exists; store secret name and key instead
					if isSet, secretName, secretKey := v2alpha1.GetAppKeySecret(creds, componentdca.GetDefaultExternalMetricSecretName(f.owner)); isSet {
						f.keySecret[apicommon.DefaultAPPKeyKey] = secret{
							name: secretName,
							key:  secretKey,
						}
					}
				}
			}
		}

		f.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)

		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *externalMetricsFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	// f.owner = dda
	// if dda.Spec.ClusterAgent.Config != nil && dda.Spec.ClusterAgent.Config.ExternalMetrics != nil {
	// 	em := dda.Spec.ClusterAgent.Config.ExternalMetrics

	// 	if em != nil && apiutils.BoolValue(em.Enabled) {
	// 		f.wpaController = em.WpaController
	// 		f.useDDM = em.UseDatadogMetrics
	// 		f.port = *em.Port
	// 		if em.Endpoint != nil {
	// 			f.url = *em.Endpoint
	// 		}
	// 		if em.Credentials != nil {
	// 			if em.Credentials != nil {
	// 				f.keySecret = make(map[string]secret)
	// 				if !v1alpha1.CheckAPIKeySufficiency(em.Credentials, apicommon.DDExternalMetricsProviderAPIKey) ||
	// 				!v1alpha1.CheckAppKeySufficiency(em.Credentials, apicommon.DDExternalMetricsProviderAppKey) {
	// 					// neither secrets nor the external metrics api/app key env vars are defined,
	// 					// so store key data to create secret later
	// 					for keyType, keyData := range v1alpha1.GetKeysFromCredentials(em.Credentials) {
	// 						f.keySecret[keyType] = secret{
	// 							data: keyData,
	// 						}
	// 					}
	// 				}
	// 				if v1alpha1.CheckAPIKeySufficiency(em.Credentials, apicommon.DDExternalMetricsProviderAPIKey) {
	// 					// api key secret exists; store secret name and key instead
	// 					if isSet, secretName, secretKey := v1alpha1.GetAPIKeySecret(em.Credentials, componentdca.GetDefaultExternalMetricSecretName(f.owner)); isSet {
	// 						f.keySecret[apicommon.DefaultAPIKeyKey] = secret{
	// 							name: secretName,
	// 							key: secretKey,
	// 						}
	// 					}
	// 				}
	// 				if v1alpha1.CheckAppKeySufficiency(em.Credentials, apicommon.DDExternalMetricsProviderAppKey) {
	// 					// app key secret exists; store secret name and key instead
	// 					if isSet, secretName, secretKey := v1alpha1.GetAppKeySecret(em.Credentials, componentdca.GetDefaultExternalMetricSecretName(f.owner)); isSet {
	// 						f.keySecret[apicommon.DefaultAPPKeyKey] = secret{
	// 							name: secretName,
	// 							key: secretKey,
	// 						}
	// 					}
	// 				}
	// 			}
	// 		}

	// 		f.serviceAccountName = v1alpha1.GetClusterAgentServiceAccount(dda)

	// 		reqComp = feature.RequiredComponents{
	// 			ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
	// 		}
	// 	}
	// }

	// return reqComp

	// do not apply this feature on v1alpha1
	// it breaks the unittests in `controller_test.go` because the `store` modifies
	// the dependency resources with additional labels which make the comparison fail

	return feature.RequiredComponents{}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *externalMetricsFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	ns := f.owner.GetNamespace()
	// service
	emPort := []corev1.ServicePort{
		{
			Protocol: corev1.ProtocolTCP,
			Port:     f.port,
			Name:     apicommon.ExternalMetricsPortName,
		},
	}
	selector := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:      f.owner.GetName(),
		apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
	}
	serviceName := componentdca.GetMetricsServerServiceName(f.owner)
	if err := managers.ServiceManager().AddService(serviceName, ns, selector, emPort, nil); err != nil {
		return fmt.Errorf("error adding external metrics provider service to store: %w", err)
	}

	// apiservice
	apiServiceSpec := apiregistrationv1.APIServiceSpec{
		Service: &apiregistrationv1.ServiceReference{
			Name:      serviceName,
			Namespace: ns,
			Port:      &f.port,
		},
		Version:               "v1beta1",
		InsecureSkipTLSVerify: true,
		Group:                 rbac.ExternalMetricsAPIGroup,
		GroupPriorityMinimum:  100,
		VersionPriority:       100,
	}
	if err := managers.APIServiceManager().AddAPIService(componentdca.GetMetricsServerAPIServiceName(), ns, apiServiceSpec); err != nil {
		return fmt.Errorf("error adding external metrics provider apiservice to store: %w", err)
	}

	// credential secret
	if len(f.keySecret) != 0 {
		for idx, s := range f.keySecret {
			if len(s.data) != 0 {
				if err := managers.SecretManager().AddSecret(ns, componentdca.GetDefaultExternalMetricSecretName(f.owner), idx, string(s.data)); err != nil {
					return fmt.Errorf("error adding external metrics provider credentials secret to store: %w", err)
				}
			}
		}
	}

	// rbac
	rbacResourcesName := componentdca.GetClusterAgentRbacResourcesName(f.owner)
	if err := managers.RBACManager().AddClusterPolicyRules(ns, rbacResourcesName, f.serviceAccountName, getDCAClusterPolicyRules(f.useDDM, f.wpaController)); err != nil {
		return fmt.Errorf("error adding external metrics provider dca clusterrole and clusterrolebinding to store: %w", err)
	}
	if err := managers.RBACManager().AddPolicyRules(ns, rbacResourcesName, f.serviceAccountName, getDCAPolicyRules()); err != nil {
		return fmt.Errorf("error adding external metrics provider dca role and rolebinding to store: %w", err)
	}
	if err := managers.RBACManager().AddClusterPolicyRules("kube-system", componentdca.GetExternalMetricsReaderClusterRoleName(f.owner, managers.Store().GetVersionInfo()), "horizontal-pod-autoscaler", getExternalMetricsReaderPolicyRules()); err != nil {
		return fmt.Errorf("error adding external metrics provider external metrics reader clusterrole and clusterrolebinding to store: %w", err)
	}
	if err := managers.RBACManager().AddClusterRoleBinding(ns, componentdca.GetHPAClusterRoleBindingName(f.owner), f.serviceAccountName, getAuthDelegatorRoleRef()); err != nil {
		return fmt.Errorf("error adding external metrics provider auth delegator clusterrolebinding to store: %w", err)
	}
	if err := managers.RBACManager().AddRoleBinding(ns, componentdca.GetApiserverAuthReaderRoleBindingName(f.owner), f.serviceAccountName, getApiserverAuthReaderRoleRef()); err != nil {
		return fmt.Errorf("error adding external metrics provider apiserver auth rolebinding to store: %w", err)
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *externalMetricsFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDExternalMetricsProviderEnabled,
		Value: "true",
	})
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDExternalMetricsProviderPort,
		Value: strconv.FormatInt(int64(f.port), 10),
	})
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDExternalMetricsProviderUseDatadogMetric,
		Value: apiutils.BoolToString(&f.useDDM),
	})
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDExternalMetricsProviderWPAController,
		Value: apiutils.BoolToString(&f.wpaController),
	})

	if f.url != "" {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDExternalMetricsProviderEndpoint,
			Value: f.url,
		})
	}

	if len(f.keySecret) != 0 {
		// api key
		if s, ok := f.keySecret[apicommon.DefaultAPIKeyKey]; ok {
			var apiKeyEnvVar *corev1.EnvVar
			// api key from existing secret
			if s.name != "" {
				apiKeyEnvVar = component.BuildEnvVarFromSource(
					apicommon.DDExternalMetricsProviderAPIKey,
					component.BuildEnvVarFromSecret(s.name, s.key),
				)
			} else {
				// api key from seret created by operator
				apiKeyEnvVar = component.BuildEnvVarFromSource(
					apicommon.DDExternalMetricsProviderAPIKey,
					component.BuildEnvVarFromSecret(componentdca.GetDefaultExternalMetricSecretName(f.owner), apicommon.DefaultAPIKeyKey),
				)
			}
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, apiKeyEnvVar)
		}
		// app key
		if s, ok := f.keySecret[apicommon.DefaultAPPKeyKey]; ok {
			var appKeyEnvVar *corev1.EnvVar
			// app key from existing secret
			if s.name != "" {
				appKeyEnvVar = component.BuildEnvVarFromSource(
					apicommon.DDExternalMetricsProviderAppKey,
					component.BuildEnvVarFromSecret(s.name, s.key),
				)
			} else {
				// api key from seret created by operator
				appKeyEnvVar = component.BuildEnvVarFromSource(
					apicommon.DDExternalMetricsProviderAppKey,
					component.BuildEnvVarFromSecret(componentdca.GetDefaultExternalMetricSecretName(f.owner), apicommon.DefaultAPPKeyKey),
				)
			}
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, appKeyEnvVar)
		}
	}

	managers.Port().AddPortToContainer(apicommonv1.ClusterAgentContainerName, &corev1.ContainerPort{
		Name:          apicommon.ExternalMetricsPortName,
		ContainerPort: f.port,
		Protocol:      corev1.ProtocolTCP,
	})

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *externalMetricsFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *externalMetricsFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
