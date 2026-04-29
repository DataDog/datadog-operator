package update

import (
	"io"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install"
)

// noopStreams returns IOStreams that drop all output. resolveOptions and
// validate never write to streams, but newOptions stores them on the struct.
func noopStreams() genericclioptions.IOStreams {
	return genericclioptions.IOStreams{
		In:     io.NopCloser(nil),
		Out:    io.Discard,
		ErrOut: io.Discard,
	}
}

// stackFixture builds a fake aws.Stack for resolveOptions tests.
func stackFixture(installModeTag string, params map[string]string) *aws.Stack {
	stack := &cfntypes.Stack{
		StackName: awssdk.String("dd-karpenter-mycluster-dd-karpenter"),
	}
	if installModeTag != "" {
		stack.Tags = append(stack.Tags, cfntypes.Tag{
			Key:   awssdk.String(install.InstallModeTagKey),
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

// changedFlags returns a `changed` predicate that reports the listed flag
// names as user-set and everything else as default.
func changedFlags(names ...string) func(string) bool {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return func(name string) bool {
		_, ok := set[name]
		return ok
	}
}

func TestResolveOptions(t *testing.T) {
	t.Run("fargate stack with no user flags fills options from CFN parameters", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-b,subnet-a",
		})
		o := newOptions(noopStreams())

		opts, err := o.resolveOptions(changedFlags(), "mycluster", stack)

		require.NoError(t, err)
		assert.Equal(t, "mycluster", opts.ClusterName)
		assert.Equal(t, "dd-karpenter", opts.KarpenterNamespace)
		assert.Equal(t, install.InstallModeFargate, opts.InstallMode)
		assert.Equal(t, []string{"subnet-a", "subnet-b"}, opts.FargateSubnets,
			"FargateSubnets must be normalised (sorted) so install.Run's immutability check passes")
	})

	t.Run("existing-nodes stack with no user flags fills options from CFN parameters", func(t *testing.T) {
		stack := stackFixture("existing-nodes", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
		})
		o := newOptions(noopStreams())

		opts, err := o.resolveOptions(changedFlags(), "mycluster", stack)

		require.NoError(t, err)
		assert.Equal(t, install.InstallModeExistingNodes, opts.InstallMode)
		assert.Empty(t, opts.FargateSubnets, "existing-nodes mode must not surface FargateSubnets")
	})

	t.Run("legacy stack with no install-mode tag is treated as existing-nodes", func(t *testing.T) {
		stack := stackFixture("", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
		})
		o := newOptions(noopStreams())

		opts, err := o.resolveOptions(changedFlags(), "mycluster", stack)

		require.NoError(t, err)
		assert.Equal(t, install.InstallModeExistingNodes, opts.InstallMode,
			"untagged stacks predate the install-mode tag and must default to existing-nodes")
	})

	t.Run("missing KarpenterNamespace parameter is reported as inconsistent state", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{})
		o := newOptions(noopStreams())

		_, err := o.resolveOptions(changedFlags(), "mycluster", stack)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no KarpenterNamespace parameter")
	})

	t.Run("matching --karpenter-namespace flag passes through", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a,subnet-b",
		})
		o := newOptions(noopStreams())
		o.karpenterNamespace = "dd-karpenter"

		opts, err := o.resolveOptions(changedFlags("karpenter-namespace"), "mycluster", stack)

		require.NoError(t, err)
		assert.Equal(t, "dd-karpenter", opts.KarpenterNamespace)
	})

	t.Run("contradicting --karpenter-namespace is rejected with explanatory error", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a",
		})
		o := newOptions(noopStreams())
		o.karpenterNamespace = "other-ns"

		_, err := o.resolveOptions(changedFlags("karpenter-namespace"), "mycluster", stack)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "--karpenter-namespace=other-ns")
		assert.Contains(t, err.Error(), "dd-karpenter")
		assert.Contains(t, err.Error(), "uninstall")
	})

	t.Run("matching --install-mode flag passes through", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a",
		})
		o := newOptions(noopStreams())
		o.installMode = install.InstallModeFargate

		opts, err := o.resolveOptions(changedFlags("install-mode"), "mycluster", stack)

		require.NoError(t, err)
		assert.Equal(t, install.InstallModeFargate, opts.InstallMode)
	})

	t.Run("contradicting --install-mode is rejected with explanatory error", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a",
		})
		o := newOptions(noopStreams())
		o.installMode = install.InstallModeExistingNodes

		_, err := o.resolveOptions(changedFlags("install-mode"), "mycluster", stack)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "--install-mode=existing-nodes")
		assert.Contains(t, err.Error(), "fargate")
	})

	t.Run("matching --fargate-subnets in any order passes through", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a,subnet-b",
		})
		o := newOptions(noopStreams())
		o.fargateSubnets = []string{"subnet-b", "subnet-a"}

		opts, err := o.resolveOptions(changedFlags("fargate-subnets"), "mycluster", stack)

		require.NoError(t, err)
		assert.Equal(t, []string{"subnet-b", "subnet-a"}, opts.FargateSubnets,
			"the user-provided order is preserved on the way to install.Run; the comparison is order-independent")
	})

	t.Run("contradicting --fargate-subnets is rejected", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a,subnet-b",
		})
		o := newOptions(noopStreams())
		o.fargateSubnets = []string{"subnet-c"}

		_, err := o.resolveOptions(changedFlags("fargate-subnets"), "mycluster", stack)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "--fargate-subnets")
	})

	t.Run("--fargate-subnets is rejected when the install is in existing-nodes mode", func(t *testing.T) {
		stack := stackFixture("existing-nodes", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
		})
		o := newOptions(noopStreams())
		o.fargateSubnets = []string{"subnet-a"}

		_, err := o.resolveOptions(changedFlags("fargate-subnets"), "mycluster", stack)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "--fargate-subnets can only be used with --install-mode=fargate")
	})

	t.Run("mutable flags are forwarded as-is", func(t *testing.T) {
		stack := stackFixture("fargate", map[string]string{
			"KarpenterNamespace": "dd-karpenter",
			"FargateSubnets":     "subnet-a",
		})
		o := newOptions(noopStreams())
		o.karpenterVersion = "1.2.3"
		o.createKarpenterResources = install.CreateKarpenterResourcesAll
		o.inferenceMethod = install.InferenceMethodNodes
		o.debug = true

		opts, err := o.resolveOptions(changedFlags(), "mycluster", stack)

		require.NoError(t, err)
		assert.Equal(t, "1.2.3", opts.KarpenterVersion)
		assert.Equal(t, install.CreateKarpenterResourcesAll, opts.CreateKarpenterResources)
		assert.Equal(t, install.InferenceMethodNodes, opts.InferenceMethod)
		assert.True(t, opts.Debug)
	})
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		args                     []string
		installMode              install.InstallMode
		createKarpenterResources install.CreateKarpenterResources
		inferenceMethod          install.InferenceMethod
		errorContains            string
	}{
		{
			name:                     "default options are valid",
			installMode:              install.InstallModeFargate,
			createKarpenterResources: install.CreateKarpenterResourcesNone,
			inferenceMethod:          install.InferenceMethodNodeGroups,
		},
		{
			name:                     "existing-nodes mode is valid",
			installMode:              install.InstallModeExistingNodes,
			createKarpenterResources: install.CreateKarpenterResourcesAll,
			inferenceMethod:          install.InferenceMethodNodes,
		},
		{
			name:                     "positional args are rejected",
			args:                     []string{"unexpected"},
			installMode:              install.InstallModeFargate,
			createKarpenterResources: install.CreateKarpenterResourcesNone,
			inferenceMethod:          install.InferenceMethodNodeGroups,
			errorContains:            "no arguments are allowed",
		},
		{
			name:                     "invalid install-mode is rejected",
			installMode:              install.InstallMode("nope"),
			createKarpenterResources: install.CreateKarpenterResourcesNone,
			inferenceMethod:          install.InferenceMethodNodeGroups,
			errorContains:            "install-mode must be one of",
		},
		{
			name:                     "invalid create-karpenter-resources is rejected",
			installMode:              install.InstallModeFargate,
			createKarpenterResources: install.CreateKarpenterResources("nope"),
			inferenceMethod:          install.InferenceMethodNodeGroups,
			errorContains:            "create-karpenter-resources must be one of",
		},
		{
			name:                     "invalid inference-method is rejected",
			installMode:              install.InstallModeFargate,
			createKarpenterResources: install.CreateKarpenterResourcesNone,
			inferenceMethod:          install.InferenceMethod("nope"),
			errorContains:            "inference-method must be one of",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &options{
				args:                     tc.args,
				installMode:              tc.installMode,
				createKarpenterResources: tc.createKarpenterResources,
				inferenceMethod:          tc.inferenceMethod,
			}

			err := o.validate()

			if tc.errorContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewOptionsDefaults(t *testing.T) {
	// The default for --create-karpenter-resources is the key UX difference
	// between install (all) and update (none) — pin it so the contract does
	// not silently drift.
	o := newOptions(noopStreams())
	assert.Equal(t, install.CreateKarpenterResourcesNone, o.createKarpenterResources,
		"update must default to --create-karpenter-resources=none to avoid overwriting hand-edited resources")
	assert.Equal(t, install.InstallModeFargate, o.installMode)
	assert.Equal(t, install.InferenceMethodNodeGroups, o.inferenceMethod)
}
