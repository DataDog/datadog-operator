// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package externalmetrics

import (
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/objects"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

func init() {
	err := feature.Register(feature.ExternalMetricsIDType, buildExternalMetricsFeature)
	if err != nil {
		panic(err)
	}
}

func buildExternalMetricsFeature(options *feature.Options) feature.Feature {
	externalMetricsFeat := &externalMetricsFeature{}

	if options != nil {
		externalMetricsFeat.logger = options.Logger
	}

	return externalMetricsFeat
}

type externalMetricsFeature struct {
	useWPA             bool
	useDDM             bool
	port               int32
	url                string
	keySecret          map[string]secret
	serviceAccountName string
	owner              metav1.Object
	logger             logr.Logger

	createKubernetesNetworkPolicy bool
	createCiliumNetworkPolicy     bool
	registerAPIService            bool
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
		// By default, we register the external metrics endpoint
		f.registerAPIService = em.RegisterAPIService == nil || apiutils.BoolValue(em.RegisterAPIService)

		f.useWPA = apiutils.BoolValue(em.WPAController)
		f.useDDM = apiutils.BoolValue(em.UseDatadogMetrics)
		f.port = *em.Port
		if em.Endpoint != nil {
			if em.Endpoint.URL != nil {
				f.url = *em.Endpoint.URL
			}
			creds := em.Endpoint.Credentials
			if creds != nil {
				f.keySecret = make(map[string]secret)
				if !secrets.CheckAPIKeySufficiency(creds, DDExternalMetricsProviderAPIKey) ||
					!secrets.CheckAppKeySufficiency(creds, DDExternalMetricsProviderAppKey) {
					// for one of api or app keys, neither secrets nor external metrics key env vars
					// are defined, so store key data to create secret later
					for keyType, keyData := range secrets.GetKeysFromCredentials(creds) {
						f.keySecret[keyType] = secret{
							data: keyData,
						}
					}
				}
				if creds.APISecret != nil {
					_, secretName, secretKey := secrets.GetAPIKeySecret(creds, componentdca.GetDefaultExternalMetricSecretName(f.owner))
					if secretName != "" && secretKey != "" {
						// api key secret exists; store secret name and key instead
						f.keySecret[v2alpha1.DefaultAPIKeyKey] = secret{
							name: secretName,
							key:  secretKey,
						}
					}
				}
				if creds.AppSecret != nil {
					_, secretName, secretKey := secrets.GetAppKeySecret(creds, componentdca.GetDefaultExternalMetricSecretName(f.owner))
					if secretName != "" && secretKey != "" {
						// app key secret exists; store secret name and key instead
						f.keySecret[v2alpha1.DefaultAPPKeyKey] = secret{
							name: secretName,
							key:  secretKey,
						}
					}
				}
			}
		}

		f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda)

		if enabled, flavor := constants.IsNetworkPolicyEnabled(dda); enabled {
			if flavor == v2alpha1.NetworkPolicyFlavorCilium {
				f.createCiliumNetworkPolicy = true
			} else {
				f.createKubernetesNetworkPolicy = true
			}
		}

		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *externalMetricsFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	ns := f.owner.GetNamespace()
	// service
	emPorts := []corev1.ServicePort{
		{
			Protocol: corev1.ProtocolTCP,
			Port:     f.port,
			Name:     externalMetricsPortName,
		},
	}
	selector := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:      f.owner.GetName(),
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
	}
	serviceName := componentdca.GetMetricsServerServiceName(f.owner)
	if err := managers.ServiceManager().AddService(serviceName, ns, selector, emPorts, nil); err != nil {
		return fmt.Errorf("error adding external metrics provider service to store: %w", err)
	}

	// Credential secret
	if len(f.keySecret) != 0 {
		for idx, s := range f.keySecret {
			if len(s.data) != 0 {
				if err := managers.SecretManager().AddSecret(ns, componentdca.GetDefaultExternalMetricSecretName(f.owner), idx, string(s.data)); err != nil {
					return fmt.Errorf("error adding external metrics provider credentials secret to store: %w", err)
				}
			}
		}
	}

	// RBACs
	rbacResourcesName := componentdca.GetClusterAgentRbacResourcesName(f.owner)
	if err := managers.RBACManager().AddClusterPolicyRules(ns, rbacResourcesName, f.serviceAccountName, getDCAClusterPolicyRules(f.useDDM, f.useWPA)); err != nil {
		return fmt.Errorf("error adding external metrics provider dca clusterrole and clusterrolebinding to store: %w", err)
	}
	if err := managers.RBACManager().AddClusterRoleBinding(ns, componentdca.GetHPAClusterRoleBindingName(f.owner), f.serviceAccountName, getAuthDelegatorRoleRef()); err != nil {
		return fmt.Errorf("error adding external metrics provider auth delegator clusterrolebinding to store: %w", err)
	}

	if f.registerAPIService {
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

		// RBAC
		platformInfo := managers.Store().GetPlatformInfo()
		if err := managers.RBACManager().AddClusterPolicyRules("kube-system", componentdca.GetExternalMetricsReaderClusterRoleName(f.owner, platformInfo.GetVersionInfo()), "horizontal-pod-autoscaler", getExternalMetricsReaderPolicyRules()); err != nil {
			return fmt.Errorf("error adding external metrics provider external metrics reader clusterrole and clusterrolebinding to store: %w", err)
		}
	}

	// network policies
	policyName, podSelector := objects.GetNetworkPolicyMetadata(f.owner, v2alpha1.ClusterAgentComponentName)
	if f.createKubernetesNetworkPolicy {
		ingressRules := []netv1.NetworkPolicyIngressRule{
			{
				Ports: []netv1.NetworkPolicyPort{
					{
						Port: &intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: f.port,
						},
					},
				},
			},
		}
		return managers.NetworkPolicyManager().AddKubernetesNetworkPolicy(
			policyName,
			f.owner.GetNamespace(),
			podSelector,
			nil,
			ingressRules,
			nil,
		)
	} else if f.createCiliumNetworkPolicy {
		policySpecs := []cilium.NetworkPolicySpec{
			{
				Description:      "Ingress from API server for external metrics",
				EndpointSelector: podSelector,
				Ingress: []cilium.IngressRule{
					{
						FromEntities: []cilium.Entity{cilium.EntityWorld},
						ToPorts: []cilium.PortRule{
							{
								Ports: []cilium.PortProtocol{
									{
										Port:     strconv.Itoa(int(f.port)),
										Protocol: cilium.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
		}
		return managers.CiliumPolicyManager().AddCiliumPolicy(policyName, f.owner.GetNamespace(), policySpecs)
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *externalMetricsFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDExternalMetricsProviderEnabled,
		Value: "true",
	})
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDExternalMetricsProviderPort,
		Value: strconv.FormatInt(int64(f.port), 10),
	})
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDExternalMetricsProviderUseDatadogMetric,
		Value: apiutils.BoolToString(&f.useDDM),
	})
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDExternalMetricsProviderWPAController,
		Value: apiutils.BoolToString(&f.useWPA),
	})

	if f.url != "" {
		managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDExternalMetricsProviderEndpoint,
			Value: f.url,
		})
	}

	if len(f.keySecret) != 0 {
		// api key
		if s, ok := f.keySecret[v2alpha1.DefaultAPIKeyKey]; ok {
			var apiKeyEnvVar *corev1.EnvVar
			// api key from existing secret
			if s.name != "" {
				apiKeyEnvVar = common.BuildEnvVarFromSource(
					DDExternalMetricsProviderAPIKey,
					common.BuildEnvVarFromSecret(s.name, s.key),
				)
			} else {
				// api key from secret created by operator
				apiKeyEnvVar = common.BuildEnvVarFromSource(
					DDExternalMetricsProviderAPIKey,
					common.BuildEnvVarFromSecret(componentdca.GetDefaultExternalMetricSecretName(f.owner), v2alpha1.DefaultAPIKeyKey),
				)
			}
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, apiKeyEnvVar)
		}
		// app key
		if s, ok := f.keySecret[v2alpha1.DefaultAPPKeyKey]; ok {
			var appKeyEnvVar *corev1.EnvVar
			// app key from existing secret
			if s.name != "" {
				appKeyEnvVar = common.BuildEnvVarFromSource(
					DDExternalMetricsProviderAppKey,
					common.BuildEnvVarFromSecret(s.name, s.key),
				)
			} else {
				// api key from secret created by operator
				appKeyEnvVar = common.BuildEnvVarFromSource(
					DDExternalMetricsProviderAppKey,
					common.BuildEnvVarFromSecret(componentdca.GetDefaultExternalMetricSecretName(f.owner), v2alpha1.DefaultAPPKeyKey),
				)
			}
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, appKeyEnvVar)
		}
	}

	managers.Port().AddPortToContainer(apicommon.ClusterAgentContainerName, &corev1.ContainerPort{
		Name:          externalMetricsPortName,
		ContainerPort: f.port,
		Protocol:      corev1.ProtocolTCP,
	})

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *externalMetricsFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *externalMetricsFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *externalMetricsFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
