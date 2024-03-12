// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	ddaExamplesPath     = "../../examples/datadogagent/v2alpha1"
	mgrKustomizeDirPath = "../../config/default"
)

var (
	k8sVersion string
	ddaConfig  string
	imageTag   string

	ddaMinimalPath = filepath.Join(ddaExamplesPath, "datadog-agent-minimum.yaml")

	namespace = strings.ToLower(fmt.Sprintf("kind-e2e-%s", apiutils.GenerateRandomString(8)))
)

// getAbsPath Return absolute path for given path
func getAbsPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	_, err = os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if os.IsNotExist(err) {
		return "", err
	}

	return absPath, nil
}

func operatorTransformationFunc() func(state map[string]interface{}, opts ...pulumi.ResourceOption) {
	return func(state map[string]interface{}, opts ...pulumi.ResourceOption) {
		name := state["metadata"].(map[string]interface{})["name"]

		if state["kind"] == "Namespace" {
			state["metadata"].(map[string]interface{})["name"] = namespace
		}
		if _, ok := state["metadata"].(map[string]interface{})["namespace"]; ok {
			state["metadata"].(map[string]interface{})["namespace"] = namespace
		}
		if state["kind"] == "ClusterRoleBinding" {
			subjects := state["subjects"].([]interface{})
			subject := subjects[0].(map[string]interface{})
			subject["namespace"] = namespace
		}
		if imageTag != "" && state["kind"] == "Deployment" && name == "datadog-operator-manager" {
			template := state["spec"].(map[string]interface{})["template"]
			templateSpec := template.(map[string]interface{})["spec"]
			containers := templateSpec.(map[string]interface{})["containers"]
			container := containers.([]interface{})[0]
			container.(map[string]interface{})["image"] = imageTag

		}
	}
}

func ddaTransformationFunc(clusterName string, apiKey pulumi.StringOutput) func(state map[string]interface{}, opts ...pulumi.ResourceOption) {
	return func(state map[string]interface{}, opts ...pulumi.ResourceOption) {
		state["metadata"].(map[string]interface{})["namespace"] = namespace
		spec := state["spec"].(map[string]interface{})
		global := spec["global"].(map[string]interface{})
		global["credentials"].(map[string]interface{})["apiKey"] = apiKey
		global["clusterName"] = clusterName
		global["kubelet"] = map[string]interface{}{
			"tlsVerify": false,
		}
		spec["override"] = map[string]interface{}{
			"nodeAgent": map[string]interface{}{
				"containers": map[string]interface{}{
					"agent": map[string]interface{}{
						"env": []map[string]interface{}{
							{
								"name":  "DD_SKIP_SSL_VALIDATION",
								"value": pulumi.String("true"),
							},
						},
					},
				},
			},
		}
	}
}
