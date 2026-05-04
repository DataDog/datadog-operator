package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		args                     []string
		installMode              InstallMode
		fargateSubnets           []string
		createKarpenterResources CreateKarpenterResources
		inferenceMethod          InferenceMethod
		expectError              bool
		errorContains            string
	}{
		{
			name:                     "Valid with nodes inference method",
			args:                     []string{},
			installMode:              InstallModeFargate,
			createKarpenterResources: CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with nodegroups inference method",
			args:                     []string{},
			installMode:              InstallModeFargate,
			createKarpenterResources: CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          InferenceMethodNodeGroups,
			expectError:              false,
		},
		{
			name:                     "Valid with create none",
			args:                     []string{},
			installMode:              InstallModeFargate,
			createKarpenterResources: CreateKarpenterResourcesNone,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with create all",
			args:                     []string{},
			installMode:              InstallModeFargate,
			createKarpenterResources: CreateKarpenterResourcesAll,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with existing-nodes mode",
			args:                     []string{},
			installMode:              InstallModeExistingNodes,
			createKarpenterResources: CreateKarpenterResourcesAll,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Valid with fargate-subnets in fargate mode",
			args:                     []string{},
			installMode:              InstallModeFargate,
			fargateSubnets:           []string{"subnet-abc", "subnet-def"},
			createKarpenterResources: CreateKarpenterResourcesAll,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              false,
		},
		{
			name:                     "Invalid with arguments",
			args:                     []string{"arg1"},
			installMode:              InstallModeFargate,
			createKarpenterResources: CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              true,
			errorContains:            "no arguments are allowed",
		},
		{
			name:                     "Invalid with invalid inference method",
			args:                     []string{},
			installMode:              InstallModeFargate,
			createKarpenterResources: CreateKarpenterResourcesEC2NodeClass,
			inferenceMethod:          InferenceMethod("invalid"),
			expectError:              true,
			errorContains:            "inference-method must be one of nodes or nodegroups",
		},
		{
			name:                     "Invalid with invalid create resources",
			args:                     []string{},
			installMode:              InstallModeFargate,
			createKarpenterResources: CreateKarpenterResources("invalid"),
			inferenceMethod:          InferenceMethodNodes,
			expectError:              true,
			errorContains:            "create-karpenter-resources must be one of none, ec2nodeclass or all",
		},
		{
			name:                     "Invalid with invalid install mode",
			args:                     []string{},
			installMode:              InstallMode("invalid"),
			createKarpenterResources: CreateKarpenterResourcesAll,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              true,
			errorContains:            "install-mode must be one of fargate or existing-nodes",
		},
		{
			name:                     "Invalid fargate-subnets with existing-nodes mode",
			args:                     []string{},
			installMode:              InstallModeExistingNodes,
			fargateSubnets:           []string{"subnet-abc"},
			createKarpenterResources: CreateKarpenterResourcesAll,
			inferenceMethod:          InferenceMethodNodes,
			expectError:              true,
			errorContains:            "--fargate-subnets can only be used with --install-mode=fargate",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Save and restore the global variables
			oldMode := installMode
			oldSubnets := fargateSubnets
			oldCreate := createKarpenterResources
			oldMethod := inferenceMethod
			installMode = tc.installMode
			fargateSubnets = tc.fargateSubnets
			createKarpenterResources = tc.createKarpenterResources
			inferenceMethod = tc.inferenceMethod
			defer func() {
				installMode = oldMode
				fargateSubnets = oldSubnets
				createKarpenterResources = oldCreate
				inferenceMethod = oldMethod
			}()

			o := &options{
				args: tc.args,
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
