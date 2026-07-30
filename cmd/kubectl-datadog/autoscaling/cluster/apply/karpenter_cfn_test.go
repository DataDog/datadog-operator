package apply

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// iamManagedPolicyMaxChars is AWS IAM's hard limit on a managed policy's
// PolicyDocument size, measured with whitespace removed. It cannot be raised
// via a quota increase. See CASCL-1645.
const iamManagedPolicyMaxChars = 6144

// eksClusterNameMaxChars is the documented maximum length of an EKS cluster
// name (see (*eks.Client).CreateCluster's ClusterName field): "must start
// with an alphanumeric character and can't be longer than 100 characters".
const eksClusterNameMaxChars = 100

// iamPolicyNameMaxChars is AWS IAM's hard limit on a managed policy's
// friendly name (distinct from the PolicyDocument size limit above). Both
// "KarpenterControllerPolicy-${ClusterName}" and
// "KarpenterControllerPolicy2-${ClusterName}" stay under this limit even at
// the true 100-char EKS cluster-name maximum, but with only 1-2 characters
// of headroom — this test exists specifically so that renaming either
// policy to something longer/more descriptive doesn't silently reintroduce
// a ServiceLimitExceeded-style failure for near-maximum cluster names, just
// via the policy *name* instead of its *document*.
const iamPolicyNameMaxChars = 128

// extractSubJSONBlock extracts and dedents the JSON literal following a
// "<key>: !Sub |" block scalar in one of the embedded Karpenter CloudFormation
// templates. It mirrors how CloudFormation reads YAML block scalars: every
// contiguous line more indented than the block's own key is part of the
// literal, dedented by the indentation of the first such line.
func extractSubJSONBlock(t *testing.T, cfn, marker string) string {
	t.Helper()
	markerIdx := strings.Index(cfn, marker)
	require.GreaterOrEqualf(t, markerIdx, 0, "marker %q not found in template", marker)

	const blockKey = "PolicyDocument: !Sub |"
	blockIdx := strings.Index(cfn[markerIdx:], blockKey)
	require.GreaterOrEqualf(t, blockIdx, 0, "%q not found after marker %q", blockKey, marker)
	idx := markerIdx + blockIdx

	rest := cfn[idx:]
	lines := strings.Split(rest, "\n")
	require.Greater(t, len(lines), 1, "marker line must be followed by a block scalar body")

	var indent string
	var body []string
	for _, line := range lines[1:] {
		if indent == "" {
			trimmed := strings.TrimLeft(line, " ")
			if trimmed == "" {
				continue
			}
			indent = line[:len(line)-len(trimmed)]
		}
		if !strings.HasPrefix(line, indent) {
			break
		}
		body = append(body, strings.TrimPrefix(line, indent))
	}
	require.NotEmpty(t, body, "block scalar following %q must not be empty", marker)
	return strings.Join(body, "\n")
}

// renderPolicyWorstCase substitutes every CloudFormation intrinsic token
// found in the Karpenter controller policy documents with the longest
// realistic value it could take in production, so the resulting length is a
// safe upper bound on what IAM will ever actually measure.
func renderPolicyWorstCase(policyJSON, clusterName string) string {
	const (
		// "aws-us-gov" (10 chars) is the longest AWS partition string.
		partition = "aws-us-gov"
		// "ap-southeast-1" (14 chars) is among the longest AWS region strings.
		region = "ap-southeast-1"
		// AWS account IDs are always exactly 12 digits.
		accountID = "123456789012"
	)
	queueArn := fmt.Sprintf("arn:%s:sqs:%s:%s:%s", partition, region, accountID, clusterName)
	nodeRoleArn := fmt.Sprintf("arn:%s:iam::%s:role/KarpenterNodeRole-%s", partition, accountID, clusterName)

	s := policyJSON
	s = strings.ReplaceAll(s, "${ClusterName}", clusterName)
	s = strings.ReplaceAll(s, "${AWS::Region}", region)
	s = strings.ReplaceAll(s, "${AWS::Partition}", partition)
	s = strings.ReplaceAll(s, "${AWS::AccountId}", accountID)
	s = strings.ReplaceAll(s, "${KarpenterInterruptionQueue.Arn}", queueArn)
	s = strings.ReplaceAll(s, "${KarpenterNodeRole.Arn}", nodeRoleArn)
	return s
}

// minifyLikeIAM strips all whitespace outside of JSON string literals, which
// is how IAM measures a policy document's size against its quota.
func minifyLikeIAM(s string) string {
	var out strings.Builder
	inString, escaped := false, false
	for _, ch := range s {
		if inString {
			out.WriteRune(ch)
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch {
		case ch == '"':
			inString = true
			out.WriteRune(ch)
		case ch == ' ' || ch == '\n' || ch == '\t' || ch == '\r':
			// dropped
		default:
			out.WriteRune(ch)
		}
	}
	return out.String()
}

type iamStatement struct {
	Sid string `json:"Sid"`
}

type iamPolicyDocument struct {
	Version   string         `json:"Version"`
	Statement []iamStatement `json:"Statement"`
}

// TestKarpenterControllerPolicySize guards against a regression of CASCL-1645
// (KarpenterControllerPolicy exceeding IAM's 6,144-character managed-policy
// size limit for long EKS cluster names, causing ServiceLimitExceeded). The
// controller's permissions are split across KarpenterControllerPolicy and
// KarpenterControllerPolicy2 specifically so that, combined, they always fit
// under the quota — this test renders both against the worst realistic
// substitution values (a maximum-length 100-character EKS cluster name, the
// longest AWS partition/region strings) and asserts each stays within limit.
func TestKarpenterControllerPolicySize(t *testing.T) {
	policyAJSON := extractSubJSONBlock(t, KarpenterCfn, `ManagedPolicyName: !Sub "KarpenterControllerPolicy-${ClusterName}"`)
	policyBJSON := extractSubJSONBlock(t, KarpenterCfn, `ManagedPolicyName: !Sub "KarpenterControllerPolicy2-${ClusterName}"`)

	var docA, docB iamPolicyDocument
	require.NoError(t, json.Unmarshal([]byte(policyAJSON), &docA), "KarpenterControllerPolicy PolicyDocument must be valid JSON")
	require.NoError(t, json.Unmarshal([]byte(policyBJSON), &docB), "KarpenterControllerPolicy2 PolicyDocument must be valid JSON")

	// Every statement must land in exactly one of the two policies: no
	// duplicates, no accidental drops when the policy was split.
	seen := map[string]int{}
	for _, s := range append(append([]iamStatement{}, docA.Statement...), docB.Statement...) {
		require.NotEmpty(t, s.Sid, "every statement must have a Sid")
		seen[s.Sid]++
	}
	for sid, count := range seen {
		assert.Equalf(t, 1, count, "statement %q must appear in exactly one of the two policies", sid)
	}

	clusterName := strings.Repeat("c", eksClusterNameMaxChars)

	renderedA := minifyLikeIAM(renderPolicyWorstCase(policyAJSON, clusterName))
	renderedB := minifyLikeIAM(renderPolicyWorstCase(policyBJSON, clusterName))

	assert.LessOrEqualf(t, len(renderedA), iamManagedPolicyMaxChars,
		"KarpenterControllerPolicy must stay under IAM's %d-char limit even for a %d-char (max) EKS cluster name; got %d chars",
		iamManagedPolicyMaxChars, eksClusterNameMaxChars, len(renderedA))
	assert.LessOrEqualf(t, len(renderedB), iamManagedPolicyMaxChars,
		"KarpenterControllerPolicy2 must stay under IAM's %d-char limit even for a %d-char (max) EKS cluster name; got %d chars",
		iamManagedPolicyMaxChars, eksClusterNameMaxChars, len(renderedB))
}

// TestKarpenterControllerPolicyNameLength guards the *name* length of both
// managed policies (as opposed to TestKarpenterControllerPolicySize, which
// guards the PolicyDocument content length): IAM caps a managed policy's
// friendly name at 128 characters, and both policy name prefixes leave only
// 1-2 characters of headroom at the true 100-char EKS cluster-name maximum.
// This test exists specifically so that renaming either policy to something
// longer doesn't silently reintroduce a ServiceLimitExceeded-style failure
// for long cluster names.
func TestKarpenterControllerPolicyNameLength(t *testing.T) {
	clusterName := strings.Repeat("c", eksClusterNameMaxChars)

	for _, prefix := range []string{"KarpenterControllerPolicy-", "KarpenterControllerPolicy2-"} {
		name := prefix + clusterName
		assert.LessOrEqualf(t, len(name), iamPolicyNameMaxChars,
			"managed policy name %q must stay under IAM's %d-char name limit even for a %d-char (max) EKS cluster name; got %d chars",
			prefix+"${ClusterName}", iamPolicyNameMaxChars, eksClusterNameMaxChars, len(name))
	}
}

// TestKarpenterControllerPolicyReferencedByBothStacks asserts that both the
// existing-nodes and Fargate mode-specific stacks attach *both*
// KarpenterControllerPolicy and KarpenterControllerPolicy2 to their
// respective controller role. This is the backward-compatibility contract
// for CASCL-1645: the change must be purely additive (a second
// ManagedPolicyArns entry) so that `kubectl datadog autoscaling cluster
// update` and `... uninstall` keep working against stacks created before
// this change existed — CloudFormation updates/deletes the whole stack by
// name regardless of how many resources it contains.
func TestKarpenterControllerPolicyReferencedByBothStacks(t *testing.T) {
	for name, cfn := range map[string]string{
		"dd-karpenter.yaml":         DdKarpenterCfn,
		"dd-karpenter-fargate.yaml": DdKarpenterFargateCfn,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, cfn, "policy/KarpenterControllerPolicy-${ClusterName}",
				"must keep attaching the original KarpenterControllerPolicy so existing stacks stay valid")
			assert.Contains(t, cfn, "policy/KarpenterControllerPolicy2-${ClusterName}",
				"must additionally attach the new KarpenterControllerPolicy2")
		})
	}
}
