package install

import (
	"fmt"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/apply"
)

// installMode is the pflag.Value wrapper used to bind the install command's
// --install-mode flag. Only `install` exposes that flag; `update` auto-detects
// the install mode from the CloudFormation stack and never accepts it as a
// flag, so the wrapper lives in this package rather than in `apply`.
type installMode apply.InstallMode

// String returns the string representation of the install mode.
func (i *installMode) String() string {
	return string(*i)
}

// Set parses a string into an install mode, rejecting unknown values.
func (i *installMode) Set(s string) error {
	switch apply.InstallMode(s) {
	case apply.InstallModeFargate, apply.InstallModeExistingNodes:
		*i = installMode(s)
		return nil
	default:
		return fmt.Errorf("install-mode must be one of fargate or existing-nodes")
	}
}

// Type returns the type name surfaced by pflag in usage strings.
func (*installMode) Type() string {
	return "InstallMode"
}
