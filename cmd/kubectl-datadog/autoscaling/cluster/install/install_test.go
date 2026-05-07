package install

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/apply"
)

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		args                     []string
		installMode              apply.InstallMode
		fargateSubnets           []string
		createKarpenterResources apply.CreateKarpenterResources
		inferenceMethod          apply.InferenceMethod
		expectError              bool
		errorContains            string
	}{
		{
			name:                     "Valid with nodes inference method",
			args:                     []string{},
			installMode:              apply.InstallModeFargate,
			createKarpenterResources: apply.CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          apply.InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with nodegroups inference method",
			args:                     []string{},
			installMode:              apply.InstallModeFargate,
			createKarpenterResources: apply.CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          apply.InferenceMethodNodeGroups,
			expectError:              false,
		},
		{
			name:                     "Valid with create none",
			args:                     []string{},
			installMode:              apply.InstallModeFargate,
			createKarpenterResources: apply.CreateKarpenterResourcesNone,
			inferenceMethod:          apply.InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with create all",
			args:                     []string{},
			installMode:              apply.InstallModeFargate,
			createKarpenterResources: apply.CreateKarpenterResourcesAll,
			inferenceMethod:          apply.InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with existing-nodes mode",
			args:                     []string{},
			installMode:              apply.InstallModeExistingNodes,
			createKarpenterResources: apply.CreateKarpenterResourcesAll,
			inferenceMethod:          apply.InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with fargate-subnets in fargate mode",
			args:                     []string{},
			installMode:              apply.InstallModeFargate,
			fargateSubnets:           []string{"subnet-abc", "subnet-def"},
			createKarpenterResources: apply.CreateKarpenterResourcesAll,
			inferenceMethod:          apply.InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Invalid with arguments",
			args:                     []string{"arg1"},
			installMode:              apply.InstallModeFargate,
			createKarpenterResources: apply.CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          apply.InferenceMethodNodes,
			expectError:              true,
			errorContains:            "no arguments are allowed",
		},
		{
			name:                     "Invalid with invalid inference method",
			args:                     []string{},
			installMode:              apply.InstallModeFargate,
			createKarpenterResources: apply.CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          apply.InferenceMethod("invalid"),
			expectError:              true,
			errorContains:            "inference-method must be one of nodes or nodegroups",
		},
		{
			name:                     "Invalid with invalid create resources",
			args:                     []string{},
			installMode:              apply.InstallModeFargate,
			createKarpenterResources: apply.CreateKarpenterResources("invalid"),
			inferenceMethod:          apply.InferenceMethodNodes,
			expectError:              true,
			errorContains:            "create-karpenter-resources must be one of none, ec2nodeclass or all",
		},
		{
			name:                     "Invalid with invalid install mode",
			args:                     []string{},
			installMode:              apply.InstallMode("invalid"),
			createKarpenterResources: apply.CreateKarpenterResourcesAll,
			inferenceMethod:          apply.InferenceMethodNodes,
			expectError:              true,
			errorContains:            "install-mode must be one of fargate or existing-nodes",
		},
		{
			name:                     "Invalid fargate-subnets with existing-nodes mode",
			args:                     []string{},
			installMode:              apply.InstallModeExistingNodes,
			fargateSubnets:           []string{"subnet-abc"},
			createKarpenterResources: apply.CreateKarpenterResourcesAll,
			inferenceMethod:          apply.InferenceMethodNodes,
			expectError:              true,
			errorContains:            "--fargate-subnets can only be used with --install-mode=fargate",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &options{
				args:                     tc.args,
				installMode:              tc.installMode,
				fargateSubnets:           tc.fargateSubnets,
				createKarpenterResources: tc.createKarpenterResources,
				inferenceMethod:          tc.inferenceMethod,
			}

			err := o.validate()

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
