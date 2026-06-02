package update

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/apply"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
)

// stackFixture builds a *aws.Stack out of two maps so each test case can
// declare the parameters and tags it cares about without boilerplate.
func stackFixture(params, tags map[string]string) *aws.Stack {
	s := &cfntypes.Stack{StackName: awssdk.String("test-stack")}
	for k, v := range params {
		s.Parameters = append(s.Parameters, cfntypes.Parameter{
			ParameterKey:   awssdk.String(k),
			ParameterValue: awssdk.String(v),
		})
	}
	for k, v := range tags {
		s.Tags = append(s.Tags, cfntypes.Tag{
			Key:   awssdk.String(k),
			Value: awssdk.String(v),
		})
	}
	return &aws.Stack{Stack: s}
}

func TestResolveOptions(t *testing.T) {
	const clusterName = "my-cluster"

	t.Run("auto-detection fargate", func(t *testing.T) {
		stack := stackFixture(
			map[string]string{
				"KarpenterNamespace": "dd-karpenter",
				"FargateSubnets":     "subnet-bbb,subnet-aaa",
			},
			map[string]string{apply.InstallModeTagKey: string(apply.InstallModeFargate)},
		)
		o := &options{
			karpenterVersion:         "1.0.0",
			createKarpenterResources: apply.CreateKarpenterResourcesNone,
			inferenceMethod:          apply.InferenceMethodNodeGroups,
		}

		opts, err := o.resolveOptions(clusterName, stack)
		require.NoError(t, err)

		assert.Equal(t, clusterName, opts.ClusterName)
		assert.Equal(t, "dd-karpenter", opts.KarpenterNamespace)
		assert.Equal(t, apply.InstallModeFargate, opts.InstallMode)
		assert.Equal(t, []string{"subnet-aaa", "subnet-bbb"}, opts.FargateSubnets,
			"FargateSubnets must be sorted so reruns produce identical CFN parameter values")
		assert.Equal(t, "1.0.0", opts.KarpenterVersion)
		assert.Equal(t, apply.CreateKarpenterResourcesNone, opts.CreateKarpenterResources)
		assert.Equal(t, apply.InferenceMethodNodeGroups, opts.InferenceMethod)
		assert.Equal(t, "Updating", opts.ActionLabel)
	})

	t.Run("auto-detection existing-nodes", func(t *testing.T) {
		stack := stackFixture(
			map[string]string{"KarpenterNamespace": "kube-system"},
			map[string]string{apply.InstallModeTagKey: string(apply.InstallModeExistingNodes)},
		)
		o := &options{
			createKarpenterResources: apply.CreateKarpenterResourcesAll,
			inferenceMethod:          apply.InferenceMethodNodes,
		}

		opts, err := o.resolveOptions(clusterName, stack)
		require.NoError(t, err)

		assert.Equal(t, "kube-system", opts.KarpenterNamespace)
		assert.Equal(t, apply.InstallModeExistingNodes, opts.InstallMode)
		assert.Nil(t, opts.FargateSubnets,
			"existing-nodes mode never carries fargate subnets")
	})

	t.Run("legacy stack without install-mode tag defaults to existing-nodes", func(t *testing.T) {
		stack := stackFixture(
			map[string]string{"KarpenterNamespace": "dd-karpenter"},
			nil, // no install-mode tag
		)
		o := &options{
			createKarpenterResources: apply.CreateKarpenterResourcesNone,
			inferenceMethod:          apply.InferenceMethodNodeGroups,
		}

		opts, err := o.resolveOptions(clusterName, stack)
		require.NoError(t, err)

		assert.Equal(t, apply.InstallModeExistingNodes, opts.InstallMode,
			"pre-tag stacks must be treated as install-mode=existing-nodes for backward compat")
	})

	t.Run("corrupt install-mode tag", func(t *testing.T) {
		stack := stackFixture(
			map[string]string{"KarpenterNamespace": "dd-karpenter"},
			map[string]string{apply.InstallModeTagKey: "garbage"},
		)
		o := &options{}

		_, err := o.resolveOptions(clusterName, stack)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported install-mode tag")
	})

	t.Run("missing KarpenterNamespace parameter", func(t *testing.T) {
		stack := stackFixture(
			nil,
			map[string]string{apply.InstallModeTagKey: string(apply.InstallModeFargate)},
		)
		o := &options{}

		_, err := o.resolveOptions(clusterName, stack)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no KarpenterNamespace parameter")
		assert.Contains(t, err.Error(), "install state is inconsistent")
	})

	t.Run("fargate stack without FargateSubnets parameter is tolerated", func(t *testing.T) {
		// A legacy fargate stack predating the FargateSubnets parameter would
		// still be readable by update; downstream CFN replay will recompute or
		// fail loudly at template-apply time.
		stack := stackFixture(
			map[string]string{"KarpenterNamespace": "dd-karpenter"},
			map[string]string{apply.InstallModeTagKey: string(apply.InstallModeFargate)},
		)
		o := &options{}

		opts, err := o.resolveOptions(clusterName, stack)
		require.NoError(t, err)
		assert.Equal(t, apply.InstallModeFargate, opts.InstallMode)
		assert.Nil(t, opts.FargateSubnets)
	})
}

func TestValidate(t *testing.T) {
	t.Run("rejects positional arguments", func(t *testing.T) {
		o := &options{
			args:                     []string{"oops"},
			createKarpenterResources: apply.CreateKarpenterResourcesNone,
			inferenceMethod:          apply.InferenceMethodNodeGroups,
		}
		err := o.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no arguments are allowed")
	})

	t.Run("accepts valid combinations", func(t *testing.T) {
		o := &options{
			createKarpenterResources: apply.CreateKarpenterResourcesAll,
			inferenceMethod:          apply.InferenceMethodNodes,
		}
		assert.NoError(t, o.validate())
	})

	t.Run("rejects unknown create-karpenter-resources", func(t *testing.T) {
		o := &options{
			createKarpenterResources: apply.CreateKarpenterResources("garbage"),
			inferenceMethod:          apply.InferenceMethodNodeGroups,
		}
		err := o.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create-karpenter-resources must be one of")
	})

	t.Run("rejects unknown inference-method", func(t *testing.T) {
		o := &options{
			createKarpenterResources: apply.CreateKarpenterResourcesNone,
			inferenceMethod:          apply.InferenceMethod("garbage"),
		}
		err := o.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "inference-method must be one of")
	})
}
