package apply

import (
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
