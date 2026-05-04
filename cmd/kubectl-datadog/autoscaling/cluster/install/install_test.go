package install

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
)

func TestInstallMode_String(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     InstallMode
		expected string
	}{
		{
			name:     "Fargate mode",
			mode:     InstallModeFargate,
			expected: "fargate",
		},
		{
			name:     "Existing-nodes mode",
			mode:     InstallModeExistingNodes,
			expected: "existing-nodes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.mode.String())
		})
	}
}

func TestInstallMode_Set(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       string
		expected    InstallMode
		expectError bool
	}{
		{
			name:        "Set to fargate",
			input:       "fargate",
			expected:    InstallModeFargate,
			expectError: false,
		},
		{
			name:        "Set to existing-nodes",
			input:       "existing-nodes",
			expected:    InstallModeExistingNodes,
			expectError: false,
		},
		{
			name:        "Invalid value",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "Empty value",
			input:       "",
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mode InstallMode
			err := mode.Set(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "install-mode must be one of fargate or existing-nodes")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, mode)
			}
		})
	}
}

func TestInstallMode_Type(t *testing.T) {
	var mode InstallMode
	assert.Equal(t, "InstallMode", mode.Type())
}

func TestCreateKarpenterResources_String(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resource CreateKarpenterResources
		expected string
	}{
		{
			name:     "None resources",
			resource: CreateKarpenterResourcesNone,
			expected: "none",
		},
		{
			name:     "EC2NodeClass only",
			resource: CreateKarpenterResourcesEC2NodeClass,
			expected: "ec2nodeclass",
		},
		{
			name:     "All resources",
			resource: CreateKarpenterResourcesAll,
			expected: "all",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.resource.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCreateKarpenterResources_Set(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       string
		expected    CreateKarpenterResources
		expectError bool
	}{
		{
			name:        "Set to none",
			input:       "none",
			expected:    CreateKarpenterResourcesNone,
			expectError: false,
		},
		{
			name:        "Set to ec2nodeclass",
			input:       "ec2nodeclass",
			expected:    CreateKarpenterResourcesEC2NodeClass,
			expectError: false,
		},
		{
			name:        "Set to all",
			input:       "all",
			expected:    CreateKarpenterResourcesAll,
			expectError: false,
		},
		{
			name:        "Invalid value",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "Empty value",
			input:       "",
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resource CreateKarpenterResources
			err := resource.Set(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "create-karpenter-resources must be one of none, ec2nodeclass or all")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, resource)
			}
		})
	}
}

func TestCreateKarpenterResources_Type(t *testing.T) {
	var resource CreateKarpenterResources
	assert.Equal(t, "CreateKarpenterResources", resource.Type())
}

func TestInferenceMethod_String(t *testing.T) {
	for _, tc := range []struct {
		name     string
		method   InferenceMethod
		expected string
	}{
		{
			name:     "Nodes method",
			method:   InferenceMethodNodes,
			expected: "nodes",
		},
		{
			name:     "NodeGroups method",
			method:   InferenceMethodNodeGroups,
			expected: "nodegroups",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.method.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestInferenceMethod_Set(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       string
		expected    InferenceMethod
		expectError bool
	}{
		{
			name:        "Set to nodes",
			input:       "nodes",
			expected:    InferenceMethodNodes,
			expectError: false,
		},
		{
			name:        "Set to nodegroups",
			input:       "nodegroups",
			expected:    InferenceMethodNodeGroups,
			expectError: false,
		},
		{
			name:        "Invalid value",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "Empty value",
			input:       "",
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var method InferenceMethod
			err := method.Set(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "inference-method must be one of nodes or nodegroups")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, method)
			}
		})
	}
}

func TestInferenceMethod_Type(t *testing.T) {
	var method InferenceMethod
	assert.Equal(t, "InferenceMethod", method.Type())
}

func TestKarpenterHelmValues(t *testing.T) {
	t.Run("existing-nodes mode carries Datadog ownership labels and no IRSA annotation", func(t *testing.T) {
		values := karpenterHelmValues("my-cluster", InstallModeExistingNodes, "")

		labels, ok := values["additionalLabels"].(map[string]any)
		require.True(t, ok, "additionalLabels must be a map")
		assert.Equal(t, guess.InstalledByValue, labels[guess.InstalledByLabel],
			"installed-by sentinel must match what FindKarpenterInstallation looks for")
		assert.Contains(t, labels, guess.InstallerVersionLabel)

		settings, ok := values["settings"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "my-cluster", settings["clusterName"])
		assert.Equal(t, "my-cluster", settings["interruptionQueue"])

		assert.NotContains(t, values, "serviceAccount",
			"existing-nodes mode must not annotate the ServiceAccount with an IRSA role")
	})

	t.Run("fargate mode annotates the ServiceAccount with the IRSA role ARN", func(t *testing.T) {
		const arn = "arn:aws:iam::123456789012:role/dd-karpenter"
		values := karpenterHelmValues("my-cluster", InstallModeFargate, arn)

		serviceAccount, ok := values["serviceAccount"].(map[string]any)
		require.True(t, ok, "fargate mode must populate serviceAccount values")
		annotations, ok := serviceAccount["annotations"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, arn, annotations["eks.amazonaws.com/role-arn"])
	})
}

func TestDisplayForeignKarpenterMessage(t *testing.T) {
	// browser.OpenURL spawns xdg-open with non-*os.File writers, which makes
	// `cmd.Wait` hang on the pipe-copy goroutine until xdg-open's descendants
	// all close the write side. Empty PATH makes the LookPath probe fail and
	// browser.OpenURL returns ErrNotFound without spawning anything.
	t.Setenv("PATH", "")

	out := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	foreign := &guess.KarpenterInstallation{Namespace: "karpenter", Name: "karpenter"}
	err := displayForeignKarpenterMessage(cmd, "my-cluster", foreign)
	require.NoError(t, err, "foreign Karpenter is a successful no-op, not an error")

	rendered := out.String()
	assert.Contains(t, rendered, "Karpenter is already installed on cluster my-cluster")
	assert.Contains(t, rendered, "Deployment karpenter/karpenter.",
		"the message must surface the foreign install's namespace/name so the user can locate it")
	assert.Contains(t, rendered, "kubectl-datadog has nothing to install.")
	assert.Contains(t, rendered, "Autoscaling settings page")
	assert.Contains(t, rendered, "kube_cluster_name%3Amy-cluster",
		"the linked URL must point at the cluster's autoscaling settings")
}

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
