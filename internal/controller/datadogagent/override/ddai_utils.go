// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

func SetOverrideFromDDA(dda *v2alpha1.DatadogAgent, ddaiSpec *v2alpha1.DatadogAgentSpec) {
	if ddaiSpec == nil {
		ddaiSpec = &v2alpha1.DatadogAgentSpec{}
	}
	if ddaiSpec.Override == nil {
		ddaiSpec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)
	}
	if _, ok := ddaiSpec.Override[v2alpha1.NodeAgentComponentName]; !ok {
		ddaiSpec.Override[v2alpha1.NodeAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{}
	}
	if ddaiSpec.Override[v2alpha1.NodeAgentComponentName].Labels == nil {
		ddaiSpec.Override[v2alpha1.NodeAgentComponentName].Labels = make(map[string]string)
	}
	// Set empty provider label
	ddaiSpec.Override[v2alpha1.NodeAgentComponentName].Labels[constants.MD5AgentDeploymentProviderLabelKey] = ""

	// Add checksum annotation to the components (nodeAgent, clusterAgent, clusterChecksRunner) pod templates if the cluster agent token is set in DDA spec
	// This is used to trigger a redeployment of the components when the cluster agent token is changed
	// This is needed as DDAI are always using a DCA token secret, while the DDA can use a token from a secret or a literal value
	if shouldAddDCATokenChecksumAnnotation(dda) {
		token := apiutils.StringValue(dda.Spec.Global.ClusterAgentToken)
		hash, _ := comparison.GenerateMD5ForSpec(map[string]string{common.DefaultTokenKey: token})
		if ddaiSpec.Override[v2alpha1.NodeAgentComponentName].Annotations == nil {
			ddaiSpec.Override[v2alpha1.NodeAgentComponentName].Annotations = make(map[string]string)
		}
		ddaiSpec.Override[v2alpha1.NodeAgentComponentName].Annotations[global.GetDCATokenChecksumAnnotationKey()] = hash

		if _, ok := ddaiSpec.Override[v2alpha1.ClusterAgentComponentName]; !ok {
			ddaiSpec.Override[v2alpha1.ClusterAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{}
		}
		if ddaiSpec.Override[v2alpha1.ClusterAgentComponentName].Annotations == nil {
			ddaiSpec.Override[v2alpha1.ClusterAgentComponentName].Annotations = make(map[string]string)
		}
		ddaiSpec.Override[v2alpha1.ClusterAgentComponentName].Annotations[global.GetDCATokenChecksumAnnotationKey()] = hash

		if _, ok := ddaiSpec.Override[v2alpha1.ClusterChecksRunnerComponentName]; !ok {
			ddaiSpec.Override[v2alpha1.ClusterChecksRunnerComponentName] = &v2alpha1.DatadogAgentComponentOverride{}
		}
		if ddaiSpec.Override[v2alpha1.ClusterChecksRunnerComponentName].Annotations == nil {
			ddaiSpec.Override[v2alpha1.ClusterChecksRunnerComponentName].Annotations = make(map[string]string)
		}
		ddaiSpec.Override[v2alpha1.ClusterChecksRunnerComponentName].Annotations[global.GetDCATokenChecksumAnnotationKey()] = hash
	}
}

// shouldAddDCATokenChecksumAnnotation checks if the cluster agent token is set as a literal value without a secret.
// If the token is set as a secret, we do not add the annotation.
func shouldAddDCATokenChecksumAnnotation(dda *v2alpha1.DatadogAgent) bool {
	return !global.IsValidSecretConfig(dda.Spec.Global.ClusterAgentTokenSecret) && apiutils.StringValue(dda.Spec.Global.ClusterAgentToken) != ""
}
