package apply

import (
	"fmt"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
)

// InstallMode defines how to run the Karpenter controller.
type InstallMode string

const (
	// InstallModeFargate runs the Karpenter controller on dedicated Fargate nodes.
	InstallModeFargate InstallMode = "fargate"
	// InstallModeExistingNodes runs the Karpenter controller on existing cluster nodes.
	InstallModeExistingNodes InstallMode = "existing-nodes"
)

// InstallModeTagKey is the CloudFormation stack tag tracking the deployment's
// install-mode. Stacks created before this tag was introduced have no tag and
// are treated as install-mode=existing-nodes.
const InstallModeTagKey = "install-mode"

// String returns the string representation of the InstallMode.
func (i *InstallMode) String() string {
	return string(*i)
}

// Set sets the InstallMode value from a string.
func (i *InstallMode) Set(s string) error {
	switch s {
	case "fargate":
		*i = InstallModeFargate
	case "existing-nodes":
		*i = InstallModeExistingNodes
	default:
		return fmt.Errorf("install-mode must be one of fargate or existing-nodes")
	}

	return nil
}

// Type returns the type name for pflag.
func (*InstallMode) Type() string {
	return "InstallMode"
}

// DetectedInstallMode reads the install-mode tag from a CFN stack. Stacks
// created before this tag was introduced have no tag and default to
// existing-nodes for backward compatibility.
func DetectedInstallMode(stack *aws.Stack) InstallMode {
	if stack == nil {
		return ""
	}
	if tag, ok := stack.TagMap()[InstallModeTagKey]; ok && tag != "" {
		return InstallMode(tag)
	}
	return InstallModeExistingNodes
}
