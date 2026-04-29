package install

import (
	"bytes"
	"io"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
)

// stackFixture builds a fake aws.Stack with the requested install-mode tag and
// parameters so check helpers and DetectedInstallMode can be unit-tested
// without an AWS client.
func stackFixture(installModeTag string, params map[string]string) *aws.Stack {
	stack := &cfntypes.Stack{
		StackName: awssdk.String("dd-karpenter-mycluster-dd-karpenter"),
	}
	if installModeTag != "" {
		stack.Tags = append(stack.Tags, cfntypes.Tag{
			Key:   awssdk.String(InstallModeTagKey),
			Value: awssdk.String(installModeTag),
		})
	}
	for k, v := range params {
		stack.Parameters = append(stack.Parameters, cfntypes.Parameter{
			ParameterKey:   awssdk.String(k),
			ParameterValue: awssdk.String(v),
		})
	}
	return &aws.Stack{Stack: stack}
}

// noopStreams returns IOStreams that drop all output, for tests that only
// exercise pure logic and do not need to assert on rendered text.
func noopStreams() genericclioptions.IOStreams {
	return genericclioptions.IOStreams{
		In:     io.NopCloser(nil),
		Out:    io.Discard,
		ErrOut: io.Discard,
	}
}

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
			"installed-by sentinel must match what FindForeignKarpenterInstallation looks for")
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
	streams := genericclioptions.IOStreams{
		Out:    out,
		ErrOut: &bytes.Buffer{},
	}

	foreign := &guess.ForeignKarpenter{Namespace: "karpenter", Name: "karpenter"}
	err := displayForeignKarpenterMessage(streams, "my-cluster", foreign)
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

func TestKarpenterStackName(t *testing.T) {
	// The mode-independent stack name is what uninstall and update both rely
	// on to find the install — pin it so a rename does not silently break
	// either command.
	assert.Equal(t, "dd-karpenter-prod-eu-karpenter", KarpenterStackName("prod-eu"))
}

func TestDDKarpenterStackName(t *testing.T) {
	// Same reasoning as TestKarpenterStackName — the dd-karpenter (mode-
	// specific) stack name is the source of truth for update's install-mode
	// auto-detection, so the format is part of the contract with previously
	// installed clusters.
	assert.Equal(t, "dd-karpenter-prod-eu-dd-karpenter", DDKarpenterStackName("prod-eu"))
}

func TestDetectedInstallMode(t *testing.T) {
	t.Run("fargate tag returns InstallModeFargate", func(t *testing.T) {
		stack := stackFixture("fargate", nil)
		assert.Equal(t, InstallModeFargate, DetectedInstallMode(stack))
	})

	t.Run("existing-nodes tag returns InstallModeExistingNodes", func(t *testing.T) {
		stack := stackFixture("existing-nodes", nil)
		assert.Equal(t, InstallModeExistingNodes, DetectedInstallMode(stack))
	})

	t.Run("missing tag defaults to existing-nodes for legacy stacks", func(t *testing.T) {
		// Stacks created before the install-mode tag was introduced predate
		// the Fargate mode entirely, so existing-nodes is the correct
		// fallback.
		stack := stackFixture("", nil)
		assert.Equal(t, InstallModeExistingNodes, DetectedInstallMode(stack))
	})

	t.Run("unknown tag value is surfaced verbatim so callers can reject it", func(t *testing.T) {
		// DetectedInstallMode is a pure decoder — it does not validate the
		// tag value. Callers (update.resolveOptions) inspect the returned
		// mode and reject unsupported values with a clear error.
		stack := stackFixture("custom", nil)
		assert.Equal(t, InstallMode("custom"), DetectedInstallMode(stack))
	})
}

func TestCheckInstallModeTag(t *testing.T) {
	t.Run("nil stack accepts any mode (fresh install)", func(t *testing.T) {
		require.NoError(t, checkInstallModeTag(nil, InstallModeFargate))
		require.NoError(t, checkInstallModeTag(nil, InstallModeExistingNodes))
	})

	t.Run("matching mode is accepted", func(t *testing.T) {
		stack := stackFixture("fargate", nil)
		require.NoError(t, checkInstallModeTag(stack, InstallModeFargate))
	})

	t.Run("mismatched mode is rejected with explanatory error", func(t *testing.T) {
		stack := stackFixture("fargate", nil)
		err := checkInstallModeTag(stack, InstallModeExistingNodes)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fargate")
		assert.Contains(t, err.Error(), "existing-nodes")
		assert.Contains(t, err.Error(), "uninstall")
	})

	t.Run("legacy untagged stack matches existing-nodes", func(t *testing.T) {
		// Symmetric with TestDetectedInstallMode — a legacy stack must let
		// existing-nodes installs proceed without forcing an uninstall.
		stack := stackFixture("", nil)
		require.NoError(t, checkInstallModeTag(stack, InstallModeExistingNodes))
	})
}

func TestCheckFargateStackImmutability(t *testing.T) {
	t.Run("nil stack accepts anything (fresh install)", func(t *testing.T) {
		require.NoError(t, checkFargateStackImmutability(nil, "dd-karpenter", []string{"subnet-a"}))
	})

	t.Run("matching namespace and subnets pass", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a,subnet-b",
		})
		require.NoError(t, checkFargateStackImmutability(stack, "dd-karpenter", []string{"subnet-a", "subnet-b"}))
	})

	t.Run("mismatched namespace is rejected", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a",
		})
		err := checkFargateStackImmutability(stack, "other-ns", []string{"subnet-a"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "KarpenterNamespace=dd-karpenter")
		assert.Contains(t, err.Error(), "uninstall")
	})

	t.Run("mismatched subnets are rejected", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a,subnet-b",
		})
		err := checkFargateStackImmutability(stack, "dd-karpenter", []string{"subnet-c"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "FargateSubnets=subnet-a,subnet-b")
	})
}

func TestNew(t *testing.T) {
	// Building the cobra command exercises the flag-registration block in
	// New, which is otherwise unreachable from validate-only tests but is
	// the part most likely to silently break a user's command line on
	// rename.
	cmd := New(noopStreams())
	require.NotNil(t, cmd)
	assert.Equal(t, "install", cmd.Use)

	for _, name := range []string{
		"cluster-name",
		"karpenter-namespace",
		"karpenter-version",
		"install-mode",
		"fargate-subnets",
		"create-karpenter-resources",
		"inference-method",
		"debug",
	} {
		assert.NotNilf(t, cmd.Flags().Lookup(name), "--%s flag must be registered", name)
	}
}

func TestComplete(t *testing.T) {
	// complete just stashes the positional args and delegates to common.Init,
	// which is exercised by the common package — pin the args plumbing so a
	// future refactor cannot silently swallow them.
	o := newOptions(noopStreams())
	cmd := New(noopStreams())
	require.NoError(t, o.complete(cmd, []string{"x", "y"}))
	assert.Equal(t, []string{"x", "y"}, o.args)
}

func TestNewOptionsDefaults(t *testing.T) {
	// install's default --create-karpenter-resources=all is the key UX
	// difference vs update (none); pin it so the contract does not silently
	// drift.
	o := newOptions(noopStreams())
	assert.Equal(t, InstallModeFargate, o.installMode)
	assert.Equal(t, CreateKarpenterResourcesAll, o.createKarpenterResources,
		"install must default to --create-karpenter-resources=all so a fresh install lays down EC2NodeClass and NodePool resources")
	assert.Equal(t, InferenceMethodNodeGroups, o.inferenceMethod)
}

func TestDisplayEKSAutoModeMessage(t *testing.T) {
	// Empty PATH neutralises browser.OpenURL — see TestDisplayForeignKarpenterMessage.
	t.Setenv("PATH", "")

	out := &bytes.Buffer{}
	streams := genericclioptions.IOStreams{
		Out:    out,
		ErrOut: &bytes.Buffer{},
	}

	require.NoError(t, displayEKSAutoModeMessage(streams, "my-cluster"),
		"EKS auto-mode is a successful no-op, not an error")

	rendered := out.String()
	assert.Contains(t, rendered, "EKS auto-mode is already active on cluster my-cluster")
	assert.Contains(t, rendered, "Karpenter is built into EKS auto-mode")
	assert.Contains(t, rendered, "kube_cluster_name%3Amy-cluster",
		"the linked URL must point at the cluster's autoscaling settings")
}

func TestDisplaySuccessMessage(t *testing.T) {
	// Empty PATH neutralises browser.OpenURL — see TestDisplayForeignKarpenterMessage.
	t.Setenv("PATH", "")

	t.Run("none mode advertises the partial-config follow-up flags", func(t *testing.T) {
		out := &bytes.Buffer{}
		streams := genericclioptions.IOStreams{Out: out, ErrOut: &bytes.Buffer{}}

		require.NoError(t, displaySuccessMessage(streams, "my-cluster", CreateKarpenterResourcesNone))

		rendered := out.String()
		assert.Contains(t, rendered, "partially configured")
		assert.Contains(t, rendered, "--create-karpenter-resources=ec2nodeclass")
		assert.Contains(t, rendered, "kube_cluster_name%3Amy-cluster")
	})

	t.Run("ec2nodeclass and all modes share the ready message", func(t *testing.T) {
		for _, mode := range []CreateKarpenterResources{CreateKarpenterResourcesEC2NodeClass, CreateKarpenterResourcesAll} {
			out := &bytes.Buffer{}
			streams := genericclioptions.IOStreams{Out: out, ErrOut: &bytes.Buffer{}}

			require.NoError(t, displaySuccessMessage(streams, "my-cluster", mode))

			rendered := out.String()
			assert.Contains(t, rendered, "ready to be enabled", "mode=%s", mode)
			assert.Contains(t, rendered, "kube_cluster_name%3Amy-cluster", "mode=%s", mode)
		}
	})
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
