// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package test

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

var (
	// apiVersion datadoghqv1alpha1 api version
	apiVersion = fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version)
	pullPolicy = v1.PullIfNotPresent
)

// NewDatadogAgentOptions set of option for the DatadogAgent creation
type NewDatadogAgentOptions struct {
	Labels                           map[string]string
	Annotations                      map[string]string
	Status                           *datadoghqv1alpha1.DatadogAgentStatus
	UseEDS                           bool
	ClusterAgentEnabled              bool
	MetricsServerEnabled             bool
	MetricsServerPort                int32
	MetricsServerEndpoint            string
	MetricsServerUseDatadogMetric    bool
	MetricsServerWPAController       bool
	ClusterChecksEnabled             bool
	NodeAgentConfig                  *datadoghqv1alpha1.NodeAgentConfig
	APMEnabled                       bool
	ProcessEnabled                   bool
	SystemProbeEnabled               bool
	SystemProbeSeccompProfileName    string
	SystemProbeAppArmorProfileName   string
	SystemProbeTCPQueueLengthEnabled bool
	SystemProbeOOMKillEnabled        bool
	Creds                            *datadoghqv1alpha1.AgentCredentials
	ClusterName                      *string
	Confd                            *datadoghqv1alpha1.ConfigDirSpec
	Checksd                          *datadoghqv1alpha1.ConfigDirSpec
	Volumes                          []corev1.Volume
	VolumeMounts                     []corev1.VolumeMount
	ClusterAgentVolumes              []corev1.Volume
	ClusterAgentVolumeMounts         []corev1.VolumeMount
	ClusterAgentEnvVars              []corev1.EnvVar
	CustomConfig                     string
	AgentDaemonsetName               string
	ClusterAgentDeploymentName       string
	ClusterChecksRunnerEnabled       bool
	ClusterChecksRunnerVolumes       []corev1.Volume
	ClusterChecksRunnerVolumeMounts  []corev1.VolumeMount
	ClusterChecksRunnerEnvVars       []corev1.EnvVar
	APIKeyExistingSecret             string
	APISecret                        *datadoghqv1alpha1.Secret
	Site                             string
	HostPort                         int32
	HostNetwork                      bool
	AdmissionControllerEnabled       bool
	AdmissionMutateUnlabelled        bool
	AdmissionServiceName             string
	ComplianceEnabled                bool
	ComplianceCheckInterval          metav1.Duration
	ComplianceConfigDir              *datadoghqv1alpha1.ConfigDirSpec
	RuntimeSecurityEnabled           bool
	RuntimeSyscallMonitorEnabled     bool
	RuntimePoliciesDir               *datadoghqv1alpha1.ConfigDirSpec
	SecurityContext                  *corev1.PodSecurityContext
	CreateNetworkPolicy              bool
}

// NewDefaultedDatadogAgent returns an initialized and defaulted DatadogAgent for testing purpose
func NewDefaultedDatadogAgent(ns, name string, options *NewDatadogAgentOptions) *datadoghqv1alpha1.DatadogAgent {
	ad := &datadoghqv1alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogAgent",
			APIVersion: apiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  ns,
			Name:       name,
			Labels:     map[string]string{},
			Finalizers: []string{"finalizer.agent.datadoghq.com"},
		},
	}
	ad.Spec = datadoghqv1alpha1.DatadogAgentSpec{
		Credentials: datadoghqv1alpha1.AgentCredentials{Token: "token-foo"},
		Agent: &datadoghqv1alpha1.DatadogAgentSpecAgentSpec{
			Image: datadoghqv1alpha1.ImageConfig{
				Name:       "datadog/agent:latest",
				PullPolicy: &pullPolicy,
			},
			Config:             datadoghqv1alpha1.NodeAgentConfig{},
			DeploymentStrategy: &datadoghqv1alpha1.DaemonSetDeploymentStrategy{},
			Apm:                datadoghqv1alpha1.APMSpec{},
			Log:                datadoghqv1alpha1.LogSpec{},
			Process:            datadoghqv1alpha1.ProcessSpec{},
		},
	}
	if options != nil {
		if options.UseEDS {
			ad.Spec.Agent.UseExtendedDaemonset = &options.UseEDS
		}
		if options.Labels != nil {
			for key, value := range options.Labels {
				ad.Labels[key] = value
			}
		}
		if options.Annotations != nil {
			ad.Annotations = make(map[string]string)
			for key, value := range options.Annotations {
				ad.Annotations[key] = value
			}
		}

		ad.Spec.Agent.DaemonsetName = options.AgentDaemonsetName
		ad.Spec.Site = options.Site
		ad.Spec.Agent.NetworkPolicy = datadoghqv1alpha1.NetworkPolicySpec{
			Create: &options.CreateNetworkPolicy,
		}

		if options.HostPort != 0 {
			ad.Spec.Agent.Config.HostPort = &options.HostPort
		}

		if options.Status != nil {
			ad.Status = *options.Status
		}

		if len(options.Volumes) != 0 {
			ad.Spec.Agent.Config.Volumes = options.Volumes
		}
		if len(options.VolumeMounts) != 0 {
			ad.Spec.Agent.Config.VolumeMounts = options.VolumeMounts
		}
		if options.ClusterAgentEnabled {
			ad.Spec.ClusterAgent = &datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Config: datadoghqv1alpha1.ClusterAgentConfig{},
				Rbac: datadoghqv1alpha1.RbacConfig{
					Create: datadoghqv1alpha1.NewBoolPointer(true),
				},
				DeploymentName: options.ClusterAgentDeploymentName,
				NetworkPolicy: datadoghqv1alpha1.NetworkPolicySpec{
					Create: &options.CreateNetworkPolicy,
				},
			}

			if options.MetricsServerEnabled {
				externalMetricsConfig := datadoghqv1alpha1.ExternalMetricsConfig{
					Enabled:           true,
					UseDatadogMetrics: options.MetricsServerUseDatadogMetric,
					WpaController:     options.MetricsServerWPAController,
				}

				if options.MetricsServerPort != 0 {
					externalMetricsConfig.Port = datadoghqv1alpha1.NewInt32Pointer(options.MetricsServerPort)
				}

				if options.MetricsServerEndpoint != "" {
					externalMetricsConfig.Endpoint = &options.MetricsServerEndpoint
				}

				ad.Spec.ClusterAgent.Config.ExternalMetrics = &externalMetricsConfig
			}

			if options.AdmissionControllerEnabled {
				ad.Spec.ClusterAgent.Config.AdmissionController = &datadoghqv1alpha1.AdmissionControllerConfig{
					Enabled:          true,
					MutateUnlabelled: &options.AdmissionMutateUnlabelled,
				}
				if options.AdmissionServiceName != "" {
					ad.Spec.ClusterAgent.Config.AdmissionController.ServiceName = &options.AdmissionServiceName
				}
			}

			if options.ClusterChecksEnabled {
				ad.Spec.ClusterAgent.Config.ClusterChecksEnabled = datadoghqv1alpha1.NewBoolPointer(true)
			}

			if len(options.ClusterAgentVolumes) != 0 {
				ad.Spec.ClusterAgent.Config.Volumes = options.ClusterAgentVolumes
			}
			if len(options.ClusterAgentVolumeMounts) != 0 {
				ad.Spec.ClusterAgent.Config.VolumeMounts = options.ClusterAgentVolumeMounts
			}
			if len(options.ClusterAgentEnvVars) != 0 {
				ad.Spec.ClusterAgent.Config.Env = options.ClusterAgentEnvVars
			}
		}

		if options.ClusterChecksRunnerEnabled {
			ad.Spec.ClusterChecksRunner = &datadoghqv1alpha1.DatadogAgentSpecClusterChecksRunnerSpec{
				Config: datadoghqv1alpha1.ClusterChecksRunnerConfig{},
				Rbac: datadoghqv1alpha1.RbacConfig{
					Create: datadoghqv1alpha1.NewBoolPointer(true),
				},
				NetworkPolicy: datadoghqv1alpha1.NetworkPolicySpec{
					Create: &options.CreateNetworkPolicy,
				},
			}
			if len(options.ClusterChecksRunnerEnvVars) != 0 {
				ad.Spec.ClusterChecksRunner.Config.Env = options.ClusterChecksRunnerEnvVars
			}
			if len(options.ClusterChecksRunnerVolumes) != 0 {
				ad.Spec.ClusterChecksRunner.Config.VolumeMounts = options.ClusterChecksRunnerVolumeMounts
			}
			if len(options.ClusterChecksRunnerVolumes) != 0 {
				ad.Spec.ClusterChecksRunner.Config.Volumes = options.ClusterChecksRunnerVolumes
			}
		}

		if options.NodeAgentConfig != nil {
			ad.Spec.Agent.Config = *options.NodeAgentConfig
		}

		if options.APMEnabled {
			ad.Spec.Agent.Apm.Enabled = datadoghqv1alpha1.NewBoolPointer(true)
		}

		if options.ProcessEnabled {
			ad.Spec.Agent.Process.Enabled = datadoghqv1alpha1.NewBoolPointer(true)
		}

		if options.HostNetwork {
			ad.Spec.Agent.HostNetwork = true
		}

		if options.SystemProbeEnabled {
			ad.Spec.Agent.SystemProbe.Enabled = datadoghqv1alpha1.NewBoolPointer(true)
			if options.SystemProbeAppArmorProfileName != "" {
				ad.Spec.Agent.SystemProbe.AppArmorProfileName = options.SystemProbeAppArmorProfileName
			}
			if options.SystemProbeSeccompProfileName != "" {
				ad.Spec.Agent.SystemProbe.SecCompProfileName = options.SystemProbeSeccompProfileName
			}

			if options.SystemProbeTCPQueueLengthEnabled {
				ad.Spec.Agent.SystemProbe.EnableTCPQueueLength = datadoghqv1alpha1.NewBoolPointer(true)
			}

			if options.SystemProbeOOMKillEnabled {
				ad.Spec.Agent.SystemProbe.EnableOOMKill = datadoghqv1alpha1.NewBoolPointer(true)
			}
		}

		if options.Creds != nil {
			ad.Spec.Credentials = *options.Creds
		}

		if options.ClusterName != nil {
			ad.Spec.ClusterName = *options.ClusterName
		}

		if options.Confd != nil {
			ad.Spec.Agent.Config.Confd = options.Confd
		}

		if options.Checksd != nil {
			ad.Spec.Agent.Config.Checksd = options.Checksd
		}

		if options.CustomConfig != "" {
			ad.Spec.Agent.CustomConfig = &datadoghqv1alpha1.CustomConfigSpec{
				ConfigData: &options.CustomConfig,
			}
		}

		if options.APIKeyExistingSecret != "" {
			ad.Spec.Credentials.APIKeyExistingSecret = options.APIKeyExistingSecret
		}

		if options.APISecret != nil {
			ad.Spec.Credentials.APISecret = options.APISecret
		}

		if options.ComplianceEnabled {
			ad.Spec.Agent.Security.Compliance.Enabled = datadoghqv1alpha1.NewBoolPointer(true)

			if options.ComplianceCheckInterval.Duration != 0 {
				ad.Spec.Agent.Security.Compliance.CheckInterval = &options.ComplianceCheckInterval
			}
			if options.ComplianceConfigDir != nil {
				ad.Spec.Agent.Security.Compliance.ConfigDir = options.ComplianceConfigDir
			}
		}

		if options.RuntimeSecurityEnabled {
			ad.Spec.Agent.Security.Runtime.Enabled = datadoghqv1alpha1.NewBoolPointer(true)

			if options.RuntimePoliciesDir != nil {
				ad.Spec.Agent.Security.Runtime.PoliciesDir = options.RuntimePoliciesDir
			}

			if options.RuntimeSyscallMonitorEnabled {
				ad.Spec.Agent.Security.Runtime.SyscallMonitor = &datadoghqv1alpha1.SyscallMonitorSpec{
					Enabled: datadoghqv1alpha1.NewBoolPointer(true),
				}
			}
		}
	}
	return datadoghqv1alpha1.DefaultDatadogAgent(ad)
}

// NewExtendedDaemonSetOptions set of option for the ExtendedDaemonset creation
type NewExtendedDaemonSetOptions struct {
	CreationTime *time.Time
	Annotations  map[string]string
	Labels       map[string]string
	Canary       *edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary
	Status       *edsdatadoghqv1alpha1.ExtendedDaemonSetStatus
}

// NewExtendedDaemonSet return new ExtendedDDaemonset instance for testing purpose
func NewExtendedDaemonSet(ns, name string, options *NewExtendedDaemonSetOptions) *edsdatadoghqv1alpha1.ExtendedDaemonSet {
	dd := &edsdatadoghqv1alpha1.ExtendedDaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExtendedDaemonSet",
			APIVersion: apiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}
	if options != nil {
		if options.CreationTime != nil {
			dd.CreationTimestamp = metav1.NewTime(*options.CreationTime)
		}
		if options.Annotations != nil {
			for key, value := range options.Annotations {
				dd.Annotations[key] = value
			}
		}
		if options.Labels != nil {
			for key, value := range options.Labels {
				dd.Labels[key] = value
			}
		}
		if options.Canary != nil {
			dd.Spec.Strategy.Canary = options.Canary
		}
		if options.Status != nil {
			dd.Status = *options.Status
		}
	}

	return dd
}

// NewDeploymentOptions set of option for the Deployment creation
type NewDeploymentOptions struct {
	CreationTime           *time.Time
	Annotations            map[string]string
	Labels                 map[string]string
	ForceAvailableReplicas *int32
}

// NewClusterAgentDeployment return new Cluster Agent Deployment instance for testing purpose
func NewClusterAgentDeployment(ns, name string, options *NewDeploymentOptions) *appsv1.Deployment {
	dca := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        fmt.Sprintf("%s-%s", name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix),
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}

	_, _ = comparison.SetMD5GenerationAnnotation(&dca.ObjectMeta, dca.Spec)
	if options != nil {
		if options.CreationTime != nil {
			dca.CreationTimestamp = metav1.NewTime(*options.CreationTime)
		}
		if options.Annotations != nil {
			for key, value := range options.Annotations {
				dca.Annotations[key] = value
			}
		}
		if options.Labels != nil {
			for key, value := range options.Labels {
				dca.Labels[key] = value
			}
		}
		if options.ForceAvailableReplicas != nil {
			dca.Status.AvailableReplicas = *options.ForceAvailableReplicas
		}
	}

	return dca
}

// NewSecretOptions used to provide option to the NewSecret function
type NewSecretOptions struct {
	CreationTime *time.Time
	Annotations  map[string]string
	Labels       map[string]string
	Data         map[string][]byte
}

// NewSecret returns new Secret instance
func NewSecret(ns, name string, opts *NewSecretOptions) *corev1.Secret {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Type: corev1.SecretTypeOpaque,
	}

	if opts != nil {
		if opts.CreationTime != nil {
			secret.CreationTimestamp = metav1.NewTime(*opts.CreationTime)
		}
		if opts.Labels != nil {
			secret.Labels = opts.Labels
		}
		if opts.Annotations != nil {
			secret.Annotations = opts.Annotations
		}
		if opts.Data != nil {
			secret.Data = opts.Data
		}
	}

	return secret
}

// NewServiceOptions used to provide options to the NewService function
type NewServiceOptions struct {
	CreationTime *time.Time
	Annotations  map[string]string
	Labels       map[string]string
	Spec         *corev1.ServiceSpec
}

// NewService returns new corev1.Service instance
func NewService(ns, name string, opts *NewServiceOptions) *corev1.Service {
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}

	if opts != nil {
		if opts.CreationTime != nil {
			service.CreationTimestamp = metav1.NewTime(*opts.CreationTime)
		}
		if opts.Labels != nil {
			service.Labels = opts.Labels
		}
		if opts.Annotations != nil {
			service.Annotations = opts.Annotations
		}
		if opts.Spec != nil {
			service.Spec = *opts.Spec
		}
	}

	return service
}

// NewAPIServiceOptions used to provide options to the NewAPIService function
type NewAPIServiceOptions struct {
	CreationTime *time.Time
	Annotations  map[string]string
	Labels       map[string]string
	Spec         *apiregistrationv1.APIServiceSpec
}

// NewAPIService returns new apiregistrationv1.APIService instance
func NewAPIService(ns, name string, opts *NewAPIServiceOptions) *apiregistrationv1.APIService {
	apiService := &apiregistrationv1.APIService{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIService",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}

	if opts != nil {
		if opts.CreationTime != nil {
			apiService.CreationTimestamp = metav1.NewTime(*opts.CreationTime)
		}
		if opts.Labels != nil {
			apiService.Labels = opts.Labels
		}
		if opts.Annotations != nil {
			apiService.Annotations = opts.Annotations
		}
		if opts.Spec != nil {
			apiService.Spec = *opts.Spec
		}
	}

	return apiService
}
