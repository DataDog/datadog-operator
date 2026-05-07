package apply

import "fmt"

// CreateKarpenterResources defines which Karpenter resources to create
type CreateKarpenterResources string

const (
	// CreateKarpenterResourcesNone does not create any Karpenter resources
	CreateKarpenterResourcesNone CreateKarpenterResources = "none"
	// CreateKarpenterResourcesEC2NodeClass creates only EC2NodeClass resources
	CreateKarpenterResourcesEC2NodeClass CreateKarpenterResources = "ec2nodeclass"
	// CreateKarpenterResourcesAll creates both EC2NodeClass and NodePool resources
	CreateKarpenterResourcesAll CreateKarpenterResources = "all"
)

// String returns the string representation of CreateKarpenterResources
func (c *CreateKarpenterResources) String() string {
	return string(*c)
}

// Set sets the CreateKarpenterResources value from a string
func (c *CreateKarpenterResources) Set(s string) error {
	switch s {
	case "none":
		*c = CreateKarpenterResourcesNone
	case "ec2nodeclass":
		*c = CreateKarpenterResourcesEC2NodeClass
	case "all":
		*c = CreateKarpenterResourcesAll
	default:
		return fmt.Errorf("create-karpenter-resources must be one of none, ec2nodeclass or all")
	}

	return nil
}

// Type returns the type name for pflag
func (_ *CreateKarpenterResources) Type() string {
	return "CreateKarpenterResources"
}
