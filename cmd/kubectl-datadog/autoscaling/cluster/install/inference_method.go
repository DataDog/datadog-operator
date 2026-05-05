package install

import "fmt"

// InferenceMethod defines how to infer EC2NodeClass and NodePool properties
type InferenceMethod string

const (
	// InferenceMethodNodes infers properties from existing Kubernetes nodes
	InferenceMethodNodes InferenceMethod = "nodes"
	// InferenceMethodNodeGroups infers properties from EKS node groups
	InferenceMethodNodeGroups InferenceMethod = "nodegroups"
)

// String returns the string representation of the InferenceMethod
func (i *InferenceMethod) String() string {
	return string(*i)
}

// Set sets the InferenceMethod value from a string
func (i *InferenceMethod) Set(s string) error {
	switch s {
	case "nodes":
		*i = InferenceMethodNodes
	case "nodegroups":
		*i = InferenceMethodNodeGroups
	default:
		return fmt.Errorf("inference-method must be one of nodes or nodegroups")
	}

	return nil
}

// Type returns the type name for pflag
func (_ *InferenceMethod) Type() string {
	return "InferenceMethod"
}
