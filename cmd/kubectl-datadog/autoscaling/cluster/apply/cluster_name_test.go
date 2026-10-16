package apply

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common"
)

// TestMaxClusterNameLength checks common.MaxClusterNameLength against the name of every
// AWS resource derived from the cluster name in the embedded templates, so that a
// template change (for instance re-syncing karpenter.yaml with upstream) that
// lowers the limit, or makes it needlessly low, is caught here and not by a
// failing install.
func TestMaxClusterNameLength(t *testing.T) {
	const clusterNameToken = "${ClusterName}"

	// Properties that hold a resource name, with the length limit AWS puts on it.
	nameLimits := map[string]int{
		"RoleName":           64,  // IAM role
		"ManagedPolicyName":  128, // IAM managed policy
		"QueueName":          80,  // SQS queue
		"FargateProfileName": 100, // EKS Fargate profile
	}

	type nameConstraint struct {
		resource  string
		maxLength int // longest cluster name that keeps the resource name within its limit
	}

	var constraints []nameConstraint
	add := func(resource, pattern string, limit int) {
		occurrences := strings.Count(pattern, clusterNameToken)
		fixed := strings.ReplaceAll(pattern, clusterNameToken, "")
		require.NotContains(t, fixed, "${", "%s: unexpected substitution in %q, extend this test", resource, pattern)
		constraints = append(constraints, nameConstraint{resource: resource, maxLength: (limit - len(fixed)) / occurrences})
	}

	for templateName, template := range map[string]string{
		"karpenter.yaml":            KarpenterCfn,
		"dd-karpenter.yaml":         DdKarpenterCfn,
		"dd-karpenter-fargate.yaml": DdKarpenterFargateCfn,
	} {
		var doc yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(template), &doc), templateName)
		require.NotEmpty(t, doc.Content, templateName)

		found := 0
		resources := mappingValue(doc.Content[0], "Resources")
		require.NotNil(t, resources, "%s has no Resources", templateName)
		for i := 0; i+1 < len(resources.Content); i += 2 {
			logicalID := resources.Content[i].Value
			properties := mappingValue(resources.Content[i+1], "Properties")
			if properties == nil {
				continue
			}
			for j := 0; j+1 < len(properties.Content); j += 2 {
				property, value := properties.Content[j].Value, properties.Content[j+1]
				limit, isName := nameLimits[property]
				if !isName || !strings.Contains(value.Value, clusterNameToken) {
					continue
				}
				add(fmt.Sprintf("%s: %s.%s (%s)", templateName, logicalID, property, value.Value), value.Value, limit)
				found++
			}
		}
		assert.Positive(t, found, "%s: no resource name derived from the cluster name found, the test is not checking anything", templateName)
	}

	// CloudFormation stack names (limit 128), built in Go.
	add("stack "+KarpenterStackName(clusterNameToken), KarpenterStackName(clusterNameToken), 128)
	add("stack "+DDKarpenterStackName(clusterNameToken), DDKarpenterStackName(clusterNameToken), 128)

	tightest := constraints[0]
	for _, c := range constraints {
		if c.maxLength < tightest.maxLength {
			tightest = c
		}
	}

	assert.Equal(t, tightest.maxLength, common.MaxClusterNameLength,
		"common.MaxClusterNameLength does not match the templates: the tightest constraint is %s", tightest.resource)
}

// mappingValue returns the value stored under key in a YAML mapping node, or nil.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
