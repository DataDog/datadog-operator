package install

import "fmt"

// InstallMode defines how to run the Karpenter controller.
type InstallMode string

const (
	// InstallModeFargate runs the Karpenter controller on dedicated Fargate nodes.
	InstallModeFargate InstallMode = "fargate"
	// InstallModeExistingNodes runs the Karpenter controller on existing cluster nodes.
	InstallModeExistingNodes InstallMode = "existing-nodes"
)

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
func (_ *InstallMode) Type() string {
	return "InstallMode"
}
