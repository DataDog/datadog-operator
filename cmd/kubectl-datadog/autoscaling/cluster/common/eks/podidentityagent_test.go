package eks

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEKSAddons struct {
	out *eks.DescribeAddonOutput
	err error

	gotInput *eks.DescribeAddonInput
}

func (f *fakeEKSAddons) DescribeAddon(_ context.Context, params *eks.DescribeAddonInput, _ ...func(*eks.Options)) (*eks.DescribeAddonOutput, error) {
	f.gotInput = params
	return f.out, f.err
}

// describeAddonError mimics what the SDK returns: the API error wrapped in an
// operation error.
func describeAddonError(err error) error {
	return &smithy.OperationError{ServiceID: "EKS", OperationName: "DescribeAddon", Err: err}
}

func TestIsThereUnmanagedEKSPodIdentityAgentInstalled(t *testing.T) {
	const (
		clusterName = "my-cluster"
		addonName   = "eks-pod-identity-agent"
	)

	addon := func(tags map[string]string) *eks.DescribeAddonOutput {
		return &eks.DescribeAddonOutput{Addon: &ekstypes.Addon{AddonName: aws.String(addonName), Tags: tags}}
	}

	tests := []struct {
		name          string
		client        *fakeEKSAddons
		wantInstalled bool
		wantErr       bool
	}{
		{
			name: "addon not installed: message returned by EKS today",
			client: &fakeEKSAddons{err: describeAddonError(
				&ekstypes.ResourceNotFoundException{Message: aws.String("The requested resource does not exist.")},
			)},
			wantInstalled: false,
			wantErr:       false,
		},
		{
			name: "addon not installed: legacy EKS message",
			client: &fakeEKSAddons{err: describeAddonError(
				&ekstypes.ResourceNotFoundException{Message: aws.String("No addon: eks-pod-identity-agent found in cluster: my-cluster")},
			)},
			wantInstalled: false,
			wantErr:       false,
		},
		{
			name: "addon not installed: untyped API error carrying the not-found code",
			client: &fakeEKSAddons{err: describeAddonError(
				&smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "anything"},
			)},
			wantInstalled: false,
			wantErr:       false,
		},
		{
			name:          "unmanaged addon installed",
			client:        &fakeEKSAddons{out: addon(nil)},
			wantInstalled: true,
			wantErr:       false,
		},
		{
			name:          "addon tagged by another tool",
			client:        &fakeEKSAddons{out: addon(map[string]string{"managed-by": "terraform"})},
			wantInstalled: true,
			wantErr:       false,
		},
		{
			name:          "addon installed by kubectl-datadog",
			client:        &fakeEKSAddons{out: addon(map[string]string{"managed-by": "dd-karpenter"})},
			wantInstalled: false,
			wantErr:       false,
		},
		{
			name: "other API error is returned",
			client: &fakeEKSAddons{err: describeAddonError(
				&smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not allowed"},
			)},
			wantInstalled: false,
			wantErr:       true,
		},
		{
			name:          "non API error is returned",
			client:        &fakeEKSAddons{err: errors.New("connection reset")},
			wantInstalled: false,
			wantErr:       true,
		},
		{
			name:          "empty response is an error",
			client:        &fakeEKSAddons{out: &eks.DescribeAddonOutput{}},
			wantInstalled: false,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installed, err := IsThereUnmanagedEKSPodIdentityAgentInstalled(t.Context(), tt.client, clusterName)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorContains(t, err, clusterName)
				assert.ErrorContains(t, err, addonName)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantInstalled, installed)

			require.NotNil(t, tt.client.gotInput)
			assert.Equal(t, clusterName, aws.ToString(tt.client.gotInput.ClusterName))
			assert.Equal(t, addonName, aws.ToString(tt.client.gotInput.AddonName))
		})
	}
}

func TestIsThereUnmanagedEKSPodIdentityAgentInstalledWrapsAPIError(t *testing.T) {
	apiErr := &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not allowed"}
	client := &fakeEKSAddons{err: describeAddonError(apiErr)}

	_, err := IsThereUnmanagedEKSPodIdentityAgentInstalled(t.Context(), client, "my-cluster")

	require.ErrorIs(t, err, apiErr)
}
