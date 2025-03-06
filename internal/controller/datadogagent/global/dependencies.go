// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

func Dependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers, components feature.RequiredComponents) (errs []error) {
	// global should not be nil from defaults

	// global dependencies
	// credentials
	if err := handleCredentials(dda, manager); err != nil {
		errs = append(errs, err)
	}
	// install info
	if err := createInstallInfoConfigMap(dda, manager); err != nil {
		errs = append(errs, err)
	}

	// dca dependencies
	if components.ClusterAgent.IsEnabled() {
		// dca token
		if err := handleDCAToken(dda, manager); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func createInstallInfoConfigMap(dda metav1.Object, manager feature.ResourceManagers) error {
	installInfoCM := buildInstallInfoConfigMap(dda)
	if err := manager.Store().AddOrUpdate(kubernetes.ConfigMapKind, installInfoCM); err != nil {
		return err
	}
	return nil
}

func handleCredentials(dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers) error {
	// prioritize existing secrets
	// credentials should be non-nil from validation
	global := dda.Spec.Global
	apiKeySecretValid := isValidSecretConfig(global.Credentials.APISecret)
	appKeySecretValid := isValidSecretConfig(global.Credentials.AppSecret)

	// user defined secret(s) exist for both keys, nothing to do
	if apiKeySecretValid && appKeySecretValid {
		return nil
	}

	// secret should be created for at least one key
	secretName := secrets.GetDefaultCredentialsSecretName(dda)
	// create api key secret
	if !apiKeySecretValid {
		if global.Credentials.APIKey == nil || *global.Credentials.APIKey == "" {
			return fmt.Errorf("api key must be set")
		}
		if err := manager.SecretManager().AddSecret(dda.Namespace, secretName, v2alpha1.DefaultAPIKeyKey, *global.Credentials.APIKey); err != nil {
			return err
		}
	}

	// create app key secret
	if !appKeySecretValid {
		if global.Credentials.AppKey == nil || *global.Credentials.AppKey == "" {
			return fmt.Errorf("app key must be set")
		}
		if err := manager.SecretManager().AddSecret(dda.Namespace, secretName, v2alpha1.DefaultAPPKeyKey, *global.Credentials.AppKey); err != nil {
			return err
		}
	}

	return nil
}

func handleDCAToken(dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers) error {
	global := dda.Spec.Global
	token := ""

	// prioritize existing secret
	if isValidSecretConfig(global.ClusterAgentTokenSecret) {
		return nil
	}

	// user specifies token
	if global.ClusterAgentToken != nil && *global.ClusterAgentToken != "" {
		token = *global.ClusterAgentToken
	} else {
		// no token specified. generate
		token = apiutils.GenerateRandomString(32)
	}

	// create secret
	secretName := secrets.GetDefaultDCATokenSecretName(dda)
	if err := manager.SecretManager().AddSecret(dda.Namespace, secretName, common.DefaultTokenKey, token); err != nil {
		return err
	}

	return nil
}
