// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
)

func TestRcLocalKindSuite(t *testing.T) {

	operatorOpts := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues("installCRDs: false"),
		// Remote configuration requires API, app keys, cluster name and site
		operatorparams.WithHelmValues("remoteConfiguration.enabled: true"),
		operatorparams.WithHelmValues("apiKeyExistingSecret: dda-datadog-credentials"),
		operatorparams.WithHelmValues("appKeyExistingSecret: dda-datadog-credentials"),
		operatorparams.WithHelmValues("clusterName: rc-updater-e2e-test-cluster"),
		operatorparams.WithHelmValues("site: datadoghq.com"),
	}

	ddaManifest := filepath.Join(common.ManifestsPath, "datadog-agent-rc-updater.yaml")
	ddaConfigPath, _ := common.GetAbsPath(ddaManifest)

	ddaOpts := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
		agentwithoperatorparams.WithTLSKubeletVerify(false),
		agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
			Name:         "datadog-agent-rc-updater",
			YamlFilePath: ddaConfigPath,
		}),
	}

	provisionerOpts := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(operatorOpts...),
		provisioners.WithDDAOptions(ddaOpts...),
	}

	e2eParams := []e2e.SuiteOption{
		e2e.WithStackName("rc-updater-e2e-test-cluster"),
		// e2e.WithDevMode(),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisioners.LocalKindRunFunc, provisionerOpts...)),
	}

	t.Parallel()

	e2e.Run(t, &updaterSuite{clusterName: "rc-updater-e2e-test-cluster"}, e2eParams...)
}
