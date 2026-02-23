// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	err := feature.Register(feature.PrivateActionRunnerIDType, buildPrivateActionRunnerFeature)
	if err != nil {
		panic(err)
	}
}

func buildPrivateActionRunnerFeature(options *feature.Options) feature.Feature {
	parFeat := &privateActionRunnerFeature{}
	if options != nil {
		parFeat.logger = options.Logger
	}
	return parFeat
}

// PrivateActionRunnerConfig represents the parsed configuration from YAML
type PrivateActionRunnerConfig struct {
	Enabled              bool     `yaml:"enabled"`
	SelfEnroll           bool     `yaml:"self_enroll"`
	URN                  string   `yaml:"urn"`
	PrivateKey           string   `yaml:"private_key"`
	IdentityUseK8sSecret *bool    `yaml:"identity_use_k8s_secret"`
	IdentitySecretName   string   `yaml:"identity_secret_name"`
	ActionsAllowlist     []string `yaml:"actions_allowlist"`
}

type privateActionRunnerFeature struct {
	owner                     metav1.Object
	logger                    logr.Logger
	nodeEnabled               bool
	nodeConfigData            string
	clusterConfig             *PrivateActionRunnerConfig
	clusterServiceAccountName string
}

// ID returns the ID of the Feature
func (f *privateActionRunnerFeature) ID() feature.IDType {
	return feature.PrivateActionRunnerIDType
}

const defaultConfigData = "private_action_runner:\n    enabled: true\n"

// Configure configures the feature from annotations on the DatadogAgent object.
func (f *privateActionRunnerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	// Check for Node Agent configuration (annotation-based)
	if featureutils.HasFeatureEnableAnnotation(dda, featureutils.EnablePrivateActionRunnerAnnotation) {
		f.nodeEnabled = true

		// Use config data from annotation directly, or fall back to default
		if configData, ok := featureutils.GetFeatureConfigAnnotation(dda, featureutils.PrivateActionRunnerConfigDataAnnotation); ok {
			f.nodeConfigData = configData
		} else {
			f.nodeConfigData = defaultConfigData
		}

		reqComp.Agent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.CoreAgentContainerName,
				apicommon.PrivateActionRunnerContainerName,
			},
		}
	}

	// Check for Cluster Agent configuration (annotation-based)
	if featureutils.HasFeatureEnableAnnotation(dda, featureutils.EnableClusterAgentPrivateActionRunnerAnnotation) {
		configData, ok := featureutils.GetFeatureConfigAnnotation(dda, featureutils.ClusterAgentPrivateActionRunnerConfigDataAnnotation)
		if !ok {
			configData = defaultConfigData
		}

		clusterConfig, err := parsePrivateActionRunnerConfig(configData)
		if err != nil {
			f.logger.Error(err, "failed to parse private action runner config")
			return reqComp
		}
		f.clusterConfig = clusterConfig
		if !f.clusterConfig.Enabled {
			// Due-diligence
			f.logger.Info("private_action_runner.enabled=false in configdata is overridden by the enable annotation")
			f.clusterConfig.Enabled = true
		}
		f.clusterServiceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

		reqComp.ClusterAgent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.ClusterAgentContainerName,
			},
		}
	}

	return reqComp
}

const (
	PrivateActionRunnerConfigPath = "/etc/datadog-agent/privateactionrunner.yaml"
	privateActionRunnerVolumeName = "privateactionrunner-config"
)

// ManageDependencies allows a feature to manage its dependencies.
func (f *privateActionRunnerFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	// Handle Node Agent dependencies (ConfigMap for annotation-based config)
	if f.nodeEnabled {
		checksumKey, checksumValue, err := checksumAnnotation(f.nodeConfigData)
		if err != nil {
			return err
		}

		// Create ConfigMap with the config content (either from annotation or default)
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      f.getConfigMapName(),
				Namespace: f.owner.GetNamespace(),
				Annotations: map[string]string{
					checksumKey: checksumValue,
				},
			},
			Data: map[string]string{
				"privateactionrunner.yaml": f.nodeConfigData,
			},
		}

		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
			return err
		}
	}

	// Handle Cluster Agent dependencies (RBAC for secret access)
	if f.clusterConfig != nil && f.clusterConfig.Enabled {
		rbacResourcesName := getPrivateActionRunnerRbacResourcesName(f.owner)

		return managers.RBACManager().AddPolicyRules(
			f.owner.GetNamespace(),
			rbacResourcesName,
			f.clusterServiceAccountName,
			getClusterAgentRBACPolicyRules(f.clusterConfig),
		)
	}

	return nil
}

func (f *privateActionRunnerFeature) getConfigMapName() string {
	return fmt.Sprintf("%s-privateactionrunner", f.owner.GetName())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	if f.clusterConfig == nil || !f.clusterConfig.Enabled {
		return nil
	}

	managers.EnvVar().AddEnvVarToContainer(
		apicommon.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  "DD_PRIVATE_ACTION_RUNNER_ENABLED",
			Value: "true",
		},
	)

	identityUseK8sSecret := f.clusterConfig.IdentityUseK8sSecret == nil || *f.clusterConfig.IdentityUseK8sSecret
	managers.EnvVar().AddEnvVarToContainer(
		apicommon.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  "DD_PRIVATE_ACTION_RUNNER_IDENTITY_USE_K8S_SECRET",
			Value: strconv.FormatBool(identityUseK8sSecret),
		},
	)

	managers.EnvVar().AddEnvVarToContainer(
		apicommon.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL",
			Value: strconv.FormatBool(f.clusterConfig.SelfEnroll),
		},
	)

	if f.clusterConfig.IdentitySecretName != "" {
		managers.EnvVar().AddEnvVarToContainer(
			apicommon.ClusterAgentContainerName,
			&corev1.EnvVar{
				Name:  "DD_PRIVATE_ACTION_RUNNER_IDENTITY_SECRET_NAME",
				Value: f.clusterConfig.IdentitySecretName,
			},
		)
	}

	if f.clusterConfig.URN != "" {
		managers.EnvVar().AddEnvVarToContainer(
			apicommon.ClusterAgentContainerName,
			&corev1.EnvVar{
				Name:  "DD_PRIVATE_ACTION_RUNNER_URN",
				Value: f.clusterConfig.URN,
			},
		)
	}

	if f.clusterConfig.PrivateKey != "" {
		managers.EnvVar().AddEnvVarToContainer(
			apicommon.ClusterAgentContainerName,
			&corev1.EnvVar{
				Name:  "DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY",
				Value: f.clusterConfig.PrivateKey,
			},
		)
	}

	if len(f.clusterConfig.ActionsAllowlist) > 0 {
		allowlistJSON, err := json.Marshal(f.clusterConfig.ActionsAllowlist)
		if err != nil {
			return fmt.Errorf("failed to marshal actions allowlist: %w", err)
		}
		managers.EnvVar().AddEnvVarToContainer(
			apicommon.ClusterAgentContainerName,
			&corev1.EnvVar{
				Name:  "DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST",
				Value: string(allowlistJSON),
			},
		)
	}

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	if !f.nodeEnabled {
		return nil
	}

	configMapName := f.getConfigMapName()

	cmConfig := &v2alpha1.ConfigMapConfig{
		Name: configMapName,
	}
	vol := volume.GetVolumeFromConfigMap(cmConfig, configMapName, privateActionRunnerVolumeName)
	managers.Volume().AddVolume(&vol)

	volMount := corev1.VolumeMount{
		Name:      privateActionRunnerVolumeName,
		MountPath: PrivateActionRunnerConfigPath,
		SubPath:   "privateactionrunner.yaml",
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.PrivateActionRunnerContainerName)

	checksumKey, checksumValue, err := checksumAnnotation(f.nodeConfigData)
	if err != nil {
		return err
	}
	managers.Annotation().AddAnnotation(checksumKey, checksumValue)

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Private Action Runner requires separate container, not compatible with single-container mode
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	// Private Action Runner doesn't run in cluster checks runner
	return nil
}

// ManageOtelAgentGateway allows a feature to configure the OtelAgentGateway's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	// Private Action Runner doesn't run in OTel Agent Gateway
	return nil
}

// parsePrivateActionRunnerConfig parses the YAML config data and returns a PrivateActionRunnerConfig
func parsePrivateActionRunnerConfig(configData string) (*PrivateActionRunnerConfig, error) {
	config := struct {
		PrivateActionRunner *PrivateActionRunnerConfig `yaml:"private_action_runner"`
	}{
		PrivateActionRunner: &PrivateActionRunnerConfig{},
	}
	if err := yaml.Unmarshal([]byte(configData), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config data: %w", err)
	}
	return config.PrivateActionRunner, nil
}

func checksumAnnotation(configData string) (string, string, error) {
	checksum, err := comparison.GenerateMD5ForSpec(configData)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate MD5 for Private Action Runner config: %w", err)
	}
	return object.GetChecksumAnnotationKey(feature.PrivateActionRunnerIDType), checksum, nil
}
